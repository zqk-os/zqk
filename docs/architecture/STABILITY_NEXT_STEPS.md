# Critical Next Steps for Test and Scheduler Stability

**Last Verified:** 2026-08-31


**Context:** Post-rebuild analysis of test bundle runs, scheduler job failures, and regression run (regression-wgfix). Goal: stabilize quickly without sacrificing accuracy or correctness.

---

## 1. Errors Identified

### 1.1 Bundle 50 (scenario builder / utility tests)

- **account:test validation:** Scenario builder data loader tests created account with `id: "account:test"` but no `username`. Account spec requires `username` (Tier 2). This caused persistence errors when tests ran in parallel with others.
- **Fix applied:** Added `username: "test"` and `status: "active"` to the account objects in `scenario_builder_data_loader_test.go` (TestBuildFromDataFile_EmitsEvents and TestBuild_DataFileObjectsTakesPrecedence).
- **account:test-user / WS-TEST-001:** Cache checker timing tests (CorrectBehavior, RaceCondition) expect workstream WS-TEST-001 to reference account:test-user; both tests create that account with username. Failures were due to cache checker being cleared or reference validation order; one test explicitly expects "creation failed because cache checker was cleared" and is testing that path.

### 1.2 Scheduler job SCH-1770928712363151 (bundle 127 – pkg/storage)

- **Job:** `run_wrapper` running storage bundle (CAS/orphan cleanup tests).
- **Result:** Exit status 1 after ~8.6s.
- **Failures in bundle-127 log:**
  1. **TestCAS_GetObjectFilePathForReferenceValidation:** Failed with temp dir cleanup error: `unlinkat .../backlog: directory not empty`. Test logic may be leaving files open or not cleaning up in correct order.
  2. **TestCASOrphanCleanupQueue_CoordinatorIntegration_NonExistentFile:** "Expected callback to be called within 5 seconds" – callback timing or non-existent file path handling.

**Fixed (2026-02-15):**
- **TestCAS_GetObjectFilePathForReferenceValidation:** Call `FlushAllCASIndexesForProjectRoot(fileStorage.GetProjectRoot())` before test end so temp dir cleanup can succeed (CAS index releases file handles).
- **TestCASOrphanCleanupQueue_CoordinatorIntegration_NonExistentFile:** Use a single callback that invokes the waiter (removed race where a second callback was set after enqueue, so the worker might have already invoked the first callback).

### 1.3 Regression run (regression-wgfix)

Failure categories (no file edits here; for prioritization):

