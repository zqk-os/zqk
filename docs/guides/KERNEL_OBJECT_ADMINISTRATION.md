# Process Administration & Management: Kernel Objects
**Taxonomy, Slicing Standards, Cardinality, and the Shift-Left Directive**

---

## 1. Executive Summary: The Shift-Left Mandate

In transient, chat-based agent harnesses, agents write massive, monolithic changes across entire codebases with superficial PR summaries. In the **ZQK Knowledge Kernel**, reality is governed by verified, immutable graph objects.

Waiting for `git commit` check-valves or `zqk-vet` to catch errors is a late-stage fail-safe. **Process Administration must Shift Left**: agents and human operators must understand the exact purpose, trigger, boundaries, and cardinality of kernel objects *before* minting objects or writing code.

---

## 2. Kernel Object Taxonomy (What / When / Why / How Many)

| Object Kind | **What** (Definition) | **When** (Trigger) | **Why** (Rationale) | **How Many** (Cardinality Ratio) |
| :--- | :--- | :--- | :--- | :--- |
| **`goal`** | Strategic end-state condition. | Project kickoff or major strategic direction shift. | Anchors strategic North Star; defines what success looks like independent of sprints. | **1 per product tier / mission** |
| **`milestone`** | Significant stakeholder boundary event. | Key external release or major capability delivery. | Demonstrates real-world value delivered to executive and external stakeholders. | **1 to 3 per Goal** |
| **`epic`** | **Thematic container grouping multiple plans.** | When an initiative spans multiple time cycles or sprints. | Keeps related execution cycles organized under one identifiable umbrella. | **1 to 3 per Goal** |
| **`priority_plan`** | **1-cycle time-bounded execution block.** | Start of each execution cycle / sprint. | **Reigns in scope commitments** to ensure work gets completed, not parked. | **2 to 5 per Epic**<br>*(1 cycle of work)* |
| **`requirement`** (REQ) | Whole feature functional contract (`MUST`, `MUST NOT`). | Outlined during design to bound problem statement and operational scope. | Formalizes functional truth and boundary conditions independently of code. | **3 to 5 per Goal**<br>*(1 to 3 per Epic)* |
| **`test_case`** (TST) | **Unified test runner proving a requirement.** | TDD upfront test harness created before or alongside code. | Cryptographically proves all linked criteria in a single logical test command. | **1:1 with Requirement** |
| **`criteria`** (CRIT) | Mathematical measurement or falsifiable condition. | Created alongside the requirement before coding begins. | Eliminates hand-waving; drives automatic shockwave state graduation. | **Minimum 3 per REQ**<br>*(The Three-Fold Proof)* |
| **`backlog_item`** (BLI) | **Atomic unit of effort** & commit evidence. | Sliced before coding starts for a single cohesive subsystem. | Enables swarm parallelism, limits blast radius, provides deterministic AST rollback. | **2 to 5 per Plan**<br>*(Each satisfies 1–3 CRIT)* |

---

## 3. The 5-Layer Relational Cascade

```
                  ┌───────────────────────────────┐
                  │          GOAL                 │
                  │   Strategic End-State         │
                  └───────────────┬───────────────┘
                                  │ 1 : 3-5
                                  ▼
                  ┌───────────────────────────────┐
                  │       REQUIREMENT (REQ)       │◄─────────────────────────────┐
                  │    Whole Feature Contract     │                              │
                  └───────────────┬───────────────┘                              │
                                  │ 1 : 3+                                       │
                                  ▼                                              │ Verifies All Criteria
                  ┌───────────────────────────────┐                              │ for the Requirement
                  │        CRITERIA (CRIT)        │                              │ (1 per Requirement)
                  │  Static · Operational · Neg   │                              │
                  └───────┬───────────────┬───────┘                              │
         Satisfies 1-3    │               │                                      │
         Criteria / Effort│               │ Evaluated by Unified Runner          │
                          ▼               ▼                                      │
           ┌──────────────────────┐   ┌──────────────────────────────────────────┴────┐
           │  BACKLOG_ITEM (BLI)  │   │                TEST_CASE (TST)                │
           │  Unit(s) of Work     │   │ Unified Test Runner / Logical Verification    │
           │  (2-5 per Plan)      │   │ Group proving the entire Requirement          │
           └──────────────────────┘   └───────────────────────────────────────────────┘
```

### Critical Linkage Rules
1. **Goals DO NOT have `criteria_refs`:**
   Goals are high-level strategic compasses. They decompose into **Requirements** via `requirement_refs`. Only Requirements own `criteria_refs`.
2. **Requirements own `criteria_refs`:**
   Every requirement MUST define at least 3 criteria satisfying the Three-Fold Proof.
