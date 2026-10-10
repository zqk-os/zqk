package callback

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

func newTestElector(t *testing.T, nodeID string, store LeaderLeaseStore, ttl time.Duration, dispatcher *MultiSubscriberDispatcher) *LeaderElector {
	t.Helper()
	cfg := LeaderElectorConfig{
		NodeID:            nodeID,
		LeaseTTL:          ttl,
		HeartbeatInterval: ttl / 2,
		Store:             store,
		Dispatcher:        dispatcher,
	}
	elector, err := NewLeaderElector(cfg)
	if err != nil {
		t.Fatalf("failed to create leader elector for %s: %v", nodeID, err)
	}
	return elector
}

func assertRole(t *testing.T, e *LeaderElector, expected LeaderRole) {
	t.Helper()
	if got := e.Role(); got != expected {
		t.Fatalf("expected role %s, got %s", expected, got)
	}
}

func assertLeader(t *testing.T, e *LeaderElector, expected bool) {
	t.Helper()
	if got := e.IsLeader(); got != expected {
		t.Fatalf("expected IsLeader() == %v, got %v", expected, got)
	}
}

func assertFencingToken(t *testing.T, e *LeaderElector, expected int64) {
	t.Helper()
	if got := e.FencingToken(); got != expected {
		t.Fatalf("expected fencing token %d, got %d", expected, got)
	}
}

type electorHarness struct {
	ctx     context.Context
	store   *MemoryLeaderLeaseStore
	elector *LeaderElector
}

func newElectorHarness(t *testing.T, nodeID string, ttl time.Duration, dispatcher *MultiSubscriberDispatcher) electorHarness {
	t.Helper()
	store := NewMemoryLeaderLeaseStore()
	return electorHarness{
		ctx:     context.Background(),
		store:   store,
		elector: newTestElector(t, nodeID, store, ttl, dispatcher),
	}
}

func assertTokenValidation(t *testing.T, e *LeaderElector, token int64, expected bool) {
	t.Helper()
	if got := e.ValidateFencingToken(token); got != expected {
		t.Fatalf("ValidateFencingToken(%d) = %v, expected %v", token, got, expected)
	}
}

func TestLeaderElector_SingleNodeCampaignAndElection(t *testing.T) {
	t.Parallel()
	h := newElectorHarness(t, "node-single", 500*time.Millisecond, nil)

	assertRole(t, h.elector, RoleFollower)
	assertLeader(t, h.elector, false)

	won, err := h.elector.Campaign(h.ctx)
	if err != nil {
		t.Fatalf("unexpected campaign error: %v", err)
	}
	if !won {
		t.Fatalf("expected single node to win campaign")
	}

	assertRole(t, h.elector, RoleLeader)
	assertLeader(t, h.elector, true)
	if got := h.elector.LeaderID(); got != "node-single" {
		t.Fatalf("expected leader ID 'node-single', got %q", got)
	}
	assertFencingToken(t, h.elector, 1)

	lease := h.elector.CurrentLease()
	if lease == nil {
		t.Fatalf("expected non-nil lease for leader")
	}
	if lease.LeaderID != "node-single" || lease.FencingToken != 1 {
		t.Fatalf("unexpected lease contents: %+v", lease)
	}
}

func TestLeaderElector_LeaseRenewal(t *testing.T) {
	t.Parallel()
	h := newElectorHarness(t, "node-renew", 300*time.Millisecond, nil)

	won, err := h.elector.Campaign(h.ctx)
	if err != nil || !won {
		t.Fatalf("campaign failed: won=%v, err=%v", won, err)
	}

	initialLease := h.elector.CurrentLease()
	time.Sleep(50 * time.Millisecond)

	if renewErr := h.elector.Renew(h.ctx); renewErr != nil {
		t.Fatalf("unexpected renew error: %v", renewErr)
	}

	renewedLease := h.elector.CurrentLease()
	if !renewedLease.ExpiresAt.After(initialLease.ExpiresAt) {
		t.Fatalf("expected renewed expiry %v to be after initial expiry %v", renewedLease.ExpiresAt, initialLease.ExpiresAt)
	}
	assertLeader(t, h.elector, true)
	assertRole(t, h.elector, RoleLeader)
}

func TestLeaderElector_LeaseExpirationAndSuccession(t *testing.T) {
	t.Parallel()
	ttl := 100 * time.Millisecond
	h := newElectorHarness(t, "node-1", ttl, nil)
	elector2 := newTestElector(t, "node-2", h.store, ttl, nil)

	won1, err1 := h.elector.Campaign(h.ctx)
	if err1 != nil || !won1 {
		t.Fatalf("node-1 campaign failed: won=%v, err=%v", won1, err1)
	}

	// While node-1 holds lease, node-2 campaign must fail
	won2, err2 := elector2.Campaign(h.ctx)
	if err2 != nil {
		t.Fatalf("node-2 campaign unexpected error: %v", err2)
	}
	if won2 {
		t.Fatalf("node-2 should not win while node-1 holds unexpired lease")
	}
	assertRole(t, elector2, RoleFollower)

	// Wait for node-1 lease to expire
	time.Sleep(ttl + 30*time.Millisecond)

	// Node 1 should now be expired
	assertLeader(t, h.elector, false)
	assertRole(t, h.elector, RoleFollower)

	// Node 2 campaigns and wins succession
	wonSuccession, errSucc := elector2.Campaign(h.ctx)
	if errSucc != nil || !wonSuccession {
		t.Fatalf("node-2 failed to succeed expired leader: won=%v, err=%v", wonSuccession, errSucc)
	}

	assertLeader(t, elector2, true)
	assertRole(t, elector2, RoleLeader)
	if got := elector2.LeaderID(); got != "node-2" {
		t.Fatalf("expected leader ID 'node-2', got %q", got)
	}
	assertFencingToken(t, elector2, 2)
}

