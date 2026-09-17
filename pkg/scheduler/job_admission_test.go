package scheduler

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/validation"
)

func TestOneTimeImmediateShouldStartOnReload(t *testing.T) {
	t.Parallel()
	now := time.Now()
	cases := []struct {
		name    string
		job     *ScheduledJob
		pending bool
		want    bool
	}{
		{
			name: "cli_submit_manual",
			job: &ScheduledJob{
				ID:            "SCH-1788056998365283000",
				ExecutionMode: ExecutionModeOneTime,
				TriggerType:   TriggerTypeImmediate,
				Category:      CategoryManual,
				Priority:      JobPriorityHigh,
				Enabled:       true,
			},
			want: true,
		},
		{
			name: "callback_bearing",
			job: &ScheduledJob{
				ID:              "SCH-1788056998365283001",
				ExecutionMode:   ExecutionModeOneTime,
				TriggerType:     TriggerTypeImmediate,
				Category:        CategoryUser,
				CallbackOnError: "true",
				Enabled:         true,
			},
			want: true,
		},
		{
			name: "pending_already_queued",
			job: &ScheduledJob{
				ID:            "SCH-1788056998365283002",
				ExecutionMode: ExecutionModeOneTime,
				TriggerType:   TriggerTypeImmediate,
				Category:      CategoryManual,
			},
			pending: true,
			want:    false,
		},
		{
			name: "already_started",
			job: &ScheduledJob{
				ID:            "SCH-1788056998365283003",
				ExecutionMode: ExecutionModeOneTime,
				TriggerType:   TriggerTypeImmediate,
				Category:      CategoryManual,
				LastRunAt:     &now,
			},
			want: false,
		},
		{
			name: "disabled_spent",
			job: &ScheduledJob{
				ID:            "SCH-1788056998365283004",
				ExecutionMode: ExecutionModeOneTime,
				TriggerType:   TriggerTypeImmediate,
				Category:      CategoryManual,
				Enabled:       false,
			},
			want: false,
		},
		{
			name: "test_bundle",
			job: &ScheduledJob{
				ID:            "SCH-run-bundle-pkg-objects",
				ExecutionMode: ExecutionModeOneTime,
				TriggerType:   TriggerTypeImmediate,
				Category:      CategoryTesting,
			},
			want: false,
		},
		{
			name: "timer_job",
			job: &ScheduledJob{
				ID:            "SCH-cache-prewarm",
				ExecutionMode: ExecutionModeReusable,
				TriggerType:   TriggerTypeTimer,
				Category:      CategoryMaintenance,
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := oneTimeImmediateShouldStartOnReload(tc.job, tc.pending); got != tc.want {
				t.Errorf("oneTimeImmediateShouldStartOnReload = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestJobIDLooksLikeCLISubmitNanos(t *testing.T) {
	t.Parallel()
	if !jobIDLooksLikeCLISubmitNanos("SCH-1788056998365283000") {
		t.Fatal("19-digit unix nano submit id should match CLI submit shape")
	}
	if jobIDLooksLikeCrossProcessTimestampTestRunner("SCH-1788056998365283000") {
		t.Fatal("19-digit unix nano submit id must not match 10-digit test-runner shape")
	}
	if jobIDLooksLikeCLISubmitNanos("SCH-1774181824") {
		t.Fatal("10-digit unix seconds must not match CLI nano shape")
	}
}

func TestScanAdmissionTimeoutsDisablesAndMarksFailed(t *testing.T) {
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		setAdmissionTimeoutForTest(0)
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second)
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	jobID := "SCH-1788056998365283999"
	created := time.Now().UTC().Add(-2 * time.Minute)
	errPath := filepath.Join(t.TempDir(), "admission.err")
	jobData := map[string]any{
		objects.FieldKeyID:                jobID,
		objects.FieldKeyKind:              objects.KindSchedulerJob,
		objects.FieldKeyTitle:             "Admission hourglass fixture",
		objects.FieldKeyStatus:            objects.ObjectStatusActive,
		objects.FieldKeyJobType:           JobTypeRunWrapper,
		objects.FieldKeyTriggerType:       TriggerTypeImmediate,
		objects.FieldKeyCategory:          CategoryManual,
		objects.FieldKeyPriority:          JobPriorityHigh,
		objects.FieldKeyExecutionMode:     ExecutionModeOneTime,
		objects.FieldKeyMaxRuntimeSeconds: 60,
		objects.FieldKeyEnabled:           true,
		objects.FieldKeyCommand:           "true",
		objects.FieldKeyCallbackOnError:   "touch " + errPath,
		objects.FieldKeyCreatedAt:         created.Format(time.RFC3339),
		objects.FieldKeyCreatedBy:         "ACC-TEST",
		objects.FieldKeyUpdatedAt:         created.Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:         "ACC-TEST",
		objects.FieldKeyOriginProject:     validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:      validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
	}
	if err := sched.storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, jobData); err != nil {
		t.Fatalf("create fixture job: %v", err)
	}

	job := &ScheduledJob{
		ID:              jobID,
		JobType:         JobTypeRunWrapper,
		Category:        CategoryManual,
		Priority:        JobPriorityHigh,
		TriggerType:     TriggerTypeImmediate,
		ExecutionMode:   ExecutionModeOneTime,
		Enabled:         true,
		Command:         "true",
		CallbackOnError: "touch " + errPath,
		CreatedAt:       created,
	}
	sched.jobsMu.Lock()
	sched.jobs[jobID] = job
	sched.jobsMu.Unlock()
	if sched.logger == nil {
		sched.logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}

	setAdmissionTimeoutForTest(50 * time.Millisecond)
	sched.scanAdmissionTimeouts(ctx)

	if !sched.admissionAlreadyFailed(jobID) {
		t.Fatal("expected admissionFailed after timeout scan")
	}
	raw, err := sched.storage.Read(ctx, secCtx, jobID)
	if err != nil {
		t.Fatalf("read after admission fail: %v", err)
	}
	if enabled, _ := raw[objects.FieldKeyEnabled].(bool); enabled {
		t.Fatal("expected job disabled after admission timeout")
	}
}

func TestScanAdmissionTimeoutsSkipsPendingDispatch(t *testing.T) {
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)
	t.Cleanup(func() { setAdmissionTimeoutForTest(0) })

	jobID := "SCH-1788059947969029001"
	job := &ScheduledJob{
		ID:              jobID,
		JobType:         JobTypeRunWrapper,
		Category:        CategoryManual,
		Priority:        JobPriorityHigh,
		TriggerType:     TriggerTypeImmediate,
		ExecutionMode:   ExecutionModeOneTime,
		Enabled:         true,
		CallbackOnError: "true",
		CreatedAt:       time.Now().UTC().Add(-2 * time.Minute),
	}
	sched.jobsMu.Lock()
	sched.jobs[jobID] = job
	sched.jobsMu.Unlock()
	sched.markOneTimeImmediatePending(job)

	setAdmissionTimeoutForTest(50 * time.Millisecond)
	sched.scanAdmissionTimeouts(pkgctx.NewSystemContext())

	if sched.admissionAlreadyFailed(jobID) {
		t.Fatal("admission timeout must not fire for a job already accepted onto the pool")
	}
	if !job.Enabled {
		t.Fatal("pending job must stay enabled")
	}
}

func TestTryScheduleJob_TestBundleStaysRegisterOnlyOnBoot(t *testing.T) {
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)

	job := &ScheduledJob{
		ID:            "SCH-run-bundle-test-sample",
		JobType:       JobTypeRunWrapper,
		Category:      CategoryTesting,
		TriggerType:   TriggerTypeImmediate,
		ExecutionMode: ExecutionModeOneTime,
		Enabled:       true,
		Status:        StatusActive,
		Command:       "echo test",
	}

	h := hydratedJob{job: job}
	result := map[string]any{
		objects.FieldKeyID:            job.ID,
		objects.FieldKeyCategory:      CategoryTesting,
		objects.FieldKeyTriggerType:   TriggerTypeImmediate,
		objects.FieldKeyExecutionMode: ExecutionModeOneTime,
		objects.FieldKeyEnabled:       true,
	}

	// On boot (reload=false), test bundle should be registered in s.jobs via registerTriggeredJob
	// and NOT submitted to the triggered pool (which would fail if triggeredPool is nil).
	sched.jobsMu.Lock()
	scheduledJob, err := sched.tryScheduleJob(result, h, false)
	sched.jobsMu.Unlock()

	if err != nil {
		t.Fatalf("tryScheduleJob failed on boot for test bundle: %v", err)
	}
	if scheduledJob == nil {
		t.Fatal("expected test bundle to be registered, got nil")
	}
	if scheduledJob.ID != job.ID {
		t.Fatalf("expected job ID %s, got %s", job.ID, scheduledJob.ID)
	}
}

func TestFailJobAdmission_DisablesJobInStorage(t *testing.T) {
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second)
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	jobID := "SCH-FAIL-ADM-001"
	created := time.Now().UTC()
	jobData := map[string]any{
		objects.FieldKeyID:                jobID,
		objects.FieldKeyKind:              objects.KindSchedulerJob,
		objects.FieldKeyTitle:             "Fail admission fixture",
		objects.FieldKeyStatus:            objects.ObjectStatusActive,
		objects.FieldKeyJobType:           JobTypeRunWrapper,
		objects.FieldKeyTriggerType:       TriggerTypeImmediate,
		objects.FieldKeyCategory:          CategoryManual,
		objects.FieldKeyPriority:          JobPriorityHigh,
		objects.FieldKeyExecutionMode:     ExecutionModeOneTime,
		objects.FieldKeyMaxRuntimeSeconds: 60,
		objects.FieldKeyEnabled:           true,
		objects.FieldKeyCommand:           "echo test",
		objects.FieldKeyCreatedAt:         created.Format(time.RFC3339),
		objects.FieldKeyCreatedBy:         "ACC-TEST",
		objects.FieldKeyUpdatedAt:         created.Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:         "ACC-TEST",
		objects.FieldKeyOriginProject:     validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:      validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
	}
	if err := sched.storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, jobData); err != nil {
		t.Fatalf("create fixture job: %v", err)
	}

	job := &ScheduledJob{
		ID:            jobID,
		JobType:       JobTypeRunWrapper,
		Category:      CategoryManual,
		Priority:      JobPriorityHigh,
		TriggerType:   TriggerTypeImmediate,
		ExecutionMode: ExecutionModeOneTime,
		Enabled:       true,
		Command:       "echo test",
		CreatedAt:     created,
	}
	sched.jobsMu.Lock()
	sched.jobs[jobID] = job
	sched.jobsMu.Unlock()
	if sched.logger == nil {
		sched.logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}

	// Call FailJobAdmission
	sched.FailJobAdmission(ctx, jobID, "not_in_cache_after_retries")

	if job.Enabled {
		t.Errorf("expected in-memory job.Enabled to be false, got true")
	}

	raw, err := sched.storage.Read(ctx, secCtx, jobID)
	if err != nil {
		t.Fatalf("read after FailJobAdmission: %v", err)
	}
	if enabled, _ := raw[objects.FieldKeyEnabled].(bool); enabled {
		t.Errorf("expected storage job.Enabled to be false, got true")
	}
}

