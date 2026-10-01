# Cellular Specialization Tiers (`pkg/specialization`)

`pkg/specialization` governs horizontal node segregation, component filtering, and side-effect sandboxing within ZQK swarm deployments.

---

## 1. Concept: The Biological Analogy

Rather than monolithic clustering where every node runs every daemon, ZQK allows nodes to specialize into biological organ roles:

```
                      ┌────────────────────────────────────────┐
                      │           ZQK Swarm Organism           │
                      └───────────────────┬────────────────────┘
                                          │
        ┌───────────────────┬─────────────┴─────┬───────────────────┐
        ▼                   ▼                   ▼                   ▼
 ┌──────────────┐    ┌──────────────┐    ┌──────────────┐    ┌──────────────┐
 │ TierNeuron   │    │  TierMuscle  │    │  TierHeart   │    │   TierLung   │
 ├──────────────┤    ├──────────────┤    ├──────────────┤    ├──────────────┤
 │ - Inference  │    │ - Execution  │    │ - Vitality   │    │ - Mesh P2P   │
 │ - Reconciler │    │ - Scheduler  │    │ - Telemetry  │    │ - Peering    │
 │ - Assessor   │    │ - Importer   │    │ - Health     │    │ - Federation │
 │ - Semantics  │    │ - Docman     │    │ - Retention  │    │ - Transport  │
 └──────────────┘    └──────────────┘    └──────────────┘    └──────────────┘
```

By default in local single-machine and community setups, the tier is **`TierAll` (`"all"`)**, meaning the single node orchestrates all components concurrently.

---

## 2. Specialization Tiers

| Tier | Value | Designated Components | Purpose |
|---|---|---|---|
| **All** | `"all"` | *All components* | Monolithic single-node mode (default developer & community setup). |
| **Neuron** | `"neuron"` | `reconciler`, `assessor`, `inference` | High-memory, GPU/LLM-adjacent compute focused on reasoning and state reconciliation. |
| **Muscle** | `"muscle"` | `scheduler`, `importer`, `docman` | High-CPU compute responsible for running scheduled jobs, building artifacts, and executing tasks. |
| **Heart** | `"heart"` | `metrics`, `health` | Low-latency monitoring node tracking process vitality, CAS integrity, and audit aggregation. |
| **Lung** | `"lung"` | `mesh`, `federation` | Gateway nodes managing inter-cluster transport, peer gossip, and packet routing. |

---

## 3. Operational Modes: Normal vs Shielded (Rubber Room)

- **`ModeNormal` (`"normal"`)**: Standard mode where mutations and state transitions commit directly to the primary storage spine.
- **`ModeShielded` (`"shielded"`)**: Sandboxed mode running within a **ShadowSpine ("Rubber Room")**. Side-effects, disk writes, and mutations are isolated from the production kernel graph, enabling safe execution of untrusted code, exploratory simulations, and fuzzing passes.

---

## 4. Configuration

The active tier is configured via environment variable `ZQK_SPECIALIZATION_TIER` (or baked into the binary at link time via `-ldflags "-X github.com/zqk-os/zqk/pkg/specialization.DefaultTier=neuron"`):

```bash
# Run node as a dedicated execution muscle
export ZQK_SPECIALIZATION_TIER=muscle
zqk scheduler daemon
```
