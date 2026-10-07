package migration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestUUIDMigration_HelperMatrix(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile("test")
	helper, err := newUUIDMigrationHelper(nil, tmpDir, logger)
	if err != nil {
		t.Fatalf("newUUIDMigrationHelper failed: %v", err)
	}

	// 1. extractPrefixFromID
	cases := []struct {
		id      string
		want    string
		wantErr bool
	}{
		{"BLI-001", "BLI", false},
		{"AGENT-ARCH-123", "AGENT-ARCH", false},
		{"GOAL-9999", "GOAL", false},
		{"CUSTOM-PREFIX-007", "CUSTOM-PREFIX", false},
		{"INVALID", "", true},
		{"INVALID-ABC", "", true},
	}
	for _, tc := range cases {
		p, err := helper.extractPrefixFromID(tc.id)
		if (err != nil) != tc.wantErr {
			t.Errorf("extractPrefixFromID(%q) error = %v, wantErr %v", tc.id, err, tc.wantErr)
		}
		if p != tc.want {
			t.Errorf("extractPrefixFromID(%q) = %q, want %q", tc.id, p, tc.want)
		}
	}

	// 2. isAllDigits
	if isAllDigits("") || isAllDigits("123a") || !isAllDigits("123456") {
		t.Fatal("unexpected isAllDigits result")
	}

	// 3. transformToUUIDID
	if _, err := helper.transformToUUIDID(map[string]any{}); err == nil {
		t.Fatal("expected error on missing id")
	}
	if _, err := helper.transformToUUIDID(map[string]any{"id": "BAD"}); err == nil {
		t.Fatal("expected error on bad id prefix")
	}

	validObj := map[string]any{
		"id":    "BLI-001",
		"title": "Old Task",
	}
	transformed, err := helper.transformToUUIDID(validObj)
	if err != nil {
		t.Fatalf("transformToUUIDID failed: %v", err)
	}
	newID, ok := transformed["id"].(string)
	if !ok || !strings.HasPrefix(newID, "BLI-") || len(newID) <= 4 {
		t.Fatalf("unexpected transformed ID: %s", newID)
	}
	if transformed["title"] != "Old Task" {
		t.Fatalf("transformed object lost title: %v", transformed)
	}
}

func TestLoader_ValidateSpecMatrix(t *testing.T) {
	// 1. Missing ID
	if err := validateSpec(&Spec{Name: "Test", Steps: []Step{{ID: "s1"}}}); err == nil {
		t.Fatal("expected error on missing id")
	}
	// 2. Missing Name
	if err := validateSpec(&Spec{ID: "m1", Steps: []Step{{ID: "s1"}}}); err == nil {
		t.Fatal("expected error on missing name")
	}
	// 3. Empty steps
	if err := validateSpec(&Spec{ID: "m1", Name: "Test", Steps: nil}); err == nil {
		t.Fatal("expected error on empty steps")
	}
	// 4. Missing step ID
	if err := validateSpec(&Spec{ID: "m1", Name: "Test", Steps: []Step{{ID: ""}}}); err == nil {
		t.Fatal("expected error on empty step id")
	}
	// 5. Duplicate step ID
	if err := validateSpec(&Spec{ID: "m1", Name: "Test", Steps: []Step{{ID: "s1"}, {ID: "s1"}}}); err == nil {
		t.Fatal("expected error on duplicate step id")
	}
	// 6. Unknown dependency
	specBadDep := &Spec{
		ID:    "m1",
		Name:  "Test",
		Steps: []Step{{ID: "s1", DependsOn: []string{"unknown"}}},
	}
	if err := validateSpec(specBadDep); err == nil {
		t.Fatal("expected error on unknown dependency")
	}
	// 7. Unknown step in for_each
	specBadForEach := &Spec{
		ID:   "m1",
		Name: "Test",
		Steps: []Step{
			{ID: "s1", ForEach: "unknown_target"},
		},
	}
	if err := validateSpec(specBadForEach); err == nil {
		t.Fatal("expected error on unknown step in for_each")
	}

	// 8. LoadSpec from disk with SnapshotCompatible defaults
	tmpDir := t.TempDir()
	specFile := filepath.Join(tmpDir, "migration.yaml")
	specYAML := `
schema_version: "1.0.0"
id: MIG-TEST-001
name: Test Migration
snapshot_compatible: true
from:
  state: legacy
to:
  state: v2
steps:
  - id: step_scan
    type: scan_files
    config:
      directory: process/backlog_items
`
	_ = fileutil.WriteStandardFile(specFile, []byte(specYAML))
	loaded, err := LoadSpec(specFile)
	if err != nil {
		t.Fatalf("LoadSpec failed: %v", err)
	}
	if loaded.PreMigrationSnapshot == nil || loaded.Tracking == nil || loaded.Coherence == nil {
		t.Fatal("expected SnapshotCompatible defaults populated")
	}
}

