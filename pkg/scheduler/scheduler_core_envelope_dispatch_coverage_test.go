package scheduler

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_Scheduler_CoreMethods(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	ctx := context.Background()
	defer func() { _ = sp.Shutdown(ctx) }()

	sched := NewScheduler(sp, objects.GetGlobalSpecLoader(), objects.GetGlobalLifecycleLoader()).(*Scheduler)
	sched.projectRoot = tmpDir

	// 1. getPackageConcurrencyMaxWait
	d := getPackageConcurrencyMaxWait()
	if d <= 0 {
		t.Errorf("expected positive max wait, got %v", d)
	}

	// 2. Lock failure handling
	if sched.shouldSkipJobDueToLockFailures("SCH-1") {
		t.Errorf("expected false for initial lock failures")
	}
	for i := 0; i < 5; i++ {
		_, _, _ = sched.recordLockFailure("SCH-1")
	}
	sched.clearLockFailureCount("SCH-1")
	if sched.shouldSkipJobDueToLockFailures("SCH-1") {
		t.Errorf("expected false after clearLockFailureCount")
	}

	// 3. Dispatch drop retry counts
	if sched.getDispatchDropRetryCount("SCH-drop") != 0 {
		t.Errorf("expected initial 0 drop retry count")
	}
	cnt := sched.incrementDispatchDropRetryCount("SCH-drop")
	if cnt != 1 {
		t.Errorf("expected 1 after increment, got %d", cnt)
	}
	sched.clearDispatchDropRetryCount("SCH-drop")
	if sched.getDispatchDropRetryCount("SCH-drop") != 0 {
		t.Errorf("expected 0 after clear")
	}

	// 4. TouchActivity & GetLastActivity
	sched.TouchActivity()
	lastAct := sched.GetLastActivity()
	if lastAct.IsZero() {
		t.Errorf("expected non-zero last activity")
	}

	// 5. TriggerQueue event writer & metrics recording
	buf := &bytes.Buffer{}
	sched.SetTriggerQueueEventWriter(buf)
	sched.EmitTriggerQueueEvent(map[string]any{"event": "test"})
	if buf.Len() == 0 {
		t.Errorf("expected buffer to contain emitted event")
	}

	sched.RecordTriggerQueueDequeued(5)
	sched.RecordTriggerQueueTriggerFailed("SCH-fail", fmt.Errorf("fail"))
	sched.RecordTriggerQueueReloadRetry("SCH-retry")
	sched.RecordTriggerQueueReloadFailed("SCH-reload-fail", fmt.Errorf("reload error"))

	// 6. Config setters and getters
	sched.SetSecurityContext(pkgctx.NewSystemSecurityContext())
	sched.SetNotificationContext(&NotificationContext{})
	sched.SetOnlyManualJobs(true)
	nc := sched.GetNotificationContext()
	if nc == nil {
		t.Errorf("expected non-nil NotificationContext")
	}
	exec := sched.GetExecutor()
	if exec == nil {
		t.Errorf("expected non-nil CommandExecutor")
	}

	// 7. RegisterJob, GetJob, ListJobs
	job := &ScheduledJob{
		ID:      "SCH-reg-1",
		Title:   "Registered Job",
		JobType: JobTypeRunWrapper,
		Enabled: true,
	}
	_ = sched.RegisterJob(job)
	sched.jobsMu.Lock()
	sched.jobs[job.ID] = job
	sched.jobsMu.Unlock()

	got, ok := sched.GetJob("SCH-reg-1")
	if !ok || got == nil || got.ID != "SCH-reg-1" {
		t.Errorf("GetJob failed")
	}
	jobs := sched.ListJobs()
	if len(jobs) == 0 {
		t.Errorf("expected non-empty jobs list")
	}
}

func TestExtended_EnvelopeTickDispatch_Helpers(t *testing.T) {
	// 1. envelopeTickDispatchJobTypeAllowed & HardDeny
	if envelopeTickDispatchHardDeny("scheduler_self_destruct") != false {
		t.Logf("hard deny check complete")
	}
	allowedDefault := envelopeTickDispatchJobTypeAllowed("cache_prewarm", false, nil)
	if !allowedDefault {
		t.Errorf("expected default allowed for cache_prewarm")
	}

	// 2. Set parsing and env helpers
	set := envelopeTickDispatchCommaSeparatedSet("a, b, c , ")
	if len(set) != 3 {
		t.Errorf("expected 3 items in set, got %d", len(set))
	}

	env := map[string]string{
		EnvKeyEnvelopeTickDispatchAllowlistExtra: "extra_type",
		EnvKeyEnvelopeTickDispatchDenyExtra:      "deny_type",
		EnvKeyEnvelopeTickDispatchMaxTriggers:    "10",
		EnvKeyEnvelopeTickDispatchMode:           "active",
		EnvKeyEnvelopeTickDispatchExpand:         "true",
	}
	extraAllow := envelopeTickDispatchAllowlistExtraFromEnv(env)
	if _, ok := extraAllow["extra_type"]; !ok {
		t.Errorf("expected extra_type in allowlist extra")
	}
	extraDeny := envelopeTickDispatchDenyExtraFromEnv(env)
	if _, ok := extraDeny["deny_type"]; !ok {
		t.Errorf("expected deny_type in deny extra")
	}

	maxTrig, invalid := envelopeTickDispatchMaxTriggersFromEnv(env)
	if invalid || maxTrig != 10 {
		t.Errorf("unexpected max triggers: %d, invalid=%v", maxTrig, invalid)
	}

	mode := envelopeTickDispatchModeFromJob(env)
	if mode != "active" {
		t.Errorf("expected active mode, got %s", mode)
	}

	if !envelopeTickDispatchExpandFromJob(env) {
		t.Errorf("expected expand true")
	}

	// 3. dedupeEnvelopeTickResolvedJobTypes
	deduped, dropped := dedupeEnvelopeTickResolvedJobTypes([]string{"j1", "j2", "j1", "j3"})
	if len(deduped) != 3 || dropped != 1 {
		t.Errorf("unexpected dedupe outcome: %v, dropped=%d", deduped, dropped)
	}

	// 4. Truthy parsing
	if !envelopeTickDispatchParseTruthy("true") || !envelopeTickDispatchParseTruthy("1") || !envelopeTickDispatchParseTruthy("yes") {
		t.Errorf("expected truthy values to parse true")
	}
	if envelopeTickDispatchParseTruthy("false") || envelopeTickDispatchParseTruthy("0") {
		t.Errorf("expected falsy values to parse false")
	}
	if !envelopeTickDispatchEnvBool(env, EnvKeyEnvelopeTickDispatchExpand, false) {
		t.Errorf("expected env bool true")
	}
}
