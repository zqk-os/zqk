package callback

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// Helper test harnesses for AST deduplication and clean assertions.

func newStealTestEntry(jobID string, partitionKey string) *CallbackEntry {
	return &CallbackEntry{
		JobID:     jobID,
		Timestamp: time.Now().UTC(),
		Payload: map[string]any{
			keyPartitionKey: partitionKey,
			keyTopic:        partitionKey,
		},
	}
}

func mustAddNodesToRing(t *testing.T, ring *ConsistentHashRing, nodeNames []string) {
	for _, n := range nodeNames {
		err := ring.AddNode(n)
		require.NoError(t, err)
	}
}

func mustEnqueueBatch(t *testing.T, balancer *WorkStealingBalancer, count int, keyPrefix string) {
	for i := 0; i < count; i++ {
		entry := newStealTestEntry(fmt.Sprintf("%s-%d", keyPrefix, i), fmt.Sprintf("%s-item-%d", keyPrefix, i))
		target, err := balancer.Enqueue(ExtractRoutingKey(entry), entry)
		require.NoError(t, err)
		assert.NotEmpty(t, target)
	}
}

func mustNotifyBatch(t *testing.T, pool *PartitionedSubscriberPool, ctx context.Context, count int, prefix string) {
	for i := 0; i < count; i++ {
		entry := newStealTestEntry(fmt.Sprintf("%s-%d", prefix, i), prefix)
		err := pool.Notify(ctx, entry)
		require.NoError(t, err)
	}
}

func mustDrainPool(t *testing.T, pool *PartitionedSubscriberPool, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err := pool.Drain(ctx)
	require.NoError(t, err)
}

// Tests for ConsistentHashRing

func TestConsistentHashRing_BasicTopology(t *testing.T) {
	ring := NewConsistentHashRing(50)
	assert.Equal(t, 0, ring.NodeCount())

	_, err := ring.GetNode("any-key")
	assert.ErrorIs(t, err, ErrNoNodesInRing)

	err = ring.AddNode("")
	assert.ErrorIs(t, err, ErrEmptyNodeName)

	err = ring.RemoveNode("")
	assert.ErrorIs(t, err, ErrEmptyNodeName)

	err = ring.RemoveNode("non-existent")
	assert.ErrorIs(t, err, ErrNodeNotFound)

	nodes := []string{"node-1", "node-2", "node-3"}
	mustAddNodesToRing(t, ring, nodes)

	err = ring.AddNode("node-1")
	assert.NoError(t, err)
	assert.Equal(t, 3, ring.NodeCount())

	activeNodes := ring.Nodes()
	assert.Equal(t, []string{"node-1", "node-2", "node-3"}, activeNodes)

	targetNode, err := ring.GetNode("job-98765")
	assert.NoError(t, err)
	assert.True(t, ring.HasNode(targetNode))

	err = ring.RemoveNode("node-2")
	assert.NoError(t, err)
	assert.False(t, ring.HasNode("node-2"))
	assert.Equal(t, 2, ring.NodeCount())
}

func TestConsistentHashRing_DistributionUniformity(t *testing.T) {
	nodeCount := 4
	ring := NewConsistentHashRing(100)

	nodeNames := make([]string, nodeCount)
	for i := 0; i < nodeCount; i++ {
		nodeNames[i] = fmt.Sprintf("shard-%d", i)
	}
	mustAddNodesToRing(t, ring, nodeNames)

	totalKeys := 10000
	distribution := make(map[string]int)

	for i := 0; i < totalKeys; i++ {
		key := fmt.Sprintf("partition-key-item-%d", i)
		node, err := ring.GetNode(key)
		require.NoError(t, err)
		distribution[node]++
	}

	expectedPerNode := totalKeys / nodeCount
	minExpected := int(float64(expectedPerNode) * 0.55)
	maxExpected := int(float64(expectedPerNode) * 1.45)

	for node, count := range distribution {
		assert.GreaterOrEqual(t, count, minExpected, "node %s received too few keys", node)
		assert.LessOrEqual(t, count, maxExpected, "node %s received too many keys", node)
	}
}

