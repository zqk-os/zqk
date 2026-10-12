# Dynamic Subscriber Topology Partitioning and Work-Stealing Load Balancer Architecture

## Executive Summary
In distributed agent event meshes and reactive execution kernels, asynchronous events (e.g. scheduler callbacks, shockwaves, agent telemetry) exhibit dynamic burstiness and severe partition skew. When events are partitioned across static consumer queues, hot topics or high-throughput jobs cause worker starvation, unbounded memory accumulation, and high tail latency on overloaded shards while peer workers sit idle.

`PartitionedSubscriberPool`, `ConsistentHashRing`, and `WorkStealingBalancer` resolve this bottleneck by uniting two distributed systems primitives:
1. **Dynamic Virtual-Node Consistent Hash Ring**: Uniformly distributes incoming events across elastic partition shards with minimal key churn upon topology changes, zero heap allocations on hot path lookups, and deterministic key extraction.
2. **Chase-Lev Work-Stealing Balancer**: Equips each partition shard with a double-ended queue (deque). Workers operate locally on their primary deque (LIFO or FIFO), while idle workers dynamically steal from the tail of the most-loaded peer deques, eliminating single-worker event starvation.
3. **Lossless Dynamic Topology Migration**: When partition shards are added or removed at runtime, pending events are drained and rebalanced across the surviving topology without dropping in-flight events.

---

## Architectural Topology

```mermaid
flowchart TD
    subgraph Ingestion ["Ingestion & Key Resolution"]
        EventStream["Callback Event Stream"] --> SubPool["PartitionedSubscriberPool"]
        SubPool --> KeyExtract["ExtractRoutingKey()"]
        KeyExtract --> HashRing["ConsistentHashRing (FNV-1a 64-bit)"]
    end

    subgraph RingTopology ["Virtual Node Topology Ring"]
        HashRing --> RingBins["Binary Search (Zero Allocations)"]
        RingBins --> Shard0["Partition Shard 0"]
        RingBins --> Shard1["Partition Shard 1"]
        RingBins --> Shard2["Partition Shard 2"]
        RingBins --> Shard3["Partition Shard 3"]
    end

    subgraph DequeLayer ["Chase-Lev Work Deques"]
        Shard0 --> DQ0["WorkDeque 0"]
        Shard1 --> DQ1["WorkDeque 1"]
        Shard2 --> DQ2["WorkDeque 2"]
        Shard3 --> DQ3["WorkDeque 3"]
    end

    subgraph ExecutionLayer ["Supervised Worker Pool"]
        W0["Worker 0"] <-->|PopLocal Head| DQ0
        W1["Worker 1"] <-->|PopLocal Head| DQ1
        W2["Worker 2"] <-->|PopLocal Head| DQ2
        W3["Worker 3"] <-->|PopLocal Head| DQ3

        W0 -.->|Steal Tail (Max Loaded)| DQ3
        W1 -.->|Steal Tail (Max Loaded)| DQ0
        W2 -.->|Steal Tail (Max Loaded)| DQ1
        W3 -.->|Steal Tail (Max Loaded)| DQ2
    end
```

---

## 1. Consistent Hash Ring & Virtual Node Topology

The `ConsistentHashRing` maps arbitrary routing keys (e.g., job IDs, partition keys, event topics) to physical partition shards.

### Virtual Node Replication
To ensure uniform statistical distribution across partitions and prevent clustering, each physical node is replicated into $V$ virtual nodes (default $V = 100$):
$$\text{vnode}_{i, k} = \text{hash}(\text{node}_i + \text{"#vn"} + k), \quad 0 \le k < V$$

### Zero-Allocation Lookup
Key resolution uses an inlined binary search over the sorted slice of 64-bit uint hashes:
```go
func (c *ConsistentHashRing) GetNode(key string) (string, error) {
    c.mu.RLock()
    defer c.mu.RUnlock()

    n := len(c.ring)
    if n == 0 {
        return "", ErrNoNodesInRing
    }

    h := hashFNV1a64(key)
    low, high := 0, n
    for low < high {
        mid := int(uint(low+high) >> 1)
        if c.ring[mid] < h {
            low = mid + 1
        } else {
            high = mid
        }
    }
    if low == n {
        low = 0
    }
    return c.vnodeToNode[c.ring[low]], nil
}
```
This hot-path implementation is verified with `testing.AllocsPerRun` to produce **zero heap allocations** per resolution.

---

## 2. Work-Stealing Deque Mechanics (Chase-Lev)

The work-stealing subsystem is implemented in `WorkDeque` and `WorkStealingBalancer`.