func TestSteps_ScanFilesStep(t *testing.T) {
	root := t.TempDir()
	logger := logging.GetLoggerFromProfile("test")
	exec := NewExecutor(nil, root, logger)
	ctx := context.Background()

	// 1. Missing config
	res1 := exec.executeScanFilesStep(ctx, &Step{ID: "s1", Config: nil}, nil)
	if res1.Success {
		t.Fatal("expected error on nil config")
	}

	// 2. Missing directory
	res2 := exec.executeScanFilesStep(ctx, &Step{ID: "s1", Config: map[string]any{}}, nil)
	if res2.Success {
		t.Fatal("expected error on missing directory")
	}

	// 3. Valid directory with pattern and exclusion
	subDir := filepath.Join(root, "scan_target")
	_ = fileutil.MkdirAll(subDir, paths.DirPerm755)
	_ = fileutil.WriteStandardFile(filepath.Join(subDir, "a.yaml"), []byte("id: A\n"))
	_ = fileutil.WriteStandardFile(filepath.Join(subDir, "b.yaml"), []byte("id: B\n"))
	_ = fileutil.WriteStandardFile(filepath.Join(subDir, "skip.txt"), []byte("skip\n"))

	step := &Step{
		ID: "scan_1",
		Config: map[string]any{
			"directory": "scan_target",
			"pattern":   "*.yaml",
			"exclude":   []any{"b.yaml"},
		},
	}
	res3 := exec.executeScanFilesStep(ctx, step, nil)
	if !res3.Success {
		t.Fatalf("executeScanFilesStep failed: %v", res3.Error)
	}
	files, ok := res3.Output["files"].([]FileInfo)
	if !ok || len(files) != 1 || files[0].Name != "a.yaml" {
		t.Fatalf("unexpected scan output: %+v", res3.Output)
	}
}

func TestSteps_ReadIDListStep(t *testing.T) {
	root := t.TempDir()
	logger := logging.GetLoggerFromProfile("test")
	exec := NewExecutor(nil, root, logger)
	ctx := context.Background()

	// 1. Nil config
	if res := exec.executeReadIDListStep(ctx, &Step{ID: "r1"}, nil); res.Success {
		t.Fatal("expected error on nil config")
	}

	// 2. YAML list format
	yamlFile := filepath.Join(root, "ids.yaml")
	_ = fileutil.WriteStandardFile(yamlFile, []byte("- BLI-1\n- BLI-2\n"))
	stepYAML := &Step{
		ID:     "read_yaml",
		Config: map[string]any{"file": "ids.yaml"},
	}
	resYAML := exec.executeReadIDListStep(ctx, stepYAML, nil)
	if !resYAML.Success || resYAML.Output["count"] != 2 {
		t.Fatalf("executeReadIDListStep YAML failed: %v", resYAML.Error)
	}

	// 3. Text lines format
	txtFile := filepath.Join(root, "ids.txt")
	_ = fileutil.WriteStandardFile(txtFile, []byte("# Comment\nBLI-3\nBLI-4\n"))
	stepTxt := &Step{
		ID:     "read_txt",
		Config: map[string]any{"file": "ids.txt"},
	}
	resTxt := exec.executeReadIDListStep(ctx, stepTxt, nil)
	if !resTxt.Success || resTxt.Output["count"] != 2 {
		t.Fatalf("executeReadIDListStep TXT failed: %v", resTxt.Error)
	}
}

