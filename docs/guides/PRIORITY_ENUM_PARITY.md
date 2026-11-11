# Cross-Object Priority Enum Coercion and Parity Guide

Comprehensive architectural specification, operational guide, and invariant definition for **Cross-Object Priority Enum Coercion and Parity** (`pkg/objects`, `pkg/validation`, `pkg/storage`, `cmd/zqk`).

---

## 1. Executive Summary & Foundational Invariants

In the ZQK Knowledge Kernel, work prioritization operates across distinct ontological planes:
1. **Strategic Contract Plane (`requirement`)**: Timeless feature contracts specifying what the system must deliver. Prioritization on requirements is expressed using relative urgency tiers: `p0`, `p1`, `p2`, `p3`.
2. **Execution Delivery Plane (`backlog_item`)**: Atomic units of effort grouped within 1-cycle Priority Plans. Prioritization on backlog items is expressed using human-legible severity/urgency labels: `critical`, `high`, `medium`, `low`, paired with an ontological tier: `P0`, `P1`, `P2`, `P3`.

Prior to this specification (resolved in **GH-928 / TDE-001**), differences between requirement priority strings (`p0`..`p3`) and backlog item priority strings (`critical`..`low`) caused friction during automated orchestration, bulk updates, and cross-object queries.

Under **POL-DOC-001** and criteria **CRIT-001**, **CRIT-001**, and **CRIT-001**, this guide establishes:
- The authoritative bidirectional mapping between requirement tiers and backlog item labels.
- The operational normalization and coercion engine in `pkg/objects` and `pkg/validation`.
- Fail-closed negative boundary rejection for invalid priority literals.
- Verification invariants for CLI mutations (`zqk object update`).

```
+──────────────────────────────────────────────────────────────────────────+
│                     Human Operator / Agent Swarms                        │
│             zqk object update <id> --field priority=<value>              │
+──────────────────────────────────────────────────────────────────────────+
                                     │
                                     ▼
+──────────────────────────────────────────────────────────────────────────+
│                     Bidirectional Coercion Engine                        │
│                 (pkg/objects, pkg/validation, pkg/storage)               │
│                                                                          │
│  [ Input Detection ] ──► Accepts p-tier or word alias (case-insensitive) │
│  [ Kind Dispatch   ] ──► requirement:   "medium" ──► "p2"                │
│                      ──► backlog_item:  "p2"     ──► "medium" (tier P2)  │
│  [ Boundary Check  ] ──► "urgent" / "p4"         ──► Reject with options │
+──────────────────────────────────────────────────────────────────────────+
                                     │
                                     ▼
+──────────────────────────────────────────────────────────────────────────+
│                      Persistent Knowledge Kernel CAS                     │
│  • requirement.priority:      p0 | p1 | p2 | p3                          │
│  • backlog_item.priority:     critical | high | medium | low             │
│  • backlog_item.priority_tier: P0 | P1 | P2 | P3                         │
+──────────────────────────────────────────────────────────────────────────+
```

---

## 2. The Ontological Model: Requirements vs. Backlog Items

### Requirements: Timeless Strategic Compass
- **Ontological Role**: Timeless functional contracts declaring feature invariants. Requirements exist outside of sprint cadence or release schedules.
- **Priority Representation**: Relative urgency tiers: `p0` (highest urgency / blocker), `p1` (high urgency), `p2` (standard / normal urgency, default), `p3` (low urgency / deferred).
- **Parity Rule**: When an operator or agent supplies word aliases (`critical`, `high`, `medium`, `low`) to a requirement, the system automatically coerces the value to its canonical lowercase p-tier representation (`p0`, `p1`, `p2`, `p3`).

### Backlog Items: Atomic Units of Execution
- **Ontological Role**: Tactical units of effort executed within a single Priority Plan cycle (2–5 BLIs per plan).
- **Priority Representation**: Word labels: `critical`, `high`, `medium`, `low`.
- **Tier Synchronization**: Backlog items additionally maintain `priority_tier` (`P0`, `P1`, `P2`, `P3`) synchronized to `priority`.
- **Parity Rule**: When an operator or agent supplies p-tier aliases (`p0`, `p1`, `p2`, `p3`, `P0`..`P3`) to a backlog item, the system automatically coerces `priority` to the canonical word label (`critical`, `high`, `medium`, `low`) and synchronizes `priority_tier` to the uppercase tier (`P0`, `P1`, `P2`, `P3`).

---

## 3. Bidirectional Priority Parity Matrix

The following table defines the normative mapping across all priority tiers and labels:

| Tier Rank | Requirement Priority (`priority`) | Backlog Item Priority (`priority`) | Backlog Item Tier (`priority_tier`) | Semantics & Operational Urgency |
|:---------:|:----------------------------------:|:-----------------------------------:|:-----------------------------------:|:--------------------------------|
| **0** | `p0` | `critical` | `P0` | System outage, blocker, critical release invariant |
| **1** | `p1` | `high` | `P1` | Major milestone deliverable, primary capability |
| **2** | `p2` | `medium` | `P2` | Standard feature scope, normal cycle delivery (default) |
| **3** | `p3` | `low` | `P3` | Non-blocking enhancement, opportunistic task |

