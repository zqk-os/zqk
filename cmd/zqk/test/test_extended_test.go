package test

import (
	"bytes"
	"context"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func findModuleRootForTestPkg() (string, error) {
	wd, err := fileutil.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return wd, nil
}

func setupTestProject(t *testing.T) (string, func()) {
	t.Helper()
	testRoot := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), testRoot)
	repoRoot := zqkenv.ProjectRoot().Get()
	if repoRoot == "" {
		if root, err := findModuleRootForTestPkg(); err == nil {
			repoRoot = root
		}
	}
	if err := testenvroot.BootstrapRoot(testRoot, repoRoot); err != nil {
		t.Fatalf("BootstrapRoot: %v", err)
	}
	cleanup := func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, nil))
	}
	return testRoot, cleanup
}

func newTestCommandBuffer(cmd *cobra.Command) *bytes.Buffer {
	var buf bytes.Buffer
	prepareTestCmd(cmd, &buf)
	return &buf
}

func prepareTestCmd(cmd *cobra.Command, buf *bytes.Buffer) {
	buf.Reset()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))
}

func TestCheckContaminationCmd_Execution(t *testing.T) {
	testRoot, cleanup := setupTestProject(t)
	defer cleanup()

	// 1. Missing command error
	cmd := NewCheckContaminationCmd()
	buf := newTestCommandBuffer(cmd)
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "specify command to run") {
		t.Fatalf("expected specify command to run error, got: %v", err)
	}

	// 2. Successful contamination check with --command flag
	cmd = NewCheckContaminationCmd()
	prepareTestCmd(cmd, buf)
	cmd.SetArgs([]string{"--command", "echo test-run", "--plane", filepath.Join(testRoot, paths.ProcessDir)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected successful contamination check, got err: %v", err)
	}
	// 3. Command execution failure
	cmd = NewCheckContaminationCmd()
	prepareTestCmd(cmd, buf)
	cmd.SetArgs([]string{"--command", "sh -c 'exit 1'", "--plane", filepath.Join(testRoot, paths.ProcessDir)})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "command failed with exit code 1") {
		t.Fatalf("expected command failed error, got: %v", err)
	}

	// 4. Schema contamination detected
	cmd = NewCheckContaminationCmd()
	prepareTestCmd(cmd, buf)
	planeDir := filepath.Join(testRoot, "schema_plane_test")
	if err := fileutil.EnsureDir(planeDir); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	cmd.SetArgs([]string{"--command", "touch " + filepath.Join(planeDir, "polluted.yaml"), "--plane", planeDir})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "test contamination detected") {
		t.Fatalf("expected test contamination detected error, got: %v", err)
	}
	if !strings.Contains(buf.String(), "TEST CONTAMINATION") {
		t.Errorf("expected TEST CONTAMINATION in output, got: %s", buf.String())
	}

	// 5. Positional arguments command
	cmd = NewCheckContaminationCmd()
	prepareTestCmd(cmd, buf)
	cmd.SetArgs([]string{"--plane", filepath.Join(testRoot, paths.ProcessDir), "echo", "zero-contamination"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected success with positional command, got: %v", err)
	}
}

