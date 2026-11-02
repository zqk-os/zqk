// Package ambient provides the autonomous holon runtime engine and conscious swarm coordination.
package ambient

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// Default timing and capacity constants for swarm task dispatching.
const (
	DefaultSwarmLeaseDuration       = 30 * time.Second
	DefaultSwarmCheckInterval       = 100 * time.Millisecond
	DefaultSwarmMaxConcurrentLeases = 32
	DefaultSwarmHeartbeatTimeout    = 10 * time.Second
	DefaultSwarmChannelBufferSize   = 64
)

// Sentinel errors for swarm task leasing and backlog observer operations.
var (
	ErrNilSwarmTaskLease          = errfmt.Errorf("swarm task lease cannot be nil")
	ErrNilBacklogObserver         = errfmt.Errorf("backlog observer cannot be nil")
	ErrNilSwarmDispatcher         = ErrNilBacklogObserver
	ErrInvalidCheckInterval       = errfmt.Errorf("check interval must be strictly positive")
	ErrInvalidMaxConcurrentLeases = errfmt.Errorf("max concurrent leases must be strictly positive")
	ErrObserverClosed             = errfmt.Errorf("backlog observer is closed")
	ErrObserverAlreadyRunning     = errfmt.Errorf("backlog observer is already running")
	ErrNilConsensusBinding        = errfmt.Errorf("consensus arbiter binding cannot be nil")
	ErrSeatNotFound               = errfmt.Errorf("seat not found")
	ErrNoHealthySeats             = errfmt.Errorf("no healthy peer seats available")
)

// SwarmTaskLease represents a discrete unit of leased work dispatched to a peer seat.
type SwarmTaskLease struct {
	ID             string         `json:"id"`
	TaskID         string         `json:"task_id"`
	PeerSeatID     string         `json:"peer_seat_id,omitempty"`
	LeaseDuration  time.Duration  `json:"lease_duration"`
	ExpiresAt      time.Time      `json:"expires_at"`
	AcquiredAt     time.Time      `json:"acquired_at"`
	State          ConsensusState `json:"state"`
	HeartbeatAt    time.Time      `json:"heartbeat_at,omitempty"`
	VersionContext string         `json:"version_context,omitempty"`
	NamespaceID    string         `json:"namespace_id,omitempty"`
	Payload        []byte         `json:"payload,omitempty"`
}

// Validate validates that SwarmTaskLease satisfies structural invariants.
func (l *SwarmTaskLease) Validate() error {
	if l == nil {
		return ErrNilSwarmTaskLease
	}
	if strings.TrimSpace(l.TaskID) == "" {
		return ErrInvalidTaskID
	}
	if !l.State.IsValid() {
		return ErrInvalidConsensusState
	}
	if l.LeaseDuration <= 0 {
		return ErrInvalidLeaseDuration
	}
	return nil
}

// IsExpired returns true if the current time exceeds ExpiresAt (if set).
func (l *SwarmTaskLease) IsExpired() bool {
	if l == nil {
		return true
	}
	if l.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(l.ExpiresAt)
}

// Clone returns a deep copy of the SwarmTaskLease.
func (l *SwarmTaskLease) Clone() *SwarmTaskLease {
	if l == nil {
		return nil
	}
	cp := *l
	if len(l.Payload) > 0 {
		cp.Payload = append([]byte(nil), l.Payload...)
	}
	return &cp
}

// SwarmDispatcherConfig configures the BacklogObserver and task leasing lifecycle.
type SwarmDispatcherConfig struct {
	LeaseDuration       time.Duration `json:"lease_duration"`
	CheckInterval       time.Duration `json:"check_interval"`
	MaxConcurrentLeases int           `json:"max_concurrent_leases"`
	HeartbeatTimeout    time.Duration `json:"heartbeat_timeout,omitempty"`
	ChannelBufferSize   int           `json:"channel_buffer_size,omitempty"`
	AutoClaimEnabled    bool          `json:"auto_claim_enabled,omitempty"`
	MaxRetries          int           `json:"max_retries,omitempty"`
	MaxClaimsPerSeat    int           `json:"max_claims_per_seat,omitempty"`
	EventBufferSize     int           `json:"event_buffer_size,omitempty"`
}

