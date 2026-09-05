package scheduler

import (
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
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
	if err := sched.storage.Create(ctx, secCtx, jobData); err != nil {
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