func TestIdentifyParallelCmd_Execution(t *testing.T) {
	_, cleanup := setupTestProject(t)
	defer cleanup()

	// 1. Basic scan with safe-only
	cmd := NewIdentifyParallelCmd()
	buf := newTestCommandBuffer(cmd)
	cmd.SetArgs([]string{"--path", ".", "--safe-only"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("identify-parallel safe-only failed: %v", err)
	}

	// 2. Scan with unsafe-only
	cmd = NewIdentifyParallelCmd()
	prepareTestCmd(cmd, buf)
	cmd.SetArgs([]string{"--path", ".", "--unsafe-only"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("identify-parallel unsafe-only failed: %v", err)
	}

	// 3. Scan with json format
	cmd = NewIdentifyParallelCmd()
	prepareTestCmd(cmd, buf)
	cmd.SetArgs([]string{"--path", ".", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("identify-parallel json format failed: %v", err)
	}

	// 4. Scan with default text format (summary reporting)
	cmd = NewIdentifyParallelCmd()
	prepareTestCmd(cmd, buf)
	cmd.SetArgs([]string{".", "--safe-only=false", "--unsafe-only=false"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("identify-parallel default format failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Summary:") {
		t.Errorf("expected Summary: in output, got: %s", buf.String())
	}
}

func TestRunCmd_ValidationErrors(t *testing.T) {
	_, cleanup := setupTestProject(t)
	defer cleanup()

	// 1. No IDs and not --all
	cmd := NewRunCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "specify one or more test_case IDs or pass --all") {
		t.Fatalf("expected specify one or more test_case IDs error, got: %v", err)
	}

	// 2. Invalid timeout duration
	buf.Reset()
	cmd = NewRunCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--timeout", "invalid_duration", "TST-001"})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid timeout duration") {
		t.Fatalf("expected invalid timeout error, got: %v", err)
	}

	// 3. --all with zero active test cases
	buf.Reset()
	cmd = NewRunCmd()
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--all"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected success with --all when no test cases exist, got: %v", err)
	}
	if !strings.Contains(buf.String(), "No active test cases found to run") {
		t.Errorf("expected 'No active test cases found to run', got: %s", buf.String())
	}
}

func TestRunCmd_DryRunAndExecution(t *testing.T) {
	testRoot, cleanup := setupTestProject(t)
	defer cleanup()

	stdctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	factory, err := storage.NewStorageFactory(stdctx, testRoot)
	if err != nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}
	sp := factory.GetStorage()
	secCtx := pkgctx.NewSystemSecurityContext()
	tcID := "TST-RUN-TEST-001"
	tcObj := map[string]any{
		objects.FieldKeyID:            tcID,
		objects.FieldKeyKind:          objects.KindTestCase,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Run Test Case Execution",
		objects.FieldKeyDescription:   "Description for test case execution",
	}
	if err := sp.Create(stdctx, secCtx, tcObj); err != nil {
		t.Fatalf("create test case: %v", err)
	}

	critID := "CRIT-RUN-TEST-001"
	critObj := map[string]any{
		objects.FieldKeyID:            critID,
		objects.FieldKeyKind:          objects.KindCriteria,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "originated",
		"category":                    "acceptance",
		objects.FieldKeyTitle:         "Run Criterion Execution",
		objects.FieldKeyDescription:   "Description for criterion execution",
		"command":                     "echo criterion-pass",
	}
	if err := sp.Create(stdctx, secCtx, critObj); err != nil {
		t.Fatalf("create criterion: %v", err)
	}
	tcObj[objects.FieldKeyCriteriaRefs] = []string{critID}
	if err := sp.Update(stdctx, secCtx, tcID, tcObj); err != nil {
		t.Fatalf("update test case with criterion: %v", err)
	}

	var buf bytes.Buffer
	cmd := NewRunCmd()
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--dry-run", "--verbose", tcID})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run --dry-run failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Executing Test Case: TST-RUN-TEST-001") {
		t.Errorf("expected Executing Test Case in output, got: %s", buf.String())
	}

	buf.Reset()
	cmd = NewRunCmd()
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--all", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run --all --dry-run failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Executing Test Case: TST-RUN-TEST-001") {
		t.Errorf("expected Executing Test Case in --all output, got: %s", buf.String())
	}
}

func TestBindCmd_HelpersAndEdgeCases(t *testing.T) {
	// 1. containsString
	list := []string{"REQ-001", "Goal-002", "bli-003"}
	if !containsString(list, "req-001") {
		t.Errorf("expected case-insensitive match for req-001")
	}
	if !containsString(list, "GOAL-002") {
		t.Errorf("expected case-insensitive match for GOAL-002")
	}
	if containsString(list, "NONEXISTENT") {
		t.Errorf("expected false for nonexistent item")
	}

	// 2. cleanLegacyFields
	obj := map[string]any{
		"category":         "legacy_category",
		"phase":            "legacy_phase",
		"estimated_effort": 10,
		"valid_field":      "keep_me",
		"archived_at":      12345, // invalid non-string
		"archived_by":      999,   // invalid non-string
	}
	cleanLegacyFields(obj)
	if obj["category"] != storage.FieldUnset {
		t.Errorf("expected category to be FieldUnset")
	}
	if obj["phase"] != storage.FieldUnset {
		t.Errorf("expected phase to be FieldUnset")
	}
	if obj["archived_at"] != storage.FieldUnset {
		t.Errorf("expected invalid non-string archived_at to be FieldUnset")
	}
	if obj["archived_by"] != storage.FieldUnset {
		t.Errorf("expected invalid non-string archived_by to be FieldUnset")
	}
	if obj["valid_field"] != "keep_me" {
		t.Errorf("expected valid_field to be retained")
	}
}

