package test

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/scheduler"
	cli "github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func setupCycle140TestProject(t *testing.T) (string, storage.ObjectStorageProvider) {
	t.Helper()
	testRoot := t.TempDir()
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(testRoot)
		teardownErr := storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, nil))
		if teardownErr != nil {
			t.Logf("cleanup teardown error: %v", teardownErr)
		}
	})

	t.Setenv(zqkenv.TestRoot().Name(), testRoot)
	repoRoot := zqkenv.ProjectRoot().Get()
	if repoRoot == "" {
		if root, err := findModuleRootForTestPkg(); err == nil {
			repoRoot = root
		}
	}
	bootErr := testenvroot.BootstrapRoot(testRoot, repoRoot)
	require.NoError(t, bootErr)

	stdctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	factory, err := storage.NewStorageFactory(stdctx, testRoot)
	require.NoError(t, err)
	require.NotNil(t, factory)

	return testRoot, factory.GetStorage()
}

func prepareCycle140Cmd(cmd *cobra.Command, buf *bytes.Buffer) {
	buf.Reset()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))
}

func TestCycle140_TopLevelTestCmd(t *testing.T) {
	cmd := NewTestCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "test", cmd.Name())

	expectedSubcommands := []string{
		"run", "discover", "dashboard", "bind",
		"check-contamination", "identify-parallel", "stream",
	}
	found := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		found[sub.Name()] = true
	}
	for _, exp := range expectedSubcommands {
		assert.True(t, found[exp], "expected subcommand %s", exp)
	}

	sch := scheduler.NewSchedulerCmd()
	require.NotNil(t, sch)
	for _, c := range sch.Commands() {
		assert.NotEqual(t, "scan-tests", c.Name(), "scan-tests must not exist on scheduler")
	}
}

func TestCycle140_ContaminationDetection(t *testing.T) {
	testRoot, _ := setupCycle140TestProject(t)
	planeDir := filepath.Join(testRoot, ".zqk", "process")

	cmd := NewCheckContaminationCmd()
	var buf bytes.Buffer
	prepareCycle140Cmd(cmd, &buf)

	// Contaminating command: modifies file in plane
	contaminateScript := fmt.Sprintf("touch %s/dirty_cycle140.yaml", planeDir)
	cmd.SetArgs([]string{"--command", contaminateScript, "--plane", planeDir})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "test contamination detected")

	// Clean command: does not modify plane
	cleanCmd := NewCheckContaminationCmd()
	prepareCycle140Cmd(cleanCmd, &buf)
	cleanCmd.SetArgs([]string{"--command", "echo uncontaminated", "--plane", planeDir})
	cleanErr := cleanCmd.Execute()
	require.NoError(t, cleanErr)
	assert.Contains(t, buf.String(), "Zero test contamination")
}

func TestCycle140_IdentifyParallelCmd_Filters(t *testing.T) {
	testRoot, _ := setupCycle140TestProject(t)

	// Create sample test file
	testFile := filepath.Join(testRoot, "sample_test.go")
	content := "package main\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n"
	writeErr := fileutil.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, writeErr)

	var buf bytes.Buffer
	cmd := NewIdentifyParallelCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{"--safe-only", testFile})
	safeErr := cmd.Execute()
	require.NoError(t, safeErr)

	cmd = NewIdentifyParallelCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{"--unsafe-only", testFile})
	unsafeErr := cmd.Execute()
	require.NoError(t, unsafeErr)

	cmd = NewIdentifyParallelCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{"--format", "yaml", testFile})
	yamlErr := cmd.Execute()
	require.NoError(t, yamlErr)
}

