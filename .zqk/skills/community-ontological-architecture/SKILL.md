---
name: community-ontological-architecture
description: Rigorous ontological decomposition of goals, requirements, criteria, test cases, and execution plans.
---

# Ontological Architecture & Verifiable Decomposition

## Objective
Govern the scrupulous breakdown of high-level intents and composite packs into orthogonal, falsifiable, and provably verifiable Knowledge Kernel objects. Eliminate superficial hand-waving and establish complete mathematical and execution traceability.

## The Five-Layer Ontological Cascade Rubric

### 1. Layer 1: Strategic Intent & Goals (`goal`)
- **Definition**: Declare the timeless ultimate desired end-state condition, not an operational or time-scheduled activity.
- **Constraints**:
  - Must link to governing `mission` and `vision`.
  - Must define explicit boundary conditions (in-scope vs. out-of-scope).
  - Must specify success invariants that outlive individual sprint intervals.
  - **Timelessness Invariant**: Goals exist outside of any time, sprint, or calendar constraint. Strictly NO `priority_plan_refs` or `criteria_refs` on `goal`.

### 2. Layer 2: State Transitions & Capabilities (`requirement`)
- **Definition**: The timeless functional contract (`MUST`, `MUST NOT`) defining whole-feature deliverables linked to goals.
- **Rules & Gates**:
  - **Orthogonality**: Requirements must be non-overlapping and mutually independent where possible.
  - **Normative Precision**: Use RFC-2119 keywords (`MUST`, `MUST NOT`, `REQUIRED`).
  - **Anti-Superficiality Doctrine**:
    - Prohibit vague adjectives (`better`, `cleaner`, `faster`, `improved`, `fixed`, `code changed`, `done`).
    - Every requirement MUST include an explicit `problem_statement` and bounded `operational_scope`.
  - **Timelessness Invariant**: Requirements exist outside of time and schedules. They MUST NOT declare `priority_plan_ref` or `priority_plan_refs`.
  - **Traceability Contract**: Requirements own `criteria_refs` (>= 3 satisfying the Three-Fold Proof) and are verified 1:1 by a `test_case`.

### 3. Layer 3: Verifiable Measurements & Conditions (`criteria`)
- **Definition**: The exact parameters, conditions, and thresholds required to confirm requirement satisfaction.
- **The Three-Fold Proof Formula** (Every requirement MUST define at least 3 criteria):
  1. **State Invariant (Static Floor)**: Verifiable structural, file, or CAS data state that must hold (e.g. `vds:object_exists`, `vds:field_nonempty`).
  2. **Dynamic Behavior (Operational Proof)**: Programmatic execution (test suite, command, benchmark) that runs and exits 0 with asserted output.
  3. **Negative Invariant (Adversarial Boundary)**: Verification that malformed, corrupted, or unauthorized inputs fail closed safely.

### 4. Layer 4: Programmatic Confirmation (`test_case`)
- **Definition**: The programmatic test or harness that confirms or denies fulfillment of criteria.
- **Constraints**:
  - Must declare concrete file targets (`path_or_id`) and test entrypoints.
  - Must be deterministic, automated, and runnable without manual human inspection.
  - Linked 1:1 with a `requirement` to wrap and verify its criteria bundle.

### 5. Layer 5: Phased Actions & Parallel Breakdown (`milestone`, `priority_plan`, `backlog_item`)
- **Definition**: The sequence of environment-mutating actions that bend reality toward the goals.
- **Rules**:
  - **Milestone Chronological Anchors**: `milestone` objects add the time constraint—acting as time-bounded subgoals across delivered reality that Priority Plans aim toward.
  - **Priority Plans (1-Cycle Batches)**: `priority_plan` groups exactly 1 cycle of work (sprint/kanban batch, 2–5 BLIs) and scope-locks upon `in_progress`.
  - **Backlog Items (Atomic Effort Slices)**: Backlog items satisfy 1 to 3 specific criteria. Never build an entire feature contract in a single monolithic BLI.
  - **Maximal Parallelism**: Partition work into decoupled packages to prevent file lock contention and git merge conflicts.
  - **Shovel-Ready Verification**: Backlog items must have problem statements, acceptance considerations, linked milestone, requirements, criteria, and tests before entering `planned` status.
  - **Convergence Binding**: Bind work intervals to `convergence_session` objects for iterative re-measurement.

