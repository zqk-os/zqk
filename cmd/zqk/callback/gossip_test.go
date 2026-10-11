package callback

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

type gossipTestHarness struct {
	transport   *MemoryGossipTransport
	dispatcher  *MultiSubscriberDispatcher
	pool        *GossipMemberPool
	detector    *SWIMFailureDetector
	events      []*CallbackEntry
	eventsMu    sync.Mutex
	virtualTime time.Time
	timeMu      sync.RWMutex
}

func (h *gossipTestHarness) now() time.Time {
	h.timeMu.RLock()
	defer h.timeMu.RUnlock()
	return h.virtualTime
}

func (h *gossipTestHarness) advanceTime(d time.Duration) {
	h.timeMu.Lock()
	defer h.timeMu.Unlock()
	h.virtualTime = h.virtualTime.Add(d)
}

func (h *gossipTestHarness) recordEvent(entry *CallbackEntry) {
	h.eventsMu.Lock()
	defer h.eventsMu.Unlock()
	h.events = append(h.events, entry)
}

func (h *gossipTestHarness) getEvents() []*CallbackEntry {
	h.eventsMu.Lock()
	defer h.eventsMu.Unlock()
	cpy := make([]*CallbackEntry, len(h.events))
	copy(cpy, h.events)
	return cpy
}

func newGossipHarness(t *testing.T, localID string, peerIDs ...string) *gossipTestHarness {
	h := &gossipTestHarness{
		transport:   NewMemoryGossipTransport(),
		dispatcher:  NewMultiSubscriberDispatcher(nil),
		virtualTime: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC),
	}

	h.pool = NewGossipMemberPool(h.now)
	sub := NewFuncSubscriber("test_recorder", func(ctx context.Context, entry *CallbackEntry) error {
		h.recordEvent(entry)
		return nil
	})
	h.dispatcher.Register(sub)

	cfg := GossipConfig{
		LocalNodeID:    localID,
		LocalAddr:      "127.0.0.1:9000",
		PingInterval:   50 * time.Millisecond,
		PingTimeout:    30 * time.Millisecond,
		SuspectTimeout: 100 * time.Millisecond,
		IndirectProbes: 3,
		Clock:          h.now,
		Transport:      h.transport,
		Dispatcher:     h.dispatcher,
	}

	detector, err := NewSWIMFailureDetector(cfg, h.pool)
	require.NoError(t, err)
	h.detector = detector

	for _, peerID := range peerIDs {
		h.addPeer(t, peerID)
	}

	return h
}

func (h *gossipTestHarness) addPeer(t *testing.T, peerID string) {
	err := h.pool.AddMember(NodeMetadata{
		ID:          peerID,
		Addr:        fmt.Sprintf("127.0.0.1:%s", peerID),
		State:       NodeAlive,
		Incarnation: 0,
		LastSeen:    h.now(),
	})
	require.NoError(t, err)
}

func (h *gossipTestHarness) assertPeerState(t *testing.T, peerID string, expected NodeState) {
	mem, ok := h.pool.GetMember(peerID)
	require.True(t, ok, "member %s should exist in pool", peerID)
	assert.Equal(t, expected, mem.State)
}

func (h *gossipTestHarness) assertPeerIncarnation(t *testing.T, peerID string, expectedInc int64) {
	mem, ok := h.pool.GetMember(peerID)
	require.True(t, ok, "member %s should exist in pool", peerID)
	assert.Equal(t, expectedInc, mem.Incarnation)
}

