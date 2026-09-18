package scheduler

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestWatchdogEvaluationHandler(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer env.Cleanup()

	provider := env.Storage.(storagepkg.ObjectStorageProvider)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create an active watchdog_registration
	registration := map[string]any{
		objects.FieldKeyKind:            "watchdog_registration",
		objects.FieldKeyStatus:          objects.ObjectStatusApproved,
		objects.FieldKeyTargetKind:      objects.KindAgentTask,
		objects.FieldKeyConditionQuery:  "status=error",
		objects.FieldKeyNotifyTargetRef: "agent_feed",
		objects.FieldKeyFrequency:       "1m",
	}
	if err := provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, registration); err != nil {
		t.Fatalf("failed to create watchdog_registration: %v", err)
	}

	handler := NewWatchdogEvaluationHandler(provider, nil, ".")
	job := &ScheduledJob{ID: "job-watchdog-1", JobType: JobTypeWatchdogEvaluation}

	if err := handler.Execute(ctx, job); err != nil {
		t.Fatalf("handler execution failed: %v", err)
	}

	// Right now the core just loops and invokes zqk query, checking output length.
	// The test completes successfully if there are no panics and standard logic runs.
	// Since zqk query runs out of process, testing full end-to-end event emission
	// inside unit tests will require the full graph environment to be present,
	// which setupSchedulerCompleteTestEnvironment provides.
}

func TestWatchdogEvaluationHandler_NilJob(t *testing.T) {
	handler := NewWatchdogEvaluationHandler(nil, nil, ".")
	if err := handler.Execute(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil job")
	}
}
