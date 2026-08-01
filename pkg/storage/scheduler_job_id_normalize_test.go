package storage

import (
	"strings"
	"testing"
)

func TestHasNestedSchedulerJobChurnID(t *testing.T) {
	if !hasNestedSchedulerJobChurnID("SCH-1-scheduler-job-SCH-2-scheduler-job-SCH-3-backlog-item-ITEM-1") {
		t.Fatal("expected nested churn id")
	}
	if hasNestedSchedulerJobChurnID("SCH-1771987486-backlog-item-ITEM-850") {
		t.Fatal("expected simple churn id to be non-nested")
	}
	if hasNestedSchedulerJobChurnID("") {
		t.Fatal("empty id")
	}
}

func TestNormalizeSchedulerJobIDIfRecursive(t *testing.T) {
	long := "SCH-1-scheduler-job-SCH-2-scheduler-job-SCH-3-backlog-item-ITEM-1"
	out := normalizeSchedulerJobIDIfRecursive("scheduler_job", long)
	if out == long {
		t.Fatalf("expected normalization, got %q", out)
	}
	if !strings.HasPrefix(out, "SCH-") || !strings.Contains(out, "-h-") {
		t.Fatalf("unexpected form: %q", out)
	}
	if normalizeSchedulerJobIDIfRecursive("scheduler_job", "SCH-9-backlog-item-ITEM-1") != "SCH-9-backlog-item-ITEM-1" {
		t.Fatal("should not change non-nested id")
	}
}
