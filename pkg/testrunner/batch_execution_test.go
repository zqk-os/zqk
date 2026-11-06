package testrunner_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/testrunner"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// GH-1003 / BLI-1793942683855970000-dbc844d5: Test Case Runner De-Duplication & Batch Criteria Execution
// Verifies that criteria sharing the same runner script execute in a single batch pass.
func TestBatchCriteriaExecution_SinglePassSuccess(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
	})
	sp := proj.FileStorage
	tempDir := proj.Root
	ctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create a shared runner script that tracks invocation count and received criteria
	invocationsLog := filepath.Join(tempDir, "invocations.log")
	runnerScriptPath := filepath.Join(tempDir, "scripts", "run_batch_tests.sh")
	if err := fileutil.MkdirAll(filepath.Dir(runnerScriptPath), 0755); err != nil {
		t.Fatalf("failed to create script dir: %v", err)
	}

	scriptContent := fmt.Sprintf(`#!/usr/bin/env bash
echo "$@" >> '%s'
# Exit 0 indicating all batched criteria passed
exit 0
`, invocationsLog)

	if err := fileutil.WriteFile(runnerScriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to write runner script: %v", err)
	}

	goalID := "GOAL-BATCH-001"
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:          goalID,
		objects.FieldKeyKind:        objects.KindGoal,
		objects.FieldKeyTitle:       "Batch Goal",
		objects.FieldKeyDescription: "Batch Goal Description",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
	}); err != nil {
		t.Fatalf("create goal: %v", err)
	}

	crit1 := "CRIT-BATCH-001"
	crit2 := "CRIT-BATCH-002"
	crit3 := "CRIT-BATCH-003"
	for _, cid := range []string{crit1, crit2, crit3} {
		if err := sp.Create(ctx, secCtx, map[string]any{
			objects.FieldKeyID:          cid,
			objects.FieldKeyKind:        objects.KindCriteria,
			objects.FieldKeyTitle:       cid + " Title",
			objects.FieldKeyDescription: cid + " Description",
			objects.FieldKeyCategory:    "functional",
			objects.FieldKeyStatus:      objects.ObjectStatusAwaitingVerification,
		}); err != nil {
			t.Fatalf("create %s: %v", cid, err)
		}
	}

	reqID := "REQ-BATCH-001"
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:           reqID,
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyTitle:        "Batch Requirement",
		objects.FieldKeyDescription:  "Batch Requirement Description",
		objects.FieldKeyStatus:       objects.ObjectStatusActive,
		objects.FieldKeyGoalRefs:     []string{goalID},
		objects.FieldKeyCriteriaRefs: []string{crit1, crit2, crit3},
	}); err != nil {
		t.Fatalf("create req: %v", err)
	}

	tcID := "TST-BATCH-001"
	relScript := filepath.Join("scripts", "run_batch_tests.sh")
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:              tcID,
		objects.FieldKeyKind:            objects.KindTestCase,
		objects.FieldKeyTitle:           "Batch Test Case",
		objects.FieldKeyDescription:     "Batch Test Case Description",
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
		objects.FieldKeyPathOrID:        relScript,
		objects.FieldKeyRequirementRefs: []string{reqID},
		objects.FieldKeyCriteriaRefs:    []string{crit1, crit2, crit3},
	}); err != nil {
		t.Fatalf("create test case: %v", err)
	}

	if err := lifecycle.SeedRemainingOpenCountFromMembers(ctx, sp, tcID, true); err != nil {
		t.Fatalf("seed remaining count: %v", err)
	}

	// Execute test case with batch execution enabled (default)
	opts := testrunner.RunOptions{}
	res, err := testrunner.RunTestCase(ctx, sp, tempDir, tcID, opts)
	if err != nil {
		t.Fatalf("RunTestCase failed: %v", err)
	}

	if !res.BatchedExecution {
		t.Errorf("expected BatchedExecution to be true")
	}
	if res.FallbackTriggered {
		t.Errorf("expected FallbackTriggered to be false")
	}
	if res.PassedCriteria != 3 || res.FailedCriteria != 0 {
		t.Errorf("expected 3 passed, 0 failed; got passed=%d failed=%d", res.PassedCriteria, res.FailedCriteria)
	}
	if res.Status != "passed" {
		t.Errorf("expected status 'passed', got %s", res.Status)
	}

	// Verify the script was executed EXACTLY ONCE (single batch pass)
	logData, err := fileutil.ReadFile(invocationsLog)
	if err != nil {
		t.Fatalf("failed to read invocation log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(logData)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly 1 invocation of runner script, got %d lines: %v", len(lines), lines)
	}
	// Verify all 3 criteria IDs were passed to the batch invocation
	expectedArgs := fmt.Sprintf("%s %s %s", crit1, crit2, crit3)
	if !strings.Contains(lines[0], expectedArgs) {
		t.Errorf("expected invocation arguments to contain '%s', got '%s'", expectedArgs, lines[0])
	}

	// Verify all criteria transitioned to complete
	for _, cid := range []string{crit1, crit2, crit3} {
		cObj, err := sp.Read(ctx, secCtx, cid)
		if err != nil {
			t.Fatalf("read %s: %v", cid, err)
		}
		if cObj[objects.FieldKeyStatus] != objects.ObjectStatusComplete {
			t.Errorf("expected %s to be complete, got %v", cid, cObj[objects.FieldKeyStatus])
		}
	}

	// Verify test case completed
	if !res.TestCaseCompleted {
		t.Errorf("expected test case to be marked completed")
	}
}