### 6. The Non-Functional Behavior Plane (`technical_debt`)
- **Definition**: Tactical entry point into technical work supporting goals without being a feature requirement contract.
- **Non-Functional Plane Story**: Documents code smells, test flakes, performance bottlenecks, teardown leaks, and negative boundary omissions.
- **The Rollup & Categorization Principle**: Multiple tech debt items MUST be rolled up to extract systemic anti-pattern stories, directly codifying new architectural best practices and formal kernel `policy` objects.

### 6. Declarative Traversal & Atomic Mutation Discipline (ZPARQL & ZQL)
- **Declarative Graph Traversal (`zqk query` / MCP `query_zparql`)**:
  Do NOT perform manual multi-step CLI loops or procedural BFS sweeps in code to discover dependencies or check DoD traceability. Use declarative ZPARQL:
  ```zparql
  MATCH (p:priority_plan {status: "in_progress"})-[:items]->(b:backlog_item),
        (b)-[:criteria_refs]->(c:criteria),
        (c)-[:test_case_refs]->(t:test_case)
  WHERE b.status != "complete"
  RETURN p.title AS plan, b.id AS bli_id, c.title AS criterion, t.id AS test_case
  ORDER BY b.id ASC;
  ```
- **Atomic Multi-Object Mutation (`zqk mutate` / MCP `mutate_zql`)**:
  Do NOT make multiple fragmented CLI calls (`zqk object create ...`) that risk partial failure and orphan CAS records. Define the entire object constellation in an atomic ZQL transaction:
  ```zql
  BEGIN TRANSACTION ISOLATION LEVEL STAGED_SNAPSHOT;
  LET $plan = UPSERT priority_plan {
      title: "Observability Overhaul",
      priority_tier: "P1",
      status: "in_progress"
  } RETURNING id;
  LET $bli = UPSERT backlog_item {
      title: "Stream Consumer Daemon",
      priority_plan_ref: $plan.id,
      priority_tier: "P1",
      status: "planned"
  } RETURNING id;
  UPSERT criteria {
      title: "Zero Memory Leak Invariant",
      backlog_item_ref: $bli.id
  };
  COMMIT TRANSACTION;
  ```
  Forward and backward variable references (`$plan.id`, `$bli.id`) are resolved via Kahn's algorithm with preflight validation and atomic rollback.

## 7. Anti-Bloat Cardinality Discipline & Epistemic Synthesis
- **Strict Prohibition on 1:1 Symptom-Mirroring**:
  Under no circumstances may an agent convert an evaluation report or list of defects into a 1:1 constellation of requirements, criteria, and backlog items. Doing so causes epistemic sprawl, inflates graph traversal costs, and produces trivial micro-tickets that mask root causes.
- **Root-Cause Clustering Target Ratios**:
  - **Symptom-to-BLI Ratio**: $\ge 5:1$ (At least 5 to 10 findings/symptoms per Backlog Item).
  - **Requirement-to-Criteria Ratio**: $1:3$ (Every requirement MUST be supported by at least 3 orthogonal criteria: Static, Dynamic, Negative).
  - **Requirement-to-BLI Ratio**: $1:2$ to $1:3$ (A requirement defines a major system invariant or capability, executed by 2–3 cohesive backlog items).
- **Metadata Binding**:
  Map original finding IDs (e.g. `F-CONC-001`) into the `description`, `notes`, or `tags` of the consolidated Backlog Item rather than minting duplicate micro-objects.

## 8. Deterministic Cybernetic Steering Loop
All planning and execution agents operate as closed-loop controllers:
1. **Target State Projection**: Project the delta between current state and target state using empirical indicators (CEF scorecard, VDS done-gates, alignment score).
2. **Hypothesis Evaluation**: Formulate candidate action sets. Select the hypothesis that names the actions most likely to bring the state projection closer to target with minimum blast radius.
3. **Deterministic Execution**: Execute changes cleanly under TDD discipline.
4. **Re-Evaluation & Measurement**: Execute objective measurement tools and compare directly against prior run output.
5. **Gain/Loss Delta Calculation**: Quantify empirical gain or loss from the execution cycle.
6. **Ambient Signal Feedback**: Inject measurement signals ambiently into the kernel graph (`zqk system align`, feed, convergence sessions).
7. **Dynamic Task Minting & Steering**: Query `zqk workflow whats-next` to mint or advance the next highest-priority task dictated by the kernel.
