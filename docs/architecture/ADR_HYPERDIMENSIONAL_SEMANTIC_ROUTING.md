# ADR: Hyper-Dimensional Semantic Routing & Topology-Aware Context Multiplexing

## Status
Accepted

## Date
2026-11-20

## Context
As federated ZQK swarm clusters scale across heterogeneous agent seats and multi-tenant workloads, broadcasting full task trees and unpartitioned context buffers to arbitrary workers causes redundant token consumption, context window saturation, and suboptimal seat utilization.

In Cycle 162, we introduce a Hyper-Dimensional Semantic Routing and Topology-Aware Context Multiplexing subsystem in `pkg/community/semantic_router.go` to route decomposed task execution packets along semantic proximity vectors to optimal cluster seats while enforcing strict isolation, affinity cutoffs, and starvation circuit-breaker boundaries.

## Decision
1. **Hyper-Dimensional Vector Embedding Parity & Cosine Distance Invariants**:
   - All task intent vectors and cluster seat capability vectors must be non-empty, finite (`!math.IsNaN`, `!math.IsInf`) `float64` slices sharing identical dimensionality (`len(a) == len(b) >= MinEmbeddingDim`).
   - Zero-magnitude vectors (`||v|| == 0`) are rejected fail-closed before routing to prevent division-by-zero in cosine similarity computation.
   - Cosine similarity $\cos(\theta) = \frac{a \cdot b}{\|a\|\|b\|}$ is clamped to `[-1.0, 1.0]` to absorb IEEE-754 floating-point drift, and semantic distance is defined deterministically as $d = 1.0 - \cos(\theta) \in [0.0, 2.0]$.

2. **Multi-Tenant Context Multiplexing & Affinity-Based Cluster Seat Dispatching**:
   - Context buffers are partitioned by tenant namespace and multiplexed into bounded, topology-aware dispatch envelopes so concurrent tenants never leak or interleave prompt state across shared worker pools.
   - The router evaluates candidate cluster seats using a composite score combining semantic proximity (cosine similarity between task intent embedding and seat capability embedding), topology locality weight, and available seat capacity, dispatching each intent node to the highest-affinity eligible seat.
   - Routing state and seat registries are protected by `sync.RWMutex` with deterministic tie-breaking by canonical seat identifier.

3. **Semantic Distance Cutoff Enforcement & Starvation Circuit Breaker Boundaries**:
   - Every routing request enforces a strict `MaxSemanticDistance` cutoff (`0.0 < MaxSemanticDistance <= 2.0`). If no eligible cluster seat satisfies `distance <= MaxSemanticDistance`, routing fails closed with a sentinel cutoff error rather than falling back to semantically mismatched seats.
   - Each seat and tenant queue maintains a starvation window and consecutive rejection/timeout counter. When starvation thresholds are breached (e.g., sustained queue wait or consecutive dispatch failures exceeding the configured trip threshold), the starvation circuit breaker transitions to `Open`, halting dispatch to degraded or saturated paths until a cooldown probe succeeds.

## Consequences
- Enables deterministic, topology-aware task packet dispatch with `O(S * D)` routing complexity across `S` cluster seats and `D`-dimensional embeddings.
- Prevents cross-tenant context contamination via namespace-isolated context buffer multiplexing.
- Guarantees fail-closed rejection of low-affinity task placements and protects cluster stability under load spikes via starvation circuit breaking.
