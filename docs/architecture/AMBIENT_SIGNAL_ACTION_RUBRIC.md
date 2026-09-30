# Ambient Signal Interpretation & Action Rubric (Anti-Thrashing Protocol)

## 1. Executive Summary & Purpose

In an autonomous multi-agent operating environment, multiple autonomous software engineers (SEs) and daemons interact concurrently with the ZQK Knowledge Kernel. Without deterministic signal interpretation, agents risk thrashing—repeatedly querying status, re-grooming already planned items, or entering idle wait states instead of driving backlog items to completion.

The **Ambient Signal Interpretation & Action Rubric** establishes a strict, unambiguous precedence hierarchy for autonomous agents and kernel stewards. Every ambient signal—whether originating from Write-Ahead Logs (WAL), filesystem notification events, scheduler heartbeats, or CLI status queries—maps to a deterministic action and an objective validation gate.

---

## 2. Strict Precedence Hierarchy (P0 → P5)

When an agent wakes up or completes a task iteration, it evaluates ambient signals according to the following strict precedence rules. Higher-precedence signals preempt lower-precedence activities immediately.

```
┌───────────────────────────────────────────────────────────┐
│ P0: System Blockers & Kernel Corruption                   │
│     (Lock contention, WAL compaction, CAS mismatches)     │
├───────────────────────────────────────────────────────────┤
│ P1: Mesh Synchronization & Watermark Lag                  │
│     (Materialized views, scheduler daemons, lock files)   │
├───────────────────────────────────────────────────────────┤
│ P2: Active In-Progress Tasks                              │
│     (Resuming claimed BLIs, latching criteria, DoD gates) │
├───────────────────────────────────────────────────────────┤
│ P3: Priority Plan Delivery                                │
│     (Executing shovel-ready items on lead active plan)    │
├───────────────────────────────────────────────────────────┤
│ P4: Runway Replenishment                                  │
│     (Backlog grooming when planned shovel-ready items < 2)│
├───────────────────────────────────────────────────────────┤
│ P5: System Hygiene & Verification                         │
│     (Running test suites, AST linting, artifact sweeps)   │
└───────────────────────────────────────────────────────────┘
```

### P0: System Blockers & Kernel Corruption
- **Trigger**: Deadlock detections, stale concurrency locks, WAL compaction errors, or CAS checksum mismatches.
- **Mandated Action**: Cease all feature mutation. Execute auto-remedy triage:
  ```bash
  zqk system check --auto-remedy
  ```
  If unresolved, consult incident runbooks (`docs/runbooks/RB-CAS-001`, `RB-LCK-001`, `RB-WAL-001`).
- **Validation Gate**: `zqk system check --format json` reports `health_status: "ok"` or `"healthy"` with zero blocking errors.

### P1: Mesh Synchronization & Watermark Lag
- **Trigger**: Materialized view watermark exceeds tolerance (>2m0s lag), scheduler daemon stopped, or uncommitted ambient state caches missing.
- **Mandated Action**: Refresh kernel materialized projections and ensure daemons are operational:
  ```bash
  zqk scheduler start
  zqk system align --format json -o .zqk/state/ambient/align-latest.json
  ```
- **Validation Gate**: `zqk workflow whats-next` reports `Materialized view: current` or `watermark within tolerance`.

### P2: Active In-Progress Tasks
- **Trigger**: The claimant holds an active backlog item (`status: in_progress`) in the Knowledge Kernel.
- **Mandated Action**: Finish the claimed task. Never context-switch or claim a new BLI while one is in progress:
  ```bash
  zqk do <BLI-ID>
  ```
- **Validation Gate**: Associated test cases PASS, acceptance criteria latched (`status: satisfied`), and BLI latched to `completed` or `implemented`.

### P3: Priority Plan Delivery
- **Trigger**: The lead active priority plan (`PRI-*`) contains shovel-ready planned items (`status: planned`).
- **Mandated Action**: Self-discover what's next, claim the lead backlog item atomically, and begin execution:
  ```bash
  zqk workflow whats-next
  zqk do <BLI-ID>
  ```
- **Validation Gate**: Object transition committed to change journal with valid HMAC / author signature.

### P4: Runway Replenishment
- **Trigger**: Active priority plan has 0 planned items remaining, but open milestones or unfulfilled requirements exist.
- **Mandated Action**: Groom next tranche of requirements into shovel-ready backlog items (Definition of Ready: clear description, linked criteria, linked verification tests):
  ```bash
  zqk workflow add <BLI-ID> [PRI-ID]
  ```
- **Validation Gate**: Minimum 2 shovel-ready backlog items planned with validated criteria before executing work.

### P5: System Hygiene & Verification
- **Trigger**: All backlog items on the active plan completed, or milestone transition reached.
- **Mandated Action**: Run comprehensive verification suites and AST linter gates:
  ```bash
  ./bin/zqk-vet
  ./bin/zqk test run --all
  ```
- **Validation Gate**: Zero lint violations, all tests green, clean git status.

---

## 3. Signal Action Matrix

