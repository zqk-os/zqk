package scheduler

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_RunWrapperExecution_DeepCoverage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-runwrapper-exec-deep-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Setenv("ZQK_TEST_ROOT", tmpDir)
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
		_ = os.RemoveAll(tmpDir)
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	handlerIface := NewRunWrapperHandlerWithProjectRoot(sp, logger, nil, nil, tmpDir)
	h := handlerIface.(*RunWrapperHandler)
	t.Cleanup(func() { h.StopNotificationContext() })

	// 1. Nil job and empty command errors
	if err := h.executeRunWrapperCore(context.Background(), nil); err == nil {
		t.Errorf("expected error for nil job")
	}
	if err := h.executeRunWrapperCore(context.Background(), &ScheduledJob{}); err == nil {
		t.Errorf("expected error for empty command")
	}

	// 2. prepareRunWrapperExecution variations
	// Script command
	scriptJob := &ScheduledJob{
		Command:           "echo hi && echo bye",
		RetryCount:        -1, // should normalize to 0
		RetryDelaySeconds: -1, // should normalize to 5s
	}
	prepScript := h.prepareRunWrapperExecution(scriptJob)
	if !prepScript.IsShellScript {
		t.Errorf("expected isShellScript true")
	}
	if prepScript.RetryCount != 0 {
		t.Errorf("expected retryCount 0, got %d", prepScript.RetryCount)
	}
	if prepScript.RetryDelay != 5*time.Second {
		t.Errorf("expected 5s retryDelay, got %v", prepScript.RetryDelay)
	}

	// Test command with testkit timeout
	testJob := &ScheduledJob{
		Command:     "go",
		CommandArgs: []string{"test", "-run", "TestFoo", "./pkg/scheduler"},
	}
	prepTest := h.prepareRunWrapperExecution(testJob)
	if prepTest.MaxRuntimeSeconds <= 0 {
		t.Errorf("expected positive maxRuntimeSeconds for test")
	}

	// 3. warnIfTestBundleMetadataFingerprintMismatch
	mismatchJob := &ScheduledJob{
		ID: "SCH-run-test-bundle-mismatch",
		Metadata: map[string]any{
			KeyBundleCommandFingerprint: "expected-fp",
		},
	}
	// Does not panic or fail
	h.warnIfTestBundleMetadataFingerprintMismatch(mismatchJob, "go test ./...")
	h.warnIfTestBundleMetadataFingerprintMismatch(nil, "go test ./...")

	// 4. logRunWrapperExecutionStart
	h.logRunWrapperExecutionStart(scriptJob, prepScript)
	debugJob := &ScheduledJob{
		ID:       "SCH-debug",
		LogLevel: "debug",
		EnvironmentVariables: map[string]string{
			"FOO": "bar",
		},
	}
	h.logRunWrapperExecutionStart(debugJob, prepScript)
}

func TestExtended_CapDispatch_DeepCoverage(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "test-cap-dispatch-deep-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Setenv("ZQK_TEST_ROOT", tmpDir)
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
		_ = os.RemoveAll(tmpDir)
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()
	bgCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.WithAllowCoreObjectDelete(ctx), "test-cap-dispatch")
	bgCtx = pkgctx.WithPromoteOnCreate(bgCtx)

	h := &CapOrchestratorHandler{
		storage:     sp,
		logger:      logger,
		projectRoot: tmpDir,
	}

	// 1. resumePendingVerificationTasks empty planID
	if err := h.resumePendingVerificationTasks(ctx, "zqk", ""); err != nil {
		t.Errorf("expected nil for empty planID, got: %v", err)
	}

	// 2. resumePendingVerificationTasks with planID and no pending tasks
	if err := h.resumePendingVerificationTasks(ctx, "zqk", "PRI-cap-none"); err != nil {
		t.Errorf("expected nil for planID with no tasks, got: %v", err)
	}

	// 3. nestPlanWorkstreams with seeded workstreams and BLIs
	planID := "PRI-cap-nested"
	wsID := "WS-cap-1"
	if err := sp.Create(bgCtx, secCtx, map[string]any{
		objects.FieldKeyID:              wsID,
		objects.FieldKeyKind:            "workstream",
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:          "active",
		objects.FieldKeyPriorityPlanRef: planID,
		objects.FieldKeyTitle:           "Cap Workstream",
		objects.FieldKeyDescription:     "Workstream description",
	}); err != nil {
		t.Fatalf("create workstream failed: %v", err)
	}

	for i := 1; i <= 3; i++ {
		bliID := fmt.Sprintf("BLI-cap-nested-%d", i)
		wsRef := wsID
		if i == 3 {
			wsRef = "" // unassigned
		}
		if err := sp.Create(bgCtx, secCtx, map[string]any{
			objects.FieldKeyID:              bliID,
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: planID,
			objects.FieldKeyWorkstreamRef:   wsRef,
			objects.FieldKeyPriorityTier:    "P0",
			objects.FieldKeyTitle:           fmt.Sprintf("Nested BLI %d", i),
			objects.FieldKeyDescription:     "Description for nested bli",
		}); err != nil {
			t.Fatalf("create bli failed: %v", err)
		}
	}

	planObj := map[string]any{
		objects.FieldKeyID: planID,
	}
	bliMap := h.nestPlanWorkstreams(ctx, planID, planObj)
	if len(bliMap) != 3 {
		t.Errorf("expected 3 items in bliMap, got %d", len(bliMap))
	}
	wsList, ok := planObj["workstreams"].([]any)
	if !ok || len(wsList) < 1 {
		t.Errorf("expected workstreams on planObj, got %v", planObj["workstreams"])
	}
}
