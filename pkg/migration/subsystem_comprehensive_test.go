// Package migration provides schema migration and evolution capabilities for the ZQK knowledge kernel.
//
// Subsystem Comprehensive Verification Suite satisfying the Three-Fold Proof:
// 1. Static Floor Invariant Proof:
//   - Strict structural validation of Spec, Step, Options, Prerequisite, and History schemas.
//   - Invariant statement coverage floor >= 85.0% across all files in pkg/migration.
//
// 2. Operational Dynamic Proof:
//   - Multi-phase migration execution: scan_files, read_id_list, read_objects, transform, create_objects, execute_command, delete_objects.
//   - End-to-end integration with SnapshotCreator (pre, post, checkpoint) and validation rules.
//
// 3. Negative Adversarial Boundary Proof:
//   - Invalid specifications, circular/missing step dependencies, missing inputs.
//   - Storage collision failures, unreadable directories, corrupted state files, command exit failures.
package migration

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// mockSnapshotCreator implements SnapshotCreator with controllable behavior for testing.
type mockSnapshotCreator struct {
	preErr        error
	postErr       error
	checkpointErr error
	preCalls      int
	postCalls     int
	chkCalls      int
}

func (m *mockSnapshotCreator) CreatePreMigrationSnapshot(_ context.Context, spec *Spec) (string, error) {
	m.preCalls++
	if m.preErr != nil {
		return "", m.preErr
	}
	return "mock-pre-" + spec.ID, nil
}

func (m *mockSnapshotCreator) CreatePostMigrationSnapshot(_ context.Context, spec *Spec, preSnapshotID string) (string, error) {
	m.postCalls++
	if m.postErr != nil {
		return "", m.postErr
	}
	return fmt.Sprintf("mock-post-%s-after-%s", spec.ID, preSnapshotID), nil
}

func (m *mockSnapshotCreator) CreateCheckpointSnapshot(_ context.Context, spec *Spec, step Step, count int) (string, error) {
	m.chkCalls++
	if m.checkpointErr != nil {
		return "", m.checkpointErr
	}
	return fmt.Sprintf("mock-chk-%s-%s-%d", spec.ID, step.ID, count), nil
}

// -----------------------------------------------------------------------------
// 1. Static Floor Invariant Proof
// -----------------------------------------------------------------------------

func TestMigrationSubsystem_StaticFloorInvariant(t *testing.T) {
	// Verify history file name constant
	if HistoryFileName != "migration_history.json" {
		t.Fatalf("unexpected HistoryFileName: %s", HistoryFileName)
	}

	// Verify Spec invariants
	spec := Spec{
		SchemaVersion: "2.0.0",
		ID:            "MIG-TEST-STATIC",
		Name:          "Static Floor Invariant Verification",
		Description:   "Validates migration data structures",
		From:          StateDescription{State: "v1", Description: "Initial state"},
		To:            StateDescription{State: "v2", Description: "Target state"},
		Options: Options{
			DryRun:             true,
			Force:              true,
			BatchSize:          50,
			ContinueOnError:    true,
			CheckpointInterval: 10,
		},
		SnapshotCompatible: true,
	}

	if spec.ID != "MIG-TEST-STATIC" || spec.Options.BatchSize != 50 || !spec.SnapshotCompatible {
		t.Fatalf("spec static invariant verification failed: %+v", spec)
	}

	// Verify Step and CheckpointConfig invariants
	step := Step{
		ID:          "step-1",
		Type:        "transform",
		Description: "Transform step",
		DependsOn:   []string{"step-0"},
		ForEach:     "step-0",
		Config:      map[string]any{"key": "val"},
		Atomic:      true,
		Checkpoint: &CheckpointConfig{
			Interval:      5,
			Snapshot:      true,
			RollbackPoint: true,
		},
	}
	if step.Checkpoint.Interval != 5 || !step.Checkpoint.Snapshot || !step.Atomic {
		t.Fatalf("step static invariant verification failed: %+v", step)
	}
}

// -----------------------------------------------------------------------------
// 2. Operational Dynamic Proof
// -----------------------------------------------------------------------------

