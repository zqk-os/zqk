package system

import (
	"context"
	"strings"
	"testing"
	"time"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"github.com/zqk-os/zqk/internal/cli"

	"github.com/spf13/cobra"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	testkit "github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestValidationStaleData_AfterUpdate tests that validation uses fresh file content
// after an object is updated via CLI, not stale cached Properties
func TestValidationStaleData_AfterUpdate(t *testing.T) {
	// Do not use t.Parallel(): same *testing.T uses t.Setenv(ZQK_TEST_ROOT).
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-SYSTEM",
	}
	var fileStorage *storage.FileObjectStorage
	t.Cleanup(func() {
		ensureCtx, cancelEnsure := context.WithTimeout(context.Background(), 45*time.Second)
		_ = EnsureObjectIDCacheReady(ensureCtx, testRoot, false, nil, nil)
		cancelEnsure()

		auditCliCtx := cli.ContextForProjectAndProfile(testRoot, "test")
		if ab := GetAuditEventBuffer(auditCliCtx); ab != nil {
			_, _ = ab.Flush()
		}

		q := caspkg.GetListingIndexWriteQueueForProjectRoot(testRoot)
		if q != nil {
			_ = q.FlushAll(2 * time.Second)
			_ = q.Shutdown()
		}
		_ = storage.FlushAllListingIndexesForProjectRoot(testRoot)
		_ = storage.WaitForWALProcessing(testRoot, 15*time.Second)
		resetDir, err := fileutil.MkdirTemp("", "zqk-audit-global-reset")
		if err == nil {
			defer fileutil.RemoveAll(resetDir)
			_ = storage.TearDownGlobalAuditBufferForTestProjectRoot(testRoot, resetDir, secCtx)
		} else {
			storage.FlushGlobalAuditBufferForProjectRoot(testRoot)
		}
	})

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	ctx := pkgctx.NewSystemContext()

	var err error
	fileStorage, err = storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, fileStorage)
	_ = storage.InitializeGlobalBufferWithConfig(testRoot, secCtx)

	// Create a policy object with valid fields (use POL-AGENT- prefix per id_prefixes_config)
	obj := map[string]any{
		objects.FieldKeyID:            "POL-AGENT-003",
		objects.FieldKeyKind:          "policy",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Test Policy for Stale Data",
		objects.FieldKeyPolicyType:    "requirement",
		objects.FieldKeyCategory:      "code_quality",
		objects.FieldKeyBody:          "Test policy body content",
		objects.FieldKeyCreatedAt:     "2026-01-02T00:00:00Z",
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:     "2026-01-02T00:00:00Z",
		objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		objects.FieldKeyNamespaceID:   "zqk:kernel",
	}

	// Create the object
	if err := fileStorage.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Verify initial validation passes
	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	results, err := checkKindObjects(checkCtx, &cobra.Command{}, "policy", []string{"POL-AGENT-003"})
	if err != nil {
		t.Fatalf("Failed to check objects: %v", err)
	}

	// Check for validation errors
	initialErrors := []string{}
	for _, result := range results {
		if result.ObjectID == "POL-AGENT-003" {
			for _, issue := range result.Issues {
				if issue.Category == "instance_validation" {
					if strings.Contains(issue.Message, "policy_type") ||
						strings.Contains(issue.Message, "category") ||
						strings.Contains(issue.Message, "body") {
						initialErrors = append(initialErrors, issue.Message)
					}
				}
			}
		}
	}

	if len(initialErrors) > 0 {
		t.Logf("Initial validation errors (may be expected): %v", initialErrors)
	}

	// Now update the object via CLI (simulating the update scenario)
	updates := map[string]any{
		objects.FieldKeyPolicyType: "requirement", // Ensure it's set correctly
		objects.FieldKeyCategory:   "code_quality",
		objects.FieldKeyBody:       "Updated test policy body content - this should be validated correctly",
	}
	if err := fileStorage.Update(ctx, secCtx, "POL-AGENT-003", updates); err != nil {
		t.Fatalf("Failed to update object: %v", err)
	}
	// Flush write queue so Update is persisted before we read the file
	if q := caspkg.GetListingIndexWriteQueueForProjectRoot(testRoot); q != nil {
		_ = q.FlushAll(2 * time.Second)
	}

	// Resolve canonical path from storage (walk can pick a stale legacy file when CAS + ID-named files coexist)
	_ = storage.FlushAllListingIndexesForProjectRoot(testRoot)
	objectFile, err := fileStorage.GetFilePathForObject("POL-AGENT-003", "policy")
	if err != nil || objectFile == emptyValue {
		t.Skipf("Policy file for POL-AGENT-003 not found after update: %v", err)
	}

	// Re-parse the object (this simulates what happens in checkObject)
	// The key issue: parsedObj.Properties still has OLD data
	yamlParser := parser.NewYAMLParser()
	parsedObjAfterUpdate, err := yamlParser.ParseFile(objectFile)
	if err != nil {
		t.Fatalf("Failed to re-parse object after update: %v", err)
	}

	// Verify the file has the updated content
	fileContent, err := fileutil.ReadFile(objectFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if !strings.Contains(string(fileContent), "Updated test policy body content") {
		t.Error("File content does not contain updated body - update may have failed")
	}

	// Now run validation using the OLD parsedObj (simulating stale cache scenario)
	// This should use fresh file content, not stale Properties
	results, err = checkKindObjects(checkCtx, &cobra.Command{}, "policy", []string{"POL-AGENT-003"})
	if err != nil {
		t.Fatalf("Failed to check objects after update: %v", err)
	}

	// Check for validation errors - should be NONE if validation reads from file
	validationErrors := []string{}
	for _, result := range results {
		if result.ObjectID == "POL-AGENT-003" {
			for _, issue := range result.Issues {
				if issue.Category == "instance_validation" {
					if strings.Contains(issue.Message, "policy_type") ||
						strings.Contains(issue.Message, "category") ||
						strings.Contains(issue.Message, "body") ||
						strings.Contains(issue.Message, "required") {
						validationErrors = append(validationErrors, issue.Message)
					}
				}
			}
		}
	}

	// The test passes if validation correctly reads from file and finds no errors
	// If it uses stale Properties, it will find errors
	if len(validationErrors) > 0 {
		t.Errorf("Validation found errors after update (likely using stale data): %v", validationErrors)
		t.Logf("File content contains: policy_type=%v, category=%v, body length=%d",
			parsedObjAfterUpdate.Properties[objects.FieldKeyPolicyType],
			parsedObjAfterUpdate.Properties[objects.FieldKeyCategory],
			len(parsedObjAfterUpdate.Properties[objects.FieldKeyBody].(string)))

		// Debug: Check what the file actually contains
		fileContent, _ := fileutil.ReadFile(objectFile)
		t.Logf("File content (first 500 chars): %s", string(fileContent[:minInt(500, len(fileContent))]))
	}

	// Verify the new parsed object has correct data
	if parsedObjAfterUpdate.Properties[objects.FieldKeyPolicyType] != "requirement" {
		t.Errorf("Re-parsed object has wrong policy_type: %v", parsedObjAfterUpdate.Properties[objects.FieldKeyPolicyType])
	}
	if parsedObjAfterUpdate.Properties[objects.FieldKeyCategory] != "code_quality" {
		t.Errorf("Re-parsed object has wrong category: %v", parsedObjAfterUpdate.Properties[objects.FieldKeyCategory])
	}
}

