package system

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/scheduler"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testenvroot"
	"github.com/lanceman/zqk/pkg/testkit"
)

// TestWatchdogSubagent_Conformance verifies CRIT-REDACTED:
// System integration, contract conformance, observability, and regression verification.
func TestWatchdogSubagent_Conformance(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	sourceRoot := paths.FindNearestProjectRoot(".")
	if err := testenvroot.BootstrapRoot(projectRoot, sourceRoot); err != nil {
		t.Fatalf("failed to bootstrap test root: %v", err)
	}

	provider, err := storagepkg.NewFileObjectStorageForTest(projectRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, projectRoot, provider)
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Create a matching error agent task
	task := map[string]any{
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyStatus: objects.ObjectStatusError,
	}
	if err := provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, task); err != nil {
		t.Fatalf("failed to create agent task: %v", err)
	}

	// 2. Create watchdog registration targeting error agent tasks
	reg := map[string]any{
		objects.FieldKeyKind:            "watchdog_registration",
		objects.FieldKeyStatus:          objects.ObjectStatusApproved,
		objects.FieldKeyTargetKind:      objects.KindAgentTask,
		objects.FieldKeyConditionQuery:  "status=error",
		objects.FieldKeyNotifyTargetRef: "agent_feed",
		objects.FieldKeyFrequency:       "1m",
	}
	if err := provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, reg); err != nil {
		t.Fatalf("failed to create watchdog registration: %v", err)
	}

	handler := scheduler.NewWatchdogEvaluationHandler(provider, nil, projectRoot)
	job := &scheduler.ScheduledJob{ID: "job-watchdog-conformance", JobType: scheduler.JobTypeWatchdogEvaluation}

	if err := handler.Execute(ctx, job); err != nil {
		t.Fatalf("conformance handler execution failed: %v", err)
	}
}