func TestFailJobAdmission_DisablesWhenNotInCache(t *testing.T) {
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second)
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	jobID := "SCH-FAIL-ADM-NOT-IN-CACHE"
	created := time.Now().UTC()
	jobData := map[string]any{
		objects.FieldKeyID:                jobID,
		objects.FieldKeyKind:              objects.KindSchedulerJob,
		objects.FieldKeyTitle:             "Fail admission not in cache fixture",
		objects.FieldKeyStatus:            objects.ObjectStatusActive,
		objects.FieldKeyJobType:           JobTypeRunWrapper,
		objects.FieldKeyTriggerType:       TriggerTypeImmediate,
		objects.FieldKeyCategory:          CategoryManual,
		objects.FieldKeyPriority:          JobPriorityHigh,
		objects.FieldKeyExecutionMode:     ExecutionModeOneTime,
		objects.FieldKeyMaxRuntimeSeconds: 60,
		objects.FieldKeyEnabled:           true,
		objects.FieldKeyCommand:           "echo test",
		objects.FieldKeyCreatedAt:         created.Format(time.RFC3339),
		objects.FieldKeyCreatedBy:         "ACC-TEST",
		objects.FieldKeyUpdatedAt:         created.Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:         "ACC-TEST",
		objects.FieldKeyOriginProject:     validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:      validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
	}
	if err := sched.storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, jobData); err != nil {
		t.Fatalf("create fixture job: %v", err)
	}
	// Note: NOT added to sched.jobs (not in cache)
	if sched.logger == nil {
		sched.logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}

	sched.FailJobAdmission(ctx, jobID, "not_in_cache_after_retries")

	raw, err := sched.storage.Read(ctx, secCtx, jobID)
	if err != nil {
		t.Fatalf("read after FailJobAdmission: %v", err)
	}
	if enabled, _ := raw[objects.FieldKeyEnabled].(bool); enabled {
		t.Errorf("expected storage job.Enabled to be false, got true")
	}
}

