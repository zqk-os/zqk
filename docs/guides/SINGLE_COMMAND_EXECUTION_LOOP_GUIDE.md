# Developer & Agent Guide: Autonomous Single-Command Execution Loop (`zqk do`)

## 1. Executive Summary

Autonomous AI agents often suffer from **context thrashing**: oscillating between multiple tools, stalling at trivial intermediate hurdles, making unvalidated assumptions, and losing track of high-level objectives.

The **ZQK Single-Command Execution Loop (`zqk do`)** solves this by encapsulating intent resolution, dependency ordering, lock management, preflight validation, execution, and verification into a single, cohesive, fail-closed command pipeline:

```bash
zqk do <intent>
```

Whether resolving a compile error, executing a planned backlog item, running a migration, or updating kernel schemas, `zqk do` provides autonomous agents and human developers with a safe, deterministic execution harness.

---

## 2. The 5-Phase Execution Engine

Every invocation of `zqk do` executes through five strictly ordered phases:

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   PHASE 1    │ ──► │   PHASE 2    │ ──► │   PHASE 3    │ ──► │   PHASE 4    │ ──► │   PHASE 5    │
│ Intent Parse │     │ Preflight    │     │ Lock Lease   │     │ Action Exec  │     │ Verification │
└──────────────┘     └──────────────┘     └──────────────┘     └──────────────┘     └──────────────┘
```

### Phase 1: Intent Parsing & Ontological Resolution
The engine parses the incoming intent string against registered system models and active workstreams:
- Resolves target objects (e.g. `BLI-AUTH-004`, `PRI-TPM-CONV`).
- Discovers implied dependencies and required toolchains.
- Maps operations to canonical CLI primitives (`zqk object`, `zqk query`, `zqk system`).

### Phase 2: Preflight Invariant Verification
Before taking any action, the engine executes fail-closed preflight checks:
- Verifies clean git working tree hygiene (zero unexpected file modifications).
- Checks active lock files in `.zqk/scheduler/locks/` to prevent concurrency conflicts.
- Validates active policies and Definition of Done constraints.

### Phase 3: Lock Acquisition & Workspace Leases
If the operation mutates kernel state:
- Acquires an atomic lock on the target entity ID in `.zqk/lock/`.
- Registers an active lease tied to the caller's agent seat identity.
- Sets up an automatic heartbeat timer to prevent stale locks upon failure.

### Phase 4: Action Execution & Transaction Staging
- Executes the planned mutation within an isolated staging sandbox.
- Intercepts and logs stdout/stderr into structured audit telemetry.
- Buffers disk writes until the entire step completes successfully.

### Phase 5: Post-Action Verification & Done-Gates
- Executes verification checks: runs `zqk-vet`, executes associated unit tests, and verifies CAS hashes.
- If all checks pass: commits changes, releases locks, and transitions object state.
- If any check fails: triggers an atomic rollback, logs failure delta, and surfaces actionable remediation recipes.

---

## 3. Practical Usage Scenarios

### Scenario 1: Executing Backlog Items
```bash
# Execute work on a claimed backlog item
zqk do "implement BLI-AUTH-004: add criteria validation check-valves"
```
`zqk do` automatically:
1. Verifies that `BLI-AUTH-004` is assigned to the current caller.
2. Checks that dependent requirements and root goals are intact.
3. Runs tests before and after code modifications.
4. Transitions status from `in_progress` to `complete`.

### Scenario 2: System Remediation
```bash
# Auto-heal environment anomalies
zqk do "heal stale locks and run system checks"
```
Resolves orphaned lock files, cleans temporary staging buffers, and verifies the CAS membrane.

### Scenario 3: Autonomous Loop Self-Continuation
In an autonomous continuous loop, merging a PR or finishing a task must never result in an idle stop. Agents invoke:
```bash
zqk do "continue workflow: query whats-next and claim highest priority BLI"
```

---

## 4. Error Recovery & Anti-Thrashing Discipline

When an error occurs during `zqk do`:

1. **Atomic Rollback**: File changes made during the failed step are discarded.
2. **Diagnostic Receipt**: An error envelope is written to `.zqk/state/last_failure.json` detailing:
   - Failing phase (e.g. `PHASE_2_PREFLIGHT`).
   - Specific invariant violation (e.g. `POL-SAFETY-001 rejected mutation`).
   - Exact entity IDs involved.
3. **Deterministic Recipe**: Surfaces a zero-guesswork command string for the agent to resolve the issue:
   ```bash
   ⚡ Remedy: Run `zqk system check --auto-remedy` to clear 2 stale locks before retrying.
   ```
