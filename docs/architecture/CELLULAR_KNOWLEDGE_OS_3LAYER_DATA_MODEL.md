# Cellular Knowledge Operating System: Three-Layer Data Model & Primitives Specification

**Version:** 1.0.0  
**Status:** Approved  
**Date:** 2026-09-14  
**Authors:** ZQK Architecture Working Group  
**Tracking Objects:**
- `PRI-CELLULAR-MICROKERNEL-REVISION-001`
- `REQ-CELLULAR-DNA-PRIMITIVES-001`
- `REQ-CELLULAR-PM-REANCHOR-002`
- `REQ-CELLULAR-KERNEL-DECOUPLING-003`
- `REQ-CELLULAR-CODEGEN-QUARANTINE-004`
- `TSP-CELLULAR-3LAYER-DATA-MODEL-001`
- `TSP-CELLULAR-TRACEABILITY-CHAIN-001`
- `TSP-CELLULAR-CODEGEN-QUARANTINE-001`

---

## 1. Executive Summary

Autonomous multi-agent swarms cannot scale on monolithic data silos, uncurated vector dumps, or ephemeral prompt scripts. When an agent hallucinates or mutates state uncontrolled, entire shared contexts rot.

ZQK addresses this at the operating system level by modeling multi-agent computation as a **distributed biological compute substrate**:
1. **The Cell (ZQK Node)**: A sovereign microkernel maintaining authoritative stewardship over its local environment and private knowledge graph.
2. **The Membrane (Deterministic Boundaries)**: A strict multi-plane state machine (`PlaneDraft` → `PlaneStaged` → `PlanePromoted` → `PlaneApoptotic`) ensuring only verified state mutates reality.
3. **The Nervous System (Active Graph)**: An operational state bus that governs task lifecycles, lineage, and scheduling rather than passive query lakes.
4. **The Organism (ZQK Mesh)**: A peer-to-peer network of specialized cellular kernels coordinating over a typed wire protocol to execute compound objectives without moment-to-moment human oversight.

---

## 2. Structural Topology: Substrate, Nucleus, and Lateral Sovereign Cells

A common misconception is that domain objects (like Project Management or SecOps) sit in a vertically stacked "Layer 3" on top of Core Kernel objects ("Layer 2"). In reality, in the Cellular Knowledge OS:
- **Distance = 0 (The Nucleus)**: The universal `BaseEntity` (`BaseObject` + `Auditable`).
- **Distance = 1 (Lateral Sovereign Cells)**: Both open-core kernel archetypes (`Account`, `Policy`, `SchedulerJob`, `BaseMetric`, `AuditEvent`) and domain objects (`Epic`, `BacklogItem`, `Goal`, `TestCase`) are **first-order specializations at the exact same distance (1 hop) from the base nucleus**.

They do not inherit from one another; they branch laterally into distinct sovereign cell partitions (`zqk:kernel`, `zqk:project-mgmt`, `zqk:secops`):

```
                                +-------------------------------------------+
                                |  SUBSTRATE: MICROKERNEL ENGINE (pkg/kernel)|
                                |  - Transactional Plane Boundary Manager   |
                                |  - Cryptographic CAS Content Store        |
                                |  - MetaSchema Invariant Membrane Engine   |
                                +-------------------------------------------+
                                                      │
                                                      ▼ hosts
                                +-------------------------------------------+
                                |  NUCLEUS (Distance = 0): BaseEntity       |
                                |  (BaseObject + Auditable + MacroPhases)   |
                                +-------------------------------------------+
                                      │                              │
          ┌───────────────────────────┴──────────────────────────────┴───────────────────────────┐
          │                                                                                       │
          ▼ extends (Distance = 1)                                                                ▼ extends (Distance = 1)
+---------------------------------------------+                         +---------------------------------------------+
| SOVEREIGN CELL: zqk:kernel                  |                         | SOVEREIGN CELL: zqk:project-mgmt            |
| (Open-Core Kernel Substrates)               |                         | (Project Management & Execution Substrates) |
| - Identity:    Account, Role, Policy        |                         | - Scopes:     Epic, PriorityPlan, Milestone |
| - Automation:  SchedulerJob, HandlerBinding |                         | - Execution:  BacklogItem, AgentTask        |
| - Telemetry:   BaseMetric (Stream Telemetry)|                         | - Assurance:  Requirement, Criteria, TestCase|
| - Audit:       AuditEvent, ChangeJournal    |                         | - Bridges:    WorkInterval, Occupancy Mixins|
| - Topology:    Namespace, DomainRegistry    |                         +---------------------------------------------+
+---------------------------------------------+                                                │
                                                                                               ▼ extends (Distance = 1)
                                                                        +---------------------------------------------+
                                                                        | SOVEREIGN CELL: zqk:secops / zqk:audit      |
                                                                        | - Advisory, VulnerabilityScan, LedgerEntry  |
                                                                        +---------------------------------------------+
```

