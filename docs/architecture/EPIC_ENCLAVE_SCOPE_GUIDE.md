# Epic Object Architecture: Cellular Objective & Enclave Scope Guide

**Document ID**: DOC-EPIC-ENCLAVE-SCOPE-GUIDE  
**Version**: 1.0.0  
**Ontology Object**: `epic` (`urn:zqk:spec:project_mgmt:epic`)  
**Status**: Authoritative  

---

## 1. Why Epics Exist

In the Cellular Knowledge OS architecture, autonomous swarms and human builders operate concurrently across high-velocity codebases. Without strict architectural blast-radius controls, task execution tends to leak mutations across package boundaries, introducing silent regressions and architectural entropy.

The **Epic** (`EPC-###`) exists as a **Cellular Objective / Enclave Scope Container**. It is not merely an agile grouping mechanism; it is a first-class transactional boundary in the knowledge graph that:

1. **Defines the Enclave Scope (`enclave_scope`)**: Restricts the architectural blast radius of member work units to explicitly declared packages, directories, and URN namespaces.
2. **Enforces Invariant Promotion Gating**: Blocks transition to authoritative equilibrium (`PlanePromoted` / `complete`) until all constituent child work units (`BLI-####`) have verified their own invariant gates and reached completion.
3. **Cryptographic Provenance Lineage**: Binds high-level strategic goals (`GOAL-####`) to verifiable commit attestation digests.

```
+---------------------------------------------------------------+
|                      Goal (GOAL-####)                         |
+---------------------------------------------------------------+
                               |
                               v
+---------------------------------------------------------------+
|                   Workstream (WS-###)                         |
+---------------------------------------------------------------+
                               |
                               v
+===============================================================+
|                   EPIC (EPC-###)                              |
|   enclave_scope: "pkg/kernel pkg/dna pkg/pm"                  |
|   promotion_gate: CanPromote(ctx) -> all BLIs complete       |
+===============================================================+
          |                                       |
          v                                       v
+-----------------------+               +-----------------------+
|  BacklogItem (BLI-1)  |               |  BacklogItem (BLI-2)  |
|  Status: complete     |               |  Status: complete     |
|  Plane: PlanePromoted |               |  Plane: PlanePromoted |
+-----------------------+               +-----------------------+
```

---

## 2. When to Use: Epic vs. Workstream vs. Milestone

| Dimension | Workstream (`workstream`) | Milestone (`milestone`) | Epic (`epic`) |
| :--- | :--- | :--- | :--- |
| **Ontology Kind** | `workstream` (`WS-###`) | `milestone` (`MIL-###`) | `epic` (`EPC-###`) |
| **Primary Scope** | Long-running organizational domain or execution track. | Time-bound checkpoint or release phase gate. | Bounded enclave subsystem or compound capability. |
| **Enclave Isolation**| Broad (cross-cutting ownership). | Temporal (all work scheduled for a release). | **Strict Spatial Boundary** (`enclave_scope`). |
| **Promotion Gate** | Workstream state transitions track health. | Release checklist verification. | **Automated Invariant Gate**: All member work units must clear invariant gates before Epic complete. |
| **Typical Lifetime**| Months to years (e.g. `WS-CELLULAR-MICROKERNEL-001`). | Sprints or quarters (e.g. `MIL-CELLULAR-MICROKERNEL-001`). | Weeks to sprint cycle (e.g. `EPC-001`). |

### Decision Rule
- Use **Workstream** to categorize *where* in the organizational architecture work lives.
- Use **Milestone** to define *when* deliverables synchronize against releases.
- Use **Epic** to bound *what* packages and files can be modified and *guarantee* that compound invariants clear before the collective capability is marked done.

---

## 3. How Epics Work: Technical Architecture

### 3.1 Schema & Nucleus Composition
Every Epic instance in the Knowledge Kernel extends `BaseObject`, embeds `Auditable`, and implements `Lifecycle`:

```go
type Epic struct {
    dna.BaseObject   `yaml:",inline" json:",inline"`
    dna.Auditable    `yaml:",inline" json:",inline"`
    dna.Lifecycle    `yaml:",inline" json:",inline"`
    Title            string         `json:"title" yaml:"title"`
    Description      string         `json:"description" yaml:"description"`
    EnclaveScope     string         `json:"enclave_scope,omitempty" yaml:"enclave_scope,omitempty"`
    GoalRefs         []string       `json:"goal_refs,omitempty" yaml:"goal_refs,omitempty"`
    WorkstreamRefs   []string       `json:"workstream_refs,omitempty" yaml:"workstream_refs,omitempty"`
    RequirementRefs  []string       `json:"requirement_refs,omitempty" yaml:"requirement_refs,omitempty"`
    BacklogItemRefs  []string       `json:"backlog_item_refs,omitempty" yaml:"backlog_item_refs,omitempty"`
    ChildItemRefs    []string       `json:"child_item_refs,omitempty" yaml:"child_item_refs,omitempty"`
    PriorityPlanRefs []string       `json:"priority_plan_refs,omitempty" yaml:"priority_plan_refs,omitempty"`
    MemberWorkUnits  []*BacklogItem `json:"-" yaml:"-"`
}
```

### 3.2 Enclave Scope Boundary Checking
The `enclave_scope` field defines allowed packages or file paths:
```go
if !epic.CheckEnclaveScope("pkg/pm/pm.go") {
    // Fail-closed: mutation attempted outside declared epic enclave boundary!
}
```

### 3.3 Fail-Closed Promotion Invariant
When transitioning the Epic to `complete`:
```go
ep.RegisterInvariantGate("complete", func(ctx context.Context, obj any) error {
    epicObj, ok := obj.(*Epic)
    if !ok {
        return errfmt.Errorf("expected *Epic for promotion gate evaluation")
    }
    return epicObj.CanPromote(ctx)
})
```
If any member work unit is still in `planned`, `testing`, or `in_progress`, or has not reached `PlanePromoted`, the Epic transition fails closed with an informative error detailing which child units are blocking completion.