func TestGossipMemberPool_LifecycleAndTransitions(t *testing.T) {
	h := newGossipHarness(t, "node-local", "node-1")
	assert.Equal(t, 2, h.pool.MemberCount())

	h.assertPeerState(t, "node-1", NodeAlive)

	// Valid transition: Alive -> Suspect
	updated := h.pool.UpdateState("node-1", NodeSuspect, 0)
	assert.True(t, updated)
	h.assertPeerState(t, "node-1", NodeSuspect)

	// Stale incarnation rejected
	staleUpdate := h.pool.UpdateState("node-1", NodeAlive, -1)
	assert.False(t, staleUpdate)
	h.assertPeerState(t, "node-1", NodeSuspect)

	// Higher incarnation overrides
	higherUpdate := h.pool.UpdateState("node-1", NodeAlive, 1)
	assert.True(t, higherUpdate)
	h.assertPeerState(t, "node-1", NodeAlive)
	h.assertPeerIncarnation(t, "node-1", 1)

	// Removal
	removeErr := h.pool.RemoveMember("node-1")
	assert.NoError(t, removeErr)
	assert.Equal(t, 1, h.pool.MemberCount())
}

func TestSWIMFailureDetector_DirectPingSuccessAndFailure(t *testing.T) {
	ctx := context.Background()
	h := newGossipHarness(t, "node-local", "node-1")

	// 1. Direct Ping success
	ack, pingErr := h.detector.Ping(ctx, "node-1")
	assert.NoError(t, pingErr)
	assert.True(t, ack)

	// 2. Direct Ping failure when target is marked unreachable
	h.transport.SetUnreachable("node-1", true)
	failAck, failErr := h.detector.Ping(ctx, "node-1")
	assert.Error(t, failErr)
	assert.False(t, failAck)

	// 3. Probing with no relays transitions directly to suspect
	probeErr := h.detector.Probe(ctx)
	assert.NoError(t, probeErr)
	h.assertPeerState(t, "node-1", NodeSuspect)
}

func TestSWIMFailureDetector_IndirectPingReqFallback(t *testing.T) {
	ctx := context.Background()
	h := newGossipHarness(t, "node-local", "node-1", "relay-1", "relay-2")

	// Target direct ping is broken, but relay-1 can reach target
	h.transport.SetDirectUnreachable("node-1", true)
	h.transport.SetPartition("relay-2", "node-1", true)

	// PingReq through relays succeeds because relay-1 is not partitioned
	reqAck, reqErr := h.detector.PingReq(ctx, "node-1", []string{"relay-1", "relay-2"})
	assert.NoError(t, reqErr)
	assert.True(t, reqAck)

	// Now partition relay-1 as well
	h.transport.SetPartition("relay-1", "node-1", true)
	allFailAck, allFailErr := h.detector.PingReq(ctx, "node-1", []string{"relay-1", "relay-2"})
	assert.Error(t, allFailErr)
	assert.False(t, allFailAck)
}

func TestSWIMFailureDetector_SuspectTimeoutToDead(t *testing.T) {
	ctx := context.Background()
	h := newGossipHarness(t, "node-local", "node-1")

	// Mark node-1 as unreachable and probe to transition to Suspect
	h.transport.SetUnreachable("node-1", true)
	probeErr := h.detector.Probe(ctx)
	assert.NoError(t, probeErr)
	h.assertPeerState(t, "node-1", NodeSuspect)

	// Advance virtual time beyond suspect timeout
	h.advanceTime(150 * time.Millisecond)
	h.detector.checkSuspectTimeouts(ctx)

	// Target should now be Dead
	h.assertPeerState(t, "node-1", NodeDead)
}

func TestSWIMFailureDetector_LocalNodeRefutation(t *testing.T) {
	ctx := context.Background()
	h := newGossipHarness(t, "node-local", "node-1")
	assert.Equal(t, int64(0), h.detector.LocalIncarnation())

	// Incoming gossip falsely claims local node is suspect at incarnation 0
	suspectEntry := &CallbackEntry{
		Payload: map[string]any{
			objects.FieldKeyCallbackType: CallbackTypeGossipMembership,
			FieldKeyNodeID:               "node-local",
			FieldKeyGossipState:          string(NodeSuspect),
			FieldKeyIncarnation:          int64(0),
		},
		Timestamp: h.now(),
	}

	notifyErr := h.detector.Notify(ctx, suspectEntry)
	assert.NoError(t, notifyErr)

	// Local node should have refuted, bumping incarnation to 1 and remaining Alive
	assert.Equal(t, int64(1), h.detector.LocalIncarnation())
	h.assertPeerState(t, "node-local", NodeAlive)

	events := h.getEvents()
	require.NotEmpty(t, events)
	lastEvent := events[len(events)-1]
	assert.Equal(t, string(NodeAlive), lastEvent.Payload[FieldKeyGossipState])
	assert.Equal(t, int64(1), lastEvent.Payload[FieldKeyIncarnation])
}