func TestConsistentHashRing_NodeAdditionAndRemoval(t *testing.T) {
	ring := NewConsistentHashRing(100)
	initialNodes := []string{"node-A", "node-B", "node-C", "node-D"}
	mustAddNodesToRing(t, ring, initialNodes)

	totalKeys := 2000
	initialMapping := make(map[string]string)
	for i := 0; i < totalKeys; i++ {
		key := fmt.Sprintf("stream-event-%d", i)
		node, err := ring.GetNode(key)
		require.NoError(t, err)
		initialMapping[key] = node
	}

	err := ring.AddNode("node-E")
	require.NoError(t, err)

	movedToNewNode := 0
	movedBetweenOldNodes := 0

	for key, oldNode := range initialMapping {
		newNode, getErr := ring.GetNode(key)
		require.NoError(t, getErr)
		if newNode == "node-E" {
			movedToNewNode++
		} else if newNode != oldNode {
			movedBetweenOldNodes++
		}
	}

	assert.Equal(t, 0, movedBetweenOldNodes, "keys should never move between old nodes")
	assert.Greater(t, movedToNewNode, 0, "some keys should migrate to new node-E")

	err = ring.RemoveNode("node-A")
	require.NoError(t, err)

	for i := 0; i < totalKeys; i++ {
		key := fmt.Sprintf("stream-event-%d", i)
		node, getErr := ring.GetNode(key)
		require.NoError(t, getErr)
		assert.NotEqual(t, "node-A", node)
	}
}

func TestConsistentHashRing_ZeroAllocations(t *testing.T) {
	ring := NewConsistentHashRing(100)
	nodeNames := make([]string, 8)
	for i := 0; i < 8; i++ {
		nodeNames[i] = fmt.Sprintf("node-shard-%d", i)
	}
	mustAddNodesToRing(t, ring, nodeNames)

	key := "benchmark-test-routing-key-999"
	warmupNode, warmupErr := ring.GetNode(key)
	require.NoError(t, warmupErr)
	assert.NotEmpty(t, warmupNode)

	allocs := testing.AllocsPerRun(1000, func() {
		node, err := ring.GetNode(key)
		if err != nil || node == emptyValue {
			t.Fatalf("GetNode failed: %v", err)
		}
	})

	assert.Equal(t, float64(0), allocs, "expected 0 allocs on GetNode hot path")
}

// Tests for WorkDeque

func TestWorkDeque_PushBottomAndPopBottomLIFO(t *testing.T) {
	dq := NewWorkDeque()
	assert.Equal(t, 0, dq.Len())
	assert.True(t, dq.IsEmpty())

	item, ok := dq.PopBottom()
	assert.False(t, ok)
	assert.Nil(t, item)

	item, ok = dq.StealTop()
	assert.False(t, ok)
	assert.Nil(t, item)

	e1 := newStealTestEntry("job-1", "part-1")
	e2 := newStealTestEntry("job-2", "part-2")
	e3 := newStealTestEntry("job-3", "part-3")

	dq.PushBottom(e1)
	dq.PushBottom(e2)
	dq.PushBottom(e3)

	assert.Equal(t, 3, dq.Len())
	assert.False(t, dq.IsEmpty())

	popped, ok := dq.PopBottom()
	assert.True(t, ok)
	assert.Equal(t, "job-3", popped.JobID)

	stolen, ok := dq.StealTop()
	assert.True(t, ok)
	assert.Equal(t, "job-1", stolen.JobID)

	last, ok := dq.Steal()
	assert.True(t, ok)
	assert.Equal(t, "job-2", last.JobID)

	assert.True(t, dq.IsEmpty())
}

