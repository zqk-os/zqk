package scheduler

import (
	"context"
	"testing"
)

func TestScheduledJob_ExecutionContextAccessors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job := &ScheduledJob{ID: "job-1"}
	if job.ExecutionCancel() != nil {
		t.Fatal("expected no cancel before bind")
	}
	job.BindExecutionContext(ctx, cancel)
	if job.ExecutionCancel() == nil {
		t.Fatal("expected cancel after bind")
	}
	job.ClearExecutionContext()
	if job.ExecutionCancel() != nil {
		t.Fatal("expected cancel cleared")
	}
}
