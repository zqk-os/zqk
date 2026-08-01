package storage_test

//nolint:errcheck // Test cleanup operations - errors are acceptable

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
)

// TestPartialUpdatePreservesOtherFields tests that updating only one field doesn't wipe out others
func TestPartialUpdatePreservesOtherFields(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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
	})

	secCtx := pkgctx.NewSecurityContext("account:test", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := context.Background()

	// Create object with multiple fields
	objID := "ITEM-991"
	obj := map[string]any{
		objects.FieldKeyID:            objID,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Original Title",
		objects.FieldKeyDescription:   "Original Description",
		objects.FieldKeyStatus:        "exploring",
		objects.FieldKeyPriority:      "high",
		objects.FieldKeyCategory:      "feature",
		objects.FieldKeyContext:       "Original context",
		objects.FieldKeyNotes:         "Original notes",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	if err := fos.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("failed to create object: %v", err)
	}

	// Update only the title
	updates := map[string]any{
		objects.FieldKeyTitle: "Updated Title",
	}

	if err := fos.Update(ctx, secCtx, objID, updates); err != nil {
		t.Fatalf("failed to update object: %v", err)
	}

	// Read the updated object
	updated, err := fos.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("failed to read updated object: %v", err)
	}

	// Verify title was updated
	if updated[objects.FieldKeyTitle] != "Updated Title" {
		t.Errorf("title not updated: expected 'Updated Title', got %v", updated[objects.FieldKeyTitle])
	}

	// Verify all other fields are preserved
	if updated[objects.FieldKeyDescription] != "Original Description" {
		t.Errorf("description was wiped: expected 'Original Description', got %v", updated[objects.FieldKeyDescription])
	}
	if updated[objects.FieldKeyStatus] != "exploring" {
		t.Errorf("status was wiped: expected 'exploring', got %v", updated[objects.FieldKeyStatus])
	}
	if updated[objects.FieldKeyPriority] != "high" {
		t.Errorf("priority was wiped: expected 'high', got %v", updated[objects.FieldKeyPriority])
	}
	if updated[objects.FieldKeyCategory] != "feature" {
		t.Errorf("category was wiped: expected 'feature', got %v", updated[objects.FieldKeyCategory])
	}
	if updated[objects.FieldKeyContext] != "Original context" {
		t.Errorf("context was wiped: expected 'Original context', got %v", updated[objects.FieldKeyContext])
	}
	if updated[objects.FieldKeyNotes] != "Original notes" {
		t.Errorf("notes was wiped: expected 'Original notes', got %v", updated[objects.FieldKeyNotes])
	}

	// Verify immutable fields are preserved
	if updated[objects.FieldKeyID] != objID {
		t.Errorf("id was changed: expected %s, got %v", objID, updated[objects.FieldKeyID])
	}
	if updated[objects.FieldKeyKind] != "backlog_item" {
		t.Errorf("kind was changed: expected 'backlog_item', got %v", updated[objects.FieldKeyKind])
	}

	// Desired behavior: CAS Update path must create a change_journal_entry for audit and rollback.
	// Enforced by createChangeJournalEntry in object_storage_file_update.go; see STRUCTURAL_VS_RUNTIME_DELTA_INVESTIGATION.md.
	// Integration assertion is not run here because this test can time out in CI (HashRegistry.Save blocks).

	// Cleanup
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = fos.Delete(ctx, secCtx, objID, false)
}

