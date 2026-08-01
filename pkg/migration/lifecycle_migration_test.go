package migration

import (
	"github.com/lanceman/zqk/pkg/datacell"

	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_v2" // Register lifecycle spec builder
	"github.com/lanceman/zqk/pkg/storage"
	"gopkg.in/yaml.v3"
)

// setupLifecycleMigrationTest sets up a test environment for lifecycle migration
func setupLifecycleMigrationTest(t *testing.T) (string, storage.ObjectStorageProvider, *lifecycleMigrationHelper, func()) {
	tmpDir := t.TempDir()

	// Create process directory structure
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	lifecyclesDir := filepath.Join(processDir, "_internal", "lifecycles")
	if err := os.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create lifecycles directory: %v", err)
	}

	// Create object_specs directory (required for validation)
	specsDir := filepath.Join(processDir, "_internal", "object_specs")
	if err := os.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create specs directory: %v", err)
	}

	// Create storage with test isolation; full project teardown before t.TempDir cleanup
	storageProvider, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(tmpDir, storageProvider)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	// Create helper
	logger := logging.GetLoggerFromProfile("test")
	helper := &lifecycleMigrationHelper{
		storageProvider: storageProvider,
		projectRoot:     tmpDir,
		logger:          logger,
	}

	return lifecyclesDir, storageProvider, helper, func() {}
}

// createTestLifecycleFile creates a test lifecycle file
func createTestLifecycleFile(t *testing.T, dir string, objectType string, content string) string {
	lifecycleFile := filepath.Join(dir, objectType+"_lifecycle.yaml")

	if content == emptyValue {
		content = `object_type: ` + objectType + `
statuses:
  - value: draft
    display: Draft
    initial: true
  - value: active
    display: Active
    terminal: true
  - value: archived
    display: Archived
    archive: true
    terminal: true
transitions:
  - from: draft
    to: active
    description: Activate
    manual: true
    auto: false
  - from: active
    to: archived
    description: Archive
    manual: true
    auto: false
percent_complete:
  method: status_defaults
  default_by_status:
    draft: 0
    active: 100
    archived: 100
`
	}

	if err := os.WriteFile(lifecycleFile, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write lifecycle file: %v", err)
	}

	return lifecycleFile
}

// TestLifecycleMigrationHelper_LifecycleToObject tests conversion of lifecycle file to object
func TestLifecycleMigrationHelper_LifecycleToObject(t *testing.T) {
	t.Parallel()
	lifecyclesDir, _, helper, cleanup := setupLifecycleMigrationTest(t)
	defer cleanup()

	// Create test lifecycle file
	objectType := "test_kind"
	createTestLifecycleFile(t, lifecyclesDir, objectType, "")

	// Load lifecycle struct
	lifecycleFile := filepath.Join(lifecyclesDir, objectType+"_lifecycle.yaml")
	data, err := os.ReadFile(lifecycleFile)
	if err != nil {
		t.Fatalf("Failed to read lifecycle file: %v", err)
	}

	var lifecycle objects.Lifecycle
	if err := yaml.Unmarshal(data, &lifecycle); err != nil {
		t.Fatalf("Failed to parse lifecycle file: %v", err)
	}

	// Convert to object
	version := "v1_0_0"
	lifecycleObj, err := helper.lifecycleToObject(&lifecycle, objectType, version)
	if err != nil {
		t.Fatalf("Failed to convert lifecycle to object: %v", err)
	}

	// Verify object structure
	if lifecycleObj[objects.FieldKeyKind] != "lifecycle" {
		t.Errorf("Expected kind 'lifecycle', got %v", lifecycleObj[objects.FieldKeyKind])
	}

	if lifecycleObj[objects.FieldKeyObjectType] != objectType {
		t.Errorf("Expected object_type '%s', got %v", objectType, lifecycleObj[objects.FieldKeyObjectType])
	}

	// Verify ID pattern (new standardized format: LIFECYCLE-{ABBR}-001)
	id, ok := lifecycleObj[objects.FieldKeyID].(string)
	if !ok {
		t.Fatal("Object ID not found or not a string")
	}

	// test_kind -> TES
	expectedID := "LIFECYCLE-TES-001"
	if id != expectedID {
		t.Errorf("Expected ID '%s', got '%s'", expectedID, id)
	}

	// Verify statuses are present
	statuses, ok := lifecycleObj[objects.FieldKeyStatuses].([]any)
	if !ok {
		t.Fatal("Statuses not found or not a list")
	}

	if len(statuses) == 0 {
		t.Error("Expected at least one status")
	}

	// Verify transitions are present
	transitions, ok := lifecycleObj[objects.FieldKeyTransitions].([]any)
	if !ok {
		t.Fatal("Transitions not found or not a list")
	}

	if len(transitions) == 0 {
		t.Error("Expected at least one transition")
	}
}

