package testrunner_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/testrunner"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// GH-1003 / BLI-1793942683855970000-dbc844d5: Test Case Runner De-Duplication & Batch Criteria Execution
// Verifies that criteria sharing the same runner script execute in a single batch pass.
func TestBatchCriteriaExecution_SinglePassSuccess(t *testing.T) {
	scriptTmpl := "#!/usr/bin/env bash\necho \"$@\" >> '%s'\nexit 0\n"
	critIDs := []string{"CRIT-BATCH-001", "CRIT-BATCH-002", "CRIT-BATCH-003"}
	proj, logPath := setupBatchTestEnv(t, "run_batch_tests.sh", scriptTmpl, "GOAL-BATCH-001", "REQ-BATCH-001", "TST-BATCH-001", critIDs)

	res, err := testrunner.RunTestCase(context.Background(), proj.FileStorage, proj.Root, "TST-BATCH-001", testrunner.RunOptions{})
	if err != nil {
		t.Fatalf("RunTestCase failed: %v", err)
	}

	if !res.BatchedExecution || res.FallbackTriggered || res.PassedCriteria != 3 || res.FailedCriteria != 0 || res.Status != "passed" {
		t.Errorf("unexpected batch result: %+v", res)
	}

	assertBatchInvocationLog(t, logPath, 1, strings.Join(critIDs, " "))
	assertCriteriaState(t, proj.FileStorage, critIDs, objects.ObjectStatusComplete)
	if !res.TestCaseCompleted {
		t.Errorf("expected test case to be marked completed")
	}
}

// GH-1003 / CRIT-1793942583212716000-f9db3acc: Negative Adversarial Boundary
// Verifies graceful fallback to isolated single-criterion execution upon batch failure.
func TestBatchCriteriaExecution_FallbackOnBatchFailure(t *testing.T) {
	scriptTmpl := `#!/usr/bin/env bash
echo "$#" "$@" >> '%s'
if [ "$#" -gt 1 ]; then
    exit 1
fi
case "$1" in
    *001) exit 0 ;;
    *002) exit 2 ;;
    *003) exit 0 ;;
    *) exit 1 ;;
esac
`
	critIDs := []string{"CRIT-FAIL-001", "CRIT-FAIL-002", "CRIT-FAIL-003"}
	proj, logPath := setupBatchTestEnv(t, "run_fallback_tests.sh", scriptTmpl, "GOAL-FALLBACK-001", "REQ-FALLBACK-001", "TST-FALLBACK-001", critIDs)

	res, err := testrunner.RunTestCase(context.Background(), proj.FileStorage, proj.Root, "TST-FALLBACK-001", testrunner.RunOptions{})
	if err != nil {
		t.Fatalf("RunTestCase failed: %v", err)
	}

	if !res.BatchedExecution || !res.FallbackTriggered || res.PassedCriteria != 2 || res.FailedCriteria != 1 || res.Status != "partial" {
		t.Errorf("unexpected fallback result: %+v", res)
	}

	assertFallbackInvocationLog(t, logPath)
	assertCriteriaState(t, proj.FileStorage, []string{"CRIT-FAIL-001", "CRIT-FAIL-003"}, objects.ObjectStatusComplete)
	assertCriteriaState(t, proj.FileStorage, []string{"CRIT-FAIL-002"}, objects.ObjectStatusAwaitingVerification)
	if res.TestCaseCompleted {
		t.Errorf("expected test case NOT to be complete due to failing criterion")
	}
}

// Verifies NoBatch option skips batch pass and runs isolated single-criterion tests directly.
func TestBatchCriteriaExecution_NoBatchOption(t *testing.T) {
	scriptTmpl := "#!/usr/bin/env bash\necho \"$#\" \"$@\" >> '%s'\nexit 0\n"
	critIDs := []string{"CRIT-NOB-001", "CRIT-NOB-002"}
	proj, logPath := setupBatchTestEnv(t, "run_nobatch_tests.sh", scriptTmpl, "GOAL-NOB-001", "REQ-NOB-001", "TST-NOB-001", critIDs)

	res, err := testrunner.RunTestCase(context.Background(), proj.FileStorage, proj.Root, "TST-NOB-001", testrunner.RunOptions{NoBatch: true})
	if err != nil {
		t.Fatalf("RunTestCase failed: %v", err)
	}

	if res.BatchedExecution || res.PassedCriteria != 2 {
		t.Errorf("unexpected no-batch result: %+v", res)
	}

	assertNoBatchInvocationLog(t, logPath, 2)
}