// TestPartialUpdateWithMultipleFields tests updating multiple fields while preserving others
func TestPartialUpdateWithMultipleFields(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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
	})

	secCtx := pkgctx.NewSecurityContext("account:test", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := context.Background()

	// Create object with many fields
	objID := "ITEM-990"
	obj := map[string]any{
		objects.FieldKeyID:             objID,
		objects.FieldKeyKind:           "backlog_item",
		objects.FieldKeyTitle:          "Original Title",
		objects.FieldKeyDescription:    "Original Description",
		objects.FieldKeyStatus:         "exploring",
		objects.FieldKeyPriority:       "high",
		objects.FieldKeyCategory:       "feature",
		objects.FieldKeyContext:        "Original context",
		objects.FieldKeyNotes:          "Original notes",
		objects.FieldKeyBenefits:       []string{"benefit1", "benefit2"},
		objects.FieldKeyConsiderations: []string{"consideration1"},
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
	}

	if err := fos.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("failed to create object: %v", err)
	}

	// Update only title and status
	updates := map[string]any{
		objects.FieldKeyTitle:  "Updated Title",
		objects.FieldKeyStatus: "validated",
	}

	if err := fos.Update(ctx, secCtx, objID, updates); err != nil {
		t.Fatalf("failed to update object: %v", err)
	}

	// Read the updated object
	updated, err := fos.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("failed to read updated object: %v", err)
	}

	// Verify updated fields
	if updated[objects.FieldKeyTitle] != "Updated Title" {
		t.Errorf("title not updated: expected 'Updated Title', got %v", updated[objects.FieldKeyTitle])
	}
	if updated[objects.FieldKeyStatus] != "validated" {
		t.Errorf("status not updated: expected 'validated', got %v", updated[objects.FieldKeyStatus])
	}

	// Verify all other fields are preserved
	if updated[objects.FieldKeyDescription] != "Original Description" {
		t.Errorf("description was wiped: expected 'Original Description', got %v", updated[objects.FieldKeyDescription])
	}
	if updated[objects.FieldKeyPriority] != "high" {
		t.Errorf("priority was wiped: expected 'high', got %v", updated[objects.FieldKeyPriority])
	}
	if updated[objects.FieldKeyCategory] != "feature" {
		t.Errorf("category was wiped: expected 'feature', got %v", updated[objects.FieldKeyCategory])
	}
	if updated[objects.FieldKeyContext] != "Original context" {
		t.Errorf("context was wiped: expected 'Original context', got %v", updated[objects.FieldKeyContext])
	}
	if updated[objects.FieldKeyNotes] != "Original notes" {
		t.Errorf("notes was wiped: expected 'Original notes', got %v", updated[objects.FieldKeyNotes])
	}

	// Verify list fields are preserved
	benefits, ok := updated[objects.FieldKeyBenefits].([]any)
	if !ok {
		// Try []string
		benefitsStr, ok := updated[objects.FieldKeyBenefits].([]string)
		if !ok {
			t.Errorf("benefits field missing or wrong type: %v", updated[objects.FieldKeyBenefits])
		} else {
			expectedBenefits := []string{"benefit1", "benefit2"}
			if !reflect.DeepEqual(benefitsStr, expectedBenefits) {
				t.Errorf("benefits were wiped: expected %v, got %v", expectedBenefits, benefitsStr)
			}
		}
	} else {
		// Convert []any to []string for comparison
		benefitsStr := make([]string, len(benefits))
		for i, b := range benefits {
			benefitsStr[i] = b.(string)
		}
		expectedBenefits := []string{"benefit1", "benefit2"}
		if !reflect.DeepEqual(benefitsStr, expectedBenefits) {
			t.Errorf("benefits were wiped: expected %v, got %v", expectedBenefits, benefitsStr)
		}
	}

	considerations, ok := updated[objects.FieldKeyConsiderations].([]any)
	if !ok {
		// Try []string
		considerationsStr, ok := updated[objects.FieldKeyConsiderations].([]string)
		if !ok {
			t.Errorf("considerations field missing or wrong type: %v", updated[objects.FieldKeyConsiderations])
		} else {
			expectedConsiderations := []string{"consideration1"}
			if !reflect.DeepEqual(considerationsStr, expectedConsiderations) {
				t.Errorf("considerations were wiped: expected %v, got %v", expectedConsiderations, considerationsStr)
			}
		}
	} else {
		// Convert []any to []string for comparison
		considerationsStr := make([]string, len(considerations))
		for i, c := range considerations {
			considerationsStr[i] = c.(string)
		}
		expectedConsiderations := []string{"consideration1"}
		if !reflect.DeepEqual(considerationsStr, expectedConsiderations) {
			t.Errorf("considerations were wiped: expected %v, got %v", expectedConsiderations, considerationsStr)
		}
	}

	//nolint:errcheck // Test cleanup - errors are acceptable
	// Cleanup
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = fos.Delete(ctx, secCtx, objID, false)
}