### The Substrate: Microkernel Engine (`pkg/kernel`)
Domain-agnostic Go runtime providing:
- **Physical Plane Isolation**: Strict physical separation between working copy (`PlaneDraft`), candidate state (`PlaneStaged`), authoritative truth (`PlanePromoted`), and quarantined state (`PlaneApoptotic`).
- **Cryptographic CAS**: Content-addressable storage where object state hashes enforce immutability.
- **Membrane Admission**: Verifies invariant predicates before state is permitted to cross planes.

### The Nucleus (Distance = 0): `BaseEntity`
The minimal universal foundation that ALL cellular entities inherit:
- **Canonical Address**: `URN` (`urn:zqk:<cell>:<kind>:<id>`).
- **Structural Identity**: `ID`, `Kind`, `NamespaceID`.
- **Governing Schema**: `SchemaRef` (`urn:zqk:spec:<cell>:<kind>`) and `SchemaVersion`.
- **Operational Boundary**: `Plane` (`PlaneDraft`, `PlaneStaged`, `PlanePromoted`, `PlaneApoptotic`).
- **Concurrency & Versioning**: Monotonic `Version` counter and deterministic timestamps (`CreatedAt`, `UpdatedAt`).
- **Epistemic State**: `Status` (mapped to universal `MacroPhase`) and `StatusHistory`.
- **Cryptographic Provenance**: Embedded `Auditable` vector (`ParentHash`, `AgentID`, `ModelHash`, `Signature`, `Hash`).

> [!IMPORTANT]
> **Paring Down the Nucleus**: The universal `BaseEntity` contains ZERO task or planning clutter (`deadline`, `target_date`, `priority_tier`, `questions`, `artifacts`, `dependencies`, `stakeholders`). These belong exclusively in the `WorkInterval` and `TaskPlanning` mixins composed by execution entities (like `BacklogItem` and `AgentTask`).

### The Lateral Peer Cells (Distance = 1)
1. **Kernel Cell (`zqk:kernel`)**:
   - `Account` (`ACC-*`): Sovereign actor identity, agent seating, human operators.
   - `Role` (`ROL-*`): Permission matrix, capabilities, and execution scopes.
   - `Policy` (`POL-*`): Invariant constraints, admission gates, and membrane rules.
   - `SchedulerJob` (`SCH-*`): Cron/event-driven background task definitions and recurrence.
   - `BaseMetric` (`MET-*`): High-frequency telemetry, performance measurements, and latency gauges (`storage_profile: stream`).
   - `AuditEvent` (`AUD-*`): Immutable, non-repudiable audit logs of transitions and tool executions.
   - `ChangeJournalEntry` (`CJE-*`): Transactional CAS mutation deltas.
   - `Namespace` (`NSP-*`): Sovereign cell and partition boundary (`zqk:kernel`, `zqk:project-mgmt`).
2. **Project Management Cell (`zqk:project-mgmt`)**:
   - `Epic`: Cellular objective and enclave scope container.
   - `PriorityPlan`: Bounded iteration execution column.
   - `Milestone`: Temporal synchronization checkpoint.
   - `BacklogItem`: Atomic unit of work execution.
   - `Requirement`, `Criteria`, `TestCase`: Traceability spine from intent to automated attestation.
3. **Other Specialized Domain Cells (`zqk:secops`, `zqk:audit`)**:
   - Equal peers extending the same base nucleus for security, financial, and compliance domains.

---

## 3. Foundational DNA Sector Primitives

