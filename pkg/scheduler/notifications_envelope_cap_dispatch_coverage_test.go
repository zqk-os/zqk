package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

func TestExtended_NotificationsAndEnvelope(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ndIface := NewNotificationDisplay(logger)
	nd := ndIface.(*NotificationDisplay)

	// 1. Notification Display paths
	for _, pri := range []NotificationPriority{PriorityCritical, PriorityHigh, PriorityMedium, PriorityLow, NotificationPriority("unknown")} {
		notif := CreateJobNotification("SCH-1", "test", "maintenance", "completed", pri, 2*time.Second, nil, map[string]any{"is_test_failure": true})
		nd.Display(notif)
		nd.DisplayTerminalNotification(notif)
		nd.DisplayDesktopNotification(notif)
	}

	// Error notification
	notifErr := CreateJobNotification("SCH-1", "test", "maintenance", "failed", PriorityCritical, 5*time.Second, errors.New("sample fail"), nil)
	nd.Display(notifErr)

	// isTerminal helper
	_ = isTerminal(nil)

	// 2. Envelope Tick Dispatch helpers
	if !envelopeTickDispatchJobTypeAllowed(JobTypeCachePrewarm, false, nil) {
		t.Errorf("expected cache_prewarm to be allowed")
	}
	if envelopeTickDispatchJobTypeAllowed("disallowed_job_type", false, nil) {
		t.Errorf("expected disallowed_job_type to be false")
	}

	extraMap := map[string]struct{}{"custom_job": {}}
	if !envelopeTickDispatchJobTypeAllowed("custom_job", false, extraMap) {
		t.Errorf("expected custom_job to be allowed via extraAllow")
	}

	set := envelopeTickDispatchCommaSeparatedSet("job_a, job_b , ")
	if len(set) != 2 {
		t.Errorf("expected 2 elements in set, got %d", len(set))
	}

	envExtra := map[string]string{
		EnvKeyEnvelopeTickDispatchAllowlistExtra: "job_c, job_d",
		EnvKeyEnvelopeTickDispatchDenyExtra:      "job_e",
		EnvKeyEnvelopeTickDispatchMaxTriggers:    "5",
		EnvKeyEnvelopeTickDispatchMode:           "parallel",
		EnvKeyEnvelopeTickDispatchExpand:         "true",
	}
	allowSet := envelopeTickDispatchAllowlistExtraFromEnv(envExtra)
	if len(allowSet) != 2 {
		t.Errorf("expected 2 in allowSet")
	}
	denySet := envelopeTickDispatchDenyExtraFromEnv(envExtra)
	if len(denySet) != 1 {
		t.Errorf("expected 1 in denySet")
	}

	maxTrig, invalid := envelopeTickDispatchMaxTriggersFromEnv(envExtra)
	if maxTrig != 5 || invalid {
		t.Errorf("expected maxTriggers 5, invalid false, got %d, %v", maxTrig, invalid)
	}
	_, invalidBad := envelopeTickDispatchMaxTriggersFromEnv(map[string]string{EnvKeyEnvelopeTickDispatchMaxTriggers: "not_an_int"})
	if !invalidBad {
		t.Errorf("expected invalidBad true")
	}

	if !envelopeTickDispatchHardDeny(JobTypeDataCellEnvelopeTick) {
		t.Errorf("expected hard deny for envelope")
	}
	if envelopeTickDispatchHardDeny("run_wrapper") {
		t.Errorf("expected false for run_wrapper")
	}

	mode := envelopeTickDispatchModeFromJob(envExtra)
	if mode != "parallel" {
		t.Errorf("expected parallel mode, got %s", mode)
	}

	deduped, dropped := dedupeEnvelopeTickResolvedJobTypes([]string{"a", "b", "a", "c", "b"})
	if len(deduped) != 3 || dropped != 2 {
		t.Errorf("expected 3 deduped and 2 dropped, got %v, %d", deduped, dropped)
	}

	if !envelopeTickDispatchParseTruthy("true") || !envelopeTickDispatchParseTruthy("1") || !envelopeTickDispatchParseTruthy("yes") {
		t.Errorf("expected truthy values")
	}
	if envelopeTickDispatchParseTruthy("false") || envelopeTickDispatchParseTruthy("0") {
		t.Errorf("expected falsy values")
	}

	if !envelopeTickDispatchEnvBool(envExtra, EnvKeyEnvelopeTickDispatchExpand, false) {
		t.Errorf("expected true from env")
	}
	if !envelopeTickDispatchExpandFromJob(envExtra) {
		t.Errorf("expected true from env for expand")
	}
}

