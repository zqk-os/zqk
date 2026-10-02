# Onboarding Roadmap & Complete Ontological Cascade

This directory defines the canonical, end-to-end ontological onboarding chain for the **Agent & User Onboarding** domain. In ZQK, onboarding curriculum is first-class, content-addressed system data rather than transient documentation or tribal knowledge.

## Ontological Architecture: The Five-Layer Cascade

This curriculum exemplifies the complete five-layer cascade defined in `community-ontological-architecture`:

```
Layer 1: Strategic Intent     [goal: GOAL-onboarding]
                                   │
Layer 2: Execution Spine       [workstream: WS-onboarding] ── [priority_plan: PRIO-onboarding]
                                   │
Layer 3: System Capability     [requirement: REQ-onboarding-foundation]
                                   │
Layer 3: Three-Fold Proof      ├── [criteria: CRIT-onboarding-philosophy-mastery] (Static Invariant)
                               ├── [criteria: CRIT-onboarding-system-health]      (Dynamic Behavior)
                               └── [criteria: CRIT-onboarding-failclosed-boundary] (Negative Invariant)
                                   │
Layer 4: Verification          ├── [test_case: TST-onboarding-health-check]
                               └── [test_case: TST-onboarding-mutation-boundary]
                                   │
Layer 5: Phased Actions        [milestone: MIL-onboarding]
                                   ├── [backlog_item: BLI-onboarding-read-philosophy]
                                   ├── [backlog_item: BLI-onboarding-start-here]
                                   └── [backlog_item: BLI-onboarding-system-health]
```

### The Three-Fold Proof Formula

To set a rigorous example, requirement satisfaction is not 1:1 with a single criteria node. Every requirement defines at least 3 orthogonal criteria:

1. **State Invariant (Static Floor)**: `CRIT-onboarding-philosophy-mastery` — verifies comprehension of core architectural tenets (event-driven architecture, declarative specifications, deterministic CAS content-addressing, acyclic graph edge ownership).
2. **Dynamic Behavior (Operational Proof)**: `CRIT-onboarding-system-health` — programmatic execution where `zqk system check` exits 0 with zero errors across CAS storage, schema registry, WAL streaming, and git hooks.
3. **Negative Invariant (Adversarial Boundary)**: `CRIT-onboarding-failclosed-boundary` — confirms unauthenticated mutations, missing mandatory fields, or writes bypassing the CAS membrane fail closed deterministically.

---

## 1. Primary Seed Flow: Declarative ZQL Transaction

The entire multi-object constellation is seeded in a single atomic ACID transaction via Kahn topological ordering:

```bash
zqk object mutate --file scripts/onboarding_roadmap/onboarding_seed.zql
```

*(Automated execution is handled by `scripts/scheduler_jobs/onboarding_roadmap_seed.yaml`)*.

---

## 2. High-Level Creation Flow (`new object`)

When creating individual onboarding nodes manually, use the high-level draft plane and promote workflow rather than raw low-level file imports:

### Minting onto the Draft Plane

```bash
# 1. Mint requirement (scaffolds trace pipeline)
zqk new object requirement --title "Establish agent and operator onboarding baseline"

# 2. Mint criteria
zqk new object criteria --title "Kernel subsystems pass comprehensive health checks"

# 3. Mint backlog items
zqk new object backlog_item --title "Read Operational Philosophy policy"
```

### Promoting Along the Lifecycle

Once fields and references are populated on `.zqk/object_drafts/<ID>.yaml`:

```bash
zqk object promote <OBJECT-ID>
```

---

## 3. Direct Template Scaffolds (Declarative YAML)

The YAML templates in this directory provide pre-populated schemas for reference or direct CAS staging:

- `goal_onboarding.yaml` (`GOAL-onboarding`)
- `workstream_onboarding.yaml` (`WS-onboarding`)
- `priority_plan_onboarding.yaml` (`PRIO-onboarding`)
- `requirement_onboarding.yaml` (`REQ-onboarding-foundation`)
- `criteria_onboarding_philosophy.yaml` (`CRIT-onboarding-philosophy-mastery`)
- `criteria_onboarding_system_health.yaml` (`CRIT-onboarding-system-health`)
- `criteria_onboarding_failclosed_boundary.yaml` (`CRIT-onboarding-failclosed-boundary`)
- `test_case_onboarding_health.yaml` (`TST-onboarding-health-check`)
- `test_case_onboarding_boundary.yaml` (`TST-onboarding-mutation-boundary`)
- `milestone_onboarding.yaml` (`MIL-onboarding`)
- `backlog_item_01_read_philosophy.yaml` (`BLI-onboarding-read-philosophy`)
- `backlog_item_02_start_here.yaml` (`BLI-onboarding-start-here`)
- `backlog_item_03_system_health.yaml` (`BLI-onboarding-system-health`)

*(Low-level CLI primitive: `zqk object create <kind> --file <path>`)*

---

## Discovery & Querying

- **List onboarding backlog items**:
  ```bash
  zqk object list backlog_item --filter priority_plan_ref=PRIO-onboarding
  ```
- **Inspect complete requirement lineage**:
  ```bash
  zqk object get REQ-onboarding-foundation
  ```
- **Traverse graph relationships**:
  ```bash
  zqk object related REQ-onboarding-foundation
  ```
