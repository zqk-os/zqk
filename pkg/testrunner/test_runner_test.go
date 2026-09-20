package testrunner_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/testrunner"
)

// CRIT-TEST-RUNNER-001: CLI test run command specification & argument resolution
func TestCRIT001_CommandAndArgumentResolution(t *testing.T) {
	tcObj := map[string]any{
		objects.FieldKeyID:           "TST-UNIT-001",
		objects.FieldKeyKind:         objects.KindTestCase,
		objects.FieldKeyTitle:        "Test Unit Spec",
		objects.FieldKeyPathOrID:     "pkg/testrunner/test_runner_test.go",
		objects.FieldKeyCriteriaRefs: []string{"CRIT-001", "CRIT-002"},
		objects.FieldKeyVerificationSuites: []string{
			"CRIT-001: echo 'PASS-1'",
			"CRIT-002: echo 'PASS-2'",
		},
	}

	invocations, err := testrunner.ResolveInvocations(tcObj, "/tmp")
	if err != nil {
		t.Fatalf("unexpected error resolving invocations: %v", err)
	}

	if len(invocations) != 2 {
		t.Fatalf("expected 2 invocations, got %d", len(invocations))
	}

	if invocations[0].CriterionID != "CRIT-001" || invocations[0].Command != "echo 'PASS-1'" {
		t.Errorf("unexpected invocation 0: %+v", invocations[0])
	}
	if invocations[1].CriterionID != "CRIT-002" || invocations[1].Command != "echo 'PASS-2'" {
		t.Errorf("unexpected invocation 1: %+v", invocations[1])
	}
}

// CRIT-TEST-RUNNER-002: Subprocess test execution & output capture adapter
func TestCRIT002_SubprocessExecutionAndOutputCapture(t *testing.T) {
	ctx := context.Background()

	// 1. Passing subprocess
	passInv := testrunner.CriterionInvocation{
		CriterionID: "CRIT-PASS",
		Command:     "echo 'Subprocess execution success'",
	}
	passRes := testrunner.ExecuteSubprocess(ctx, passInv)
	if !passRes.Passed {
		t.Errorf("expected passing result, got failure: %s", passRes.Error)
	}
	if passRes.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", passRes.ExitCode)
	}
	if passRes.Stdout == "" {
		t.Errorf("expected stdout capture, got empty")
	}

	// 2. Failing subprocess (RED test)
	failInv := testrunner.CriterionInvocation{
		CriterionID: "CRIT-FAIL",
		Command:     "sh -c 'echo \"Fatal failure\" >&2; exit 2'",
	}
	failRes := testrunner.ExecuteSubprocess(ctx, failInv)
	if failRes.Passed {
		t.Errorf("expected failing result for RED test, got passed")
	}
	if failRes.ExitCode != 2 {
		t.Errorf("expected exit code 2, got %d", failRes.ExitCode)
	}
	if failRes.Stderr == "" {
		t.Errorf("expected stderr capture, got empty")
	}
}