```
                +----------------------------------+
                |           MetaSchema             | <-- Invariant & Field Vetting Engine
                +----------------------------------+
                                 │ verifies
                                 ▼
+---------------------------------------------------------------------------------+
|                                  BaseObject                                     |
| - URN / UUIDv7 (Canonical Identity)                                             |
| - Kind & SchemaVersion                                                          |
| - Plane (PlaneDraft | PlaneStaged | PlanePromoted | PlaneApoptotic)             |
| - Version, CreatedAt, UpdatedAt                                                 |
+---------------------------------------------------------------------------------+
       ▲                                                          ▲
       │ implements                                               │ implements
+-----------------------------+                    +-----------------------------+
|       Auditable Trait       |                    |       Lifecycle Trait       |
| - Cryptographic Provenance  |                    | - Discrete State Machine    |
| - Agent Attestation Vector  |                    | - Invariant Gate Hooks      |
| - Parent / Causal Hashes    |                    | - Apoptosis / TTL Policy    |
+-----------------------------+                    +-----------------------------+
```

### Primitive A: `BaseObject` (The Structural Nucleus)
Every entity in the kernel embeds `BaseObject`:
```go
package dna

import "time"

type Plane uint8

const (
    PlaneDraft Plane = iota      // Scratchpad/worktree space. Zero blast radius.
    PlaneStaged                 // Candidate state waiting for gate attestation.
    PlanePromoted               // Authoritative cellular reality. Immutable.
    PlaneApoptotic              // Quarantined or scheduled for deletion.
)

type BaseObject struct {
    URN         string    `json:"urn" yaml:"urn"`                 // urn:zqk:<cell_id>:<kind>:<ulid_or_uuidv7>
    Kind        string    `json:"kind" yaml:"kind"`               // Registered MetaSchema identifier
    SchemaRef   string    `json:"schema_ref" yaml:"schema_ref"`   // Content-addressed hash of governing MetaSchema
    Plane       Plane     `json:"plane" yaml:"plane"`             // Active operational boundary
    Version     uint64    `json:"version" yaml:"version"`         // Monotonic revision counter
    CreatedAt   time.Time `json:"created_at" yaml:"created_at"`   // Deterministic timestamp
    UpdatedAt   time.Time `json:"updated_at" yaml:"updated_at"`
}
```

### Primitive B: `Auditable` Trait (The Immune & Provenance Chain)
Ensures zero anonymous mutations across the swarm:
```go
type Provenance struct {
    ParentHash string            `json:"parent_hash" yaml:"parent_hash"` // Parent state hash / causal vector
    AgentID    string            `json:"agent_id" yaml:"agent_id"`       // Identity of acting agent
    ModelHash  string            `json:"model_hash,omitempty"`          // Model/runtime executable hash
    Signature  string            `json:"signature" yaml:"signature"`     // Cryptographic signature
    Metadata   map[string]string `json:"metadata,omitempty"`
}

type Auditable interface {
    GetProvenance() Provenance
    ComputeDigest() (string, error)
}
```

### Primitive C: `LifecycleFacet` (Universal Metabolic State Machine)
Replaces arbitrary string status updates with a two-tier Hierarchical State Machine (HSM):
- **Universal Macro-Phases**:
  - `MacroPhaseGestation` (`PlaneDraft`): Inception, conceptualization, drafting (`conceptual`, `draft`, `proposed`).
  - `MacroPhaseMetabolism` (`PlaneStaged`): Active execution (`active`, `in_progress`).
  - `MacroPhaseAttestation` (`PlaneStaged`): Test execution and invariant verification (`testing`, `metrics_captured`, `verifying`).
  - `MacroPhaseEquilibrium` (`PlanePromoted`): Complete, validated, authoritative state (`complete`, `validated`, `approved`).
  - `MacroPhaseRegression` (`PlanePromoted`): Graduated sentinel suites defending against regression.
  - `MacroPhaseApoptosis` (`PlaneApoptotic`): Retirement, quarantine, or archival (`archived`, `rejected`, `error`).
- **Domain Sub-Status Retention**: Existing kind lifecycles (`backlog_item_lifecycle.yaml`, `test_case_lifecycle.yaml`, etc.) are retained in full as domain sub-states mapped onto these core macro-phases.