// TRACK: BLI-SCHED-ADMIT-STORAGE-001 / CRIT-SCHED-ADMIT-STORAGE-001 / CRIT-SCHED-TRIGGER-QUEUE-FALLBACK-001
func TestAdmitJobFromStorage_PopulatesCacheWhenNotListed(t *testing.T) {
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second)
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	jobID := "SCH-ADMIT-FROM-STORAGE-001"
	created := time.Now().UTC()
	jobData := map[string]any{
		objects.FieldKeyID:                jobID,
		objects.FieldKeyKind:              objects.KindSchedulerJob,
		objects.FieldKeyTitle:             "Admit from storage fixture",
		objects.FieldKeyStatus:            objects.ObjectStatusActive,
		objects.FieldKeyJobType:           JobTypeRunWrapper,
		objects.FieldKeyTriggerType:       TriggerTypeImmediate,
		objects.FieldKeyCategory:          CategoryManual,
		objects.FieldKeyPriority:          JobPriorityHigh,
		objects.FieldKeyExecutionMode:     ExecutionModeOneTime,
		objects.FieldKeyMaxRuntimeSeconds: 60,
		objects.FieldKeyEnabled:           true,
		objects.FieldKeyCommand:           "echo test",
		objects.FieldKeyCreatedAt:         created.Format(time.RFC3339),
		objects.FieldKeyCreatedBy:         "ACC-TEST",
		objects.FieldKeyUpdatedAt:         created.Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:         "ACC-TEST",
		objects.FieldKeyOriginProject:     validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:      validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
	}
	if err := sched.storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, jobData); err != nil {
		t.Fatalf("create fixture job: %v", err)
	}
	if sched.logger == nil {
		sched.logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	if sched.JobInCache(jobID) {
		t.Fatal("fixture must start absent from s.jobs (List lag simulation)")
	}
	if !sched.AdmitJobFromStorage(ctx, jobID) {
		t.Fatal("AdmitJobFromStorage: expected true when storage.Read succeeds")
	}
	if !sched.JobInCache(jobID) {
		t.Fatal("expected job in cache after AdmitJobFromStorage")
	}
}

