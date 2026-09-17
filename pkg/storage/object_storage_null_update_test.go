package storage

//nolint:errcheck // Test cleanup operations - errors are acceptable

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestUpdateFieldToEmptyString tests that fields can be set to empty string
func TestUpdateFieldToEmptyString(t *testing.T) {
	tmpDir, err := fileutil.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	mustEnsureProcessSpecsLayout(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	storage, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(tmpDir, storage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := context.Background()

	// Create object with non-empty fields
	objID := "BLI-986"
	obj := map[string]any{
		objects.FieldKeyID:            objID,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Original Title",
		objects.FieldKeyDescription:   "Original Description",
		objects.FieldKeyNotes:         "Original notes",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	if err := storage.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("failed to create object: %v", err)
	}

	// Update description to empty string
	updates := map[string]any{
		objects.FieldKeyDescription: "",
	}

	if err := storage.Update(ctx, secCtx, objID, updates); err != nil {
		t.Fatalf("failed to update object: %v", err)
	}

	// Read the updated object
	updated, err := storage.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("failed to read updated object: %v", err)
	}

	// Verify description is now empty
	if updated[objects.FieldKeyDescription] != emptyValue {
		t.Errorf("description not set to empty: expected '', got %v", updated[objects.FieldKeyDescription])
	}

	// Verify other fields are preserved
	if updated[objects.FieldKeyTitle] != "Original Title" {
		t.Errorf("title was wiped: expected 'Original Title', got %v", updated[objects.FieldKeyTitle])
	}
	if updated[objects.FieldKeyNotes] != "Original notes" {
		t.Errorf("notes was wiped: expected 'Original notes', got %v", updated[objects.FieldKeyNotes])
	}

	// Cleanup
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = storage.Delete(ctx, secCtx, objID, false)
}

// TestUpdateFieldToNil tests that fields can be set to nil
func TestUpdateFieldToNil(t *testing.T) {
	tmpDir, err := fileutil.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	mustEnsureProcessSpecsLayout(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	storage, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(tmpDir, storage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := context.Background()

	// Create object with non-empty fields
	objID := "BLI-985"
	obj := map[string]any{
		objects.FieldKeyID:            objID,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Original Title",
		objects.FieldKeyDescription:   "Original Description",
		objects.FieldKeyNotes:         "Original notes",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	if err := storage.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("failed to create object: %v", err)
	}

	// Update description to nil
	updates := map[string]any{
		objects.FieldKeyDescription: nil,
	}

	if err := storage.Update(ctx, secCtx, objID, updates); err != nil {
		t.Fatalf("failed to update object: %v", err)
	}

	// Read the updated object
	updated, err := storage.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("failed to read updated object: %v", err)
	}

	// Verify description is now nil or missing
	desc, exists := updated[objects.FieldKeyDescription]
	if exists && desc != nil && desc != emptyValue {
		t.Errorf("description not set to nil: expected nil or missing, got %v", desc)
	}

	// Verify other fields are preserved
	if updated[objects.FieldKeyTitle] != "Original Title" {
		t.Errorf("title was wiped: expected 'Original Title', got %v", updated[objects.FieldKeyTitle])
	}
	if updated[objects.FieldKeyNotes] != "Original notes" {
		t.Errorf("notes was wiped: expected 'Original notes', got %v", updated[objects.FieldKeyNotes])
	}

	//nolint:errcheck // Test cleanup - errors are acceptable
	// Cleanup
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = storage.Delete(ctx, secCtx, objID, false)
}

// TestUpdateListFieldToEmpty tests that list fields can be set to empty
func TestUpdateListFieldToEmpty(t *testing.T) {
	tmpDir, err := fileutil.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	mustEnsureProcessSpecsLayout(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	storage, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(tmpDir, storage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := context.Background()

	// Create object with non-empty list fields
	objID := "BLI-984"
	obj := map[string]any{
		objects.FieldKeyID:             objID,
		objects.FieldKeyKind:           "backlog_item",
		objects.FieldKeyTitle:          "Original Title",
		objects.FieldKeyBenefits:       []string{"benefit1", "benefit2"},
		objects.FieldKeyConsiderations: []string{"consideration1"},
		objects.FieldKeyStatus:         objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
	}

	if err := storage.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("failed to create object: %v", err)
	}

	// Update benefits to empty list
	updates := map[string]any{
		objects.FieldKeyBenefits: []string{},
	}

	if err := storage.Update(ctx, secCtx, objID, updates); err != nil {
		t.Fatalf("failed to update object: %v", err)
	}

	// Read the updated object
	updated, err := storage.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("failed to read updated object: %v", err)
	}

	// Verify benefits is now empty
	benefits, ok := updated[objects.FieldKeyBenefits]
	if !ok {
		t.Error("benefits field missing")
	} else {
		// Check if it's an empty list
		switch v := benefits.(type) {
		case []string:
			if len(v) != 0 {
				t.Errorf("benefits not set to empty: expected [], got %v", v)
			}
		case []any:
			if len(v) != 0 {
				t.Errorf("benefits not set to empty: expected [], got %v", v)
			}
		default:
			t.Errorf("benefits has unexpected type: %T", v)
		}
	}

	// Verify considerations are preserved
	considerations, ok := updated[objects.FieldKeyConsiderations]
	if !ok {
		t.Error("considerations field missing")
	} else {
		// Should still have the original value
		switch v := considerations.(type) {
		case []string:
			if len(v) == 0 {
				t.Error("considerations were wiped: expected non-empty list")
			}
		case []any:
			if len(v) == 0 {
				t.Error("considerations were wiped: expected non-empty list")
			}
		}
	}

	// Cleanup
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = storage.Delete(ctx, secCtx, objID, false)
}

// TestUpdateMultipleFieldsToEmpty tests updating multiple fields to empty/null
func TestUpdateMultipleFieldsToEmpty(t *testing.T) {
	tmpDir, err := fileutil.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	mustEnsureProcessSpecsLayout(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	storage, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(tmpDir, storage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := context.Background()

	// Create object with multiple fields
	objID := "BLI-983"
	obj := map[string]any{
		objects.FieldKeyID:            objID,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Original Title",
		objects.FieldKeyDescription:   "Original Description",
		objects.FieldKeyNotes:         "Original notes",
		objects.FieldKeyContext:       "Original context",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	if err := storage.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("failed to create object: %v", err)
	}

	// Update multiple fields to empty/null
	updates := map[string]any{
		objects.FieldKeyDescription: "",  // Empty string
		objects.FieldKeyNotes:       nil, // Nil
		objects.FieldKeyContext:     "",  // Empty string
	}

	if err := storage.Update(ctx, secCtx, objID, updates); err != nil {
		t.Fatalf("failed to update object: %v", err)
	}

	// Read the updated object
	updated, err := storage.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("failed to read updated object: %v", err)
	}

	// Verify fields are set to empty/nil
	if updated[objects.FieldKeyDescription] != emptyValue {
		t.Errorf("description not set to empty: expected '', got %v", updated[objects.FieldKeyDescription])
	}

	// Notes should be nil or missing
	notes, exists := updated[objects.FieldKeyNotes]
	if exists && notes != nil && notes != emptyValue {
		t.Errorf("notes not set to nil: expected nil or missing, got %v", notes)
	}

	if updated[objects.FieldKeyContext] != emptyValue {
		t.Errorf("context not set to empty: expected '', got %v", updated[objects.FieldKeyContext])
	}

	// Verify other fields are preserved
	if updated[objects.FieldKeyTitle] != "Original Title" {
		t.Errorf("title was wiped: expected 'Original Title', got %v", updated[objects.FieldKeyTitle])
	}
	if updated[objects.FieldKeyStatus] != "exploring" {
		t.Errorf("status was wiped: expected 'exploring', got %v", updated[objects.FieldKeyStatus])
	}

	// Cleanup
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = storage.Delete(ctx, secCtx, objID, false)
}
