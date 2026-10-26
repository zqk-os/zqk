# Distributed Tracing, Requirement-to-Backlog Binding & Span Correlation Subsystem Operational Architecture Guide

## 1. Architectural Overview

The **Distributed Tracing, Requirement-to-Backlog Binding & Span Correlation Subsystem** (`pkg/tracing`) coordinates the association between high-level functional contracts (requirements) and active execution units (backlog items), providing strict lifecycle tracking and span correlation across the ZQK Core runtime (governed by POL-DEFAULT-142277d3).

In multi-agent swarms and continuous autonomous execution, traceability between specifications and operational tasks must adhere to foundational system invariants:
1. **Deterministic Binding Invariant:** Every active requirement entering an execution phase must either bind to a covering backlog item or transition deterministically into a parked state (`no_covering_bli`) to prevent untracked drift or phantom execution.
2. **Lifecycle Gate Isolation:** Only objects in active lifecycle states participate in live binding evaluations. Non-active entities (`draft`, `completed`, `parked`, `archived`) are segregated and preserved without mutation.
3. **Stateless Thread-Safe Orchestration:** The binding coordinator operates with zero shared mutable state, enabling high-throughput concurrent evaluations across distributed agent workers.
4. **End-to-End Span Correlation:** Bounded execution units maintain distributed tracing context, linking strategic goals, requirements, backlog tasks, and execution spans into an auditable graph.

---

## 2. Component Topology & Architecture

The tracing and binding subsystem is composed of focused, decoupled primitives:

```mermaid
flowchart TD
    subgraph InputSurface["Ontological Input Streams"]
        ReqStream["Requirement Stream\n(req: active / draft / completed)"]
        BliStream["Backlog Item Stream\n(backlog_item: active / in_progress)"]
    end

    subgraph CoreEngine["pkg/tracing Subsystem"]
        Validator["Lifecycle Status Evaluator\n(req.Status == 'active')"]
        Binder["Trace Binder\n(NewBinder & BindOrPark)"]
        OrphanResolver["Orphan Resolver\n(Parking Sentinel)"]
        OwnershipModel["Ownership Descriptor\n(ID, Kind, Status, ParkReason)"]
    end

    subgraph OutputTiers["Execution & Audit Sinks"]
        BoundSet["Bound Execution Queue\n(Active REQ ➔ Active BLI)"]
        ParkedSet["Parked Quarantine\n(Status: parked, Reason: no_covering_bli)"]
        SpanMesh["Distributed Span Mesh\n(Telemetry & Trace Context)"]
    end

    ReqStream --> Validator
    BliStream --> Binder
    Validator -->|Active Only| Binder
    Binder --> OwnershipModel
    Binder -->|Covered| BoundSet
    Binder -->|Uncovered| OrphanResolver
    OrphanResolver --> ParkedSet
    BoundSet --> SpanMesh
```

### Component Breakdown
- **`Ownership`:** The canonical lifecycle envelope representing an ontological object for binding. Captures the object identity (`ID`), entity kind (`Kind`), lifecycle state (`Status`), and quarantine annotation (`ParkReason`).
- **`Binder`:** The stateless binding engine. Instantiated via `NewBinder()`, it coordinates the assignment of active requirements to covering backlog items.
- **Lifecycle Status Evaluator:** Filters candidate inputs so that only active requirements proceed to binding allocation. Inactive states (`draft`, `in_progress`, `completed`, `parked`, `archived`) bypass allocation without generating spurious park events.
- **Orphan Resolver:** Intercepts active requirements that lack a corresponding backlog item, safely assigning `Status = "parked"` with `ParkReason = "no_covering_bli"`.
- **Span Correlator:** Maps bound requirement-backlog pairs into distributed execution spans across worker pools, preserving parent-child lineage in telemetry traces.

---

## 3. Requirement & Backlog Item Binding Lifecycle

The sequence below illustrates the evaluation of incoming requirement and backlog item streams during a work cycle:

