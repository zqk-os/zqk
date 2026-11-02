package ambient

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
	qa "github.com/zqk-os/zqk/pkg/validation/qa"
)

// Traceability Verification:
// Backlog Item: BLI-1793631698944513500-a8b8611e (Seat 1 - Static Floor)
// Requirement:  REQ-1793631698303335000-edb41ed9
// Criteria:     CRIT-1793631698944513000-a9f64b1d (Static Floor: AmbientEngine Task Leasing & Backlog Observer Invariants)

func TestSwarmDispatch_StaticFloorInvariants(t *testing.T) {
	t.Run("SwarmDispatcherConfigValidation", func(t *testing.T) {
		cfg := DefaultSwarmDispatcherConfig()
		require.NoError(t, cfg.Validate())

		invalidInterval := cfg
		invalidInterval.CheckInterval = 0
		assert.ErrorIs(t, invalidInterval.Validate(), ErrInvalidCheckInterval)

		invalidDuration := cfg
		invalidDuration.LeaseDuration = -1 * time.Second
		assert.ErrorIs(t, invalidDuration.Validate(), ErrInvalidLeaseDuration)

		invalidCapacity := cfg
		invalidCapacity.MaxConcurrentLeases = 0
		assert.ErrorIs(t, invalidCapacity.Validate(), ErrInvalidMaxConcurrentLeases)
	})

	t.Run("SwarmTaskLeaseValidation", func(t *testing.T) {
		var nilLease *SwarmTaskLease
		assert.ErrorIs(t, nilLease.Validate(), ErrNilSwarmTaskLease)
		assert.True(t, nilLease.IsExpired())
		assert.Nil(t, nilLease.Clone())

		validLease := &SwarmTaskLease{
			ID:            "lease-1",
			TaskID:        "BLI-test-1",
			PeerSeatID:    "peer-1",
			LeaseDuration: 10 * time.Second,
			ExpiresAt:     time.Now().Add(10 * time.Second),
			State:         ConsensusStateLeased,
		}
		require.NoError(t, validLease.Validate())
		assert.False(t, validLease.IsExpired())

		cloned := validLease.Clone()
		require.NotNil(t, cloned)
		assert.Equal(t, validLease.ID, cloned.ID)
		assert.Equal(t, validLease.TaskID, cloned.TaskID)
	})

	t.Run("NilObserverSafety", func(t *testing.T) {
		var nilObserver *BacklogObserver
		assert.ErrorIs(t, nilObserver.Start(context.Background()), ErrNilBacklogObserver)
		assert.ErrorIs(t, nilObserver.Stop(), ErrNilBacklogObserver)
		assert.Nil(t, nilObserver.Arbiter())
		assert.Nil(t, nilObserver.LeaseChannel())
		assert.False(t, nilObserver.IsRunning())
		assert.Equal(t, 0, nilObserver.LeaseCount())
		assert.Nil(t, nilObserver.ActiveLeases())
		assert.Nil(t, nilObserver.HealthySeats())
		assert.False(t, nilObserver.IsSeatHealthy("any"))
		assert.Equal(t, 0, nilObserver.EvictExpiredLeases())
	})

	t.Run("EngineBindingInvariants", func(t *testing.T) {
		engCfg := DefaultAmbientEngineConfig()
		engine, err := NewAmbientEngine(engCfg)
		require.NoError(t, err)
		require.NotNil(t, engine)

		observer := engine.BacklogObserver()
		require.NotNil(t, observer)
		assert.NotNil(t, observer.Arbiter())
		assert.NotNil(t, observer.LeaseChannel())
		assert.Equal(t, observer, engine.SwarmDispatcher())

		var nilEngine *AmbientEngine
		assert.Nil(t, nilEngine.BacklogObserver())
		assert.Nil(t, nilEngine.SwarmDispatcher())
	})
}

// Traceability Verification:
// Backlog Item: BLI-1793631698944514000-d403d587 (Seat 2 - Operational Dynamic Proof)
// Requirement:  REQ-1793631698303335000-edb41ed9
// Criteria:     CRIT-1793631698944514000-5c3e7f33 (Operational Dynamic Proof: Dynamic Peer Seat Lease Allocation & Verification)

