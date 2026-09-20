package migration

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func setupExecutorForPrereqTest(t *testing.T) (*Executor, func()) {
	t.Helper()
	tmpDir := t.TempDir()

	processDir := datacell.ProcessPrimaryDir(tmpDir)
	_ = fileutil.MkdirAll(processDir, paths.DirPerm755)
	specsDir := filepath.Join(processDir, "_internal", "object_specs")
	_ = fileutil.MkdirAll(specsDir, paths.DirPerm755)

	storageProvider, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, storageProvider)
	logger := logging.GetLoggerFromProfile("test")
	executor := NewExecutor(storageProvider, tmpDir, logger)
	return executor, func() {
		_ = storageProvider.Shutdown(context.Background())
	}
}

func TestValidatePrerequisites_EmptyPasses(t *testing.T) {
	t.Parallel()
	executor, cleanup := setupExecutorForPrereqTest(t)
	defer cleanup()

	ctx := context.Background()
	spec := &Spec{ID: "test", Prerequisites: nil}
	if err := executor.validatePrerequisites(ctx, spec); err != nil {
		t.Fatalf("expected nil for empty prerequisites: %v", err)
	}

	spec.Prerequisites = []Prerequisite{}
	if err := executor.validatePrerequisites(ctx, spec); err != nil {
		t.Fatalf("expected nil for empty slice: %v", err)
	}
}

func TestValidatePrerequisites_DirectoryMustExist(t *testing.T) {
	t.Parallel()
	executor, cleanup := setupExecutorForPrereqTest(t)
	defer cleanup()

	ctx := context.Background()
	existsTrue := true
	spec := &Spec{
		ID: "test",
		Prerequisites: []Prerequisite{
			{Type: "directory", Directory: paths.ProcessDir, Exists: &existsTrue},
		},
	}
	if err := executor.validatePrerequisites(ctx, spec); err != nil {
		t.Fatalf("%s exists in tmpDir, expected pass: %v", paths.ProcessDir, err)
	}

	existsFalse := false
	spec.Prerequisites[0].Exists = &existsFalse
	if err := executor.validatePrerequisites(ctx, spec); err == nil {
		t.Fatal("directory must not exist but it does; expected error")
	}
}

func TestValidatePrerequisites_DirectoryMustNotExist(t *testing.T) {
	t.Parallel()
	executor, cleanup := setupExecutorForPrereqTest(t)
	defer cleanup()

	ctx := context.Background()
	existsFalse := false
	spec := &Spec{
		ID: "test",
		Prerequisites: []Prerequisite{
			{Type: "directory", Directory: "nonexistent_subdir", Exists: &existsFalse},
		},
	}
	if err := executor.validatePrerequisites(ctx, spec); err != nil {
		t.Fatalf("nonexistent_subdir does not exist, expected pass: %v", err)
	}
}

func TestRunValidation_EmptyPasses(t *testing.T) {
	t.Parallel()
	executor, cleanup := setupExecutorForPrereqTest(t)
	defer cleanup()

	ctx := context.Background()
	spec := &Spec{ID: "test", Validation: nil}
	if err := executor.runValidation(ctx, spec, nil); err != nil {
		t.Fatalf("expected nil for empty validation: %v", err)
	}
	spec.Validation = []ValidationRule{}
	if err := executor.runValidation(ctx, spec, nil); err != nil {
		t.Fatalf("expected nil for empty slice: %v", err)
	}
}

func TestRunValidation_ObjectCount(t *testing.T) {
	t.Parallel()
	executor, cleanup := setupExecutorForPrereqTest(t)
	defer cleanup()

	ctx := context.Background()
	stepOutputs := map[string]any{
		"scan-step": map[string]any{"count": 10},
	}
	spec := &Spec{
		ID: "test",
		Validation: []ValidationRule{
			{Type: "object_count", Config: map[string]any{"step_id": "scan-step", "min": 5}},
			{Type: "object_count", Config: map[string]any{"step_id": "scan-step", "max": 20}},
			{Type: "object_count", Config: map[string]any{"step_id": "scan-step", "expected": 10}},
		},
	}
	if err := executor.runValidation(ctx, spec, stepOutputs); err != nil {
		t.Fatalf("expected pass: %v", err)
	}

	spec.Validation = []ValidationRule{{Type: "object_count", Config: map[string]any{"step_id": "scan-step", "expected": 99}}}
	if err := executor.runValidation(ctx, spec, stepOutputs); err == nil {
		t.Fatal("expected error when count != expected")
	}
}
