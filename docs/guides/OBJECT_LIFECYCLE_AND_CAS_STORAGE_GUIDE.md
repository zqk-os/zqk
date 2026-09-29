# Developer & Agent Guide: Object Lifecycle, State Machines, and Content-Addressed Storage (CAS)

## 1. Executive Summary

All state within the **ZQK Knowledge Kernel** is represented as strongly-typed, discrete entities called **Kernel Objects**. Whether tracking high-level system objectives (`goal`), actionable work items (`backlog_item`), execution roadmaps (`priority_plan`), verifiable criteria (`criteria`), or operational safeguards (`policy`), every object is subject to:

1. **Schema Validation**: Explicit attribute typings and constraint schemas managed under `kernel-specs/objects/`.
2. **Content-Addressed Storage (CAS)**: Cryptographic SHA-256 fingerprinting guaranteeing immutable provenance and tamper-evident history.
3. **Formal Lifecycle State Machines**: Rigorous, check-valved state machines governing valid lifecycle progressions (e.g. `originated` ➔ `planned` ➔ `in_progress` ➔ `complete`).
4. **Two-Phase Isolation (Draft vs. Authoritative Planes)**: Unverified mutations exist in an isolated draft membrane until verified and promoted into the authoritative system plane.

This guide provides both human engineers and autonomous AI agents with actionable protocols for safely creating, transitioning, linking, and promoting kernel objects.

---

## 2. Kernel Object Anatomy & Taxonomy

Every kernel object is serialized on disk as a YAML or JSON document containing standard architectural envelopes:

```yaml
schema_version: 2.0.0
id: BLI-AUTH-004
kind: backlog_item
status: in_progress
title: "Implement Content-Addressed Storage Membrane Check-Valves"
claimed_by: "agent-alpha"
priority_tier: P0
priority_plan_ref: PRI-TPM-CONV
goal_refs:
  - GOAL-001
requirement_refs:
  - REQ-012
criteria_refs:
  - CRIT-049
test_case_refs:
  - TST-030-01
created_at: "2026-09-29T14:00:00Z"
updated_at: "2026-09-29T14:30:00Z"
storage_profile:
  cas_hash: "sha256:d8a9f4e2c8104598bfaaa107ac635e1070be99d5c10eba5793cac0baf38ba8a1"
  plane: "authoritative"
  etag: "rev-003-d8a9f"
```

### Core Entity Taxonomy

| Object Kind | Primary Role & Operational Purpose | Canonical Storage Path |
| :--- | :--- | :--- |
| `goal` | High-level system aspirations and business targets | `.zqk/process/goal/` |
| `requirement` | Formal technical requirements satisfying goals | `.zqk/process/requirement/` |
| `priority_plan` | Tactical execution plans scoping backlog items | `.zqk/process/priority_plan/` |
| `backlog_item` | Actionable unit of work (BLI) executed by agents | `.zqk/process/backlog_item/` |
| `criteria` | Verifiable Definition of Done (DoD) acceptance item | `.zqk/process/criteria/` |
| `test_case` | Automated test suite verifying criteria satisfaction | `.zqk/process/test_case/` |
| `policy` | Declarative governance invariant and boundary rule | `.zqk/process/policy/` |
| `risk_blocker` | Active impediment obstructing workstream completion | `.zqk/process/risk_blocker/` |
| `audit_event` | Append-only cryptographic log of state mutations | `.zqk/process/audit_event/` |

---

## 3. Two-Phase Storage: Draft vs. Authoritative Planes

To prevent corrupt, partially written, or non-compliant states from destabilizing production swarms, ZQK utilizes a dual-plane cellular storage architecture:

```
┌────────────────────────────────────────────────────────┐
│                      DRAFT PLANE                       │
│  Isolated working memory for speculative mutations     │
│  • Unchecked attribute edits                           │
│  • Ephemeral agent experiments                         │
│  • Zero impact on swarm schedulers or done-gates       │
└──────────────────────────┬─────────────────────────────┘
                           │
                           │ Validation Gate Check:
                           │ ✓ Schema Compliance
                           │ ✓ CAS Hash Verification
                           │ ✓ DoD Lineage Traceability
                           │ ✓ Policy Preflight Passes
                           ▼
┌────────────────────────────────────────────────────────┐
│                  AUTHORITATIVE PLANE                   │
│  The immutable, cryptographically verified truth       │
│  • Pre-commit hooks armed                              │
│  • Real-time TUI Mission Control indexing              │
│  • Consensus state for all agent swarms                │
└────────────────────────────────────────────────────────┘
```

---

## 4. Minting & Manipulating Objects

