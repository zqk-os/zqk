# L-CODE-QUALITY Narrative: Code Quality & Consistency Evaluation

**Run Package:** `2026-09-25-GEMINI_RUN`  
**Lens:** `L-CODE-QUALITY`  
**Density Class:** `D-HIGH` + `D-LOW` (Budget: Exhaustive D-HIGH + Top-10 D-LOW; Emitted: 11)  
**Primary Axes:** `MNT` (Maintainability), `RDB` (Readability)  
**Secondary Axes:** `ROB` (Robustness), `REL` (Reliability), `TST` (Testability), `MOD` (Modularity), `OPS` (Operability)  
**Evidence Baseline:** Git SHA `392fb153b506c0ecee2047a3ddb05a327a161f4e` (8,452 tracked files; 7,341 Go files; 1,100,614 Go LOC)

---

## 1. Executive Assessment

The ZQK codebase demonstrates a high level of domain ambition, rich type modeling, and comprehensive test harness infrastructure. However, exhaustive mechanical analysis (D-HIGH) combined with architectural quality inspection (D-LOW) reveals systemic code quality anti-patterns, severe error handling degradation, dual-tree code clone redundancy, and widespread idiomatic drift that impair developer velocity and runtime resilience:

1. **Systemic Error Swallowing via Blank Identifier Proliferation (`F-MNT-ERR-BLANK-SUPPRESSION`, High):** A repository-wide census identified 2,894 occurrences of the blank identifier `_ = ` in non-test Go source files. In numerous critical locations—including JSON parameter unmarshaling (`pkg/swarm/mcp_client.go:193`, `pkg/contextevents/json_codec.go:53`), append-only kernel mutations (`pkg/swarm/metabolism/exhaust.go:120, 153`), and HTTP response streaming (`pkg/scheduler/handlers_callback_listener.go:367`)—errors are silently dropped without logging or handling. This practice is institutionalized in `.golangci.yml`, where `errcheck` explicitly sets `check-blank: false` and `check-type-assertions: false`, blinding the build system to unhandled errors and panic-prone type assertions.
2. **Silent Failure Suppression via Checked `nilerr` Returns (`F-MNT-ERR-NILERR-DISCARD`, High):** Static analysis using the `nilerr` analyzer uncovered over 50 instances in `cmd/` alone where functions explicitly execute `if err != nil`, yet return `nil` or `obj, nil`. Prominent examples include `cmd/zqk/ambient/daemon.go:279` (swallowing stream reader errors), `cmd/zqk/scheduler/scheduler_daemon.go:84` (swallowing status collection failures), and `cmd/zqk/object/update.go:439`. This anti-pattern produces false-positive success states where operations fail silently without warning operators or upstream callers.
3. **Broken Error Wrapping Chains Using `%v` / `%s` Instead of `%w` (`F-MNT-ERR-CHAIN-SEVERANCE`, Moderate):** 50 production Go locations format underlying error variables into format strings using `%v` or `%s` rather than Go 1.13's `%w` wrapping verb. In critical modules such as `cmd/zqk/app/auth_middleware.go:179` (`unauthorized: storage not available to validate session: %v`), `cmd/zqk/system/git.go:169`, and `cmd/zqk/object/demote.go:50`, formatting with `%v` destroys the causal error chain, preventing callers from inspecting error types or sentinel errors via `errors.Is()` or `errors.As()`.
4. **Magic File Permissions, Hardcoded Paths, and Proliferation of Duplicate Literals (`F-MNT-LITERALS-PATHS-PERMS`, Moderate):** Execution of `./scripts/check-hardcoded-paths-and-perms-repo.sh` detected 20,962 literal violations: 20,709 repeated duplicate string literals, 199 magic permissions (e.g. `0644`, `0755`, `0600`), and 54 raw path literals containing hardcoded `.zqk` references. Notably, `pkg/quality/cef_pack_builder.go:155, 167, 168` embeds raw strings `".zqk/runs/code-eval-latest"`, `".zqk/process/"`, and `".zqk/audit/"` directly in Go code instead of referencing centralized `pkg/paths` constants.
5. **Hardcoded Canonical CLI Name Literals Violating Repository Hygiene (`F-MNT-HYGIENE-CLI-LITERALS`, Moderate):** Automated verification via `./bin/zqk-vet -suite hygiene` failed with 4 high-priority errors in `cmd/zqk/swarm/run.go` (lines 388, 411, 443) and `pkg/swarm/pack/manifest.go` (line 338). User-facing output and execution fallbacks hardcode the literal string `"zqk"` (`zqk agent orchestrate`, `zqk agent status`, `zqk new swarm`, and fallback `exe = "zqk"` on line 418), bypassing canonical brand token helpers (`paths.CLIUsage`, `CLIInvocation`, `RewriteCanonicalCLIInvocations`).
6. **Systematic Code Clones and Dual-Tree Redundancy in CLI Builders (`F-MNT-CLONES-DUAL-TREE`, High):** The codebase maintains two competing directory trees for command builders: `pkg/cli/bldr_cli_cmd_v1` (669 files) and `pkg/cli/command_builders/bldr_cli_cmd_v1` (209 files). A byte-for-byte comparison identified 168 identical files duplicated across both trees. In addition, intra-package clone analysis with `dupl` detected 18 copy-paste duplicates (e.g., `supervise_command_builder.go` vs `mcp_supervise_command_builder.go`). This redundant tree structure multiplies maintenance overhead and confuses code generation tooling.
7. **Uncontrolled Panic Anti-Pattern in Library Subsystems (`F-MNT-PANIC-IN-LIBRARIES`, High):** An AST audit identified 19 `panic()` invocations in non-test `pkg/` library packages. Rather than returning idiomatic Go errors to callers, packages panic during runtime operations: `pkg/mcp/permission_format_helpers.go:87` panics if object kind examples cannot be formatted; `pkg/mcp/role_aware_prompts.go:270` panics if role guidance fails; `pkg/config/property.go:92` panics on a missing config property; and `pkg/kernelcas/compose/evaluate.go:43, 154` panics if `WarmDefaultRegistry` returns an error. Crashing library callers terminates long-running daemons and breaks system resilience.
8. **Detached Root Contexts and Un-cancellable Sleep Loops (`F-MNT-CONTEXT-SLEEP-HYGIENE`, Moderate):** Auditing context and concurrency hygiene revealed 216 occurrences of detached `context.Background()` / `context.TODO()` in library packages, alongside 42 synchronous `time.Sleep()` invocations. In `pkg/mcp/proxy.go:153-172`, `connectionLoop` invokes `time.Sleep(1 * time.Second)` unconditionally inside a loop without selecting on `ctx.Done()`, preventing prompt daemon shutdown. In `pkg/mesh/webhook_listener.go:43`, incoming webhook requests invoke `store.Update(context.Background(), ...)` rather than using the request context, severing cancellation propagation.
9. **Orphaned Uncompilable Test Source Causing Package Build Failure (`F-MNT-ORPHANED-TEST-BUILD-FAIL`, Moderate):** Running `go test ./pkg/testing` fails compilation due to an undefined function reference in `pkg/testing/package_timeouts_test.go:10`. The test invokes `GetMinTimeoutSecondsForPackage(dir, ...)`, a symbol that exists nowhere in the repository. This broken test causes `FAIL github.com/zqk-os/zqk/pkg/testing [build failed]`, indicating absent test compilation gates in pre-commit hooks.
10. **Pervasive Exported Symbol Package Name Stutter (`F-MNT-NAMING-PACKAGE-STUTTER`, Low):** An AST census identified 270 exported symbols repeating their enclosing package name as a prefix (e.g. `pipeline.PipelineRouter`, `metabolism.MetabolismEngine`, `sentinel.SentinelPayload`, `vitality.VitalityMonitor`). This creates redundant stuttering identifiers upon import and violates Go's canonical naming philosophy.
11. **Monolithic Package Sprawl and Single-File Complexity Outliers (`F-MNT-MONOLITH-PACKAGE-OUTLIERS`, Moderate):** Severe package sprawl has created monolithic "god packages": `pkg/cli/bldr_cli_cmd_v1` (669 files), `pkg/storage` (627 files), `cmd/zqk/system` (478 files), and `pkg/scheduler` (447 files). In parallel, single-file complexity outliers exceed 1,500 to 2,000 LOC (`cmd/zqk/ui/model.go` at 2,004 LOC, `cmd/zqk/test/dashboard.go` at 1,893 LOC, `cmd/zqk/ui/views.go` at 1,785 LOC), severely degrading local reasoning and reviewability.