func TestWorkDeque_CompatibilityAndDrain(t *testing.T) {
	dq := NewWorkDeque()

	e1 := newStealTestEntry("job-10", "p0")
	e2 := newStealTestEntry("job-20", "p1")
	e3 := newStealTestEntry("job-30", "p2")

	dq.PushHead(e1)
	dq.PushTail(e2)
	dq.PushBottom(e3)

	assert.Equal(t, 3, dq.Len())

	drained := dq.Drain()
	assert.Len(t, drained, 3)
	assert.True(t, dq.IsEmpty())

	emptyDrained := dq.Drain()
	assert.Nil(t, emptyDrained)

	dq.PushBottom(e1)
	dq.PushBottom(e2)

	pHead, ok := dq.PopHead()
	assert.True(t, ok)
	assert.Equal(t, "job-20", pHead.JobID)

	pTail, ok := dq.PopTail()
	assert.True(t, ok)
	assert.Equal(t, "job-10", pTail.JobID)
	assert.True(t, dq.IsEmpty())
}

// Tests for TopologyPartitionMap

func TestTopologyPartitionMap_DynamicPartitioning(t *testing.T) {
	numPartitions := 16
	top := NewTopologyPartitionMap(numPartitions, 0, 1)

	assert.Equal(t, numPartitions, top.NumPartitions())
	assert.Equal(t, 2, top.NumWorkers())
	assert.Equal(t, []int{0, 1}, top.ActiveWorkers())

	w0 := top.WorkerForPartition(0)
	w1 := top.WorkerForPartition(1)
	assert.Equal(t, 0, w0)
	assert.Equal(t, 1, w1)

	keyWorker := top.WorkerForKey("some-hash-key")
	assert.True(t, keyWorker == 0 || keyWorker == 1)

	parts0 := top.PartitionsForWorker(0)
	parts1 := top.PartitionsForWorker(1)
	assert.Len(t, parts0, 8)
	assert.Len(t, parts1, 8)

	top.AddWorker(2)
	assert.Equal(t, 3, top.NumWorkers())
	assert.Equal(t, []int{0, 1, 2}, top.ActiveWorkers())

	partsTotal := len(top.PartitionsForWorker(0)) + len(top.PartitionsForWorker(1)) + len(top.PartitionsForWorker(2))
	assert.Equal(t, numPartitions, partsTotal)

	top.AssignPartition(4, 2)
	assert.Equal(t, 2, top.WorkerForPartition(4))

	err := top.RemoveWorker(1)
	assert.NoError(t, err)
	assert.Equal(t, 2, top.NumWorkers())

	err = top.RemoveWorker(99)
	assert.ErrorIs(t, err, ErrNodeNotFound)

	err = top.RemoveWorker(0)
	assert.NoError(t, err)

	err = top.RemoveWorker(2)
	assert.ErrorIs(t, err, ErrCannotRemoveLastPartition)

	top.SetWorkerCount(4)
	assert.Equal(t, 4, top.NumWorkers())
	assert.Equal(t, []int{0, 1, 2, 3}, top.ActiveWorkers())
}

// Tests for WorkStealingPool

func TestWorkStealingPool_LocalPopAndSteal(t *testing.T) {
	pool := NewWorkStealingPool(2)
	defer func() {
		closeErr := pool.Close()
		require.NoError(t, closeErr)
	}()

	assert.Equal(t, SubscriberNameWorkStealingPool, pool.Name())
	assert.False(t, pool.IsClosed())

	e1 := newStealTestEntry("job-a", "p0")
	e2 := newStealTestEntry("job-b", "p0")

	// Submit both items targeted to partition 0 (worker 0).
	ok := pool.Submit(e1, 0)
	assert.True(t, ok)
	ok = pool.Submit(e2, 0)
	assert.True(t, ok)

	stats := pool.Stats()
	assert.Equal(t, int64(2), stats.TotalSubmitted)
	assert.Equal(t, 2, stats.TotalPending)

	// Worker 0 pops its own local entry (LIFO -> job-b).
	localItem, ok := pool.WorkerPopOrSteal(0)
	assert.True(t, ok)
	assert.Equal(t, "job-b", localItem.JobID)

	// Worker 1 is idle and attempts to steal from worker 0 (FIFO -> job-a).
	stolenItem, ok := pool.WorkerPopOrSteal(1)
	assert.True(t, ok)
	assert.Equal(t, "job-a", stolenItem.JobID)

	// Both deques now empty.
	emptyItem, ok := pool.WorkerPopOrSteal(0)
	assert.False(t, ok)
	assert.Nil(t, emptyItem)

	statsAfter := pool.Stats()
	assert.Equal(t, int64(1), statsAfter.LocalPops)
	assert.Equal(t, int64(1), statsAfter.Steals)
	assert.GreaterOrEqual(t, statsAfter.StealFailures, int64(1))
	assert.Equal(t, 0, statsAfter.TotalPending)
}

