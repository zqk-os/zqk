---
name: community-ontological-architecture
description: Rigorous ontological decomposition of goals, requirements, criteria, test cases, and execution plans.
---

# Ontological Architecture & Verifiable Decomposition

## Objective
Govern the scrupulous breakdown of high-level intents and composite packs into orthogonal, falsifiable, and provably verifiable Knowledge Kernel objects. Eliminate superficial hand-waving and establish complete mathematical and execution traceability.

## The Five-Layer Ontological Cascade Rubric

### 1. Layer 1: Strategic Intent & Goals (`goal`)
- **Definition**: Declare the ultimate desired end-state condition, not an operational activity.
- **Constraints**:
  - Must link to governing `mission` and `vision`.
  - Must define explicit boundary conditions (in-scope vs. out-of-scope).
  - Must specify success invariants that outlive individual sprint intervals.

### 2. Layer 2: State Transitions & Capabilities (`requirement`)
- **Definition**: The unique structural contributions or environment state changes required to achieve the goal.
- **Rules & Gates**:
  - **Orthogonality**: Requirements must be non-overlapping and mutually independent where possible.
  - **Normative Precision**: Use RFC-2119 keywords (`MUST`, `MUST NOT`, `REQUIRED`).
  - **Anti-Superficiality Doctrine**:
    - Prohibit vague adjectives (`better`, `cleaner`, `faster`, `improved`, `fixed`, `code changed`, `done`).
    - Every requirement MUST include an explicit `problem_statement` and bounded `operational_scope`.

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

### 5. Layer 5: Phased Actions & Parallel Breakdown (`milestone`, `priority_plan`, `backlog_item`)
- **Definition**: The sequence of environment-mutating actions that bend reality toward the goals.
- **Rules**:
  - **Maximal Parallelism**: Partition work into decoupled packages to prevent file lock contention and git merge conflicts.
  - **Shovel-Ready Verification**: Backlog items must have problem statements, acceptance considerations, linked milestone, requirements, criteria, and tests before entering `planned` status.
  - **Convergence Binding**: Bind work intervals to `convergence_session` objects for iterative re-measurement.