---

## 2. Findings Summary Table

| Finding ID | Title | Severity | Grade | Axes | Density |
|------------|-------|----------|-------|------|---------|
| `F-MNT-ERR-BLANK-SUPPRESSION` | Systemic Error Swallowing via Blank Identifier and Relaxed Linter Configuration | high | E2 | MNT, ROB, REL | D-HIGH |
| `F-MNT-ERR-NILERR-DISCARD` | Silent Failure Suppression via Checked nilerr Returns in CLI and System Daemons | high | E2 | REL, ROB, MNT | D-HIGH |
| `F-MNT-CLONES-DUAL-TREE` | Systematic Code Clones and Dual-Tree Redundancy in CLI Command Builders | high | E2 | MNT, MOD | D-HIGH |
| `F-MNT-PANIC-IN-LIBRARIES` | Uncontrolled Panic Anti-Pattern in Library and Subsystem Initialization | high | E2 | REL, ROB, MNT | D-HIGH |
| `F-MNT-ERR-CHAIN-SEVERANCE` | Broken Error Wrapping Chains Using %v/%s Formatting Instead of %w | moderate | E2 | MNT, RDB | D-HIGH |
| `F-MNT-LITERALS-PATHS-PERMS` | Magic File Permissions, Hardcoded Paths, and Proliferation of Duplicate Literals | moderate | E2 | MNT, ROB | D-HIGH |
| `F-MNT-HYGIENE-CLI-LITERALS` | Hardcoded Canonical CLI Name Literals Violating Repository Hygiene Rules | moderate | E2 | MNT, OPS | D-HIGH |
| `F-MNT-CONTEXT-SLEEP-HYGIENE` | Detached Root Contexts and Un-cancellable Sleep Loops in Service Pipelines | moderate | E2 | REL, ROB, MNT | D-HIGH |
| `F-MNT-ORPHANED-TEST-BUILD-FAIL` | Orphaned Uncompilable Test Source Causing Package Build Failure in pkg/testing | moderate | E2 | TST, MNT | D-HIGH |
| `F-MNT-MONOLITH-PACKAGE-OUTLIERS` | Monolithic Package Sprawl and Single-File Complexity Outliers | moderate | E2 | MNT, MOD, RDB | D-LOW |
| `F-MNT-NAMING-PACKAGE-STUTTER` | Pervasive Exported Symbol Package Name Stutter Across Core Domain Packages | low | E2 | RDB, MNT | D-LOW |

