package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtended_AuditAggregationSession_DeepCoverage(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	registerFileStorageTestTeardown(t, tmpDir, sp)
	defer func() {
		_ = sp.Shutdown(context.Background())
	}()

	handler := NewAuditAggregationHandlerWithProjectRoot(sp, tmpDir).(*AuditAggregationHandler)

	ctx := context.Background()
	job := &ScheduledJob{
		ID:      "SCH-audit-agg-1",
		JobType: JobTypeAuditEventAggregation,
		EnvironmentVariables: map[string]string{
			EnvKeyBatchSize:          "50",
			EnvKeyAggregationWindow:  "30m",
			EnvKeyRetentionDuration:  "2h",
			EnvKeyArchiveEnabled:     "true",
			EnvKeyDeleteEnabled:      "false",
			"MAX_RUNTIME_SECONDS":    "600",
		},
		MaxRuntimeSeconds: 600,
	}

	session := &auditAggregationSession{
		handler: handler,
		ctx:     ctx,
		job:     job,
	}

	// 1. Phases
	session.startPhase("init")
	time.Sleep(2 * time.Millisecond)
	session.endPhase("init")
	if session.phaseDurations["init"] <= 0 {
		t.Errorf("expected phaseDuration to be positive")
	}

	// 2. Load configuration
	if err := session.loadConfiguration(); err != nil {
		t.Fatalf("failed to load configuration: %v", err)
	}
	if session.batchSize != 50 {
		t.Errorf("expected batch size 50, got %d", session.batchSize)
	}
	if session.windowDuration != 30*time.Minute {
		t.Errorf("expected 30m window, got %v", session.windowDuration)
	}
	if session.deleteAfterDuration != 2*time.Hour {
		t.Errorf("expected 2h delete after, got %v", session.deleteAfterDuration)
	}
	if !session.archiveEnabled || session.deleteEnabled {
		t.Errorf("unexpected archive/delete settings: archive=%v, delete=%v", session.archiveEnabled, session.deleteEnabled)
	}

	// 3. Setup phase contexts (no deadline)
	session.setupPhaseContexts()
	if !session.hasTimeRemaining() {
		t.Errorf("expected time remaining")
	}

	// 4. Setup phase contexts with deadline
	ctxDeadline, cancel := context.WithDeadline(ctx, time.Now().Add(5*time.Minute))
	defer cancel()
	sessionDeadline := &auditAggregationSession{
		handler: handler,
		ctx:     ctxDeadline,
		job:     job,
	}
	_ = sessionDeadline.loadConfiguration()
	sessionDeadline.setupPhaseContexts()
	defer sessionDeadline.cleanup()

	// 5. Run pre checks
	_, _ = session.runPreChecks()
	session.cleanup()

	// 6. determineEffectiveRetention
	session.deleteAfterDuration = 2 * time.Hour
	session.determineEffectiveRetention()
	if session.effectiveRetention != 2*time.Hour {
		t.Errorf("expected 2h effective retention for empty storage, got %v", session.effectiveRetention)
	}

	// 7. Retention first pass
	session.setupPhaseContexts()
	session.runRetentionFirstPass()

	// Cancelled preAggCtx
	session.preAggCancel()
	session.runRetentionFirstPass()

	// 8. Catch up
	session.setupPhaseContexts()
	session.runCatchUp()
	session.runAggressiveCleanup()
	session.runProactiveCleanup()

	// 9. Second pass and finalize
	session.runRetentionSecondPass()
	res := &storagepkg.AuditAggregationResult{
		EventCount:     5,
		MetricsCreated: 1,
		MetricID:       "MET-1",
	}
	session.finalize(res)
}

