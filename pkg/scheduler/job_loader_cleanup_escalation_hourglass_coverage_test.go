package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtended_JobLoader_Wave52(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	defer func() { _ = sp.Shutdown(ctx) }()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()
	jl := NewJobLoader(sp, secCtx, logger, nil)

	// 1. rawJobPriority
	if rawJobPriority(map[string]any{"priority": "high"}) != "high" {
		t.Errorf("expected high priority")
	}
	if rawJobPriority(map[string]any{}) != "normal" {
		t.Errorf("expected normal priority default")
	}

	// 2. HydrateJob & parser helpers
	rawJob := map[string]any{
		objects.FieldKeyID:            "SCH-hydrate-test",
		objects.FieldKeyKind:          objects.KindSchedulerJob,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyJobType:       JobTypeRunWrapper,
		"trigger_type":                "manual",
		"execution_mode":              "reusable",
		"command":                     "echo hello",
		"command_args":                []any{"hello"},
		"environment_variables": map[string]any{
			"VAR1": "val1",
		},
		"callback_on_completion": "http://example.com/ok",
		"callback_on_error":      "http://example.com/err",
	}

	job, hErr := jl.HydrateJob(rawJob)
	if hErr != nil || job == nil {
		t.Errorf("HydrateJob failed: %v", hErr)
	}

	jl.RefreshEnvironmentVariablesFromRaw(job, rawJob)
}

func TestExtended_CleanupConfig_InternalHandlers_Wave52(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	h := NewCleanupConfigHandler(tmpDir, logger).(*CleanupConfigHandler)

	// 1. Param extraction helpers
	m := map[string]any{
		"str":   "value",
		"slice": []any{"a", "b"},
		"int":   42,
	}
	if strParam(m, "str") != "value" || strParam(m, "missing") != "" {
		t.Errorf("unexpected strParam")
	}
	if len(strSliceParam(m, "slice")) != 2 {
		t.Errorf("unexpected strSliceParam")
	}
	if intParam(m, "int") != 42 || intParam(m, "missing") != 0 {
		t.Errorf("unexpected intParam")
	}

	d, _ := parseDuration("10m")
	if d != 10*time.Minute {
		t.Errorf("unexpected parseDuration: %v", d)
	}

	// 2. File and lock cleanup handlers
	testFile := filepath.Join(tmpDir, "cleanup_test.txt")
	_ = fileutil.WriteFile(testFile, []byte("content to truncate or delete"), 0600)

	_ = h.runTruncateFiles(tmpDir, map[string]any{"paths": []any{testFile}, "max_bytes": 5})
	_ = h.runDeleteFiles(tmpDir, map[string]any{"paths": []any{testFile}})
	_ = h.runReapStaleLocks(tmpDir, map[string]any{"stale_age": "0s"})
	_ = h.runReapTempFiles(tmpDir, map[string]any{"stale_age": "0s"})
	_ = h.runEnforceLogRetention(tmpDir, map[string]any{"retention": "1s"})
}

func TestExtended_EscalationProviders_Wave52(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	notice := EscalationNotice{
		Title:            "Cap Stage Review Failed",
		Severity:         EscalationSeverityCritical,
		Issues:           []string{"verification tests failed repeatedly"},
		SuggestedActions: []string{"check logs", "rerun"},
	}

	// 1. Notice prompt
	prompt := notice.buildPrompt()
	if len(prompt) == 0 {
		t.Errorf("expected non-empty prompt")
	}

	// 2. Providers
	inboxProv := &InboxEscalationProvider{
		InboxDir:   tmpDir,
		MaxHistory: 10,
	}
	_ = inboxProv.Escalate(ctx, notice)
	inboxProv.pruneHistory()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	webProv := &WebhookEscalationProvider{
		WebhookURL: server.URL,
	}
	_ = webProv.Escalate(ctx, notice)

	cmdProv := &CommandEscalationProvider{
		Command: "true",
	}
	_ = cmdProv.Escalate(ctx, notice)

	macProv := &MacOSNotificationProvider{}
	_ = macProv.Escalate(ctx, notice)

	multiProv := &MultiEscalationProvider{
		Providers: []EscalationProvider{inboxProv, webProv},
	}
	_ = multiProv.Escalate(ctx, notice)

	// 3. Slack webhook resolver
	_ = ResolveSlackWebhookURL(tmpDir)

	// 4. EscalationChain Evaluate
	chain := &EscalationChain{
		Tiers: []EscalationTier{
			{
				Threshold: 1,
				Provider:  inboxProv,
				Name:      "inbox",
			},
		},
	}
	_ = chain.Evaluate(ctx, 2, notice)
}

func TestExtended_Hourglass_Wave52(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	defer func() { _ = sp.Shutdown(ctx) }()

	secCtx := pkgctx.NewSystemSecurityContext()
	sched := NewScheduler(sp, objects.GetGlobalSpecLoader(), objects.GetGlobalLifecycleLoader()).(*Scheduler)

	// 1. Static helpers
	_ = actionForExpiredTimer("checkin")
	_ = actionForExpiredTimer("deadline")
	_ = actionForExpiredTimer("unknown")

	if !refsContainID([]any{"ID1", "ID2"}, "ID1") || refsContainID([]any{"ID1"}, "ID3") || refsContainID(nil, "ID1") {
		t.Errorf("unexpected refsContainID behavior")
	}

	blockerID := newMissedDeadlineRiskBlockerID()
	if len(blockerID) == 0 {
		t.Errorf("expected non-empty blockerID")
	}

	blockerObj, bErr := buildMissedDeadlineRiskBlocker(blockerID, "ATK-task-1", "agent_task", "Task Title")
	if bErr != nil || blockerObj == nil {
		t.Errorf("buildMissedDeadlineRiskBlocker failed: %v", bErr)
	}

	_ = hourglassSourcePresent(ctx, sp, secCtx, "ATK-task-1")
	_ = hasOpenMissedDeadlineEscalation(ctx, sp, secCtx, "ATK-task-1")

	// 2. Scheduler methods
	sched.checkHourglassTimers(ctx)
	sched.sweepStaleAgentTasks(ctx)
	sched.handleMissedCheckin(ctx, secCtx, filepath.Join(tmpDir, "missing_checkin.json"))
	sched.recordSilentClaimBlocker(ctx, secCtx, "ATK-task-1")
	sched.escalateMissedDeadline(ctx, secCtx, "ATK-task-1", "agent_task", "Task Title")
}
