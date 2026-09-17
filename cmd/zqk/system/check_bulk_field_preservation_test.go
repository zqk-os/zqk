package system

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/lanceman/zqk/internal/cli"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/spf13/cobra"
)

// TestBulkCheckPreservesAllFields tests that bulk check operations preserve all fields
// This is critical for catching issues like POL-DEBUG-001 where fields appear to be
// wiped during bulk operations but are actually present in the file.
func TestBulkCheckPreservesAllFields(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tmpDir := proj.Root

	// Setup test environment
	if _, err := setupSystemTestEnvironmentRoot(t, tmpDir); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	// Use ForTest to avoid global wiring and ensure storage uses tmpDir only (no ZQK_TEST_ROOT cross-test pollution).
	fileStorage, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, fileStorage)

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := pkgctx.NewSystemContext()

	// Create multiple policy objects; use 810-819 to avoid collision with other tests in this file (801, 802) and 9xx.
	numObjects := 10
	objectIDs := make([]string, numObjects)
	for i := 0; i < numObjects; i++ {
		objID := fmt.Sprintf("POL-CODE-%03d", 810+i) // 810-819
		objectIDs[i] = objID

		obj := map[string]any{
			objects.FieldKeyID:            objID,
			objects.FieldKeyKind:          "policy",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusActive,
			objects.FieldKeyTitle:         fmt.Sprintf("Bulk Test Policy %d", i),
			objects.FieldKeyPolicyType:    "requirement",                                                                                            // Required enum field
			objects.FieldKeyCategory:      "code_quality",                                                                                           // Required field
			objects.FieldKeyBody:          fmt.Sprintf("Test policy body content for object %d that should be preserved during bulk operations", i), // Required field
			objects.FieldKeyDescription:   fmt.Sprintf("Test description %d", i),
			"enforcement_level":           "required",
			objects.FieldKeyCreatedAt:     "2026-01-02T00:00:00Z",
			objects.FieldKeyCreatedBy:     "ACC-TEST",
			objects.FieldKeyUpdatedAt:     "2026-01-02T00:00:00Z",
			objects.FieldKeyUpdatedBy:     "ACC-TEST",
			objects.FieldKeyOriginProject: validation.DefaultOriginProject,
			objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
			objects.FieldKeyNamespaceID:   paths.KernelNamespaceID,
		}

		storage.CreateCASVisible(t, fileStorage, ctx, secCtx, obj, objects.ObjectStatusActive)
	}
	storage.FlushAllOrFail(t, tmpDir)

	// Verify all objects were created with all fields
	for _, objID := range objectIDs {
		obj, err := fileStorage.Read(ctx, secCtx, objID)
		if err != nil {
			t.Fatalf("Failed to read object %s: %v", objID, err)
		}

		requiredFields := []string{"policy_type", "category", "body"}
		for _, field := range requiredFields {
			if _, ok := obj[field]; !ok {
				t.Errorf("Object %s missing required field %s after creation", objID, field)
			}
		}
	}

	// Run bulk check on all policy objects
	checkCtx := cli.ContextForProjectAndProfile(tmpDir, "test")

	cmd := &cobra.Command{}
	results, err := checkKindObjects(checkCtx, cmd, "policy", nil)
	if err != nil {
		t.Fatalf("Bulk check failed: %v", err)
	}

	// Verify all objects were checked
	if len(results) < numObjects {
		t.Errorf("Expected at least %d results, got %d", numObjects, len(results))
	}

	// Verify no fields were lost during bulk check
	// This is the critical test - if fields are missing, the bulk check path has a bug
	for _, result := range results {
		if result.ObjectID == emptyValue {
			continue // Skip invalid results
		}

		// Re-read object after bulk check to verify fields are still present
		obj, err := fileStorage.Read(ctx, secCtx, result.ObjectID)
		if err != nil {
			t.Errorf("Failed to read object %s after bulk check: %v", result.ObjectID, err)
			continue
		}

		requiredFields := []string{"policy_type", "category", "body"}
		for _, field := range requiredFields {
			if _, ok := obj[field]; !ok {
				t.Errorf("Object %s missing required field %s after bulk check! This indicates fields were wiped during bulk operations.", result.ObjectID, field)
			}
		}

		// Verify specific values are preserved
		if obj[objects.FieldKeyPolicyType] != "requirement" {
			t.Errorf("Object %s: policy_type was changed during bulk check: expected 'requirement', got %v", result.ObjectID, obj[objects.FieldKeyPolicyType])
		}
		if obj[objects.FieldKeyCategory] != "code_quality" {
			t.Errorf("Object %s: category was changed during bulk check: expected 'code_quality', got %v", result.ObjectID, obj[objects.FieldKeyCategory])
		}
		body, ok := obj[objects.FieldKeyBody].(string)
		if !ok || body == emptyValue {
			t.Errorf("Object %s: body was wiped or empty after bulk check: got %v", result.ObjectID, obj[objects.FieldKeyBody])
		}
	}

	// Cleanup
	for _, objID := range objectIDs {
		_ = fileStorage.Delete(ctx, secCtx, objID, false) //nolint:errcheck // Test cleanup - errors are acceptable
	}
}