func TestAdmitJobFromStorage_SkipsDisabledAndMissing(t *testing.T) {
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second)
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	jobID := "SCH-ADMIT-DISABLED-001"
	created := time.Now().UTC()
	jobData := map[string]any{
		objects.FieldKeyID:                jobID,
		objects.FieldKeyKind:              objects.KindSchedulerJob,
		objects.FieldKeyTitle:             "Disabled admit fixture",
		objects.FieldKeyStatus:            objects.ObjectStatusActive,
		objects.FieldKeyJobType:           JobTypeRunWrapper,
		objects.FieldKeyTriggerType:       TriggerTypeImmediate,
		objects.FieldKeyCategory:          CategoryManual,
		objects.FieldKeyPriority:          JobPriorityHigh,
		objects.FieldKeyExecutionMode:     ExecutionModeOneTime,
		objects.FieldKeyMaxRuntimeSeconds: 60,
		objects.FieldKeyEnabled:           false,
		objects.FieldKeyCommand:           "echo test",
		objects.FieldKeyCreatedAt:         created.Format(time.RFC3339),
		objects.FieldKeyCreatedBy:         "ACC-TEST",
		objects.FieldKeyUpdatedAt:         created.Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:         "ACC-TEST",
		objects.FieldKeyOriginProject:     validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:      validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
	}
	if err := sched.storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, jobData); err != nil {
		t.Fatalf("create fixture job: %v", err)
	}
	if sched.logger == nil {
		sched.logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	if sched.AdmitJobFromStorage(ctx, jobID) {
		t.Fatal("disabled job must not be admitted")
	}
	if sched.AdmitJobFromStorage(ctx, "SCH-ADMIT-MISSING-001") {
		t.Fatal("missing job must not be admitted")
	}
}