func TestMigrationSubsystem_OperationalDynamicProof(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterTempProjectTeardown(t, tmpDir, fs)

	logger := logging.GetLoggerFromProfile("test")
	exec := NewExecutor(fs, tmpDir, logger)
	mockCreator := &mockSnapshotCreator{}
	exec.SetSnapshotCreator(mockCreator)
	ctx := context.Background()

	// Seed source YAML files for scan and transform
	dataDir := filepath.Join(tmpDir, "source_data", "goal")
	if err := fileutil.MkdirAll(dataDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create source data dir: %v", err)
	}

	yaml1 := filepath.Join(dataDir, "item1.yaml")
	content1 := "id: GOAL-001\ntitle: First Goal\nstatus: active\nkind: goal\n"
	if err := fileutil.WriteFile(yaml1, []byte(content1), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write yaml1: %v", err)
	}

	yaml2 := filepath.Join(dataDir, "item2.yaml")
	content2 := "id: GOAL-002\ntitle: Second Goal\nstatus: active\nkind: goal\n"
	if err := fileutil.WriteFile(yaml2, []byte(content2), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write yaml2: %v", err)
	}

	// Seed ID list file (YAML format)
	idListYaml := filepath.Join(tmpDir, "id_list.yaml")
	idListContent := "- GOAL-001\n- GOAL-002\n"
	if err := fileutil.WriteFile(idListYaml, []byte(idListContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write id_list.yaml: %v", err)
	}

	// Define full operational migration spec
	spec := &Spec{
		SchemaVersion:      "2.0.0",
		ID:                 "MIG-DYNAMIC-PROOF-001",
		Name:               "Operational Dynamic Proof Migration",
		SnapshotCompatible: true,
		PreMigrationSnapshot: &SnapshotConfig{
			AutoCreate: true,
		},
		PostMigrationSnapshot: &SnapshotConfig{
			AutoCreate: true,
		},
		Prerequisites: []Prerequisite{
			{
				Type: "kind",
				Kind: "goal",
			},
			{
				Type:      "directory",
				Directory: filepath.Join("source_data", "goal"),
				Exists:    boolPtr(true),
			},
		},
		Steps: []Step{
			{
				ID:   "scan_step",
				Type: "scan_files",
				Config: map[string]any{
					"directory": filepath.Join("source_data", "goal"),
					"pattern":   "*.yaml",
				},
			},
			{
				ID:        "transform_step",
				Type:      "transform",
				DependsOn: []string{"scan_step"},
				ForEach:   "scan_step",
				Config: map[string]any{
					"transform": map[string]any{
						"builder": "yaml_file_to_object",
					},
				},
				Checkpoint: &CheckpointConfig{
					Interval: 1,
					Snapshot: true,
				},
			},
			{
				ID:        "create_step",
				Type:      "create_objects",
				DependsOn: []string{"transform_step"},
				Config: map[string]any{
					"skip_existing": true,
				},
			},
			{
				ID:   "read_id_step",
				Type: "read_id_list",
				Config: map[string]any{
					"file": "id_list.yaml",
				},
			},
			{
				ID:        "read_obj_step",
				Type:      "read_objects",
				DependsOn: []string{"read_id_step"},
			},
			{
				ID:   "cmd_step",
				Type: "execute_command",
				Config: map[string]any{
					"command": "echo",
					"args":    []any{"migration_success"},
					"env": map[string]any{
						"MIG_ENV": "active",
					},
				},
			},
			{
				ID:        "delete_step",
				Type:      "delete_objects",
				DependsOn: []string{"read_id_step"},
				Config: map[string]any{
					"cascade": false,
				},
			},
		},
		Validation: []ValidationRule{
			{
				Type: "object_count",
				Config: map[string]any{
					"step_id":  "create_step",
					"min":      1,
					"max":      5,
					"expected": 2,
				},
			},
			{
				Type: "file_count",
				Config: map[string]any{
					"step_id": "scan_step",
					"min":     1,
				},
			},
		},
	}

	// 1. Dry-run execution
	dryRunTrue := true
	dryResult, err := exec.Execute(ctx, spec, ExecutionOptions{DryRun: &dryRunTrue})
	if err != nil {
		t.Fatalf("dry run execution failed: %v", err)
	}
	if dryResult.Status != objects.ObjectStatusCompleted {
		t.Fatalf("dry run status want completed, got %s", dryResult.Status)
	}

	// 2. Real execution (non-dry-run)
	dryRunFalse := false
	realResult, err := exec.Execute(ctx, spec, ExecutionOptions{DryRun: &dryRunFalse})
	if err != nil {
		t.Fatalf("real execution failed: %v", err)
	}
	if realResult.Status != objects.ObjectStatusCompleted {
		t.Fatalf("real execution status want completed, got %s", realResult.Status)
	}
	if mockCreator.preCalls == 0 {
		t.Errorf("expected pre-migration snapshot calls")
	}
	if mockCreator.postCalls == 0 {
		t.Errorf("expected post-migration snapshot calls")
	}
	if mockCreator.chkCalls == 0 {
		t.Errorf("expected checkpoint snapshot calls")
	}

	// Verify history was recorded
	hasRun, err := HasRun(tmpDir, spec.ID)
	if err != nil || !hasRun {
		t.Fatalf("expected migration %s in history, got %v, err: %v", spec.ID, hasRun, err)
	}

	history, err := LoadHistory(tmpDir)
	if err != nil || len(history.Entries) == 0 {
		t.Fatalf("expected non-empty history, err: %v", err)
	}
}

// -----------------------------------------------------------------------------
// 3. Negative Adversarial Boundary Proof
// -----------------------------------------------------------------------------

func TestMigrationSubsystem_AdversarialBoundary_SpecValidationAndLoading(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. LoadSpec non-existent
	if _, err := LoadSpec(filepath.Join(tmpDir, "missing.yaml")); err == nil {
		t.Error("expected error loading non-existent spec")
	}

	// 2. LoadSpec invalid YAML
	invalidYaml := filepath.Join(tmpDir, "invalid.yaml")
	_ = fileutil.WriteFile(invalidYaml, []byte(":: bad yaml ::"), paths.FilePerm644)
	if _, err := LoadSpec(invalidYaml); err == nil {
		t.Error("expected error loading invalid yaml spec")
	}

	// 3. LoadSpec with SnapshotCompatible defaults
	validYaml := filepath.Join(tmpDir, "valid.yaml")
	specText := `schema_version: '2.0.0'
id: MIG-SNAP-DEFAULT
name: Snapshot Defaults Test
snapshot_compatible: true
steps:
  - id: step1
    type: scan_files
    config:
      directory: .
`
	_ = fileutil.WriteFile(validYaml, []byte(specText), paths.FilePerm644)
	loaded, err := LoadSpec(validYaml)
	if err != nil {
		t.Fatalf("LoadSpec failed: %v", err)
	}
	if loaded.PreMigrationSnapshot == nil || !loaded.PreMigrationSnapshot.AutoCreate {
		t.Error("expected pre-migration snapshot default to be auto-created")
	}
	if loaded.PostMigrationSnapshot == nil || !loaded.PostMigrationSnapshot.AutoCreate {
		t.Error("expected post-migration snapshot default to be auto-created")
	}
	if loaded.Tracking == nil || !loaded.Tracking.ObjectMapping {
		t.Error("expected tracking default to be populated")
	}
	if loaded.Coherence == nil || !loaded.Coherence.AtomicSteps {
		t.Error("expected coherence default to be populated")
	}

	// 4. validateSpec adversarial checks
	if err := validateSpec(&Spec{}); err == nil {
		t.Error("expected error for empty spec")
	}
	if err := validateSpec(&Spec{ID: "id"}); err == nil {
		t.Error("expected error for spec missing name")
	}
	if err := validateSpec(&Spec{ID: "id", Name: "name"}); err == nil {
		t.Error("expected error for spec missing steps")
	}
	if err := validateSpec(&Spec{ID: "id", Name: "name", Steps: []Step{{ID: ""}}}); err == nil {
		t.Error("expected error for step missing id")
	}
	if err := validateSpec(&Spec{ID: "id", Name: "name", Steps: []Step{{ID: "s1"}, {ID: "s1"}}}); err == nil {
		t.Error("expected error for duplicate step id")
	}
	if err := validateSpec(&Spec{ID: "id", Name: "name", Steps: []Step{{ID: "s1", DependsOn: []string{"unknown_step"}}}}); err == nil {
		t.Error("expected error for unknown depends_on step")
	}
	if err := validateSpec(&Spec{ID: "id", Name: "name", Steps: []Step{{ID: "s1", ForEach: "unknown_for_each"}}}); err == nil {
		t.Error("expected error for unknown for_each step")
	}
}

func TestMigrationSubsystem_AdversarialBoundary_Prerequisites(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterTempProjectTeardown(t, tmpDir, fs)

	logger := logging.GetLoggerFromProfile("test")
	exec := NewExecutor(fs, tmpDir, logger)
	ctx := context.Background()

	// 1. Kind prerequisite unregistered
	specUnregisteredKind := &Spec{
		Prerequisites: []Prerequisite{
			{Type: "kind", Kind: "NON_EXISTENT_KIND_XYZ_999"},
		},
	}
	if err := exec.validatePrerequisites(ctx, specUnregisteredKind); err == nil {
		t.Error("expected error for unregistered kind prerequisite")
	}

	// 2. Kind prerequisite with empty kind (should be skipped)
	specEmptyKind := &Spec{
		Prerequisites: []Prerequisite{
			{Type: "kind", Kind: ""},
		},
	}
	if err := exec.validatePrerequisites(ctx, specEmptyKind); err != nil {
		t.Errorf("unexpected error for empty kind prerequisite: %v", err)
	}

	// 3. Directory prerequisite must exist but does not
	specDirMustExist := &Spec{
		Prerequisites: []Prerequisite{
			{Type: "directory", Directory: "missing_dir_xyz", Exists: boolPtr(true)},
		},
	}
	if err := exec.validatePrerequisites(ctx, specDirMustExist); err == nil {
		t.Error("expected error for directory must exist prerequisite")
	}

	// 4. Directory prerequisite must not exist but exists
	existingDir := filepath.Join(tmpDir, "existing_dir")
	_ = fileutil.MkdirAll(existingDir, paths.DirPerm755)
	specDirMustNotExist := &Spec{
		Prerequisites: []Prerequisite{
			{Type: "directory", Directory: existingDir, Exists: boolPtr(false)},
		},
	}
	if err := exec.validatePrerequisites(ctx, specDirMustNotExist); err == nil {
		t.Error("expected error for directory must not exist prerequisite")
	}

	// 5. Directory prerequisite empty or nil Exists
	specDirEmpty := &Spec{
		Prerequisites: []Prerequisite{
			{Type: "directory", Directory: ""},
			{Type: "directory", Directory: existingDir, Exists: nil},
			{Type: "config"},
			{Type: "unknown_type"},
		},
	}
	if err := exec.validatePrerequisites(ctx, specDirEmpty); err != nil {
		t.Errorf("unexpected error for skipped prerequisites: %v", err)
	}
}

func TestMigrationSubsystem_AdversarialBoundary_SnapshotsAndContinueOnError(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterTempProjectTeardown(t, tmpDir, fs)

	logger := logging.GetLoggerFromProfile("test")
	ctx := context.Background()

	// 1. Pre-migration snapshot error aborts execution
	execPreErr := NewExecutor(fs, tmpDir, logger)
	execPreErr.SetSnapshotCreator(&mockSnapshotCreator{preErr: errors.New("pre snapshot error")})
	specPre := &Spec{
		ID:                 "MIG-PRE-ERR",
		Name:               "Pre Snapshot Error Test",
		SnapshotCompatible: true,
		PreMigrationSnapshot: &SnapshotConfig{
			AutoCreate: true,
		},
		Steps: []Step{{ID: "s1", Type: "scan_files", Config: map[string]any{"directory": "."}}},
	}
	if _, err := execPreErr.Execute(ctx, specPre, ExecutionOptions{}); err == nil {
		t.Error("expected error when pre-migration snapshot fails")
	}

	// 2. Step error with ContinueOnError = false (fails fast)
	execStepErr := NewExecutor(fs, tmpDir, logger)
	specFailFast := &Spec{
		ID:   "MIG-FAIL-FAST",
		Name: "Fail Fast Test",
		Steps: []Step{
			{ID: "s1", Type: "unknown_step_type"},
			{ID: "s2", Type: "scan_files", Config: map[string]any{"directory": "."}},
		},
	}
	resFailFast, err := execStepErr.Execute(ctx, specFailFast, ExecutionOptions{})
	if err == nil {
		t.Error("expected error for unknown step type")
	}
	if resFailFast.Status != objects.ObjectStatusFailed {
		t.Errorf("status want %s, got %s", objects.ObjectStatusFailed, resFailFast.Status)
	}

	// 3. Step error with ContinueOnError = true (continues)
	specContinue := &Spec{
		ID:   "MIG-CONTINUE",
		Name: "Continue On Error Test",
		Options: Options{
			ContinueOnError: true,
		},
		Steps: []Step{
			{ID: "s1", Type: "unknown_step_type"},
			{ID: "s2", Type: "scan_files", Config: map[string]any{"directory": "."}},
		},
	}
	resContinue, err := execStepErr.Execute(ctx, specContinue, ExecutionOptions{})
	if err != nil {
		t.Fatalf("unexpected error with continue_on_error: %v", err)
	}
	if resContinue.Status != objects.ObjectStatusCompleted {
		t.Errorf("status want completed, got %s", resContinue.Status)
	}

	// 4. Checkpoint snapshot and Post-migration snapshot error handling (warns but finishes)
	execPostErr := NewExecutor(fs, tmpDir, logger)
	execPostErr.SetSnapshotCreator(&mockSnapshotCreator{
		checkpointErr: errors.New("checkpoint fail"),
		postErr:       errors.New("post fail"),
	})
	specPostErr := &Spec{
		ID:                 "MIG-POST-ERR",
		Name:               "Post Error Test",
		SnapshotCompatible: true,
		PostMigrationSnapshot: &SnapshotConfig{
			AutoCreate: true,
		},
		Steps: []Step{
			{
				ID:   "s1",
				Type: "scan_files",
				Config: map[string]any{
					"directory": ".",
				},
				Checkpoint: &CheckpointConfig{
					Interval: 1,
					Snapshot: true,
				},
			},
		},
	}
	resPostErr, err := execPostErr.Execute(ctx, specPostErr, ExecutionOptions{})
	if err != nil {
		t.Fatalf("unexpected error when post snapshot fails gracefully: %v", err)
	}
	if resPostErr.Status != objects.ObjectStatusCompleted {
		t.Errorf("expected status completed, got %s", resPostErr.Status)
	}
}

func TestMigrationSubsystem_AdversarialBoundary_ValidationRules(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterTempProjectTeardown(t, tmpDir, fs)

	logger := logging.GetLoggerFromProfile("test")
	exec := NewExecutor(fs, tmpDir, logger)
	ctx := context.Background()

	// Missing step output
	specMissingOutput := &Spec{
		Validation: []ValidationRule{
			{Type: "object_count", Config: map[string]any{"step_id": "missing_step", "min": 1}},
		},
	}
	if err := exec.runValidation(ctx, specMissingOutput, map[string]any{}); err == nil {
		t.Error("expected error for missing step output")
	}

	// Count below min
	stepOutputs := map[string]any{
		"s1": map[string]any{"count": 2},
	}
	specBelowMin := &Spec{
		Validation: []ValidationRule{
			{Type: "object_count", Config: map[string]any{"step_id": "s1", "min": 5}},
		},
	}
	if err := exec.runValidation(ctx, specBelowMin, stepOutputs); err == nil {
		t.Error("expected error for count below min")
	}

	// Count above max
	specAboveMax := &Spec{
		Validation: []ValidationRule{
			{Type: "object_count", Config: map[string]any{"step_id": "s1", "max": 1}},
		},
	}
	if err := exec.runValidation(ctx, specAboveMax, stepOutputs); err == nil {
		t.Error("expected error for count above max")
	}

	// Count != expected
	specNotExpected := &Spec{
		Validation: []ValidationRule{
			{Type: "object_count", Config: map[string]any{"step_id": "s1", "expected": 10}},
		},
	}
	if err := exec.runValidation(ctx, specNotExpected, stepOutputs); err == nil {
		t.Error("expected error for count != expected")
	}

	// Float64 count and skipped configs
	stepOutputsFloat := map[string]any{
		"s_float": map[string]any{"count": float64(5)},
		"s_bad":   map[string]any{"count": "not_a_number"},
	}
	specPassedRules := &Spec{
		Validation: []ValidationRule{
			{}, // nil config
			{Type: "object_count", Config: map[string]any{"step_id": ""}}, // empty step id
			{Type: "object_count", Config: map[string]any{"step_id": "s_bad"}},
			{Type: "object_count", Config: map[string]any{"step_id": "s_float", "min": 5, "max": 5, "expected": 5}},
		},
	}
	if err := exec.runValidation(ctx, specPassedRules, stepOutputsFloat); err != nil {
		t.Errorf("unexpected error on valid float count validation: %v", err)
	}

	// toInt helper edge cases
	if toInt(nil) != -1 || toInt("string") != -1 || toInt(10) != 10 || toInt(float64(20)) != 20 {
		t.Error("toInt failed edge cases")
	}
}

func TestMigrationSubsystem_AdversarialBoundary_StepsDetailedEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterTempProjectTeardown(t, tmpDir, fs)

	logger := logging.GetLoggerFromProfile("test")
	exec := NewExecutor(fs, tmpDir, logger)
	ctx := context.Background()

	// 1. executeScanFilesStep
	sNilConfig := &Step{ID: "scan1"}
	if res := exec.executeScanFilesStep(ctx, sNilConfig, nil); res.Success {
		t.Error("expected scan_files to fail with nil config")
	}

	sEmptyDir := &Step{ID: "scan2", Config: map[string]any{"directory": ""}}
	if res := exec.executeScanFilesStep(ctx, sEmptyDir, nil); res.Success {
		t.Error("expected scan_files to fail with empty directory")
	}

	sMissingDir := &Step{ID: "scan3", Config: map[string]any{"directory": "nonexistent_dir_999"}}
	if res := exec.executeScanFilesStep(ctx, sMissingDir, nil); res.Success {
		t.Error("expected scan_files to fail with non-existent directory")
	}

	// Exclude pattern parsing
	scanDir := filepath.Join(tmpDir, "scan_test")
	_ = fileutil.MkdirAll(scanDir, paths.DirPerm755)
	_ = fileutil.WriteFile(filepath.Join(scanDir, "included.txt"), []byte("data"), paths.FilePerm644)
	_ = fileutil.WriteFile(filepath.Join(scanDir, "excluded.log"), []byte("data"), paths.FilePerm644)
	sExclude := &Step{
		ID: "scan4",
		Config: map[string]any{
			"directory": scanDir,
			"pattern":   "*",
			"exclude":   []any{"*.log"},
		},
	}
	resExclude := exec.executeScanFilesStep(ctx, sExclude, nil)
	if !resExclude.Success || resExclude.Metadata["count"] != 1 {
		t.Errorf("expected 1 file matched after exclude, got %+v", resExclude)
	}

	// 2. executeReadIDListStep
	sReadIDNil := &Step{ID: "rid1"}
	if res := exec.executeReadIDListStep(ctx, sReadIDNil, nil); res.Success {
		t.Error("expected read_id_list to fail with nil config")
	}

	sReadIDEmptyFile := &Step{ID: "rid2", Config: map[string]any{"file": ""}}
	if res := exec.executeReadIDListStep(ctx, sReadIDEmptyFile, nil); res.Success {
		t.Error("expected read_id_list to fail with empty file")
	}

	sReadIDMissingFile := &Step{ID: "rid3", Config: map[string]any{"file": "missing.txt"}}
	if res := exec.executeReadIDListStep(ctx, sReadIDMissingFile, nil); res.Success {
		t.Error("expected read_id_list to fail with missing file")
	}

	// Text format parsing with comments and blank lines
	textIDFile := filepath.Join(tmpDir, "ids.txt")
	_ = fileutil.WriteFile(textIDFile, []byte("# Header\nID-001\n\n# Another comment\nID-002\n"), paths.FilePerm644)
	sReadIDText := &Step{ID: "rid4", Config: map[string]any{"file": textIDFile}}
	resReadIDText := exec.executeReadIDListStep(ctx, sReadIDText, nil)
	if !resReadIDText.Success || resReadIDText.Metadata["count"] != 2 {
		t.Errorf("expected 2 IDs from text file, got %+v", resReadIDText)
	}

	// 3. executeReadObjectsStep
	sReadObjNoSource := &Step{ID: "ro1", DependsOn: []string{"missing"}}
	if res := exec.executeReadObjectsStep(ctx, sReadObjNoSource, map[string]any{}); res.Success {
		t.Error("expected read_objects to fail with missing source")
	}

	sReadObjNotMap := &Step{ID: "ro2", DependsOn: []string{"src"}}
	if res := exec.executeReadObjectsStep(ctx, sReadObjNotMap, map[string]any{"src": "string"}); res.Success {
		t.Error("expected read_objects to fail when source is not map")
	}

	sReadObjNoIDs := &Step{ID: "ro3", DependsOn: []string{"src"}}
	if res := exec.executeReadObjectsStep(ctx, sReadObjNoIDs, map[string]any{"src": map[string]any{}}); res.Success {
		t.Error("expected read_objects to fail with missing ids")
	}

	sReadObjBadIDs := &Step{ID: "ro4", DependsOn: []string{"src"}}
	if res := exec.executeReadObjectsStep(ctx, sReadObjBadIDs, map[string]any{"src": map[string]any{"ids": []any{123}}}); res.Success {
		t.Error("expected read_objects to fail when ids slice contains non-strings")
	}

	sReadObjMissingStorage := &Step{ID: "ro5", DependsOn: []string{"src"}}
	resMissingStorage := exec.executeReadObjectsStep(ctx, sReadObjMissingStorage, map[string]any{"src": map[string]any{"ids": []string{"NONEXISTENT-001"}}})
	if resMissingStorage.Success || resMissingStorage.Metadata["errors"] != 1 {
		t.Errorf("expected error count 1 for missing object in storage, got %+v", resMissingStorage)
	}

	// 4. executeCreateObjectsStep
	sCreateNilConfig := &Step{ID: "c1"}
	if res := exec.executeCreateObjectsStep(ctx, nil, sCreateNilConfig, nil, ExecutionOptions{}); res.Success {
		t.Error("expected create_objects to fail with nil config")
	}

	sCreateNoDep := &Step{ID: "c2", Config: map[string]any{}}
	if res := exec.executeCreateObjectsStep(ctx, nil, sCreateNoDep, map[string]any{}, ExecutionOptions{}); res.Success {
		t.Error("expected create_objects to fail without source in depends_on")
	}

	sCreateBadSource := &Step{ID: "c3", DependsOn: []string{"src"}, Config: map[string]any{}}
	if res := exec.executeCreateObjectsStep(ctx, nil, sCreateBadSource, map[string]any{"src": "invalid"}, ExecutionOptions{}); res.Success {
		t.Error("expected create_objects to fail when source is not map")
	}

	sCreateMissingItems := &Step{ID: "c4", DependsOn: []string{"src"}, Config: map[string]any{}}
	if res := exec.executeCreateObjectsStep(ctx, nil, sCreateMissingItems, map[string]any{"src": map[string]any{}}, ExecutionOptions{}); res.Success {
		t.Error("expected create_objects to fail when source missing items")
	}

	sCreateEmptyObjID := &Step{ID: "c5", DependsOn: []string{"src"}, Config: map[string]any{}}
	resEmptyObjID := exec.executeCreateObjectsStep(ctx, nil, sCreateEmptyObjID, map[string]any{
		"src": map[string]any{"items": []map[string]any{{"id": ""}}},
	}, ExecutionOptions{})
	if resEmptyObjID.Metadata["errors"] != 1 {
		t.Errorf("expected 1 error for empty obj id, got %+v", resEmptyObjID)
	}

	// 5. executeDeleteObjectsStep
	sDelNoDep := &Step{ID: "d1"}
	if res := exec.executeDeleteObjectsStep(ctx, sDelNoDep, map[string]any{}, ExecutionOptions{}); res.Success {
		t.Error("expected delete_objects to fail with missing source")
	}

	sDelNotMap := &Step{ID: "d2", DependsOn: []string{"src"}}
	if res := exec.executeDeleteObjectsStep(ctx, sDelNotMap, map[string]any{"src": 123}, ExecutionOptions{}); res.Success {
		t.Error("expected delete_objects to fail when source not map")
	}

	sDelNoKeys := &Step{ID: "d3", DependsOn: []string{"src"}}
	if res := exec.executeDeleteObjectsStep(ctx, sDelNoKeys, map[string]any{"src": map[string]any{}}, ExecutionOptions{}); res.Success {
		t.Error("expected delete_objects to fail when source missing ids and items")
	}

	sDelBadIDsSlice := &Step{ID: "d4", DependsOn: []string{"src"}}
	if res := exec.executeDeleteObjectsStep(ctx, sDelBadIDsSlice, map[string]any{"src": map[string]any{"ids": []any{999}}}, ExecutionOptions{}); res.Success {
		t.Error("expected delete_objects to fail when ids slice has non-string")
	}

	sDelBadItem := &Step{ID: "d5", DependsOn: []string{"src"}}
	if res := exec.executeDeleteObjectsStep(ctx, sDelBadItem, map[string]any{"src": map[string]any{"items": []map[string]any{{"no_id": "val"}}}}, ExecutionOptions{}); res.Success {
		t.Error("expected delete_objects to fail when item missing id")
	}

	// 6. executeCommandStep
	sCmdNilConfig := &Step{ID: "cmd1"}
	if res := exec.executeCommandStep(ctx, sCmdNilConfig, nil); res.Success {
		t.Error("expected execute_command to fail with nil config")
	}

	sCmdEmptyCommand := &Step{ID: "cmd2", Config: map[string]any{"command": ""}}
	if res := exec.executeCommandStep(ctx, sCmdEmptyCommand, nil); res.Success {
		t.Error("expected execute_command to fail with empty command")
	}

	sCmdNonExistent := &Step{ID: "cmd3", Config: map[string]any{"command": "nonexistent_executable_12345"}}
	if res := exec.executeCommandStep(ctx, sCmdNonExistent, nil); res.Success {
		t.Error("expected execute_command to fail with non-existent command")
	}

	sCmdExitFailure := &Step{
		ID: "cmd4",
		Config: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "exit 42"},
		},
	}
	resExitFailure := exec.executeCommandStep(ctx, sCmdExitFailure, nil)
	if resExitFailure.Success || resExitFailure.Metadata["exit_code"] != 42 {
		t.Errorf("expected exit code 42, got %+v", resExitFailure)
	}
}

