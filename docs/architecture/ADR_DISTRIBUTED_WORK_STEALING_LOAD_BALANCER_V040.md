# ADR-DISTRIBUTED-WORK-STEALING-LOAD-BALANCER-V040: Distributed Work-Stealing & Dynamic Swarm Load Balancer

## Status
Accepted

## Context
In heterogeneous, asynchronous agent swarms, tasks of unpredictable computational duration (code synthesis, AST verification, test suite execution) are dispatched to worker queues. Static partition or round-robin dispatch inevitably results in stragglers, where certain workers possess deep queues while others sit completely idle.

Centralized master rebalancing introduces single points of bottleneck and high IPC traffic. To sustain continuous throughput without latency spikes, the ZQK swarm scheduler requires a decentralized work-stealing protocol (Chase-Lev inspired dual-ended deque) where idle workers autonomously steal work from saturated peers without lock contention on hot execution paths.

## Decision
1. **Work-Stealing Task Deque Architecture**:
   - `TaskDeque`: Thread-safe dual-ended queue owned by a worker process.
   - The local worker pushes and pops from the bottom of the deque in LIFO order (optimizing cache locality and call-stack traversal).
   - Remote thief workers steal from the top of the deque in FIFO order (stealing the largest/oldest units of granular work).
2. **Work-Stealing Balancer & Coordination**:
   - `WorkStealingBalancer`: Coordinates peer queue discovery and manages steal requests across active swarm nodes.
   - Steal requests specify a target victim and batch size limit (`MaxStealBatchSize`).
   - If a queue contains fewer items than the minimum steal threshold, the steal attempt returns zero tasks without error, preventing victim thrashing.
3. **Anti-Starvation & Concurrency Safety**:
   - Fine-grained mutex locking ensures thread safety between local push/pop and concurrent remote steals without deadlocks.
   - Empty queues or invalid worker requests return cleanly defined boundary errors.
4. **Declarative Hygiene & Verification**:
   - All string and ID validations adhere strictly to declarative helpers (`strutil.RequireNonBlank` / `strutil.ErrorOnBlank`).
   - Verified across three-fold proof invariants: static deque configuration, dynamic work-stealing across unbalanced queues, and empty steal/batch throttling boundaries.

## Consequences
- **Positive**:
  - Automatic load balancing across saturated and underutilized agent nodes.
  - Reduced idle time and improved swarm throughput under bursty workloads.
  - Zero centralized scheduler bottlenecks; peer-to-peer work distribution.
- **Negative**:
  - Requires thread synchronization on deque boundary operations.