// SwarmDispatchConfig is an alias for SwarmDispatcherConfig.
type SwarmDispatchConfig = SwarmDispatcherConfig

// DefaultSwarmDispatcherConfig returns production-grade defaults for swarm task dispatching.
func DefaultSwarmDispatcherConfig() SwarmDispatcherConfig {
	return SwarmDispatcherConfig{
		LeaseDuration:       DefaultSwarmLeaseDuration,
		CheckInterval:       DefaultSwarmCheckInterval,
		MaxConcurrentLeases: DefaultSwarmMaxConcurrentLeases,
		HeartbeatTimeout:    DefaultSwarmHeartbeatTimeout,
		ChannelBufferSize:   DefaultSwarmChannelBufferSize,
		AutoClaimEnabled:    true,
		MaxRetries:          DefaultSwarmMaxRetries,
		MaxClaimsPerSeat:    DefaultSwarmMaxClaimsPerSeat,
		EventBufferSize:     DefaultSwarmEventBufferSize,
	}
}

// DefaultSwarmDispatchConfig returns production-grade defaults for swarm task dispatching.
func DefaultSwarmDispatchConfig() SwarmDispatchConfig {
	return DefaultSwarmDispatcherConfig()
}

// Validate verifies that the SwarmDispatcherConfig satisfies fail-closed floor invariants.
func (c SwarmDispatcherConfig) Validate() error {
	if c.LeaseDuration <= 0 {
		return ErrInvalidLeaseDuration
	}
	if c.CheckInterval <= 0 {
		return ErrInvalidCheckInterval
	}
	if c.MaxConcurrentLeases <= 0 {
		return ErrInvalidMaxConcurrentLeases
	}
	if c.MaxRetries < 0 {
		return ErrInvalidMaxRetries
	}
	if c.MaxClaimsPerSeat < 0 {
		return ErrInvalidClaimsPerSeat
	}
	return nil
}

// PeerSeat captures the registration state and health heartbeat of an active worker peer seat.
type PeerSeat struct {
	SeatID   string    `json:"seat_id"`
	Role     string    `json:"role"`
	Healthy  bool      `json:"healthy"`
	LastSeen time.Time `json:"last_seen"`
}

// BacklogObserver continuously observes backlog items, leases work via ConsensusArbiter,
// and publishes task lease events through unbuffered/buffered channels.
type BacklogObserver struct {
	mu        sync.RWMutex
	config    SwarmDispatcherConfig
	arbiter   *ConsensusArbiter
	leaseCh   chan SwarmTaskLease
	leases    map[string]*SwarmTaskLease
	seats     map[string]*PeerSeat
	running   bool
	cancelFn  context.CancelFunc
	wg        sync.WaitGroup
	ticksSeen uint64
	loopState *SwarmLoopState
}

// SwarmDispatcher is an alias for BacklogObserver.
type SwarmDispatcher = BacklogObserver

// NewBacklogObserver constructs a BacklogObserver bound to a ConsensusArbiter.
func NewBacklogObserver(cfg SwarmDispatcherConfig, arbiter *ConsensusArbiter) (*BacklogObserver, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if arbiter == nil {
		return nil, ErrNilConsensusBinding
	}
	bufSize := cfg.ChannelBufferSize
	if bufSize <= 0 {
		bufSize = DefaultSwarmChannelBufferSize
	}

	return &BacklogObserver{
		config:    cfg,
		arbiter:   arbiter,
		leaseCh:   make(chan SwarmTaskLease, bufSize),
		leases:    make(map[string]*SwarmTaskLease),
		seats:     make(map[string]*PeerSeat),
		loopState: NewSwarmLoopState(cfg.EventBufferSize),
	}, nil
}

// NewSwarmDispatcher constructs a SwarmDispatcher (BacklogObserver) bound to a ConsensusArbiter.
func NewSwarmDispatcher(arbiter *ConsensusArbiter, cfg SwarmDispatcherConfig) (*SwarmDispatcher, error) {
	return NewBacklogObserver(cfg, arbiter)
}