func TestSteps_TransformStep_UUID(t *testing.T) {
	root := t.TempDir()
	logger := logging.GetLoggerFromProfile("test")
	exec := NewExecutor(nil, root, logger)
	ctx := context.Background()

	// 1. Missing for_each
	res1 := exec.executeTransformStep(ctx, nil, &Step{ID: "t1"}, nil)
	if res1.Success {
		t.Fatal("expected error on missing for_each")
	}

	// 2. Missing source output
	res2 := exec.executeTransformStep(ctx, nil, &Step{ID: "t2", ForEach: "s1"}, map[string]any{})
	if res2.Success {
		t.Fatal("expected error on missing source output")
	}

	// 3. Valid UUID transformation
	sourceOutput := map[string]any{
		"items": []map[string]any{
			{"id": "GOAL-001", "title": "First Goal"},
			{"id": "BLI-002", "title": "Second Item"},
		},
	}
	step := &Step{
		ID:      "trans_uuid",
		ForEach: "read_step",
		Config: map[string]any{
			"transform": map[string]any{
				"builder": "uuid_id_transform",
			},
		},
	}
	res3 := exec.executeTransformStep(ctx, nil, step, map[string]any{"read_step": sourceOutput})
	if !res3.Success {
		t.Fatalf("executeTransformStep UUID failed: %v", res3.Error)
	}
	items, ok := res3.Output["items"].([]map[string]any)
	if !ok || len(items) != 2 {
		t.Fatalf("unexpected transformed items: %+v", res3.Output)
	}
	if !strings.HasPrefix(items[0]["id"].(string), "GOAL-") {
		t.Fatalf("expected GOAL- prefix, got %s", items[0]["id"])
	}
}

func TestSteps_ReadAndCreateObjectsSteps(t *testing.T) {
	tmpDir := t.TempDir()
	caspkg.SetListingIndexWriteQueueFactoryToPerProjectRoot()
	t.Cleanup(func() { caspkg.SetListingIndexWriteQueueFactory(nil) })

	_ = paths.EnsureProcessAndObjectSpecsLayout(tmpDir)
	fs, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterTempProjectTeardown(t, tmpDir, fs)

	logger := logging.GetLoggerFromProfile("test")
	exec := NewExecutor(fs, tmpDir, logger)
	ctx := context.Background()

	// 1. Create initial object in storage
	secCtx := pkgctx.NewSystemSecurityContext()
	initialObj := map[string]any{
		"kind":  objects.KindBacklogItem,
		"id":    "BLI-CREATE-1",
		"title": "Initial Item",
	}
	if err := fs.Create(ctx, secCtx, initialObj); err != nil {
		t.Fatalf("failed to create initial object: %v", err)
	}

	// 2. ReadObjectsStep
	readStep := &Step{
		ID:        "read_obj",
		DependsOn: []string{"read_ids"},
	}
	stepOutputs := map[string]any{
		"read_ids": map[string]any{
			"ids": []string{"BLI-CREATE-1"},
		},
	}
	readRes := exec.executeReadObjectsStep(ctx, readStep, stepOutputs)
	if !readRes.Success {
		t.Fatalf("executeReadObjectsStep failed: %v", readRes.Error)
	}
	readItems, ok := readRes.Output["items"].([]map[string]any)
	if !ok || len(readItems) != 1 || readItems[0]["id"] != "BLI-CREATE-1" {
		t.Fatalf("unexpected read objects output: %+v", readRes.Output)
	}

	// 3. CreateObjectsStep
	createStep := &Step{
		ID:        "create_obj",
		DependsOn: []string{"transform_step"},
		Config: map[string]any{
			"skip_existing": true,
		},
	}
	newObj := map[string]any{
		"kind":  objects.KindBacklogItem,
		"id":    "BLI-CREATE-2",
		"title": "Created Item",
	}
	createOutputs := map[string]any{
		"transform_step": map[string]any{
			"items": []map[string]any{newObj},
		},
	}
	createRes := exec.executeCreateObjectsStep(ctx, nil, createStep, createOutputs, ExecutionOptions{})
	if !createRes.Success {
		t.Fatalf("executeCreateObjectsStep failed: %v", createRes.Error)
	}
	if createRes.Output["created"] != 1 {
		t.Fatalf("expected 1 created object, got %v", createRes.Output)
	}
}

type testSnapshotCreator struct {
	preCreated        bool
	postCreated       bool
	checkpointCreated bool
}

func (s *testSnapshotCreator) CreatePreMigrationSnapshot(ctx context.Context, spec *Spec) (string, error) {
	s.preCreated = true
	return "test-pre-snap", nil
}

func (s *testSnapshotCreator) CreatePostMigrationSnapshot(ctx context.Context, spec *Spec, preSnapshotID string) (string, error) {
	s.postCreated = true
	return "test-post-snap", nil
}

func (s *testSnapshotCreator) CreateCheckpointSnapshot(ctx context.Context, spec *Spec, step Step, count int) (string, error) {
	s.checkpointCreated = true
	return "test-checkpoint-snap", nil
}