```mermaid
sequenceDiagram
    autonumber
    participant Planner as Workflow Planner / Swarm
    participant Binder as Tracing Binder
    participant Queue as Active Execution Queue
    participant Quarantine as Parked Registry

    Planner->>Binder: BindOrPark(reqs, blis)
    loop For Each Requirement
        alt Status != "active"
            Binder->>Binder: Ignore / Passthrough (skip)
        else Status == "active"
            alt Covering BLI Available
                Binder->>Queue: Append Bound Requirement
            else No Covering BLI Available
                Binder->>Quarantine: Park Requirement (Reason: no_covering_bli)
            end
        end
    end
    Binder-->>Planner: return bound, parked
```

---

## 4. Ownership State Machine

The binding lifecycle transitions requirements between initial discovery, active matching, and quarantined parking:

```mermaid
stateDiagram-v2
    [*] --> Discovered: Object Created

    state Discovered {
        [*] --> Draft
        Draft --> Active: Activation Gate
    }

    state BindingEvaluation {
        [*] --> Evaluating
        Evaluating --> Bound: Covering BLI Available
        Evaluating --> Parked: No Covering BLI Available
    }

    Active --> BindingEvaluation: Submit to BindOrPark
    Bound --> InProgress: Execution Start
    InProgress --> Completed: Criteria Verified
    Parked --> Active: New BLI Provisioned
    Completed --> [*]
```

### State Semantics
- **Draft / Inactive:** Requirements undergoing definition or refinement. Excluded from runtime binding.
- **Active:** Validated requirements ready for execution. Must be evaluated against available backlog items.
- **Bound:** Paired with an active or in-progress backlog item and ready for worker assignment.
- **Parked (`no_covering_bli`):** Quarantined active requirements that have no available execution task. Held safely until backlog capacity or matching items are scheduled.

---

## 5. Concurrency & Thread-Safety Guarantees

The `Binder` implementation provides thread-safety and predictability:
- **Zero Mutable Shared State:** `Binder` struct contains no internal mutable fields or pointers. Any number of concurrent goroutines can safely invoke `BindOrPark` on a single shared instance or on distinct instances.
- **Immutability of Inputs:** Input slices (`reqs`, `blis`) are never mutated in-place. Returned `bound` and `parked` slices are newly allocated collections with copy-by-value `Ownership` structs.
- **Isolated Allocation:** `bound` and `parked` slices are allocated per invocation with sized capacity hints (`make([]Ownership, 0, len(reqs))`), avoiding memory leakage or contention.
- **Managed Concurrency Integration:** Background evaluation workers use `goroutinelabels.NewGoroutine` for concurrency tracing, ensuring proper labelling, stack isolation, and resource budgeting.

---

## 6. Verification & Three-Fold Proof Breakdown

The distributed tracing and binding subsystem is validated under the Three-Fold Proof:

| Dimension | Proof Type | Verification Target & Invariant | Status |
| :--- | :--- | :--- | :--- |
| **Static Floor** | Statement Coverage Floor | Statement test coverage strictly $\ge 85.0\%$ across all functions in `pkg/tracing` (achieved: 100.0%). | **VERIFIED** |
| **Operational Dynamic** | Functional Behavior Proof | `NewBinder`, `BindOrPark`, active/inactive status segregation, exact 1:1 binding, partial matching, empty collections, and 10k-item scaling. | **VERIFIED** |
| **Negative Boundary** | Adversarial Hardening | Zero-BLI handling, orphaned requirements parked with `no_covering_bli`, concurrent execution under race detectors, and public payload sanitization. | **VERIFIED** |

### Execution Commands
```bash
# Targeted test suite execution with coverage verification
ZQK_ALLOW_FOREGROUND_GO_TEST=1 go test -v -timeout 30s -cover ./pkg/tracing

# Verification engine suites
./bin/zqk-vet -suite hygiene,tree

# Open-core release payload verification
sh scripts/open-core/check-public-release-payload.sh .
```