func TestBindCmd_ExplicitBinding(t *testing.T) {
	testRoot, cleanup := setupTestProject(t)
	defer cleanup()

	stdctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	factory, err := storage.NewStorageFactory(stdctx, testRoot)
	if err != nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}
	sp := factory.GetStorage()
	secCtx := pkgctx.NewSystemSecurityContext()

	tcID := "TST-TEST-BIND-001"
	reqID := "REQ-TEST-BIND-001"
	goalID := "GOAL-TEST-BIND-001"

	tcObj := map[string]any{
		objects.FieldKeyID:            tcID,
		objects.FieldKeyKind:          objects.KindTestCase,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Lineage Binding Test Case",
		objects.FieldKeyDescription:   "Description for test case to cross CAS membrane",
	}
	if err := sp.Create(stdctx, secCtx, tcObj); err != nil {
		t.Fatalf("create test case: %v", err)
	}

	reqObj := map[string]any{
		objects.FieldKeyID:            reqID,
		objects.FieldKeyKind:          objects.KindRequirement,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Lineage Binding Requirement",
		objects.FieldKeyDescription:   "Description for requirement to cross CAS membrane",
	}
	if err := sp.Create(stdctx, secCtx, reqObj); err != nil {
		t.Fatalf("create requirement: %v", err)
	}

	goalObj := map[string]any{
		objects.FieldKeyID:            goalID,
		objects.FieldKeyKind:          objects.KindGoal,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Lineage Binding Goal",
		objects.FieldKeyDescription:   "Description for goal to cross CAS membrane",
	}
	if err := sp.Create(stdctx, secCtx, goalObj); err != nil {
		t.Fatalf("create goal: %v", err)
	}

	// 1. Dry run explicit binding
	cmd := NewBindCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{tcID, "--req", reqID, "--goal", goalID, "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry run bind failed: %v", err)
	}
	if !strings.Contains(buf.String(), "[DRY RUN]") {
		t.Errorf("expected [DRY RUN] in output, got: %s", buf.String())
	}

	// 2. Real explicit binding
	buf.Reset()
	cmd = NewBindCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{tcID, "--req", reqID, "--goal", goalID, "--bli", "BLI-TEST-001"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("explicit bind failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Lineage updated successfully") {
		t.Errorf("expected Lineage updated successfully, got: %s", buf.String())
	}
}

func TestBindCmd_AssistantAndAutoBinding(t *testing.T) {
	testRoot, cleanup := setupTestProject(t)
	defer cleanup()

	stdctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	factory, err := storage.NewStorageFactory(stdctx, testRoot)
	if err != nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}
	sp := factory.GetStorage()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Display binding assistant with broken test cases
	tcBrokenID := "TST-BROKEN-001"
	tcBrokenObj := map[string]any{
		objects.FieldKeyID:            tcBrokenID,
		objects.FieldKeyKind:          objects.KindTestCase,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Broken Test Case For Assistant",
		objects.FieldKeyDescription:   "Test Case with missing requirement",
	}
	if err := sp.Create(stdctx, secCtx, tcBrokenObj); err != nil {
		t.Fatalf("create broken test case: %v", err)
	}

	reqAutoID := "REQ-AUTO-001"
	reqAutoObj := map[string]any{
		objects.FieldKeyID:            reqAutoID,
		objects.FieldKeyKind:          objects.KindRequirement,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Broken Test Case Requirement",
		objects.FieldKeyDescription:   "Requirement for broken test case auto binding",
	}
	_ = sp.Create(stdctx, secCtx, reqAutoObj)

	goalAutoID := "GOAL-AUTO-001"
	goalAutoObj := map[string]any{
		objects.FieldKeyID:            goalAutoID,
		objects.FieldKeyKind:          objects.KindGoal,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Broken Test Case Goal",
		objects.FieldKeyDescription:   "Goal for broken test case auto binding",
	}
	_ = sp.Create(stdctx, secCtx, goalAutoObj)

	cmd := NewBindCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("assistant display failed: %v", err)
	}

	// Auto binding dry run
	buf.Reset()
	cmd = NewBindCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--auto", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auto dry run failed: %v", err)
	}

	// Auto binding real execution
	buf.Reset()
	cmd = NewBindCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--auto"})
	_ = cmd.Execute()
}

