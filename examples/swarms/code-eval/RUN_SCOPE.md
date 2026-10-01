# CEF Run Scope — PRI-CODE_EVAL (code-eval swarm)

Status: certified-for-execution
Run ID basis: branch `agent/ATK-1790855833454500000-99caca84`, worktree `zqk-public-candidate-8a1fd554`
Lead: lead_integrator (PER-CODE_EVAL-LEAD_INTEGRATOR), account ACC-SWARM-WORKER (lane=doer)

## 1. Codebase Inventory (live, verified via AST observer + repo inspection)

| Dimension | Evidence |
| :-- | :-- |
| Module | `github.com/zqk-os/zqk` (go.mod) |
| Go version | go 1.26.0, toolchain go1.26.6 |
| Working tree | clean, at 9f2b35a2 (refactor(systemcheck): asynccheck decoupling, PR #400) |
| Kernel status | 3,594 objects; scheduler running; 12 warnings; **partial check only** (refs skipped — not an authoritative health verdict) |
| CLI entrypoint | `cmd/zqk/main.go` (main + init verified), codegen builders in `pkg/cli/bldr_cli_cmd_v1` |
| Convergence surface | `pkg/scheduler/convergence_cef_diamond.go` — `BuildCEFDiamondMeasureResult` / `cefMeasureFromRows`, matrix alias `cef_diamond_scorecard` (EvaluationSurfaceCEFDiamondScorecard via `pkg/quality.ResolveMatrixForCLI`) |
| Supporting packages | `pkg/agentfeed` (doctor), `pkg/events` (inspectors), `pkg/drifthotspots`, `pkg/convergerollup` (outcome enum), `pkg/errfmt`, `pkg/objects`, `pkg/scheduler` (evaluation_surface_adapters) |
| Test assets | `pkg/scheduler/convergence_cef_diamond_test.go` incl. `TestCEFMeasureFromRows_BLI_CEF_R28_REMEASURE_001_EnvelopeMeetsFloor` (dual-seat remeasure gate) |

Dependencies of note (go.mod): cobra, testify, x/tools (AST), x/sync, gopsutil, fsnotify, neo4j driver (mesh tier), goleak (goroutine hygiene).

## 2. Tool Availability (this sandbox)

**Available (use these, not bash, for object/file ops):**
- `zqk_observer_search` — live Go AST lookup (name/path/kind). ✔ verified working.
- `zqk_read_code` / `zqk_write_code` / `zqk_write_file` — source read/write with AST validation. ✔ verified.
- `zqk_execute_bash` — **allowlisted only**: `go test`, `go build`, `go vet`, `go fmt`, `go mod tidy|download`, `make`. 1m hard timeout (observed two 65s timeouts on cold-cache `go build/test`; plan for warmed-cache second run).

**Unavailable / gated:**
- `zqk_object_get` on plan root objects → permission denied for worker account (fail-closed; do not retry).
- `zqk_object_list` → not exposed in this context.
- Network fetchers, arbitrary shells, `ls` → soft-blocked.
- MCP catalog browsing discouraged; use the fixed function set.

## 3. Diamond Scale — Envelope Definition (from CEF surface, no invention)

Source of truth: `pkg/scheduler/convergence_cef_diamond.go`.

- **Axes**: matrix rows of `cef_diamond_scorecard`; envelope seat = `seat_role=envelope`.
- **Envelope measure**: `envelope_min` (min axis grade across completed diamond seat rows for the session).
- **Floor**: `thresholds.min_axis_grade`, default **4**.
- **Completion gates** (fail-closed outcomes, enum from `pkg/convergerollup`):
  1. `package_complete=pending` on any row → `dual_seat_remesure_in_flight` blocker (R28 remeasure mandate); outcome = Divergence if last complete `envelope_min < floor`, else Ambiguous.
  2. Last complete envelope row with `envelope_min >= min_axis_grade` → outcome = **Convergence**, `ready_for_session_completion=true`; next = verify desired_end_state axes / VDS evaluate.
  3. `envelope_min < min_axis_grade` → outcome = Divergence, blocker `envelope_min_below_<n>`; remediates weakest axes, dual-seat remeasure.
  4. No complete envelope row → blocker `no_complete_envelope_row`, Ambiguous.
- **Convergence certified only when gate 2 holds AND no pending seats.**

## 4. Execution Scope for This Run

### In scope
- **t-eval-boundaries** (evaluator-architect): AST-verified boundary audit of `pkg/scheduler` (convergence/CEF surface), `pkg/quality` matrix resolution, `pkg/events` inspector contracts, `pkg/cli/bldr_cli_cmd_v1` builder isolation; confirm open-core decoupling (no mesh-tier imports leaked into single-cell paths).
- **t-eval-antagonist** (evaluator-antagonist): failure-mode probed on `cefMeasureFromRows` (row ordering, `assessed_at` string comparison, `na` vs `package_complete` semantics, sessionID collision), `pkg/agentfeed/doctor.go` degradation paths, error-swallowing per BLI-SYNC-LOOP-ERROR-001 discipline.
- **Verification gates** (TDD policy): `make lint` (hygiene suite), `go vet ./pkg/agentfeed/... ./pkg/scheduler/...` (cold-cache aware), `go test -short -count=1 ./pkg/scheduler/ ./pkg/agentfeed/` — budget ~2× the 65s sandbox timeout (warm cache) per target.
- Matrix fill: dual-seat axis rows for this run recorded against session/plan `PRI-CODE_EVAL`; envelope row grade written per axis evidence.

### Out of scope (explicit)
- Scheduler `health.jsonl` test-bundle fingerprints (noted Out of scope in the CEF surface itself).
- Mesh/cluster tier: neo4j driver paths, P2P wire protocol — single-cell core only.
- Any mutation of system-managed kernel objects (created_at, status, PRI root) — worker permission model forbids it; deliverables are files only.
- Codegen rebuild (`make codegen`) — no spec/IDL changes are produced by this run.

### Convergence certification criteria (exit)
1. Both task seats report `package_complete=yes` with axis grades grounded in AST/test evidence above.
2. Envelope seat grade = min(axis grades); `envelope_min >= 4` (min_axis_grade default).
3. `make lint` + targeted `go vet`/`go test -short` green (or failures objectified as residual BLIs with blockers, never swallowed).
4. Lead (this agent) certifies Convergence outcome with `session_completion_blocked_reasons=[]` and records it in the swarm feed; any Divergence → next action = remediates per CEF table, NOT session close (anti-idle: milestone transition, continue autonomous).

## 5. Execution Constraints & Hygiene
- Branch-scoped only (`agent/ATK-...` worktree); PR-only promotion; never push to main; worktree does not hop repo parents (`worktree_gate`).
- Goroutine hygiene (POL resource hygiene): tests use goleak; no orphaned daemons from `scheduler start` in sandbox.
- Fail-closed: permission denials and tool 65s timeouts are recorded as environment facts, not retried >1×; partial kernel health check is flagged as non-authoritative.
- Deterministic ordering: antagonist seat depends on boundaries seat (per `swarm.yaml` tasks).
