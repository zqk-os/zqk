// Package ambient provides integration verification for the autonomous swarm work-claim event loop.
package ambient

import (
	"context"
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
// Backlog Item: BLI-1793636167343282500-97d71f10 (Seat 1 - Static Floor Invariant)
// Requirement:  REQ-1793636166665066000-3cd039c4
// Criteria:     CRIT-1793636167343282000-cfcfdcca (Functional Acceptance: Static Floor Invariants)

func TestSwarmLoop_StaticFloorInvariants(t *testing.T) {
	t.Run("ConfigValidation", func(t *testing.T) {
		cfg := DefaultSwarmDispatcherConfig()
		assert.NoError(t, cfg.Validate())
		assert.True(t, cfg.AutoClaimEnabled)
		assert.Equal(t, DefaultSwarmMaxRetries, cfg.MaxRetries)
		assert.Equal(t, DefaultSwarmMaxClaimsPerSeat, cfg.MaxClaimsPerSeat)

		invalidRetries := cfg
		invalidRetries.MaxRetries = -1
		assert.ErrorIs(t, invalidRetries.Validate(), ErrInvalidMaxRetries)

		invalidClaims := cfg
		invalidClaims.MaxClaimsPerSeat = -1
		assert.ErrorIs(t, invalidClaims.Validate(), ErrInvalidClaimsPerSeat)
	})

	t.Run("EventEnumValidity", func(t *testing.T) {
		assert.True(t, TaskEventClaimed.IsValid())
		assert.True(t, TaskEventHeartbeat.IsValid())
		assert.True(t, TaskEventYielded.IsValid())
		assert.True(t, TaskEventCompleted.IsValid())
		assert.True(t, TaskEventRevoked.IsValid())
		assert.True(t, TaskEventQuarantined.IsValid())
		assert.False(t, SwarmTaskEventType("bogus").IsValid())
	})

	t.Run("NilReceiverSafety", func(t *testing.T) {
		var nilObs *BacklogObserver
		assert.Nil(t, nilObs.LoopState())
		assert.Nil(t, nilObs.EventStream())
		assert.False(t, nilObs.PublishEvent(SwarmTaskEvent{}))
		assert.False(t, nilObs.EnqueueTask("t-1"))
		assert.ErrorIs(t, nilObs.CompleteTask("t-1", "s-1"), ErrNilBacklogObserver)
		assert.ErrorIs(t, nilObs.YieldTask("t-1", "s-1", "err"), ErrNilBacklogObserver)
		assert.Equal(t, 0, nilObs.AutoClaimTick(context.Background()))
		quarantined, reason := nilObs.IsTaskQuarantined("t-1")
		assert.False(t, quarantined)
		assert.Empty(t, reason)
		assert.Equal(t, 0, nilObs.GetRetryCount("t-1"))

		var nilLoop *SwarmLoopState
		assert.Nil(t, nilLoop.EventStream())
		assert.False(t, nilLoop.PublishEvent(SwarmTaskEvent{}))
		assert.False(t, nilLoop.Enqueue("t-1"))
		_, ok := nilLoop.NextUnassigned()
		assert.False(t, ok)
		assert.Equal(t, 0, nilLoop.UnassignedCount())
		retries, isQ := nilLoop.RecordRetry("t-1", "reason", 3)
		assert.Equal(t, 0, retries)
		assert.False(t, isQ)
	})
}

// Traceability Verification:
// Backlog Item: BLI-1793636167343283000-3e077e56 (Seat 2 - Operational Dynamic Proof)
// Requirement:  REQ-1793636166665066000-3cd039c4
// Criteria:     CRIT-1793636167343283000-b05e419e (Operational Dynamic Proof: Auto-Claim, Yield & Completion Lifecycle)

func TestSwarmLoop_OperationalDynamicProof(t *testing.T) {
	arb := NewConsensusArbiter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, arb.Start(ctx))
	defer func() {
		_ = arb.Stop()
	}()

	cfg := DefaultSwarmDispatcherConfig()
	cfg.MaxClaimsPerSeat = 2
	obs, err := NewBacklogObserver(cfg, arb)
	require.NoError(t, err)

	require.NoError(t, obs.RegisterPeerSeat("seat-eng", "software-engineer"))
	require.NoError(t, obs.RegisterPeerSeat("seat-qa", "qa-sentinel"))

	assert.True(t, obs.EnqueueTask("task-1"))
	assert.True(t, obs.EnqueueTask("task-2"))
	assert.True(t, obs.EnqueueTask("task-3"))

	events := obs.EventStream()
	require.NotNil(t, events)

	claimed := obs.AutoClaimTick(ctx)
	assert.Equal(t, 3, claimed)
	assert.Equal(t, 3, obs.LeaseCount())

	// Verify task-1 was leased
	lease1, ok := obs.GetTaskLease("task-1")
	require.True(t, ok)
	require.NotEmpty(t, lease1.PeerSeatID)

	// Seat completes task-1
	err = obs.CompleteTask("task-1", lease1.PeerSeatID)
	assert.NoError(t, err)
	assert.Equal(t, 2, obs.LeaseCount())

	// Verify arbiter state is committed
	arbLease, found := arb.GetLease("task-1")
	require.True(t, found)
	assert.Equal(t, ConsensusStateCommitted, arbLease.State)

	// Seat yields task-2
	lease2, ok := obs.GetTaskLease("task-2")
	require.True(t, ok)
	err = obs.YieldTask("task-2", lease2.PeerSeatID, "simulated transient network failure")
	assert.NoError(t, err)
	assert.Equal(t, 1, obs.LeaseCount())
	assert.Equal(t, 1, obs.GetRetryCount("task-2"))

	// Next tick re-claims task-2
	reclaimed := obs.AutoClaimTick(ctx)
	assert.Equal(t, 1, reclaimed)
	assert.Equal(t, 2, obs.LeaseCount())
}