func TestCycle140_DiscoverCmd_SyncAndFormats(t *testing.T) {
	testRoot, sp := setupCycle140TestProject(t)

	testDir := filepath.Join(testRoot, "pkg", "sample")
	require.NoError(t, fileutil.MkdirAll(testDir, paths.DirPerm755))
	testFile := filepath.Join(testDir, "sample_test.go")
	content := "package sample\n\nimport \"testing\"\n\nfunc TestFeatureOne(t *testing.T) {}\n"
	require.NoError(t, fileutil.WriteFile(testFile, []byte(content), 0644))

	var buf bytes.Buffer
	// Test --lang filter and absolute path args
	cmd := NewDiscoverCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{"--lang", "go,python", "--verbose", testDir})
	discErr := cmd.Execute()
	require.NoError(t, discErr)

	// Test --format yaml
	cmd = NewDiscoverCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{"--format", "yaml", testDir})
	yamlErr := cmd.Execute()
	require.NoError(t, yamlErr)

	// Test --sync with --dry-run
	cmd = NewDiscoverCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{"--sync", "--dry-run", testDir})
	drySyncErr := cmd.Execute()
	require.NoError(t, drySyncErr)

	// Test --sync without dry-run (materializing test_case in storage)
	cmd = NewDiscoverCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{"--sync", testDir})
	syncErr := cmd.Execute()
	require.NoError(t, syncErr)
	assert.Contains(t, buf.String(), "Successfully synchronized")

	secCtx := pkgctx.NewSystemSecurityContext()
	listRes, listErr := sp.List(context.Background(), secCtx, nil, storage.ListFilter{
		Kind: objects.KindTestCase,
	})
	require.NoError(t, listErr)
	assert.NotEmpty(t, listRes.Objects)
}

func TestCycle140_RunCmd_AllEmptyAndExecution(t *testing.T) {
	_, sp := setupCycle140TestProject(t)
	var buf bytes.Buffer

	// 1. --all with no test cases in storage
	cmd := NewRunCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{"--all"})
	allErr := cmd.Execute()
	require.NoError(t, allErr)
	assert.Contains(t, buf.String(), "No active test cases found to run")

	// 2. Invalid timeout duration
	cmd = NewRunCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{"--timeout", "invalid_duration_140", "TST-001"})
	invErr := cmd.Execute()
	require.Error(t, invErr)
	assert.Contains(t, invErr.Error(), "invalid timeout duration")

	// 3. Create test case with failing criterion
	stdctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()
	tcFailID := "TST-FAIL-140"
	critFailID := "CRIT-FAIL-140"

	failCritObj := map[string]any{
		objects.FieldKeyID:            critFailID,
		objects.FieldKeyKind:          objects.KindCriteria,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "originated",
		"category":                    "acceptance",
		objects.FieldKeyTitle:         "Failing Criterion",
		objects.FieldKeyDescription:   "Description for failing criterion verification execution.",
	}
	require.NoError(t, sp.Create(stdctx, secCtx, failCritObj))

	failTcObj := map[string]any{
		objects.FieldKeyID:                 tcFailID,
		objects.FieldKeyKind:               objects.KindTestCase,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:             objects.ObjectStatusActive,
		objects.FieldKeyTitle:              "Failing Test Case",
		objects.FieldKeyDescription:        "Description for failing test case verification execution.",
		objects.FieldKeyCriteriaRefs:       []string{critFailID},
		objects.FieldKeyVerificationSuites: []string{fmt.Sprintf("%s: sh -c 'echo failing_stderr >&2; exit 2'", critFailID)},
	}
	require.NoError(t, sp.Create(stdctx, secCtx, failTcObj))

	cmd = NewRunCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{"--verbose", tcFailID})
	runFailErr := cmd.Execute()
	require.Error(t, runFailErr)
	assert.Contains(t, runFailErr.Error(), "failed")
	assert.Contains(t, buf.String(), "[FAIL]")
}