// TestLifecycleMigrationHelper_LifecycleToObjectFromFile tests file-based conversion
func TestLifecycleMigrationHelper_LifecycleToObjectFromFile(t *testing.T) {
	t.Parallel()
	lifecyclesDir, _, helper, cleanup := setupLifecycleMigrationTest(t)
	defer cleanup()

	objectType := "test_kind"
	lifecycleFile := createTestLifecycleFile(t, lifecyclesDir, objectType, "")

	version := "v1_0_0"
	lifecycleObj, err := helper.lifecycleToObjectFromFile(lifecycleFile, objectType, version)
	if err != nil {
		t.Fatalf("Failed to convert lifecycle file to object: %v", err)
	}

	if lifecycleObj == nil {
		t.Fatal("Lifecycle object is nil")
	}

	if lifecycleObj[objects.FieldKeyKind] != "lifecycle" {
		t.Errorf("Expected kind 'lifecycle', got %v", lifecycleObj[objects.FieldKeyKind])
	}
}

// TestLifecycleMigrationHelper_MigrateLifecycleFile tests full migration of a lifecycle file
func TestLifecycleMigrationHelper_MigrateLifecycleFile(t *testing.T) {
	t.Parallel()
	lifecyclesDir, storageProvider, helper, cleanup := setupLifecycleMigrationTest(t)
	defer cleanup()

	objectType := "test_kind"
	lifecycleFile := createTestLifecycleFile(t, lifecyclesDir, objectType, "")

	ctx := pkgctx.NewSystemContext()
	version := "v1_0_0"

	// Migrate lifecycle file
	err := helper.migrateLifecycleFile(ctx, lifecycleFile, objectType, version, false, false)
	if err != nil {
		t.Fatalf("Failed to migrate lifecycle file: %v", err)
	}

	// Verify object was created
	// New standardized ID format: LIFECYCLE-{ABBR}-001
	expectedID := "LIFECYCLE-TES-001" // test_kind -> TES
	secCtx := pkgctx.NewSystemSecurityContext()

	obj, err := storageProvider.Read(ctx, secCtx, expectedID)
	if err != nil {
		t.Fatalf("Failed to read migrated lifecycle object: %v", err)
	}

	if obj[objects.FieldKeyKind] != "lifecycle" {
		t.Errorf("Expected kind 'lifecycle', got %v", obj[objects.FieldKeyKind])
	}

	if obj[objects.FieldKeyObjectType] != objectType {
		t.Errorf("Expected object_type '%s', got %v", objectType, obj[objects.FieldKeyObjectType])
	}
}