| Signal Event | Source | Priority | Mandated CLI Command | Objective Done-Gate |
| :--- | :--- | :---: | :--- | :--- |
| `STALE_LOCK_FILE` | `.zqk/locks/*.lock` | **P0** | `zqk system check --auto-remedy` | Stale locks pruned; exit code 0 |
| `CAS_HASH_MISMATCH` | `pkg/storage` | **P0** | Consult `docs/runbooks/RB-CAS-001` | Object restored from WAL or rebuilt |
| `SCHEDULER_STOPPED` | `zqk system status` | **P1** | `zqk scheduler start` | Scheduler daemon PID active in `.zqk/state` |
| `VIEW_LAG_EXCEEDED` | `zqk workflow whats-next` | **P1** | `zqk system align` | Watermark age < 120s |
| `BLI_IN_PROGRESS` | `zqk object list --kind bli` | **P2** | `zqk do <BLI_ID>` | Criteria latched; BLI status: `implemented` |
| `PLAN_SHOVEL_READY` | `zqk workflow whats-next` | **P3** | `zqk do <BLI-ID>` or `zqk agent claim <ATK-ID>` | BLI claimed by seat; status: `in_progress` |
| `PLAN_EXHAUSTED` | `zqk pplan current` | **P4** | `zqk workflow add <BLI-ID>` | Shovel-ready queue replenished |
| `MILESTONE_MERGED` | Git commit / PR | **P5** | Promote binary & query `whats-next` | Continuous loop advances without idleness |

---

## 4. Continuous Autonomous Loop Discipline (Anti-Idleness Protocol)

### Prohibition of Summary-as-Terminal State
In autonomous development loops, merging a pull request, rendering an artifact summary, or running a verification suite is a **milestone transition**, never a stopping condition.
Autonomous agents must not halt or yield control to an idle wait state when tasks remain in the pipeline.

### Post-Merge Self-Continuation Sequence
Upon completing a backlog item or merging an integration branch:
1. Build updated binaries and refresh background daemons:
   ```bash
   make build
   zqk scheduler stop && zqk scheduler start
   ```
2. Query the Knowledge Kernel for the next priority:
   ```bash
   zqk workflow whats-next
   ```
3. Checkout or create the next integration branch:
   ```bash
   git checkout -b integration/<PRI-ID> origin/main
   ```
4. Claim the first shovel-ready backlog item atomically:
   ```bash
   zqk do <BLI-ID>
   ```
5. Resume execution chain (`zqk do <BLI-ID>`) without yielding control.

---

## 5. Architectural Invariants

1. **No Out-of-Kernel State**: Ambient state, claimed work, and priority signals exist strictly within the Knowledge Kernel (`.zqk/`). Local vendor scratchpads or agent memory files are not authoritative.
2. **Fail-Closed Verification**: An item is never marked completed unless all associated criteria are cryptographically latched and verified by automated tests.
3. **Idempotent Self-Healing**: Auto-remedy operations must be idempotent and safe to execute repeatedly under high concurrency.
4. **Anti-Bloat Cardinality Invariant**: Symptom lists, evaluation findings, and bug reports MUST be synthesized into root-cause engineering packages ($\ge 5:1$ symptom-to-BLI ratio). Mechanical 1:1 symptom-mirroring is prohibited.
5. **Deterministic Cybernetic Steering**: Agents evaluate whether candidate actions bring the target projection closer or farther away, execute the most promising hypothesis, re-measure deltas, calculate gain/loss, feed ambient signals to the kernel, and dynamically execute the highest kernel priority.

---

## 6. The Deterministic Cybernetic Steering Loop

```
                     ┌───────────────────────────────┐
                     │ 1. State Projection           │
                     │    Measure delta vs target    │
                     │    (CEF Diamond, VDS, Align)  │
                     └───────────────┬───────────────┘
                                     │
                                     ▼
                     ┌───────────────────────────────┐
                     │ 2. Hypothesis Formulation     │
                     │    Evaluate candidate actions │
                     │    Select max-gain hypothesis │
                     └───────────────┬───────────────┘
                                     │
                                     ▼
                     ┌───────────────────────────────┐
                     │ 3. Deterministic Execution    │
                     │    Atomic TDD mutation        │
                     │    (zqk do <BLI-ID>)          │
                     └───────────────┬───────────────┘
                                     │
                                     ▼
                     ┌───────────────────────────────┐
                     │ 4. Re-Evaluation & Measure    │
                     │    Run empirical benchmarks   │
                     │    (test-race, CEF eval)      │
                     └───────────────┬───────────────┘
                                     │
                                     ▼
                     ┌───────────────────────────────┐
                     │ 5. Delta & Gain Calculation   │
                     │    Compare to prior baseline  │
                     │    Quantify empirical delta   │
                     └───────────────┬───────────────┘
                                     │
                                     ▼
                     ┌───────────────────────────────┐
                     │ 6. Ambient Feedback & Steer   │
                     │    zqk system align           │
                     │    zqk workflow whats-next    │
                     │    Mint next highest-priority │
                     └───────────────┬───────────────┘
                                     │
                                     └─────── Loop back to 1
```
