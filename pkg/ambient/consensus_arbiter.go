package ambient

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// Named constants enforcing bounded lease timing floors and deterministic execution.
const (
	DefaultLeaseDuration     = 30 * time.Second
	DefaultReapInterval      = 100 * time.Millisecond
	DefaultShutdownWaitTime  = 2 * time.Second
	DefaultLeaseTTL          = DefaultLeaseDuration
	DefaultMaxConsensusTasks = 4096
)

// ConsensusState represents the distributed lifecycle phase of a task lease.
type ConsensusState string

const (
	ConsensusStatePending   ConsensusState = "pending"
	ConsensusStateLeased    ConsensusState = "leased"
	ConsensusStateCommitted ConsensusState = "committed"
	ConsensusStateRevoked   ConsensusState = "revoked"
	ConsensusStateExpired   ConsensusState = "expired"

	// ConsensusStateActive is an alias for ConsensusStateLeased.
	ConsensusStateActive ConsensusState = "leased"
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

// Sentinel domain errors for consensus arbiter lifecycle and leasing operations.
var (
	ErrNilReceiver           = errfmt.Errorf("consensus arbiter: receiver is nil")
	ErrNilConsensusArbiter   = ErrNilReceiver
	ErrNilTaskLease          = errfmt.Errorf("task lease cannot be nil")
	ErrInvalidTaskID         = errfmt.Errorf("invalid task id")
	ErrEmptyTaskID           = ErrInvalidTaskID
	ErrInvalidWorkerID       = errfmt.Errorf("invalid worker id")
	ErrEmptyWorkerID         = ErrInvalidWorkerID
	ErrInvalidLeaseDuration  = errfmt.Errorf("invalid lease duration: duration must be positive")
	ErrTaskAlreadyLeased     = errfmt.Errorf("task is already leased")
	ErrTaskAlreadyExists     = errfmt.Errorf("task already exists")
	ErrLeaseExpired          = errfmt.Errorf("task lease has expired")
	ErrLeaseNotFound         = errfmt.Errorf("task lease not found")
	ErrTaskNotFound          = ErrLeaseNotFound
	ErrInvalidConsensusState = errfmt.Errorf("invalid consensus state")
	ErrTaskAlreadyCommitted  = errfmt.Errorf("task is already committed")
	ErrTaskNotLeased         = errfmt.Errorf("task is not currently leased")
	ErrInvalidLeaseToken     = errfmt.Errorf("invalid or mismatched lease token")
	ErrUnauthorizedWorker    = errfmt.Errorf("worker is not authorized for this lease")
	ErrNoPendingTasks        = errfmt.Errorf("no pending tasks available for leasing")
	ErrArbiterClosed         = errfmt.Errorf("consensus arbiter is closed")
	ErrArbiterAlreadyRun     = errfmt.Errorf("consensus arbiter is already running")
	ErrInvalidReapInterval   = errfmt.Errorf("reap interval must be strictly positive")
	ErrInvalidMaxTasks       = errfmt.Errorf("max tasks must be strictly positive")
)

// ConsensusArbiterConfig configures an AmbientConsensusArbiter instance.
type ConsensusArbiterConfig struct {
	DefaultLeaseTTL time.Duration `json:"default_lease_ttl"`
	ReapInterval    time.Duration `json:"reap_interval"`
	MaxTasks        int           `json:"max_tasks"`
}

// DefaultConsensusArbiterConfig returns safe default configuration for the consensus arbiter.
func DefaultConsensusArbiterConfig() ConsensusArbiterConfig {
	return ConsensusArbiterConfig{
		DefaultLeaseTTL: DefaultLeaseDuration,
		ReapInterval:    DefaultReapInterval,
		MaxTasks:        DefaultMaxConsensusTasks,
	}
}

// Validate validates that the consensus arbiter configuration satisfies floor invariants.
func (c ConsensusArbiterConfig) Validate() error {
	if c.DefaultLeaseTTL <= 0 {
		return ErrInvalidLeaseDuration
	}
	if c.ReapInterval <= 0 {
		return ErrInvalidReapInterval
	}
	if c.MaxTasks <= 0 {
		return ErrInvalidMaxTasks
	}
	return nil
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
	RenewedAt     time.Time      `json:"renewed_at,omitempty"`
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

// Clone creates a detached shallow copy of the lease.
func (l *TaskLease) Clone() *TaskLease {
	if l == nil {
		return nil
	}
	dst := *l
	if len(l.Payload) > 0 {
		dst.Payload = append([]byte(nil), l.Payload...)
	}
	return &dst
}

// IsExpired returns true if current time has passed the lease expiration timestamp.
func (l *TaskLease) IsExpired() bool {
	if l == nil {
		return true
	}
	if l.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().UTC().After(l.ExpiresAt)
}

// IsActive returns true if the lease is actively held and has not expired.
func (l *TaskLease) IsActive() bool {
	if l == nil {
		return false
	}
	return l.State == ConsensusStateLeased && !l.IsExpired()
}

// ArbiterOption configures runtime parameters for the consensus arbiter.
type ArbiterOption func(*AmbientConsensusArbiter)

// ConsensusArbiterOption is an alias for ArbiterOption.
type ConsensusArbiterOption = ArbiterOption

// WithDefaultLeaseDuration overrides the fallback duration for newly granted leases.
func WithDefaultLeaseDuration(d time.Duration) ArbiterOption {
	return func(a *AmbientConsensusArbiter) {
		if d > 0 {
			a.defaultLeaseDuration = d
		}
	}
}

// WithDefaultLeaseTTL sets the default lease duration.
func WithDefaultLeaseTTL(ttl time.Duration) ArbiterOption {
	return WithDefaultLeaseDuration(ttl)
}

// WithReapInterval overrides the interval between background expiration sweeps.
func WithReapInterval(d time.Duration) ArbiterOption {
	return func(a *AmbientConsensusArbiter) {
		if d > 0 {
			a.reapInterval = d
		}
	}
}

// WithMaxConsensusTasks sets the upper bound on tracked tasks.
func WithMaxConsensusTasks(maxTasks int) ArbiterOption {
	return func(a *AmbientConsensusArbiter) {
		if maxTasks > 0 {
			a.maxTasks = maxTasks
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

// AmbientConsensusArbiter manages atomic task proposals, exclusive worker leases,
// background expiration reaping, and consensus commits.
type AmbientConsensusArbiter struct {
	mu                   sync.RWMutex
	leases               map[string]*TaskLease
	pendingQueue         []string
	defaultLeaseDuration time.Duration
	reapInterval         time.Duration
	maxTasks             int
	nowFunc              func() time.Time
	running              bool
	closed               bool
	stopCh               chan struct{}
	doneCh               chan struct{}
}

// ConsensusArbiter is a type alias for AmbientConsensusArbiter.
type ConsensusArbiter = AmbientConsensusArbiter

// NewConsensusArbiter creates an initialized, thread-safe AmbientConsensusArbiter.
func NewConsensusArbiter(opts ...ArbiterOption) *AmbientConsensusArbiter {
	arbiter := &AmbientConsensusArbiter{
		leases:               make(map[string]*TaskLease),
		pendingQueue:         make([]string, 0),
		defaultLeaseDuration: DefaultLeaseDuration,
		reapInterval:         DefaultReapInterval,
		maxTasks:             DefaultMaxConsensusTasks,
		nowFunc:              func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(arbiter)
	}
	return arbiter
}

// NewConsensusArbiterWithConfig creates an initialized, thread-safe AmbientConsensusArbiter from a configuration struct.
func NewConsensusArbiterWithConfig(cfg ConsensusArbiterConfig, opts ...ArbiterOption) (*AmbientConsensusArbiter, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	arbiter := NewConsensusArbiter(opts...)
	arbiter.defaultLeaseDuration = cfg.DefaultLeaseTTL
	arbiter.reapInterval = cfg.ReapInterval
	arbiter.maxTasks = cfg.MaxTasks
	return arbiter, nil
}

func validateIdentifier(val string, errIfEmpty error) (string, error) {
	cleaned := strings.TrimSpace(val)
	if cleaned == "" {
		return "", errIfEmpty
	}
	return cleaned, nil
}

func (a *AmbientConsensusArbiter) checkActiveReceiver() error {
	if a == nil {
		return ErrNilReceiver
	}
	return nil
}

func (a *AmbientConsensusArbiter) validateTaskInput(taskID string) (string, error) {
	if err := a.checkActiveReceiver(); err != nil {
		return "", err
	}
	return validateIdentifier(taskID, ErrInvalidTaskID)
}

func (a *AmbientConsensusArbiter) validateTaskAndWorker(taskID, workerID string) (string, string, error) {
	cleanTaskID, err := a.validateTaskInput(taskID)
	if err != nil {
		return "", "", err
	}
	cleanWorkerID, err := validateIdentifier(workerID, ErrInvalidWorkerID)
	if err != nil {
		return "", "", err
	}
	return cleanTaskID, cleanWorkerID, nil
}

func validateDuration(d time.Duration) error {
	if d <= 0 {
		return ErrInvalidLeaseDuration
	}
	return nil
}

func (a *AmbientConsensusArbiter) execMutation(action func(now time.Time) error) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return ErrArbiterClosed
	}
	return action(a.nowFunc())
}

// Start spawns the background lease reaper goroutine with graceful context lifecycle.
func (a *AmbientConsensusArbiter) Start(ctx context.Context) error {
	if err := a.checkActiveReceiver(); err != nil {
		return err
	}

	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return ErrArbiterClosed
	}
	if a.running {
		a.mu.Unlock()
		return ErrArbiterAlreadyRun
	}
	a.running = true
	a.stopCh = make(chan struct{})
	a.doneCh = make(chan struct{})
	a.mu.Unlock()

	goroutinelabels.NewGoroutine("consensus_arbiter_reaper", "reap expired task leases").
		StartSimple(func() {
			a.runReaper(ctx)
		})

	return nil
}

// Stop initiates graceful shutdown of the background reaper and awaits completion.
func (a *AmbientConsensusArbiter) Stop() error {
	if err := a.checkActiveReceiver(); err != nil {
		return err
	}

	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return nil
	}
	a.running = false
	close(a.stopCh)
	done := a.doneCh
	a.mu.Unlock()

	select {
	case <-done:
	case <-time.After(DefaultShutdownWaitTime):
	}
	return nil
}

// runReaper executes the periodic expiration reap loop.
func (a *AmbientConsensusArbiter) runReaper(ctx context.Context) {
	ticker := time.NewTicker(a.reapInterval)
	defer ticker.Stop()
	defer close(a.doneCh)

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case <-ticker.C:
			reaped := a.ReapExpiredLeases()
			if reaped < 0 {
				continue
			}
		}
	}
}

