package community

import (
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func validDPGossipConfig(peerID, clusterID string) DifferentialPrivacyGossipConfig {
	return DifferentialPrivacyGossipConfig{
		LocalPeerID: peerID,
		ClusterID:   clusterID,
		Epsilon:     1.0,
		Delta:       1e-5,
		Sensitivity: 1.0,
		TotalBudget: 3.0,
	}
}

func makeCommittedEnvelope(
	t *testing.T,
	peerID string,
	clusterID string,
	epoch uint64,
	perturbed float64,
	samples uint64,
	eps float64,
) GossipPrivacyEnvelope {
	t.Helper()
	env := GossipPrivacyEnvelope{
		EnvelopeID:         clusterID + "-" + peerID,
		SenderPeerID:       peerID,
		ClusterID:          clusterID,
		Epoch:              epoch,
		PerturbedAggregate: perturbed,
		SampleCount:        samples,
		EpsilonSpent:       eps,
	}
	commitment, err := env.ComputeCommitment()
	require.NoError(t, err)
	env.Commitment = commitment
	return env
}

// TestDifferentialPrivacyGossip_StaticFloorInvariant verifies parameter boundaries and envelope schema invariants.
func TestDifferentialPrivacyGossip_StaticFloorInvariant(t *testing.T) {
	t.Parallel()

	engine, err := NewDifferentialPrivacyGossipEngine(validDPGossipConfig("peer-a", "cluster-dp-01"))
	require.NoError(t, err)
	require.NotNil(t, engine)
	assert.InDelta(t, 3.0, engine.RemainingBudget(), 1e-12)
	assert.InDelta(t, 0.0, engine.ConsumedBudget(), 1e-12)

	invalidConfigs := []struct {
		name string
		edit func(c *DifferentialPrivacyGossipConfig)
		want error
	}{
		{"blank local peer", func(c *DifferentialPrivacyGossipConfig) { c.LocalPeerID = "   " }, ErrBlankPeerID},
		{"blank cluster", func(c *DifferentialPrivacyGossipConfig) { c.ClusterID = "" }, ErrBlankClusterID},
		{"zero epsilon", func(c *DifferentialPrivacyGossipConfig) { c.Epsilon = 0 }, ErrInvalidPrivacyParameter},
		{"nan epsilon", func(c *DifferentialPrivacyGossipConfig) { c.Epsilon = math.NaN() }, ErrInvalidPrivacyParameter},
		{"inf sensitivity", func(c *DifferentialPrivacyGossipConfig) { c.Sensitivity = math.Inf(1) }, ErrInvalidPrivacyParameter},
		{"zero delta", func(c *DifferentialPrivacyGossipConfig) { c.Delta = 0 }, ErrInvalidPrivacyParameter},
		{"delta >= 1", func(c *DifferentialPrivacyGossipConfig) { c.Delta = 1.0 }, ErrInvalidPrivacyParameter},
		{"negative budget", func(c *DifferentialPrivacyGossipConfig) { c.TotalBudget = -1.0 }, ErrInvalidPrivacyParameter},
	}

	for _, tc := range invalidConfigs {
		cfg := validDPGossipConfig("peer-a", "cluster-dp-01")
		tc.edit(&cfg)
		_, cfgErr := NewDifferentialPrivacyGossipEngine(cfg)
		assert.ErrorIs(t, cfgErr, tc.want, tc.name)
	}

	env := makeCommittedEnvelope(t, "peer-b", "cluster-dp-01", 1, 42.5, 10, 0.5)
	assert.NoError(t, env.Validate())
}

// TestDifferentialPrivacyGossip_OperationalDynamicProof verifies Laplace noise perturbation,
// concurrent budget accounting, and homomorphic additive gossip aggregation across peers.
func TestDifferentialPrivacyGossip_OperationalDynamicProof(t *testing.T) {
	t.Parallel()

	clusterID := "cluster-mesh-dp"
	peerA, err := NewDifferentialPrivacyGossipEngine(validDPGossipConfig("peer-a", clusterID))
	require.NoError(t, err)
	peerB, err := NewDifferentialPrivacyGossipEngine(validDPGossipConfig("peer-b", clusterID))
	require.NoError(t, err)

	envA1, err := peerA.PerturbAndPublish(100.0, 20, 0.5)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), envA1.Epoch)
	assert.InDelta(t, 0.5, peerA.ConsumedBudget(), 1e-12)
	assert.InDelta(t, 2.5, peerA.RemainingBudget(), 1e-12)

	envB1, err := peerB.PerturbAndPublish(200.0, 30, 0.5)
	require.NoError(t, err)

	require.NoError(t, peerA.MergeEnvelope(envB1))
	require.NoError(t, peerB.MergeEnvelope(envA1))

	aggA := peerA.AggregateClusterState()
	aggB := peerB.AggregateClusterState()
	assert.Equal(t, clusterID, aggA.ClusterID)
	assert.Equal(t, 2, aggA.PeerCount)
	assert.Equal(t, uint64(50), aggA.TotalSampleCount)
	assert.InDelta(t, 1.0, aggA.TotalEpsilonSpent, 1e-12)
	assert.InDelta(t, envA1.PerturbedAggregate+envB1.PerturbedAggregate, aggA.TotalPerturbedSum, 1e-9)
	assert.InDelta(t, aggA.TotalPerturbedSum, aggB.TotalPerturbedSum, 1e-9)

	// Verify thread-safe concurrent budget consumption under race detector.
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("dp_gossip_worker", "concurrent perturb and publish").StartSimple(func() {
			defer wg.Done()
			_, pubErr := peerA.PerturbAndPublish(10.0, 5, 0.5)
			if pubErr != nil {
				assert.ErrorIs(t, pubErr, ErrPrivacyBudgetExhausted)
			}
		})
	}
	wg.Wait()
	assert.LessOrEqual(t, peerA.ConsumedBudget(), 3.0+1e-12)
}