func TestTryScheduleJob_OneTimeImmediateAlreadyRunDoesNotReRunOnBoot(t *testing.T) {
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)

	now := time.Now().UTC()
	job := &ScheduledJob{
		ID:            "SCH-ONETIME-ALREADY-RUN",
		Category:      CategoryManual,
		TriggerType:   TriggerTypeImmediate,
		ExecutionMode: ExecutionModeOneTime,
		Enabled:       true,
		LastRunAt:     &now,
		Status:        StatusActive,
		Command:       "echo test",
	}

	h := hydratedJob{job: job}
	result := map[string]any{
		objects.FieldKeyID:            job.ID,
		objects.FieldKeyCategory:      CategoryManual,
		objects.FieldKeyTriggerType:   TriggerTypeImmediate,
		objects.FieldKeyExecutionMode: ExecutionModeOneTime,
		objects.FieldKeyEnabled:       true,
	}

	// On boot (reload=false), one-time job that already ran should be register-only
	// and NOT submitted to the triggered pool.
	sched.jobsMu.Lock()
	scheduledJob, err := sched.tryScheduleJob(result, h, false)
	sched.jobsMu.Unlock()

	if err != nil {
		t.Fatalf("tryScheduleJob failed on boot for already run job: %v", err)
	}
	if scheduledJob == nil {
		t.Fatal("expected already run job to be registered, got nil")
	}
}

type mockJobHandler struct {
	executeFunc func(context.Context, *ScheduledJob) error
}

func (m *mockJobHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, job)
	}
	return nil
}

func TestOneTimeJobDisabledAfterExecutionFailure(t *testing.T) {
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	jobID := "SCH-1788059947969029998"
	created := time.Now().UTC()
	jobData := map[string]any{
		objects.FieldKeyID:            jobID,
		objects.FieldKeyKind:          objects.KindSchedulerJob,
		objects.FieldKeyTitle:         "One-time failure fixture",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyJobType:       JobTypeRunWrapper,
		objects.FieldKeyTriggerType:   TriggerTypeImmediate,
		objects.FieldKeyCategory:      CategoryManual,
		objects.FieldKeyPriority:      JobPriorityHigh,
		objects.FieldKeyExecutionMode: ExecutionModeOneTime,
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyCreatedAt:     created.Format(time.RFC3339),
		objects.FieldKeyCreatedBy:     "ACC-TEST",
		objects.FieldKeyUpdatedAt:     created.Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:     "ACC-TEST",
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	if err := sched.storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, jobData); err != nil {
		t.Fatalf("create fixture job: %v", err)
	}

	job := &ScheduledJob{
		ID:            jobID,
		JobType:       JobTypeRunWrapper,
		Category:      CategoryManual,
		Priority:      JobPriorityHigh,
		TriggerType:   TriggerTypeImmediate,
		ExecutionMode: ExecutionModeOneTime,
		Enabled:       true,
		CreatedAt:     created,
	}
	sched.jobsMu.Lock()
	sched.jobs[jobID] = job
	sched.jobsMu.Unlock()

	failingHandler := &mockJobHandler{
		executeFunc: func(context.Context, *ScheduledJob) error {
			return errfmt.Errorf("simulated non-zero exit")
		},
	}

	sched.executeJob(ctx, job, failingHandler)

	if job.Enabled {
		t.Fatal("expected one-time job in-memory struct to be disabled after non-zero exit")
	}

	sched.jobsMu.RLock()
	cachedJob := sched.jobs[jobID]
	sched.jobsMu.RUnlock()
	if cachedJob != nil && cachedJob.Enabled {
		t.Fatal("expected cached job in s.jobs to be disabled after non-zero exit")
	}

	raw, err := sched.storage.Read(ctx, secCtx, jobID)
	if err != nil {
		t.Fatalf("read job from storage: %v", err)
	}
	if enabled, _ := raw[objects.FieldKeyEnabled].(bool); enabled {
		t.Fatal("expected one-time job in storage to be enabled=false after non-zero exit")
	}
	if status, _ := raw[objects.FieldKeyStatus].(string); status != StatusDisabled {
		t.Fatalf("expected one-time job in storage to have status=%q after non-zero exit, got %q", StatusDisabled, status)
	}
	if job.Status != StatusDisabled {
		t.Fatalf("expected in-memory job.Status to be %q, got %q", StatusDisabled, job.Status)
	}
	if !IsSchedulerJobMarkedForDeletion(raw) {
		t.Fatal("expected one-time job to be marked for deletion so LoadJobs excludes it")
	}

	// Recycle check: should reload/recycle re-fire?
	if oneTimeImmediateShouldStartOnReload(job, false) {
		t.Fatal("recycle must not re-fire disabled one_time job")
	}
}