func TestSWIMFailureDetector_GracefulLeave(t *testing.T) {
	ctx := context.Background()
	h := newGossipHarness(t, "node-local", "node-1")

	startErr := h.detector.Start(ctx)
	assert.NoError(t, startErr)
	assert.True(t, h.detector.IsRunning())

	leaveErr := h.detector.Leave(ctx)
	assert.NoError(t, leaveErr)
	assert.False(t, h.detector.IsRunning())
	h.assertPeerState(t, "node-local", NodeLeft)

	events := h.getEvents()
	require.NotEmpty(t, events)
	lastEvent := events[len(events)-1]
	assert.Equal(t, string(NodeLeft), lastEvent.Payload[FieldKeyGossipState])
}

func TestSWIMFailureDetector_NetworkPartitionAndHealing(t *testing.T) {
	ctx := context.Background()
	h := newGossipHarness(t, "node-local", "node-partitioned")

	// Simulate partition: direct and indirect probes fail
	h.transport.SetUnreachable("node-partitioned", true)
	probeErr := h.detector.Probe(ctx)
	assert.NoError(t, probeErr)
	h.assertPeerState(t, "node-partitioned", NodeSuspect)

	h.advanceTime(150 * time.Millisecond)
	h.detector.checkSuspectTimeouts(ctx)
	h.assertPeerState(t, "node-partitioned", NodeDead)

	// Partition heals: node-partitioned sends Alive heartbeat with bumped incarnation
	h.transport.HealAll()
	healEntry := &CallbackEntry{
		Payload: map[string]any{
			objects.FieldKeyCallbackType: CallbackTypeGossipMembership,
			FieldKeyNodeID:               "node-partitioned",
			FieldKeyGossipState:          string(NodeAlive),
			FieldKeyIncarnation:          int64(1),
		},
		Timestamp: h.now(),
	}

	notifyErr := h.detector.Notify(ctx, healEntry)
	assert.NoError(t, notifyErr)
	h.assertPeerState(t, "node-partitioned", NodeAlive)
	h.assertPeerIncarnation(t, "node-partitioned", 1)
}

func TestSWIMFailureDetector_CallbackSubscriberIntegration(t *testing.T) {
	ctx := context.Background()
	h := newGossipHarness(t, "node-local")

	// Register detector as subscriber in dispatcher
	h.dispatcher.Register(h.detector)

	// Broadcast membership event for a new peer
	newPeerEntry := &CallbackEntry{
		Payload: map[string]any{
			objects.FieldKeyCallbackType: CallbackTypeGossipMembership,
			FieldKeyNodeID:               "node-discovered",
			FieldKeyGossipState:          string(NodeAlive),
			FieldKeyIncarnation:          int64(0),
			FieldKeyMemberAddr:           "10.0.0.5:9000",
		},
		Timestamp: h.now(),
	}

	dispatchErr := h.dispatcher.Dispatch(ctx, newPeerEntry)
	assert.NoError(t, dispatchErr)

	// Peer should be learned dynamically
	h.assertPeerState(t, "node-discovered", NodeAlive)
	mem, ok := h.pool.GetMember("node-discovered")
	assert.True(t, ok)
	assert.Equal(t, "10.0.0.5:9000", mem.Addr)
}