func TestExtended_CapDispatchAndRunWrapperExecution(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	handler := NewCapOrchestratorHandler(sp, tmpDir, logger).(*CapOrchestratorHandler)

	ctx := context.Background()

	// 1. mintAgentTaskID
	taskID := mintAgentTaskID()
	if len(taskID) == 0 {
		t.Errorf("expected non-empty task ID")
	}

	// 2. capDispatchPlanIDs
	plans := []whatsnext.WhatsNextPriorityPlan{
		{ID: "PRI-2"},
		{ID: "PRI-3"},
	}
	pids := capDispatchPlanIDs("PRI-1", plans)
	if len(pids) != 3 || pids[0] != "PRI-1" {
		t.Errorf("unexpected capDispatchPlanIDs: %v", pids)
	}

	// 3. decodeCAPPlanOutput
	validJSON := []byte(`{"id":"PRI-1","title":"Plan 1"}`)
	var dst map[string]any
	err = decodeCAPPlanOutput(validJSON, &dst)
	if err != nil || dst["id"] != "PRI-1" {
		t.Errorf("unexpected decodeCAPPlanOutput: %v, err=%v", dst, err)
	}
	err = decodeCAPPlanOutput([]byte("invalid json"), &dst)
	if err == nil {
		t.Errorf("expected error for invalid json")
	}

	// 4. nestPlanWorkstreams
	workstreams := handler.nestPlanWorkstreams(ctx, "PRI-1", map[string]any{"id": "PRI-1"})
	t.Logf("nestPlanWorkstreams: %v", workstreams)

	// 5. RunWrapperExecution helpers
	// stripShellOutputRedirect
	s1, r1 := stripShellOutputRedirect("echo hello > out.txt 2>&1")
	if r1 != "out.txt" || s1 != "echo hello" {
		t.Errorf("unexpected stripShellOutputRedirect: %q, %q", s1, r1)
	}
	s2, r2 := stripShellOutputRedirect("echo hello >> out.log 2>&1")
	if r2 != "out.log" || s2 != "echo hello" {
		t.Errorf("unexpected stripShellOutputRedirect >>: %q, %q", s2, r2)
	}
	s3, r3 := stripShellOutputRedirect("echo hello")
	if r3 != "" || s3 != "echo hello" {
		t.Errorf("unexpected stripShellOutputRedirect normal: %q, %q", s3, r3)
	}

	// copyStreamFilesToRedirectTarget
	copyStreamFilesToRedirectTarget(tmpDir, tmpDir, "SCH-job-1", "/dev/null")

	// writeSeparateJobLogsIfConfigured
	writeSeparateJobLogsIfConfigured(tmpDir, "SCH-job-1", "stdout data", "stderr data")

	// prepareRunWrapperExecution & runWrapperHandlePanic
	rwHandlerIface := NewRunWrapperHandlerWithProjectRoot(sp, logger, nil, nil, tmpDir)
	rwHandler := rwHandlerIface.(*RunWrapperHandler)
	defer rwHandler.StopNotificationContext()

	prep := rwHandler.prepareRunWrapperExecution(&ScheduledJob{
		ID:          "SCH-rw-prep",
		Command:     "echo",
		CommandArgs: []string{"test"},
	})
	if prep.CmdStr == "" {
		t.Errorf("expected non-empty CmdStr in prep")
	}

	rwHandler.warnIfTestBundleMetadataFingerprintMismatch(&ScheduledJob{ID: "SCH-rw-prep"}, "echo test")
	rwHandler.logRunWrapperExecutionStart(&ScheduledJob{ID: "SCH-rw-prep"}, prep)

	panicErr := rwHandler.runWrapperHandlePanic(&ScheduledJob{ID: "SCH-rw-prep"}, "sample panic", []byte("stack trace"))
	if panicErr == nil {
		t.Errorf("expected non-nil error from runWrapperHandlePanic")
	}
}