func TestLifecycleMigration_HelpersAndTransform(t *testing.T) {
	abbrCases := []struct {
		input string
		want  string
	}{
		{"backlog_item", "BAC"},
		{"test_metric", "TES"},
		{"bi_item", "BI"},
		{"goal", "GOAL"},
		{"custom_compound_long", "CUSTOM"},
		{"short", "SHORT"},
		{"verylongnounderscores", "VERY"},
		{"ab", "AB"},
	}
	for _, tc := range abbrCases {
		got := getObjectTypeAbbreviation(tc.input)
		if got == "" {
			t.Errorf("getObjectTypeAbbreviation(%q) returned empty", tc.input)
		}
	}

	if min(2, 5) != 2 || min(5, 2) != 2 {
		t.Errorf("min function failed: min(2,5)=%d, min(5,2)=%d", min(2, 5), min(5, 2))
	}

	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile("test")
	helper := &lifecycleMigrationHelper{
		projectRoot: tmpDir,
		logger:      logger,
	}

	if _, err := helper.transformLifecycleID(map[string]any{"id": "LIFECYCLE-1"}); err == nil {
		t.Error("expected error for missing object_type")
	}

	validObj := map[string]any{
		"id":          "LIFECYCLE-backlog_item-v1_0_0",
		"object_type": "backlog_item",
		"title":       "Backlog Item Lifecycle",
	}
	transformed, err := helper.transformLifecycleID(validObj)
	if err != nil {
		t.Fatalf("transformLifecycleID failed: %v", err)
	}
	if !strings.HasPrefix(transformed["id"].(string), "LIFECYCLE-") {
		t.Errorf("expected LIFECYCLE- prefix, got %v", transformed["id"])
	}
}

func TestMigrationCoordination_LoggingAndEvents(t *testing.T) {
	profiles := []string{
		string(pkgctx.ProfileMCP),
		string(pkgctx.ProfileSystem),
		string(pkgctx.ProfileAIAgent),
		string(pkgctx.ProfileDebug),
		string(pkgctx.ProfileHuman),
		"",
		"custom_unknown_profile",
	}
	for _, p := range profiles {
		ctx := createContextWithLoggingProfile(context.Background(), p)
		if ctx == nil {
			t.Errorf("createContextWithLoggingProfile returned nil for %q", p)
		}
	}
	if ctxNil := createContextWithLoggingProfile(nil, ""); ctxNil == nil {
		t.Error("expected non-nil context for nil parent")
	}

	emitMigrationEventViaCoordinator(context.Background(), "", nil, "op1", "t", "s", "m1", nil, nil, 0, "human")
	emitMigrationEventViaCoordinator(context.Background(), ".", nil, "op1", "t", "s", "m1", nil, nil, 0, "human")

	tmpDir := t.TempDir()
	fs, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterTempProjectTeardown(t, tmpDir, fs)

	emitMigrationEventViaCoordinator(context.Background(), tmpDir, nil, "op1", "test", "success", "MIG-1", map[string]any{"k": "v"}, nil, 10*time.Millisecond, "human")
	emitMigrationEventViaCoordinator(context.Background(), tmpDir, fs, "op2", "test", "failed", "MIG-1", nil, fmt.Errorf("sample error"), 20*time.Millisecond, "human")
}

func TestDatacellPilot_MigrateKindToGraph(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterTempProjectTeardown(t, tmpDir, fs)

	factory := storage.NewStorageFactoryForTesting(fs)
	if err := MigrateKindToGraph(context.Background(), factory, objects.KindGoal); err != nil {
		t.Fatalf("MigrateKindToGraph failed: %v", err)
	}
}