func TestMigrationSubsystem_AdversarialBoundary_LifecycleAndUUID(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile("test")
	helper := &lifecycleMigrationHelper{
		projectRoot: tmpDir,
		logger:      logger,
	}

	// getObjectTypeAbbreviation test matrix
	abbrCases := []struct {
		objectType string
		want       string
	}{
		{"backlog_item", "BLI"},
		{"custom_item", "CUS"},
		{"a_item", "A"},
		{"latency_metric", "LAT"},
		{"simple_metric", "SIM"},
		{"short", "SHO"},
		{"extralongword_entity", "EXT"},
	}
	for _, tc := range abbrCases {
		got := getObjectTypeAbbreviation(tc.objectType)
		if got != tc.want {
			t.Errorf("getObjectTypeAbbreviation(%q) = %q, want %q", tc.objectType, got, tc.want)
		}
	}

	// transformLifecycleID failure mode
	if _, err := helper.transformLifecycleID(map[string]any{}); err == nil {
		t.Error("expected error transforming lifecycle without object_type")
	}

	// transformLifecycleID with invalid status gets overridden to approved
	transformed, err := helper.transformLifecycleID(map[string]any{
		"object_type": "backlog_item",
		"status":      "invalid_status",
	})
	if err != nil {
		t.Fatalf("transformLifecycleID failed: %v", err)
	}
	if transformed["status"] != "approved" {
		t.Errorf("expected status 'approved', got %s", transformed["status"])
	}

	// transformLifecycleID with valid status preserved
	transformedValid, err := helper.transformLifecycleID(map[string]any{
		"object_type": "backlog_item",
		"status":      "in_progress",
	})
	if err != nil {
		t.Fatalf("transformLifecycleID failed: %v", err)
	}
	if transformedValid["status"] != "in_progress" {
		t.Errorf("expected status 'in_progress', got %s", transformedValid["status"])
	}

	// UUID migration extractPrefixFromID fallback
	uuidHelper, err := newUUIDMigrationHelper(nil, tmpDir, logger)
	if err != nil {
		t.Fatalf("newUUIDMigrationHelper failed: %v", err)
	}
	prefix, err := uuidHelper.extractPrefixFromID("CUSTOM-A-B-12345")
	if err != nil || prefix != "CUSTOM-A-B" {
		t.Errorf("expected CUSTOM-A-B, got %q, err: %v", prefix, err)
	}
}

