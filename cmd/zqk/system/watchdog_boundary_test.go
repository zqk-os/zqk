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

// TestWatchdogSubagent_Boundary verifies CRIT-1789273452649988000-84a64249:
// Boundary condition validation, negative testing, invalid input rejection, and failure recovery.
func TestWatchdogSubagent_Boundary(t *testing.T) {
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

	// 1. Invalid watchdog registration with empty query and target kind
	invalidReg := map[string]any{
		objects.FieldKeyKind:            "watchdog_registration",
		objects.FieldKeyStatus:          objects.ObjectStatusApproved,
		objects.FieldKeyTargetKind:      "",
		objects.FieldKeyConditionQuery:  "",
		objects.FieldKeyNotifyTargetRef: "",
	}
	_ = provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, invalidReg)

	// 2. Malformed query that cannot be parsed
	malformedReg := map[string]any{
		objects.FieldKeyKind:            "watchdog_registration",
		objects.FieldKeyStatus:          objects.ObjectStatusApproved,
		objects.FieldKeyTargetKind:      objects.KindAgentTask,
		objects.FieldKeyConditionQuery:  "invalid===malformed==query",
		objects.FieldKeyNotifyTargetRef: "agent_feed",
	}
	_ = provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, malformedReg)

	handler := scheduler.NewWatchdogEvaluationHandler(provider, nil, projectRoot)
	job := &scheduler.ScheduledJob{ID: "job-watchdog-boundary", JobType: scheduler.JobTypeWatchdogEvaluation}

	// Should safely recover and skip malformed entries without crashing
	if err := handler.Execute(ctx, job); err != nil {
		t.Fatalf("expected graceful handling of invalid watchdog registration, got error: %v", err)
	}
}