func TestSWIMFailureDetector_ConcurrentProbingUnderRace(t *testing.T) {
	ctx := context.Background()
	h := newGossipHarness(t, "node-local", "node-1", "node-2", "node-3")

	startErr := h.detector.Start(ctx)
	assert.NoError(t, startErr)

	const workerCount = 6
	var wg sync.WaitGroup
	var ops atomic.Int64

	for i := 0; i < workerCount; i++ {
		workerID := i
		wg.Add(1)
		goroutinelabels.NewGoroutine("swim_race_test_worker", "concurrent membership updater").
			StartSimple(func() {
				defer wg.Done()
				nodeID := fmt.Sprintf("node-%d", (workerID%3)+1)
				var lastMem NodeMetadata
				for j := 0; j < 25; j++ {
					h.pool.TouchMember(nodeID)
					targetMem, ok := h.pool.GetMember(nodeID)
					if ok {
						ops.Add(1)
						lastMem = targetMem
					}
					probeErr := h.detector.Probe(ctx)
					if probeErr == nil {
						ops.Add(1)
					}
					time.Sleep(1 * time.Millisecond)
				}
				assert.NotEmpty(t, lastMem.ID)
			})
	}

	wg.Wait()
	assert.Positive(t, ops.Load())

	stopErr := h.detector.Stop()
	assert.NoError(t, stopErr)
}

func TestSWIMFailureDetector_BoundaryAndErrorHandling(t *testing.T) {
	ctx := context.Background()

	// 1. Invalid configuration
	invalidCfg := GossipConfig{LocalNodeID: ""}
	invalidDetector, cfgErr := NewSWIMFailureDetector(invalidCfg, nil)
	assert.Error(t, cfgErr)
	assert.Nil(t, invalidDetector)

	// 2. Empty member pool operations
	pool := NewGossipMemberPool(nil)
	emptyAddErr := pool.AddMember(NodeMetadata{ID: ""})
	assert.Error(t, emptyAddErr)
	emptyRemoveErr := pool.RemoveMember("")
	assert.Error(t, emptyRemoveErr)
	emptyMem, found := pool.GetMember("non-existent")
	assert.False(t, found)
	assert.Empty(t, emptyMem.ID)

	// 3. PingReq with zero relays
	h := newGossipHarness(t, "node-local", "node-1")
	reqAck, reqErr := h.detector.PingReq(ctx, "node-1", []string{})
	assert.Error(t, reqErr)
	assert.False(t, reqAck)

	// 4. Duplicate Start / Stop idempotency
	startErr1 := h.detector.Start(ctx)
	assert.NoError(t, startErr1)
	startErr2 := h.detector.Start(ctx)
	assert.NoError(t, startErr2)

	stopErr1 := h.detector.Stop()
	assert.NoError(t, stopErr1)
	stopErr2 := h.detector.Stop()
	assert.NoError(t, stopErr2)
}

func TestGossipMember_CanonicalAliasesAndConstructors(t *testing.T) {
	t.Parallel()

	assert.Equal(t, MemberStateAlive, NodeAlive)
	assert.Equal(t, MemberStateSuspect, NodeSuspect)
	assert.Equal(t, MemberStateDead, NodeDead)
	assert.Equal(t, MemberStateLeft, NodeLeft)

	now := time.Now()
	member := GossipMember{
		ID:          "alias-node",
		Addr:        "127.0.0.1:8080",
		State:       MemberStateAlive,
		Incarnation: 1,
		LastSeen:    now,
	}
	assert.Equal(t, "alias-node", member.ID)
	assert.Equal(t, MemberStateAlive, member.State)

	cfg := GossipConfig{
		LocalNodeID: "alias-node",
	}
	detector, err := NewGossipFailureDetector(cfg, nil)
	require.NoError(t, err)
	assert.NotNil(t, detector)

	sub, err := NewGossipSubscriber(cfg, nil)
	require.NoError(t, err)
	assert.NotNil(t, sub)
	assert.Equal(t, DefaultSWIMSubscriberName, sub.Name())
}