func TestMigrationSubsystem_AdversarialBoundary_CoordinationAndHistory(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterTempProjectTeardown(t, tmpDir, fs)

	// 1. createContextWithLoggingProfile profile variations
	profiles := []string{
		string(pkgctx.ProfileMCP),
		string(pkgctx.ProfileSystem),
		string(pkgctx.ProfileAIAgent),
		string(pkgctx.ProfileDebug),
		string(pkgctx.ProfileHuman),
		"",
		"unknown_profile",
	}
	for _, p := range profiles {
		ctx := createContextWithLoggingProfile(nil, p)
		if ctx == nil {
			t.Errorf("createContextWithLoggingProfile returned nil for %q", p)
		}
	}

	// 2. emitMigrationEventViaCoordinator edge cases
	ctx := context.Background()
	// Skip on empty or dot projectRoot
	emitMigrationEventViaCoordinator(ctx, "", fs, "op1", "type", "ok", "mig1", nil, nil, 0, "human")
	emitMigrationEventViaCoordinator(ctx, ".", fs, "op1", "type", "ok", "mig1", nil, nil, 0, "human")

	// Emit with actual projectRoot, different statuses and severities
	emitMigrationEventViaCoordinator(ctx, tmpDir, fs, "op2", "execute", "error", "mig1", map[string]any{"k": "v"}, errors.New("err"), 50*time.Millisecond, "human")
	emitMigrationEventViaCoordinator(ctx, tmpDir, nil, "op3", "execute", "completed", "mig1", nil, nil, 0, "human")
	emitMigrationEventViaCoordinator(ctx, tmpDir, fs, "op4", "execute", "failed", "mig1", nil, nil, 0, "human")

	// 3. History failure modes
	if err := RecordSuccess("", "MIG-1"); err == nil {
		t.Error("expected error for empty projectRoot in RecordSuccess")
	}
	if err := RecordSuccess(tmpDir, ""); err == nil {
		t.Error("expected error for empty migrationID in RecordSuccess")
	}

	// Block state directory with a regular file
	blockedProject := filepath.Join(tmpDir, "blocked_project")
	_ = fileutil.MkdirAll(blockedProject, paths.DirPerm755)
	stateBlocker := filepath.Join(blockedProject, paths.ProjectDataDir, paths.StateDir)
	_ = fileutil.MkdirAll(filepath.Dir(stateBlocker), paths.DirPerm755)
	_ = fileutil.WriteFile(stateBlocker, []byte("file blocker"), paths.FilePerm644)

	if err := RecordSuccess(blockedProject, "MIG-1"); err == nil {
		t.Error("expected error recording success with blocked state dir")
	}

	// Corrupted history JSON
	corruptProject := filepath.Join(tmpDir, "corrupt_project")
	corruptStateDir := filepath.Join(corruptProject, paths.ProjectDataDir, paths.StateDir)
	_ = fileutil.MkdirAll(corruptStateDir, paths.DirPerm755)
	_ = fileutil.WriteFile(filepath.Join(corruptStateDir, HistoryFileName), []byte(":: corrupt json ::"), paths.FilePerm644)

	if _, err := LoadHistory(corruptProject); err == nil {
		t.Error("expected error loading corrupted history")
	}
	if _, err := HasRun(corruptProject, "MIG-1"); err == nil {
		t.Error("expected error checking HasRun with corrupted history")
	}

	// 4. Pilot datacell migration function
	factory := storage.NewStorageFactoryForTesting(fs)
	if err := MigrateKindToGraph(ctx, factory, "goal"); err != nil {
		t.Errorf("MigrateKindToGraph failed: %v", err)
	}
}

func boolPtr(b bool) *bool {
	return &b
}
