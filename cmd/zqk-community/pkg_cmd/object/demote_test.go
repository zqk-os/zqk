package object

import (
	"os"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestObjectDemoteCommand(t *testing.T) {
	testEnv := SetupTestEnvironment(t)

	srcDir := filepath.Join(testEnv.ProjectRoot, "docs/process/_internal/lifecycles")
	destDir := filepath.Join(testEnv.TestRoot, "docs/process/_internal/lifecycles")
	if err := fileutil.EnsureDir(destDir); err != nil {
		t.Fatalf("failed to create dest lifecycles dir: %v", err)
	}

	files, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatalf("failed to read src lifecycles dir: %v", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		srcFile := filepath.Join(srcDir, file.Name())
		destFile := filepath.Join(destDir, file.Name())
		data, err := os.ReadFile(srcFile)
		if err != nil {
			t.Fatalf("read file %s: %v", srcFile, err)
		}
		if err := fileutil.WriteStandardFile(destFile, data); err != nil {
			t.Fatalf("write file %s: %v", destFile, err)
		}
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
	bliObj[objects.FieldKeyStatus] = "in_progress"
	bliObj[objects.FieldKeyDescription] = "This is a valid problem statement/description of sufficient length."
	bliObj[objects.FieldKeyAcceptanceCriteria] = []any{"Check that A works", "Check that B works"}
	bliObj[objects.FieldKeyPriority] = "high"
	bliObj[objects.FieldKeyPriorityPlanRef] = "PLAN-TEST-PLAN"
	bliObj[objects.FieldKeyMilestoneRefs] = []any{"MIL-11111"}
	bliObj[objects.FieldKeyPriorityTier] = "P1"
	bliObj["owner"] = "account:system"

	if err := fs.Create(cliCtx, secCtx, bliObj); err != nil {
		t.Fatalf("create backlog item: %v", err)
	}

	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	if err := fs.EnsureCLIObjectMutationVisible(flushCtx, []string{"backlog_item"}); err != nil {
		t.Fatalf("ensure visible: %v", err)
	}

	cmd1 := testEnv.CreateCLICommand("object", "demote", bliID, "--allow-degraded")
	out1, err := cmd1.CombinedOutput()
	if err != nil {
		t.Fatalf("demote command failed: %v, output=%s", err, string(out1))
	}
	t.Logf("demote output 1: %s", string(out1))

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
}