// CRIT-TEST-RUNNER-003 & CRIT-TEST-RUNNER-004: State machine transitions, counter decrement & terminal gating
func TestCRIT003_CRIT004_LifecycleShockwaveAndTerminalGating(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
	})
	sp := proj.FileStorage
	tempDir := proj.Root
	ctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()

	// Setup Goal
	goalID := "GOAL-E2E-001"
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:          goalID,
		objects.FieldKeyKind:        objects.KindGoal,
		objects.FieldKeyTitle:       "E2E Goal",
		objects.FieldKeyDescription: "E2E Goal Description",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
	}); err != nil {
		t.Fatalf("failed to create goal: %v", err)
	}

	// Setup 2 Criteria
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:          "CRIT-E2E-001",
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyTitle:       "Criteria 1",
		objects.FieldKeyDescription: "Criteria 1 Description",
		objects.FieldKeyStatus:      objects.ObjectStatusAwaitingVerification,
		objects.FieldKeyCategory:    "test",
	}); err != nil {
		t.Fatalf("failed to create crit1: %v", err)
	}
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:          "CRIT-E2E-002",
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyTitle:       "Criteria 2",
		objects.FieldKeyDescription: "Criteria 2 Description",
		objects.FieldKeyStatus:      objects.ObjectStatusAwaitingVerification,
		objects.FieldKeyCategory:    "test",
	}); err != nil {
		t.Fatalf("failed to create crit2: %v", err)
	}

	// Setup Requirement
	reqID := "REQ-E2E-001"
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:          reqID,
		objects.FieldKeyKind:        objects.KindRequirement,
		objects.FieldKeyTitle:       "E2E Requirement",
		objects.FieldKeyDescription: "E2E Requirement Description",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
		objects.FieldKeyGoalRefs:    []string{goalID},
		objects.FieldKeyCriteriaRefs: []string{
			"CRIT-E2E-001",
			"CRIT-E2E-002",
		},
	}); err != nil {
		t.Fatalf("failed to create req: %v", err)
	}

	// Setup Test Case
	tcID := "TST-E2E-001"
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:              tcID,
		objects.FieldKeyKind:            objects.KindTestCase,
		objects.FieldKeyTitle:           "E2E Test Suite",
		objects.FieldKeyDescription:     "E2E Test Suite Description",
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
		objects.FieldKeyRequirementRefs: []string{reqID},
		objects.FieldKeyCriteriaRefs: []string{
			"CRIT-E2E-001",
			"CRIT-E2E-002",
		},
		objects.FieldKeyVerificationSuites: []string{
			"CRIT-E2E-001: echo 'Criteria 1 Green'",
			"CRIT-E2E-002: exit 1", // Intentionally RED first!
		},
	}); err != nil {
		t.Fatalf("failed to create test case: %v", err)
	}

	// Seed remaining_open_count on test_case: should be 2
	if err := lifecycle.SeedRemainingOpenCountFromMembers(ctx, sp, tcID, true); err != nil {
		t.Fatalf("failed to seed remaining_open_count: %v", err)
	}

	tcObj, _ := sp.Read(ctx, secCtx, tcID)
	rem, _ := tcObj[objects.FieldKeyRemainingOpenCount].(int)
	if rem != 2 {
		t.Fatalf("expected initial remaining_open_count=2, got %d", rem)
	}

	// 1. Run with 1 GREEN and 1 RED test
	opts := testrunner.RunOptions{}
	res1, err := testrunner.RunTestCase(ctx, sp, tempDir, tcID, opts)
	if err != nil {
		t.Fatalf("failed to run test case: %v", err)
	}

	if res1.PassedCriteria != 1 || res1.FailedCriteria != 1 {
		t.Fatalf("expected 1 pass, 1 fail, got pass=%d fail=%d", res1.PassedCriteria, res1.FailedCriteria)
	}

	// Verify CRIT-E2E-001 transitioned to complete
	c1, _ := sp.Read(ctx, secCtx, "CRIT-E2E-001")
	if c1[objects.FieldKeyStatus] != objects.ObjectStatusComplete {
		t.Errorf("expected CRIT-E2E-001 to be complete, got %v", c1[objects.FieldKeyStatus])
	}

	// Verify Test Case remaining_open_count decremented from 2 to 1
	tcObj, _ = sp.Read(ctx, secCtx, tcID)
	rem, _ = tcObj[objects.FieldKeyRemainingOpenCount].(int)
	if rem != 1 {
		t.Errorf("expected remaining_open_count=1 after 1 criterion met, got %d", rem)
	}

	// Gating: Test Case must NOT be terminal yet!
	if tcObj[objects.FieldKeyStatus] == objects.ObjectStatusComplete {
		t.Fatalf("test_case must NOT complete while 1 criterion is still pending")
	}

	// 2. Now turn RED test to GREEN
	tcObj[objects.FieldKeyVerificationSuites] = []string{
		"CRIT-E2E-001: echo 'Criteria 1 Green'",
		"CRIT-E2E-002: echo 'Criteria 2 Green'",
	}
	_ = sp.Update(ctx, secCtx, tcID, tcObj)

	res2, err := testrunner.RunTestCase(ctx, sp, tempDir, tcID, opts)
	if err != nil {
		t.Fatalf("failed to run second test case pass: %v", err)
	}

	if res2.PassedCriteria != 2 {
		t.Fatalf("expected 2 passes, got %d", res2.PassedCriteria)
	}

	// Verify CRIT-E2E-002 transitioned to complete
	c2, _ := sp.Read(ctx, secCtx, "CRIT-E2E-002")
	if c2[objects.FieldKeyStatus] != objects.ObjectStatusComplete {
		t.Errorf("expected CRIT-E2E-002 to be complete, got %v", c2[objects.FieldKeyStatus])
	}

	// Terminal Gating: Test Case must now be complete!
	tcObj, _ = sp.Read(ctx, secCtx, tcID)
	if tcObj[objects.FieldKeyStatus] != objects.ObjectStatusComplete {
		t.Errorf("expected test_case to complete when all criteria met, got %v", tcObj[objects.FieldKeyStatus])
	}

	// Reactive Cascade: Requirement should now be complete!
	reqObj, rErr := sp.Read(ctx, secCtx, reqID)
	if rErr != nil {
		t.Fatalf("failed to read reqObj: %v", rErr)
	}
	if reqObj[objects.FieldKeyStatus] != objects.ObjectStatusComplete {
		t.Errorf("expected requirement to auto-complete when test_case completes, got %v, obj: %+v", reqObj[objects.FieldKeyStatus], reqObj)
	}

	// Cascade to Goal: Goal should now be complete!
	goalObj, _ := sp.Read(ctx, secCtx, goalID)
	if goalObj[objects.FieldKeyStatus] != objects.ObjectStatusComplete {
		t.Errorf("expected goal to auto-complete when requirement completes, got %v", goalObj[objects.FieldKeyStatus])
	}
}

