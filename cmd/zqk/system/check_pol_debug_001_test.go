package system

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/cliapp"

	"github.com/spf13/cobra"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	testkit "github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// TestPOL_DEBUG_001_ValidationIssue tests the exact validation issue observed
// with POL-DEBUG-001 and verifies it can be resolved.
//
// This test isolates the issue where POL-DEBUG-001 shows validation errors for:
// - body: Field body is required (minCount: 1)
// - category: Field category is required (minCount: 1)
// - policy_type: Field policy_type has invalid value
//
// The test verifies that:
// 1. The object can be created with all required fields
// 2. Validation passes when all fields are present
// 3. The issue is not a data loss problem but a validation cache issue
func TestPOL_DEBUG_001_ValidationIssue(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: pkgctx.SystemAccountID,
	}

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, fileStorage)
	testkit.RegisterStandardTeardown(t, testkit.TeardownOptions{
		ProjectRoot:           testRoot,
		FileStorage:           fileStorage,
		StripProcessArtifacts: true,
		WALTimeout:            20 * time.Second,
		ShutdownTimeout:       20 * time.Second,
	})
	// checkKindObjects may start background cache work; join via cond (not wall-clock polling) before storage teardown.
	t.Cleanup(func() {
		_ = WaitProjectCacheBackgroundWork(context.Background(), testRoot)
		auditCliCtx := cli.ContextForProjectAndProfile(testRoot, "test")
		if ab := GetAuditEventBuffer(auditCliCtx); ab != nil {
			_, _ = ab.Flush()
		}
	})

	// Create policy with all required fields (exact replica of POL-DEBUG-001)
	obj := map[string]any{
		objects.FieldKeyID:            "POL-DEBUG-001",
		objects.FieldKeyKind:          "policy",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Test-First Debugging Policy (v1.0)",
		objects.FieldKeyPolicyType:    "requirement",
		objects.FieldKeyCategory:      "code_quality",
		objects.FieldKeyBody:          "**Policy governing the approach to debugging issues in the ZQK system.\nWhen encountering hangs, crashes, unexpected behavior, or performance issues,\ndevelopers must write tests first to isolate and reproduce the issue before\nattempting fixes. This prevents wasted time, improves efficiency, and ensures\nissues are properly understood before resolution.**",
		objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:     pkgctx.SystemAccountID,
		objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:     pkgctx.SystemAccountID,
		objects.FieldKeyNamespaceID:   paths.KernelNamespaceID,
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		"enforcement_level":           "required",
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, obj, objects.ObjectStatusActive)
	storage.FlushAllOrFail(t, testRoot)

	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	// Step 1: Check object - should pass validation
	results, err := checkKindObjects(checkCtx, &cobra.Command{}, "policy", []string{"POL-DEBUG-001"})
	if err != nil {
		t.Fatalf("Failed to check objects: %v", err)
	}

	// Verify no validation errors for required fields
	for _, result := range results {
		if result.ObjectID == "POL-DEBUG-001" {
			for _, issue := range result.Issues {
				if issue.Category == "instance_validation" {
					if strings.Contains(issue.Message, "body") && strings.Contains(issue.Message, "required") {
						t.Errorf("Unexpected validation error for body field: %s", issue.Message)
					}
					if strings.Contains(issue.Message, "category") && strings.Contains(issue.Message, "required") {
						t.Errorf("Unexpected validation error for category field: %s", issue.Message)
					}
					if strings.Contains(issue.Message, "policy_type") && strings.Contains(issue.Message, "invalid value") {
						t.Errorf("Unexpected validation error for policy_type field: %s", issue.Message)
					}
				}
			}
		}
	}

	// Step 2: Read object back and verify all fields are present
	readObj, err := fileStorage.Read(ctx, secCtx, "POL-DEBUG-001")
	if err != nil {
		t.Fatalf("Failed to read object: %v", err)
	}

	// Verify all required fields are present
	if body, ok := readObj[objects.FieldKeyBody].(string); !ok || body == emptyValue {
		t.Error("Body field is missing or empty")
	}
	if category, ok := readObj[objects.FieldKeyCategory].(string); !ok || category == emptyValue {
		t.Error("Category field is missing or empty")
	}
	if policyType, ok := readObj[objects.FieldKeyPolicyType].(string); !ok || policyType == emptyValue {
		t.Error("Policy_type field is missing or empty")
	}

	// Step 3: Update object (simulating what happens during check)
	updates := map[string]any{
		objects.FieldKeyTitle: "Updated Title",
	}
	if err := fileStorage.Update(ctx, secCtx, "POL-DEBUG-001", updates); err != nil {
		t.Fatalf("Failed to update object: %v", err)
	}

	// Step 4: Re-check after update - should still pass validation
	results, err = checkKindObjects(checkCtx, &cobra.Command{}, "policy", []string{"POL-DEBUG-001"})
	if err != nil {
		t.Fatalf("Failed to re-check objects: %v", err)
	}

	// Verify no validation errors after update
	for _, result := range results {
		if result.ObjectID == "POL-DEBUG-001" {
			for _, issue := range result.Issues {
				if issue.Category == "instance_validation" {
					if strings.Contains(issue.Message, "body") && strings.Contains(issue.Message, "required") {
						t.Errorf("Validation error for body field after update (data loss?): %s", issue.Message)
					}
					if strings.Contains(issue.Message, "category") && strings.Contains(issue.Message, "required") {
						t.Errorf("Validation error for category field after update (data loss?): %s", issue.Message)
					}
					if strings.Contains(issue.Message, "policy_type") && strings.Contains(issue.Message, "invalid value") {
						t.Errorf("Validation error for policy_type field after update (data loss?): %s", issue.Message)
					}
				}
			}
		}
	}

	// Step 5: Verify file on disk has all fields (resolve path via storage; policy may be CAS or ID-based)
	filePath, err := fileStorage.GetFilePathForObject("POL-DEBUG-001", "policy")
	if err != nil {
		t.Fatalf("Failed to get file path for object: %v", err)
	}
	fileContent, err := fileutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	fileContentStr := string(fileContent)
	if !strings.Contains(fileContentStr, "body:") {
		t.Error("File on disk missing 'body:' field")
	}
	if !strings.Contains(fileContentStr, "category:") {
		t.Error("File on disk missing 'category:' field")
	}
	if !strings.Contains(fileContentStr, "policy_type:") {
		t.Error("File on disk missing 'policy_type:' field")
	}
}
