// Package testkit provides reusable test helpers intended for extraction into a shared Go
// testing library later. It composes storage/CAS/audit teardown using the canonical
// [github.com/lanceman/zqk/pkg/pipeline] builder so ordering and observability stay consistent.
//
// Typical use: [PrepareIsolatedTempProject] for full bootstrap (env, [github.com/lanceman/zqk/pkg/testenvroot.Setup],
// file storage, teardown registration), [PrepareIsolatedTempProjectForBenchmark] for the same in benchmarks,
// or [RegisterTempProjectTeardown] when you already have a root and *storage.FileObjectStorage.
//
// Scheduler daemons started by tests must be time-bounded: use [StartBoundCLIScheduler] for CLI
// `scheduler start`, or [BoundSchedulerStartContext] for in-process Start(ctx). Prefer job
// completion callbacks ([WaitChanOrBound] / [WaitChanOrBoundWithCap]) to stop early; the bound is
// the hard ceiling against orphan processes. See docs/architecture/SCHEDULER_TEST_DAEMON_BOUNDS.md.
package testkit
