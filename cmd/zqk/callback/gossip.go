package callback

import (
	"context"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// NodeState defines the lifecycle state of a cluster peer in the SWIM membership protocol.
type NodeState string

const (
	NodeAlive   NodeState = "alive"
	NodeSuspect NodeState = "suspect"
	NodeDead    NodeState = "dead"
	NodeLeft    NodeState = "left"
)

// MemberState is an alias for NodeState for canonical naming.
type MemberState = NodeState

const (
	MemberStateAlive   MemberState = NodeAlive
	MemberStateSuspect MemberState = NodeSuspect
	MemberStateDead    MemberState = NodeDead
	MemberStateLeft    MemberState = NodeLeft
)

// GossipMember is an alias for NodeMetadata representing a cluster member.
type GossipMember = NodeMetadata

// GossipFailureDetector is an alias for SWIMFailureDetector.
type GossipFailureDetector = SWIMFailureDetector

// GossipSubscriber is an alias for SWIMFailureDetector implementing CallbackSubscriber.
type GossipSubscriber = SWIMFailureDetector

// Default timing, tuning, and metadata constants for the distributed gossip protocol.
const (
	DefaultGossipPingInterval   = 500 * time.Millisecond
	DefaultGossipPingTimeout    = 200 * time.Millisecond
	DefaultGossipSuspectTimeout = 1 * time.Second
	DefaultIndirectProbes       = 3

	CallbackTypeGossipMembership = "gossip_membership"
	DefaultSWIMSubscriberName    = "swim_failure_detector"

	FieldKeyGossipState    = "gossip_state"
	FieldKeyGossipOldState = "gossip_old_state"
	FieldKeyIncarnation    = "incarnation"
	FieldKeyMemberAddr     = "member_addr"
	FieldKeyTimestamp      = "timestamp"
)

var (
	_ CallbackSubscriber = (*SWIMFailureDetector)(nil)
	_ Subscriber         = (*SWIMFailureDetector)(nil)
)

// NodeMetadata encapsulates identity, address, state, and versioning for a cluster member.
type NodeMetadata struct {
	ID           string            `json:"id"`
	Addr         string            `json:"addr"`
	State        NodeState         `json:"state"`
	Incarnation  int64             `json:"incarnation"`
	LastSeen     time.Time         `json:"last_seen"`
	SuspectSince time.Time         `json:"suspect_since,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// Clone returns a deep copy of NodeMetadata to prevent race conditions across readers.
func (m NodeMetadata) Clone() NodeMetadata {
	cpy := m
	if m.Metadata != nil {
		cpy.Metadata = make(map[string]string, len(m.Metadata))
		for k, v := range m.Metadata {
			cpy.Metadata[k] = v
		}
	}
	return cpy
}

// GossipTransport defines network probing primitives for direct and indirect pinging.
type GossipTransport interface {
	Ping(ctx context.Context, targetID string) (bool, error)
	PingReq(ctx context.Context, relayID string, targetID string) (bool, error)
}

// MemoryGossipTransport provides an in-memory transport supporting fault and partition injection.
type MemoryGossipTransport struct {
	mu                sync.RWMutex
	unreachable       map[string]bool
	directUnreachable map[string]bool
	partitions        map[string]map[string]bool
	pingHooks         map[string]func(ctx context.Context, targetID string) (bool, error)
	pingReqHooks      map[string]func(ctx context.Context, relayID, targetID string) (bool, error)
}

// NewMemoryGossipTransport creates an initialized in-memory transport.
func NewMemoryGossipTransport() *MemoryGossipTransport {
	return &MemoryGossipTransport{
		unreachable:       make(map[string]bool),
		directUnreachable: make(map[string]bool),
		partitions:        make(map[string]map[string]bool),
		pingHooks:         make(map[string]func(ctx context.Context, targetID string) (bool, error)),
		pingReqHooks:      make(map[string]func(ctx context.Context, relayID, targetID string) (bool, error)),
	}
}

// SetUnreachable flags a node as completely unreachable or responsive.
func (m *MemoryGossipTransport) SetUnreachable(nodeID string, unreachable bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unreachable[nodeID] = unreachable
}

// SetDirectUnreachable sets whether direct pings to a target fail while indirect relays may still reach it.
func (m *MemoryGossipTransport) SetDirectUnreachable(nodeID string, unreachable bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.directUnreachable[nodeID] = unreachable
}

// SetPartition configures a directional network partition between from and to nodes.
func (m *MemoryGossipTransport) SetPartition(from, to string, partitioned bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.partitions[from]; !ok {
		m.partitions[from] = make(map[string]bool)
	}
	m.partitions[from][to] = partitioned
}

// HealPartition removes any configured partition between from and to nodes.
func (m *MemoryGossipTransport) HealPartition(from, to string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if inner, ok := m.partitions[from]; ok {
		delete(inner, to)
	}
}

// HealAll clears all partitions and unreachability flags.
func (m *MemoryGossipTransport) HealAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unreachable = make(map[string]bool)
	m.directUnreachable = make(map[string]bool)
	m.partitions = make(map[string]map[string]bool)
}

// SetPingHook registers a custom ping handler for a specific node ID.
func (m *MemoryGossipTransport) SetPingHook(nodeID string, hook func(ctx context.Context, targetID string) (bool, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pingHooks[nodeID] = hook
}

// SetPingReqHook registers a custom ping-req handler for a specific relay ID.
func (m *MemoryGossipTransport) SetPingReqHook(relayID string, hook func(ctx context.Context, relayID, targetID string) (bool, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pingReqHooks[relayID] = hook
}

// Ping performs a simulated direct ping to the target node.
func (m *MemoryGossipTransport) Ping(ctx context.Context, targetID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.unreachable[targetID] || m.directUnreachable[targetID] {
		return false, errfmt.Errorf("target node %s is unreachable", targetID)
	}
	if hook, ok := m.pingHooks[targetID]; ok && hook != nil {
		return hook(ctx, targetID)
	}
	return true, nil
}

// PingReq performs a simulated indirect ping via relay to the target node.
func (m *MemoryGossipTransport) PingReq(ctx context.Context, relayID string, targetID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.unreachable[relayID] {
		return false, errfmt.Errorf("relay node %s is unreachable", relayID)
	}
	if inner, ok := m.partitions[relayID]; ok && inner[targetID] {
		return false, errfmt.Errorf("relay %s partitioned from target %s", relayID, targetID)
	}
	if m.unreachable[targetID] {
		return false, errfmt.Errorf("target %s unreachable via relay %s", targetID, relayID)
	}
	if hook, ok := m.pingReqHooks[relayID]; ok && hook != nil {
		return hook(ctx, relayID, targetID)
	}
	return true, nil
}

// GossipConfig specifies configuration options for the gossip membership protocol.
type GossipConfig struct {
	LocalNodeID         string
	LocalAddr           string
	PingInterval        time.Duration
	PingTimeout         time.Duration
	SuspectTimeout      time.Duration
	IndirectProbes      int
	Clock               func() time.Time
	Transport           GossipTransport
	Dispatcher          *MultiSubscriberDispatcher
	Logger              logging.Logger
	OnMembershipChanged func(node NodeMetadata, oldState, newState NodeState)
}

// GossipMemberPool tracks active cluster nodes and handles conflict resolution.
type GossipMemberPool struct {
	mu      sync.RWMutex
	members map[string]NodeMetadata
	clock   func() time.Time
}

// NewGossipMemberPool initializes a thread-safe member pool.
func NewGossipMemberPool(clock func() time.Time) *GossipMemberPool {
	if clock == nil {
		clock = time.Now
	}
	return &GossipMemberPool{
		members: make(map[string]NodeMetadata),
		clock:   clock,
	}
}

// AddMember registers or updates a node in the pool.
func (p *GossipMemberPool) AddMember(meta NodeMetadata) error {
	if meta.ID == emptyValue {
		return errfmt.Errorf("member ID cannot be empty")
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if meta.LastSeen.IsZero() {
		meta.LastSeen = p.clock()
	}
	if meta.State == emptyValue {
		meta.State = NodeAlive
	}
	p.members[meta.ID] = meta.Clone()
	return nil
}

// RemoveMember unregisters a member from the pool.
func (p *GossipMemberPool) RemoveMember(nodeID string) error {
	if nodeID == emptyValue {
		return errfmt.Errorf("member ID cannot be empty")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.members, nodeID)
	return nil
}

// GetMember retrieves a copy of member metadata by node ID.
func (p *GossipMemberPool) GetMember(nodeID string) (NodeMetadata, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	m, ok := p.members[nodeID]
	if !ok {
		return NodeMetadata{}, false
	}
	return m.Clone(), true
}

// ListMembers returns a snapshot of members optionally filtered by state.
func (p *GossipMemberPool) ListMembers(filterState NodeState) []NodeMetadata {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]NodeMetadata, 0, len(p.members))
	for _, m := range p.members {
		if filterState == emptyValue || m.State == filterState {
			result = append(result, m.Clone())
		}
	}
	return result
}

// MemberCount returns the total number of members in the pool.
func (p *GossipMemberPool) MemberCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.members)
}

// TouchMember updates the LastSeen timestamp for a member.
func (p *GossipMemberPool) TouchMember(nodeID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if m, ok := p.members[nodeID]; ok {
		m.LastSeen = p.clock()
		p.members[nodeID] = m
	}
}

func (p *GossipMemberPool) mutateMemberStateLocked(nodeID string, curr NodeMetadata, state NodeState, incarnation int64) bool {
	now := p.clock()
	curr.State = state
	curr.Incarnation = incarnation
	curr.LastSeen = now
	if state == NodeSuspect {
		curr.SuspectSince = now
	} else {
		curr.SuspectSince = time.Time{}
	}
	p.members[nodeID] = curr
	return true
}

func (p *GossipMemberPool) modifyMember(nodeID string, mutateFn func(curr NodeMetadata) (NodeState, int64, bool)) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	curr, exists := p.members[nodeID]
	if !exists {
		return false
	}
	newState, newInc, ok := mutateFn(curr)
	if !ok {
		return false
	}
	return p.mutateMemberStateLocked(nodeID, curr, newState, newInc)
}

// ClearSuspicion clears suspect status, returning the node to alive.
func (p *GossipMemberPool) ClearSuspicion(nodeID string) bool {
	return p.modifyMember(nodeID, func(curr NodeMetadata) (NodeState, int64, bool) {
		if curr.State != NodeSuspect {
			return emptyValue, 0, false
		}
		return NodeAlive, curr.Incarnation, true
	})
}

// SetState explicitly sets node state and incarnation (used for local state or explicit partition healing).
func (p *GossipMemberPool) SetState(nodeID string, state NodeState, incarnation int64) bool {
	return p.modifyMember(nodeID, func(curr NodeMetadata) (NodeState, int64, bool) {
		return state, incarnation, true
	})
}

// UpdateState applies a state transition using SWIM incarnation and precedence rules.
func (p *GossipMemberPool) UpdateState(nodeID string, state NodeState, incarnation int64) bool {
	return p.modifyMember(nodeID, func(curr NodeMetadata) (NodeState, int64, bool) {
		if incarnation < curr.Incarnation {
			return emptyValue, 0, false
		}
		if incarnation > curr.Incarnation {
			return state, incarnation, true
		}
		if curr.State == state || !canTransitionAtSameIncarnation(curr.State, state) {
			return emptyValue, 0, false
		}
		return state, incarnation, true
	})
}

func canTransitionAtSameIncarnation(from, to NodeState) bool {
	switch from {
	case NodeAlive:
		return to == NodeSuspect || to == NodeDead || to == NodeLeft
	case NodeSuspect:
		return to == NodeDead || to == NodeLeft
	case NodeDead:
		return to == NodeLeft
	case NodeLeft:
		return false
	default:
		return false
	}
}

// SelectRandomMembers selects up to count alive members, excluding specified IDs.
func (p *GossipMemberPool) SelectRandomMembers(count int, excludeIDs ...string) []NodeMetadata {
	p.mu.RLock()
	defer p.mu.RUnlock()

	excluded := make(map[string]bool, len(excludeIDs))
	for _, id := range excludeIDs {
		excluded[id] = true
	}

	candidates := make([]NodeMetadata, 0, len(p.members))
	for id, m := range p.members {
		if !excluded[id] && m.State == NodeAlive {
			candidates = append(candidates, m.Clone())
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	rand.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	if count > len(candidates) {
		count = len(candidates)
	}
	return candidates[:count]
}

// SWIMFailureDetector orchestrates periodic direct and indirect health probing.
type SWIMFailureDetector struct {
	config GossipConfig
	pool   *GossipMemberPool

	localIncarnation atomic.Int64
	running          atomic.Bool
	loopCtx          context.Context
	loopCancel       context.CancelFunc
	wg               sync.WaitGroup

	probeIndex int
	mu         sync.Mutex
}

// NewSWIMFailureDetector initializes a failure detector with provided configuration.
func NewSWIMFailureDetector(cfg GossipConfig, pool *GossipMemberPool) (*SWIMFailureDetector, error) {
	if cfg.LocalNodeID == emptyValue {
		return nil, errfmt.Errorf("local node ID cannot be empty")
	}
	if cfg.PingInterval <= 0 {
		cfg.PingInterval = DefaultGossipPingInterval
	}
	if cfg.PingTimeout <= 0 {
		cfg.PingTimeout = DefaultGossipPingTimeout
	}
	if cfg.SuspectTimeout <= 0 {
		cfg.SuspectTimeout = DefaultGossipSuspectTimeout
	}
	if cfg.IndirectProbes <= 0 {
		cfg.IndirectProbes = DefaultIndirectProbes
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.Transport == nil {
		cfg.Transport = NewMemoryGossipTransport()
	}
	if pool == nil {
		pool = NewGossipMemberPool(cfg.Clock)
	}

	d := &SWIMFailureDetector{
		config: cfg,
		pool:   pool,
	}

	addErr := pool.AddMember(NodeMetadata{
		ID:          cfg.LocalNodeID,
		Addr:        cfg.LocalAddr,
		State:       NodeAlive,
		Incarnation: 0,
		LastSeen:    cfg.Clock(),
	})
	if addErr != nil {
		return nil, addErr
	}

	return d, nil
}

// NewGossipFailureDetector creates an initialized failure detector using GossipConfig.
func NewGossipFailureDetector(cfg GossipConfig, pool *GossipMemberPool) (*GossipFailureDetector, error) {
	return NewSWIMFailureDetector(cfg, pool)
}

// NewGossipSubscriber creates a new GossipSubscriber.
func NewGossipSubscriber(cfg GossipConfig, pool *GossipMemberPool) (*GossipSubscriber, error) {
	return NewSWIMFailureDetector(cfg, pool)
}

// Name returns the canonical subscriber identifier.
func (d *SWIMFailureDetector) Name() string {
	return DefaultSWIMSubscriberName
}

// Pool returns the associated member pool.
func (d *SWIMFailureDetector) Pool() *GossipMemberPool {
	return d.pool
}

// LocalIncarnation returns the active monotonic incarnation number of the local node.
func (d *SWIMFailureDetector) LocalIncarnation() int64 {
	return d.localIncarnation.Load()
}

// IsRunning reports whether the periodic failure detector loop is actively executing.
func (d *SWIMFailureDetector) IsRunning() bool {
	return d.running.Load()
}

// Start launches the background failure detection and suspect timeout loop.
func (d *SWIMFailureDetector) Start(ctx context.Context) error {
	if !d.running.CompareAndSwap(false, true) {
		return nil
	}

	d.loopCtx, d.loopCancel = context.WithCancel(ctx)
	d.wg.Add(1)
	goroutinelabels.NewGoroutine("swim_failure_detector_loop", "background swim periodic probing and suspect timeout loop").
		StartSimple(d.runLoop)

	return nil
}

func (d *SWIMFailureDetector) runLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(d.config.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.loopCtx.Done():
			return
		case <-ticker.C:
			d.tick()
		}
	}
}

func (d *SWIMFailureDetector) tick() {
	probeErr := d.Probe(d.loopCtx)
	if probeErr != nil && d.config.Logger != nil {
		d.config.Logger.Warn("swim probe iteration error", logging.Error(probeErr))
	}
	d.checkSuspectTimeouts(d.loopCtx)
}

// Stop terminates the background failure detector loop and waits for all goroutines to finish.
func (d *SWIMFailureDetector) Stop() error {
	if !d.running.CompareAndSwap(true, false) {
		return nil
	}

	if d.loopCancel != nil {
		d.loopCancel()
	}
	d.wg.Wait()
	return nil
}

// Ping sends a direct ping probe to the target node.
func (d *SWIMFailureDetector) Ping(ctx context.Context, targetID string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	pingCtx, cancel := context.WithTimeout(ctx, d.config.PingTimeout)
	defer cancel()

	ack, err := d.config.Transport.Ping(pingCtx, targetID)
	if err != nil {
		return false, err
	}
	return ack, nil
}

type pingReqResult struct {
	ack bool
	err error
}

// PingReq issues indirect ping requests through specified relay nodes to the target.
func (d *SWIMFailureDetector) PingReq(ctx context.Context, targetID string, relays []string) (bool, error) {
	if len(relays) == 0 {
		return false, errfmt.Errorf("no relays provided for ping-req")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	reqCtx, cancel := context.WithTimeout(ctx, d.config.PingTimeout)
	defer cancel()

	resCh := make(chan pingReqResult, len(relays))
	for _, relayID := range relays {
		rID := relayID
		goroutinelabels.NewGoroutine("swim_ping_req_relay", "indirect probe relay").
			StartSimple(func() {
				ack, err := d.config.Transport.PingReq(reqCtx, rID, targetID)
				resCh <- pingReqResult{ack: ack, err: err}
			})
	}

	var lastErr error
	for i := 0; i < len(relays); i++ {
		select {
		case res := <-resCh:
			if res.ack && res.err == nil {
				return true, nil
			}
			if res.err != nil {
				lastErr = res.err
			}
		case <-reqCtx.Done():
			return false, reqCtx.Err()
		}
	}

	if lastErr != nil {
		return false, lastErr
	}
	return false, nil
}

// Probe selects a cluster peer and executes direct followed by indirect probing.
func (d *SWIMFailureDetector) Probe(ctx context.Context) error {
	target, ok := d.selectProbeTarget()
	if !ok {
		return nil
	}

	ack, pingErr := d.Ping(ctx, target.ID)
	if pingErr == nil && ack {
		d.handleProbeSuccess(ctx, target)
		return nil
	}

	relays := d.selectRelays(target.ID)
	if len(relays) > 0 {
		relayAck, relayErr := d.PingReq(ctx, target.ID, relays)
		if relayErr == nil && relayAck {
			d.handleProbeSuccess(ctx, target)
			return nil
		}
	}

	d.handleProbeFailure(ctx, target)
	return nil
}

func (d *SWIMFailureDetector) selectProbeTarget() (NodeMetadata, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	members := d.pool.ListMembers("")
	candidates := make([]NodeMetadata, 0, len(members))
	for _, m := range members {
		if m.ID != d.config.LocalNodeID && m.State != NodeDead && m.State != NodeLeft {
			candidates = append(candidates, m)
		}
	}
	if len(candidates) == 0 {
		return NodeMetadata{}, false
	}

	idx := d.probeIndex % len(candidates)
	d.probeIndex++
	return candidates[idx], true
}

func (d *SWIMFailureDetector) selectRelays(targetID string) []string {
	nodes := d.pool.SelectRandomMembers(d.config.IndirectProbes, d.config.LocalNodeID, targetID)
	relays := make([]string, 0, len(nodes))
	for _, n := range nodes {
		relays = append(relays, n.ID)
	}
	return relays
}

func (d *SWIMFailureDetector) handleProbeSuccess(ctx context.Context, target NodeMetadata) {
	d.pool.TouchMember(target.ID)
	if target.State == NodeSuspect {
		cleared := d.pool.ClearSuspicion(target.ID)
		if cleared {
			d.dispatchStateChange(ctx, target.ID, NodeSuspect, NodeAlive, target.Incarnation, target.Addr)
		}
	}
}

func (d *SWIMFailureDetector) handleProbeFailure(ctx context.Context, target NodeMetadata) {
	if target.State == NodeAlive {
		updated := d.pool.UpdateState(target.ID, NodeSuspect, target.Incarnation)
		if updated {
			d.dispatchStateChange(ctx, target.ID, NodeAlive, NodeSuspect, target.Incarnation, target.Addr)
		}
	}
}

func (d *SWIMFailureDetector) checkSuspectTimeouts(ctx context.Context) {
	suspects := d.pool.ListMembers(NodeSuspect)
	now := d.config.Clock()

	for _, s := range suspects {
		if s.ID == d.config.LocalNodeID {
			continue
		}
		if !s.SuspectSince.IsZero() && now.Sub(s.SuspectSince) >= d.config.SuspectTimeout {
			updated := d.pool.UpdateState(s.ID, NodeDead, s.Incarnation)
			if updated {
				d.dispatchStateChange(ctx, s.ID, NodeSuspect, NodeDead, s.Incarnation, s.Addr)
			}
		}
	}
}

// Refute asserts node liveness by incrementing incarnation and disseminating NodeAlive.
func (d *SWIMFailureDetector) Refute(ctx context.Context) error {
	nextInc := d.localIncarnation.Add(1)
	updated := d.pool.SetState(d.config.LocalNodeID, NodeAlive, nextInc)
	if updated {
		m, ok := d.pool.GetMember(d.config.LocalNodeID)
		addr := ""
		if ok {
			addr = m.Addr
		}
		d.dispatchStateChange(ctx, d.config.LocalNodeID, NodeSuspect, NodeAlive, nextInc, addr)
	}
	return nil
}

// Leave announces graceful departure from the cluster and terminates failure detection.
func (d *SWIMFailureDetector) Leave(ctx context.Context) error {
	inc := d.localIncarnation.Load()
	updated := d.pool.SetState(d.config.LocalNodeID, NodeLeft, inc)
	if updated {
		m, ok := d.pool.GetMember(d.config.LocalNodeID)
		addr := ""
		if ok {
			addr = m.Addr
		}
		d.dispatchStateChange(ctx, d.config.LocalNodeID, NodeAlive, NodeLeft, inc, addr)
	}
	return d.Stop()
}

// Notify responds to incoming callback entries, updating membership and refuting suspicions.
func (d *SWIMFailureDetector) Notify(ctx context.Context, entry *CallbackEntry) error {
	if entry == nil || entry.Payload == nil {
		return nil
	}
	cbType := objects.GetString(entry.Payload, objects.FieldKeyCallbackType)
	if cbType != CallbackTypeGossipMembership {
		return nil
	}

	nodeID := objects.GetString(entry.Payload, FieldKeyNodeID)
	if nodeID == emptyValue {
		return nil
	}

	stateStr := objects.GetString(entry.Payload, FieldKeyGossipState)
	newState := NodeState(stateStr)
	incarnation := coerceIncarnationNumber(entry.Payload[FieldKeyIncarnation])

	if nodeID == d.config.LocalNodeID {
		if newState == NodeSuspect && incarnation >= d.localIncarnation.Load() {
			return d.Refute(ctx)
		}
		return nil
	}

	return d.handlePeerGossipUpdate(nodeID, newState, incarnation, entry.Payload)
}

func (d *SWIMFailureDetector) handlePeerGossipUpdate(nodeID string, newState NodeState, incarnation int64, payload map[string]any) error {
	oldMem, exists := d.pool.GetMember(nodeID)
	oldState := NodeState("")
	if exists {
		oldState = oldMem.State
	} else if newState == NodeAlive {
		addr := objects.GetString(payload, FieldKeyMemberAddr)
		addErr := d.pool.AddMember(NodeMetadata{
			ID:          nodeID,
			Addr:        addr,
			State:       NodeAlive,
			Incarnation: incarnation,
			LastSeen:    d.config.Clock(),
		})
		if addErr != nil {
			return addErr
		}
		d.notifyMembershipCallback(nodeID, oldState, newState)
		return nil
	}

	updated := d.pool.UpdateState(nodeID, newState, incarnation)
	if updated {
		d.notifyMembershipCallback(nodeID, oldState, newState)
	}
	return nil
}

func (d *SWIMFailureDetector) notifyMembershipCallback(nodeID string, oldState, newState NodeState) {
	if d.config.OnMembershipChanged != nil {
		mem, ok := d.pool.GetMember(nodeID)
		if ok {
			d.config.OnMembershipChanged(mem, oldState, newState)
		}
	}
}

func (d *SWIMFailureDetector) dispatchStateChange(ctx context.Context, nodeID string, oldState, newState NodeState, incarnation int64, addr string) {
	d.notifyMembershipCallback(nodeID, oldState, newState)

	if d.config.Dispatcher == nil {
		return
	}

	entry := &CallbackEntry{
		Payload: map[string]any{
			objects.FieldKeyCallbackType: CallbackTypeGossipMembership,
			FieldKeyNodeID:               nodeID,
			FieldKeyGossipState:          string(newState),
			FieldKeyGossipOldState:       string(oldState),
			FieldKeyIncarnation:          incarnation,
			FieldKeyMemberAddr:           addr,
			FieldKeyTimestamp:            d.config.Clock().Format(time.RFC3339Nano),
		},
		Timestamp: d.config.Clock(),
		JobID:     "gossip:" + nodeID,
		Priority:  100,
	}

	dispatchErr := d.config.Dispatcher.Dispatch(ctx, entry)
	if dispatchErr != nil && d.config.Logger != nil {
		d.config.Logger.Warn("failed to dispatch gossip membership change",
			logging.String("node_id", nodeID),
			logging.String("new_state", string(newState)),
			logging.Error(dispatchErr))
	}
}

func coerceIncarnationNumber(raw any) int64 {
	switch v := raw.(type) {
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