func TestResolveInvocations_GoTestFileTargeting(t *testing.T) {
	tcObj := map[string]any{
		objects.FieldKeyID:           "TST-TEST-TARGET",
		objects.FieldKeyKind:         objects.KindTestCase,
		objects.FieldKeyTitle:        "Test Unit Targeting",
		objects.FieldKeyPathOrID:     "pkg/testrunner/test_runner_test.go",
		objects.FieldKeyCriteriaRefs: []string{"CRIT-TARGET-1"},
	}

	invocations, err := testrunner.ResolveInvocations(tcObj, "")
	if err != nil {
		t.Fatalf("unexpected error resolving invocations: %v", err)
	}
	if len(invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(invocations))
	}
	if !strings.Contains(invocations[0].Command, "-run '^(Test") {
		t.Errorf("expected command to have targeted -run pattern, got: %s", invocations[0].Command)
	}
}

// TRACK: BLI-TESTCASE-SANDBOX-PGID-001
func TestCRIT_PGID_Isolation_And_Teardown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	inv := testrunner.CriterionInvocation{
		CriterionID: "CRIT-PGID-TIMEOUT",
		Command:     "sleep 10",
	}

	start := time.Now()
	res := testrunner.ExecuteSubprocess(ctx, inv)
	duration := time.Since(start)

	if res.Passed {
		t.Errorf("expected command to fail due to timeout/cancel, got passed")
	}
	if duration > 2*time.Second {
		t.Errorf("expected process group to be killed promptly on timeout, took %v", duration)
	}
}

// TRACK: BLI-TESTCASE-SANDBOX-TMPDIR-002
func TestCRIT_Ephemeral_TMPDIR_Provisioning(t *testing.T) {
	ctx := context.Background()

	inv := testrunner.CriterionInvocation{
		CriterionID: "CRIT-TMPDIR",
		Command:     `echo "$TMPDIR"`,
	}

	res := testrunner.ExecuteSubprocess(ctx, inv)
	if !res.Passed {
		t.Fatalf("expected command to pass, got error: %s", res.Error)
	}

	tmpPath := strings.TrimSpace(res.Stdout)
	if tmpPath == "" {
		t.Fatalf("expected non-empty TMPDIR in subprocess")
	}
	if !strings.Contains(tmpPath, "zqk-testcase-") {
		t.Errorf("expected TMPDIR to contain 'zqk-testcase-', got: %s", tmpPath)
	}

	// After ExecuteSubprocess completes, tmpPath must have been swept
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("expected ephemeral TMPDIR %s to be deleted after subprocess termination", tmpPath)
	}
}

func init() {
	// Set test environment variable so tests can run
	_ = filepath.Separator
}