func withObserverRLock[T any](o *BacklogObserver, fn func(*BacklogObserver) T) T {
	var zero T
	if o == nil {
		return zero
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	return fn(o)
}

func withObserverLock(o *BacklogObserver, fn func(*BacklogObserver) error) error {
	if o == nil {
		return ErrNilBacklogObserver
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return fn(o)
}

// Config returns a copy of the observer configuration. Returns empty config if receiver is nil.
func (o *BacklogObserver) Config() SwarmDispatcherConfig {
	return withObserverRLock(o, func(obs *BacklogObserver) SwarmDispatcherConfig {
		return obs.config
	})
}

// Arbiter returns the bound ConsensusArbiter. Returns nil if receiver is nil.
func (o *BacklogObserver) Arbiter() *ConsensusArbiter {
	return withObserverRLock(o, func(obs *BacklogObserver) *ConsensusArbiter {
		return obs.arbiter
	})
}

// LeaseChannel returns the receive-only channel for published task leases. Returns nil if receiver is nil.
func (o *BacklogObserver) LeaseChannel() <-chan SwarmTaskLease {
	return withObserverRLock(o, func(obs *BacklogObserver) <-chan SwarmTaskLease {
		return obs.leaseCh
	})
}

// IsRunning returns true if the observer background loop is running. Returns false if receiver is nil.
func (o *BacklogObserver) IsRunning() bool {
	return withObserverRLock(o, func(obs *BacklogObserver) bool {
		return obs.running
	})
}

// Start launches the observer background loop. Returns ErrNilBacklogObserver if receiver is nil.
func (o *BacklogObserver) Start(ctx context.Context) error {
	if o == nil {
		return ErrNilBacklogObserver
	}
	o.mu.Lock()
	if o.running {
		o.mu.Unlock()
		return ErrObserverAlreadyRunning
	}
	obsCtx, cancel := context.WithCancel(ctx)
	o.cancelFn = cancel
	o.running = true
	interval := o.config.CheckInterval
	if interval <= 0 {
		interval = DefaultSwarmCheckInterval
	}
	o.mu.Unlock()

	goroutinelabels.NewGoroutine("ambient.backlog.observer", "backlog task leasing observer loop").
		WithWaitGroup(&o.wg).
		StartSimple(func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-obsCtx.Done():
					return
				case <-ticker.C:
					o.ObserveTick(obsCtx)
				}
			}
		})

	return nil
}

// Stop halts the observer background loop. Returns ErrNilBacklogObserver if receiver is nil.
func (o *BacklogObserver) Stop() error {
	if o == nil {
		return ErrNilBacklogObserver
	}
	o.mu.Lock()
	if !o.running {
		o.mu.Unlock()
		return nil
	}
	cancel := o.cancelFn
	o.running = false
	o.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	o.wg.Wait()
	return nil
}

// RegisterPeerSeat registers a peer seat eligible for task dispatch.
func (o *BacklogObserver) RegisterPeerSeat(seatID, role string) error {
	if strings.TrimSpace(seatID) == "" {
		return ErrInvalidWorkerID
	}
	return withObserverLock(o, func(obs *BacklogObserver) error {
		obs.seats[seatID] = &PeerSeat{
			SeatID:   seatID,
			Role:     role,
			Healthy:  true,
			LastSeen: time.Now(),
		}
		return nil
	})
}

// RegisterSeat registers a peer seat using the PeerSeat struct.
func (o *BacklogObserver) RegisterSeat(seat PeerSeat) error {
	return o.RegisterPeerSeat(seat.SeatID, seat.Role)
}

// HealthySeats returns the identifiers of all currently healthy peer seats. Returns nil if receiver is nil.
func (o *BacklogObserver) HealthySeats() []string {
	return withObserverRLock(o, func(obs *BacklogObserver) []string {
		var res []string
		now := time.Now()
		timeout := obs.config.HeartbeatTimeout
		if timeout <= 0 {
			timeout = DefaultSwarmHeartbeatTimeout
		}
		for id, s := range obs.seats {
			if s.Healthy && (s.LastSeen.IsZero() || now.Sub(s.LastSeen) <= timeout) {
				res = append(res, id)
			}
		}
		return res
	})
}