---

## 3. Structural & Architectural Hotspots

### 3.1 Error Handling & Concurrency Hygiene Failure Modes
Refer to diagram `diagrams/D-CODE-QUALITY-01.md` ("Error Handling Anti-Patterns and Propagation Failures in ZQK Pipelines") for structural illumination.

The evaluation identified a compounding chain of error degradation:
1. **Suppression at Source:** Core operations discard returned errors via `_ = ` (e.g. `_ = r.streamA.RecordKernelMutation(...)` in `pkg/swarm/metabolism/exhaust.go:120`), meaning disk write failures or schema rejections are unobserved.
2. **False Success Inversion:** CLI subcommands test `if err != nil`, but proceed to `return nil` (e.g. `cmd/zqk/ambient/daemon.go:279`), signalling successful execution to the shell and upstream callers despite internal abortion.
3. **Type Destruction:** When errors are wrapped, 50 instances format via `fmt.Errorf("...: %v", err)` (e.g. `cmd/zqk/app/auth_middleware.go:179`), stripping the `Unwrap() error` interface and preventing structured handling via `errors.Is` / `errors.As`.
4. **Un-cancellable Blocking:** Daemons manage retry loops with bare `time.Sleep()` instead of timer selects on `ctx.Done()`, causing process teardown and context cancellation to block indefinitely.
5. **Brittle Panics:** Rather than propagating errors through the call hierarchy, library packages abort the process via `panic()`, converting recoverable configuration mismatches into daemon crashes.

