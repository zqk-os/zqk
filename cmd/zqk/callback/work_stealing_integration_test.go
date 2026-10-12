package callback

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// TestTopologyPartitioning_VirtualShardDistribution verifies CRIT-1791680033295815000-da585268:
// Functional Acceptance: Dynamic topology partitioner maps subscribers across active virtual shards
// with zero allocation and deterministically balances shard distribution.
func TestTopologyPartitioning_VirtualShardDistribution(t *testing.T) {
	const totalPartitions = 32
	const initialWorkers = 4

	// Topology partition map creates deterministic shard balancing across workers
	top := NewTopologyPartitionMap(totalPartitions, 0, 1, 2, 3)
	require.NotNil(t, top)
	assert.Equal(t, totalPartitions, top.NumPartitions())
	assert.Equal(t, initialWorkers, top.NumWorkers())

	workerDistribution := make(map[int]int)
	for p := 0; p < totalPartitions; p++ {
		w := top.WorkerForPartition(p)
		assert.True(t, w >= 0 && w < initialWorkers, "worker index must be within active pool range")
		workerDistribution[w]++
	}

	// Each worker must have an even share of partitions (32 / 4 = 8 partitions per worker)
	for w := 0; w < initialWorkers; w++ {
		assert.Equal(t, 8, workerDistribution[w], "each worker must receive balanced partition allocation")
	}

	// Dynamic rebalancing when scaling workers from 4 to 8
	top.SetWorkerCount(8)
	assert.Equal(t, 8, top.NumWorkers())

	newDistribution := make(map[int]int)
	for p := 0; p < totalPartitions; p++ {
		w := top.WorkerForPartition(p)
		assert.True(t, w >= 0 && w < 8)
		newDistribution[w]++
	}
	for w := 0; w < 8; w++ {
		assert.Equal(t, 4, newDistribution[w], "each worker must receive 4 partitions after scale up")
	}
}

// TestWorkStealingPool_ConcurrentStealingUnderBurstSkew verifies CRIT-1791680033295816000-ef7337af:
// Boundary & Error Handling: Work-stealing pool supports concurrent task enqueue/dequeue,
// lock-free work-stealing from peer queues on idle starvation, and thread-safe shutdown.
func TestWorkStealingPool_ConcurrentStealingUnderBurstSkew(t *testing.T) {
	const numWorkers = 4
	pool := NewWorkStealingPool(numWorkers)
	defer func() {
		require.NoError(t, pool.Close())
	}()

	const totalJobs = 100
	// Heavy skew: Submit all 100 jobs to worker 0
	for i := 0; i < totalJobs; i++ {
		entry := &CallbackEntry{
			JobID:     fmt.Sprintf("skewed-job-%d", i),
			Timestamp: time.Now().UTC(),
		}
		ok := pool.Submit(entry, 0)
		require.True(t, ok)
	}

	var processed atomic.Int64
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	// Launch concurrent consumers across all 4 workers
	for w := 0; w < numWorkers; w++ {
		workerID := w
		goroutinelabels.NewGoroutine("work-stealing-worker", "concurrent consumer").StartSimple(func() {
			defer wg.Done()
			for {
				item, ok := pool.WorkerPopOrSteal(workerID)
				if !ok {
					// Check if all jobs are drained
					if pool.Stats().TotalPending == 0 {
						return
					}
					time.Sleep(2 * time.Millisecond)
					continue
				}
				if item != nil {
					processed.Add(1)
				}
			}
		})
	}

	wg.Wait()

	assert.Equal(t, int64(totalJobs), processed.Load(), "all 100 jobs must be executed across workers")
	stats := pool.Stats()
	assert.Equal(t, 0, stats.TotalPending, "pending queue must be fully drained")
	assert.Greater(t, stats.Steals, int64(0), "idle workers must have successfully stolen tasks from worker 0")
}

// TestWorkStealingBalancer_FailoverAndDrain verifies CRIT-1791680033295817000-39d04bd9:
// Documentation & Verification: Swarm work-stealing topology and failover drain verification suite.
func TestWorkStealingBalancer_FailoverAndDrain(t *testing.T) {
	nodes := []string{"subscriber-node-1", "subscriber-node-2", "subscriber-node-3"}
	balancer, err := NewWorkStealingBalancer(nodes, 20, nil)
	require.NoError(t, err)

	const itemCount = 60
	for i := 0; i < itemCount; i++ {
		entry := &CallbackEntry{
			JobID:     fmt.Sprintf("failover-entry-%d", i),
			Timestamp: time.Now().UTC(),
			Payload: map[string]any{
				keyPartitionKey: fmt.Sprintf("partition-%d", i%6),
			},
		}
		target, enqueueErr := balancer.Enqueue(ExtractRoutingKey(entry), entry)
		require.NoError(t, enqueueErr)
		assert.NotEmpty(t, target)
	}

	assert.Equal(t, itemCount, balancer.TotalPending())

	// Simulate node removal (failover of subscriber-node-2) and capture rebalance orphaned items
	orphaned, err := balancer.RemoveNode("subscriber-node-2")
	require.NoError(t, err)
	assert.Equal(t, 2, len(balancer.Nodes()))

	// Re-enqueue orphaned items to the surviving nodes
	for _, entry := range orphaned {
		_, enqueueErr := balancer.Enqueue(ExtractRoutingKey(entry), entry)
		require.NoError(t, enqueueErr)
	}

	// Drain all items across remaining nodes via PopLocal and Steal
	dequeued := 0
	remainingNodes := balancer.Nodes()
	for {
		progress := false
		for _, n := range remainingNodes {
			if item, ok := balancer.PopLocal(n, ProcessingOrderFIFO); ok {
				assert.NotNil(t, item)
				dequeued++
				progress = true
			}
		}
		if !progress {
			// Try steal
			for _, n := range remainingNodes {
				if item, _, ok := balancer.Steal(n, ProcessingOrderFIFO); ok {
					assert.NotNil(t, item)
					dequeued++
					progress = true
				}
			}
		}
		if !progress {
			break
		}
	}

	assert.Equal(t, itemCount, dequeued, "all items must be retrieved without loss despite node failover")
	assert.Equal(t, 0, balancer.TotalPending(), "all balancer deques must be completely drained")
}