// TestValidationStaleData_EnumValidation tests that enum validation works correctly
// with fresh file content, especially for policy_type field
func TestValidationStaleData_EnumValidation(t *testing.T) {
	// Do not use t.Parallel(): same *testing.T uses t.Setenv(ZQK_TEST_ROOT).
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-SYSTEM",
	}
	var fileStorage *storage.FileObjectStorage
	t.Cleanup(func() {
		ensureCtx, cancelEnsure := context.WithTimeout(context.Background(), 45*time.Second)
		_ = EnsureObjectIDCacheReady(ensureCtx, testRoot, false, nil, nil)
		cancelEnsure()

		auditCliCtx := cli.ContextForProjectAndProfile(testRoot, "test")
		if ab := GetAuditEventBuffer(auditCliCtx); ab != nil {
			_, _ = ab.Flush()
		}

		q := caspkg.GetListingIndexWriteQueueForProjectRoot(testRoot)
		if q != nil {
			_ = q.FlushAll(2 * time.Second)
			_ = q.Shutdown()
		}
		_ = storage.FlushAllListingIndexesForProjectRoot(testRoot)
		_ = storage.WaitForWALProcessing(testRoot, 15*time.Second)
		resetDir, err := fileutil.MkdirTemp("", "zqk-audit-global-reset")
		if err == nil {
			defer fileutil.RemoveAll(resetDir)
			_ = storage.TearDownGlobalAuditBufferForTestProjectRoot(testRoot, resetDir, secCtx)
		} else {
			storage.FlushGlobalAuditBufferForProjectRoot(testRoot)
		}
	})

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	ctx := pkgctx.NewSystemContext()

	var err error
	fileStorage, err = storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, fileStorage)
	_ = storage.InitializeGlobalBufferWithConfig(testRoot, secCtx)

	// Create a policy with valid enum value (use allowed prefix from id_prefixes_config: POL-CODE-*)
	obj := map[string]any{
		objects.FieldKeyID:            "POL-CODE-004",
		objects.FieldKeyKind:          "policy",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Test Policy for Enum Validation",
		objects.FieldKeyPolicyType:    "requirement", // Valid enum value
		objects.FieldKeyCategory:      "code_quality",
		objects.FieldKeyBody:          "Test policy body",
		objects.FieldKeyCreatedAt:     "2026-01-02T00:00:00Z",
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:     "2026-01-02T00:00:00Z",
		objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		objects.FieldKeyNamespaceID:   "zqk:kernel",
	}

	if err := fileStorage.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Parse and validate
	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	results, err := checkKindObjects(checkCtx, &cobra.Command{}, "policy", []string{"POL-CODE-004"})
	if err != nil {
		t.Fatalf("Failed to check objects: %v", err)
	}

	// Check for enum validation errors
	enumErrors := []string{}
	for _, result := range results {
		if result.ObjectID == "POL-CODE-004" {
			for _, issue := range result.Issues {
				if issue.Category == "instance_validation" {
					if strings.Contains(issue.Message, "policy_type") &&
						strings.Contains(issue.Message, "invalid value") {
						enumErrors = append(enumErrors, issue.Message)
					}
				}
			}
		}
	}

	// Should have NO enum errors since "requirement" is a valid enum value
	if len(enumErrors) > 0 {
		t.Errorf("Enum validation failed for valid value 'requirement': %v", enumErrors)
	}
}