### Dual-Ended Operations
- **Local Owner Access (Head)**: The worker owning the shard pushes and pops from the head of its local deque. In standard LIFO mode, recently pushed items benefit from cache locality. In FIFO mode, items are processed in arrival order.
- **Thief Access (Tail)**: When a worker exhausts its local deque, it inspects neighboring and peer deques. Thieves steal from the tail (the oldest unconsumed items), minimizing lock contention with the owner operating at the head.

### Heuristic Victim Selection
When a thief initiates a steal:
1. It queries the active partition deques and selects the victim with the maximum pending queue length ($\max \text{Len} > 0$).
2. It attempts `StealTail()` on that victim deque.
3. If concurrent thieves race on the victim, the thief performs a sweep over remaining active deques before entering an idle sleep state.

---

## 3. Dynamic Topology Rebalancing & Lossless Migration

In dynamic microservices and elastic multi-agent swarms, workers and partitions scale up and down dynamically.

### Node Addition Protocol
When `AddNode(newNode)` or `AddPartition(newNode)` is invoked:
1. The new node and its $V$ virtual nodes are inserted into the hash ring.
2. A new `WorkDeque` is instantiated.
3. Supervised background workers are spawned via `goroutinelabels.NewGoroutine`.
4. Idle workers immediately begin stealing work from saturated peers to service the new shard.

### Node Removal Protocol (Lossless Drain)
When `RemoveNode(oldNode)` or `RemovePartition(oldNode)` is invoked:
1. Fail-closed safeguard: removing the final active partition is rejected with `ErrCannotRemoveLastPartition`.
2. The removing node's deque is atomically drained via `Drain()`.
3. The node and its virtual nodes are removed from the ring and balancer topology.
4. **Immediate Rebalancing**: Drained pending events are immediately re-routed through `balancer.Enqueue()` to remaining partitions according to the updated hash ring. Zero events are lost or dropped during partition decommissioning.

---

## 4. Supervised Lifecycle & Resource Hygiene

`PartitionedSubscriberPool` integrates with the ZQK supervisor framework:
- **Goroutine Supervision**: Every partition worker is spawned using `goroutinelabels.NewGoroutine("partition_worker_...", ...)`.
- **Clean Context Teardown**: Shutdown gracefully cancels worker contexts, notifies wake channels, drains in-flight handlers, and joins all worker goroutines via `sync.WaitGroup`, guaranteeing zero goroutine leaks under `-race`.
- **Operational Telemetry**: `Metrics()` reports atomic counters for `EnqueuedTotal`, `ProcessedTotal`, `StolenTotal`, `DroppedTotal`, `FailedTotal`, `InFlight`, and per-partition queue lengths.

---

## 5. Verification Traceability Matrix

| Requirement / Criterion | Description | Verification Method | Status |
| :--- | :--- | :--- | :--- |
| **REQ-DYNAMIC-TOPOLOGY-PARTITIONING** | Dynamic subscriber topology partitioning, shard migration, and work-stealing load balancing. | Unit, Concurrency, and Race Detector Suites | Verified |
| **CRIT-CONSISTENT-HASH-RING** | Virtual node replication ($V=100$), uniform distribution, zero-allocation binary search, minimal remapping churn. | `TestConsistentHashRing_BasicTopology`<br>`TestConsistentHashRing_DistributionUniformity`<br>`TestConsistentHashRing_NodeAdditionAndRemoval`<br>`TestConsistentHashRing_ZeroAllocations` | Verified |
| **CRIT-WORK-STEALING-BALANCER** | Chase-Lev dual-ended queue, local LIFO/FIFO processing, cross-shard work stealing under skewed load, lossless dynamic rebalancing. | `TestWorkDeque_HeadAndTailDiscipline`<br>`TestWorkStealingBalancer_BasicRoutingAndStealing`<br>`TestWorkStealingBalancer_DynamicRebalanceOnNodeRemoval`<br>`TestPartitionedSubscriberPool_SkewedLoadWorkStealing`<br>`TestPartitionedSubscriberPool_FIFOVsLIFOOrder`<br>`TestPartitionedSubscriberPool_DynamicPartitionManagement` | Verified |
| **CRIT-DYNAMIC-TOPOLOGY-SPEC** | Comprehensive architectural documentation, topology models, protocol specifications, and negative boundary verification. | `docs/architecture/DYNAMIC_SUBSCRIBER_TOPOLOGY_WORK_STEALING.md`<br>`TestPartitionedSubscriberPool_CleanTeardownAndZeroLeaks`<br>`TestPartitionedSubscriberPool_NegativeBoundaryAndErrorHandling` | Verified |
| **TST-DYNAMIC-TOPOLOGY-VERIFICATION** | Unified verification test suite for dynamic topology partitioning and work stealing. | `go test -v -race ./cmd/zqk/callback/...` | Verified |