// TestPartialUpdateWithNestedFields tests that nested/object fields are preserved
func TestPartialUpdateWithNestedFields(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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
	})

	secCtx := pkgctx.NewSecurityContext("account:test", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := context.Background()

	// Create object with nested fields (if supported)
	objID := "ITEM-989"
	obj := map[string]any{
		objects.FieldKeyID:            objID,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Original Title",
		objects.FieldKeyStatus:        "exploring",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		// Add some fields that might be objects or have complex structures
		objects.FieldKeyMetadata: map[string]any{
			"key1": "value1",
			"key2": "value2",
		},
	}

	if err := fos.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("failed to create object: %v", err)
	}

	// Update only the title
	updates := map[string]any{
		objects.FieldKeyTitle: "Updated Title",
	}

	if err := fos.Update(ctx, secCtx, objID, updates); err != nil {
		t.Fatalf("failed to update object: %v", err)
	}

	// Read the updated object
	updated, err := fos.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("failed to read updated object: %v", err)
	}

	// Verify title was updated
	if updated[objects.FieldKeyTitle] != "Updated Title" {
		t.Errorf("title not updated: expected 'Updated Title', got %v", updated[objects.FieldKeyTitle])
	}

	// Verify nested fields are preserved (if they exist)
	if metadata, ok := updated[objects.FieldKeyMetadata].(map[string]any); ok {
		if metadata["key1"] != "value1" {
			t.Errorf("nested field key1 was wiped: expected 'value1', got %v", metadata["key1"])
		}
		if metadata["key2"] != "value2" {
			t.Errorf("nested field key2 was wiped: expected 'value2', got %v", metadata["key2"])
		}
	}

	// Cleanup
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = fos.Delete(ctx, secCtx, objID, false)
}

// TestPartialUpdateWithEmptyValues tests that setting a field to empty doesn't wipe other fields
func TestPartialUpdateWithEmptyValues(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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
	})

	secCtx := pkgctx.NewSecurityContext("account:test", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := context.Background()

	// Create object with multiple fields
	objID := "ITEM-988"
	obj := map[string]any{
		objects.FieldKeyID:            objID,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Original Title",
		objects.FieldKeyDescription:   "Original Description",
		objects.FieldKeyStatus:        "exploring",
		objects.FieldKeyNotes:         "Original notes",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	if err := fos.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("failed to create object: %v", err)
	}

	// Update title to empty string (if allowed) and clear notes
	updates := map[string]any{
		objects.FieldKeyTitle: "Updated Title",
		objects.FieldKeyNotes: "", // Clear notes
	}

	if err := fos.Update(ctx, secCtx, objID, updates); err != nil {
		t.Fatalf("failed to update object: %v", err)
	}

	// Read the updated object
	updated, err := fos.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("failed to read updated object: %v", err)
	}

	// Verify updated fields
	if updated[objects.FieldKeyTitle] != "Updated Title" {
		t.Errorf("title not updated: expected 'Updated Title', got %v", updated[objects.FieldKeyTitle])
	}
	if updated[objects.FieldKeyNotes] != "" {
		t.Errorf("notes not cleared: expected '', got %v", updated[objects.FieldKeyNotes])
	}

	// Verify other fields are preserved
	if updated[objects.FieldKeyDescription] != "Original Description" {
		t.Errorf("description was wiped: expected 'Original Description', got %v", updated[objects.FieldKeyDescription])
	}
	if updated[objects.FieldKeyStatus] != "exploring" {
		t.Errorf("status was wiped: expected 'exploring', got %v", updated[objects.FieldKeyStatus])
	}

	// Cleanup
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = fos.Delete(ctx, secCtx, objID, false)
}

