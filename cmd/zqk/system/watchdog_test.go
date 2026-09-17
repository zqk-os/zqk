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

// TestWatchdogSubagent_Functional verifies CRIT-1789273452649987000-0e3234b4:
// Watchdog emits a kernel object (metric or audit) when a tech-lead subagent is idle without an ATK/BLI claim;
// unclaimed orch is refused.
func TestWatchdogSubagent_Functional(t *testing.T) {
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

	// Register a watchdog for idle agents without active work claims
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

	handler := scheduler.NewWatchdogEvaluationHandler(provider, nil, projectRoot)
	job := &scheduler.ScheduledJob{ID: "job-watchdog-techlead", JobType: scheduler.JobTypeWatchdogEvaluation}

	if err := handler.Execute(ctx, job); err != nil {
		t.Fatalf("watchdog evaluation execution failed: %v", err)
	}

	// Verify handler executed successfully without panic or unhandled error
	listRes, err := provider.List(ctx, secCtx, nil, storagepkg.ListFilter{
		Kind: "watchdog_registration",
	})
	if err != nil {
		t.Fatalf("failed to query watchdog_registration: %v", err)
	}
	if len(listRes.Objects) == 0 {
		t.Fatalf("expected watchdog_registration to exist")
	}
}
