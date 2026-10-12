package callback

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

type ringTestHarness struct {
	t    *testing.T
	ring *ConsistentHashingRing
}

func newRingHarness(t *testing.T, vnodes int) *ringTestHarness {
	t.Helper()
	cfg := HashRingConfig{
		VnodeCount: vnodes,
	}
	r := NewConsistentHashingRing(cfg)
	require.NotNil(t, r)
	return &ringTestHarness{
		t:    t,
		ring: r,
	}
}

func (h *ringTestHarness) addNodes(nodes ...string) {
	h.t.Helper()
	for _, n := range nodes {
		err := h.ring.AddNode(n)
		require.NoError(h.t, err)
	}
}

func (h *ringTestHarness) mustGetNode(key string) string {
	h.t.Helper()
	node, err := h.ring.GetNode(key)
	require.NoError(h.t, err)
	require.NotEmpty(h.t, node)
	return node
}

func (h *ringTestHarness) mustGetNodes(key string, count int) []string {
	h.t.Helper()
	nodes, err := h.ring.GetNodes(key, count)
	require.NoError(h.t, err)
	return nodes
}

func TestConsistentHashRing_EmptyRing(t *testing.T) {
	h := newRingHarness(t, 128)

	assert.Equal(t, 0, h.ring.Size())
	assert.Empty(t, h.ring.Members())

	node, err := h.ring.GetNode("event-key-1")
	assert.ErrorIs(t, err, ErrEmptyRing)
	assert.Empty(t, node)

	nodes, err := h.ring.GetNodes("event-key-1", 3)
	assert.ErrorIs(t, err, ErrEmptyRing)
	assert.Nil(t, nodes)
}

func TestConsistentHashRing_InvalidInputs(t *testing.T) {
	h := newRingHarness(t, 128)

	err := h.ring.AddNode("")
	assert.ErrorIs(t, err, ErrInvalidNodeID)

	err = h.ring.RemoveNode("")
	assert.ErrorIs(t, err, ErrInvalidNodeID)

	h.addNodes("worker-pod-1")

	err = h.ring.AddNode("worker-pod-1")
	assert.ErrorIs(t, err, ErrNodeExists)

	err = h.ring.RemoveNode("non-existent-pod")
	assert.ErrorIs(t, err, ErrNodeNotFound)

	nodes, err := h.ring.GetNodes("event-key-1", 0)
	assert.ErrorIs(t, err, ErrInvalidCount)
	assert.Nil(t, nodes)

	nodes, err = h.ring.GetNodes("event-key-1", -5)
	assert.ErrorIs(t, err, ErrInvalidCount)
	assert.Nil(t, nodes)
}

func TestConsistentHashRing_AddRemoveMembers(t *testing.T) {
	h := newRingHarness(t, 64)

	podA := "worker-pod-a"
	podB := "worker-pod-b"
	podC := "worker-pod-c"

	h.addNodes(podA, podB, podC)

	assert.Equal(t, 3, h.ring.Size())
	expectedMembers := []string{podA, podB, podC}
	sort.Strings(expectedMembers)
	assert.Equal(t, expectedMembers, h.ring.Members())

	err := h.ring.RemoveNode(podB)
	assert.NoError(t, err)

	assert.Equal(t, 2, h.ring.Size())
	expectedRemaining := []string{podA, podC}
	sort.Strings(expectedRemaining)
	assert.Equal(t, expectedRemaining, h.ring.Members())
}

func TestConsistentHashRing_DeterministicRouting(t *testing.T) {
	h := newRingHarness(t, 128)
	h.addNodes("node-alpha", "node-beta", "node-gamma")

	routes := make(map[string]string)
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("partition-key-%d", i)
		routes[key] = h.mustGetNode(key)
	}

	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("partition-key-%d", i)
		assert.Equal(t, routes[key], h.mustGetNode(key))
	}
}

