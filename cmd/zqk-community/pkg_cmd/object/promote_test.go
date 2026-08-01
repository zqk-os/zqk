package object

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/internal/bootstrap"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
)

func TestObjectPromoteCommand(t *testing.T) {
	testEnv := SetupTestEnvironment(t)

	// Community source has no on-disk docs/architecture/_internal — seed schemas via
	// the same embedded bootstrap extract path used by `system init` (temp project only).
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(nil))
	if err := bootstrap.ExtractTo(testEnv.TestRoot, logger, true); err != nil {
		t.Fatalf("bootstrap ExtractTo for lifecycles/specs: %v", err)
	}
	lifecycleDir := filepath.Join(testEnv.TestRoot, paths.ProcessInternalDir, "lifecycles")
	if st, err := os.Stat(lifecycleDir); err != nil || !st.IsDir() {
		t.Fatalf("expected lifecycles after bootstrap extract at %s: %v", lifecycleDir, err)
	}

	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("failed to load field registry: %v", err)
	}

	bliFields, err := fieldRegistry.GetFieldsForKind("backlog_item")
	if err != nil {
		t.Fatalf("failed to get backlog_item fields: %v", err)
	}

	fs, err := storage.NewFileObjectStorage(testEnv.GetTestRoot())
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	cliCtx := storage.WithCLIOperation(ctx)

	bliID := "ITEM-88888"
	bliObj := createTestObject("backlog_item", bliID, bliFields, 0)
	bliObj[objects.FieldKeyStatus] = "exploring"
	delete(bliObj, objects.FieldKeyPriority)
	delete(bliObj, objects.FieldKeyPriorityPlanRef)
	delete(bliObj, "priority_tier")
	delete(bliObj, "description")
	delete(bliObj, "acceptance_criteria")

	if err := fs.Create(cliCtx, secCtx, bliObj); err != nil {
		t.Fatalf("create backlog item: %v", err)
	}

	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"backlog_item"}); err != nil {
		t.Fatalf("ensure visible: %v", err)
	}

	// 1. Promote when preconditions are NOT met (problem statement / acceptance considerations are empty)
	cmd1 := testEnv.CreateCLICommand("object", "promote", bliID, "--allow-degraded")
	out1, err := cmd1.CombinedOutput()
	if err == nil {
		t.Fatalf("promote command should have failed because preconditions are not met: output=%s", string(out1))
	}
	t.Logf("promote output 1: %s", string(out1))

	fs, err = storage.NewFileObjectStorage(testEnv.GetTestRoot())
	if err != nil {
		t.Fatalf("reopen storage: %v", err)
	}
	got1, err := fs.Read(cliCtx, secCtx, bliID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got1[objects.FieldKeyStatus] != "exploring" {
		t.Errorf("expected status 'exploring', got '%v'", got1[objects.FieldKeyStatus])
	}

	bliObj[objects.FieldKeyDescription] = "This is a valid problem statement/description of sufficient length."
	bliObj[objects.FieldKeyAcceptanceCriteria] = []any{"Check that A works", "Check that B works"}
	bliObj[objects.FieldKeyPriority] = "high"
	if err := fs.Update(cliCtx, secCtx, bliID, bliObj); err != nil {
		t.Fatalf("update: %v", err)
	}
	_ = fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"backlog_item"})

	// Promote. Since validated preconditions are met, it should promote to "validated".
	cmd2 := testEnv.CreateCLICommand("object", "promote", bliID, "--allow-degraded")
	out2, err := cmd2.CombinedOutput()
	if err != nil {
		t.Fatalf("promote command failed: %v, output=%s", err, string(out2))
	}
	t.Logf("promote output 2: %s", string(out2))

	fs, err = storage.NewFileObjectStorage(testEnv.GetTestRoot())
	if err != nil {
		t.Fatalf("reopen storage: %v", err)
	}
	got2, err := fs.Read(cliCtx, secCtx, bliID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got2[objects.FieldKeyStatus] != "validated" {
		t.Errorf("expected status 'validated', got '%v'", got2[objects.FieldKeyStatus])
	}

	// 3. Now satisfy preconditions for "planned": set priority_plan_ref, milestone_refs, owner, and priority_tier
	updates := map[string]any{
		objects.FieldKeyPriorityPlanRef: "PLAN-TEST-PLAN",
		objects.FieldKeyMilestoneRefs:   []any{"MIL-11111"},
		objects.FieldKeyPriorityTier:    "P1",
		"owner":                         "account:system",
		objects.FieldKeyEstimatedEffort: "3d",
	}
	if err := fs.Update(cliCtx, secCtx, bliID, updates); err != nil {
		t.Fatalf("update: %v", err)
	}
	_ = fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"backlog_item"})

	// Promote again. It should promote straight to "in_progress".
	cmd3 := testEnv.CreateCLICommand("object", "promote", bliID, "--allow-degraded")
	out3, err := cmd3.CombinedOutput()
	if err != nil {
		t.Fatalf("promote command failed: %v, output=%s", err, string(out3))
	}
	t.Logf("promote output 3: %s", string(out3))

	fs, err = storage.NewFileObjectStorage(testEnv.GetTestRoot())
	if err != nil {
		t.Fatalf("reopen storage: %v", err)
	}
	got3, err := fs.Read(cliCtx, secCtx, bliID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got3[objects.FieldKeyStatus] != "in_progress" {
		t.Errorf("expected status 'in_progress', got '%v'", got3[objects.FieldKeyStatus])
	}
}