func setupTestSwarmEngine(t *testing.T) (*AmbientEngine, context.CancelFunc) {
	t.Helper()
	cfg := DefaultAmbientEngineConfig()
	cfg.TickInterval = 20 * time.Millisecond
	cfg.GracefulStopTimeout = 2 * time.Second
	cfg.SwarmDispatchConfig.CheckInterval = 20 * time.Millisecond

	engine, err := NewAmbientEngine(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	err = engine.Start(ctx)
	require.NoError(t, err)
	return engine, cancel
}

func TestSwarmDispatch_OperationalDynamicProof(t *testing.T) {
	engine, cancel := setupTestSwarmEngine(t)
	defer func() {
		cancel()
		if stopErr := engine.Stop(); stopErr != nil {
			// Engine may already be stopped
		}
	}()

	observer := engine.BacklogObserver()
	require.NotNil(t, observer)
	require.True(t, observer.IsRunning())

	t.Run("PeerSeatRegistrationAndHeartbeat", func(t *testing.T) {
		err := observer.RegisterPeerSeat("seat-alpha", "worker")
		require.NoError(t, err)
		err = observer.RegisterPeerSeat("seat-beta", "worker")
		require.NoError(t, err)

		healthy := observer.HealthySeats()
		assert.Len(t, healthy, 2)
		assert.True(t, observer.IsSeatHealthy("seat-alpha"))
		assert.True(t, observer.IsSeatHealthy("seat-beta"))

		err = observer.RecordHeartbeat("seat-alpha")
		require.NoError(t, err)
		assert.ErrorIs(t, observer.RecordHeartbeat("seat-nonexistent"), ErrSeatNotFound)
	})

	t.Run("DynamicTaskLeasingAndChannelDelivery", func(t *testing.T) {
		taskIDs := []string{"BLI-cycle28-001", "BLI-cycle28-002"}
		leases, err := observer.DetectAndLeaseTasks(taskIDs)
		require.NoError(t, err)
		require.Len(t, leases, 2)

		assert.Equal(t, 2, observer.LeaseCount())
		l1, found := observer.GetLease("BLI-cycle28-001")
		require.True(t, found)
		assert.Equal(t, ConsensusStateLeased, l1.State)

		// Verify publication to channel
		select {
		case chLease := <-observer.LeaseChannel():
			assert.NotEmpty(t, chLease.TaskID)
			assert.NotEmpty(t, chLease.PeerSeatID)
		case <-time.After(500 * time.Millisecond):
			t.Fatal("expected lease event on LeaseChannel, timed out")
		}
	})

	t.Run("RevocationLifecycle", func(t *testing.T) {
		err := observer.RevokeLease("BLI-cycle28-001", "test release")
		require.NoError(t, err)
		_, found := observer.GetLease("BLI-cycle28-001")
		assert.False(t, found)
	})

	t.Run("DispatchWorkClaimAndEngineDelegation", func(t *testing.T) {
		// Test DispatchWorkClaim directly on observer
		claimLease, err := observer.DispatchWorkClaim("seat-beta", "BLI-cycle28-003")
		require.NoError(t, err)
		require.NotNil(t, claimLease)
		assert.Equal(t, "BLI-cycle28-003", claimLease.TaskID)
		assert.Equal(t, "seat-beta", claimLease.PeerSeatID)
		assert.Equal(t, ConsensusStateLeased, claimLease.State)

		// Test engine surface delegation for DispatchWorkClaim
		engClaim, err := engine.DispatchWorkClaim("seat-alpha", "BLI-cycle28-004")
		require.NoError(t, err)
		require.NotNil(t, engClaim)
		assert.Equal(t, "BLI-cycle28-004", engClaim.TaskID)
		assert.Equal(t, "seat-alpha", engClaim.PeerSeatID)

		// Verify active lease retrieval from engine
		activeLease, ok := engine.GetActiveLease("BLI-cycle28-004")
		require.True(t, ok)
		assert.Equal(t, ConsensusStateLeased, activeLease.State)
		assert.Equal(t, "seat-alpha", activeLease.PeerSeatID)

		// Verify consensus arbiter active tracking
		arb := engine.ConsensusArbiter()
		require.NotNil(t, arb)
		arbLease, err := arb.LookupLease("BLI-cycle28-004")
		require.NoError(t, err)
		assert.Equal(t, ConsensusStateLeased, arbLease.State)
		assert.Equal(t, "seat-alpha", arbLease.WorkerID)
	})
}

// Traceability Verification:
// Backlog Item: BLI-1793631698944514500-86462a9f (Seat 3 - Negative Adversarial Boundary)
// Requirement:  REQ-1793631698303335000-edb41ed9
// Criteria:     CRIT-1793631698944515000-e6177bf0 (Negative Adversarial Boundary: Expired Lease Eviction & Concurrent Claim Race Safety)

func TestSwarmDispatch_NegativeAdversarialBoundaries(t *testing.T) {
	t.Run("ExpiredLeaseEviction", func(t *testing.T) {
		arb := NewConsensusArbiter(WithDefaultLeaseTTL(30 * time.Millisecond))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		require.NoError(t, arb.Start(ctx))
		defer func() {
			if stopErr := arb.Stop(); stopErr != nil {
				// Arbiter may already be stopped
			}
		}()

		cfg := DefaultSwarmDispatcherConfig()
		cfg.LeaseDuration = 20 * time.Millisecond
		obs, err := NewBacklogObserver(cfg, arb)
		require.NoError(t, err)

		require.NoError(t, obs.RegisterPeerSeat("seat-1", "worker"))
		lease, err := obs.DispatchLease("task-exp-1", "seat-1", 20*time.Millisecond)
		require.NoError(t, err)
		require.NotNil(t, lease)
		assert.Equal(t, 1, obs.LeaseCount())

		// Wait for expiration
		time.Sleep(50 * time.Millisecond)

		evicted := obs.EvictExpiredLeases()
		assert.GreaterOrEqual(t, evicted, 1)
		assert.Equal(t, 0, obs.LeaseCount())
	})

	t.Run("ConcurrentDoubleClaimsFailClosed", func(t *testing.T) {
		arb := NewConsensusArbiter()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		require.NoError(t, arb.Start(ctx))
		defer func() {
			if stopErr := arb.Stop(); stopErr != nil {
				// Arbiter may already be stopped
			}
		}()

		cfg := DefaultSwarmDispatcherConfig()
		obs, err := NewBacklogObserver(cfg, arb)
		require.NoError(t, err)

		const workerCount = 60
		const targetTask = "task-race-exclusive-1"

		var successes int32
		var failures int32
		var wg sync.WaitGroup
		wg.Add(workerCount)

		for i := 0; i < workerCount; i++ {
			workerID := fmt.Sprintf("worker-%03d", i)
			goroutinelabels.NewGoroutine("test.swarm.claim", "concurrent claim attempt").
				StartSimple(func() {
					defer wg.Done()
					l, claimErr := obs.DispatchLease(targetTask, workerID, 5*time.Second)
					if claimErr == nil && l != nil {
						atomic.AddInt32(&successes, 1)
					} else {
						atomic.AddInt32(&failures, 1)
					}
				})
		}
		wg.Wait()

		assert.Equal(t, int32(1), successes, "exactly one worker must acquire the exclusive lease")
		assert.Equal(t, int32(workerCount-1), failures, "all competing claims must fail closed")
	})

	t.Run("EngineGracefulStopIdempotence", func(t *testing.T) {
		engine, cancel := setupTestSwarmEngine(t)
		cancel()

		err := engine.Stop()
		require.NoError(t, err)
		assert.Equal(t, EngineStateStopped, engine.State())

		// Second stop should be safe and return ErrEngineStopped
		err = engine.Stop()
		assert.ErrorIs(t, err, ErrEngineStopped)
	})
}

// TestSwarmDispatch_ASTAudit enforces zero naked goroutines, function line length bounds,
// and zero blank identifier swallowed errors across swarm dispatch source files.
func TestSwarmDispatch_ASTAudit(t *testing.T) {
	auditor := qa.NewASTAuditor()
	targetFiles := []string{
		"swarm_dispatch.go",
		"engine.go",
		"engine_lifecycle.go",
	}

	for _, file := range targetFiles {
		violations, err := auditor.AuditFile(file)
		require.NoError(t, err, "failed to audit "+file)
		for _, v := range violations {
			if v.Severity == "high" || v.Severity == "medium" || v.Severity == "error" {
				t.Errorf("AST violation [%s] in %s at %s: %s", v.Severity, file, v.Pos, v.Message)
			}
		}
	}
}