### Primitive D: `TraceabilityFacet` & `AttestationFacet`
- **`TraceabilityFacet`**: Tracks the unbroken upward lineage from an entity to its root Intent (`GOAL`, `PRI`, `ROAD`). Enforces that an entity cannot exit Gestation to Metabolism without `IsIntact == true`.
- **`AttestationFacet`**: Encapsulates invariant criteria and execution verdicts, governing graduation into Equilibrium.

### Primitive E: `MetaSchema` (The Membrane & Vetting Engine)
Active gatekeeper that vets structural types and dynamic epistemic invariants before data enters the cell:
- **Structural Typing**: Scalar, enum, graph edges, embedded sub-traits.
- **Sanitization Rules**: Regex, boundary, and encoding assertions.
- **Epistemic Invariants**: Semantic assertions evaluated dynamically across related entities.
- **Sub-State Mapping**: Maps each domain kind's sub-statuses to their governing MacroPhase and Plane.
- **Self-Describing**: MetaSchema itself embeds `BaseObject` and implements `Auditable`.

---

## 4. Requirement-Level Test Case & Criteria Traceability Chain

```
[ Epic / Intent ]
       │
       ▼
[ Requirement / Workstream ] ◄────────────────────────┐
       │                                              │
       ├──────────────────────────┐                   │ (requirement_refs)
       ▼                          ▼                   │
[ Backlog Item (WorkUnit) ]   [ Test Case (Attestation Suite) ]
       │                                  │
       │ (criteria_refs)                  │ (criteria_refs & verification_suites)
       ▼                                  ▼
[ Acceptance Criteria (Invariant Gates) ] ┘
```

1. **The Test Case Container Role**:
   - Individual Acceptance Criteria (`criteria`) are discrete, non-negotiable assertions.
   - A `test_case` binds related criteria together at the **requirement level** into an automated execution suite.
   - `test_case.criteria_refs`: Lists all criteria validated by the suite.
   - `test_case.requirement_refs`: Explicitly maps the test suite to the governing requirement.
   - `test_case.verification_suites`: Maps each criterion to its executable command.
2. **Attestation & Verdict**:
   - Agents cannot self-certify completion.
   - Test execution generates a deterministic `Verdict`.
   - `InvariantGate` verifies the verdict before allowing state transition to `PlanePromoted`.

---

## 5. CQRS Materialized Projection: Test Dashboard Lite-File

To eliminate heavy filesystem walks and graph traversal overhead during terminal observation:
1. **Materialized Lite-File (`.zqk/state/test_dashboard_lite.json`)**:
   - Holds a zero-cost, pre-calculated snapshot of the test matrix, intact lineages, and recent events.
   - Loaded in `<2ms` by `zqk test dashboard`, providing instant visual feedback.
   - Updated incrementally as WAL events arrive or when `zqk test run` toggles verification widgets.
   - Re-materialized on-demand with `--refresh` (`-r`).
2. **TPM Definition of Done (DoD) & Quick-Bind Assistant**:
   - **DoD Rule**: The structure is DONE when the chain displays intact (`✓ Chain Intact`) from test case to root object.
   - **Verification Gate**: `zqk test dashboard --check-dod` asserts 100% active chain intactness and fails closed if broken.
   - **Assistant Tool**: `zqk test bind` analyzes incomplete chains and surfaces limited, ranked options for one-command resolution, or auto-binds with `zqk test bind --auto`.

---

## 6. In-Tree Package Layout & Codegen Quarantine

```
zqk/
├── cmd/
│   └── zqk/              # Main CLI entrypoint
├── pkg/
│   ├── kernel/           # LAYER 1: Core engine (Graph, Planes, IPC, Scheduler)
│   ├── dna/              # LAYER 2: Core governance schema (BaseObject, Auditable, Lifecycle)
│   │   └── pm/           # Standard DNA Library bindings (Intent, WorkUnit, Gate, Verdict)
│   └── profiles/
│       └── software/     # LAYER 3: Existing PM/Dev lifecycle specializations
└── internal/
    └── codegen/          # PROPRIETARY: Spec-driven codegen & AST compilers
```
- **Public DNA Contract (`pkg/dna`)**: Declarative schemas and contracts remain public so external cells and agents know how to construct valid state.
- **Codegen Quarantine (`internal/codegen`)**: Proprietary AST synthesizers, macro builders, and compiler pipelines are quarantined within `internal/codegen`, consuming public schemas without leaking compiler internals into the open core.

---