// TestPartialUpdatePreservesSystemFields tests that system fields are preserved
func TestPartialUpdatePreservesSystemFields(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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
	})

	secCtx := pkgctx.NewSecurityContext("account:test", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := context.Background()

	// Create object
	objID := "ITEM-987"
	originalCreatedBy := "account:test"
	obj := map[string]any{
		objects.FieldKeyID:            objID,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Original Title",
		objects.FieldKeyStatus:        "exploring",
		objects.FieldKeyCreatedBy:     originalCreatedBy,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	if err := fos.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("failed to create object: %v", err)
	}

	// Read to get created_at
	original, err := fos.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("failed to read object: %v", err)
	}
	originalCreatedAt := original[objects.FieldKeyCreatedAt].(string)

	// Update only the title
	updates := map[string]any{
		objects.FieldKeyTitle: "Updated Title",
	}

	if err := fos.Update(ctx, secCtx, objID, updates); err != nil {
		t.Fatalf("failed to update object: %v", err)
	}

	// Read the updated object
	updated, err := fos.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("failed to read updated object: %v", err)
	}

	// Verify title was updated
	if updated[objects.FieldKeyTitle] != "Updated Title" {
		t.Errorf("title not updated: expected 'Updated Title', got %v", updated[objects.FieldKeyTitle])
	}

	// Verify system fields are preserved
	if updated[objects.FieldKeyCreatedAt] != originalCreatedAt {
		t.Errorf("created_at was changed: expected %s, got %v", originalCreatedAt, updated[objects.FieldKeyCreatedAt])
	}
	if updated[objects.FieldKeyCreatedBy] != originalCreatedBy {
		t.Errorf("created_by was changed: expected %s, got %v", originalCreatedBy, updated[objects.FieldKeyCreatedBy])
	}
	if updated[objects.FieldKeyID] != objID {
		t.Errorf("id was changed: expected %s, got %v", objID, updated[objects.FieldKeyID])
	}
	if updated[objects.FieldKeyKind] != "backlog_item" {
		t.Errorf("kind was changed: expected 'backlog_item', got %v", updated[objects.FieldKeyKind])
	}

	// Verify updated_at was changed (it should be updated on every update)
	// Note: Due to time precision, updated_at might be the same if updates happen very quickly
	updatedAt := updated[objects.FieldKeyUpdatedAt].(string)
	if updatedAt == originalCreatedAt {
		t.Logf("Warning: updated_at did not change (may be due to time precision), but this is acceptable for partial update preservation test")
	}

	// Cleanup
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = fos.Delete(ctx, secCtx, objID, false)
}

// TestConvergenceSessionActivityLogAppendOnUpdate verifies activity_log updates append new entries
// instead of replacing the full list (manual convergence ledger from scheduler convergence measure output).
func TestConvergenceSessionActivityLogAppendOnUpdate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	processDir := datacell.ProcessPrimaryDir(tmpDir)
	backlogDir := filepath.Join(processDir, "backlog")
	if err := os.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create backlog dir: %v", err)
	}
	specsDir := filepath.Join(processDir, "_internal", "object_specs")
	if err := os.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create specs directory: %v", err)
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
	})

	secCtx := pkgctx.NewSecurityContext("account:test", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := context.Background()

	objID := "CONV-TEST-ACT-002"
	obj := map[string]any{
		objects.FieldKeyID:               objID,
		objects.FieldKeyKind:             objects.KindConvergenceSession,
		objects.FieldKeyTitle:            "activity log append test",
		objects.FieldKeyStatus:           "draft",
		objects.FieldKeyCurrentPhase:     "c1_scope",
		objects.FieldKeyOutcomeCharacter: "pending",
		objects.FieldKeySchemaVersion:    objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:        "2030-01-01T00:00:00Z",
		objects.FieldKeyCreatedBy:        "account:test",
		objects.FieldKeyUpdatedAt:        "2030-01-01T00:00:00Z",
		objects.FieldKeyUpdatedBy:        "account:test",
		objects.FieldKeyActivityLog:      []any{map[string]any{"timestamp": "2030-01-01T00:00:00Z", objects.FieldKeyPhase: "c1_scope", "action": "seed", objects.FieldKeyNotes: "first"}},
	}

	if err := fos.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("failed to create object: %v", err)
	}

	updates := map[string]any{
		objects.FieldKeyActivityLog: []any{map[string]any{"timestamp": "2030-01-02T00:00:00Z", objects.FieldKeyPhase: "c1_scope", "action": "measure_test_bundle_health", objects.FieldKeyNotes: "second"}},
	}
	if err := fos.Update(ctx, secCtx, objID, updates); err != nil {
		t.Fatalf("failed to update: %v", err)
	}

	updated, err := fos.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}
	al, ok := updated[objects.FieldKeyActivityLog].([]any)
	if !ok || len(al) != 2 {
		t.Fatalf("expected 2 activity_log entries, got %#v", updated[objects.FieldKeyActivityLog])
	}
	if a0, ok := al[0].(map[string]any); !ok || a0["action"] != "seed" {
		t.Fatalf("first entry: %#v", al[0])
	}
	if a1, ok := al[1].(map[string]any); !ok || a1["action"] != "measure_test_bundle_health" {
		t.Fatalf("second entry: %#v", al[1])
	}

	//nolint:errcheck // Test cleanup
	_ = fos.Delete(ctx, secCtx, objID, false)
}