func TestLeaderElector_SplitBrainFencingTokenRejection(t *testing.T) {
	t.Parallel()
	h := newElectorHarness(t, "node-split", 500*time.Millisecond, nil)

	// Non-positive tokens rejected
	assertTokenValidation(t, h.elector, 0, false)
	assertTokenValidation(t, h.elector, -1, false)

	// Campaign to establish fencing token 1
	won, err := h.elector.Campaign(h.ctx)
	if err != nil || !won {
		t.Fatalf("campaign failed: won=%v, err=%v", won, err)
	}
	assertFencingToken(t, h.elector, 1)

	// Active token is 1
	assertTokenValidation(t, h.elector, 1, true)

	// Advance to token 5
	assertTokenValidation(t, h.elector, 5, true)

	// Older tokens (1, 2, 3, 4) must now be rejected as stale split-brain tokens
	assertTokenValidation(t, h.elector, 1, false)
	assertTokenValidation(t, h.elector, 2, false)
	assertTokenValidation(t, h.elector, 3, false)
	assertTokenValidation(t, h.elector, 4, false)

	// Monotonically equal or higher is accepted
	assertTokenValidation(t, h.elector, 5, true)
	assertTokenValidation(t, h.elector, 6, true)
}

func TestLeaderElector_VoluntaryStepDown(t *testing.T) {
	t.Parallel()
	ttl := 1 * time.Second
	h := newElectorHarness(t, "node-step-1", ttl, nil)
	elector2 := newTestElector(t, "node-step-2", h.store, ttl, nil)

	won1, err1 := h.elector.Campaign(h.ctx)
	if err1 != nil || !won1 {
		t.Fatalf("node-step-1 campaign failed: won=%v, err=%v", won1, err1)
	}
	assertLeader(t, h.elector, true)

	// Voluntary step down
	if err := h.elector.StepDown(h.ctx); err != nil {
		t.Fatalf("step down failed: %v", err)
	}
	assertLeader(t, h.elector, false)
	assertRole(t, h.elector, RoleFollower)

	// Node 2 can immediately claim leadership without waiting for TTL
	won2, err2 := elector2.Campaign(h.ctx)
	if err2 != nil || !won2 {
		t.Fatalf("node-step-2 immediate takeover failed: won=%v, err=%v", won2, err2)
	}
	assertLeader(t, elector2, true)
	assertRole(t, elector2, RoleLeader)
}

func TestLeaderElector_ConcurrentCampaigns(t *testing.T) {
	t.Parallel()
	nodeCount := 10
	ttl := 2 * time.Second
	h := newElectorHarness(t, "node-concurrent-0", ttl, nil)

	electors := make([]*LeaderElector, nodeCount)
	electors[0] = h.elector
	for i := 1; i < nodeCount; i++ {
		electors[i] = newTestElector(t, fmt.Sprintf("node-concurrent-%d", i), h.store, ttl, nil)
	}

	var wg sync.WaitGroup
	var winners atomic.Int32

	for i := 0; i < nodeCount; i++ {
		wg.Add(1)
		idx := i
		goroutinelabels.NewGoroutine(fmt.Sprintf("concurrent_campaign_%d", idx), "concurrent campaign test").
			StartSimple(func() {
				defer wg.Done()
				won, err := electors[idx].Campaign(h.ctx)
				if err == nil && won {
					winners.Add(1)
				}
			})
	}
	wg.Wait()

	if got := winners.Load(); got != 1 {
		t.Fatalf("expected exactly 1 leader election winner, got %d", got)
	}

	var activeLeaderCount int
	for _, e := range electors {
		if e.IsLeader() {
			activeLeaderCount++
			assertRole(t, e, RoleLeader)
		} else {
			assertRole(t, e, RoleFollower)
		}
	}
	if activeLeaderCount != 1 {
		t.Fatalf("expected exactly 1 active leader across cluster, got %d", activeLeaderCount)
	}
}

