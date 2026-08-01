// Package testkit provides reusable test helpers intended for extraction into a shared Go
// testing library later. It composes storage/CAS/audit teardown using the canonical
// [github.com/lanceman/zqk/pkg/pipeline] builder so ordering and observability stay consistent.
//
// Typical use: [PrepareIsolatedTempProject] for full bootstrap (env, [github.com/lanceman/zqk/pkg/testenvroot.Setup],
// file storage, teardown registration), [PrepareIsolatedTempProjectForBenchmark] for the same in benchmarks,
// or [RegisterTempProjectTeardown] when you already have a root and *storage.FileObjectStorage.
package testkit
