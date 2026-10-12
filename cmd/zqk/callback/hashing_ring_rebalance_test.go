package callback

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

type rebalanceTestHarness struct {
	t          *testing.T
	rebalancer *PartitionRebalancer
}

func newRebalanceHarness(t *testing.T) *rebalanceTestHarness {
	t.Helper()
	return &rebalanceTestHarness{
		t:          t,
		rebalancer: NewPartitionRebalancer(),
	}
}

func (h *rebalanceTestHarness) createRing(vnodes int, nodes ...string) *ConsistentHashingRing {
	h.t.Helper()
	cfg := HashRingConfig{
		VnodeCount: vnodes,
	}
	r := NewConsistentHashingRing(cfg)
	require.NotNil(h.t, r)
	for i := 0; i < len(nodes); i++ {
		node := nodes[i]
		err := r.AddNode(node)
		require.NoError(h.t, err)
	}
	return r
}

func (h *rebalanceTestHarness) assertAddedNodeMigrations(migrations []PartitionMigration, addedNode string, expectedCount int) {
	h.t.Helper()
	require.Len(h.t, migrations, expectedCount)
	for i := 0; i < len(migrations); i++ {
		m := migrations[i]
		assert.Equal(h.t, addedNode, m.TargetNode)
		assert.NotEqual(h.t, addedNode, m.SourceNode)
		assert.False(h.t, m.Timestamp.IsZero())
	}
}

func (h *rebalanceTestHarness) assertRemovedNodeMigrations(migrations []PartitionMigration, removedNode string, expectedCount int, remaining []string) {
	h.t.Helper()
	require.Len(h.t, migrations, expectedCount)
	for i := 0; i < len(migrations); i++ {
		m := migrations[i]
		assert.Equal(h.t, removedNode, m.SourceNode)
		assert.NotEqual(h.t, removedNode, m.TargetNode)
		assert.Contains(h.t, remaining, m.TargetNode)
		assert.False(h.t, m.Timestamp.IsZero())
	}
}

func (h *rebalanceTestHarness) assertHandoffPlan(plan map[string][]string, failedNode string, expectedVnodes int, expectedHealthy []string) {
	h.t.Helper()
	require.Len(h.t, plan, expectedVnodes)
	for key, replicas := range plan {
		assert.Contains(h.t, key, failedNode+"#vn")
		assert.NotEmpty(h.t, replicas)
		assert.NotContains(h.t, replicas, failedNode)
		for j := 0; j < len(replicas); j++ {
			rep := replicas[j]
			assert.Contains(h.t, expectedHealthy, rep)
		}
	}
}

func (h *rebalanceTestHarness) setupMigrationTest(newNodes []string) (*ConsistentHashingRing, []PartitionMigration, int) {
	h.t.Helper()
	vnodes := 128
	oldNodes := []string{"node-0", "node-1", "node-2", "node-3"}
	oldRing := h.createRing(vnodes, oldNodes...)
	newRing := h.createRing(vnodes, newNodes...)
	migrations, err := h.rebalancer.CalculateMigrations(oldRing, newRing)
	require.NoError(h.t, err)
	return newRing, migrations, vnodes
}

func TestPartitionRebalancer_NodeAdditionMigration(t *testing.T) {
	h := newRebalanceHarness(t)
	newNodes := []string{"node-0", "node-1", "node-2", "node-3", "node-4"}
	newRing, migrations, vnodes := h.setupMigrationTest(newNodes)

	h.assertAddedNodeMigrations(migrations, "node-4", vnodes)

	totalPartitions := len(newRing.ring)
	fractionMoved := float64(len(migrations)) / float64(totalPartitions)
	expectedFraction := 1.0 / float64(len(newNodes))
	assert.InDelta(t, expectedFraction, fractionMoved, 0.001)
}

func TestPartitionRebalancer_NodeRemovalMigration(t *testing.T) {
	h := newRebalanceHarness(t)
	remainingNodes := []string{"node-0", "node-2", "node-3"}
	_, migrations, vnodes := h.setupMigrationTest(remainingNodes)

	h.assertRemovedNodeMigrations(migrations, "node-1", vnodes, remainingNodes)
}

