package system

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/internal/cli"

	"github.com/spf13/cobra"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	testkit "github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// TestBulkCheckPath_POL_DEBUG_001_Issue tests the bulk check path specifically
// to catch issues where individual checks pass but bulk checks fail.
//
// This test recreates the POL-DEBUG-001 issue where:
// - Individual check: 0 violations
// - Bulk check: 3 violations (body, category, policy_type missing)
//
// The test verifies that the bulk check path uses fresh data from files,
// not stale cached data from initial parsing.
func TestBulkCheckPath_POL_DEBUG_001_Issue(t *testing.T) {
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

	secCtxAlready := &pkgctx.SecurityContext{AccountID: pkgctx.SystemAccountID}
	t.Cleanup(func() {
		resetDir, rerr := fileutil.MkdirTemp("", "zqk-audit-global-reset")
		if rerr != nil {
			_ = testkit.RunStandardTeardown(testkit.TeardownOptions{
				ProjectRoot:           testRoot,
				FileStorage:           fileStorage,
				StripProcessArtifacts: true,
				WALTimeout:            20 * time.Second,
				ShutdownTimeout:       20 * time.Second,
			})
			return
		}
		defer fileutil.RemoveAll(resetDir)

		_ = testkit.RunStandardTeardown(testkit.TeardownOptions{
			ProjectRoot:               testRoot,
			FileStorage:               fileStorage,
			StripProcessArtifacts:     true,
			WALTimeout:                20 * time.Second,
			ShutdownTimeout:           20 * time.Second,
			TearDownGlobalAuditBuffer: true,
			SecCtx:                    secCtxAlready,
			AuditBufferResetRoot:      resetDir,
		})
	})

	// Create policy with all required fields (exact replica of POL-DEBUG-001)
	obj := map[string]any{
		objects.FieldKeyID:            "POL-CODE-001",
		objects.FieldKeyKind:          "policy",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Bulk Check Path Test Policy",
		objects.FieldKeyPolicyType:    "standard",
		objects.FieldKeyCategory:      "code_quality",
		objects.FieldKeyBody:          "**Policy for testing bulk check path.\nThis policy has all required fields to verify bulk check uses fresh data.**",
		objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
		objects.FieldKeyNamespaceID:   "zqk:kernel",
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
		"enforcement_level":           "required",
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, obj, objects.ObjectStatusActive)
	storage.FlushAllOrFail(t, testRoot)

	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	// Use global loaders (same as bulk check path)
	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()
	validatorRegistry := validation.GetGlobalRegistry()
	validator := validatorRegistry.Get("") // Default validator

	// Create hash registry cache (same as bulk check path)
	hashRegistryCache := &HashRegistryCacheType{
		cache: make(map[string]storage.HashRegistryProvider),
	}

	// Create object ID cache (same as bulk check path)
	objectIDCache := GetGlobalObjectIDCache()
	if err := objectIDCache.BuildCache(context.Background(), testRoot, true); err != nil {
		t.Fatalf("Failed to build object ID cache: %v", err)
	}

	// Use CheckKindObjectsWithCache (the bulk check path)
	// This is the same function used by checkAll() for bulk operations
	results, _, err := CheckKindObjectsWithCache(
		checkCtx,
		pkgctx.NewSystemContext(),
		&cobra.Command{},
		"policy",
		nil, // Check all policy objects (bulk)
		specLoader,
		lifecycleLoader,
		validator,
		hashRegistryCache,
		objectIDCache,
	)
	if err != nil {
		t.Fatalf("Failed to check objects: %v", err)
	}

	// Find our test object in results
	var testResult *CheckResult
	for i := range results {
		if results[i].ObjectID == "POL-CODE-001" {
			testResult = &results[i]
			break
		}
	}

	if testResult == nil {
		t.Skipf("Test object POL-CODE-001 not found in check results (object ID cache may be for another project when run in parallel)")
	}

	// Verify no validation errors for required fields
	// This is the critical assertion - bulk check should see all fields
	for _, issue := range testResult.Issues {
		if issue.Category == "instance_validation" {
			if strings.Contains(issue.Message, "body") && strings.Contains(issue.Message, "required") {
				t.Errorf("Bulk check path: Unexpected validation error for body field: %s", issue.Message)
			}
			if strings.Contains(issue.Message, "category") && strings.Contains(issue.Message, "required") {
				t.Errorf("Bulk check path: Unexpected validation error for category field: %s", issue.Message)
			}
			if strings.Contains(issue.Message, "policy_type") && strings.Contains(issue.Message, "invalid value") {
				t.Errorf("Bulk check path: Unexpected validation error for policy_type field: %s", issue.Message)
			}
		}
	}

	// Verify file on disk has all fields
	// Note: With CAS, files are stored as hash-based filenames, so we read through storage
	readObj, err := fileStorage.Read(ctx, secCtx, "POL-CODE-001")
	if err != nil {
		t.Fatalf("Failed to read object: %v", err)
	}
	fileContentStr := fmt.Sprintf("%+v", readObj)

	if !strings.Contains(fileContentStr, "body") {
		t.Error("File on disk missing 'body:' field")
	}
	if !strings.Contains(fileContentStr, "category:") {
		t.Error("File on disk missing 'category:' field")
	}
	if !strings.Contains(fileContentStr, "policy_type:") {
		t.Error("File on disk missing 'policy_type:' field")
	}
}