// GH-1003 / CRIT-1793942583212716000-f9db3acc: Negative Adversarial Boundary
// Verifies graceful fallback to isolated single-criterion execution upon batch failure.
func TestBatchCriteriaExecution_FallbackOnBatchFailure(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
	})
	sp := proj.FileStorage
	tempDir := proj.Root
	ctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()

	// Runner script behavior:
	// If called with more than 1 argument (batch mode), it exits 1 (fails batch).
	// If called with 1 argument:
	//   CRIT-FAIL-001 -> passes (exit 0)
	//   CRIT-FAIL-002 -> fails (exit 2)
	//   CRIT-FAIL-003 -> passes (exit 0)
	invocationsLog := filepath.Join(tempDir, "fallback_invocations.log")
	runnerScriptPath := filepath.Join(tempDir, "scripts", "run_fallback_tests.sh")
	if err := fileutil.MkdirAll(filepath.Dir(runnerScriptPath), 0755); err != nil {
		t.Fatalf("failed to create script dir: %v", err)
	}

	scriptContent := fmt.Sprintf(`#!/usr/bin/env bash
echo "$#" "$@" >> '%s'
if [ "$#" -gt 1 ]; then
    # Batch run fails
    exit 1
fi

case "$1" in
    *001) exit 0 ;;
    *002) exit 2 ;;
    *003) exit 0 ;;
    *) exit 1 ;;
esac
`, invocationsLog)

	if err := fileutil.WriteFile(runnerScriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to write runner script: %v", err)
	}

	goalID := "GOAL-FALLBACK-001"
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:          goalID,
		objects.FieldKeyKind:        objects.KindGoal,
		objects.FieldKeyTitle:       "Fallback Goal",
		objects.FieldKeyDescription: "Fallback Goal Description",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
	}); err != nil {
		t.Fatalf("create goal: %v", err)
	}

	crit1 := "CRIT-FAIL-001"
	crit2 := "CRIT-FAIL-002"
	crit3 := "CRIT-FAIL-003"
	for _, cid := range []string{crit1, crit2, crit3} {
		if err := sp.Create(ctx, secCtx, map[string]any{
			objects.FieldKeyID:          cid,
			objects.FieldKeyKind:        objects.KindCriteria,
			objects.FieldKeyTitle:       cid + " Title",
			objects.FieldKeyDescription: cid + " Description",
			objects.FieldKeyCategory:    "functional",
			objects.FieldKeyStatus:      objects.ObjectStatusAwaitingVerification,
		}); err != nil {
			t.Fatalf("create %s: %v", cid, err)
		}
	}

	reqID := "REQ-FALLBACK-001"
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:           reqID,
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyTitle:        "Fallback Requirement",
		objects.FieldKeyDescription:  "Fallback Requirement Description",
		objects.FieldKeyStatus:       objects.ObjectStatusActive,
		objects.FieldKeyGoalRefs:     []string{goalID},
		objects.FieldKeyCriteriaRefs: []string{crit1, crit2, crit3},
	}); err != nil {
		t.Fatalf("create req: %v", err)
	}

	tcID := "TST-FALLBACK-001"
	relScript := filepath.Join("scripts", "run_fallback_tests.sh")
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:              tcID,
		objects.FieldKeyKind:            objects.KindTestCase,
		objects.FieldKeyTitle:           "Fallback Test Case",
		objects.FieldKeyDescription:     "Fallback Test Case Description",
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
		objects.FieldKeyPathOrID:        relScript,
		objects.FieldKeyRequirementRefs: []string{reqID},
		objects.FieldKeyCriteriaRefs:    []string{crit1, crit2, crit3},
	}); err != nil {
		t.Fatalf("create test case: %v", err)
	}

	if err := lifecycle.SeedRemainingOpenCountFromMembers(ctx, sp, tcID, true); err != nil {
		t.Fatalf("seed remaining count: %v", err)
	}

	opts := testrunner.RunOptions{}
	res, err := testrunner.RunTestCase(ctx, sp, tempDir, tcID, opts)
	if err != nil {
		t.Fatalf("RunTestCase failed: %v", err)
	}

	// Verify fallback was triggered
	if !res.BatchedExecution {
		t.Errorf("expected BatchedExecution to be true")
	}
	if !res.FallbackTriggered {
		t.Errorf("expected FallbackTriggered to be true")
	}

	// Passed criteria should be 2, failed should be 1
	if res.PassedCriteria != 2 {
		t.Errorf("expected 2 passed criteria, got %d", res.PassedCriteria)
	}
	if res.FailedCriteria != 1 {
		t.Errorf("expected 1 failed criterion, got %d", res.FailedCriteria)
	}
	if res.Status != "partial" {
		t.Errorf("expected status 'partial', got %s", res.Status)
	}

	// Invocations log should have:
	// Line 1: batch pass with 3 args
	// Line 2: single pass for crit1
	// Line 3: single pass for crit2
	// Line 4: single pass for crit3
	logData, err := fileutil.ReadFile(invocationsLog)
	if err != nil {
		t.Fatalf("failed to read invocation log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(logData)), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 invocations (1 batch + 3 fallback), got %d: %v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "3 ") {
		t.Errorf("expected line 1 to be 3 args (batch), got: %s", lines[0])
	}
	if !strings.HasPrefix(lines[1], "1 ") || !strings.HasPrefix(lines[2], "1 ") || !strings.HasPrefix(lines[3], "1 ") {
		t.Errorf("expected lines 2-4 to be 1 arg (isolated), got: %v", lines[1:])
	}

	// Verify criteria states: crit1 and crit3 complete, crit2 still awaiting
	c1, _ := sp.Read(ctx, secCtx, crit1)
	if c1[objects.FieldKeyStatus] != objects.ObjectStatusComplete {
		t.Errorf("expected %s to be complete, got %v", crit1, c1[objects.FieldKeyStatus])
	}

	c2, _ := sp.Read(ctx, secCtx, crit2)
	if c2[objects.FieldKeyStatus] == objects.ObjectStatusComplete {
		t.Errorf("expected %s NOT to be complete, got %v", crit2, c2[objects.FieldKeyStatus])
	}

	c3, _ := sp.Read(ctx, secCtx, crit3)
	if c3[objects.FieldKeyStatus] != objects.ObjectStatusComplete {
		t.Errorf("expected %s to be complete, got %v", crit3, c3[objects.FieldKeyStatus])
	}

	// Verify test case is NOT complete
	if res.TestCaseCompleted {
		t.Errorf("expected test case NOT to be complete due to failing criterion")
	}
}