// ReapExpiredLeases scans active leases, revoking ownership and marking expired leases.
func (a *AmbientConsensusArbiter) ReapExpiredLeases() int {
	if a == nil {
		return 0
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	reaped := 0
	now := a.nowFunc()
	for _, lease := range a.leases {
		if lease.State == ConsensusStateLeased && a.isExpiredUnsafe(lease, now) {
			lease.State = ConsensusStateExpired
			lease.WorkerID = ""
			lease.UpdatedAt = now
			reaped++
		}
	}
	return reaped
}

// ProposeTask registers a new task candidate in the consensus pool in Pending state.
func (a *AmbientConsensusArbiter) ProposeTask(taskID string, payload []byte) (*TaskLease, error) {
	cleanTaskID, err := a.validateTaskInput(taskID)
	if err != nil {
		return nil, err
	}

	var created *TaskLease
	mutErr := a.execMutation(func(now time.Time) error {
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
		created = cloneTaskLease(lease)
		return nil
	})
	return created, mutErr
}

// LeaseTask grants exclusive execution rights for a proposed or unassigned task to the given worker.
func (a *AmbientConsensusArbiter) LeaseTask(taskID string, workerID string, duration time.Duration) (*TaskLease, error) {
	if err := validateDuration(duration); err != nil {
		return nil, err
	}
	cleanTaskID, cleanWorkerID, err := a.validateTaskAndWorker(taskID, workerID)
	if err != nil {
		return nil, err
	}

	var assigned *TaskLease
	mutErr := a.execMutation(func(now time.Time) error {
		existing, exists := a.leases[cleanTaskID]
		if exists {
			if existing.State == ConsensusStateCommitted {
				return ErrTaskAlreadyCommitted
			}
			if existing.State == ConsensusStateLeased && !a.isExpiredUnsafe(existing, now) {
				if existing.WorkerID != cleanWorkerID {
					return ErrTaskAlreadyLeased
				}
				existing.ExpiresAt = now.Add(duration)
				existing.RenewedAt = now
				existing.UpdatedAt = now
				assigned = cloneTaskLease(existing)
				return nil
			}
			assigned = a.bindLeaseUnsafe(existing, cleanWorkerID, duration, now)
			return nil
		}

		lease := &TaskLease{
			ID:            fmt.Sprintf("lease-%s-%d", cleanTaskID, now.UnixNano()),
			TaskID:        cleanTaskID,
			WorkerID:      cleanWorkerID,
			LeaseDuration: duration,
			ExpiresAt:     now.Add(duration),
			State:         ConsensusStateLeased,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		a.leases[cleanTaskID] = lease
		assigned = cloneTaskLease(lease)
		return nil
	})
	return assigned, mutErr
}

// AcquireLease is an alias for LeaseTask for compatibility with boundary callers.
func (a *AmbientConsensusArbiter) AcquireLease(taskID string, workerID string, duration time.Duration) (*TaskLease, error) {
	return a.LeaseTask(taskID, workerID, duration)
}

// LeaseNext allocates the oldest pending task in FIFO order to the requesting worker.
func (a *AmbientConsensusArbiter) LeaseNext(workerID string, duration time.Duration) (*TaskLease, error) {
	if err := validateDuration(duration); err != nil {
		return nil, err
	}
	if err := a.checkActiveReceiver(); err != nil {
		return nil, err
	}
	cleanWorkerID, err := validateIdentifier(workerID, ErrInvalidWorkerID)
	if err != nil {
		return nil, err
	}

	var assigned *TaskLease
	mutErr := a.execMutation(func(now time.Time) error {
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
			assigned = a.bindLeaseUnsafe(lease, cleanWorkerID, duration, now)
			return nil
		}
		return ErrNoPendingTasks
	})
	return assigned, mutErr
}

func (a *AmbientConsensusArbiter) resolveAuthorizedLease(taskID, workerID string, now time.Time) (*TaskLease, error) {
	lease, exists := a.leases[taskID]
	if !exists {
		return nil, ErrLeaseNotFound
	}
	if lease.State == ConsensusStateCommitted {
		return nil, ErrTaskAlreadyCommitted
	}
	if lease.State == ConsensusStateExpired {
		return nil, ErrLeaseExpired
	}
	if lease.State != ConsensusStateLeased {
		return nil, ErrLeaseNotFound
	}
	if lease.WorkerID != workerID {
		return nil, ErrUnauthorizedWorker
	}
	if a.isExpiredUnsafe(lease, now) {
		lease.State = ConsensusStateExpired
		lease.WorkerID = ""
		lease.UpdatedAt = now
		return nil, ErrLeaseExpired
	}
	return lease, nil
}

// RenewLease extends an active task lease by an additional duration for the authorized worker.
func (a *AmbientConsensusArbiter) RenewLease(taskID string, workerID string, additional time.Duration) (*TaskLease, error) {
	if err := validateDuration(additional); err != nil {
		return nil, err
	}
	cleanTaskID, cleanWorkerID, err := a.validateTaskAndWorker(taskID, workerID)
	if err != nil {
		return nil, err
	}

	var renewed *TaskLease
	mutErr := a.execMutation(func(now time.Time) error {
		lease, resErr := a.resolveAuthorizedLease(cleanTaskID, cleanWorkerID, now)
		if resErr != nil {
			return resErr
		}
		lease.ExpiresAt = lease.ExpiresAt.Add(additional)
		lease.RenewedAt = now
		lease.UpdatedAt = now
		renewed = cloneTaskLease(lease)
		return nil
	})
	return renewed, mutErr
}

// CommitTask finalizes the lease upon successful worker completion and transitions state to Committed.
func (a *AmbientConsensusArbiter) CommitTask(taskID string, workerID string) error {
	cleanTaskID, cleanWorkerID, err := a.validateTaskAndWorker(taskID, workerID)
	if err != nil {
		return err
	}

	return a.execMutation(func(now time.Time) error {
		lease, resErr := a.resolveAuthorizedLease(cleanTaskID, cleanWorkerID, now)
		if resErr != nil {
			return resErr
		}
		lease.State = ConsensusStateCommitted
		lease.UpdatedAt = now
		return nil
	})
}

// RevokeTask cancels an in-flight or pending lease, marking the state as Revoked.
func (a *AmbientConsensusArbiter) RevokeTask(taskID string, reason string) error {
	cleanTaskID, err := a.validateTaskInput(taskID)
	if err != nil {
		return err
	}
	_ = reason

	return a.execMutation(func(now time.Time) error {
		lease, exists := a.leases[cleanTaskID]
		if !exists {
			return ErrLeaseNotFound
		}
		if lease.State == ConsensusStateCommitted {
			return ErrTaskAlreadyCommitted
		}

		lease.State = ConsensusStateRevoked
		lease.WorkerID = ""
		lease.UpdatedAt = now
		a.removeFromQueueUnsafe(cleanTaskID)
		return nil
	})
}

// LookupLease retrieves a snapshot of the lease by task ID returning explicit domain errors.
func (a *AmbientConsensusArbiter) LookupLease(taskID string) (*TaskLease, error) {
	cleanTaskID, err := a.validateTaskInput(taskID)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	lease, exists := a.leases[cleanTaskID]
	if !exists {
		return nil, ErrLeaseNotFound
	}

	now := a.nowFunc()
	if lease.State == ConsensusStateLeased && a.isExpiredUnsafe(lease, now) {
		lease.State = ConsensusStateExpired
		lease.WorkerID = ""
		lease.UpdatedAt = now
	}

	return cloneTaskLease(lease), nil
}

// GetLease retrieves a point-in-time snapshot of the lease by task ID, nil-safe.
func (a *AmbientConsensusArbiter) GetLease(taskID string) (*TaskLease, bool) {
	lease, err := a.LookupLease(taskID)
	return lease, err == nil
}

// ListLeases returns a point-in-time snapshot of all registered task leases.
func (a *AmbientConsensusArbiter) ListLeases() []*TaskLease {
	if a == nil {
		return []*TaskLease{}
	}
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]*TaskLease, 0, len(a.leases))
	for _, l := range a.leases {
		result = append(result, cloneTaskLease(l))
	}
	return result
}