// setupBatchTestEnv initializes an isolated test project with goals, requirements, criteria, and a runner script.
func setupBatchTestEnv(
	t *testing.T,
	scriptName, scriptTemplate, goalID, reqID, tcID string,
	critIDs []string,
) (testkit.IsolatedTempProject, string) {
	t.Helper()
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
	})
	sp := proj.FileStorage
	tempDir := proj.Root
	ctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()

	logPath := filepath.Join(tempDir, scriptName+".log")
	scriptPath := filepath.Join(tempDir, "scripts", scriptName)
	if err := fileutil.MkdirAll(filepath.Dir(scriptPath), 0755); err != nil {
		t.Fatalf("create script dir: %v", err)
	}
	content := fmt.Sprintf(scriptTemplate, logPath)
	if err := fileutil.WriteFile(scriptPath, []byte(content), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:          goalID,
		objects.FieldKeyKind:        objects.KindGoal,
		objects.FieldKeyTitle:       goalID + " Title",
		objects.FieldKeyDescription: goalID + " Description",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
	}); err != nil {
		t.Fatalf("create goal: %v", err)
	}

	for _, cid := range critIDs {
		if err := sp.Create(ctx, secCtx, map[string]any{
			objects.FieldKeyID:          cid,
			objects.FieldKeyKind:        objects.KindCriteria,
			objects.FieldKeyTitle:       cid + " Title",
			objects.FieldKeyDescription: cid + " Description",
			objects.FieldKeyCategory:    "functional",
			objects.FieldKeyStatus:      objects.ObjectStatusAwaitingVerification,
		}); err != nil {
			t.Fatalf("create criteria %s: %v", cid, err)
		}
	}

	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:           reqID,
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyTitle:        reqID + " Title",
		objects.FieldKeyDescription:  reqID + " Description",
		objects.FieldKeyStatus:       objects.ObjectStatusActive,
		objects.FieldKeyGoalRefs:     []string{goalID},
		objects.FieldKeyCriteriaRefs: critIDs,
	}); err != nil {
		t.Fatalf("create requirement: %v", err)
	}

	relScript := filepath.Join("scripts", scriptName)
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:              tcID,
		objects.FieldKeyKind:            objects.KindTestCase,
		objects.FieldKeyTitle:           tcID + " Title",
		objects.FieldKeyDescription:     tcID + " Description",
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
		objects.FieldKeyPathOrID:        relScript,
		objects.FieldKeyRequirementRefs: []string{reqID},
		objects.FieldKeyCriteriaRefs:    critIDs,
	}); err != nil {
		t.Fatalf("create test case: %v", err)
	}

	if err := lifecycle.SeedRemainingOpenCountFromMembers(ctx, sp, tcID, true); err != nil {
		t.Fatalf("seed remaining count: %v", err)
	}

	return proj, logPath
}

func assertBatchInvocationLog(t *testing.T, logPath string, expectedCount int, expectedArgs string) {
	t.Helper()
	data, err := fileutil.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read invocation log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != expectedCount {
		t.Fatalf("expected %d invocations, got %d: %v", expectedCount, len(lines), lines)
	}
	if !strings.Contains(lines[0], expectedArgs) {
		t.Errorf("expected '%s' in line, got: %s", expectedArgs, lines[0])
	}
}

func assertFallbackInvocationLog(t *testing.T, logPath string) {
	t.Helper()
	data, err := fileutil.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fallback log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 invocations (1 batch + 3 fallback), got %d: %v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "3 ") {
		t.Errorf("expected batch 3 args, got: %s", lines[0])
	}
	for i := 1; i <= 3; i++ {
		if !strings.HasPrefix(lines[i], "1 ") {
			t.Errorf("expected line %d to be 1 arg, got: %s", i+1, lines[i])
		}
	}
}

func assertNoBatchInvocationLog(t *testing.T, logPath string, expectedCount int) {
	t.Helper()
	data, err := fileutil.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read nobatch log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != expectedCount {
		t.Fatalf("expected %d invocations, got %d: %v", expectedCount, len(lines), lines)
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, "1 ") {
			t.Errorf("expected 1-arg line, got: %s", l)
		}
	}
}

func assertCriteriaState(t *testing.T, sp storage.ObjectStorageProvider, critIDs []string, expectedStatus string) {
	t.Helper()
	secCtx := pkgctx.NewSystemSecurityContext()
	for _, cid := range critIDs {
		cObj, err := sp.Read(context.Background(), secCtx, cid)
		if err != nil {
			t.Fatalf("read %s: %v", cid, err)
		}
		if cObj[objects.FieldKeyStatus] != expectedStatus {
			t.Errorf("expected %s to have status %s, got %v", cid, expectedStatus, cObj[objects.FieldKeyStatus])
		}
	}
}