## 7. Specification Layer & DNA Alignment

To bridge the Cellular Knowledge OS primitives with the YAML specification system:
1. **Metaschema (`.zqk/cli/specs/schemas/object_spec.schema.json`)**:
   - Formally supports `urn` (`pattern: ^urn:zqk:spec:[a-zA-Z0-9_\-]+:[a-zA-Z0-9_\-]+$`) and `namespace` (`pattern: ^(zqk|domain|integration):[a-z0-9_]+(:[a-z0-9_]+)*$`) on specification definitions.
   - `SpecLoader` exposes `spec.URN` and `spec.Namespace` alongside ontology and version metadata.
2. **`auditable.yaml` (`urn:zqk:spec:kernel:auditable`)**:
   - Defines the `provenance` field (`AUD-010`, object containing `parent_hash`, `agent_id`, `model_hash`, `signature`, `hash`, `metadata`), aligning directly with `dna.Provenance` and `dna.Auditable`.
   - Maintains legacy operational timestamps (`created_at`, `created_by`, `updated_at`, `updated_by`, `archived_at`, `archived_by`, `change_log`) for full backward compatibility across all 9,877 existing CAS objects.
3. **`base_object.yaml` (`urn:zqk:spec:kernel:base_object`)**:
   - Formally declares cellular base properties:
     - `urn` (`BSE-023`, identifier, `urn:zqk:<cell>:<kind>:<id>`)
     - `schema_ref` (`BSE-024`, reference, `urn:zqk:spec:<cell>:<kind>`)
     - `plane` (`BSE-025`, enum: `draft`, `staged`, `promoted`, `apoptotic`)
     - `version` (`BSE-026`, monotonic uint64 revision counter)
   - Ensures that every system entity inherits both cryptographic provenance and cellular boundary attributes while preserving existing task and planning fields (`priority_tier`, `deadline`, `target_date`, etc.).

---

## 8. Foundational Lifecycles & Behavioral Traits vs Bolted-on Primitives

### A. Foundational Lifecycles: The Metabolic Kernel State Machine
Historically, lifecycles were often treated as bolted-on YAML files (`*_lifecycle.yaml`) defining arbitrary strings (`in_progress`, `approved`, `active`) evaluated by simple lookup tables. In the Cellular Knowledge OS, lifecycles are **not bolted-on string checkers; they are the operating system's foundational process states**:

1. **Deterministic Plane Mapping**:
   Every domain sub-status deterministically maps into a universal `MacroPhase` and a physical transactional boundary `Plane`:
   - `Gestation` (`PlaneDraft`): Entity creation, formulation, drafting. Zero blast radius.
   - `Metabolism` (`PlaneStaged`): Active work, task execution, data mutation.
   - `Attestation` (`PlaneStaged`): Invariant evaluation, test execution, criteria attestation.
   - `Equilibrium` (`PlanePromoted`): Completed, verified, authoritative truth.
   - `Regression` (`PlanePromoted`): Graduated test suite actively defending system baseline.
   - `Apoptosis` (`PlaneApoptotic`): Programmed cell death, TTL expiration, or archival.
2. **Invariant-Guarded Plane Transitions**:
   State cannot cross planes via raw database writes. Plane transitions are guarded by `VetTransition`:
   - Transitioning into `PlaneStaged` mandates Actor Attestation (`AgentID`).
   - Transitioning into `PlanePromoted` mandates Causal Lineage (`ParentHash`) and all invariant gates passing green.

### B. Foundational Traits: Behavioral Capabilities vs UI Display Tags
Historically, traits like `listable`, `searchable`, `sortable`, `readable`, `writable` were bolted-on display flags describing CLI presentation. In the Cellular Knowledge OS, traits are **foundational behavioral contracts**:

| Trait | Category | Runtime Guarantee & Invariant Contract |
| :--- | :--- | :--- |
| **`AuditableTrait`** | Provenance | Cryptographic non-repudiation, causal DAG parent hashes, content digests (`GetProvenance`, `ComputeDigest`). |
| **`OccupiableTrait`** | Concurrency | Exclusive agent seat leasing (`ClaimedBy`, `ClaimedAt`, `LeaseTTL`). Prevents dual-agent execution conflicts. |
| **`TraceableTrait`** | Lineage | Unbroken directed acyclic graph from leaf entity up to root Intent/Epic (`IsIntact == true`). |
| **`AttestableTrait`** | Verification | Verifiable criteria contracts and test verdicts. Guards promotion out of `PlaneStaged`. |
| **`ApoptoticTrait`** | Memory Mgmt | Automatic lease reclamation, self-healing decay, and orderly resource cleanup. |
| **`StreamableTrait`** | Telemetry | High-throughput, non-blocking telemetry and event streams (`storage_profile: stream`). |

