package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtended_CapStageGates_DeepCoverage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-cap-stage-gates-deep-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Setenv("ZQK_TEST_ROOT", tmpDir)
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
		_ = os.RemoveAll(tmpDir)
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	h := &CapOrchestratorHandler{
		storage:     sp,
		logger:      logger,
		projectRoot: tmpDir,
	}

	// 1. Directory and path helpers
	dir := h.capStateDir()
	if !strings.HasSuffix(dir, filepath.Join(paths.ProjectDataDir, paths.StateDir)) {
		t.Errorf("unexpected capStateDir: %s", dir)
	}
	p := h.capStatePath("test.json")
	if !strings.HasSuffix(p, "test.json") {
		t.Errorf("unexpected capStatePath: %s", p)
	}
	lp := h.legacyCapStatePath("test.json")
	if !strings.HasSuffix(lp, "test.json") {
		t.Errorf("unexpected legacyCapStatePath: %s", lp)
	}

	// 2. markStageEntered and readPendingStage
	h.markStageEntered("cap_stage_planning", "PRI-gate-1")
	pending, err := h.readPendingStage()
	if err != nil || pending.Stage != "cap_stage_planning" {
		t.Errorf("readPendingStage failed: %v, pending: %+v", err, pending)
	}
	if pending.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", pending.Attempts)
	}

	// Mark same stage entered again (should increment attempts)
	h.markStageEntered("cap_stage_planning", "PRI-gate-1")
	pending2, err := h.readPendingStage()
	if err != nil || pending2.Attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", pending2.Attempts)
	}

	// 3. Failure attempts tracking
	h.recordCAPStageFailureAttempt("cap_stage_planning")
	pendingFail, _ := h.readPendingStage()
	if pendingFail.FailureAttempts != 1 {
		t.Errorf("expected 1 failure attempt, got %d", pendingFail.FailureAttempts)
	}

	h.clearCAPStageFailureAttempts("cap_stage_planning")
	pendingClear, _ := h.readPendingStage()
	if pendingClear.FailureAttempts != 0 {
		t.Errorf("expected 0 failure attempts, got %d", pendingClear.FailureAttempts)
	}

	// 4. Quarantine operations
	h.quarantineCAPStage(pending2)
	if !h.capStageQuarantineActive(pending2) {
		t.Errorf("expected quarantine to be active")
	}

	ready := h.capStageQuarantineRetryReady(pending2, time.Now().Add(1*time.Hour))
	if !ready {
		t.Errorf("expected retry ready after 1 hour")
	}

	h.clearCAPStageQuarantine()
	if h.capStageQuarantineActive(pending2) {
		t.Errorf("expected quarantine to be inactive after clear")
	}

	// 5. Grooming planned zero latch
	h.clearGroomingPlannedZeroLatch()
	_, exh := h.groomingPlannedZeroExhausted(5)
	if exh {
		t.Errorf("expected false when planned > 0")
	}

	// 6. capStageAttemptExhausted helper
	pExh := capStagePending{FailureAttempts: 10}
	if !capStageAttemptExhausted(pExh) {
		t.Errorf("expected attempt exhausted for 10 failures")
	}
	pNotExh := capStagePending{FailureAttempts: 2}
	if capStageAttemptExhausted(pNotExh) {
		t.Errorf("expected attempt not exhausted for 2 failures")
	}
}

func TestExtended_ConvergenceTestBundle_DeepCoverage(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "test-convergence-bundle-deep-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Setenv("ZQK_TEST_ROOT", tmpDir)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// 1. Outcome classification helpers
	if !outcomeIsBad("test_fail") || !outcomeIsBad("fail") || !outcomeIsBad("timeout") || !outcomeIsBad("flake") {
		t.Errorf("outcomeIsBad failed for bad outcomes")
	}
	if outcomeIsBad("pass") || outcomeIsBad("completed") {
		t.Errorf("outcomeIsBad failed for good outcomes")
	}

	if !outcomeIsFlake("flake") || !outcomeIsFlake("quarantined_flake") {
		t.Errorf("outcomeIsFlake failed for flakes")
	}
	if outcomeIsFlake("pass") || outcomeIsFlake("fail") {
		t.Errorf("outcomeIsFlake failed for non-flakes")
	}

	// 2. parseSuggestedRerunCommands
	m := map[string]any{
		KeySuggestedRerunCommands: []any{"go test -run TestA", "go test -run TestB"},
	}
	sug := parseSuggestedRerunCommands(m)
	if len(sug) != 2 || sug[0] != "go test -run TestA" {
		t.Errorf("unexpected suggested reruns: %v", sug)
	}
	if parseSuggestedRerunCommands(map[string]any{}) != nil {
		t.Errorf("expected nil for empty map")
	}

	// 3. Shared tail readers with missing files
	_, err = ReadTestBundleHealthTailLines(ctx, tmpDir, 10)
	if err == nil {
		t.Errorf("expected error for non-existent health file")
	}
	_, err = ReadTestBundleEventsTailLines(ctx, tmpDir, 10)
	if err == nil {
		t.Errorf("expected error for non-existent events file")
	}
	_, err = ReadTestBundleProgressTailLines(ctx, tmpDir, 10)
	if err == nil {
		t.Errorf("expected error for non-existent progress file")
	}

	// 4. Create real health.jsonl file and test tail / scan
	healthPath := TestBundlesHealthFilePath(tmpDir)
	healthDir := filepath.Dir(healthPath)
	if err := fileutil.MkdirAll(healthDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	content := `{"job_id":"SCH-1","test_outcome":"pass","bundle_command_fingerprint":"fp1"}
{"job_id":"SCH-2","test_outcome":"fail","bundle_command_fingerprint":"fp2"}
`
	if err := fileutil.WriteFile(healthPath, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("write health file failed: %v", err)
	}

	tail, err := ReadTestBundleHealthTailLines(ctx, tmpDir, 10)
	if err != nil || len(tail) != 2 {
		t.Errorf("expected 2 lines in tail, got %d err %v", len(tail), err)
	}

	fullRows, isMissing, err := readTestBundleHealthJSONLFullScan(tmpDir, 100)
	if err != nil || isMissing || len(fullRows) != 2 {
		t.Errorf("expected 2 rows in full scan, got %d isMissing %v err %v", len(fullRows), isMissing, err)
	}
}