### Case-Insensitive Acceptance
All validation and coercion entry points accept case-insensitive inputs:
- Requirement: `MEDIUM`, `Medium`, `medium`, `P2`, `p2` all normalize to `p2`.
- Requirement: `CRITICAL`, `Critical`, `critical`, `P0`, `p0` all normalize to `p0`.
- Backlog Item: `P2`, `p2`, `MEDIUM`, `Medium`, `medium` all normalize to `priority: "medium"` and `priority_tier: "P2"`.
- Backlog Item: `P0`, `p0`, `CRITICAL`, `Critical`, `critical` all normalize to `priority: "critical"` and `priority_tier: "P0"`.

---

## 4. Operational Normalization & Coercion Engine

### 4.1 Package Architecture (`pkg/objects`)
- `PriorityToTier(priority string) (string, bool)`: Maps words (`critical`, `high`, `medium`, `low`) or p-tier strings (`p0`..`p3`) to uppercase tiers (`P0`..`P3`).
- `TierToPriority(tier string) (string, bool)`: Maps uppercase tiers (`P0`..`P3`) or words to canonical lowercase words (`critical`, `high`, `medium`, `low`).
- `IsLegitimatePriority(priority string) bool`: Validates that a string is a recognized priority level (`critical`, `high`, `medium`, `low`) or tier alias (`p0`..`p3`), case-insensitively.
- `IsLegitimatePriorityTier(tier string) bool`: Validates that a string is a recognized tier (`P0`..`P3`) or word alias, case-insensitively.
- `CoerceRequirementPriority(obj map[string]any)`: Normalizes requirement priority to `p0`..`p3`.
- `CoerceBacklogItemPriorityAndTier(obj map[string]any)`: Normalizes backlog item priority to `critical`..`low` and synchronizes `priority_tier` to `P0`..`P3`.
- `NormalizePriority(kind string, obj map[string]any)`: Unified kind-dispatched entry point.
- `CoerceMutationFields(kind string, obj map[string]any)`: Invokes normalization across all create, update, and persist pipelines.

### 4.2 Validation Pipeline (`pkg/validation`)
- `NormalizePriority(kind string, obj map[string]any)`: Exposed in `pkg/validation` for pipeline symmetry.
- `GoValidator.Validate`: Calls `NormalizePriority` prior to SHACL shape inspection, ensuring that objects submitted with valid equivalent priorities pass schema validation seamlessly.
- `ValidatePriority(kind string, priority string) error`: Validates priority literals, returning an informative enum error on rejection.

### 4.3 Storage Pipeline (`pkg/storage`)
- `prepareFileObjectUpdate`: Executes `CoerceMutationFields(kind, updates)` before value-restricted field validation, ensuring commands like `zqk object update REQ-001 --field priority=medium` or `zqk object update BLI-001 --field priority=p2` succeed without schema constraint errors.
- `mergeFileObjectUpdate`: Synchronizes `priority` and `priority_tier` when only one field is mutated in an update payload.

---

## 5. Negative Adversarial Boundary & Fail-Closed Validation

Any priority literal outside the legitimate spectrum (`p0`..`p3` and `critical`..`low`) must be cleanly and immediately rejected.

### Disallowed Values
Values such as `urgent`, `p4`, `p5`, `blocker`, `normal`, `unknown`, `tbd`, or arbitrary strings are invalid.

### Error Message Formatting
When an invalid value is supplied, validators emit an explicit error listing the allowed enum options:
```text
field "priority" has invalid value "urgent"; must be one of: [critical, high, medium, low, p0, p1, p2, p3]
```
Or from schema enum validation:
```text
Field priority must be one of: p0, p1, p2, p3, critical, high, medium, low
```

---

## 6. Verification & Test Case Traceability

This specification is verified by automated test suites satisfying the three-fold proof:

1. **Static Invariant Proof (`CRIT-001`)**:
   - Verified by this specification: `docs/guides/PRIORITY_ENUM_PARITY.md`.
2. **Operational Dynamic Proof (`CRIT-001`)**:
   - `pkg/validation/priority_enum_coercion_test.go:TestPriority_EnumCoercionAndParity`
   - Unit tests covering bidirectional coercion across requirements (`medium` -> `p2`) and backlog items (`p2` -> `medium`, `priority_tier: "P2"`).
   - Validation test confirming `zqk object update` compatibility for both kinds.
3. **Negative Adversarial Boundary Proof (`CRIT-001`)**:
   - Tests asserting that literals `urgent`, `p4`, `unknown` fail with descriptive enum option errors.