func TestCycle140_RunCmd_PassingCascade(t *testing.T) {
	_, sp := setupCycle140TestProject(t)
	var buf bytes.Buffer
	stdctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()

	reqID := "REQ-PASS-140"
	critID := "CRIT-PASS-140"
	tcID := "TST-PASS-140"

	critObj := map[string]any{
		objects.FieldKeyID:            critID,
		objects.FieldKeyKind:          objects.KindCriteria,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "originated",
		"category":                    "acceptance",
		objects.FieldKeyTitle:         "Passing Criterion",
		objects.FieldKeyDescription:   "Description for passing criterion verification execution.",
	}
	require.NoError(t, sp.Create(stdctx, secCtx, critObj))

	reqObj := map[string]any{
		objects.FieldKeyID:            reqID,
		objects.FieldKeyKind:          objects.KindRequirement,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Requirement for Passing Test",
		objects.FieldKeyDescription:   "Description for passing requirement verification execution.",
		objects.FieldKeyCriteriaRefs:  []string{critID},
	}
	require.NoError(t, sp.Create(stdctx, secCtx, reqObj))

	tcObj := map[string]any{
		objects.FieldKeyID:                 tcID,
		objects.FieldKeyKind:               objects.KindTestCase,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:             objects.ObjectStatusActive,
		objects.FieldKeyTitle:              "Passing Test Case",
		objects.FieldKeyDescription:        "Description for passing test case verification execution.",
		objects.FieldKeyRequirementRefs:    []string{reqID},
		objects.FieldKeyCriteriaRefs:       []string{critID},
		objects.FieldKeyVerificationSuites: []string{fmt.Sprintf("%s: echo success_cycle140", critID)},
	}
	require.NoError(t, sp.Create(stdctx, secCtx, tcObj))

	cmd := NewRunCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{tcID})
	runErr := cmd.Execute()
	require.NoError(t, runErr)
	assert.Contains(t, buf.String(), "[PASS]")
	assert.Contains(t, buf.String(), "1 passed, 0 failed")
}

func TestCycle140_StreamCmd_Patterns(t *testing.T) {
	_, _ = setupCycle140TestProject(t)
	var buf bytes.Buffer

	cmd := NewStreamCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{"--parallel", "1", "nonexistent_pkg_cycle140/..."})
	streamErr := cmd.Execute()
	// Should fail cleanly for nonexistent package or execute without panic
	if streamErr != nil {
		assert.Contains(t, streamErr.Error(), "test run exited")
	}
}

func TestCycle140_Dashboard_RunWithPager(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	err := runWithPager(cancelledCtx, func(w ioWriter) error { return nil })
	require.Error(t, err)

	t.Setenv("PAGER", "cat")
	catErr := runWithPager(context.Background(), func(w ioWriter) error {
		_, writeErr := fmt.Fprintln(w, "test pager line")
		return writeErr
	})
	require.NoError(t, catErr)

	t.Setenv("PAGER", "nonexistent_pager_cmd_140")
	fallbackErr := runWithPager(context.Background(), func(w ioWriter) error {
		return nil
	})
	require.NoError(t, fallbackErr)
}

