package object

import (
	"bytes"
	stdctx "context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	clitool "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func executeObjectCommand(t *testing.T, projectRoot string, provider storage.ObjectStorageProvider, args ...string) (string, error) {
	rootCmd := clitool.NewCommandBuilder("zqk").Build()
	if rootCmd.PersistentFlags().Lookup("format") == nil {
		rootCmd.PersistentFlags().String("format", "table", "Output format")
	}
	objectCmd := NewObjectCmd()
	rootCmd.AddCommand(objectCmd)

	fullArgs := append([]string{"object"}, args...)
	rootCmd.SetArgs(fullArgs)

	var buf bytes.Buffer
	testCtx := setTestCLIContext(t, rootCmd, projectRoot, provider)
	testCtx = pkgctx.WithCommandOutputWriter(testCtx, &buf)

	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetContext(testCtx)

	var propagate func(c *cobra.Command)
	propagate = func(c *cobra.Command) {
		c.SetContext(testCtx)
		c.SetOut(&buf)
		c.SetErr(&buf)
		for _, child := range c.Commands() {
			propagate(child)
		}
	}
	propagate(rootCmd)

	err := rootCmd.ExecuteContext(testCtx)
	return buf.String(), err
}

func TestInProcess_ObjectGet_Matrix(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedBulkUpdateSupportObjects(t, provider)

	secCtx := pkgctx.NewSystemSecurityContext()
	obj := plannedBLI("BLI-GET-001", "Get Test Item")
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, obj, objectStatusPlanned)

	// 1. Get single object in table format
	out, err := executeObjectCommand(t, tempDir, provider, "get", "BLI-GET-001")
	if err != nil {
		t.Fatalf("object get failed: %v", err)
	}
	if !strings.Contains(out, "BLI-GET-001") {
		t.Errorf("expected BLI-GET-001 in output, got: %s", out)
	}

	// 2. Get single object in JSON format
	outJSON, err := executeObjectCommand(t, tempDir, provider, "get", "BLI-GET-001", "--format", "json")
	if err != nil {
		t.Fatalf("object get json failed: %v", err)
	}
	if !strings.Contains(outJSON, `"id": "BLI-GET-001"`) && !strings.Contains(outJSON, `"id":"BLI-GET-001"`) {
		t.Errorf("expected json id in output, got: %s", outJSON)
	}

	// 3. Get non-existent object
	_, errMissing := executeObjectCommand(t, tempDir, provider, "get", "BLI-NONEXISTENT")
	if errMissing == nil {
		t.Error("expected error for non-existent object")
	}
}

func TestInProcess_ObjectList_Matrix(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedBulkUpdateSupportObjects(t, provider)

	secCtx := pkgctx.NewSystemSecurityContext()
	obj1 := plannedBLI("BLI-LST-001", "List Test Item 1")
	obj2 := plannedBLI("BLI-LST-002", "List Test Item 2")
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, obj1, objectStatusPlanned)
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, obj2, objectStatusPlanned)

	// 1. List backlog items
	out, err := executeObjectCommand(t, tempDir, provider, "list", pplanKindBacklogItem)
	if err != nil {
		t.Fatalf("object list failed: %v", err)
	}
	if !strings.Contains(out, "BLI-LST-001") || !strings.Contains(out, "BLI-LST-002") {
		t.Errorf("expected items in list output, got: %s", out)
	}

	// 2. List in JSON format
	outJSON, err := executeObjectCommand(t, tempDir, provider, "list", pplanKindBacklogItem, "--format", "json")
	if err != nil {
		t.Fatalf("object list json failed: %v", err)
	}
	if !strings.Contains(outJSON, "BLI-LST-001") {
		t.Errorf("expected json output to contain items, got: %s", outJSON)
	}

	// 3. List with limit
	outLim, err := executeObjectCommand(t, tempDir, provider, "list", pplanKindBacklogItem, "--limit", "1")
	if err != nil {
		t.Fatalf("object list with limit failed: %v", err)
	}
	if outLim == "" {
		t.Error("expected non-empty output with limit")
	}
}

func TestInProcess_ObjectCount_Matrix(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedBulkUpdateSupportObjects(t, provider)

	secCtx := pkgctx.NewSystemSecurityContext()
	obj := plannedBLI("BLI-CNT-001", "Count Test Item")
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, obj, objectStatusPlanned)

	// 1. Count single kind
	out, err := executeObjectCommand(t, tempDir, provider, "count", pplanKindBacklogItem)
	if err != nil {
		t.Fatalf("object count failed: %v", err)
	}
	if !strings.Contains(out, "1") {
		t.Errorf("expected count 1 in output, got: %s", out)
	}

	// 2. Count single kind with group-by
	outGrp, err := executeObjectCommand(t, tempDir, provider, "count", pplanKindBacklogItem, "--group-by", "status")
	if err != nil {
		t.Fatalf("object count group-by failed: %v", err)
	}
	if !strings.Contains(outGrp, "planned") && !strings.Contains(outGrp, "1") {
		t.Errorf("expected planned in grouped output, got: %s", outGrp)
	}

	// 3. Count all kinds
	outAll, err := executeObjectCommand(t, tempDir, provider, "count", "--all-kinds")
	if err != nil {
		t.Fatalf("object count all kinds failed: %v", err)
	}
	if outAll == "" {
		t.Error("expected non-empty all kinds count")
	}
}