3. **Test Cases are 1:1 with Requirements:**
   A test case wraps all criteria linked to a requirement. Running `zqk test run <tst_id>` evaluates the complete feature contract in one unified pass.
4. **Backlog Items satisfy 1–3 Criteria:**
   A Backlog Item is an atomic slice of implementation effort. It links to a time-bounded `priority_plan` and satisfies 1 to 3 specific criteria.

---

## 4. The Gantt Matrix Topology

In ZQK Studio and execution planning, the graph renders as an audited two-dimensional execution grid:

```
                            TIME-BOUNDED EXECUTION CYCLES (Priority Plans)
                         Cycle 1 (Sprint A)        Cycle 2 (Sprint B)       Cycle 3 (Sprint C)
                        ┌─────────────────────┐   ┌─────────────────────┐   ┌─────────────────────┐
                        │ PRI-PLAN-001        │   │ PRI-PLAN-002        │   │ PRI-PLAN-003        │
                        └─────────────────────┘   └─────────────────────┘   └─────────────────────┘
┌─────────────────────┐ ┌─────────────────────┐
│ WORKSTREAM 1        │ │ [BLI-001] [BLI-002] │ ──► Milestone 1: Core Engine Alpha ★
│ (Lanes / Rows)      │ └─────────────────────┘
├─────────────────────┤                           ┌─────────────────────┐
│ WORKSTREAM 2        │                           │ [BLI-003] [BLI-004] │
│ (e.g. Studio UI)    │                           └─────────────────────┘
├─────────────────────┤                                                     ┌─────────────────────┐
│ WORKSTREAM 3        │                                                     │ [BLI-005] [BLI-006] │ ──► Milestone 2: Public Preview ★
│ (e.g. DevRel)       │                                                     └─────────────────────┘
└─────────────────────┘
```

* **Workstreams (Rows / Swimlanes):** Ongoing functional domains (e.g., *Core Engine*, *Studio UI*, *Developer Relations*).
* **Epics (Grouping Banners):** Unifying thematic containers that span across multiple cycles.
* **Priority Plans (Columns / Blocks):** Discrete, 1-cycle execution windows that lock scope.
* **Backlog Items (Intersections):** The units of effort executed within that cycle and lane.
* **Milestones (Flags / Anchors):** Significant stakeholder boundary events crossing delivered reality.

---

## 5. Epic Shockwaves & Permissive Scope Invariant

* **Child ➔ Parent Shockwave Progression:**
  * When the *first* linked `priority_plan` transitions to `in_progress`, the parent `epic` automatically shockwaves from `planned` ➔ `in_progress`.
  * When *all* linked priority plans reach `complete`, the parent `epic` automatically shockwaves ➔ `completed`.
* **Permissive Scope Invariant:**
  * **Priority Plans** are strictly **scope-locked** upon entering `in_progress` to guarantee cycle completion.
  * **Epics** are **permissive by default**. Operators and agents can freely attach new `priority_plan` objects to an in-progress `epic` as the initiative evolves across cycles.
  * An `epic` can be explicitly locked by setting `execution_locked: true` when no further plans may be added.

---

## 6. Cardinal Anti-Patterns vs. Correct Practice

### Anti-Pattern 1: The "BLI Hijacking" Anti-Pattern (Scope Stacking)
* **Violation:** The user asks for a new Web UI or macro feature, and the agent tacks it onto an existing or completed BLI (e.g. adding a prompt studio to `BLI-REPLY-ENGINE-001`).
* **Correct Practice:** Never mutate or stack scope onto an in-flight or completed BLI. Mint a new BLI (`zqk new bli "Prompt Studio Web UI" --plan PRI-xxx`).

### Anti-Pattern 2: The Degenerate 1:1:1:1 Pipeline
* **Violation:** 1 Goal ➔ 1 Plan ➔ 1 BLI ➔ 1 REQ ➔ 1 CRIT. The BLI becomes a massive monolithic PR bundling database, engine, kill switch, and UI into one commit.
* **Correct Practice:** Decompose the plan into 2 to 5 orthogonal BLIs, each satisfying 1–3 criteria.

### Anti-Pattern 3: The Three-Fold Proof Omission
* **Violation:** Defining a single criterion: `CRIT-001: Works as expected`.
* **Correct Practice:** Every requirement must have at least:
  1. **Static Invariant Floor:** Schema validated, types correct, lint zero.
  2. **Operational Dynamic Proof:** Programmatic test suite exits 0 with asserted output.
  3. **Negative / Adversarial Boundary:** Invalid inputs, timeouts, or unauthorized calls fail closed safely.
