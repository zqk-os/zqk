package callback

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// Role state enum values defining the cluster consensus lifecycle.
type LeaderRole string

const (
	RoleFollower  LeaderRole = "follower"
	RoleCandidate LeaderRole = "candidate"
	RoleLeader    LeaderRole = "leader"
)

// Leader election constants and event transition definitions.
const (
	DefaultLeaderElectorSubscriberName = "leader_elector"
	DefaultLeaseTTL                    = 3 * time.Second
	DefaultHeartbeatInterval           = 1 * time.Second

	CallbackTypeLeaderElection = "leader_election"

	TransitionElected     = "elected"
	TransitionRenewed     = "renewed"
	TransitionSteppedDown = "stepped_down"
	TransitionExpired     = "expired"

	FieldKeyTransition   = "transition"
	FieldKeyLeaderID     = "leader_id"
	FieldKeyFencingToken = "fencing_token"
	FieldKeyNodeID       = "node_id"
	FieldKeyRole         = "role"
	FieldKeyExpiresAt    = "expires_at"
)

var (
	ErrNotLeader         = errfmt.Errorf("node is not leader")
	ErrLeaseHeldByOther  = errfmt.Errorf("lease is held by another node")
	ErrLeaseExpired      = errfmt.Errorf("leader lease has expired")
	ErrStaleFencingToken = errfmt.Errorf("stale fencing token")
	ErrInvalidNodeID     = errfmt.Errorf("node ID cannot be empty")
)

var (
	_ CallbackSubscriber = (*LeaderElector)(nil)
	_ Subscriber         = (*LeaderElector)(nil)
)

