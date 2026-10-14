# Local CI & Precommit Verification Governance Guide (`zqk ci` & `zqk precommit`)

## 1. Overview & System Mission

The ZQK Knowledge Kernel enforces fail-closed safety, invariant gate compliance, and strict code quality before commits can enter source control or integration branches. Two dedicated command groups govern pre-flight validation and local isolated test execution:

1. **Local CI Engine (`zqk ci`)**: Manages isolated staging workdirs (`.zqk/local-ci/`), executes test cases against specific commit hashes or working trees (`zqk ci checkout`, `zqk ci run`), provides inspectable status (`zqk ci status`), and cleanly cleans up staging trees (`zqk ci demote`).
2. **Precommit Gate Verification (`zqk precommit`)**: Enforces pre-commit invariants across categorized checks (`lint`, `policy`, `integrity`, `ast`), records incremental results (`zqk precommit write-result`), aggregates verdicts (`zqk precommit aggregate`), renders diagnostic reports, and verifies that commits only proceed when all gates pass (`zqk precommit status`).

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│                        LOCAL CI & PRECOMMIT GOVERNANCE ARCHITECTURE                    │
│                                                                                        │
│  Git Hook Execution                       Precommit Verification Matrix                │
│ ┌────────────────────────┐              ┌─────────────────────────────┐                │
│ │ git commit             │ ───────────► │ - lint (zqk-vet / golangci) │                │
│ └────────────────────────┘              │ - policy (DSL compliance)   │                │
│                                         │ - integrity (CAS/specs)     │                │
│                                         │ - ast (zero-violation)      │                │
│                                         └──────────────┬──────────────┘                │
│                                                        ▼                               │
│                                         ┌─────────────────────────────┐                │
│                                         │ zqk precommit aggregate     │                │
│                                         │ Result: Block / Allow       │                │
│                                         └─────────────────────────────┘                │
│                                                                                        │
│  Isolated Staging Pipeline                Local CI Engine                              │
│ ┌────────────────────────┐              ┌─────────────────────────────┐                │
│ │ zqk ci checkout --sha  │ ───────────► │ Target: .zqk/local-ci/      │                │
│ └────────────────────────┘              │ Sandbox Workdir Isolation   │                │
│                                         │ Test Suite Execution        │                │
│                                         └─────────────────────────────┘                │
└────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Local CI Engine (`zqk ci`)

### 2.1 Basic Usage & Subcommands

```bash
# Check status of local CI workdir and recorded SHA
zqk ci status

# Check out a target SHA into the isolated local CI sandbox
zqk ci checkout --sha <commit-sha>

# Execute local CI test suites within the sandboxed environment
zqk ci run

# Execute checkout only without triggering the full test suite
zqk ci run --checkout-only

# Demote/clean local CI sandbox artifacts
zqk ci demote
```

### 2.2 Operational Behavior

- **Sandboxed Directory**: Operates within `.zqk/local-ci/workdir`, ensuring that tests run in an isolated environment without mutating active developer state.
- **Fail-Closed Root Resolution**: Uses project-aware context resolution (`cli.GetContext(cmd)`) falling back to root discovery.
- **SHA Tracking**: Persists `SOURCE_SHA` in the local CI root to trace which commit state was checked out and evaluated.

---

## 3. Precommit Gate Governance (`zqk precommit`)

### 3.1 Subcommand Hierarchy

- `zqk precommit status`: Displays current precommit verdict (Allowed / Blocked), listing all blocking categories and check summaries.
- `zqk precommit aggregate`: Aggregates category results into the master `.zqk/state/precommit.json` summary.
- `zqk precommit write-result`: Records a structured category verdict (`--category`, `--ok`, `--summary`, `--blocking`, `--details`).
- `zqk precommit write-result-from-last`: Ingests staged output from category tools and aggregates verdicts.
- `zqk precommit clear`: Clears recorded category verdicts to reset precommit state.
- Diagnostic Reports:
  - `zqk precommit lint-report`: Displays latest lint check diagnostics.
  - `zqk precommit policy-report`: Displays latest policy compliance diagnostics.
  - `zqk precommit integrity-report`: Displays CAS and spec integrity diagnostics.

### 3.2 Gate Invariant Lifecycle

```
[Tool Run] ──► [write-result --category <cat> --ok=<bool>] ──► [aggregate] ──► [status check]
```

If any category with `--blocking=true` reports `ok=false`, `zqk precommit status` returns `Block: true` and prevents `git commit` from proceeding.

---

## 4. Verification & Testing

Both `cmd/zqk/ci` and `cmd/zqk/precommit` maintain >=80.0% statement test coverage with comprehensive unit and CLI execution tests:
- `cmd/zqk/ci/ci_cmd_test.go`: Validates subcommand hierarchy, clean and configured status checks, demote lifecycle, missing root handling, and checkout flags.
- `cmd/zqk/precommit/precommit_cli_test.go`: Validates category writes, last-result ingestion, aggregation, status blocking verdicts, and diagnostic reports.