// Verifies NoBatch option skips batch pass and runs isolated single-criterion tests directly.
func TestBatchCriteriaExecution_NoBatchOption(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
	})
	sp := proj.FileStorage
	tempDir := proj.Root
	ctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()

	invocationsLog := filepath.Join(tempDir, "nobatch_invocations.log")
	runnerScriptPath := filepath.Join(tempDir, "scripts", "run_nobatch_tests.sh")
	if err := fileutil.MkdirAll(filepath.Dir(runnerScriptPath), 0755); err != nil {
		t.Fatalf("failed to create script dir: %v", err)
	}

	scriptContent := fmt.Sprintf(`#!/usr/bin/env bash
echo "$#" "$@" >> '%s'
exit 0
`, invocationsLog)

	if err := fileutil.WriteFile(runnerScriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to write runner script: %v", err)
	}

	crit1 := "CRIT-NOB-001"
	crit2 := "CRIT-NOB-002"
	for _, cid := range []string{crit1, crit2} {
		if err := sp.Create(ctx, secCtx, map[string]any{
			objects.FieldKeyID:          cid,
			objects.FieldKeyKind:        objects.KindCriteria,
			objects.FieldKeyTitle:       cid,
			objects.FieldKeyDescription: cid + " Description",
			objects.FieldKeyCategory:    "functional",
			objects.FieldKeyStatus:      objects.ObjectStatusAwaitingVerification,
		}); err != nil {
			t.Fatalf("create %s: %v", cid, err)
		}
	}

	tcID := "TST-NOB-001"
	relScript := filepath.Join("scripts", "run_nobatch_tests.sh")
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:           tcID,
		objects.FieldKeyKind:         objects.KindTestCase,
		objects.FieldKeyTitle:        "NoBatch Test Case",
		objects.FieldKeyStatus:       objects.ObjectStatusActive,
		objects.FieldKeyPathOrID:     relScript,
		objects.FieldKeyCriteriaRefs: []string{crit1, crit2},
	}); err != nil {
		t.Fatalf("create test case: %v", err)
	}

	opts := testrunner.RunOptions{
		NoBatch: true,
	}
	res, err := testrunner.RunTestCase(ctx, sp, tempDir, tcID, opts)
	if err != nil {
		t.Fatalf("RunTestCase failed: %v", err)
	}

	if res.BatchedExecution {
		t.Errorf("expected BatchedExecution to be false with NoBatch=true")
	}
	if res.PassedCriteria != 2 {
		t.Errorf("expected 2 passed criteria, got %d", res.PassedCriteria)
	}

	// Should have exactly 2 single-criterion invocations
	logData, err := fileutil.ReadFile(invocationsLog)
	if err != nil {
		t.Fatalf("failed to read invocation log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(logData)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 invocations, got %d: %v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "1 ") || !strings.HasPrefix(lines[1], "1 ") {
		t.Errorf("expected 1-arg invocations, got: %v", lines)
	}
}