func TestDashboardCmd_Execution(t *testing.T) {
	_, cleanup := setupTestProject(t)
	defer cleanup()

	// 1. Dashboard with check-dod
	cmd := NewDashboardCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--check-dod", "--color"})
	_ = cmd.Execute()

	// 2. Dashboard with refresh and view all
	buf.Reset()
	cmd = NewDashboardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--refresh", "--view", "all"})
	_ = cmd.Execute()

	// 3. Dashboard with status and test-case filter
	buf.Reset()
	cmd = NewDashboardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--status", "active", "--test-case", "TST-001"})
	_ = cmd.Execute()

	// 4. Dashboard with view regression
	buf.Reset()
	cmd = NewDashboardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--view", "regression"})
	_ = cmd.Execute()

	// 5. Dashboard with --all
	buf.Reset()
	cmd = NewDashboardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--all"})
	_ = cmd.Execute()

	// 6. Dashboard with pager
	buf.Reset()
	cmd = NewDashboardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--pager"})
	_ = cmd.Execute()
}

func TestDashboardState_InternalHelpers(t *testing.T) {
	// 1. statusIcon
	statuses := []string{
		"complete", "validated", "verified", "passed", "success",
		"active", "in_progress", "metrics_captured", "testing", "verifying", "awaiting_verification",
		"originated", "draft", "proposed", "conceptual", "planned",
		"failed", "error", "rejected",
		"archived", "deferred", "parked",
		"unknown_status_xyz",
	}
	for _, st := range statuses {
		icon := statusIcon(st)
		if icon == "" {
			t.Errorf("status %s returned empty icon", st)
		}
	}

	// 2. formatLineageStrip nil chain
	nilStrip := formatLineageStrip(nil, "TST-1", "active")
	if !strings.Contains(nilStrip, "NO LINEAGE") {
		t.Errorf("expected NO LINEAGE in nil chain strip, got: %s", nilStrip)
	}

	// 3. renderProgressBar
	pb1 := renderProgressBar(-10, 10)
	pb2 := renderProgressBar(150, 10)
	pb3 := renderProgressBar(50, 10)
	if len(pb1) == 0 || len(pb2) == 0 || len(pb3) == 0 {
		t.Errorf("expected non-empty progress bars")
	}

	// 4. DefaultPayload, Engine, WarmLiteFile
	testRoot, cleanup := setupTestProject(t)
	defer cleanup()

	state := NewDashboardState()
	defPayload := state.DefaultPayload()
	if defPayload == nil {
		t.Errorf("expected non-nil default payload")
	}
	_ = state.EnsureEngine(testRoot)
	eng := state.Engine()
	if eng == nil {
		t.Errorf("expected non-nil engine")
	}

	stdctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	factory, err := storage.NewStorageFactory(stdctx, testRoot)
	if err == nil {
		_ = WarmLiteFile(stdctx, testRoot, factory.GetStorage())
	}
	_ = state.Name()
	_ = state.ApplyEvent(&lifecycle.LifecycleEvent{EventType: lifecycle.EventTypeCriterionSatisfied, CriterionID: "CRIT-1"})
	_ = state.BuildPayload()
}

