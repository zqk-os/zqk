package ambient

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Named constants enforcing bounded lease timing floors and deterministic execution.
const (
	DefaultLeaseDuration = 30 * time.Second
	DefaultReapInterval  = 100 * time.Millisecond
)

// Sentinel domain errors for consensus arbiter lifecycle and leasing operations.
var (
	ErrNilTaskLease          = errors.New("task lease cannot be nil")
	ErrInvalidTaskID         = errors.New("invalid task id")
	ErrInvalidWorkerID       = errors.New("invalid worker id")
	ErrTaskAlreadyLeased     = errors.New("task is already leased")
	ErrLeaseExpired          = errors.New("task lease has expired")
	ErrLeaseNotFound         = errors.New("task lease not found")
	ErrInvalidConsensusState = errors.New("invalid consensus state")
	ErrTaskAlreadyCommitted  = errors.New("task is already committed")
	ErrUnauthorizedWorker    = errors.New("worker is not authorized for this lease")
	ErrNoPendingTasks        = errors.New("no pending tasks available for leasing")
	ErrArbiterClosed         = errors.New("consensus arbiter is closed")
)

// ConsensusState represents the distributed lifecycle phase of a task lease.
type ConsensusState string

const (
	ConsensusStatePending   ConsensusState = "pending"
	ConsensusStateLeased    ConsensusState = "leased"
	ConsensusStateCommitted ConsensusState = "committed"
	ConsensusStateRevoked   ConsensusState = "revoked"
	ConsensusStateExpired   ConsensusState = "expired"
)

// IsValid validates whether the ConsensusState is an authorized lifecycle state.
func (s ConsensusState) IsValid() bool {
	switch s {
	case ConsensusStatePending,
		ConsensusStateLeased,
		ConsensusStateCommitted,
		ConsensusStateRevoked,
		ConsensusStateExpired:
		return true
	default:
		return false
	}
}

// String returns the string representation of the ConsensusState.
func (s ConsensusState) String() string {
	return string(s)
}