func TestConsistentHashRing_UniformDistribution(t *testing.T) {
	numNodes := 5
	vnodesPerNode := 256
	h := newRingHarness(t, vnodesPerNode)

	for i := 0; i < numNodes; i++ {
		h.addNodes(fmt.Sprintf("node-%d", i))
	}

	totalKeys := 20000
	counts := make(map[string]int)
	for i := 0; i < totalKeys; i++ {
		key := "dist-metric-key-" + strconv.Itoa(i)
		node := h.mustGetNode(key)
		counts[node]++
	}

	expectedPerNode := float64(totalKeys) / float64(numNodes)
	var sumSquares float64
	for _, count := range counts {
		diff := float64(count) - expectedPerNode
		sumSquares += diff * diff
	}
	stdDev := math.Sqrt(sumSquares / float64(numNodes))
	cv := stdDev / expectedPerNode

	assert.Less(t, cv, 0.15)
	for _, count := range counts {
		assert.Greater(t, count, int(expectedPerNode*0.75))
		assert.Less(t, count, int(expectedPerNode*1.25))
	}
}

func TestConsistentHashRing_ReplicaSelection(t *testing.T) {
	h := newRingHarness(t, 128)
	nodeList := []string{"node-1", "node-2", "node-3", "node-4", "node-5"}
	h.addNodes(nodeList...)

	testKey := "transaction-partition-42"
	replicas := h.mustGetNodes(testKey, 3)
	assert.Len(t, replicas, 3)

	replicaSet := make(map[string]bool)
	for _, rep := range replicas {
		assert.False(t, replicaSet[rep], "expected distinct physical replica node")
		replicaSet[rep] = true
	}

	primary := h.mustGetNode(testKey)
	assert.Equal(t, primary, replicas[0])

	allReplicas := h.mustGetNodes(testKey, 10)
	assert.Len(t, allReplicas, 5)
}

func TestConsistentHashRing_NodeRemovalAndReassignment(t *testing.T) {
	h := newRingHarness(t, 128)
	nodes := []string{"cluster-a", "cluster-b", "cluster-c"}
	h.addNodes(nodes...)

	testKeys := make([]string, 500)
	for i := 0; i < len(testKeys); i++ {
		testKeys[i] = fmt.Sprintf("reassign-key-%d", i)
	}

	before := make(map[string]string)
	for _, k := range testKeys {
		before[k] = h.mustGetNode(k)
	}

	err := h.ring.RemoveNode("cluster-b")
	assert.NoError(t, err)

	for _, k := range testKeys {
		current := h.mustGetNode(k)
		assert.NotEqual(t, "cluster-b", current)
		if before[k] != "cluster-b" {
			assert.Equal(t, before[k], current)
		}
	}
}

func TestConsistentHashRing_ConcurrentReadWriteRace(t *testing.T) {
	h := newRingHarness(t, 64)
	h.addNodes("initial-worker-0", "initial-worker-1")

	var wg sync.WaitGroup
	readerCount := 8
	iterations := 200

	for r := 0; r < readerCount; r++ {
		wg.Add(1)
		readerID := r
		goroutinelabels.NewGoroutine("test_hash_ring_reader", "concurrent read lookup").StartSimple(func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				k := fmt.Sprintf("race-key-%d-%d", readerID, i)
				node, err := h.ring.GetNode(k)
				if err == nil {
					assert.NotEmpty(t, node)
				}
				nodes, err := h.ring.GetNodes(k, 2)
				if err == nil {
					assert.NotEmpty(t, nodes)
				}
				members := h.ring.Members()
				assert.NotNil(t, members)
				sz := h.ring.Size()
				assert.GreaterOrEqual(t, sz, 0)
			}
		})
	}

	wg.Add(1)
	goroutinelabels.NewGoroutine("test_hash_ring_writer", "concurrent write mutation").StartSimple(func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			nodeID := fmt.Sprintf("dynamic-worker-%d", i)
			addErr := h.ring.AddNode(nodeID)
			assert.NoError(t, addErr)
			if i%2 == 0 {
				remErr := h.ring.RemoveNode(nodeID)
				assert.NoError(t, remErr)
			}
		}
	})

	wg.Wait()
}