### 3.2 Dual-Tree Redundancy & Command Builder Clones
The CLI layer suffers from duplicate code generation pipelines:
- `pkg/cli/bldr_cli_cmd_v1/` contains 669 command builders actively utilized by CLI commands.
- `pkg/cli/command_builders/bldr_cli_cmd_v1/` contains 209 command builders, of which 168 are byte-for-byte identical duplicates.
- The generator in `scripts/generate_command_builders.go` and `pkg/zqkdev/generate_command_builders.go` contains conflicting defaults, allowing code divergence between generated artifacts.

---

## 4. Key Recommendations

1. **Tighten Linter Error Configuration (`F-MNT-ERR-BLANK-SUPPRESSION`, `F-MNT-ERR-NILERR-DISCARD`, `F-MNT-ERR-CHAIN-SEVERANCE`):**
   - Update `.golangci.yml` to enable `nilerr` and `errorlint`.
   - Set `errcheck.check-blank: true` and `errcheck.check-type-assertions: true`.
   - Enforce `%w` formatting across all `fmt.Errorf` and `errfmt.Errorf` calls.
2. **Purge Orphaned Builder Tree and De-duplicate Clones (`F-MNT-CLONES-DUAL-TREE`):**
   - Delete `pkg/cli/command_builders/` and consolidate all command builder generation exclusively into `pkg/cli/bldr_cli_cmd_v1/`.
   - Parameterize duplicate builder pairs (`supervise` vs `mcp_supervise`, `trigger` vs `job_trigger`).
3. **Eliminate Library Panics and Restore Idiomatic Error Returns (`F-MNT-PANIC-IN-LIBRARIES`):**
   - Refactor `pkg/mcp/permission_format_helpers.go`, `pkg/mcp/role_aware_prompts.go`, `pkg/config/property.go`, and `pkg/kernelcas/compose/evaluate.go` to return `(T, error)` tuples instead of invoking `panic()`.
4. **Fix Orphaned Test and Integrate Compile-Only Pre-Commit Gates (`F-MNT-ORPHANED-TEST-BUILD-FAIL`):**
   - Remove or implement `pkg/testing/package_timeouts_test.go`.
   - Add `go test -run=^$ ./...` to pre-commit git hooks to guarantee no uncompilable test files can be committed.
5. **Remediate Hardcoded Paths, Permissions, and CLI Tokens (`F-MNT-LITERALS-PATHS-PERMS`, `F-MNT-HYGIENE-CLI-LITERALS`):**
   - Run `./scripts/check-hardcoded-paths-and-perms-repo.sh . --fix` to automate migration to `paths.FilePerm*` and `paths.ProjectDataDir`.
   - Fix the 4 hygiene failures in `cmd/zqk/swarm/run.go` and `pkg/swarm/pack/manifest.go` using `paths.CLIInvocation()` and `paths.RewriteCanonicalCLIInvocations()`.
6. **Refactor Stuttering Exported Identifiers (`F-MNT-NAMING-PACKAGE-STUTTER`):**
   - Rename stuttering exported symbols across `pkg/pipeline`, `pkg/swarm/metabolism`, `pkg/sentinel`, and `pkg/vitality` to follow canonical Go naming conventions (`pipeline.Router`, `metabolism.Engine`, `sentinel.Payload`, `vitality.Monitor`).