// IsSeatHealthy returns true if the peer seat exists and has not missed heartbeats. Returns false if receiver is nil.
func (o *BacklogObserver) IsSeatHealthy(seatID string) bool {
	return withObserverRLock(o, func(obs *BacklogObserver) bool {
		s, found := obs.seats[seatID]
		if !found || !s.Healthy {
			return false
		}
		timeout := obs.config.HeartbeatTimeout
		if timeout <= 0 {
			timeout = DefaultSwarmHeartbeatTimeout
		}
		if !s.LastSeen.IsZero() && time.Since(s.LastSeen) > timeout {
			return false
		}
		return true
	})
}

// RecordHeartbeat refreshes the last seen timestamp of a peer seat.
func (o *BacklogObserver) RecordHeartbeat(seatID string) error {
	return withObserverLock(o, func(obs *BacklogObserver) error {
		s, found := obs.seats[seatID]
		if !found {
			return ErrSeatNotFound
		}
		s.LastSeen = time.Now()
		s.Healthy = true
		return nil
	})
}

// EvictExpiredLeases scans active leases, marks expired leases as ConsensusStateExpired,
// removes them from active tracking, and returns the count of evicted leases.
func (o *BacklogObserver) EvictExpiredLeases() int {
	if o == nil {
		return 0
	}
	o.mu.Lock()
	defer o.mu.Unlock()

	now := time.Now()
	evicted := 0
	for id, lease := range o.leases {
		if !lease.ExpiresAt.IsZero() && now.After(lease.ExpiresAt) {
			lease.State = ConsensusStateExpired
			delete(o.leases, id)
			evicted++
		}
	}
	if o.arbiter != nil {
		o.arbiter.ReapExpiredLeases()
	}
	return evicted
}

// ObserveTick executes a nil-safe observation step.
func (o *BacklogObserver) ObserveTick(ctx context.Context) {
	if o == nil {
		return
	}
	if ctx != nil && ctx.Err() != nil {
		return
	}
	o.mu.Lock()
	o.ticksSeen++
	o.mu.Unlock()

	o.EvictExpiredLeases()
	if o.config.AutoClaimEnabled {
		o.AutoClaimTick(ctx)
	}
}

// ActiveLeases returns copies of all currently active task leases. Returns nil if receiver is nil.
func (o *BacklogObserver) ActiveLeases() []*SwarmTaskLease {
	return withObserverRLock(o, func(obs *BacklogObserver) []*SwarmTaskLease {
		result := make([]*SwarmTaskLease, 0, len(obs.leases))
		for _, l := range obs.leases {
			result = append(result, l.Clone())
		}
		return result
	})
}

// LeaseCount returns the current count of active leases. Returns 0 if receiver is nil.
func (o *BacklogObserver) LeaseCount() int {
	return withObserverRLock(o, func(obs *BacklogObserver) int {
		return len(obs.leases)
	})
}

