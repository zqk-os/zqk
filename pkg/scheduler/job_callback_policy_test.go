package scheduler

import (
	"context"
	"testing"
)

func TestShouldInvokeConfiguredJobCallback(t *testing.T) {
	t.Parallel()

	if !shouldInvokeConfiguredJobCallback(context.Background(), &ScheduledJob{ExecutionMode: jobExecutionModeOneTime}) {
		t.Fatal("manual one-time job must invoke configured callback")
	}
	if shouldInvokeConfiguredJobCallback(context.Background(), &ScheduledJob{ExecutionMode: "recurring"}) {
		t.Fatal("timer/recurring job must not invoke configured callback without callback trigger origin")
	}
	preCommitCtx := ContextWithTriggerOrigin(context.Background(), TriggerOriginPreCommit)
	if !shouldInvokeConfiguredJobCallback(preCommitCtx, &ScheduledJob{ExecutionMode: "recurring"}) {
		t.Fatal("pre-commit trigger must invoke configured callback")
	}
}