// TaskLease captures the exclusive assignment and commitment contract for a distributed task.
type TaskLease struct {
	ID            string         `json:"id"`
	TaskID        string         `json:"task_id"`
	WorkerID      string         `json:"worker_id,omitempty"`
	LeaseDuration time.Duration  `json:"lease_duration"`
	ExpiresAt     time.Time      `json:"expires_at"`
	State         ConsensusState `json:"state"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	Payload       []byte         `json:"payload,omitempty"`
}

// Validate checks whether the task lease satisfies minimum invariant contracts.
func (l *TaskLease) Validate() error {
	if l == nil {
		return ErrNilTaskLease
	}
	if strings.TrimSpace(l.TaskID) == "" {
		return ErrInvalidTaskID
	}
	if !l.State.IsValid() {
		return ErrInvalidConsensusState
	}
	return nil
}

// ArbiterOption configures runtime parameters for the consensus arbiter.
type ArbiterOption func(*AmbientConsensusArbiter)

// WithDefaultLeaseDuration overrides the fallback duration for newly granted leases.
func WithDefaultLeaseDuration(d time.Duration) ArbiterOption {
	return func(a *AmbientConsensusArbiter) {
		if d > 0 {
			a.defaultLeaseDuration = d
		}
	}
}

// WithReapInterval overrides the interval between background expiration sweeps.
func WithReapInterval(d time.Duration) ArbiterOption {
	return func(a *AmbientConsensusArbiter) {
		if d > 0 {
			a.reapInterval = d
		}
	}
}

// WithClock injects a custom time provider for deterministic testing.
func WithClock(c func() time.Time) ArbiterOption {
	return func(a *AmbientConsensusArbiter) {
		if c != nil {
			a.nowFunc = c
		}
	}
}

// AmbientConsensusArbiter manages atomic task proposals, exclusive worker leases, and consensus commits.
type AmbientConsensusArbiter struct {
	mu                   sync.RWMutex
	leases               map[string]*TaskLease
	pendingQueue         []string
	defaultLeaseDuration time.Duration
	reapInterval         time.Duration
	nowFunc              func() time.Time
	closed               bool
}

// NewConsensusArbiter creates an initialized, thread-safe AmbientConsensusArbiter.
func NewConsensusArbiter(opts ...ArbiterOption) *AmbientConsensusArbiter {
	arbiter := &AmbientConsensusArbiter{
		leases:               make(map[string]*TaskLease),
		pendingQueue:         make([]string, 0),
		defaultLeaseDuration: DefaultLeaseDuration,
		reapInterval:         DefaultReapInterval,
		nowFunc:              time.Now,
	}
	for _, opt := range opts {
		opt(arbiter)
	}
	return arbiter
}

// ProposeTask registers a new task candidate in the consensus pool in Pending state.
func (a *AmbientConsensusArbiter) ProposeTask(taskID string, payload []byte) (*TaskLease, error) {
	cleanTaskID, err := sanitizeIdentifier(a, taskID, ErrInvalidTaskID)
	if err != nil {
		return nil, err
	}

	var result *TaskLease
	err = a.execMutation(func(now time.Time) error {
		if existing, exists := a.leases[cleanTaskID]; exists {
			if existing.State == ConsensusStateCommitted {
				return ErrTaskAlreadyCommitted
			}
			if existing.State == ConsensusStateLeased && !a.isExpiredUnsafe(existing, now) {
				return ErrTaskAlreadyLeased
			}
		}

		lease := &TaskLease{
			ID:            fmt.Sprintf("lease-%s-%d", cleanTaskID, now.UnixNano()),
			TaskID:        cleanTaskID,
			LeaseDuration: a.defaultLeaseDuration,
			State:         ConsensusStatePending,
			CreatedAt:     now,
			UpdatedAt:     now,
			Payload:       append([]byte(nil), payload...),
		}
		a.leases[cleanTaskID] = lease
		a.enqueuePendingUnsafe(cleanTaskID)
		result = cloneTaskLease(lease)
		return nil
	})

	return result, err
}

// LeaseTask grants exclusive execution rights for a proposed task to the given worker.
func (a *AmbientConsensusArbiter) LeaseTask(taskID string, workerID string, duration time.Duration) (*TaskLease, error) {
	cleanTaskID, err := sanitizeIdentifier(a, taskID, ErrInvalidTaskID)
	if err != nil {
		return nil, err
	}
	cleanWorkerID, err := sanitizeIdentifier(a, workerID, ErrInvalidWorkerID)
	if err != nil {
		return nil, err
	}

	var result *TaskLease
	err = a.execMutation(func(now time.Time) error {
		lease, lookupErr := a.getActiveLeaseUnsafe(cleanTaskID)
		if lookupErr != nil {
			return lookupErr
		}
		if lease.State == ConsensusStateLeased && !a.isExpiredUnsafe(lease, now) {
			return ErrTaskAlreadyLeased
		}
		result = a.bindLeaseUnsafe(lease, cleanWorkerID, duration, now)
		return nil
	})

	return result, err
}

// LeaseNext allocates the oldest pending task in FIFO order to the requesting worker.
func (a *AmbientConsensusArbiter) LeaseNext(workerID string, duration time.Duration) (*TaskLease, error) {
	worker, err := sanitizeIdentifier(a, workerID, ErrInvalidWorkerID)
	if err != nil {
		return nil, err
	}

	var assigned *TaskLease
	mutationErr := a.execMutation(func(now time.Time) error {
		for len(a.pendingQueue) > 0 {
			candidateID := a.pendingQueue[0]
			a.pendingQueue = a.pendingQueue[1:]

			lease, exists := a.leases[candidateID]
			if !exists || lease.State == ConsensusStateCommitted || lease.State == ConsensusStateRevoked {
				continue
			}
			if lease.State == ConsensusStateLeased && !a.isExpiredUnsafe(lease, now) {
				continue
			}
			assigned = a.bindLeaseUnsafe(lease, worker, duration, now)
			return nil
		}
		return ErrNoPendingTasks
	})
	return assigned, mutationErr
}

// CommitTask finalizes the lease upon successful worker completion and transitions state to Committed.
func (a *AmbientConsensusArbiter) CommitTask(taskID string, workerID string) error {
	cleanTaskID, err := sanitizeIdentifier(a, taskID, ErrInvalidTaskID)
	if err != nil {
		return err
	}
	cleanWorkerID, err := sanitizeIdentifier(a, workerID, ErrInvalidWorkerID)
	if err != nil {
		return err
	}

	return a.execMutation(func(now time.Time) error {
		lease, lookupErr := a.getActiveLeaseUnsafe(cleanTaskID)
		if lookupErr != nil {
			return lookupErr
		}
		if lease.State != ConsensusStateLeased {
			return ErrLeaseNotFound
		}
		if lease.WorkerID != cleanWorkerID {
			return ErrUnauthorizedWorker
		}
		if a.isExpiredUnsafe(lease, now) {
			lease.State = ConsensusStateExpired
			lease.UpdatedAt = now
			return ErrLeaseExpired
		}
		lease.State = ConsensusStateCommitted
		lease.UpdatedAt = now
		return nil
	})
}

// RevokeTask cancels an in-flight or pending lease, marking the state as Revoked.
func (a *AmbientConsensusArbiter) RevokeTask(taskID string, reason string) error {
	cleanTaskID, err := sanitizeIdentifier(a, taskID, ErrInvalidTaskID)
	if err != nil {
		return err
	}

	return a.execMutation(func(now time.Time) error {
		lease, lookupErr := a.getActiveLeaseUnsafe(cleanTaskID)
		if lookupErr != nil {
			return lookupErr
		}
		lease.State = ConsensusStateRevoked
		lease.UpdatedAt = now
		a.removeFromQueueUnsafe(cleanTaskID)
		_ = reason
		return nil
	})
}

// GetLease retrieves a point-in-time snapshot of the lease by task ID, nil-safe.
func (a *AmbientConsensusArbiter) GetLease(taskID string) (*TaskLease, bool) {
	if a == nil {
		return nil, false
	}
	cleanTaskID := strings.TrimSpace(taskID)
	if cleanTaskID == "" {
		return nil, false
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	lease, exists := a.leases[cleanTaskID]
	if !exists {
		return nil, false
	}

	now := a.nowFunc()
	if lease.State == ConsensusStateLeased && a.isExpiredUnsafe(lease, now) {
		lease.State = ConsensusStateExpired
		lease.UpdatedAt = now
	}

	return cloneTaskLease(lease), true
}

// Close marks the arbiter closed and prevents further proposals or lease acquisitions.
func (a *AmbientConsensusArbiter) Close() error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	a.closed = true
	a.mu.Unlock()
	return nil
}

func sanitizeIdentifier(arbiter *AmbientConsensusArbiter, raw string, errIfEmpty error) (string, error) {
	if arbiter == nil {
		return "", ErrArbiterClosed
	}
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return "", errIfEmpty
	}
	return cleaned, nil
}

func (a *AmbientConsensusArbiter) getActiveLeaseUnsafe(taskID string) (*TaskLease, error) {
	lease, exists := a.leases[taskID]
	if !exists {
		return nil, ErrLeaseNotFound
	}
	if lease.State == ConsensusStateCommitted {
		return nil, ErrTaskAlreadyCommitted
	}
	return lease, nil
}

func (a *AmbientConsensusArbiter) execMutation(action func(time.Time) error) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return ErrArbiterClosed
	}
	return action(a.nowFunc())
}

func (a *AmbientConsensusArbiter) bindLeaseUnsafe(l *TaskLease, workerID string, d time.Duration, now time.Time) *TaskLease {
	dur := d
	if dur <= 0 {
		dur = a.defaultLeaseDuration
	}
	l.WorkerID = workerID
	l.LeaseDuration = dur
	l.ExpiresAt = now.Add(dur)
	l.State = ConsensusStateLeased
	l.UpdatedAt = now
	a.removeFromQueueUnsafe(l.TaskID)
	return cloneTaskLease(l)
}

func (a *AmbientConsensusArbiter) isExpiredUnsafe(l *TaskLease, at time.Time) bool {
	return !l.ExpiresAt.IsZero() && at.After(l.ExpiresAt)
}

func (a *AmbientConsensusArbiter) enqueuePendingUnsafe(taskID string) {
	for _, id := range a.pendingQueue {
		if id == taskID {
			return
		}
	}
	a.pendingQueue = append(a.pendingQueue, taskID)
}

func (a *AmbientConsensusArbiter) removeFromQueueUnsafe(taskID string) {
	filtered := a.pendingQueue[:0]
	for _, id := range a.pendingQueue {
		if id != taskID {
			filtered = append(filtered, id)
		}
	}
	a.pendingQueue = filtered
}

func cloneTaskLease(src *TaskLease) *TaskLease {
	if src == nil {
		return nil
	}
	dst := *src
	if len(src.Payload) > 0 {
		dst.Payload = append([]byte(nil), src.Payload...)
	}
	return &dst
}