func TestInProcess_ObjectBulkGet_Matrix(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedBulkUpdateSupportObjects(t, provider)

	secCtx := pkgctx.NewSystemSecurityContext()
	obj1 := plannedBLI("BLI-BGT-001", "Bulk Get Item 1")
	obj2 := plannedBLI("BLI-BGT-002", "Bulk Get Item 2")
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, obj1, objectStatusPlanned)
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, obj2, objectStatusPlanned)

	// 1. Bulk get by IDs
	out, err := executeObjectCommand(t, tempDir, provider, "bulk", "get", "--ids", "BLI-BGT-001,BLI-BGT-002")
	if err != nil {
		t.Fatalf("bulk get failed: %v", err)
	}
	if !strings.Contains(out, "BLI-BGT-001") && !strings.Contains(out, "Success: 2") {
		t.Errorf("expected bulk get items in output, got: %s", out)
	}

	// 2. Bulk get with JSON format
	outJSON, err := executeObjectCommand(t, tempDir, provider, "bulk", "get", "--ids", "BLI-BGT-001", "--format", "json")
	if err != nil {
		t.Fatalf("bulk get json failed: %v", err)
	}
	if !strings.Contains(outJSON, "BLI-BGT-001") {
		t.Errorf("expected json output to contain item id, got: %s", outJSON)
	}
}

func TestInProcess_ObjectDelete_Matrix(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedBulkUpdateSupportObjects(t, provider)

	secCtx := pkgctx.NewSystemSecurityContext()
	obj := plannedBLI("BLI-DEL-001", "Delete Test Item")
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, obj, objectStatusPlanned)

	// 1. Delete without reason code fails for core kind
	_, errNoReason := executeObjectCommand(t, tempDir, provider, "delete", "BLI-DEL-001")
	if errNoReason == nil {
		t.Error("expected error for delete without reason code")
	}

	// 2. Delete with valid reason code and cascade succeeds
	out, err := executeObjectCommand(t, tempDir, provider, "delete", "BLI-DEL-001", "--cascade", "--reason-code", "valid reason for deleting this core object item")
	if err != nil {
		t.Fatalf("delete with reason code failed: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output for delete")
	}

	// 3. Verify object is gone
	if _, err := provider.Read(stdctx.Background(), secCtx, "BLI-DEL-001"); err == nil {
		t.Error("expected object to be deleted")
	}
}

func TestInProcess_ObjectBulkDelete_Matrix(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedBulkUpdateSupportObjects(t, provider)

	secCtx := pkgctx.NewSystemSecurityContext()
	obj1 := plannedBLI("BLI-BDEL-001", "Bulk Del 1")
	obj2 := plannedBLI("BLI-BDEL-002", "Bulk Del 2")
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, obj1, objectStatusPlanned)
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, obj2, objectStatusPlanned)

	out, err := executeObjectCommand(t, tempDir, provider, "bulk", "delete", "--ids", "BLI-BDEL-001,BLI-BDEL-002", "--cascade", "--reason-code", "valid reason for deleting these core object items")
	if err != nil {
		t.Fatalf("bulk delete failed: %v", err)
	}
	if !strings.Contains(out, "success: 2") && !strings.Contains(out, "total: 2") {
		t.Errorf("expected bulk delete summary in output, got: %s", out)
	}

	if _, err := provider.Read(stdctx.Background(), secCtx, "BLI-BDEL-001"); err == nil {
		t.Error("expected BLI-BDEL-001 to be deleted")
	}
	if _, err := provider.Read(stdctx.Background(), secCtx, "BLI-BDEL-002"); err == nil {
		t.Error("expected BLI-BDEL-002 to be deleted")
	}
}

func TestInProcess_ObjectPromoteAndPark_Matrix(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedBulkUpdateSupportObjects(t, provider)

	secCtx := pkgctx.NewSystemSecurityContext()
	obj := plannedBLI("BLI-PRO-001", "Promote Test Item")
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, obj, objectStatusPlanned)

	if _, err := executeObjectCommand(t, tempDir, provider, "promote"); err == nil {
		t.Error("expected error for promote without arguments")
	}

	if _, err := executeObjectCommand(t, tempDir, provider, "park"); err == nil {
		t.Error("expected error for park without arguments")
	}

	if _, err := executeObjectCommand(t, tempDir, provider, "promote", "BLI-NONEXISTENT"); err == nil {
		t.Error("expected error promoting non-existent object")
	}
}