func TestPartitionRebalancer_PlanHandoff(t *testing.T) {
	h := newRebalanceHarness(t)
	vnodes := 64
	nodes := []string{"node-alpha", "node-beta", "node-gamma"}
	ring := h.createRing(vnodes, nodes...)

	plan, err := h.rebalancer.PlanHandoff("node-beta", ring)
	require.NoError(t, err)

	expectedHealthy := []string{"node-alpha", "node-gamma"}
	h.assertHandoffPlan(plan, "node-beta", vnodes, expectedHealthy)
}

func TestPartitionRebalancer_PlanHandoff_SingleNode(t *testing.T) {
	h := newRebalanceHarness(t)
	vnodes := 64
	ring := h.createRing(vnodes, "solo-node")

	plan, err := h.rebalancer.PlanHandoff("solo-node", ring)
	require.NoError(t, err)
	require.Len(t, plan, vnodes)

	for key, replicas := range plan {
		assert.Contains(t, key, "solo-node#vn")
		assert.Empty(t, replicas)
	}
}

func TestPartitionRebalancer_EquivalentRings(t *testing.T) {
	h := newRebalanceHarness(t)
	vnodes := 64
	nodes := []string{"node-0", "node-1"}

	ringA := h.createRing(vnodes, nodes...)
	ringB := h.createRing(vnodes, nodes...)

	sameInstanceMigrations, err := h.rebalancer.CalculateMigrations(ringA, ringA)
	require.NoError(t, err)
	assert.Empty(t, sameInstanceMigrations)

	identicalMigrations, err := h.rebalancer.CalculateMigrations(ringA, ringB)
	require.NoError(t, err)
	assert.Empty(t, identicalMigrations)
}

func TestPartitionRebalancer_ErrorCases(t *testing.T) {
	h := newRebalanceHarness(t)
	emptyRing := h.createRing(64)
	validRing := h.createRing(64, "node-0", "node-1")

	migrations, err := h.rebalancer.CalculateMigrations(nil, validRing)
	assert.ErrorIs(t, err, ErrNilRing)
	assert.Nil(t, migrations)

	migrations, err = h.rebalancer.CalculateMigrations(validRing, nil)
	assert.ErrorIs(t, err, ErrNilRing)
	assert.Nil(t, migrations)

	migrations, err = h.rebalancer.CalculateMigrations(emptyRing, validRing)
	assert.ErrorIs(t, err, ErrEmptyRing)
	assert.Nil(t, migrations)

	migrations, err = h.rebalancer.CalculateMigrations(validRing, emptyRing)
	assert.ErrorIs(t, err, ErrEmptyRing)
	assert.Nil(t, migrations)

	plan, err := h.rebalancer.PlanHandoff("node-0", nil)
	assert.ErrorIs(t, err, ErrNilRing)
	assert.Nil(t, plan)

	plan, err = h.rebalancer.PlanHandoff("node-0", emptyRing)
	assert.ErrorIs(t, err, ErrEmptyRing)
	assert.Nil(t, plan)

	plan, err = h.rebalancer.PlanHandoff("unknown-node", validRing)
	assert.ErrorIs(t, err, ErrNodeNotFound)
	assert.Nil(t, plan)

	plan, err = h.rebalancer.PlanHandoff("", validRing)
	assert.ErrorIs(t, err, ErrNodeNotFound)
	assert.Nil(t, plan)
}

func TestPartitionRebalancer_ConcurrentCalculationRace(t *testing.T) {
	h := newRebalanceHarness(t)
	oldRing := h.createRing(64, "worker-0", "worker-1", "worker-2")
	newRing := h.createRing(64, "worker-0", "worker-1", "worker-2", "worker-3")

	var wg sync.WaitGroup
	workers := 8
	iterations := 50

	for w := 0; w < workers; w++ {
		wg.Add(1)
		workerID := w
		goroutinelabels.NewGoroutine(fmt.Sprintf("rebalance_worker_%d", workerID), "concurrent migration calculate").StartSimple(func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				migs, migErr := h.rebalancer.CalculateMigrations(oldRing, newRing)
				assert.NoError(t, migErr)
				assert.NotEmpty(t, migs)

				plan, planErr := h.rebalancer.PlanHandoff("worker-1", oldRing)
				assert.NoError(t, planErr)
				assert.NotEmpty(t, plan)
			}
		})
	}

	wg.Wait()
}