// Traceability Verification:
// Backlog Item: BLI-1793636167343283500-f9330a3b (Seat 3 - Negative Adversarial Boundary)
// Requirement:  REQ-1793636166665066000-3cd039c4
// Criteria:     CRIT-1793636167343284000-c75edcf9 (Negative Adversarial Boundary: Seat Mismatch, Quarantine Escalation & Concurrency Safety)

func TestSwarmLoop_NegativeAdversarialBoundaries(t *testing.T) {
	t.Run("SeatMismatchRejection", func(t *testing.T) {
		arb := NewConsensusArbiter()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		require.NoError(t, arb.Start(ctx))
		defer func() { _ = arb.Stop() }()

		cfg := DefaultSwarmDispatcherConfig()
		obs, err := NewBacklogObserver(cfg, arb)
		require.NoError(t, err)

		require.NoError(t, obs.RegisterPeerSeat("seat-owner", "engineer"))
		require.NoError(t, obs.RegisterPeerSeat("seat-rogue", "impostor"))

		_, err = obs.DispatchLease("task-exclusive", "seat-owner", 10*time.Second)
		require.NoError(t, err)

		// Rogue tries to complete
		err = obs.CompleteTask("task-exclusive", "seat-rogue")
		assert.ErrorIs(t, err, ErrSeatMismatch)

		// Rogue tries to yield
		err = obs.YieldTask("task-exclusive", "seat-rogue", "unauthorized yield")
		assert.ErrorIs(t, err, ErrSeatMismatch)

		// Non-existent task complete
		err = obs.CompleteTask("nonexistent-task", "seat-owner")
		assert.ErrorIs(t, err, ErrTaskNotLeased)
	})

	t.Run("QuarantineEscalationOnMaxRetries", func(t *testing.T) {
		arb := NewConsensusArbiter()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		require.NoError(t, arb.Start(ctx))
		defer func() { _ = arb.Stop() }()

		cfg := DefaultSwarmDispatcherConfig()
		cfg.MaxRetries = 2
		obs, err := NewBacklogObserver(cfg, arb)
		require.NoError(t, err)

		require.NoError(t, obs.RegisterPeerSeat("seat-worker", "engineer"))
		assert.True(t, obs.EnqueueTask("task-fragile"))

		for i := 1; i <= 2; i++ {
			assert.Equal(t, 1, obs.AutoClaimTick(ctx))
			err = obs.YieldTask("task-fragile", "seat-worker", "retry failure")
			assert.NoError(t, err)
			assert.Equal(t, i, obs.GetRetryCount("task-fragile"))
			quarantined, _ := obs.IsTaskQuarantined("task-fragile")
			assert.False(t, quarantined)
		}

		// 3rd yield exceeds maxRetries (2)
		assert.Equal(t, 1, obs.AutoClaimTick(ctx))
		err = obs.YieldTask("task-fragile", "seat-worker", "fatal crash limit reached")
		assert.NoError(t, err)
		assert.Equal(t, 3, obs.GetRetryCount("task-fragile"))

		quarantined, reason := obs.IsTaskQuarantined("task-fragile")
		assert.True(t, quarantined)
		assert.Equal(t, "fatal crash limit reached", reason)

		// AutoClaimTick should NOT re-claim quarantined task
		assert.Equal(t, 0, obs.AutoClaimTick(ctx))
	})

	t.Run("ConcurrentSwarmExecutionRaceFree", func(t *testing.T) {
		arb := NewConsensusArbiter()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		require.NoError(t, arb.Start(ctx))
		defer func() { _ = arb.Stop() }()

		cfg := DefaultSwarmDispatcherConfig()
		cfg.MaxClaimsPerSeat = 4
		obs, err := NewBacklogObserver(cfg, arb)
		require.NoError(t, err)

		for i := 0; i < 5; i++ {
			seatID := "seat-" + string(rune('a'+i))
			require.NoError(t, obs.RegisterPeerSeat(seatID, "worker"))
		}

		const numTasks = 40
		for i := 0; i < numTasks; i++ {
			obs.EnqueueTask("concurrent-task-" + string(rune('0'+(i%10))) + "-" + string(rune('A'+(i/10))))
		}

		var completedOps int32
		var wg sync.WaitGroup

		for w := 0; w < 10; w++ {
			goroutinelabels.NewGoroutine("ambient.test.swarm_loop", "concurrent worker in swarm loop test").
				WithWaitGroup(&wg).
				StartSimple(func() {
					for i := 0; i < 5; i++ {
						obs.AutoClaimTick(ctx)
						leases := obs.ActiveLeases()
						for _, l := range leases {
							if l != nil {
								if err := obs.CompleteTask(l.TaskID, l.PeerSeatID); err == nil {
									atomic.AddInt32(&completedOps, 1)
								}
							}
						}
						time.Sleep(2 * time.Millisecond)
					}
				})
		}
		wg.Wait()
		assert.GreaterOrEqual(t, atomic.LoadInt32(&completedOps), int32(1))
	})

	t.Run("EngineSupervisorTickIntegration", func(t *testing.T) {
		engCfg := DefaultAmbientEngineConfig()
		engCfg.TickInterval = 15 * time.Millisecond
		eng, err := NewAmbientEngine(engCfg)
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		require.NoError(t, eng.Start(ctx))
		defer func() { _ = eng.Stop() }()

		seat := PeerSeat{SeatID: "seat-eng-loop", Role: "engineer", Healthy: true, LastSeen: time.Now()}
		require.NoError(t, eng.RegisterPeerSeat(seat))

		assert.True(t, eng.EnqueueTask("eng-task-1"))
		assert.True(t, eng.EnqueueTask("eng-task-2"))

		// Wait for supervisor ticks to auto-claim
		time.Sleep(60 * time.Millisecond)

		l1, ok1 := eng.GetActiveLease("eng-task-1")
		l2, ok2 := eng.GetActiveLease("eng-task-2")

		if ok1 {
			assert.NoError(t, eng.CompleteTask("eng-task-1", l1.PeerSeatID))
		}
		if ok2 {
			assert.NoError(t, eng.YieldTask("eng-task-2", l2.PeerSeatID, "engine yield"))
		}
	})
}

// TestSwarmLoop_ASTAudit enforces AST compliance (function length, zero naked goroutines, zero swallowed errors).
func TestSwarmLoop_ASTAudit(t *testing.T) {
	auditor := qa.NewASTAuditor()
	targetFiles := []string{
		"swarm_loop.go",
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