// TestBulkCheckWithUpdates tests that bulk check correctly handles objects that were
// updated between creation and bulk check, ensuring no stale data is used.
func TestBulkCheckWithUpdates(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tmpDir := proj.Root

	// Setup test environment
	if _, err := setupSystemTestEnvironmentRoot(t, tmpDir); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	fileStorage, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, fileStorage)

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := pkgctx.NewSystemContext()

	// Unique ID in 8xx range to avoid "object already exists" when run in parallel (e.g. test bundler)
	objID := "POL-CODE-801"
	obj := map[string]any{
		objects.FieldKeyID:            objID,
		objects.FieldKeyKind:          "policy",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Update Test Policy",
		objects.FieldKeyPolicyType:    "requirement",
		objects.FieldKeyCategory:      "code_quality",
		objects.FieldKeyBody:          "Original body content",
		objects.FieldKeyDescription:   "Original description",
		"enforcement_level":           "required",
		objects.FieldKeyCreatedAt:     "2026-01-02T00:00:00Z",
		objects.FieldKeyCreatedBy:     "ACC-TEST",
		objects.FieldKeyUpdatedAt:     "2026-01-02T00:00:00Z",
		objects.FieldKeyUpdatedBy:     "ACC-TEST",
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		objects.FieldKeyNamespaceID:   paths.KernelNamespaceID,
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, obj, objects.ObjectStatusActive)
	storage.FlushAllOrFail(t, tmpDir)

	// Update only the title (partial update)
	updates := map[string]any{
		objects.FieldKeyTitle: "Updated Title",
	}
	if err := fileStorage.Update(ctx, secCtx, objID, updates); err != nil {
		t.Fatalf("Failed to update object: %v", err)
	}

	// Verify update preserved all fields
	updated, err := fileStorage.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("Failed to read updated object: %v", err)
	}

	requiredFields := []string{"policy_type", "category", "body"}
	for _, field := range requiredFields {
		if _, ok := updated[field]; !ok {
			t.Errorf("Required field %s missing after update", field)
		}
	}

	// Now run bulk check - this should use fresh data, not stale cached data
	checkCtx := cli.ContextForProjectAndProfile(tmpDir, "test")

	cmd := &cobra.Command{}
	results, err := checkKindObjects(checkCtx, cmd, "policy", []string{objID})
	if err != nil {
		t.Fatalf("Bulk check failed: %v", err)
	}

	// Filter results to only the object we're testing (may have other objects from test setup)
	var testResult *CheckResult
	for i := range results {
		if results[i].ObjectID == objID {
			testResult = &results[i]
			break
		}
	}
	if testResult == nil {
		t.Fatalf("Expected result for %s, got %d results: %v", objID, len(results), results)
	}

	// Verify the bulk check used fresh data (not stale)
	// The object should have the updated title and all required fields
	finalObj, err := fileStorage.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("Failed to read object after bulk check: %v", err)
	}

	if finalObj[objects.FieldKeyTitle] != "Updated Title" {
		t.Errorf("Bulk check used stale data: expected title 'Updated Title', got %v", finalObj[objects.FieldKeyTitle])
	}

	for _, field := range requiredFields {
		if _, ok := finalObj[field]; !ok {
			t.Errorf("Required field %s missing after bulk check - bulk check may have used stale data", field)
		}
	}

	// Cleanup
	_ = fileStorage.Delete(ctx, secCtx, objID, false) //nolint:errcheck // Test cleanup - errors are acceptable

	ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	_ = fileStorage.Shutdown(ctxShutdown)
}