// GetLease retrieves an active lease copy by task ID.
func (o *BacklogObserver) GetLease(taskID string) (*SwarmTaskLease, bool) {
	if o == nil {
		return nil, false
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	l, found := o.leases[taskID]
	if !found {
		return nil, false
	}
	return l.Clone(), true
}

// GetTaskLease is an alias for GetLease.
func (o *BacklogObserver) GetTaskLease(taskID string) (*SwarmTaskLease, bool) {
	return o.GetLease(taskID)
}

// PublishLease attempts to publish a lease event to the leasing channel without blocking.
func (o *BacklogObserver) PublishLease(lease SwarmTaskLease) bool {
	if o == nil {
		return false
	}
	o.mu.RLock()
	ch := o.leaseCh
	o.mu.RUnlock()

	if ch == nil {
		return false
	}

	select {
	case ch <- lease:
		return true
	default:
		return false
	}
}

// DispatchLease coordinates with ConsensusArbiter to propose, lease, and emit a task lease.
func (o *BacklogObserver) DispatchLease(taskID, peerSeatID string, duration time.Duration) (*SwarmTaskLease, error) {
	if o == nil {
		return nil, ErrNilBacklogObserver
	}
	if strings.TrimSpace(taskID) == "" {
		return nil, ErrInvalidTaskID
	}
	if strings.TrimSpace(peerSeatID) == "" {
		return nil, ErrInvalidWorkerID
	}
	if duration <= 0 {
		duration = o.config.LeaseDuration
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	if len(o.leases) >= o.config.MaxConcurrentLeases {
		return nil, errfmt.Errorf("max concurrent leases capacity reached (%d)", o.config.MaxConcurrentLeases)
	}

	if o.arbiter == nil {
		return nil, ErrNilConsensusBinding
	}

	// Check if already leased in local cache
	if _, exists := o.leases[taskID]; exists {
		return nil, ErrTaskAlreadyLeased
	}

	// Propose task in arbiter if not present
	if _, found := o.arbiter.GetLease(taskID); !found {
		if _, err := o.arbiter.ProposeTask(taskID, nil); err != nil && !errors.Is(err, ErrTaskAlreadyExists) {
			return nil, err
		}
	}

	arbLease, err := o.arbiter.LeaseTask(taskID, peerSeatID, duration)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	swarmLease := &SwarmTaskLease{
		ID:            arbLease.ID,
		TaskID:        taskID,
		PeerSeatID:    peerSeatID,
		LeaseDuration: duration,
		ExpiresAt:     arbLease.ExpiresAt,
		AcquiredAt:    now,
		State:         arbLease.State,
		HeartbeatAt:   now,
	}

	o.leases[taskID] = swarmLease

	// Non-blocking publish to channel
	select {
	case o.leaseCh <- *swarmLease:
	default:
	}

	return swarmLease.Clone(), nil
}

// DispatchWorkClaim delegates work claiming for a peer seat on a task.
func (o *BacklogObserver) DispatchWorkClaim(seatID, bliID string) (*SwarmTaskLease, error) {
	if o == nil {
		return nil, ErrNilBacklogObserver
	}
	return o.DispatchLease(bliID, seatID, o.config.LeaseDuration)
}

// EnqueuePlannedBLI proposes a planned backlog item to the ConsensusArbiter if needed.
func (o *BacklogObserver) EnqueuePlannedBLI(bliID string) error {
	if o == nil {
		return ErrNilBacklogObserver
	}
	if strings.TrimSpace(bliID) == "" {
		return ErrInvalidTaskID
	}
	if o.arbiter != nil {
		_, err := o.arbiter.ProposeTask(bliID, nil)
		if err != nil && !errors.Is(err, ErrTaskAlreadyExists) {
			return err
		}
	}
	return nil
}

// DetectAndLeaseTasks iterates unassigned backlog items, allocates leases to healthy peer seats, and emits lease events.
func (o *BacklogObserver) DetectAndLeaseTasks(taskIDs []string) ([]*SwarmTaskLease, error) {
	if o == nil {
		return nil, ErrNilBacklogObserver
	}
	seats := o.HealthySeats()
	if len(seats) == 0 {
		return nil, ErrNoHealthySeats
	}

	var leases []*SwarmTaskLease
	for i, taskID := range taskIDs {
		seatID := seats[i%len(seats)]
		l, err := o.DispatchLease(taskID, seatID, o.config.LeaseDuration)
		if err != nil {
			return leases, err
		}
		leases = append(leases, l)
	}
	return leases, nil
}

// RevokeLease releases or revokes an active task lease.
func (o *BacklogObserver) RevokeLease(taskID string, reasons ...string) error {
	return withObserverLock(o, func(obs *BacklogObserver) error {
		delete(obs.leases, taskID)
		if obs.arbiter != nil {
			reason := "lease revoked by observer"
			if len(reasons) > 0 && reasons[0] != "" {
				reason = reasons[0]
			}
			return obs.arbiter.RevokeTask(taskID, reason)
		}
		return nil
	})
}