### 4.1 CLI Commands

#### Minting a New Object
```bash
# Mint a new backlog item in the draft plane
zqk object create backlog_item \
  --title "Integrate KaTeX Math Delimiters" \
  --fields priority_tier:P1,category:feature,priority_plan_ref:PRI-TPM-CONV
```

#### Viewing an Object
```bash
# Formatted interactive console view
zqk object inspect backlog_item BLI-AUTH-004

# Machine-readable JSON output for autonomous agents
zqk object get backlog_item BLI-AUTH-004 -f json
```

#### Mutating an Object
```bash
# Update lifecycle status
zqk object update backlog_item BLI-AUTH-004 --set status=in_progress

# Claim work under the active agent seat
zqk agent claim-work --id BLI-AUTH-004
```

### 4.2 Declarative Mutation Language (ZQL)

For multi-object operations, avoid sequential CLI calls. Use **ZQL** for atomic multi-entity mutations:

```zql
MUTATE
  NEW bli:backlog_item {
    title: "Implement Stream Backpressure Buffer",
    priority_tier: "P0",
    status: "planned",
    priority_plan_ref: "PRI-TPM-CONV"
  },
  NEW crit:criteria {
    title: "Buffer enforces O(1) memory bound under 10k event surge",
    status: "originated"
  }
LINK
  (bli)-[:criteria_refs]->(crit)
ON_ABORT
  ROLLBACK;
```

---

## 5. Lifecycle State Machines & Transition Check-Valves

Every entity kind enforces strict, directional state transitions defined in `kernel-specs/lifecycles/`.

### Canonical Backlog Item Lifecycle

```
┌──────────────┐     plan      ┌──────────────┐     claim     ┌──────────────┐
│  originated  │ ────────────► │   planned    │ ────────────► │ in_progress  │
└──────────────┘               └──────────────┘               └──────┬───────┘
                                                                     │
                         ┌───────────────────────────────────────────┴────────────────┐
                         │                                                            │
                         ▼ complete                                                   ▼ block
                  ┌──────────────┐                                             ┌──────────────┐
                  │   complete   │                                             │   blocked    │
                  └──────────────┘                                             └──────────────┘
```

### Transition Invariants (Done-Gates)

1. **`originated` ➔ `planned`**: Must be associated with an active `priority_plan` and have an assigned `priority_tier` (`P0`, `P1`, `P2`, or `P3`).
2. **`planned` ➔ `in_progress`**: Requires a valid `claimed_by` assignee and active workspace lease.
3. **`in_progress` ➔ `complete`**:
   - **Criteria Satisfaction**: 100% of bound `criteria_refs` must be marked `complete`.
   - **Test Lineage**: All bound `test_case_refs` must exist and report zero failing assertions.
   - **Audit Receipt**: Mutation must generate a corresponding `audit_event`.

---

## 6. Definition of Done (DoD) & Graph Lineage Binding

A core tenet of the ZQK Knowledge Kernel is **Unbroken Traceability**. No unit of work can be completed in isolation; every backlog item must participate in an unbroken lineage chain:

```
[Goal] ➔ [Requirement] ➔ [Backlog Item] ➔ [Test Case] ➔ [Criteria]
```

### Verifying Lineage with ZPARQL

Autonomous agents verify unbroken DoD chains with a single declarative traversal query:

```zparql
MATCH (g:goal)<-[:goal_refs]-(b:backlog_item {id: "BLI-AUTH-004"})-[:criteria_refs]->(c:criteria),
      (b)-[:test_case_refs]->(t:test_case)
RETURN b.id AS item, b.status AS status, c.id AS criterion, c.status AS crit_status, t.status AS test_status;
```

If any link in the chain is severed (`missing root goal`, `unbound criteria`), promotion check-valves fail-closed and reject the transition.

---

## 7. Promoting Objects to the Authoritative Plane

When an object in the draft plane satisfies all schema, lineage, and policy checks, promote it to the authoritative plane:

```bash
# Explicit promotion command
zqk object promote backlog_item BLI-AUTH-004

# Promote via Object Inspector TUI
zqk object inspect backlog_item BLI-AUTH-004
# Press [Enter] to open Action Palette -> Press [p] for Promote
```

### Promotion Gate Verification Receipt

Upon promotion, the storage provider:
1. Recomputes the object's canonical SHA-256 CAS hash.
2. Updates `storage_profile.plane` from `draft` to `authoritative`.
3. Stamps a new immutable `etag` revision.
4. Appends a tamper-evident audit record to `.zqk/process/audit_event/`.
5. Emits an event to the `coordination.EventCoordinator` spinal cord.