func TestOneTimeJobDisabledAfterFailJobAdmission(t *testing.T) {
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	jobID := "SCH-1788059947969029997"
	created := time.Now().UTC()
	jobData := map[string]any{
		objects.FieldKeyID:            jobID,
		objects.FieldKeyKind:          objects.KindSchedulerJob,
		objects.FieldKeyTitle:         "Admission fail fixture",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyJobType:       JobTypeRunWrapper,
		objects.FieldKeyTriggerType:   TriggerTypeImmediate,
		objects.FieldKeyCategory:      CategoryManual,
		objects.FieldKeyPriority:      JobPriorityHigh,
		objects.FieldKeyExecutionMode: ExecutionModeOneTime,
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyCreatedAt:     created.Format(time.RFC3339),
		objects.FieldKeyCreatedBy:     "ACC-TEST",
		objects.FieldKeyUpdatedAt:     created.Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:     "ACC-TEST",
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	if err := sched.storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, jobData); err != nil {
		t.Fatalf("create fixture job: %v", err)
	}

	job := &ScheduledJob{
		ID:            jobID,
		JobType:       JobTypeRunWrapper,
		Category:      CategoryManual,
		Priority:      JobPriorityHigh,
		TriggerType:   TriggerTypeImmediate,
		ExecutionMode: ExecutionModeOneTime,
		Enabled:       true,
		CreatedAt:     created,
	}
	sched.jobsMu.Lock()
	sched.jobs[jobID] = job
	sched.jobsMu.Unlock()

	sched.FailJobAdmission(ctx, jobID, "cache_miss_retry_exhausted")

	if job.Enabled {
		t.Fatal("expected one-time job in-memory struct to be disabled after FailJobAdmission")
	}

	sched.jobsMu.RLock()
	cachedJob := sched.jobs[jobID]
	sched.jobsMu.RUnlock()
	if cachedJob != nil && cachedJob.Enabled {
		t.Fatal("expected cached job in s.jobs to be disabled after FailJobAdmission")
	}

	raw, err := sched.storage.Read(ctx, secCtx, jobID)
	if err != nil {
		t.Fatalf("read job from storage: %v", err)
	}
	if enabled, _ := raw[objects.FieldKeyEnabled].(bool); enabled {
		t.Fatal("expected one-time job in storage to be enabled=false after FailJobAdmission")
	}
	if status, _ := raw[objects.FieldKeyStatus].(string); status != StatusDisabled {
		t.Fatalf("expected one-time job in storage to have status=%q after FailJobAdmission, got %q", StatusDisabled, status)
	}
	if job.Status != StatusDisabled {
		t.Fatalf("expected in-memory job.Status to be %q, got %q", StatusDisabled, job.Status)
	}
	if !IsSchedulerJobMarkedForDeletion(raw) {
		t.Fatal("expected one-time job to be marked for deletion so LoadJobs excludes it")
	}

	// Recycle check: should reload/recycle re-fire?
	if oneTimeImmediateShouldStartOnReload(job, false) {
		t.Fatal("recycle must not re-fire disabled one_time job")
	}
}