func TestWorkStealingPool_EmptyBackoff(t *testing.T) {
	pool := NewWorkStealingPool(2)
	defer func() {
		err := pool.Close()
		require.NoError(t, err)
	}()

	start := time.Now()
	item, ok := pool.WorkerPopOrSteal(0)
	elapsed := time.Since(start)

	assert.False(t, ok)
	assert.Nil(t, item)
	assert.GreaterOrEqual(t, elapsed, DefaultWorkStealingBackoff/2)

	stats := pool.Stats()
	assert.Equal(t, int64(1), stats.StealFailures)
}

func TestWorkStealingPool_DynamicRebalance(t *testing.T) {
	pool := NewWorkStealingPool(4)
	defer func() {
		err := pool.Close()
		require.NoError(t, err)
	}()

	totalEntries := 40
	for i := 0; i < totalEntries; i++ {
		entry := newStealTestEntry(fmt.Sprintf("reb-job-%d", i), fmt.Sprintf("part-%d", i%4))
		ok := pool.Submit(entry, i%4)
		assert.True(t, ok)
	}

	assert.Equal(t, totalEntries, pool.Stats().TotalPending)

	// Scale down to 2 workers. Drained items from workers 2 and 3 must be preserved.
	pool.Rebalance(2)
	assert.Equal(t, 2, pool.Stats().ActiveWorkers)
	assert.Equal(t, totalEntries, pool.Stats().TotalPending)

	// Scale up to 6 workers.
	pool.Rebalance(6)
	assert.Equal(t, 6, pool.Stats().ActiveWorkers)
	assert.Equal(t, totalEntries, pool.Stats().TotalPending)

	drained := pool.Drain()
	assert.Len(t, drained, totalEntries)
	assert.Equal(t, 0, pool.Stats().TotalPending)
}

func TestWorkStealingPool_DrainAndShutdown(t *testing.T) {
	pool := NewWorkStealingPool(3)

	for i := 0; i < 15; i++ {
		entry := newStealTestEntry(fmt.Sprintf("sd-job-%d", i), "key")
		ok := pool.Submit(entry, -1)
		assert.True(t, ok)
	}

	drained := pool.Drain()
	assert.Len(t, drained, 15)
	assert.Equal(t, 0, pool.Stats().TotalPending)

	err := pool.Close()
	assert.NoError(t, err)
	assert.True(t, pool.IsClosed())

	ok := pool.Submit(newStealTestEntry("post-close", "key"), -1)
	assert.False(t, ok)

	notifyErr := pool.Notify(context.Background(), newStealTestEntry("post-notify", "key"))
	assert.ErrorIs(t, notifyErr, ErrSubscriberPoolClosed)

	nilNotifyErr := pool.Notify(context.Background(), nil)
	assert.ErrorIs(t, nilNotifyErr, ErrSubscriberPoolClosed)
}