func TestCycle140_Dashboard_LineageResolution(t *testing.T) {
	mockStore := map[string]map[string]any{
		"REQ-MIL": {
			objects.FieldKeyID:            "REQ-MIL",
			objects.FieldKeyStatus:        "active",
			objects.FieldKeyTitle:         "Milestone Req",
			objects.FieldKeyMilestoneRefs: []string{"MIL-CYCLE140"},
		},
		"MIL-CYCLE140": {
			objects.FieldKeyID:     "MIL-CYCLE140",
			objects.FieldKeyStatus: "active",
			objects.FieldKeyTitle:  "Cycle 140 Milestone",
		},
		"BLI-PRI": {
			objects.FieldKeyID:              "BLI-PRI",
			objects.FieldKeyStatus:          "in_progress",
			objects.FieldKeyTitle:           "Priority BLI",
			objects.FieldKeyPriorityPlanRef: "PRI-CYCLE140",
		},
		"PRI-CYCLE140": {
			objects.FieldKeyID:     "PRI-CYCLE140",
			objects.FieldKeyStatus: "in_progress",
			objects.FieldKeyTitle:  "Cycle 140 Priority Plan",
		},
		"BLI-GOAL": {
			objects.FieldKeyID:       "BLI-GOAL",
			objects.FieldKeyStatus:   "in_progress",
			objects.FieldKeyTitle:    "Goal BLI",
			objects.FieldKeyGoalRefs: []string{"GOAL-CYCLE140"},
		},
		"GOAL-CYCLE140": {
			objects.FieldKeyID:     "GOAL-CYCLE140",
			objects.FieldKeyStatus: "active",
			objects.FieldKeyTitle:  "Cycle 140 Goal",
		},
		"REQ-NOGOAL": {
			objects.FieldKeyID:     "REQ-NOGOAL",
			objects.FieldKeyStatus: "active",
			objects.FieldKeyTitle:  "No Goal Req",
		},
	}
	readObj := func(id string) map[string]any {
		return mockStore[id]
	}

	// 1. Milestone root resolution
	chainMil := resolveLineageChain(readObj, []string{"REQ-MIL"}, nil, nil)
	require.NotNil(t, chainMil)
	assert.True(t, chainMil.IsIntact)
	require.NotNil(t, chainMil.RootObject)
	assert.Equal(t, objects.KindMilestone, chainMil.RootObject.Kind)

	// 2. Priority plan root resolution
	chainPri := resolveLineageChain(readObj, nil, []string{"BLI-PRI"}, nil)
	require.NotNil(t, chainPri)
	assert.True(t, chainPri.IsIntact)
	require.NotNil(t, chainPri.RootObject)
	assert.Equal(t, objects.KindPriorityPlan, chainPri.RootObject.Kind)

	// 3. Goal root resolution via BLI
	chainGoal := resolveLineageChain(readObj, nil, []string{"BLI-GOAL"}, nil)
	require.NotNil(t, chainGoal)
	assert.True(t, chainGoal.IsIntact)
	require.NotNil(t, chainGoal.RootObject)
	assert.Equal(t, objects.KindGoal, chainGoal.RootObject.Kind)

	// 4. Missing root object error
	chainMissingRoot := resolveLineageChain(readObj, []string{"REQ-NOGOAL"}, nil, nil)
	require.NotNil(t, chainMissingRoot)
	assert.False(t, chainMissingRoot.IsIntact)
	assert.Equal(t, "missing root object binding", chainMissingRoot.BrokenReason)

	// 5. Completely unbound chain
	chainEmpty := resolveLineageChain(readObj, nil, nil, nil)
	require.NotNil(t, chainEmpty)
	assert.False(t, chainEmpty.IsIntact)
	assert.Contains(t, chainEmpty.BrokenReason, "missing root object and requirement")
}

func TestCycle140_Dashboard_DoDAndWarmLite(t *testing.T) {
	testRoot, sp := setupCycle140TestProject(t)
	warmErr := WarmLiteFile(context.Background(), testRoot, sp)
	require.NoError(t, warmErr)

	state := NewDashboardStateWithProjectRoot(testRoot)
	require.NotNil(t, state)

	// Verify getOrInitStorageLocked directly
	state.storage = nil
	initializedSp := state.getOrInitStorageLocked(context.Background())
	require.NotNil(t, initializedSp)

	var outBuf, errBuf bytes.Buffer
	// Add unbroken test case
	state.TestCases["TST-INTACT"] = &TestCaseModel{
		ID:     "TST-INTACT",
		Status: objects.ObjectStatusActive,
		Lineage: &LineageChain{
			IsIntact: true,
		},
	}
	state.TestCaseOrder = []string{"TST-INTACT"}
	dodErr := state.ValidateTPMDefinitionOfDone(&outBuf, &errBuf)
	require.NoError(t, dodErr)
	assert.Contains(t, outBuf.String(), "TPM Definition of Done SATISFIED")

	// Add unbound test criterion
	state.UnboundTestCriteria = append(state.UnboundTestCriteria, &UnboundCriterionModel{
		ID:               "CRIT-UNBOUND-140",
		Status:           objects.ObjectStatusActive,
		Category:         "test",
		ValidationMethod: "automated_test",
	})
	unboundDodErr := state.ValidateTPMDefinitionOfDone(&outBuf, &errBuf)
	require.Error(t, unboundDodErr)
	assert.Contains(t, errBuf.String(), "unbound test criteria detected")
}