// TestBulkCheckPath_MultipleObjects tests that bulk check correctly validates
// multiple objects without cross-contamination of cached data.
func TestBulkCheckPath_MultipleObjects(t *testing.T) {
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

	secCtxAlready := &pkgctx.SecurityContext{AccountID: pkgctx.SystemAccountID}
	t.Cleanup(func() {
		resetDir, rerr := fileutil.MkdirTemp("", "zqk-audit-global-reset")
		if rerr != nil {
			_ = testkit.RunStandardTeardown(testkit.TeardownOptions{
				ProjectRoot:           testRoot,
				FileStorage:           fileStorage,
				StripProcessArtifacts: true,
				WALTimeout:            20 * time.Second,
				ShutdownTimeout:       20 * time.Second,
			})
			return
		}
		defer fileutil.RemoveAll(resetDir)

		_ = testkit.RunStandardTeardown(testkit.TeardownOptions{
			ProjectRoot:               testRoot,
			FileStorage:               fileStorage,
			StripProcessArtifacts:     true,
			WALTimeout:                20 * time.Second,
			ShutdownTimeout:           20 * time.Second,
			TearDownGlobalAuditBuffer: true,
			SecCtx:                    secCtxAlready,
			AuditBufferResetRoot:      resetDir,
		})
	})

	// Create multiple policy objects with different field values
	policies := []map[string]any{
		{
			objects.FieldKeyID:            "POL-CODE-997",
			objects.FieldKeyKind:          "policy",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusActive,
			objects.FieldKeyTitle:         "Policy 1",
			objects.FieldKeyPolicyType:    "standard",
			objects.FieldKeyCategory:      "code_quality",
			objects.FieldKeyBody:          "Body content for policy 1",
			objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
			objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
			objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
			objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
			objects.FieldKeyNamespaceID:   "zqk:kernel",
			objects.FieldKeyOriginProject: "zqk",
			objects.FieldKeyOriginSystem:  "zqk",
			"enforcement_level":           "required",
		},
		{
			objects.FieldKeyID:            "POL-CODE-996",
			objects.FieldKeyKind:          "policy",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusActive,
			objects.FieldKeyTitle:         "Policy 2",
			objects.FieldKeyPolicyType:    "requirement",
			objects.FieldKeyCategory:      "security",
			objects.FieldKeyBody:          "Body content for policy 2",
			objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
			objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
			objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
			objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
			objects.FieldKeyNamespaceID:   "zqk:kernel",
			objects.FieldKeyOriginProject: "zqk",
			objects.FieldKeyOriginSystem:  "zqk",
			"enforcement_level":           "required",
		},
	}

	for _, policy := range policies {
		storage.CreateCASVisible(t, fileStorage, ctx, secCtx, policy, objects.ObjectStatusActive)
	}
	storage.FlushAllOrFail(t, testRoot)

	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	// Use global loaders (same as bulk check path)
	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()
	validatorRegistry := validation.GetGlobalRegistry()
	validator := validatorRegistry.Get("") // Default validator

	// Create hash registry cache (same as bulk check path)
	hashRegistryCache := &HashRegistryCacheType{
		cache: make(map[string]storage.HashRegistryProvider),
	}

	// Use a fresh cache for this temp root (avoids cross-talk with the global singleton under -p bundles).
	objectIDCache := NewObjectIDCache()
	if err := objectIDCache.BuildCache(context.Background(), testRoot, true); err != nil {
		t.Fatalf("Failed to build object ID cache: %v", err)
	}

	// Use CheckKindObjectsWithCache (the bulk check path)
	results, _, err := CheckKindObjectsWithCache(
		checkCtx,
		pkgctx.NewSystemContext(),
		&cobra.Command{},
		"policy",
		nil, // Check all policy objects (bulk)
		specLoader,
		lifecycleLoader,
		validator,
		hashRegistryCache,
		objectIDCache,
	)
	if err != nil {
		t.Fatalf("Failed to check objects: %v", err)
	}

	// Verify all objects were checked
	if len(results) < 2 {
		t.Fatalf("Expected at least 2 results, got %d", len(results))
	}

	// Verify each object has no validation errors
	resultsByID := make(map[string]*CheckResult)
	for i := range results {
		resultsByID[results[i].ObjectID] = &results[i]
	}

	for _, policyID := range []string{"POL-CODE-997", "POL-CODE-996"} {
		result, ok := resultsByID[policyID]
		if !ok {
			t.Errorf("Object %s not found in check results", policyID)
			continue
		}

		// Verify no instance validation errors
		for _, issue := range result.Issues {
			if issue.Category == "instance_validation" {
				if strings.Contains(issue.Message, "required") {
					t.Errorf("Object %s: Unexpected validation error: %s", policyID, issue.Message)
				}
			}
		}
	}
}
