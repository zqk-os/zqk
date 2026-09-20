package system

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/systemcheckwake"
)

func TestBuildSystemCheckWakeSummary(t *testing.T) {
	root := t.TempDir()
	results := []CheckResult{
		{ObjectID: "A", Status: objects.ObjectStatusError, Issues: []Issue{{Tier: 1}, {Tier: 2}}},
		{ObjectID: "B", Status: objects.ObjectStatusInProgress, Issues: []Issue{{Tier: 3}, {Tier: 4}}},
	}
	sum := buildSystemCheckWakeSummary(root, results)
	if sum.ErrorStatusObjects != 1 {
		t.Fatalf("error_status=%d", sum.ErrorStatusObjects)
	}
	if sum.BlockingIssues != 1 || sum.Warnings != 1 || sum.Informational != 1 || sum.Recommendations != 1 {
		t.Fatalf("tiers: %+v", sum)
	}
	reasons := systemcheckwake.Evaluate(sum, systemcheckwake.DefaultConfig())
	if len(reasons) < 3 {
		t.Fatalf("expected multiple trips, got %#v", reasons)
	}
}
