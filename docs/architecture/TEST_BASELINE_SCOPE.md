# Test baseline scope (storage, cache, reporting, scheduler, spec)

**Last Verified:** 2026-08-31


**Purpose:** Before large refactors (e.g. WAL shrink, minimal records), run a focused baseline so we know the state of primary touch points. Full suite can follow once this is green.

**Primary concerns:** storage, cache updates, reporting, scheduler, spec generation and tangential touch points.

**Baseline packages:**

| Area        | Package(s)           | Notes                                      |
|------------|----------------------|--------------------------------------------|
| Storage    | `./pkg/storage`      | Stream, CAS, WAL, list/count, change journal |
| Cache      | (in storage)         | list_cache, cas_cache_update, high_volume_event_cache |
| Reporting  | `./cmd/zqk/reports`  | Reports CLI, quick_test                    |
| Scheduler  | `./pkg/scheduler`     | Handlers, aggregation, storage update, job loading |
| Spec/gen   | `./pkg/specbuilder`  | Builders, codegen, integration             |

**Run baseline (output to file):**

```bash
go test ./pkg/storage ./pkg/scheduler ./pkg/specbuilder ./cmd/zqk/reports -timeout 600s -count=1 2>&1 | tee .zqk/logs/tests/baseline-storage-cache-reporting-scheduler-spec.log
```

**Inspect result:**

```bash
tail -100 .zqk/logs/tests/baseline-storage-cache-reporting-scheduler-spec.log
# or: grep -E '^(FAIL|--- FAIL|PASS|ok)' .zqk/logs/tests/baseline-storage-cache-reporting-scheduler-spec.log
```

**After stabilization:** run full suite (e.g. `./scripts/test-runner.sh` or `zqk scheduler scan-tests`) and compare.
