// Package ambient provides the autonomous holon runtime engine and conscious swarm coordination.
package ambient

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// Default constants for swarm work-claim event loop and task lifecycle.
const (
	DefaultSwarmMaxRetries       = 3
	DefaultSwarmMaxClaimsPerSeat = 2
	DefaultSwarmEventBufferSize  = 128
)

// SwarmTaskEventType represents the state transition lifecycle of a task lease event.
type SwarmTaskEventType string

const (
	TaskEventClaimed     SwarmTaskEventType = "claimed"
	TaskEventHeartbeat   SwarmTaskEventType = "heartbeat"
	TaskEventYielded     SwarmTaskEventType = "yielded"
	TaskEventCompleted   SwarmTaskEventType = "completed"
	TaskEventRevoked     SwarmTaskEventType = "revoked"
	TaskEventQuarantined SwarmTaskEventType = "quarantined"
)

// IsValid returns true if the event type is one of the recognized enum variants.
func (t SwarmTaskEventType) IsValid() bool {
	switch t {
	case TaskEventClaimed, TaskEventHeartbeat, TaskEventYielded,
		TaskEventCompleted, TaskEventRevoked, TaskEventQuarantined:
		return true
	default:
		return false
	}
}

// SwarmTaskEvent represents a telemetry event emitted during task lifecycle transitions.
type SwarmTaskEvent struct {
	ID         string             `json:"id"`
	EventType  SwarmTaskEventType `json:"event_type"`
	TaskID     string             `json:"task_id"`
	SeatID     string             `json:"seat_id,omitempty"`
	RetryCount int                `json:"retry_count,omitempty"`
	Reason     string             `json:"reason,omitempty"`
	Timestamp  time.Time          `json:"timestamp"`
}

// Sentinel errors for swarm claim loop operations.
var (
	ErrSeatMismatch           = errfmt.Errorf("seat does not own task lease")
	ErrSeatCapacityReached    = errfmt.Errorf("seat max concurrent claim capacity reached")
	ErrTaskAlreadyQuarantined = errfmt.Errorf("task has exceeded max retries and is quarantined")
	ErrInvalidEventBuffer     = errfmt.Errorf("event buffer size must be strictly positive")
	ErrInvalidMaxRetries      = errfmt.Errorf("max retries cannot be negative")
	ErrInvalidClaimsPerSeat   = errfmt.Errorf("max claims per seat must be strictly positive")
)

// SwarmLoopState encapsulates the queue, retries, and event streaming state of the claim loop.
type SwarmLoopState struct {
	mu          sync.RWMutex
	unassigned  []string
	retryCounts map[string]int
	quarantine  map[string]string
	eventCh     chan SwarmTaskEvent
}

// NewSwarmLoopState constructs an initialized SwarmLoopState.
func NewSwarmLoopState(bufferSize int) *SwarmLoopState {
	if bufferSize <= 0 {
		bufferSize = DefaultSwarmEventBufferSize
	}
	return &SwarmLoopState{
		unassigned:  make([]string, 0),
		retryCounts: make(map[string]int),
		quarantine:  make(map[string]string),
		eventCh:     make(chan SwarmTaskEvent, bufferSize),
	}
}

// EventStream returns the receive-only channel for published task lifecycle events.
func (s *SwarmLoopState) EventStream() <-chan SwarmTaskEvent {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.eventCh
}

// PublishEvent emits a task event non-blockingly to the event channel.
func (s *SwarmLoopState) PublishEvent(evt SwarmTaskEvent) bool {
	if s == nil || !evt.EventType.IsValid() {
		return false
	}
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now()
	}
	s.mu.RLock()
	ch := s.eventCh
	s.mu.RUnlock()
	if ch == nil {
		return false
	}
	select {
	case ch <- evt:
		return true
	default:
		return false
	}
}