// TestBulkCheckFieldPreservationAcrossKinds tests that bulk check preserves fields
// across different object kinds, not just policies.
func TestBulkCheckFieldPreservationAcrossKinds(t *testing.T) {
	// Not t.Parallel: storage teardown must finish before t.TempDir cleanup.
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tmpDir := proj.Root

	// Setup test environment
	if _, err := setupSystemTestEnvironmentRoot(t, tmpDir); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	fileStorage, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, fileStorage)

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := pkgctx.NewSystemContext()

	// Unique IDs (8xx / 8xxx) to avoid "object already exists" when run in parallel (e.g. test bundler)
	testObjects := []struct {
		id       string
		kind     string
		obj      map[string]any
		required []string
	}{
		{
			id:   "POL-CODE-802",
			kind: "policy",
			obj: map[string]any{
				objects.FieldKeyID:            "POL-CODE-802",
				objects.FieldKeyKind:          "policy",
				objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
				objects.FieldKeyStatus:        objects.ObjectStatusActive,
				objects.FieldKeyTitle:         "Multi-kind Test Policy",
				objects.FieldKeyPolicyType:    "requirement",
				objects.FieldKeyCategory:      "code_quality",
				objects.FieldKeyBody:          "Policy body content",
				"enforcement_level":           "required",
				objects.FieldKeyCreatedAt:     "2026-01-02T00:00:00Z",
				objects.FieldKeyCreatedBy:     "ACC-TEST",
				objects.FieldKeyUpdatedAt:     "2026-01-02T00:00:00Z",
				objects.FieldKeyUpdatedBy:     "ACC-TEST",
				objects.FieldKeyOriginProject: validation.DefaultOriginProject,
				objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
				objects.FieldKeyNamespaceID:   paths.KernelNamespaceID,
			},
			required: []string{"policy_type", "category", "body"},
		},
		{
			id:   "BLI-8002",
			kind: "backlog_item",
			obj: map[string]any{
				objects.FieldKeyID:            "BLI-8002",
				objects.FieldKeyKind:          "backlog_item",
				objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
				objects.FieldKeyStatus:        objects.ObjectStatusExploring,
				objects.FieldKeyTitle:         "Multi-kind Test Backlog Item",
				objects.FieldKeyDescription:   "Backlog item description",
				objects.FieldKeyCreatedAt:     "2026-01-02T00:00:00Z",
				objects.FieldKeyCreatedBy:     "ACC-TEST",
				objects.FieldKeyUpdatedAt:     "2026-01-02T00:00:00Z",
				objects.FieldKeyUpdatedBy:     "ACC-TEST",
				objects.FieldKeyOriginProject: validation.DefaultOriginProject,
				objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
				objects.FieldKeyNamespaceID:   paths.KernelNamespaceID,
			},
			required: []string{"title", "status"},
		},
	}

	// Create all test objects (promote off draft plane so bulk check scanner sees CAS files).
	for _, testObj := range testObjects {
		leave := objects.ObjectStatusActive
		if testObj.kind == "backlog_item" {
			leave = objects.ObjectStatusInProgress
		}
		storage.CreateCASVisible(t, fileStorage, ctx, secCtx, testObj.obj, leave)
	}
	storage.FlushAllOrFail(t, tmpDir)

	// Run bulk check on each kind
	checkCtx := cli.ContextForProjectAndProfile(tmpDir, "test")

	cmd := &cobra.Command{}

	for _, testObj := range testObjects {
		t.Run(fmt.Sprintf("Bulk check %s", testObj.kind), func(t *testing.T) {
			results, err := checkKindObjects(checkCtx, cmd, testObj.kind, []string{testObj.id})
			if err != nil {
				t.Fatalf("Bulk check failed for %s: %v", testObj.kind, err)
			}

			if len(results) != 1 {
				t.Fatalf("Expected 1 result for %s, got %d", testObj.id, len(results))
			}

			// Verify fields are preserved after bulk check
			obj, err := fileStorage.Read(ctx, secCtx, testObj.id)
			if err != nil {
				t.Fatalf("Failed to read %s after bulk check: %v", testObj.id, err)
			}

			for _, field := range testObj.required {
				if _, ok := obj[field]; !ok {
					t.Errorf("%s %s missing required field %s after bulk check", testObj.kind, testObj.id, field)
				}
			}
		})
	}

	// Cleanup
	for _, testObj := range testObjects {
		_ = fileStorage.Delete(ctx, secCtx, testObj.id, false) //nolint:errcheck // Test cleanup - errors are acceptable
	}
}