// LeaderLease encapsulates a time-bounded distributed leadership lease.
type LeaderLease struct {
	LeaderID     string    `json:"leader_id"`
	FencingToken int64     `json:"fencing_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// LeaderLeaseStore defines atomic lease acquisition, renewal, and release primitives.
type LeaderLeaseStore interface {
	GetLease(ctx context.Context) (*LeaderLease, error)
	AcquireLease(ctx context.Context, nodeID string, ttl time.Duration, now time.Time) (*LeaderLease, bool, error)
	RenewLease(ctx context.Context, nodeID string, fencingToken int64, ttl time.Duration, now time.Time) (*LeaderLease, error)
	ReleaseLease(ctx context.Context, nodeID string, fencingToken int64) error
}

// LeaderElectorConfig provides configuration parameters for a LeaderElector instance.
type LeaderElectorConfig struct {
	NodeID            string
	LeaseTTL          time.Duration
	HeartbeatInterval time.Duration
	Clock             func() time.Time
	Store             LeaderLeaseStore
	Dispatcher        *MultiSubscriberDispatcher
	OnLeaderChanged   func(leaderID string, fencingToken int64)
	Logger            logging.Logger
}

// LeaderElector coordinates distributed leader election, manages heartbeat renewals,
// generates monotonic fencing tokens, and guards against split-brain execution.
type LeaderElector struct {
	config LeaderElectorConfig

	mu             sync.RWMutex
	role           LeaderRole
	lease          *LeaderLease
	observedLeader *LeaderLease

	fencingToken          atomic.Int64
	highestValidatedToken atomic.Int64

	running    bool
	loopCtx    context.Context
	loopCancel context.CancelFunc
	wg         sync.WaitGroup
}

// NewLeaderElector initializes an elector with safe defaults.
func NewLeaderElector(cfg LeaderElectorConfig) (*LeaderElector, error) {
	if cfg.NodeID == emptyValue {
		return nil, ErrInvalidNodeID
	}
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = DefaultLeaseTTL
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = DefaultHeartbeatInterval
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.Store == nil {
		cfg.Store = NewMemoryLeaderLeaseStore()
	}

	return &LeaderElector{
		config: cfg,
		role:   RoleFollower,
	}, nil
}

// Name returns the canonical subscriber identifier.
func (e *LeaderElector) Name() string {
	return DefaultLeaderElectorSubscriberName
}

func (e *LeaderElector) now() time.Time {
	if e.config.Clock != nil {
		return e.config.Clock()
	}
	return time.Now()
}

// IsLeader reports whether this node currently holds a valid, unexpired lease.
func (e *LeaderElector) IsLeader() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.isLeaderLocked()
}

func (e *LeaderElector) isLeaderLocked() bool {
	if e.role != RoleLeader || e.lease == nil {
		return false
	}
	return e.now().Before(e.lease.ExpiresAt)
}

// Role returns the current consensus role, demoting to follower if expired.
func (e *LeaderElector) Role() LeaderRole {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.role == RoleLeader && !e.now().Before(e.lease.ExpiresAt) {
		return RoleFollower
	}
	return e.role
}

// LeaderID returns the identifier of the active cluster leader, or empty string.
func (e *LeaderElector) LeaderID() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.isLeaderLocked() {
		return e.config.NodeID
	}
	if e.observedLeader != nil && e.now().Before(e.observedLeader.ExpiresAt) {
		return e.observedLeader.LeaderID
	}
	return emptyValue
}

// FencingToken returns the highest monotonic fencing token known to this elector.
func (e *LeaderElector) FencingToken() int64 {
	return e.fencingToken.Load()
}

// CurrentLease returns an immutable copy of the active leadership lease held by this node.
func (e *LeaderElector) CurrentLease() *LeaderLease {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.lease == nil {
		return nil
	}
	cpy := *e.lease
	return &cpy
}

// ValidateFencingToken verifies that a presentation token is strictly non-stale and monotonic.
func (e *LeaderElector) ValidateFencingToken(token int64) bool {
	if token <= 0 {
		return false
	}
	threshold := e.highestValidatedToken.Load()
	currentFencing := e.fencingToken.Load()
	if currentFencing > threshold {
		threshold = currentFencing
	}
	if token < threshold {
		return false
	}
	e.advanceHighestValidatedToken(token)
	return true
}

func (e *LeaderElector) advanceHighestValidatedToken(token int64) {
	for {
		highest := e.highestValidatedToken.Load()
		if token <= highest {
			break
		}
		if e.highestValidatedToken.CompareAndSwap(highest, token) {
			break
		}
	}
}

// Campaign attempts to acquire distributed leadership.
func (e *LeaderElector) Campaign(ctx context.Context) (bool, error) {
	if ctx != nil && ctx.Err() != nil {
		return false, ctx.Err()
	}

	e.mu.Lock()
	if e.isLeaderLocked() {
		e.mu.Unlock()
		renewErr := e.Renew(ctx)
		if renewErr != nil {
			return false, renewErr
		}
		return true, nil
	}
	e.role = RoleCandidate
	e.mu.Unlock()

	now := e.now()
	lease, won, err := e.config.Store.AcquireLease(ctx, e.config.NodeID, e.config.LeaseTTL, now)
	if err != nil {
		e.mu.Lock()
		e.role = RoleFollower
		e.mu.Unlock()
		return false, err
	}

	if !won {
		e.handleCampaignLoss(lease)
		return false, nil
	}

	e.handleCampaignWin(ctx, lease)
	return true, nil
}

func (e *LeaderElector) handleCampaignLoss(lease *LeaderLease) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.role = RoleFollower
	if lease != nil {
		e.observedLeader = lease
		e.updateFencingToken(lease.FencingToken)
	}
}

func (e *LeaderElector) handleCampaignWin(ctx context.Context, lease *LeaderLease) {
	e.mu.Lock()
	e.role = RoleLeader
	e.lease = lease
	e.observedLeader = lease
	e.updateFencingToken(lease.FencingToken)
	e.mu.Unlock()

	e.dispatchTransition(ctx, TransitionElected, lease)
}

// Renew extends the leader lease duration before expiration.
func (e *LeaderElector) Renew(ctx context.Context) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}

	e.mu.Lock()
	if e.role != RoleLeader || e.lease == nil {
		e.mu.Unlock()
		return ErrNotLeader
	}
	currentToken := e.lease.FencingToken
	e.mu.Unlock()

	now := e.now()
	updatedLease, err := e.config.Store.RenewLease(ctx, e.config.NodeID, currentToken, e.config.LeaseTTL, now)
	if err != nil {
		e.handleRenewFailure(ctx)
		return err
	}

	e.mu.Lock()
	e.lease = updatedLease
	e.observedLeader = updatedLease
	e.mu.Unlock()

	e.dispatchTransition(ctx, TransitionRenewed, updatedLease)
	return nil
}

func (e *LeaderElector) vacateLeadershipLocked() *LeaderLease {
	prev := e.lease
	e.role = RoleFollower
	e.lease = nil
	return prev
}

func (e *LeaderElector) handleRenewFailure(ctx context.Context) {
	e.mu.Lock()
	oldLease := e.vacateLeadershipLocked()
	e.mu.Unlock()

	if oldLease != nil {
		e.dispatchTransition(ctx, TransitionExpired, oldLease)
	}
}

// StepDown voluntarily yields leadership and releases the active lease.
func (e *LeaderElector) StepDown(ctx context.Context) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}

	e.mu.Lock()
	if e.role != RoleLeader || e.lease == nil {
		e.mu.Unlock()
		return nil
	}
	currentLease := e.vacateLeadershipLocked()
	e.mu.Unlock()

	relErr := e.config.Store.ReleaseLease(ctx, e.config.NodeID, currentLease.FencingToken)
	e.dispatchTransition(ctx, TransitionSteppedDown, currentLease)
	return relErr
}

func (e *LeaderElector) dispatchTransition(ctx context.Context, transition string, lease *LeaderLease) {
	if e.config.OnLeaderChanged != nil && lease != nil {
		e.config.OnLeaderChanged(lease.LeaderID, lease.FencingToken)
	}
	if e.config.Dispatcher == nil || lease == nil {
		return
	}

	entry := &CallbackEntry{
		Payload: map[string]any{
			objects.FieldKeyCallbackType: CallbackTypeLeaderElection,
			FieldKeyTransition:           transition,
			FieldKeyLeaderID:             lease.LeaderID,
			FieldKeyFencingToken:         lease.FencingToken,
			FieldKeyNodeID:               e.config.NodeID,
			FieldKeyRole:                 string(e.Role()),
			FieldKeyExpiresAt:            lease.ExpiresAt.Format(time.RFC3339Nano),
		},
		Timestamp: e.now(),
		JobID:     "leader_election:" + e.config.NodeID,
		Priority:  100,
	}
	if dispatchErr := e.config.Dispatcher.Dispatch(ctx, entry); dispatchErr != nil {
		if e.config.Logger != nil {
			e.config.Logger.Warn("failed to dispatch leadership transition",
				logging.String("transition", transition),
				logging.Error(dispatchErr))
		}
	}
}

// Notify responds to incoming callback entries, observing cluster leader state.
func (e *LeaderElector) Notify(ctx context.Context, entry *CallbackEntry) error {
	if entry == nil || entry.Payload == nil {
		return nil
	}
	cbType := objects.GetString(entry.Payload, objects.FieldKeyCallbackType)
	if cbType != CallbackTypeLeaderElection {
		return nil
	}

	transition := objects.GetString(entry.Payload, FieldKeyTransition)
	leaderID := objects.GetString(entry.Payload, FieldKeyLeaderID)
	fencingToken := extractFencingToken(entry.Payload)
	expiresAtStr := objects.GetString(entry.Payload, FieldKeyExpiresAt)
	expiresAt, parseErr := time.Parse(time.RFC3339Nano, expiresAtStr)
	if parseErr != nil {
		expiresAt = e.now().Add(e.config.LeaseTTL)
	}

	e.handleTransitionNotification(ctx, transition, leaderID, fencingToken, expiresAt)
	return nil
}

func (e *LeaderElector) handleTransitionNotification(ctx context.Context, transition, leaderID string, token int64, expiresAt time.Time) {
	e.updateFencingToken(token)

	switch transition {
	case TransitionElected, TransitionRenewed:
		e.handleLeaderObserved(ctx, leaderID, token, expiresAt)
	case TransitionSteppedDown, TransitionExpired:
		e.handleLeaderVacated(leaderID)
	}
}

func (e *LeaderElector) handleLeaderObserved(ctx context.Context, leaderID string, token int64, expiresAt time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.observedLeader = &LeaderLease{
		LeaderID:     leaderID,
		FencingToken: token,
		ExpiresAt:    expiresAt,
	}

	if e.role == RoleLeader && leaderID != e.config.NodeID && token >= e.fencingToken.Load() {
		e.role = RoleFollower
		e.lease = nil
	}
}

func (e *LeaderElector) handleLeaderVacated(leaderID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.observedLeader != nil && e.observedLeader.LeaderID == leaderID {
		e.observedLeader = nil
	}
}

func (e *LeaderElector) updateFencingToken(token int64) {
	if token <= 0 {
		return
	}
	for {
		curr := e.fencingToken.Load()
		if token <= curr {
			break
		}
		if e.fencingToken.CompareAndSwap(curr, token) {
			break
		}
	}
	e.advanceHighestValidatedToken(token)
}

func extractFencingToken(payload map[string]any) int64 {
	if payload == nil {
		return 0
	}
	val, ok := payload[FieldKeyFencingToken]
	if !ok {
		return 0
	}
	switch v := val.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}

// Start launches the background election and heartbeat renewal loop.
func (e *LeaderElector) Start(ctx context.Context) error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return nil
	}
	e.running = true
	e.loopCtx, e.loopCancel = context.WithCancel(ctx)
	e.mu.Unlock()

	e.wg.Add(1)
	goroutinelabels.NewGoroutine("leader_elector_loop", "background leader election and heartbeat renewal").
		StartSimple(e.runElectionLoop)

	return nil
}

func (e *LeaderElector) runElectionLoop() {
	defer e.wg.Done()

	interval := e.config.HeartbeatInterval
	if interval <= 0 {
		interval = DefaultHeartbeatInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-e.loopCtx.Done():
			return
		case <-ticker.C:
			e.tick()
		}
	}
}

func (e *LeaderElector) tick() {
	if e.IsLeader() {
		renewErr := e.Renew(e.loopCtx)
		if renewErr != nil && e.config.Logger != nil {
			e.config.Logger.Warn("failed to renew leadership lease", logging.Error(renewErr))
		}
		return
	}

	lease, getErr := e.config.Store.GetLease(e.loopCtx)
	if getErr != nil {
		return
	}
	now := e.now()
	if lease == nil || !now.Before(lease.ExpiresAt) {
		won, campErr := e.Campaign(e.loopCtx)
		if campErr == nil && won && e.config.Logger != nil {
			e.config.Logger.Info("acquired leadership via failover",
				logging.String("node_id", e.config.NodeID),
				logging.Int("fencing_token", int(e.FencingToken())))
		}
	}
}

// Stop terminates the background election loop and waits for completion.
func (e *LeaderElector) Stop() error {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return nil
	}
	e.running = false
	cancel := e.loopCancel
	e.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	e.wg.Wait()
	return nil
}

// MemoryLeaderLeaseStore provides an in-memory thread-safe implementation of LeaderLeaseStore.
type MemoryLeaderLeaseStore struct {
	mu               sync.RWMutex
	lease            *LeaderLease
	nextFencingToken int64
}

// NewMemoryLeaderLeaseStore creates an initialized in-memory lease store.
func NewMemoryLeaderLeaseStore() *MemoryLeaderLeaseStore {
	return &MemoryLeaderLeaseStore{}
}

// GetLease returns the current active lease.
func (m *MemoryLeaderLeaseStore) GetLease(ctx context.Context) (*LeaderLease, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.lease == nil {
		return nil, nil
	}
	cpy := *m.lease
	return &cpy, nil
}

// AcquireLease attempts to acquire the lease for nodeID.
func (m *MemoryLeaderLeaseStore) AcquireLease(ctx context.Context, nodeID string, ttl time.Duration, now time.Time) (*LeaderLease, bool, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, false, ctx.Err()
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.lease != nil && now.Before(m.lease.ExpiresAt) && m.lease.LeaderID != nodeID {
		cpy := *m.lease
		return &cpy, false, nil
	}

	m.nextFencingToken++
	m.lease = &LeaderLease{
		LeaderID:     nodeID,
		FencingToken: m.nextFencingToken,
		ExpiresAt:    now.Add(ttl),
	}
	cpy := *m.lease
	return &cpy, true, nil
}

// RenewLease extends the expiration of an existing lease if nodeID and fencingToken match.
func (m *MemoryLeaderLeaseStore) RenewLease(ctx context.Context, nodeID string, fencingToken int64, ttl time.Duration, now time.Time) (*LeaderLease, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.lease == nil || m.lease.LeaderID != nodeID {
		return nil, ErrNotLeader
	}
	if m.lease.FencingToken != fencingToken {
		return nil, ErrStaleFencingToken
	}
	if !now.Before(m.lease.ExpiresAt) {
		return nil, ErrLeaseExpired
	}

	m.lease.ExpiresAt = now.Add(ttl)
	cpy := *m.lease
	return &cpy, nil
}

// ReleaseLease clears the lease if held by nodeID and matching fencingToken.
func (m *MemoryLeaderLeaseStore) ReleaseLease(ctx context.Context, nodeID string, fencingToken int64) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.lease != nil && m.lease.LeaderID == nodeID && m.lease.FencingToken == fencingToken {
		m.lease.ExpiresAt = time.Time{}
		m.lease.LeaderID = emptyValue
	}
	return nil
}