### C. Foundational Telemetry: BaseMetric as an Epistemic Peer
Metrics are not an auxiliary tool; they are a first-class archetype in the kernel cell (`zqk:kernel:base_metric`):
- Measures execution latency, memory pressure, scheduler lock wait times, and failure frequencies.
- Evaluated by `InvariantGate` predicates to trigger automated self-healing or backpressure before system health degrades.

---

## 9. The MetaSchema as the Structural Shape Blueprint

A pivotal architectural question is: **Does the MetaSchema contain the structural shape of entities?**
**Yes, absolutely.** Per the Open-Core Microkernel architecture and foundational schema theory:

```
┌────────────────────────────────────────────────────────────────────────┐
│                              MetaSchema                                │
│       (Declarative Blueprint: Structural Shape, Types & Rules)         │
│  - TargetKind, Extends, Composes, StorageProfile, KernelCritical       │
│  - FieldRules (Types, Required, Min/Max Length, Pattern, Enum, Default)│
│  - AllowedPlanes, Invariants, Behavioral Traits, MacroPhase Mappings   │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │ governs & vets instances
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│                              BaseEntity                                │
│                = BaseObject + Auditable (Distance = 0)                 │
│  - Identity: URN, Kind, SchemaRef, Plane, Version                      │
│  - Provenance: ParentHash, AgentID, ModelHash, Signature, Hash         │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │ specialized laterally (Distance = 1)
                  ┌─────────────────┴─────────────────┐
                  ▼                                   ▼
┌───────────────────────────────────┐ ┌───────────────────────────────────┐
│     Sovereign Cell: zqk:kernel    │ │ Sovereign Cell: zqk:project-mgmt  │
│  - Account, Role, Policy          │ │  - Epic, PriorityPlan, Milestone  │
│  - SchedulerJob, BaseMetric       │ │  - BacklogItem, Requirement       │
│  - AuditEvent, ChangeJournalEntry │ │  - TestCase, Scenario             │
└───────────────────────────────────┘ └───────────────────────────────────┘
```

1. **Authoritative Structural Descriptor**:
   - The **MetaSchema** (`dna.MetaSchema`) is the authoritative gatekeeper that declares the **structural shape** of an entity kind.
   - It specifies:
     - `FieldRules`: Detailed attribute definitions including data types (`string`, `int`, `bool`, `list`, `map`, `urn`, `object`, `enum`), mandatory vs optional requirements, regex validation patterns, string length bounds, and enumerated value sets.
     - `AllowedPlanes`: The physical lifecycle planes (`PlaneDraft`, `PlaneStaged`, `PlanePromoted`, `PlaneApoptotic`) the entity is allowed to occupy.
     - `Invariants`: Epistemic invariants that must evaluate true before plane promotion.
     - `Traits`: Foundational behavioral contracts (`auditable`, `occupiable`, `traceable`, `attestable`, `streamable`, `apoptotic`).
     - `MacroPhases`: Mapping of domain sub-statuses into the universal metabolic phases (`Gestation`, `Metabolism`, `Attestation`, `Equilibrium`, `Regression`, `Apoptosis`).
2. **Declarative Serialization & Hydration**:
   - The YAML object specifications (`.zqk/specs/objects/*.yaml`) are the declarative serialization of the MetaSchema.
   - `objects.Spec` loaded from disk hydrates into `dna.MetaSchema` via `spec.ToMetaSchema()`, enabling zero-cost runtime compilation of membrane rules into active Go memory.
3. **Membrane Structural Vetting (`VetObject`)**:
   - Before an object can be staged or written into the Content-Addressable Store (CAS), the membrane evaluates `MetaSchema.VetObject(data)`.
   - Any missing required fields, type mismatches, pattern violations, or unlisted enum values are rejected at the membrane boundary before corrupting graph state.

---