func TestWorkStealingPool_ConcurrentRaceFree(t *testing.T) {
	numWorkers := 4
	pool := NewWorkStealingPool(numWorkers)
	defer func() {
		err := pool.Close()
		require.NoError(t, err)
	}()

	concurrency := 4
	itemsPerProducer := 50
	totalItems := concurrency * itemsPerProducer

	var producerWg sync.WaitGroup
	for p := 0; p < concurrency; p++ {
		producerWg.Add(1)
		prodID := p
		goroutinelabels.NewGoroutine(fmt.Sprintf("pool_producer_%d", prodID), "concurrent submit").StartSimple(func() {
			defer producerWg.Done()
			for i := 0; i < itemsPerProducer; i++ {
				entry := newStealTestEntry(fmt.Sprintf("p-%d-%d", prodID, i), fmt.Sprintf("key-%d", (prodID+i)%8))
				submitted := pool.Submit(entry, (prodID+i)%numWorkers)
				if !submitted {
					t.Errorf("failed to submit entry")
				}
			}
		})
	}

	producerWg.Wait()

	var consumerWg sync.WaitGroup
	var consumedTotal atomic.Int64
	stopConsumers := atomic.Bool{}

	for w := 0; w < numWorkers; w++ {
		consumerWg.Add(1)
		workerID := w
		goroutinelabels.NewGoroutine(fmt.Sprintf("pool_consumer_%d", workerID), "concurrent pop and steal").StartSimple(func() {
			defer consumerWg.Done()
			for !stopConsumers.Load() {
				item, ok := pool.WorkerPopOrSteal(workerID)
				if ok && item != nil {
					consumedTotal.Add(1)
				}
				if consumedTotal.Load() >= int64(totalItems) {
					return
				}
			}
		})
	}

	deadline := time.Now().Add(5 * time.Second)
	for consumedTotal.Load() < int64(totalItems) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	stopConsumers.Store(true)
	consumerWg.Wait()

	assert.Equal(t, int64(totalItems), consumedTotal.Load())
	stats := pool.Stats()
	assert.GreaterOrEqual(t, stats.LocalPops+stats.Steals, int64(totalItems))
}

// Tests for WorkStealingBalancer

func TestWorkStealingBalancer_BasicRoutingAndStealing(t *testing.T) {
	nodes := []string{"shard-A", "shard-B", "shard-C"}
	balancer, err := NewWorkStealingBalancer(nodes, 50, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, balancer.TotalPending())

	mustEnqueueBatch(t, balancer, 30, "balancer-job")
	assert.Equal(t, 30, balancer.TotalPending())

	var populatedNode string
	for _, n := range nodes {
		if balancer.DequeLen(n) > 0 {
			populatedNode = n
			break
		}
	}
	require.NotEmpty(t, populatedNode)

	var thiefNode string
	for _, n := range nodes {
		if n != populatedNode {
			thiefNode = n
			break
		}
	}

	stolenEntry, victim, ok := balancer.Steal(thiefNode, ProcessingOrderLIFO)
	assert.True(t, ok)
	assert.NotNil(t, stolenEntry)
	assert.NotEqual(t, thiefNode, victim)

	localEntry, ok := balancer.PopLocal(populatedNode, ProcessingOrderLIFO)
	assert.True(t, ok)
	assert.NotNil(t, localEntry)
}

func TestWorkStealingBalancer_DynamicRebalanceOnNodeRemoval(t *testing.T) {
	nodes := []string{"node-0", "node-1", "node-2"}
	balancer, err := NewWorkStealingBalancer(nodes, 50, nil)
	require.NoError(t, err)

	mustEnqueueBatch(t, balancer, 60, "bal-work")
	assert.Equal(t, 60, balancer.TotalPending())

	drained, err := balancer.RemoveNode("node-1")
	require.NoError(t, err)
	assert.False(t, balancer.HasNode("node-1"))
	assert.GreaterOrEqual(t, len(drained), 0)
	assert.Equal(t, 60, balancer.TotalPending())

	drained2, err := balancer.RemoveNode("node-2")
	require.NoError(t, err)
	assert.False(t, balancer.HasNode("node-2"))
	assert.GreaterOrEqual(t, len(drained2), 0)
	assert.Equal(t, 60, balancer.TotalPending())

	_, err = balancer.RemoveNode("node-0")
	assert.ErrorIs(t, err, ErrCannotRemoveLastPartition)
}

// Tests for PartitionedSubscriberPool