func TestSteps_DeleteObjectsStep(t *testing.T) {
	tmpDir := t.TempDir()
	caspkg.SetListingIndexWriteQueueFactoryToPerProjectRoot()
	t.Cleanup(func() { caspkg.SetListingIndexWriteQueueFactory(nil) })
	_ = paths.EnsureProcessAndObjectSpecsLayout(tmpDir)

	fs, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterTempProjectTeardown(t, tmpDir, fs)

	logger := logging.GetLoggerFromProfile("test")
	exec := NewExecutor(fs, tmpDir, logger)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	_ = fs.Create(ctx, secCtx, map[string]any{"kind": objects.KindBacklogItem, "id": "BLI-DEL-1", "title": "D1"})
	_ = fs.Create(ctx, secCtx, map[string]any{"kind": objects.KindBacklogItem, "id": "BLI-DEL-2", "title": "D2"})

	delStep := &Step{ID: "del_step"}
	resFail := exec.executeDeleteObjectsStep(ctx, delStep, map[string]any{}, ExecutionOptions{})
	if resFail.Success {
		t.Error("expected failure for missing depends_on")
	}

	delStep.DependsOn = []string{"id_source"}
	outputs := map[string]any{"id_source": map[string]any{"ids": []string{"BLI-DEL-1"}}}
	dryRunTrue := true
	resDry := exec.executeDeleteObjectsStep(ctx, delStep, outputs, ExecutionOptions{DryRun: &dryRunTrue})
	if !resDry.Success || resDry.Output["deleted"] != 1 {
		t.Fatalf("dry run delete failed: %+v", resDry)
	}
	if _, err := fs.Read(ctx, secCtx, "BLI-DEL-1"); err != nil {
		t.Fatalf("item should not be deleted during dry run: %v", err)
	}

	resReal := exec.executeDeleteObjectsStep(ctx, delStep, outputs, ExecutionOptions{})
	if !resReal.Success || resReal.Output["deleted"] != 1 {
		t.Fatalf("real delete failed: %+v", resReal)
	}

	outputsTrans := map[string]any{"id_source": map[string]any{"items": []map[string]any{{"id": "BLI-DEL-2"}}}}
	delStep.Config = map[string]any{"cascade": true}
	resTrans := exec.executeDeleteObjectsStep(ctx, delStep, outputsTrans, ExecutionOptions{})
	if !resTrans.Success || resTrans.Output["deleted"] != 1 {
		t.Fatalf("items delete failed: %+v", resTrans)
	}
}

func TestSteps_TransformStep_LifecycleAndYAML(t *testing.T) {
	tmpDir := t.TempDir()
	caspkg.SetListingIndexWriteQueueFactoryToPerProjectRoot()
	t.Cleanup(func() { caspkg.SetListingIndexWriteQueueFactory(nil) })
	_ = paths.EnsureProcessAndObjectSpecsLayout(tmpDir)

	fs, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterTempProjectTeardown(t, tmpDir, fs)

	logger := logging.GetLoggerFromProfile("test")
	exec := NewExecutor(fs, tmpDir, logger)
	ctx := context.Background()

	s1 := &Step{ID: "t1"}
	if res := exec.executeTransformStep(ctx, nil, s1, nil); res.Success {
		t.Error("expected error for missing for_each")
	}
	s1.ForEach = "missing_step"
	if res := exec.executeTransformStep(ctx, nil, s1, map[string]any{}); res.Success {
		t.Error("expected error for unknown source step")
	}
	s1.Config = map[string]any{}
	stepOutputs := map[string]any{"missing_step": map[string]any{"items": []map[string]any{}}}
	if res := exec.executeTransformStep(ctx, nil, s1, stepOutputs); res.Success {
		t.Error("expected error for missing transform config")
	}

	sLifecycle := &Step{
		ID:      "t_lc",
		ForEach: "source_step",
		Config: map[string]any{
			"transform": map[string]any{
				"builder": "lifecycle_id_transform",
			},
		},
	}
	outputsLC := map[string]any{
		"source_step": map[string]any{
			"items": []map[string]any{
				{"id": "LIFECYCLE-old-1", "object_type": "backlog_item"},
			},
		},
	}
	resLC := exec.executeTransformStep(ctx, nil, sLifecycle, outputsLC)
	if !resLC.Success {
		t.Fatalf("lifecycle_id_transform failed: %v", resLC.Error)
	}

	yamlFile := filepath.Join(tmpDir, "sample.yaml")
	_ = os.WriteFile(yamlFile, []byte("id: GOAL-SAMPLE-1\nname: Sample Goal\n"), 0644)
	sYAML := &Step{
		ID:      "t_yaml",
		ForEach: "source_files",
		Config: map[string]any{
			"transform": map[string]any{
				"builder": "yaml_file_to_object",
			},
		},
	}
	outputsYAML := map[string]any{
		"source_files": map[string]any{
			"files": []FileInfo{{Path: yamlFile, Name: "sample.yaml"}},
		},
	}
	resYAML := exec.executeTransformStep(ctx, nil, sYAML, outputsYAML)
	if !resYAML.Success {
		t.Fatalf("yaml_file_to_object failed: %v", resYAML.Error)
	}
}