func TestStreamCmd_Properties(t *testing.T) {
	cmd := NewStreamCmd()
	if cmd == nil {
		t.Fatalf("expected NewStreamCmd to return non-nil command")
	}
	if len(cmd.Aliases) == 0 || cmd.Aliases[0] != "run-progress" {
		t.Errorf("expected alias run-progress, got %v", cmd.Aliases)
	}
	if cmd.Flags().Lookup("parallel") == nil {
		t.Errorf("expected --parallel flag on stream command")
	}
	if cmd.Flags().Lookup("pkg") == nil {
		t.Errorf("expected --pkg flag on stream command")
	}
	if cmd.Flags().Lookup("test-timeout") == nil {
		t.Errorf("expected --test-timeout flag on stream command")
	}
}

func TestStreamCmd_Execution(t *testing.T) {
	_, cleanup := setupTestProject(t)
	defer cleanup()

	// 1. With flags
	cmd := NewStreamCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--pkg", "./pkg/errfmt/...", "--parallel", "1", "--test-timeout", "10s"})
	_ = cmd.Execute()

	// 2. With positional arg
	buf.Reset()
	cmd = NewStreamCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"./pkg/errfmt/..."})
	_ = cmd.Execute()

	// 3. Nonexistent package error path
	buf.Reset()
	cmd = NewStreamCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"./pkg/nonexistent_xyz/..."})
	_ = cmd.Execute()

	// 4. Invalid timeout flag error
	cmd = NewStreamCmd()
	cmd.SetArgs([]string{"--test-timeout", "invalid_dur"})
	_ = cmd.Execute()
}

func TestDiscoverCmd_Extended(t *testing.T) {
	_, cleanup := setupTestProject(t)
	defer cleanup()

	cmd := NewDiscoverCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--format", "table"})
	_ = cmd.Execute()

	buf.Reset()
	cmd = NewDiscoverCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--format", "yaml"})
	_ = cmd.Execute()
}

func TestIdentifyParallelCmd_Extended(t *testing.T) {
	testRoot, cleanup := setupTestProject(t)
	defer cleanup()

	cmd := NewIdentifyParallelCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--format", "json", "--path", filepath.Join(testRoot, "pkg")})
	_ = cmd.Execute()

	buf.Reset()
	cmd = NewIdentifyParallelCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--format", "yaml", "--safe-only", "--path", filepath.Join(testRoot, "pkg")})
	_ = cmd.Execute()

	buf.Reset()
	cmd = NewIdentifyParallelCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--unsafe-only", "--path", filepath.Join(testRoot, "pkg")})
	_ = cmd.Execute()
}

func TestRunCmd_Extended(t *testing.T) {
	_, cleanup := setupTestProject(t)
	defer cleanup()

	cmd := NewRunCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--dry-run", "non-existent-tc"})
	_ = cmd.Execute()

	buf.Reset()
	cmd = NewRunCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{})
	_ = cmd.Execute()
}