// ListTasks is an alias for ListLeases.
func (a *AmbientConsensusArbiter) ListTasks() []*TaskLease {
	return a.ListLeases()
}

func (a *AmbientConsensusArbiter) countByState(targetState ConsensusState) int {
	if a == nil {
		return 0
	}
	a.mu.RLock()
	defer a.mu.RUnlock()

	matched := 0
	for _, lease := range a.leases {
		if lease != nil && lease.State == targetState {
			matched++
		}
	}
	return matched
}

// ProposedCount returns the total tasks currently awaiting worker leasing.
func (a *AmbientConsensusArbiter) ProposedCount() int {
	return a.countByState(ConsensusStatePending)
}

// LeasedCount returns the total tasks currently actively leased.
func (a *AmbientConsensusArbiter) LeasedCount() int {
	return a.countByState(ConsensusStateLeased)
}

// CommittedCount returns the total completed and committed tasks.
func (a *AmbientConsensusArbiter) CommittedCount() int {
	return a.countByState(ConsensusStateCommitted)
}

// RevokedCount returns the total revoked tasks.
func (a *AmbientConsensusArbiter) RevokedCount() int {
	return a.countByState(ConsensusStateRevoked)
}

// ExpiredCount returns the total expired tasks.
func (a *AmbientConsensusArbiter) ExpiredCount() int {
	return a.countByState(ConsensusStateExpired)
}

// Close gracefully stops the arbiter and rejects future mutations.
func (a *AmbientConsensusArbiter) Close() error {
	if err := a.checkActiveReceiver(); err != nil {
		return err
	}
	a.mu.Lock()
	a.closed = true
	a.mu.Unlock()
	return a.Stop()
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