// TestLifecycleMigrationHelper_MigrateLifecycleFile_SkipExisting tests skipping existing objects
func TestLifecycleMigrationHelper_MigrateLifecycleFile_SkipExisting(t *testing.T) {
	t.Parallel()
	lifecyclesDir, storageProvider, helper, cleanup := setupLifecycleMigrationTest(t)
	defer cleanup()

	objectType := "test_kind"
	lifecycleFile := createTestLifecycleFile(t, lifecyclesDir, objectType, "")

	ctx := pkgctx.NewSystemContext()
	version := "v1_0_0"
	expectedID := "LIFECYCLE-TES-001" // test_kind -> TES

	// Create object first
	lifecycleObj, err := helper.lifecycleToObjectFromFile(lifecycleFile, objectType, version)
	if err != nil {
		t.Fatalf("Failed to convert lifecycle: %v", err)
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	err = storageProvider.Create(ctx, secCtx, lifecycleObj)
	if err != nil {
		t.Fatalf("Failed to create lifecycle object: %v", err)
	}

	// Try to migrate again (should skip)
	err = helper.migrateLifecycleFile(ctx, lifecycleFile, objectType, version, false, false)
	if err != nil {
		t.Fatalf("Migration should skip existing object, got error: %v", err)
	}

	// Verify object still exists
	_, err = storageProvider.Read(ctx, secCtx, expectedID)
	if err != nil {
		t.Fatalf("Object should still exist after skip: %v", err)
	}
}

// TestLifecycleMigrationHelper_MigrateLifecycleFile_DryRun tests dry-run mode
func TestLifecycleMigrationHelper_MigrateLifecycleFile_DryRun(t *testing.T) {
	t.Parallel()
	lifecyclesDir, storageProvider, helper, cleanup := setupLifecycleMigrationTest(t)
	defer cleanup()

	objectType := "test_kind"
	lifecycleFile := createTestLifecycleFile(t, lifecyclesDir, objectType, "")

	ctx := pkgctx.NewSystemContext()
	version := "v1_0_0"
	expectedID := "LIFECYCLE-TES-001" // test_kind -> TES

	// Run in dry-run mode
	err := helper.migrateLifecycleFile(ctx, lifecycleFile, objectType, version, false, true)
	if err != nil {
		t.Fatalf("Dry-run should not error: %v", err)
	}

	// Verify object was NOT created
	secCtx := pkgctx.NewSystemSecurityContext()
	_, err = storageProvider.Read(ctx, secCtx, expectedID)
	if err == nil {
		t.Error("Object should not exist after dry-run")
	}
}

// TestLifecycleMigrationHelper_LifecycleToObject_InvalidFile tests handling of invalid lifecycle files
func TestLifecycleMigrationHelper_LifecycleToObject_InvalidFile(t *testing.T) {
	t.Parallel()
	_, _, helper, cleanup := setupLifecycleMigrationTest(t)
	defer cleanup()

	// Test with non-existent file
	_, err := helper.lifecycleToObjectFromFile("/nonexistent/file.yaml", "test_kind", "v1_0_0")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

// TestLifecycleMigrationHelper_LifecycleToObject_MissingObjectType tests object_type inference from filename
func TestLifecycleMigrationHelper_LifecycleToObject_MissingObjectType(t *testing.T) {
	t.Parallel()
	lifecyclesDir, _, helper, cleanup := setupLifecycleMigrationTest(t)
	defer cleanup()

	// Create lifecycle file without object_type field
	content := `statuses:
  - value: draft
    display: Draft
    initial: true
  - value: active
    display: Active
    terminal: true
`
	objectType := "test_kind"
	lifecycleFile := createTestLifecycleFile(t, lifecyclesDir, objectType, content)

	// Convert using filename for object_type
	version := "v1_0_0"
	lifecycleObj, err := helper.lifecycleToObjectFromFile(lifecycleFile, objectType, version)
	if err != nil {
		t.Fatalf("Failed to convert lifecycle (object_type from filename): %v", err)
	}

	if lifecycleObj[objects.FieldKeyObjectType] != objectType {
		t.Errorf("Expected object_type '%s' from filename, got %v", objectType, lifecycleObj[objects.FieldKeyObjectType])
	}
}

// TestLifecycleMigrationHelper_LifecycleToObject_ComplexLifecycle tests complex lifecycle with inheritance
func TestLifecycleMigrationHelper_LifecycleToObject_ComplexLifecycle(t *testing.T) {
	t.Parallel()
	lifecyclesDir, _, helper, cleanup := setupLifecycleMigrationTest(t)
	defer cleanup()

	// Create complex lifecycle with all fields
	content := `object_type: complex_kind
extends: base_lifecycle
status_mapping:
  proposed: draft
  approved: active
statuses:
  - value: draft
    display: Draft
    initial: true
  - value: active
    display: Active
    terminal: true
  - value: archived
    display: Archived
    archive: true
    terminal: true
transitions:
  - from: draft
    to: active
    description: Activate
    manual: true
    auto: false
    preconditions:
      - approval required
  - from: active
    to: archived
    description: Archive
    manual: true
    auto: false
percent_complete:
  method: status_defaults
  default_by_status:
    draft: 0
    active: 100
    archived: 100
`
	objectType := "complex_kind"
	lifecycleFile := createTestLifecycleFile(t, lifecyclesDir, objectType, content)

	version := "v1_0_0"
	lifecycleObj, err := helper.lifecycleToObjectFromFile(lifecycleFile, objectType, version)
	if err != nil {
		t.Fatalf("Failed to convert complex lifecycle: %v", err)
	}

	// Verify all fields are present
	if lifecycleObj[objects.FieldKeyExtends] == nil {
		t.Error("Expected 'extends' field")
	}

	if lifecycleObj[objects.FieldKeyStatusMapping] == nil {
		t.Error("Expected 'status_mapping' field")
	}

	// Verify preconditions in transitions
	transitions, ok := lifecycleObj[objects.FieldKeyTransitions].([]any)
	if !ok || len(transitions) == 0 {
		t.Fatal("Transitions not found")
	}

	firstTransition, ok := transitions[0].(map[string]any)
	if !ok {
		t.Fatal("First transition is not a map")
	}

	if firstTransition["preconditions"] == nil {
		t.Error("Expected 'preconditions' in transition")
	}
}