func seedDashboardTestObjects(t *testing.T, sp storage.ObjectStorageProvider, stdctx context.Context, secCtx *pkgctx.SecurityContext) (string, string) {
	t.Helper()
	goalID := "GOAL-1792024710618118000-TEST001"
	reqID := "REQ-1792024710618118000-TEST001"
	critID := "CRIT-1792024710618118000-TEST001"
	tcID := "TST-1792024710618118000-TEST001"
	bliID := "BLI-1792024710618118000-TEST001"

	goalObj := map[string]any{
		objects.FieldKeyID:            goalID,
		objects.FieldKeyKind:          objects.KindGoal,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:         "Test Goal",
		objects.FieldKeyDescription:   "Test Goal Description",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
	}
	if err := sp.Create(stdctx, secCtx, goalObj); err != nil {
		t.Fatalf("create goal failed: %v", err)
	}

	reqObj := map[string]any{
		objects.FieldKeyID:            reqID,
		objects.FieldKeyKind:          objects.KindRequirement,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:         "Test Requirement",
		objects.FieldKeyDescription:   "Test Requirement Description",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyGoalRefs:      []string{goalID},
	}
	if err := sp.Create(stdctx, secCtx, reqObj); err != nil {
		t.Fatalf("create req failed: %v", err)
	}

	critObj := map[string]any{
		objects.FieldKeyID:              critID,
		objects.FieldKeyKind:            objects.KindCriteria,
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:           "Test Criteria",
		objects.FieldKeyDescription:     "Test Criteria Description",
		"category":                      "acceptance",
		objects.FieldKeyStatus:          "originated",
		objects.FieldKeyRequirementRefs: []string{reqID},
	}
	if err := sp.Create(stdctx, secCtx, critObj); err != nil {
		t.Fatalf("create crit failed: %v", err)
	}

	bliObj := map[string]any{
		objects.FieldKeyID:              bliID,
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:           "Test BLI",
		objects.FieldKeyDescription:     "Test BLI Description",
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
		objects.FieldKeyCriteriaRefs:    []string{critID},
		objects.FieldKeyRequirementRefs: []string{reqID},
	}
	if err := sp.Create(stdctx, secCtx, bliObj); err != nil {
		t.Fatalf("create bli failed: %v", err)
	}

	tcObj := map[string]any{
		objects.FieldKeyID:              tcID,
		objects.FieldKeyKind:            objects.KindTestCase,
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:           "Test Test Case",
		objects.FieldKeyDescription:     "Test Test Case Description",
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
		objects.FieldKeyPathOrID:        "pkg/test",
		"scope":                         "unit",
		"category":                      "regression",
		objects.FieldKeyCriteriaRefs:    []string{critID},
		objects.FieldKeyRequirementRefs: []string{reqID},
		objects.FieldKeyBacklogItemRefs: []string{bliID},
		objects.FieldKeyGoalRefs:        []string{goalID},
	}
	if err := sp.Create(stdctx, secCtx, tcObj); err != nil {
		t.Fatalf("create tc failed: %v", err)
	}
	return tcID, critID
}

func TestDashboardState_HydrationAndEnrichment(t *testing.T) {
	testRoot, cleanup := setupTestProject(t)
	defer cleanup()

	stdctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	factory, err := storage.NewStorageFactory(stdctx, testRoot)
	if err != nil {
		t.Fatalf("StorageFactory error: %v", err)
	}
	sp := factory.GetStorage()
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	tcID, _ := seedDashboardTestObjects(t, sp, stdctx, secCtx)

	state := NewDashboardState()
	state.projectRoot = testRoot
	state.storage = sp

	hydrated := state.hydrateTestCaseFromStorageLocked(tcID)
	if hydrated == nil {
		t.Fatalf("expected non-nil hydrated test case")
	}
	if hydrated.Title != "Test Test Case" {
		t.Errorf("expected Title 'Test Test Case', got %q", hydrated.Title)
	}
	if len(hydrated.Criteria) == 0 {
		t.Errorf("expected hydrated criteria")
	}

	tcModel := &TestCaseModel{
		ID: tcID,
	}
	state.enrichTestCaseFromStorageLocked(tcModel)
	if tcModel.Title != "Test Test Case" {
		t.Errorf("expected enriched Title, got %q", tcModel.Title)
	}

	state.refreshLineageLocked(tcModel)
	if tcModel.Lineage == nil {
		t.Errorf("expected non-nil lineage after refresh")
	}

	cmd := NewBindCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	state.TestCaseOrder = []string{tcID}
	_ = displayBindingAssistant(ctx, sp, secCtx, state, "", cmd)
	_ = displayBindingAssistant(ctx, sp, secCtx, state, tcID, cmd)
}

func TestIdentifyParallelCmd_FlagsAndFormats(t *testing.T) {
	_, cleanup := setupTestProject(t)
	defer cleanup()

	// 1. JSON format
	cmd := NewIdentifyParallelCmd()
	buf := newTestCommandBuffer(cmd)
	cmd.SetArgs([]string{"--format", "json", "."})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("identify-parallel --format json failed: %v", err)
	}

	// 2. YAML format
	cmd = NewIdentifyParallelCmd()
	prepareTestCmd(cmd, buf)
	cmd.SetArgs([]string{"--format", "yaml", "."})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("identify-parallel --format yaml failed: %v", err)
	}

	// 3. Safe-only flag
	cmd = NewIdentifyParallelCmd()
	prepareTestCmd(cmd, buf)
	cmd.SetArgs([]string{"--safe-only", "."})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("identify-parallel --safe-only failed: %v", err)
	}

	// 4. Unsafe-only flag
	cmd = NewIdentifyParallelCmd()
	prepareTestCmd(cmd, buf)
	cmd.SetArgs([]string{"--unsafe-only", "."})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("identify-parallel --unsafe-only failed: %v", err)
	}

	// 5. Audit cmd/zqk/test to exercise formatted safety printer
	cmd = NewIdentifyParallelCmd()
	prepareTestCmd(cmd, buf)
	cmd.SetArgs([]string{"cmd/zqk/test"})
	_ = cmd.Execute()
}

