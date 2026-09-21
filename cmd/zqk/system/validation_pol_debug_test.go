package system

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"github.com/zqk-os/zqk/internal/cli"

	"github.com/spf13/cobra"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

// TestValidation_POL_DEBUG_001_ExactReplica tests validation with exact POL-DEBUG-001 structure
// This helps debug why POL-DEBUG-001 shows validation errors
// Do not use t.Parallel(): same *testing.T uses t.Setenv(ZQK_TEST_ROOT).
func TestValidation_POL_DEBUG_001_ExactReplica(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-SYSTEM",
	}

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, fileStorage)
	_ = storage.InitializeGlobalBufferWithConfig(testRoot, secCtx)

	t.Cleanup(func() {
		_ = WaitProjectCacheBackgroundWork(context.Background(), testRoot)

		auditCliCtx := cli.ContextForProjectAndProfile(testRoot, "test")
		if ab := GetAuditEventBuffer(auditCliCtx); ab != nil {
			_, _ = ab.Flush()
		}

		q := caspkg.GetListingIndexWriteQueueForProjectRoot(testRoot)
		if q != nil {
			_ = q.FlushAll(5 * time.Second)
			_ = q.Shutdown()
		}
		_ = storage.FlushAllListingIndexesForProjectRoot(testRoot)
		_ = storage.WaitForWALProcessing(testRoot, 15*time.Second)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = fileStorage.Shutdown(shutdownCtx)
		resetDir, err := fileutil.MkdirTemp("", "zqk-audit-global-reset")
		if err == nil {
			defer fileutil.RemoveAll(resetDir)
			_ = storage.TearDownGlobalAuditBufferForTestProjectRoot(testRoot, resetDir, secCtx)
		} else {
			storage.FlushGlobalAuditBufferForProjectRoot(testRoot)
		}
	})

	// Create policy with exact POL-DEBUG-001 structure
	obj := map[string]any{
		objects.FieldKeyID:            "POL-CODE-999",
		objects.FieldKeyKind:          "policy",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Test-First Debugging Policy",
		objects.FieldKeyPolicyType:    "requirement",
		objects.FieldKeyCategory:      "code_quality",
		objects.FieldKeyBody:          "**Policy governing the approach to debugging issues in the ZQK system.\nWhen encountering hangs, crashes, unexpected behavior, or performance issues,\ndevelopers must write tests first to isolate and reproduce the issue before\nattempting fixes. This prevents wasted time, improves efficiency, and ensures\nissues are properly understood before resolution.**",
		objects.FieldKeyDescription:   "Policy governing the approach to debugging issues in the ZQK system. When encountering hangs, crashes, unexpected behavior, or performance issues, developers must write tests first to isolate and reproduce the issue before attempting fixes. This prevents wasted time, improves efficiency, and ensures issues are properly understood before resolution.",
		"enforcement_level":           "required",
		objects.FieldKeyCreatedAt:     "2026-01-01T00:00:00Z",
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:     "2026-01-02T00:00:00Z",
		objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		objects.FieldKeyNamespaceID:   "zqk:kernel",
	}

	// Create the object
	if err := fileStorage.Create(ctx, secCtx, obj); err != nil {
		if strings.Contains(err.Error(), "unknown object kind: policy") {
			t.Skipf("Policy kind not available in test env (e.g. object_specs not present under bundler): %v", err)
		}
		t.Fatalf("Failed to create object: %v", err)
	}

	// Check for validation errors
	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	results, err := checkKindObjects(checkCtx, &cobra.Command{}, "policy", []string{"POL-CODE-999"})
	if err != nil {
		t.Fatalf("Failed to check objects: %v", err)
	}

	// Collect validation errors
	validationErrors := []string{}
	for _, result := range results {
		if result.ObjectID == "POL-CODE-999" {
			for _, issue := range result.Issues {
				if issue.Category == "instance_validation" {
					validationErrors = append(validationErrors, issue.Message)
					t.Logf("Validation error: %s", issue.Message)
				}
			}
		}
	}

	// Check for specific errors that POL-DEBUG-001 shows
	hasPolicyTypeError := false
	hasCategoryError := false
	hasBodyError := false
	for _, errMsg := range validationErrors {
		if strings.Contains(errMsg, "policy_type") && strings.Contains(errMsg, "invalid value") {
			hasPolicyTypeError = true
		}
		if strings.Contains(errMsg, "category") && strings.Contains(errMsg, "required") {
			hasCategoryError = true
		}
		if strings.Contains(errMsg, "body") && strings.Contains(errMsg, "required") {
			hasBodyError = true
		}
	}

	if hasPolicyTypeError {
		t.Errorf("policy_type validation failed - 'requirement' should be valid enum value")
	}
	if hasCategoryError {
		t.Errorf("category validation failed - field exists and has value")
	}
	if hasBodyError {
		t.Errorf("body validation failed - field exists and has content")
	}

	// If we have any of these errors, the validation is broken
	if hasPolicyTypeError || hasCategoryError || hasBodyError {
		// Debug: Read the actual file to see what's there
		policyDir := filepath.Join(testRoot, paths.ProcessPoliciesDir)
		objectFile := filepath.Join(policyDir, "POL-CODE-999.yaml")
		fileContent, _ := fileutil.ReadFile(objectFile)
		t.Logf("File content (first 1000 chars):\n%s", string(fileContent[:min(1000, len(fileContent))]))
	}
}