| Category | Examples | Likely cause |
|----------|----------|--------------|
| **Missing _internal in temp dir** | TestFixCommandResolution_RealData (lifecycle file not found), TestUpdateLoopProcessor_BuildCurrentValues (specs directory), TestStreamingTemplateCache_PreloadAllKindsConcurrent | Tests use `t.TempDir()` but do not copy or embed `docs/process/_internal` (lifecycles, object_specs). |
| **Reference validation in temp dir** | MIL-999, CRIT-9002, account:test-user, BLI-999 not found | Test data expects referenced objects to exist; creation order or fixture setup is wrong. |
| **Policy ID format** | invalid ID format for kind policy: POL-TEST-004 | Policy kind likely expects a different ID pattern (e.g. POL-### or different prefix). |
| **Auto-generated field “status”** | criteria/status, streaming template filters | Spec or lifecycle now marks `status` as auto-generated; tests expect it in required/tokens. |
| **Hash/registry expectations** | TestHashMismatchTailChase, TestHashMismatchForBucketedObjects | Expected hash or registry state no longer matches after code/spec changes. |
| **ParseQueryHintToFilter** | `$in` to contain slice, got nil | Parser or test expectation for `$in` operator. |
| **Integrity / CAS / violation resolution** | Multiple TestIntegrityCheck_*, TestViolationResolution_* | Temp dir setup, file moves, or resolution logic. |
| **MCP / callback / scheduler** | TestJobLock_SequentialExecution, TestStartupDeadlockWithParseError (30s timeout), TestWaitGroupPanic_Reproduce, TestParallelCreate_WithValidation | Timeouts, deadlocks, or panic handling. |

---

## 2. Critical Next Steps (Priority Order)

1. **Restart scheduler (after optional local re-run)**  
   You had not restarted the scheduler after rebuild. Restart so the daemon runs the new binary and job definitions.

2. **~~Fix the two failing storage tests in bundle 127~~** (done)  
   - **TestCAS_GetObjectFilePathForReferenceValidation:** Flush CAS index for project root before test end so cleanup succeeds.  
   - **TestCASOrphanCleanupQueue_CoordinatorIntegration_NonExistentFile:** Single callback that invokes waiter (removed race).

3. **Re-run the failing test bundles**  
   After the account fix and (when done) the storage fixes, re-run the bundles that previously failed (e.g. via `scripts/test-runner.sh` or `zqk scheduler scan-tests`) and capture logs to confirm:
   - Bundle 50 (utility): no more account:test validation errors from data loader tests.
   - Bundle 127 (storage): both CAS tests pass.

4. **Regression run: prioritize by impact**  
   - **High impact:** Tests that fail due to missing `docs/process/_internal` in temp dirs – consider a test helper that copies or embeds the minimal _internal layout (lifecycles, object_specs) into the test root so many tests can pass without per-test boilerplate.  
   - **Medium:** Policy ID format (POL-TEST-004) – align test IDs with `docs/process/_internal/object_specs` (or id_prefixes) for policy.  
   - **Medium:** Auto-generated “status” for criteria/streaming – align test expectations with current lifecycle/spec (status as auto-generated).  
   - **Lower (flaky or env-specific):** Hash/registry expectations, 30s timeout in TestStartupDeadlockWithParseError – fix or quarantine so regression run is stable.

5. **Do not change validation or spec rules for “speed”**  
   Keep account `username` required and reference validation as-is; fixes should be test data and test environment (temp dir, _internal), not weakening correctness.

6. **Orphan test process cleanup**  
   If many test processes were run and abandoned (e.g. before run_wrapper panic/reap fix): run `scripts/cleanup_orphan_test_processes.sh` to list, or `scripts/cleanup_orphan_test_processes.sh --kill` to terminate them. Use `--older-than 7200` to only target processes running longer than 2 hours. The run_wrapper now defers kill+reap so new scheduler-run tests should not leave orphans.

7. **Source of abandoned “zqk object list” processes (e.g. PIDs 25046, 25288)**  
   Those were **CLI invocations from object tests** that run without a timeout:
   - **Comprehensive tests** (`cmd/zqk/object/comprehensive_test_helper.go`): `testFilteringViaCLI`, `testSortingViaCLI`, `testGroupingViaCLI` run `exec.Command(cliBinary, "object", "list", kind, ...).CombinedOutput()`. When `kind` is `audit_event`, list can be very slow (many events, sort-by aggregated_count). If the test is interrupted or times out, the child `zqk` process is not killed (no CommandContext).
   - **fields_test.go**: `object list` (no kind) with no timeout.
   - **Fix (control going forward):** Use `exec.CommandContext(ctx, timeout)` for all such CLI invocations so the child process is killed when the context expires or the test ends. Added `runCLIWithTimeout(t, cliBinary, 60*time.Second, args...)` in comprehensive_test_helper and used it for filter/sort/group list calls; added 60s CommandContext for `object list` in fields_test.

---

## 3. Summary

- **Done:** Account `account:test` in scenario_builder_data_loader_test now includes required `username` (and status), so parallel runs should not hit that validation error for that fixture.
- **Next:** Fix or quarantine the two storage tests (GetObjectFilePathForReferenceValidation cleanup; NonExistentFile callback), then restart scheduler and re-run the failing bundles.
- **Then:** Tackle regression failures in order: _internal-in-temp-dir → policy ID format → status auto-generated → integrity/CAS/violation resolution → timeouts/panics. Use a single regression run log (e.g. regression-wgfix) and this doc to track progress.
