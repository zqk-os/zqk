package callback

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

type conformanceHarness struct {
	t          *testing.T
	ring       *ConsistentHashingRing
	rebalancer *PartitionRebalancer
}

func newConformanceHarness(t *testing.T, vnodes int) *conformanceHarness {
	t.Helper()
	cfg := HashRingConfig{
		VnodeCount: vnodes,
	}
	r := NewConsistentHashingRing(cfg)
	require.NotNil(t, r)
	reb := NewPartitionRebalancer()
	require.NotNil(t, reb)
	return &conformanceHarness{
		t:          t,
		ring:       r,
		rebalancer: reb,
	}
}

func (h *conformanceHarness) addNodes(nodes ...string) {
	h.t.Helper()
	for _, n := range nodes {
		err := h.ring.AddNode(n)
		require.NoError(h.t, err)
	}
}

func TestHashingRing_EndToEndClusterExpansion(t *testing.T) {
	h := newConformanceHarness(t, 64)
	h.addNodes("worker-alpha", "worker-beta", "worker-gamma")

	oldRing := NewConsistentHashingRing(HashRingConfig{VnodeCount: 64})
	require.NotNil(t, oldRing)
	require.NoError(t, oldRing.AddNode("worker-alpha"))
	require.NoError(t, oldRing.AddNode("worker-beta"))
	require.NoError(t, oldRing.AddNode("worker-gamma"))

	newRing := NewConsistentHashingRing(HashRingConfig{VnodeCount: 64})
	require.NotNil(t, newRing)
	require.NoError(t, newRing.AddNode("worker-alpha"))
	require.NoError(t, newRing.AddNode("worker-beta"))
	require.NoError(t, newRing.AddNode("worker-gamma"))
	require.NoError(t, newRing.AddNode("worker-delta"))

	migrations, err := h.rebalancer.CalculateMigrations(oldRing, newRing)
	require.NoError(t, err)
	assert.NotEmpty(t, migrations)

	for _, m := range migrations {
		assert.Equal(t, "worker-delta", m.TargetNode)
		assert.Contains(t, []string{"worker-alpha", "worker-beta", "worker-gamma"}, m.SourceNode)
	}
}

func TestHashingRing_HandoffFailoverConformance(t *testing.T) {
	h := newConformanceHarness(t, 32)
	h.addNodes("node-1", "node-2", "node-3")

	handoffMap, err := h.rebalancer.PlanHandoff("node-2", h.ring)
	require.NoError(t, err)
	assert.Len(t, handoffMap, 32)

	for vnodeKey, replicas := range handoffMap {
		assert.NotEmpty(t, vnodeKey)
		assert.NotEmpty(t, replicas)
		assert.NotContains(t, replicas, "node-2")
	}
}

func TestHashingRing_ConcurrentReadWriteStress(t *testing.T) {
	h := newConformanceHarness(t, 16)
	h.addNodes("srv-1", "srv-2", "srv-3")

	var wg sync.WaitGroup
	workers := 8
	iterations := 50

	for i := 0; i < workers; i++ {
		workerID := i
		wg.Add(1)
		goroutinelabels.NewGoroutine(fmt.Sprintf("test_stress_worker_%d", workerID), "concurrent stress key lookup").StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := fmt.Sprintf("stress-key-%d-%d", workerID, j)
				node, err := h.ring.GetNode(key)
				assert.NoError(t, err)
				assert.NotEmpty(t, node)
			}
		})
	}
	wg.Wait()
}