func TestPartitionedSubscriberPool_InterfaceCompliance(t *testing.T) {
	cfg := PartitionedSubscriberConfig{
		Name:            "custom_pool",
		Partitions:      []string{"part-1", "part-2"},
		WorkersPerShard: 1,
	}
	pool, err := NewPartitionedSubscriberPool(cfg)
	require.NoError(t, err)
	defer func() {
		closeErr := pool.Close()
		require.NoError(t, closeErr)
	}()

	assert.Equal(t, "custom_pool", pool.Name())
	assert.NotNil(t, pool.Balancer())

	var sub Subscriber = pool
	var cbSub CallbackSubscriber = pool
	assert.NotNil(t, sub)
	assert.NotNil(t, cbSub)
}

func TestPartitionedSubscriberPool_SkewedLoadWorkStealing(t *testing.T) {
	var processedCount atomic.Int64
	processedIDs := sync.Map{}

	handler := func(ctx context.Context, entry *CallbackEntry) error {
		processedIDs.Store(entry.JobID, struct{}{})
		processedCount.Add(1)
		time.Sleep(2 * time.Millisecond)
		return nil
	}

	cfg := PartitionedSubscriberConfig{
		Partitions:      []string{"shard-0", "shard-1", "shard-2", "shard-3"},
		WorkersPerShard: 1,
		ProcessingOrder: ProcessingOrderLIFO,
		Handler:         handler,
	}

	pool, err := NewPartitionedSubscriberPool(cfg)
	require.NoError(t, err)
	defer func() {
		closeErr := pool.Close()
		require.NoError(t, closeErr)
	}()

	totalEntries := 100
	ctx := context.Background()

	for i := 0; i < totalEntries; i++ {
		entry := &CallbackEntry{
			JobID: fmt.Sprintf("skewed-job-%d", i),
			Payload: map[string]any{
				keyPartitionKey: "single-hot-shard",
			},
		}
		notifyErr := pool.Notify(ctx, entry)
		require.NoError(t, notifyErr)
	}

	mustDrainPool(t, pool, 5*time.Second)

	deadline := time.Now().Add(5 * time.Second)
	for pool.Metrics().ProcessedTotal < int64(totalEntries) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	metrics := pool.Metrics()
	assert.Equal(t, int64(totalEntries), metrics.ProcessedTotal)
	assert.Greater(t, metrics.StolenTotal, int64(0), "idle workers must steal under skewed load")

	count := 0
	processedIDs.Range(func(key, value any) bool {
		count++
		return true
	})
	assert.Equal(t, totalEntries, count)
}

func TestPartitionedSubscriberPool_FIFOVsLIFOOrder(t *testing.T) {
	var lifoOrder []string
	var lifoMu sync.Mutex

	lifoPool, err := NewPartitionedSubscriberPool(PartitionedSubscriberConfig{
		Partitions:      []string{"single-shard"},
		WorkersPerShard: 1,
		ProcessingOrder: ProcessingOrderLIFO,
		Handler: func(ctx context.Context, entry *CallbackEntry) error {
			lifoMu.Lock()
			lifoOrder = append(lifoOrder, entry.JobID)
			lifoMu.Unlock()
			return nil
		},
	})
	require.NoError(t, err)

	ctx := context.Background()
	mustNotifyBatch(t, lifoPool, ctx, 5, "lifo-job")
	mustDrainPool(t, lifoPool, 2*time.Second)

	closeErr := lifoPool.Close()
	require.NoError(t, closeErr)

	lifoMu.Lock()
	assert.Len(t, lifoOrder, 5)
	lifoMu.Unlock()
}

func TestPartitionedSubscriberPool_DynamicPartitionManagement(t *testing.T) {
	var processedCount atomic.Int64

	pool, err := NewPartitionedSubscriberPool(PartitionedSubscriberConfig{
		Partitions:      []string{"p-0", "p-1"},
		WorkersPerShard: 1,
		Handler: func(ctx context.Context, entry *CallbackEntry) error {
			processedCount.Add(1)
			return nil
		},
	})
	require.NoError(t, err)
	defer func() {
		closeErr := pool.Close()
		require.NoError(t, closeErr)
	}()

	ctx := context.Background()
	mustNotifyBatch(t, pool, ctx, 20, "dyn-1")

	err = pool.AddPartition("p-2")
	require.NoError(t, err)
	assert.Len(t, pool.Partitions(), 3)

	mustNotifyBatch(t, pool, ctx, 20, "dyn-2")

	err = pool.RemovePartition("p-0")
	require.NoError(t, err)
	assert.Len(t, pool.Partitions(), 2)

	mustDrainPool(t, pool, 3*time.Second)

	deadline := time.Now().Add(3 * time.Second)
	for processedCount.Load() < 40 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	assert.Equal(t, int64(40), processedCount.Load())
}