func TestDiscoverCmd_FlagsAndFormats(t *testing.T) {
	testRoot, cleanup := setupTestProject(t)
	defer cleanup()

	// 1. Format JSON
	cmd := NewDiscoverCmd()
	buf := newTestCommandBuffer(cmd)
	cmd.SetArgs([]string{"--format", "json", "--lang", "go", "."})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("discover --format json failed: %v", err)
	}

	// 2. Format YAML
	cmd = NewDiscoverCmd()
	prepareTestCmd(cmd, buf)
	cmd.SetArgs([]string{"--format", "yaml", "--lang", "go", "."})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("discover --format yaml failed: %v", err)
	}

	// 3. Verbose and dry-run
	cmd = NewDiscoverCmd()
	prepareTestCmd(cmd, buf)
	cmd.SetArgs([]string{"--verbose", "--dry-run", "--sync", "."})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("discover --verbose --dry-run failed: %v", err)
	}

	// 4. Absolute path handling
	cmd = NewDiscoverCmd()
	prepareTestCmd(cmd, buf)
	cmd.SetArgs([]string{testRoot})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("discover with absolute path failed: %v", err)
	}
}

func TestDashboardCmd_WatchAndDoDModes(t *testing.T) {
	_, cleanup := setupTestProject(t)
	defer cleanup()

	// 1. Check DoD (should satisfy when empty or clean)
	cmd := NewDashboardCmd()
	buf := newTestCommandBuffer(cmd)
	cmd.SetArgs([]string{"--check-dod"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("dashboard --check-dod failed: %v", err)
	}
	if !strings.Contains(buf.String(), "TPM Definition of Done SATISFIED") {
		t.Errorf("expected DoD satisfied output, got: %s", buf.String())
	}

	// 2. Watch mode with early cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	cmd = NewDashboardCmd()
	cmd.SetContext(ctx)
	prepareTestCmd(cmd, buf)
	cmd.SetArgs([]string{"--watch", "10ms"})
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("dashboard --watch failed: %v", err)
	}

	// 3. Filtering and view modes
	for _, view := range []string{"active", "regression", "all"} {
		cmd = NewDashboardCmd()
		prepareTestCmd(cmd, buf)
		cmd.SetArgs([]string{"--view", view, "--refresh"})
		if err := cmd.Execute(); err != nil {
			t.Errorf("dashboard --view %s --refresh failed: %v", view, err)
		}
	}

	// 4. Status filter and test case filter
	cmd = NewDashboardCmd()
	prepareTestCmd(cmd, buf)
	cmd.SetArgs([]string{"--status", "in_progress", "--test-case", "non_existent"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("dashboard --status failed: %v", err)
	}
}

func TestBindCmd_ErrorsAndValidation(t *testing.T) {
	_, cleanup := setupTestProject(t)
	defer cleanup()

	// 1. Non-existent test case
	cmd := NewBindCmd()
	buf := newTestCommandBuffer(cmd)
	cmd.SetArgs([]string{"TST-NONEXISTENT-999", "--req", "REQ-TEST-123"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected test case not found error, got: %v", err)
	}

	// 2. Target test case with no parent bindings runs binding assistant
	cmd = NewBindCmd()
	prepareTestCmd(cmd, buf)
	cmd.SetArgs([]string{"TST-TEST-123"})
	err = cmd.Execute()
	if err != nil {
		t.Fatalf("expected binding assistant execution, got err: %v", err)
	}
}
