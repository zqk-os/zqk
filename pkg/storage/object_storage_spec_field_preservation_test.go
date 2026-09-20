package storage_test

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/validation"
)

func setupFileObjectStorageForFieldPreservationTest(t *testing.T) (*storage.FileObjectStorage, *pkgctx.SecurityContext) {
	t.Helper()
	tmpDir, err := fileutil.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	fos, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	defer func() { _ = fos.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(tmpDir, fos)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
		if err := fileutil.RemoveAll(tmpDir); err != nil {
			t.Logf("remove temp dir: %v", err)
		}
	})

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	return fos, secCtx
}

// TestUpdatePreservesAllSpecFields tests that Update preserves ALL fields from the spec,
// not just the ones being updated. This ensures that when we update an object, we don't
// accidentally wipe out fields that weren't in the updates map.
func TestUpdatePreservesAllSpecFields(t *testing.T) {
	fos, secCtx := setupFileObjectStorageForFieldPreservationTest(t)
	ctx := context.Background()

	objID := "POL-CODE-999"
	obj := map[string]any{
		objects.FieldKeyID:            objID,
		objects.FieldKeyKind:          "policy",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Test Policy for Field Preservation",
		objects.FieldKeyPolicyType:    "requirement",
		objects.FieldKeyCategory:      "code_quality",
		objects.FieldKeyBody:          "Test policy body content that should be preserved",
		objects.FieldKeyDescription:   "Test description",
		"enforcement_level":           "required",
		objects.FieldKeyCreatedAt:     "2030-01-02T00:00:00Z",
		objects.FieldKeyCreatedBy:     "ACC-TEST",
		objects.FieldKeyUpdatedAt:     "2030-01-02T00:00:00Z",
		objects.FieldKeyUpdatedBy:     "ACC-TEST",
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		objects.FieldKeyNamespaceID:   "zqk:kernel",
	}

	if err := fos.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	original, err := fos.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("Failed to read object: %v", err)
	}

	requiredFields := []string{"policy_type", "category", "body"}
	for _, field := range requiredFields {
		if _, ok := original[field]; !ok {
			t.Errorf("Required field %s missing from created object", field)
		}
	}

	updates := map[string]any{
		objects.FieldKeyTitle: "Updated Title Only",
	}

	if err := fos.Update(ctx, secCtx, objID, updates); err != nil {
		t.Fatalf("Failed to update object: %v", err)
	}

	updated, err := fos.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("Failed to read updated object: %v", err)
	}

	if updated[objects.FieldKeyTitle] != "Updated Title Only" {
		t.Errorf("Title not updated: expected 'Updated Title Only', got %v", updated[objects.FieldKeyTitle])
	}

	for _, field := range requiredFields {
		if _, ok := updated[field]; !ok {
			t.Errorf("Required field %s was WIPED OUT during update! This is a critical bug.", field)
		}
	}

	if updated[objects.FieldKeyPolicyType] != "requirement" {
		t.Errorf("policy_type was changed: expected 'requirement', got %v", updated[objects.FieldKeyPolicyType])
	}
	if updated[objects.FieldKeyCategory] != "code_quality" {
		t.Errorf("category was changed: expected 'code_quality', got %v", updated[objects.FieldKeyCategory])
	}
	body, ok := updated[objects.FieldKeyBody].(string)
	if !ok || body == "" {
		t.Errorf("body was wiped or empty: got %v", updated[objects.FieldKeyBody])
	}
	if body != "Test policy body content that should be preserved" {
		t.Errorf("body was changed: expected original content, got %v", body)
	}

	if updated[objects.FieldKeyDescription] != "Test description" {
		t.Errorf("description was wiped: expected 'Test description', got %v", updated[objects.FieldKeyDescription])
	}
	if updated["enforcement_level"] != "required" {
		t.Errorf("enforcement_level was wiped: expected 'required', got %v", updated["enforcement_level"])
	}

	_ = storage.FlushAllListingIndexesForProjectRoot(fos.GetProjectRoot()) //nolint:errcheck

	originalFieldCount := 0
	updatedFieldCount := 0
	for k := range original {
		if k != "updated_at" && k != "updated_by" {
			originalFieldCount++
			if _, ok := updated[k]; ok {
				updatedFieldCount++
			}
		}
	}

	if updatedFieldCount < originalFieldCount {
		t.Errorf("Field count decreased after update: original had %d fields, updated has %d fields. Fields were wiped!", originalFieldCount, updatedFieldCount)
	}

	_ = storage.FlushAllListingIndexesForProjectRoot(fos.GetProjectRoot()) //nolint:errcheck

	_ = fos.Delete(ctx, secCtx, objID, false) //nolint:errcheck // Test cleanup - errors are acceptable
}

// TestUpdatePreservesAllSpecFields_WithNewFields tests that Update preserves fields even when
// new fields are added to the spec after the object was created
func TestUpdatePreservesAllSpecFields_WithNewFields(t *testing.T) {
	fos, secCtx := setupFileObjectStorageForFieldPreservationTest(t)
	ctx := context.Background()

	objID := "POL-CODE-998"
	obj := map[string]any{
		objects.FieldKeyID:            objID,
		objects.FieldKeyKind:          "policy",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Test Policy",
		objects.FieldKeyPolicyType:    "requirement",
		objects.FieldKeyCategory:      "code_quality",
		objects.FieldKeyBody:          "Test body",
		objects.FieldKeyDescription:   "Test description",
		"enforcement_level":           "required",
		objects.FieldKeyCreatedAt:     "2030-01-02T00:00:00Z",
		objects.FieldKeyCreatedBy:     "ACC-TEST",
		objects.FieldKeyUpdatedAt:     "2030-01-02T00:00:00Z",
		objects.FieldKeyUpdatedBy:     "ACC-TEST",
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		objects.FieldKeyNamespaceID:   "zqk:kernel",
		"custom_field_1":              "custom value 1",
		"custom_field_2":              "custom value 2",
	}

	if err := fos.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	updates := map[string]any{
		objects.FieldKeyTitle: "Updated Title",
	}

	if err := fos.Update(ctx, secCtx, objID, updates); err != nil {
		t.Fatalf("Failed to update object: %v", err)
	}

	updated, err := fos.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("Failed to read updated object: %v", err)
	}

	if updated["custom_field_1"] != "custom value 1" {
		t.Errorf("custom_field_1 was wiped: expected 'custom value 1', got %v", updated["custom_field_1"])
	}
	if updated["custom_field_2"] != "custom value 2" {
		t.Errorf("custom_field_2 was wiped: expected 'custom value 2', got %v", updated["custom_field_2"])
	}

	_ = storage.FlushAllListingIndexesForProjectRoot(fos.GetProjectRoot()) //nolint:errcheck

	_ = fos.Delete(ctx, secCtx, objID, false) //nolint:errcheck // Test cleanup - errors are acceptable
}