// TestDifferentialPrivacyGossip_NegativeAdversarialBoundary verifies strict rejection of
// budget-exhausting queries, foreign-cluster envelopes, stale epochs, and tampered payloads.
func TestDifferentialPrivacyGossip_NegativeAdversarialBoundary(t *testing.T) {
	t.Parallel()

	clusterID := "cluster-secure-01"
	engine, err := NewDifferentialPrivacyGossipEngine(validDPGossipConfig("peer-local", clusterID))
	require.NoError(t, err)

	_, err = engine.PerturbAndPublish(50.0, 10, 2.0)
	require.NoError(t, err)

	// Exceeding remaining budget (1.0 left) must fail without mutating budget.
	_, err = engine.PerturbAndPublish(50.0, 10, 1.5)
	assert.ErrorIs(t, err, ErrPrivacyBudgetExhausted)
	assert.InDelta(t, 2.0, engine.ConsumedBudget(), 1e-12)
	assert.InDelta(t, 1.0, engine.RemainingBudget(), 1e-12)

	// Foreign cluster rejection.
	foreignEnv := makeCommittedEnvelope(t, "peer-ext", "cluster-foreign", 1, 10.0, 5, 0.5)
	assert.ErrorIs(t, engine.MergeEnvelope(foreignEnv), ErrClusterMismatch)

	// Stale epoch rejection.
	validEnv := makeCommittedEnvelope(t, "peer-remote", clusterID, 2, 25.0, 5, 0.5)
	require.NoError(t, engine.MergeEnvelope(validEnv))
	staleEnv := makeCommittedEnvelope(t, "peer-remote", clusterID, 2, 30.0, 5, 0.5)
	assert.ErrorIs(t, engine.MergeEnvelope(staleEnv), ErrStaleEpoch)

	// Tampered commitment and malformed field rejections.
	tampered := validEnv
	tampered.PerturbedAggregate += 999.0
	assert.ErrorIs(t, engine.MergeEnvelope(tampered), ErrMalformedGossipEnvelope)

	malformedCases := []GossipPrivacyEnvelope{
		makeCommittedEnvelope(t, "peer-x", clusterID, 0, 10.0, 5, 0.5),
		makeCommittedEnvelope(t, "peer-x", clusterID, 1, 10.0, 0, 0.5),
		makeCommittedEnvelope(t, "peer-x", clusterID, 1, math.NaN(), 5, 0.5),
		makeCommittedEnvelope(t, "peer-x", clusterID, 1, math.Inf(1), 5, 0.5),
		makeCommittedEnvelope(t, "peer-x", clusterID, 1, 10.0, 5, 0),
		makeCommittedEnvelope(t, "", clusterID, 1, 10.0, 5, 0.5),
	}
	for _, bad := range malformedCases {
		assert.ErrorIs(t, engine.MergeEnvelope(bad), ErrMalformedGossipEnvelope)
	}
}
