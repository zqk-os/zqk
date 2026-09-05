# Scheduler test daemon bounds

**Last Verified:** 2026-08-31


**Problem:** Tests (and shell harnesses) that start a real scheduler daemon can leave orphans if a wait hangs or the process exits without `scheduler stop`. Incident 2026-08-10: an 8h daemon under `/tmp/zqk-clean-clone-test`.

**Policy:** Any test that fires up a scheduler must be **time-bounded** to the maximum reasonable duration for that test to complete. Prefer **callbacks** (job `callback_on_completion`, condition channels) to initiate teardown early; the bound is a hard ceiling, not the intended run length.

## Defaults (Go)

| Mode | Helper | Default ceiling |
|------|--------|-----------------|
| CLI `scheduler start` (detached) | `testkit.StartBoundCLIScheduler` | **3m** (`DefaultMaxCLISchedulerDaemonLifetime`) |
| In-process `Scheduler.Start(ctx)` | `testkit.BoundSchedulerStartContext` | **2m** (`DefaultMaxInProcessSchedulerLifetime`) |

Helpers register `t.Cleanup` → `scheduler stop --force` (CLI) or cancel the start context (in-process). A CLI watchdog also force-stops when the bound expires.

Wait helpers that encourage callback-driven teardown:

- `testkit.WaitChanOrBound`
- `testkit.WaitChanOrBoundWithCap` (e.g. 60s for one job under a longer daemon ceiling)

## Required patterns

```go
h := testkit.StartBoundCLIScheduler(t, testkit.BoundCLISchedulerOpts{
    CLIBinary: cli, ProjectRoot: root, Env: env, PreferCallback: true,
})
if err := testkit.WaitChanOrBoundWithCap(done, h.BoundCtx, 60*time.Second); err != nil {
    t.Fatal(err)
}
// t.Cleanup already stops --force
```

```go
ctx, cancel := testkit.BoundSchedulerStartContext(t, parent, 0)
defer cancel() // also registered via t.Cleanup
go sched.Start(ctx)
// assert, then cancel() early
```

## Shell / ephemeral trees

- Prefer `scripts/run-clean-clone-verify.sh` (EXIT trap stop + optional wall-clock via `CLEAN_CLONE_VERIFY_MAX_SECONDS`).
- Do not leave `configure-restored-project.sh` scheduler start running on disposable `/tmp` trees.
- Orphan sweep: `scripts/cleanup_orphan_test_processes.sh`, `scripts/cleanup-ephemeral-zqk-trees.sh`.

## Tracking

- `REDACTED` — ephemeral teardown + bound helper adoption.
- `TRACK:` on `pkg/testkit/scheduler_daemon_bound.go` for remaining ad-hoc start/stop call sites.

## Checklist

See `docs/architecture/PRE_CHANGE_CHECKLIST.md` §6 (scheduler daemon bound bullet) and `.cursor/rules/tests-go-test-timeout.mdc`.
