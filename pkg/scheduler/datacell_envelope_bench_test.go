// Benchmark for policy dry-run on synthetic project root (no scheduler daemon).
//
// Use -run '^$' so only benchmarks run; otherwise go test runs the full pkg/scheduler
// test suite first (including slow integration tests) and may hit -timeout.
//
// Run: go test ./pkg/scheduler -run '^$' -bench 'BenchmarkStreamOrganism' -benchmem -count=3 -timeout 60s
package scheduler

import (
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
)

func BenchmarkStreamOrganism_DryRunDataCellEnvelopePolicy(b *testing.B) {
	root := b.TempDir()
	b.ReportAllocs()
	for b.Loop() {
		_, err := DryRunDataCellEnvelopePolicy(root, datacell.ProfileStream)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStreamOrganism_PolicyEvaluate_DataCellEnvelope measures [PolicyEngine.Evaluate] only
// (one synthetic project root). Separates policy/fs hot path from [DryRunDataCellEnvelopePolicy],
// which also allocates registry/engine and builds the report each iteration.
func BenchmarkStreamOrganism_PolicyEvaluate_DataCellEnvelope(b *testing.B) {
	root := b.TempDir()
	reg := NewJobStateRegistry(root)
	pe := NewPolicyEngine(reg, root, nil)
	job := &ScheduledJob{
		ID:       "SCH-envelope-policy-dryrun",
		JobType:  JobTypeRunWrapper,
		Category: CategoryDataCellEnvelope,
	}
	const jobID = "SCH-envelope-policy-dryrun"
	b.ReportAllocs()
	for b.Loop() {
		_, err := pe.Evaluate(jobID, job)
		if err != nil {
			b.Fatal(err)
		}
	}
}