## 10. Lite-File Event Stream & Near-Realtime Dashboard Architecture

A critical operational requirement for multi-agent swarms is dashboard observability without memory bloat:
> *"The dashboard should load a static file that gets updated as events transpire — the lite-file use case. It doesn't need to be immediate; near-realtime is sufficient. The user can refresh to see new events. We do NOT want a heavy graph structure sucked into memory to display current status; that's a slow block-and-wait approach. We treat it like an event stream that toggles widgets."*

To satisfy this requirement:

1. **Lightweight Delta Mutation Stream (`DeltaEvent`)**:
   - When an entity undergoes a status transition or phase promotion, it does NOT trigger a full repository graph rebuild.
   - Instead, the mutation emits a compact `DeltaEvent` containing:
     - `EventID`, `Timestamp`, `EntityURN`, `Kind`, `FromPhase`, `ToPhase`, `SubStatus`, `Actor`, and `Summary`.
2. **Near-Realtime Static Projection (`DashboardProjection`)**:
   - Delta events are applied via `AppendDeltaToStreamFile` to an atomic static JSON projection file (`.zqk/state/dashboard_projection.json`).
   - The projection maintains:
     - `GeneratedAt`: UTC timestamp of latest event.
     - `PhaseDistribution`: O(1) counters for each `MacroPhase` (`Gestation`, `Metabolism`, `Attestation`, `Equilibrium`, `Regression`, `Apoptosis`).
     - `ActiveChains`: High-level TPM execution chains (e.g. Plan → Milestone → BacklogItem → Task) showing which nodes are active.
     - `RecentDeltas`: Sliding window of the last 50 events.
3. **O(1) Dashboard Refresh & Zero Memory Bloat**:
   - The dashboard UI loads the static JSON file in O(1) time (< 5ms) without instantiating or parsing the 9,800+ CAS objects in memory.
   - When a TPM or operator inspects the dashboard, widget toggles are driven by this event stream.
   - The display of the completed chain serves as the verifiable **Definition of Done** for the TPM, with easy, constrained options rather than deep graph manual assembly.

---

## 11. Complete Object Evolution Chain

The complete object evolution chain across the Cellular Knowledge OS is defined as follows:

| Layer / Altitude | Construct | Role & Responsibilities | Location |
| :--- | :--- | :--- | :--- |
| **Meta Layer** | `MetaSchema` | Authoritative blueprint defining **structural shape**, field rules, regex patterns, enums, allowed planes, behavioral traits, and metabolic macro-phase mappings. | `pkg/dna/membrane.go`, `.zqk/specs/objects/` |
| **Distance = 0 (Nucleus)** | `BaseEntity` (`BaseObject` + `Auditable`) | Minimal universal foundation. Canonical URN addressing, plane boundary, monotonic versioning, status history, and cryptographic provenance sealing (`ParentHash`, `AgentID`, `Signature`). | `pkg/dna/primitives.go` |
| **Distance = 1 (Lateral Cells)** | Sovereign Peer Cells (`zqk:kernel`, `zqk:project-mgmt`) | First-order specializations hanging directly off `BaseEntity`. `zqk:kernel` defines open-core system archetypes (Account, Role, Policy, SchedulerJob, BaseMetric, AuditEvent). `zqk:project-mgmt` defines execution archetypes (Epic, PriorityPlan, Milestone, BacklogItem, Requirement, TestCase). | `pkg/dna/kernel/`, `pkg/dna/pm/` |
| **Mixins & Facets** | `WorkInterval`, `Occupancy`, `TraceabilityFacet` | Composable behavioral facets. Task and planning fields (`deadline`, `priority_tier`, `artifacts`, `claimed_by`) are composed only into execution entities, leaving kernel archetypes unpolluted. | `pkg/dna/primitives.go`, `.zqk/specs/objects/work_unit.yaml` |
| **Behavioral Traits** | `Auditable`, `Occupiable`, `Traceable`, `Attestable`, `Apoptotic`, `Streamable` | Native runtime contracts evaluated during plane and phase transitions. | `pkg/dna/traits.go` |
| **Operational Stream** | `DeltaEvent`, `DashboardProjection` | Append-only delta mutation stream projecting near-realtime execution chains into a static lite file for instant dashboard rendering. | `pkg/dna/event_stream.go` |

