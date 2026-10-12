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
)

const (
	testIntegrationLeaseTTL          = 300 * time.Millisecond
	testIntegrationHeartbeatInterval = 50 * time.Millisecond
	testIntegrationNodeA             = "node-alpha"
	testIntegrationNodeB             = "node-bravo"
	testIntegrationNodeC             = "node-charlie"
	testConcurrentWorkerCount        = 25
	testConcurrentIterations         = 50
)

// TestLeaderElection_SwarmFailoverAndMonotonicFencing verifies multi-node campaign competition,
// lease expiration failover, voluntary step-down, and strict split-brain fencing token rejection.
func TestLeaderElection_SwarmFailoverAndMonotonicFencing(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var currentTimeMu sync.Mutex
	simulatedTime := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time {
		currentTimeMu.Lock()
		defer currentTimeMu.Unlock()
		return simulatedTime
	}
	advanceClock := func(d time.Duration) {
		currentTimeMu.Lock()
		defer currentTimeMu.Unlock()
		simulatedTime = simulatedTime.Add(d)
	}

	store := NewMemoryLeaderLeaseStore()
	dispatcher := NewMultiSubscriberDispatcher(nil)

	cfgA := LeaderElectorConfig{
		NodeID:            testIntegrationNodeA,
		LeaseTTL:          testIntegrationLeaseTTL,
		HeartbeatInterval: testIntegrationHeartbeatInterval,
		Clock:             clock,
		Store:             store,
		Dispatcher:        dispatcher,
	}
	electorA, err := NewLeaderElector(cfgA)
	require.NoError(t, err)

	cfgB := LeaderElectorConfig{
		NodeID:            testIntegrationNodeB,
		LeaseTTL:          testIntegrationLeaseTTL,
		HeartbeatInterval: testIntegrationHeartbeatInterval,
		Clock:             clock,
		Store:             store,
		Dispatcher:        dispatcher,
	}
	electorB, err := NewLeaderElector(cfgB)
	require.NoError(t, err)

	cfgC := LeaderElectorConfig{
		NodeID:            testIntegrationNodeC,
		LeaseTTL:          testIntegrationLeaseTTL,
		HeartbeatInterval: testIntegrationHeartbeatInterval,
		Clock:             clock,
		Store:             store,
		Dispatcher:        dispatcher,
	}
	electorC, err := NewLeaderElector(cfgC)
	require.NoError(t, err)

	dispatcher.Register(electorA)
	dispatcher.Register(electorB)
	dispatcher.Register(electorC)

	// Step 1: Node A campaigns and wins initial election
	wonA, err := electorA.Campaign(ctx)
	require.NoError(t, err)
	assert.True(t, wonA)
	assert.True(t, electorA.IsLeader())
	assert.Equal(t, RoleLeader, electorA.Role())
	tokenA := electorA.FencingToken()
	assert.Positive(t, tokenA)

	// Verify worker validation passes on tokenA
	assert.True(t, electorA.ValidateFencingToken(tokenA))

	// Step 2: Node B attempts campaign while Node A holds unexpired lease -> must fail
	wonB, err := electorB.Campaign(ctx)
	require.NoError(t, err)
	assert.False(t, wonB)
	assert.False(t, electorB.IsLeader())
	assert.Equal(t, RoleFollower, electorB.Role())

	// Step 3: Advance clock past Node A's lease expiration
	advanceClock(testIntegrationLeaseTTL + 10*time.Millisecond)

	// Node A's local check should now report not leader
	assert.False(t, electorA.IsLeader())

	// Step 4: Node B campaigns after expiration -> must win with higher monotonic token
	wonBAfterExp, err := electorB.Campaign(ctx)
	require.NoError(t, err)
	assert.True(t, wonBAfterExp)
	assert.True(t, electorB.IsLeader())
	tokenB := electorB.FencingToken()
	assert.Greater(t, tokenB, tokenA)

	// Step 5: Split-brain fencing guard: Old Leader A attempts to validate its stale tokenA -> must be rejected
	// Update electorA's internal highest validated threshold by simulating observing tokenB
	electorA.advanceHighestValidatedToken(tokenB)
	assert.False(t, electorA.ValidateFencingToken(tokenA), "stale token must be rejected by fencing guard")

	// Node B's valid token passes
	assert.True(t, electorB.ValidateFencingToken(tokenB))

	// Step 6: Node C attempts campaign while Node B is healthy -> must fail
	wonC, err := electorC.Campaign(ctx)
	require.NoError(t, err)
	assert.False(t, wonC)

	// Step 7: Node B voluntarily steps down
	err = electorB.StepDown(ctx)
	require.NoError(t, err)
	assert.False(t, electorB.IsLeader())

	// Step 8: Node C immediately campaigns and succeeds
	wonCAfterStepDown, err := electorC.Campaign(ctx)
	require.NoError(t, err)
	assert.True(t, wonCAfterStepDown)
	assert.True(t, electorC.IsLeader())
	tokenC := electorC.FencingToken()
	assert.Greater(t, tokenC, tokenB)
	assert.True(t, electorC.ValidateFencingToken(tokenC))
}

// TestLeaderElection_HighConcurrencyFencingStress tests high-throughput concurrent token validation,
// ensuring zero race conditions and strict monotonic token advancement under load.
func TestLeaderElection_HighConcurrencyFencingStress(t *testing.T) {
	t.Parallel()

	cfg := LeaderElectorConfig{
		NodeID:   testIntegrationNodeA,
		LeaseTTL: testIntegrationLeaseTTL,
		Store:    NewMemoryLeaderLeaseStore(),
	}
	elector, err := NewLeaderElector(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	won, err := elector.Campaign(ctx)
	require.NoError(t, err)
	require.True(t, won)

	// Case 1: High-concurrency validation of current leader token across 25 parallel workers
	activeToken := elector.FencingToken()
	var wg sync.WaitGroup
	var validationFailures atomic.Int64

	for w := 0; w < testConcurrentWorkerCount; w++ {
		wg.Add(1)
		workerID := w
		goroutinelabels.NewGoroutine(fmt.Sprintf("test_leader_worker_%d", workerID), "concurrent token validation stress").StartSimple(func() {
			defer wg.Done()
			for i := 0; i < testConcurrentIterations; i++ {
				if !elector.ValidateFencingToken(activeToken) {
					validationFailures.Add(1)
				}
			}
		})
	}

	wg.Wait()
	assert.Zero(t, validationFailures.Load(), "all concurrent workers with active leader token must validate successfully")

	// Case 2: Monotonic token sequence advancement
	var seqFailures int
	baseToken := activeToken + 10
	for i := int64(0); i < 50; i++ {
		if !elector.ValidateFencingToken(baseToken + i) {
			seqFailures++
		}
	}
	assert.Zero(t, seqFailures, "monotonically increasing tokens must always validate successfully")

	// Case 3: Verify stale and invalid tokens are rejected
	latestToken := baseToken + 49
	assert.False(t, elector.ValidateFencingToken(latestToken-1), "stale token below current highest must fail")
	assert.False(t, elector.ValidateFencingToken(activeToken), "original token must now fail as stale")
	assert.False(t, elector.ValidateFencingToken(0), "zero token must fail")
	assert.False(t, elector.ValidateFencingToken(-10), "negative token must fail")
}