func TestExtended_CapOrchestrator_DeepCoverage(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	registerFileStorageTestTeardown(t, tmpDir, sp)
	defer func() {
		_ = sp.Shutdown(context.Background())
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	handler := NewCapOrchestratorHandler(sp, tmpDir, logger).(*CapOrchestratorHandler)

	ctx := context.Background()

	// 1. prepareCmd
	cmd := exec.CommandContext(ctx, "echo", "hello")
	cmd = handler.prepareCmd(cmd)
	foundSysAccount := false
	for _, env := range cmd.Env {
		if env == "ZQK_API_KEY="+objects.DefaultSystemAccountID {
			foundSysAccount = true
		}
	}
	if !foundSysAccount {
		t.Errorf("expected default system account in cmd env")
	}

	// 2. buildAgentProvider and buildHumanProvider with escalation config
	agentRuntimeDir := filepath.Join(tmpDir, paths.ProjectDataDir, "agent-runtime")
	_ = fileutil.MkdirAll(agentRuntimeDir, 0755)
	escConfig := map[string]any{
		"agent_command": "true",
		"agent_args":    []string{"--agent"},
		"human_command": "true",
		"human_args":    []string{"--human"},
	}
	rawEsc, _ := json.Marshal(escConfig)
	_ = os.WriteFile(filepath.Join(agentRuntimeDir, "escalation.json"), rawEsc, 0644)

	ap := buildAgentProvider(tmpDir)
	if ap == nil {
		t.Errorf("expected non-nil agent provider")
	}

	hp := buildHumanProvider(tmpDir)
	if hp == nil {
		t.Errorf("expected non-nil human provider")
	}

	// 3. Failure tracker lifecycle
	tracker := handler.readFailureTracker()
	if tracker.ConsecutiveFailures != 0 {
		t.Errorf("expected 0 initial failures")
	}

	handler.recordFailure("test_stage", fmt.Errorf("test error 1"))
	tracker = handler.readFailureTracker()
	if tracker.ConsecutiveFailures != 1 || tracker.LastError != "test error 1" {
		t.Errorf("unexpected tracker state after failure 1: %+v", tracker)
	}

	handler.recordFailure("test_stage", fmt.Errorf("test error 2"))
	tracker = handler.readFailureTracker()
	if tracker.ConsecutiveFailures != 2 {
		t.Errorf("expected 2 failures")
	}

	handler.clearFailures()
	tracker = handler.readFailureTracker()
	if tracker.ConsecutiveFailures != 0 {
		t.Errorf("expected 0 failures after clear")
	}

	// 4. Wake agent & hourglass
	handler.wakeAgentAndScheduleHourglass("ATK-wake-1", "software_engineer")
	handler.wakeAgentAndScheduleHourglass("ATK-wake-tpm", "tpm")

	// 6. resolveWakePlanID & topOpenPlanBLIs
	planID := handler.resolveWakePlanID(ctx, "ATK-none")
	t.Logf("resolveWakePlanID returned: %s", planID)

	blis := handler.topOpenPlanBLIs(ctx, planID, 3)
	t.Logf("topOpenPlanBLIs returned: %d items", len(blis))

	// 7. Direct methods on CapOrchestratorHandler
	_, _ = handler.resolveCLIExecutable()
	passed, hardErrs, softErrs := handler.verifyCriticalPackagesHealth(ctx, nil)
	t.Logf("verifyCriticalPackagesHealth: passed=%v hard=%v soft=%v", passed, hardErrs, softErrs)

	_ = handler.autoRecoverPlanTasks(ctx, "PRI-none")

	instIdx := handler.buildOpenAgentInstructionIndex(ctx)
	if instIdx == nil {
		t.Errorf("expected non-nil instruction index")
	}
	hasInst := handler.hasOpenAgentInstruction(ctx, "PRI-none", "tpm", "test")
	t.Logf("hasOpenAgentInstruction: %v", hasInst)

	taskIdx := handler.buildOpenAgentTaskIndex(ctx)
	if taskIdx == nil {
		t.Errorf("expected non-nil task index")
	}
	hasTask := handler.hasOpenTaskForPlan(ctx, "PRI-none", "ATK-none")
	t.Logf("hasOpenTaskForPlan: %v", hasTask)

	// Execute with missing CLI executable error path
	_ = handler.Execute(ctx, &ScheduledJob{ID: "SCH-cap"})
}