func TestLeaderElector_AutomaticFailoverWithBackgroundLoop(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := NewMemoryLeaderLeaseStore()
	ttl := 120 * time.Millisecond
	heartbeat := 30 * time.Millisecond

	cfg1 := LeaderElectorConfig{
		NodeID:            "failover-node-1",
		LeaseTTL:          ttl,
		HeartbeatInterval: heartbeat,
		Store:             store,
	}
	e1, err := NewLeaderElector(cfg1)
	if err != nil {
		t.Fatalf("failed to create e1: %v", err)
	}

	cfg2 := LeaderElectorConfig{
		NodeID:            "failover-node-2",
		LeaseTTL:          ttl,
		HeartbeatInterval: heartbeat,
		Store:             store,
	}
	e2, err := NewLeaderElector(cfg2)
	if err != nil {
		t.Fatalf("failed to create e2: %v", err)
	}

	// E1 campaigns first
	won1, err1 := e1.Campaign(ctx)
	if err1 != nil || !won1 {
		t.Fatalf("e1 campaign failed: won=%v, err=%v", won1, err1)
	}
	assertLeader(t, e1, true)

	// Start background failover loop on e2
	if startErr := e2.Start(ctx); startErr != nil {
		t.Fatalf("failed to start e2 loop: %v", startErr)
	}

	// Stop e1 (simulating crash/unavailability, no heartbeat sent)
	// Do not step down, just let lease expire!
	time.Sleep(ttl + 60*time.Millisecond)

	// E2 background loop should detect expiration and assume leadership
	deadline := time.Now().Add(1 * time.Second)
	var e2BecameLeader bool
	for time.Now().Before(deadline) {
		if e2.IsLeader() {
			e2BecameLeader = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if stopErr := e2.Stop(); stopErr != nil {
		t.Fatalf("failed to stop e2: %v", stopErr)
	}

	if !e2BecameLeader {
		t.Fatalf("e2 failed to assume leadership automatically via failover loop")
	}
	assertRole(t, e2, RoleLeader)
	assertLeader(t, e2, true)
}

func TestLeaderElector_DispatcherAndSubscriberIntegration(t *testing.T) {
	t.Parallel()
	dispatcher := NewMultiSubscriberDispatcher(nil)
	h := newElectorHarness(t, "node-events", 500*time.Millisecond, dispatcher)
	dispatcher.Register(h.elector)

	if h.elector.Name() != DefaultLeaderElectorSubscriberName {
		t.Fatalf("expected subscriber name %q, got %q", DefaultLeaderElectorSubscriberName, h.elector.Name())
	}

	var transitionsReceived []string
	var transitionMu sync.Mutex

	listener := NewFuncSubscriber("test_transition_listener", func(lCtx context.Context, entry *CallbackEntry) error {
		transitionMu.Lock()
		defer transitionMu.Unlock()
		if objects.GetString(entry.Payload, objects.FieldKeyCallbackType) == CallbackTypeLeaderElection {
			tr := objects.GetString(entry.Payload, FieldKeyTransition)
			transitionsReceived = append(transitionsReceived, tr)
		}
		return nil
	})
	dispatcher.Register(listener)

	// Campaign -> triggers "elected"
	won, err := h.elector.Campaign(h.ctx)
	if err != nil || !won {
		t.Fatalf("campaign failed: won=%v, err=%v", won, err)
	}

	// Renew -> triggers "renewed"
	if rErr := h.elector.Renew(h.ctx); rErr != nil {
		t.Fatalf("renew failed: %v", rErr)
	}

	// StepDown -> triggers "stepped_down"
	if sErr := h.elector.StepDown(h.ctx); sErr != nil {
		t.Fatalf("step down failed: %v", sErr)
	}

	transitionMu.Lock()
	defer transitionMu.Unlock()
	expectedOrder := []string{TransitionElected, TransitionRenewed, TransitionSteppedDown}
	if len(transitionsReceived) != len(expectedOrder) {
		t.Fatalf("expected %d transitions, got %d: %v", len(expectedOrder), len(transitionsReceived), transitionsReceived)
	}
	for i, exp := range expectedOrder {
		if transitionsReceived[i] != exp {
			t.Fatalf("transition[%d] = %q, expected %q", i, transitionsReceived[i], exp)
		}
	}
}

func TestLeaderElector_BoundaryAndErrorHandling(t *testing.T) {
	t.Parallel()

	// Empty node ID rejected
	_, err := NewLeaderElector(LeaderElectorConfig{NodeID: ""})
	if err != ErrInvalidNodeID {
		t.Fatalf("expected ErrInvalidNodeID, got %v", err)
	}

	h := newElectorHarness(t, "node-err", 500*time.Millisecond, nil)

	// Calling Renew when not leader returns ErrNotLeader
	if renewErr := h.elector.Renew(h.ctx); renewErr != ErrNotLeader {
		t.Fatalf("expected ErrNotLeader, got %v", renewErr)
	}

	// StepDown when not leader is safe no-op
	if stepErr := h.elector.StepDown(h.ctx); stepErr != nil {
		t.Fatalf("unexpected step down error: %v", stepErr)
	}

	// Context cancellation check
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, campErr := h.elector.Campaign(canceledCtx)
	if campErr == nil {
		t.Fatalf("expected error on canceled context campaign")
	}

	// Notify with nil entry is safe
	if notifyErr := h.elector.Notify(h.ctx, nil); notifyErr != nil {
		t.Fatalf("unexpected notify error on nil entry: %v", notifyErr)
	}
}