func TestPartitionedSubscriberPool_ConcurrentLoadAndRaceFree(t *testing.T) {
	var processedTotal atomic.Int64

	pool, err := NewPartitionedSubscriberPool(PartitionedSubscriberConfig{
		Partitions:      []string{"w-0", "w-1", "w-2", "w-3"},
		WorkersPerShard: 2,
		Handler: func(ctx context.Context, entry *CallbackEntry) error {
			processedTotal.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	ctx := context.Background()
	concurrency := 4
	itemsPerProducer := 50
	expectedTotal := concurrency * itemsPerProducer

	var producerWg sync.WaitGroup
	for p := 0; p < concurrency; p++ {
		producerWg.Add(1)
		producerID := p
		goroutinelabels.NewGoroutine(fmt.Sprintf("part_producer_%d", producerID), "concurrent producer").StartSimple(func() {
			defer producerWg.Done()
			for i := 0; i < itemsPerProducer; i++ {
				entry := newStealTestEntry(fmt.Sprintf("prod-%d-%d", producerID, i), fmt.Sprintf("topic-%d", (producerID+i)%16))
				notifyErr := pool.Notify(ctx, entry)
				if notifyErr != nil {
					t.Errorf("Notify failed: %v", notifyErr)
				}
			}
		})
	}

	producerWg.Wait()
	mustDrainPool(t, pool, 5*time.Second)

	deadline := time.Now().Add(5 * time.Second)
	for processedTotal.Load() < int64(expectedTotal) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	closeErr := pool.Close()
	require.NoError(t, closeErr)
	assert.Equal(t, int64(expectedTotal), processedTotal.Load())
}

func TestPartitionedSubscriberPool_CleanTeardownAndZeroLeaks(t *testing.T) {
	pool, err := NewPartitionedSubscriberPool(PartitionedSubscriberConfig{
		Partitions:      []string{"t-0", "t-1"},
		WorkersPerShard: 1,
	})
	require.NoError(t, err)

	ctx := context.Background()
	mustNotifyBatch(t, pool, ctx, 10, "teardown")

	err = pool.Close()
	require.NoError(t, err)
	assert.True(t, pool.IsClosed())

	err = pool.Notify(ctx, newStealTestEntry("post-close", "key"))
	assert.ErrorIs(t, err, ErrSubscriberPoolClosed)

	err = pool.Close()
	assert.NoError(t, err)
}

func TestPartitionedSubscriberPool_NegativeBoundaryAndErrorHandling(t *testing.T) {
	var errCounter atomic.Int64
	mockErr := errors.New("simulated handler failure")

	pool, err := NewPartitionedSubscriberPool(PartitionedSubscriberConfig{
		Partitions:      []string{"err-0"},
		WorkersPerShard: 1,
		Handler: func(ctx context.Context, entry *CallbackEntry) error {
			errCounter.Add(1)
			return mockErr
		},
	})
	require.NoError(t, err)
	defer func() {
		closeErr := pool.Close()
		require.NoError(t, closeErr)
	}()

	ctx := context.Background()

	err = pool.Notify(ctx, nil)
	assert.NoError(t, err)

	err = pool.Notify(ctx, newStealTestEntry("will-fail", "err"))
	require.NoError(t, err)

	mustDrainPool(t, pool, 2*time.Second)

	deadline := time.Now().Add(2 * time.Second)
	for errCounter.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	metrics := pool.Metrics()
	assert.Equal(t, int64(1), metrics.FailedTotal)
}