func TestExecutor_Execute_EndToEndAndSnapshots(t *testing.T) {
	tmpDir := t.TempDir()
	caspkg.SetListingIndexWriteQueueFactoryToPerProjectRoot()
	t.Cleanup(func() { caspkg.SetListingIndexWriteQueueFactory(nil) })
	_ = paths.EnsureProcessAndObjectSpecsLayout(tmpDir)

	fs, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterTempProjectTeardown(t, tmpDir, fs)

	logger := logging.GetLoggerFromProfile("test")
	exec := NewExecutor(fs, tmpDir, logger)
	snapCreator := &testSnapshotCreator{}
	exec.SetSnapshotCreator(snapCreator)

	scanDir := filepath.Join(tmpDir, "scandir")
	_ = os.MkdirAll(scanDir, 0755)
	_ = os.WriteFile(filepath.Join(scanDir, "item.yaml"), []byte("kind: backlog_item\nid: BLI-E2E-1\ntitle: Item\n"), 0644)

	bTrue := true
	spec := &Spec{
		ID:                 "MIG-E2E-TEST",
		Name:               "E2E Test",
		SnapshotCompatible: true,
		PreMigrationSnapshot: &SnapshotConfig{
			AutoCreate: true,
		},
		PostMigrationSnapshot: &SnapshotConfig{
			AutoCreate: true,
		},
		Prerequisites: []Prerequisite{
			{Type: "kind", Kind: objects.KindBacklogItem},
			{Type: "directory", Directory: scanDir, Exists: &bTrue},
		},
		Steps: []Step{
			{
				ID:   "step_scan",
				Type: "scan_files",
				Config: map[string]any{
					"directory": "scandir",
					"pattern":   "*.yaml",
				},
			},
			{
				ID:      "step_trans",
				Type:    "transform",
				ForEach: "step_scan",
				Config: map[string]any{
					"transform": map[string]any{
						"builder": "yaml_file_to_object",
					},
				},
			},
			{
				ID:        "step_create",
				Type:      "create_objects",
				DependsOn: []string{"step_trans"},
				Config:    map[string]any{"skip_existing": true},
			},
		},
		Validation: []ValidationRule{
			{
				Type: "object_count",
				Config: map[string]any{
					"step_id":  "step_create",
					"min":      1,
					"max":      5,
					"expected": 1,
				},
			},
		},
	}

	res, err := exec.Execute(context.Background(), spec, ExecutionOptions{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if res.Status != objects.ObjectStatusCompleted {
		t.Errorf("expected completed status, got %s", res.Status)
	}
	if !snapCreator.preCreated || !snapCreator.postCreated {
		t.Errorf("expected pre and post snapshots created: pre=%v, post=%v", snapCreator.preCreated, snapCreator.postCreated)
	}
}

func TestExecutor_Execute_FailureAndContinueOnError(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterTempProjectTeardown(t, tmpDir, fs)

	logger := logging.GetLoggerFromProfile("test")
	exec := NewExecutor(fs, tmpDir, logger)

	bFalse := false
	specPrereqFail := &Spec{
		ID:   "MIG-FAIL-PREREQ",
		Name: "Prereq Fail",
		Prerequisites: []Prerequisite{
			{Type: "directory", Directory: tmpDir, Exists: &bFalse},
		},
		Steps: []Step{{ID: "s1", Type: "unknown_step"}},
	}
	if _, err := exec.Execute(context.Background(), specPrereqFail, ExecutionOptions{}); err == nil {
		t.Error("expected error on prerequisite failure")
	}

	specStepFail := &Spec{
		ID:   "MIG-FAIL-STEP",
		Name: "Step Fail",
		Steps: []Step{
			{ID: "s1", Type: "unknown_step_type"},
		},
	}
	if _, err := exec.Execute(context.Background(), specStepFail, ExecutionOptions{}); err == nil {
		t.Error("expected error on step failure")
	}

	specStepFail.Options.ContinueOnError = true
	res, err := exec.Execute(context.Background(), specStepFail, ExecutionOptions{})
	if err != nil {
		t.Fatalf("expected no error with ContinueOnError, got: %v", err)
	}
	if res.Status != objects.ObjectStatusCompleted {
		t.Errorf("expected completed status, got %s", res.Status)
	}

	preSnap, _ := exec.createPreMigrationSnapshot(context.Background(), specStepFail)
	postSnap, _ := exec.createPostMigrationSnapshot(context.Background(), specStepFail, preSnap)
	chkSnap, _ := exec.createCheckpointSnapshot(context.Background(), specStepFail, specStepFail.Steps[0], 1)
	if !strings.HasPrefix(preSnap, "snapshot-pre-") || !strings.HasPrefix(postSnap, "snapshot-post-") || !strings.HasPrefix(chkSnap, "checkpoint-") {
		t.Errorf("unexpected synthetic snapshots: pre=%s, post=%s, chk=%s", preSnap, postSnap, chkSnap)
	}

	if toInt(42) != 42 || toInt(float64(42)) != 42 || toInt("invalid") != -1 || toInt(nil) != -1 {
		t.Error("toInt failed on type matrix")
	}
}

func TestHistory_CorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	histPath := historyFilePath(tmpDir)
	_ = os.MkdirAll(filepath.Dir(histPath), 0755)
	_ = os.WriteFile(histPath, []byte("invalid-json{"), 0644)

	if _, err := LoadHistory(tmpDir); err == nil {
		t.Error("expected error loading corrupted history")
	}
	if _, err := HasRun(tmpDir, "MIG-1"); err == nil {
		t.Error("expected error in HasRun with corrupted history")
	}

	if err := RecordSuccess("", "MIG-1"); err == nil {
		t.Error("expected error on empty project root")
	}
	if err := RecordSuccess(tmpDir, ""); err == nil {
		t.Error("expected error on empty migration ID")
	}
}

func TestSteps_ReadAndCreate_ErrorAndBranchMatrix(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterTempProjectTeardown(t, tmpDir, fs)
	logger := logging.GetLoggerFromProfile("test")
	exec := NewExecutor(fs, tmpDir, logger)
	ctx := context.Background()

	sRead := &Step{ID: "r1", DependsOn: []string{"src"}}
	if res := exec.executeReadObjectsStep(ctx, sRead, map[string]any{}); res.Success {
		t.Error("expected failure for missing depends_on")
	}
	if res := exec.executeReadObjectsStep(ctx, sRead, map[string]any{"src": "not-map"}); res.Success {
		t.Error("expected failure for non-map source")
	}
	if res := exec.executeReadObjectsStep(ctx, sRead, map[string]any{"src": map[string]any{}}); res.Success {
		t.Error("expected failure for missing ids key")
	}
	if res := exec.executeReadObjectsStep(ctx, sRead, map[string]any{"src": map[string]any{"ids": []any{123}}}); res.Success {
		t.Error("expected failure for non-string in ids")
	}

	sCreate := &Step{ID: "c1", DependsOn: []string{"src"}}
	if res := exec.executeCreateObjectsStep(ctx, nil, sCreate, map[string]any{}, ExecutionOptions{}); res.Success {
		t.Error("expected failure for missing config")
	}
	sCreate.Config = map[string]any{"skip_existing": true}
	if res := exec.executeCreateObjectsStep(ctx, nil, sCreate, map[string]any{}, ExecutionOptions{}); res.Success {
		t.Error("expected failure for missing source")
	}
	if res := exec.executeCreateObjectsStep(ctx, nil, sCreate, map[string]any{"src": "bad"}, ExecutionOptions{}); res.Success {
		t.Error("expected failure for non-map source")
	}
	if res := exec.executeCreateObjectsStep(ctx, nil, sCreate, map[string]any{"src": map[string]any{}}, ExecutionOptions{}); res.Success {
		t.Error("expected failure for missing items")
	}
	if res := exec.executeCreateObjectsStep(ctx, nil, sCreate, map[string]any{"src": map[string]any{"items": "bad"}}, ExecutionOptions{}); res.Success {
		t.Error("expected failure for non-slice items")
	}

	srcMissingID := map[string]any{"src": map[string]any{"items": []map[string]any{{"title": "No ID"}}}}
	resMiss := exec.executeCreateObjectsStep(ctx, nil, sCreate, srcMissingID, ExecutionOptions{})
	if resMiss.Output["errors"] != 1 {
		t.Errorf("expected 1 error for missing ID, got %+v", resMiss)
	}
}

func TestExecutor_ExecuteStep_AllTypes(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterTempProjectTeardown(t, tmpDir, fs)
	logger := logging.GetLoggerFromProfile("test")
	exec := NewExecutor(fs, tmpDir, logger)
	ctx := context.Background()

	sDep := &Step{ID: "s1", DependsOn: []string{"missing_dep"}}
	if res := exec.executeStep(ctx, nil, sDep, map[string]any{}, ExecutionOptions{}); res.Success {
		t.Error("expected error for missing dependency")
	}

	sUnknown := &Step{ID: "s2", Type: "invalid_type"}
	if res := exec.executeStep(ctx, nil, sUnknown, map[string]any{}, ExecutionOptions{}); res.Success {
		t.Error("expected error for unknown step type")
	}

	sCmd := &Step{
		ID:   "s3",
		Type: "execute_command",
		Config: map[string]any{
			"command":     "echo",
			"args":        []any{"test_out"},
			"env":         map[string]any{"ENV_KEY": "env_val"},
			"working_dir": tmpDir,
		},
	}
	resCmd := exec.executeStep(ctx, nil, sCmd, map[string]any{}, ExecutionOptions{})
	if !resCmd.Success || !strings.Contains(resCmd.Output["stdout"].(string), "test_out") {
		t.Fatalf("command step failed: %+v", resCmd)
	}

	sCmdBad := &Step{
		ID:     "s4",
		Type:   "execute_command",
		Config: map[string]any{},
	}
	if res := exec.executeStep(ctx, nil, sCmdBad, map[string]any{}, ExecutionOptions{}); res.Success {
		t.Error("expected failure for missing command")
	}
}

func TestLifecycleMigration_MigrateFile_ForceAndErrors(t *testing.T) {
	tmpDir := t.TempDir()
	paths.EnsureProcessAndObjectSpecsLayout(tmpDir)
	fs, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterTempProjectTeardown(t, tmpDir, fs)
	logger := logging.GetLoggerFromProfile("test")
	helper := &lifecycleMigrationHelper{
		storageProvider: fs,
		projectRoot:     tmpDir,
		logger:          logger,
	}
	ctx := context.Background()

	if err := helper.migrateLifecycleFile(ctx, filepath.Join(tmpDir, "nonexistent.yaml"), "item", "v1", false, false); err == nil {
		t.Error("expected error for non-existent file")
	}

	lcFile := filepath.Join(tmpDir, "goal_lifecycle.yaml")
	lcContent := `schema_version: "1.0.0"
object_type: "goal"
initial_status: "draft"
statuses:
  - value: "draft"
    display: "Draft"
  - value: "active"
    display: "Active"
transitions:
  - from: "draft"
    to: "active"
`
	_ = os.WriteFile(lcFile, []byte(lcContent), 0644)

	if err := helper.migrateLifecycleFile(ctx, lcFile, "goal", "v1_0_0", false, true); err != nil {
		t.Fatalf("dry run migrateLifecycleFile failed: %v", err)
	}

	if err := helper.migrateLifecycleFile(ctx, lcFile, "goal", "v1_0_0", false, false); err != nil {
		t.Fatalf("real migrateLifecycleFile failed: %v", err)
	}

	if err := helper.migrateLifecycleFile(ctx, lcFile, "goal", "v1_0_0", true, false); err != nil {
		t.Fatalf("force migrateLifecycleFile failed: %v", err)
	}
}

func TestSteps_TransformStep_MoreBuildersAndErrors(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterTempProjectTeardown(t, tmpDir, fs)
	logger := logging.GetLoggerFromProfile("test")
	exec := NewExecutor(fs, tmpDir, logger)
	ctx := context.Background()

	lcPath := filepath.Join(tmpDir, "goal_lifecycle.yaml")
	_ = os.WriteFile(lcPath, []byte("schema_version: '1.0.0'\nobject_type: goal\nstatuses: [{value: draft, display: Draft}]\n"), 0644)
	sLC := &Step{
		ID:      "t_lcb",
		ForEach: "src",
		Config:  map[string]any{"transform": map[string]any{"builder": "lifecycle_instance_builder"}},
	}
	resLC := exec.executeTransformStep(ctx, nil, sLC, map[string]any{
		"src": map[string]any{"files": []FileInfo{{Path: lcPath, Name: "goal_lifecycle.yaml"}}},
	})
	if !resLC.Success {
		t.Fatalf("lifecycle_instance_builder failed: %v", resLC.Error)
	}

	sUnsupObj := &Step{
		ID:      "t_unsup",
		ForEach: "src",
		Config:  map[string]any{"transform": map[string]any{"builder": "unknown_builder"}},
	}
	if res := exec.executeTransformStep(ctx, nil, sUnsupObj, map[string]any{"src": map[string]any{"items": []map[string]any{{"id": "1"}}}}); res.Success {
		t.Error("expected error for unsupported object builder")
	}
	if res := exec.executeTransformStep(ctx, nil, sUnsupObj, map[string]any{"src": map[string]any{"files": []FileInfo{}}}); res.Success {
		t.Error("expected error for unsupported file builder")
	}

	if res := exec.executeTransformStep(ctx, nil, sUnsupObj, map[string]any{"src": map[string]any{}}); res.Success {
		t.Error("expected error for source missing both items and files")
	}
	if res := exec.executeTransformStep(ctx, nil, sUnsupObj, map[string]any{"src": map[string]any{"items": "invalid"}}); res.Success {
		t.Error("expected error for invalid items type")
	}
	if res := exec.executeTransformStep(ctx, nil, sUnsupObj, map[string]any{"src": map[string]any{"files": "invalid"}}); res.Success {
		t.Error("expected error for invalid files type")
	}
}