func TestCycle140_Dashboard_WALEventsAndViews(t *testing.T) {
	testRoot, _ := setupCycle140TestProject(t)

	// Write lifecycle event to WAL
	wal, walErr := lifecycle.GetOrCreateLifecycleWAL(testRoot)
	require.NoError(t, walErr)
	appendErr := wal.Append(&lifecycle.LifecycleEvent{
		EventType:   lifecycle.EventTypeCriterionSatisfied,
		CriterionID: "CRIT-WAL-140",
		Ts:          time.Now(),
	})
	require.NoError(t, appendErr)

	appendStatusErr := wal.Append(&lifecycle.LifecycleEvent{
		EventType:  lifecycle.EventTypeStatusTransition,
		Kind:       objects.KindCriteria,
		ID:         "CRIT-WAL-140",
		FromStatus: "originated",
		ToStatus:   "in_progress",
		Ts:         time.Now(),
	})
	require.NoError(t, appendStatusErr)

	state := NewDashboardStateWithProjectRoot(testRoot)
	state.seedRecentEventsFromWAL(testRoot)

	// Test dashboard commands with various views
	var buf bytes.Buffer
	for _, view := range []string{"active", "regression", "all"} {
		cmd := NewDashboardCmd()
		prepareCycle140Cmd(cmd, &buf)
		cmd.SetArgs([]string{"--view", view})
		runErr := cmd.Execute()
		require.NoError(t, runErr)
	}

	// Test refresh flag
	cmd := NewDashboardCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{"--refresh"})
	refErr := cmd.Execute()
	require.NoError(t, refErr)
}