// Enqueue adds a task ID to the unassigned queue if not quarantined or already queued.
func (s *SwarmLoopState) Enqueue(taskID string) bool {
	if s == nil || strings.TrimSpace(taskID) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, quarantined := s.quarantine[taskID]; quarantined {
		return false
	}
	for _, id := range s.unassigned {
		if id == taskID {
			return false
		}
	}
	s.unassigned = append(s.unassigned, taskID)
	return true
}

// NextUnassigned returns and removes the first unassigned task from the queue.
func (s *SwarmLoopState) NextUnassigned() (string, bool) {
	if s == nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.unassigned) == 0 {
		return "", false
	}
	taskID := s.unassigned[0]
	s.unassigned = s.unassigned[1:]
	return taskID, true
}

func withLoopRLock[T any](s *SwarmLoopState, fn func(*SwarmLoopState) T) T {
	var zero T
	if s == nil {
		return zero
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fn(s)
}

// UnassignedCount returns the count of items in the unassigned queue.
func (s *SwarmLoopState) UnassignedCount() int {
	return withLoopRLock(s, func(state *SwarmLoopState) int {
		return len(state.unassigned)
	})
}

// RecordRetry increments retry count and reports if max retries is exceeded.
func (s *SwarmLoopState) RecordRetry(taskID, reason string, maxRetries int) (int, bool) {
	if s == nil {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	count := s.retryCounts[taskID] + 1
	s.retryCounts[taskID] = count
	if count > maxRetries {
		s.quarantine[taskID] = reason
		return count, true
	}
	s.unassigned = append(s.unassigned, taskID)
	return count, false
}

// IsQuarantined checks if a task is currently quarantined.
func (s *SwarmLoopState) IsQuarantined(taskID string) (bool, string) {
	if s == nil {
		return false, ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	reason, found := s.quarantine[taskID]
	return found, reason
}

// GetRetries returns the retry count for a given task.
func (s *SwarmLoopState) GetRetries(taskID string) int {
	return withLoopRLock(s, func(state *SwarmLoopState) int {
		return state.retryCounts[taskID]
	})
}

// LoopState returns the underlying SwarmLoopState. Returns nil if receiver is nil.
func (o *BacklogObserver) LoopState() *SwarmLoopState {
	if o == nil {
		return nil
	}
	return o.loopState
}

// EventStream returns the receive-only channel for published task lifecycle events.
func (o *BacklogObserver) EventStream() <-chan SwarmTaskEvent {
	if o == nil || o.loopState == nil {
		return nil
	}
	return o.loopState.EventStream()
}

// PublishEvent emits a task event non-blockingly.
func (o *BacklogObserver) PublishEvent(evt SwarmTaskEvent) bool {
	if o == nil || o.loopState == nil {
		return false
	}
	return o.loopState.PublishEvent(evt)
}

// EnqueueTask adds a task ID to the unassigned queue and proposes it to ConsensusArbiter if needed.
func (o *BacklogObserver) EnqueueTask(taskID string) bool {
	if o == nil || o.loopState == nil || strings.TrimSpace(taskID) == "" {
		return false
	}
	if !o.loopState.Enqueue(taskID) {
		return false
	}
	if o.arbiter != nil {
		if _, err := o.arbiter.ProposeTask(taskID, nil); err != nil && !errors.Is(err, ErrTaskAlreadyExists) {
			return false
		}
	}
	return true
}

func (o *BacklogObserver) releaseHeldLease(taskID, seatID string) error {
	if o == nil {
		return ErrNilBacklogObserver
	}
	if strings.TrimSpace(taskID) == "" {
		return ErrInvalidTaskID
	}
	if strings.TrimSpace(seatID) == "" {
		return ErrInvalidWorkerID
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	lease, found := o.leases[taskID]
	if !found {
		return ErrTaskNotLeased
	}
	if lease.PeerSeatID != seatID {
		return ErrSeatMismatch
	}
	delete(o.leases, taskID)
	return nil
}

// CompleteTask finalizes task execution by the holding peer seat and commits the consensus state.
func (o *BacklogObserver) CompleteTask(taskID, seatID string) error {
	if err := o.releaseHeldLease(taskID, seatID); err != nil {
		return err
	}

	if o.arbiter != nil {
		if err := o.arbiter.CommitTask(taskID, seatID); err != nil {
			return err
		}
	}

	o.PublishEvent(SwarmTaskEvent{
		ID:        "evt-complete-" + taskID,
		EventType: TaskEventCompleted,
		TaskID:    taskID,
		SeatID:    seatID,
		Timestamp: time.Now(),
	})
	return nil
}

// YieldTask relinquishes a task lease, revokes consensus, and handles retry/quarantine escalation.
func (o *BacklogObserver) YieldTask(taskID, seatID, reason string) error {
	if err := o.releaseHeldLease(taskID, seatID); err != nil {
		return err
	}

	if o.arbiter != nil {
		if err := o.arbiter.RevokeTask(taskID, reason); err != nil {
			return err
		}
	}

	maxRetries := o.config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = DefaultSwarmMaxRetries
	}

	retries, quarantined := o.loopState.RecordRetry(taskID, reason, maxRetries)
	evtType := TaskEventYielded
	if quarantined {
		evtType = TaskEventQuarantined
	}

	o.PublishEvent(SwarmTaskEvent{
		ID:         "evt-yield-" + taskID,
		EventType:  evtType,
		TaskID:     taskID,
		SeatID:     seatID,
		RetryCount: retries,
		Reason:     reason,
		Timestamp:  time.Now(),
	})
	return nil
}

// AutoClaimTick matches unassigned tasks with healthy seats up to MaxClaimsPerSeat.
func (o *BacklogObserver) AutoClaimTick(ctx context.Context) int {
	if o == nil || (ctx != nil && ctx.Err() != nil) {
		return 0
	}
	seats := o.HealthySeats()
	if len(seats) == 0 {
		return 0
	}

	maxPerSeat := o.config.MaxClaimsPerSeat
	if maxPerSeat <= 0 {
		maxPerSeat = DefaultSwarmMaxClaimsPerSeat
	}

	counts := o.seatLeaseCounts()
	claimed := 0
	var unhandled []string

	for {
		taskID, ok := o.loopState.NextUnassigned()
		if !ok {
			break
		}
		seatID := o.findEligibleSeat(seats, counts, maxPerSeat)
		if seatID == "" {
			unhandled = append(unhandled, taskID)
			break
		}
		lease, err := o.DispatchLease(taskID, seatID, o.config.LeaseDuration)
		if err != nil {
			unhandled = append(unhandled, taskID)
			continue
		}
		counts[seatID]++
		claimed++
		o.PublishEvent(SwarmTaskEvent{
			ID:        "evt-claim-" + lease.ID,
			EventType: TaskEventClaimed,
			TaskID:    taskID,
			SeatID:    seatID,
			Timestamp: time.Now(),
		})
	}

	for _, taskID := range unhandled {
		o.loopState.Enqueue(taskID)
	}
	return claimed
}

func (o *BacklogObserver) seatLeaseCounts() map[string]int {
	counts := make(map[string]int)
	o.mu.RLock()
	defer o.mu.RUnlock()
	for _, l := range o.leases {
		if l != nil && l.PeerSeatID != "" {
			counts[l.PeerSeatID]++
		}
	}
	return counts
}

func (o *BacklogObserver) findEligibleSeat(seats []string, counts map[string]int, maxPerSeat int) string {
	for _, seatID := range seats {
		if counts[seatID] < maxPerSeat {
			return seatID
		}
	}
	return ""
}

// IsTaskQuarantined checks if a task has exceeded max retries.
func (o *BacklogObserver) IsTaskQuarantined(taskID string) (bool, string) {
	if o == nil || o.loopState == nil {
		return false, ""
	}
	return o.loopState.IsQuarantined(taskID)
}

// GetRetryCount returns the number of retries recorded for a task.
func (o *BacklogObserver) GetRetryCount(taskID string) int {
	if o == nil || o.loopState == nil {
		return 0
	}
	return o.loopState.GetRetries(taskID)
}