func TestCycle140_BindingAssistant_Branches(t *testing.T) {
	testRoot, sp := setupCycle140TestProject(t)
	stdctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()

	// Seed 2 requirements and 2 goals
	for i := 1; i <= 2; i++ {
		reqID := fmt.Sprintf("REQ-BIND-%d", i)
		require.NoError(t, sp.Create(stdctx, secCtx, map[string]any{
			objects.FieldKeyID:            reqID,
			objects.FieldKeyKind:          objects.KindRequirement,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusActive,
			objects.FieldKeyTitle:         fmt.Sprintf("Binding Requirement %d", i),
			objects.FieldKeyDescription:   "Description for binding assistant requirement validation.",
		}))
		goalID := fmt.Sprintf("GOAL-BIND-%d", i)
		require.NoError(t, sp.Create(stdctx, secCtx, map[string]any{
			objects.FieldKeyID:            goalID,
			objects.FieldKeyKind:          objects.KindGoal,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusActive,
			objects.FieldKeyTitle:         fmt.Sprintf("Binding Goal %d", i),
			objects.FieldKeyDescription:   "Description for binding assistant goal validation.",
		}))
	}

	state := NewDashboardStateWithProjectRoot(testRoot)
	// Create >10 broken test cases to trigger the "... and X more test cases" branch
	for i := 1; i <= 12; i++ {
		tcID := fmt.Sprintf("TST-BROKEN-%d", i)
		var lineage *LineageChain
		if i == 1 {
			// Missing root goal
			lineage = &LineageChain{
				Requirements: []*LineageNode{{ID: "REQ-BIND-1", Kind: objects.KindRequirement, Status: "active"}},
				RootObject:   nil,
				IsIntact:     false,
			}
		} else {
			// Missing requirement & bli
			lineage = &LineageChain{IsIntact: false}
		}
		state.TestCases[tcID] = &TestCaseModel{
			ID:              tcID,
			Title:           fmt.Sprintf("Broken Test Case %d", i),
			Status:          objects.ObjectStatusActive,
			Lineage:         lineage,
			RequirementRefs: []string{},
		}
		if i == 1 {
			state.TestCases[tcID].RequirementRefs = []string{"REQ-BIND-1"}
		}
		state.TestCaseOrder = append(state.TestCaseOrder, tcID)
	}

	cmd := NewBindCmd()
	var buf bytes.Buffer
	prepareCycle140Cmd(cmd, &buf)

	assistErr := displayBindingAssistant(context.Background(), sp, secCtx, state, "", cmd)
	require.NoError(t, assistErr)
	out := buf.String()
	assert.Contains(t, out, "TPM LINEAGE BINDING ASSISTANT")
	assert.Contains(t, out, "more test cases (use --auto to repair all)")
	assert.Contains(t, out, "(Recommended)")
	assert.Contains(t, out, "(Alternative)")

	// Test assistant when 0 broken test cases
	stateClean := NewDashboardStateWithProjectRoot(testRoot)
	var cleanBuf bytes.Buffer
	prepareCycle140Cmd(cmd, &cleanBuf)
	cleanAssistErr := displayBindingAssistant(context.Background(), sp, secCtx, stateClean, "", cmd)
	require.NoError(t, cleanAssistErr)
	assert.Contains(t, cleanBuf.String(), "TPM Definition of Done SATISFIED")
}

func TestCycle140_BindCmd_ExplicitAndAutoExecution(t *testing.T) {
	_, sp := setupCycle140TestProject(t)
	stdctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()

	reqID := "REQ-EXEC-140"
	goalID := "GOAL-EXEC-140"
	tcID := "TST-EXEC-140"

	require.NoError(t, sp.Create(stdctx, secCtx, map[string]any{
		objects.FieldKeyID:            reqID,
		objects.FieldKeyKind:          objects.KindRequirement,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Exec Requirement",
		objects.FieldKeyDescription:   "Description for explicit binding requirement execution.",
	}))
	require.NoError(t, sp.Create(stdctx, secCtx, map[string]any{
		objects.FieldKeyID:            goalID,
		objects.FieldKeyKind:          objects.KindGoal,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Exec Goal",
		objects.FieldKeyDescription:   "Description for explicit binding goal execution.",
	}))
	require.NoError(t, sp.Create(stdctx, secCtx, map[string]any{
		objects.FieldKeyID:            tcID,
		objects.FieldKeyKind:          objects.KindTestCase,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Exec Test Case",
		objects.FieldKeyDescription:   "Description for explicit binding test case execution.",
	}))

	var buf bytes.Buffer
	// Explicit binding with --req and --goal
	cmd := NewBindCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{tcID, "--req", reqID, "--goal", goalID})
	bindErr := cmd.Execute()
	require.NoError(t, bindErr)
	assert.Contains(t, buf.String(), "Lineage updated successfully")

	// Explicit binding with --dry-run and --bli
	cmd = NewBindCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{tcID, "--bli", "BLI-SAMPLE-140", "--dry-run"})
	dryErr := cmd.Execute()
	require.NoError(t, dryErr)

	// Missing test case error branch
	cmd = NewBindCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{"NONEXISTENT-TC", "--req", reqID})
	missingTcErr := cmd.Execute()
	require.Error(t, missingTcErr)

	// Auto binding with --dry-run
	cmd = NewBindCmd()
	prepareCycle140Cmd(cmd, &buf)
	cmd.SetArgs([]string{"--auto", "--dry-run"})
	autoErr := cmd.Execute()
	require.NoError(t, autoErr)
}
