package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestSchedulerCycle31_Static_JobAdmission_ValidateJobEnvelope_NilAndEmpty(t *testing.T) {
	t.Parallel()

	errNil := ValidateJobEnvelope(nil)
	require.Error(t, errNil)
	assert.True(t, errors.Is(errNil, ErrNilJobEnvelope))

	errEmptyID := ValidateJobEnvelope(&ScheduledJob{
		ID:          "",
		TriggerType: TriggerTypeManual,
	})
	require.Error(t, errEmptyID)
	assert.True(t, errors.Is(errEmptyID, ErrEmptyJobID))

	errWhitespaceID := ValidateJobEnvelope(&ScheduledJob{
		ID:          "   \t\n  ",
		TriggerType: TriggerTypeManual,
	})
	require.Error(t, errWhitespaceID)
	assert.True(t, errors.Is(errWhitespaceID, ErrEmptyJobID))
}

func TestSchedulerCycle31_Static_JobAdmission_ValidateJobEnvelope_PriorityBounds(t *testing.T) {
	t.Parallel()

	validPriorities := []string{"", JobPriorityNormal, JobPriorityHigh, JobPriorityCritical}
	for _, p := range validPriorities {
		job := &ScheduledJob{
			ID:          "SCH-test-prio-" + p,
			TriggerType: TriggerTypeManual,
			Priority:    p,
		}
		err := ValidateJobEnvelope(job)
		assert.NoError(t, err, "priority %q should be valid", p)
	}

	invalidPriorities := []string{"urgent", "super_high", "LOW", "critical_priority", "1"}
	for _, p := range invalidPriorities {
		job := &ScheduledJob{
			ID:          "SCH-test-invalid-prio",
			TriggerType: TriggerTypeManual,
			Priority:    p,
		}
		err := ValidateJobEnvelope(job)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidJobPriority), "expected ErrInvalidJobPriority for %q", p)
	}
}

func TestSchedulerCycle31_Static_JobAdmission_ValidateJobEnvelope_TriggerOptions(t *testing.T) {
	t.Parallel()

	emptyTriggerJob := &ScheduledJob{ID: "SCH-test-empty-trigger"}
	errEmptyTrigger := ValidateJobEnvelope(emptyTriggerJob)
	require.Error(t, errEmptyTrigger)
	assert.True(t, errors.Is(errEmptyTrigger, ErrInvalidTriggerType))

	bogusTriggerJob := &ScheduledJob{ID: "SCH-test-bogus", TriggerType: "unsupported_trigger"}
	errBogus := ValidateJobEnvelope(bogusTriggerJob)
	require.Error(t, errBogus)
	assert.True(t, errors.Is(errBogus, ErrInvalidTriggerType))

	workflowMissing := &ScheduledJob{ID: "SCH-wf-1", TriggerType: TriggerTypeWorkflow}
	errWfMissing := ValidateJobEnvelope(workflowMissing)
	require.Error(t, errWfMissing)
	assert.True(t, errors.Is(errWfMissing, ErrMissingScheduleOption))

	workflowValid := &ScheduledJob{ID: "SCH-wf-2", TriggerType: TriggerTypeWorkflow, WorkflowRef: "WF-001"}
	require.NoError(t, ValidateJobEnvelope(workflowValid))

	eventMissing := &ScheduledJob{ID: "SCH-ev-1", TriggerType: TriggerTypeEvent}
	errEvMissing := ValidateJobEnvelope(eventMissing)
	require.Error(t, errEvMissing)
	assert.True(t, errors.Is(errEvMissing, ErrMissingScheduleOption))

	eventValid := &ScheduledJob{ID: "SCH-ev-2", TriggerType: TriggerTypeEvent, EventFilter: "type == commit"}
	require.NoError(t, ValidateJobEnvelope(eventValid))

	lifecycleMissing := &ScheduledJob{ID: "SCH-lc-1", TriggerType: TriggerTypeLifecycle}
	errLcMissing := ValidateJobEnvelope(lifecycleMissing)
	require.Error(t, errLcMissing)
	assert.True(t, errors.Is(errLcMissing, ErrMissingScheduleOption))

	lifecycleValid := &ScheduledJob{ID: "SCH-lc-2", TriggerType: TriggerTypeLifecycle, LifecycleFilter: "post_boot"}
	require.NoError(t, ValidateJobEnvelope(lifecycleValid))
}

func TestSchedulerCycle31_Static_JobAdmission_ValidateJobEnvelope_CronParsing(t *testing.T) {
	t.Parallel()

	timerMissingExpr := &ScheduledJob{ID: "SCH-timer-empty", TriggerType: TriggerTypeTimer}
	errMissing := ValidateJobEnvelope(timerMissingExpr)
	require.Error(t, errMissing)
	assert.True(t, errors.Is(errMissing, ErrMissingScheduleOption))

	timerInvalidExpr := &ScheduledJob{ID: "SCH-timer-bad", TriggerType: TriggerTypeTimer, ScheduleExpr: "not-a-cron"}
	errBad := ValidateJobEnvelope(timerInvalidExpr)
	require.Error(t, errBad)
	assert.True(t, errors.Is(errBad, ErrInvalidCronSchedule))

	timerValid5Field := &ScheduledJob{ID: "SCH-timer-5f", TriggerType: TriggerTypeTimer, ScheduleExpr: "*/5 * * * *"}
	require.NoError(t, ValidateJobEnvelope(timerValid5Field))

	timerValid6Field := &ScheduledJob{ID: "SCH-timer-6f", TriggerType: TriggerTypeTimer, ScheduleExpr: "0 */10 * * * *"}
	require.NoError(t, ValidateJobEnvelope(timerValid6Field))

	timerValidDescriptor := &ScheduledJob{ID: "SCH-timer-desc", TriggerType: TriggerTypeTimer, ScheduleExpr: "@hourly"}
	require.NoError(t, ValidateJobEnvelope(timerValidDescriptor))
}

func TestSchedulerCycle31_Static_JobAdmission_ValidateJobEnvelope_ExecutionModes(t *testing.T) {
	t.Parallel()

	validModes := []string{"", ExecutionModeReusable, ExecutionModeOneTime}
	for _, m := range validModes {
		job := &ScheduledJob{
			ID:            "SCH-mode-" + m,
			TriggerType:   TriggerTypeManual,
			ExecutionMode: m,
		}
		assert.NoError(t, ValidateJobEnvelope(job), "execution mode %q should be valid", m)
	}

	invalidModes := []string{"daemon", "continuous", "infinite", "batch"}
	for _, m := range invalidModes {
		job := &ScheduledJob{
			ID:            "SCH-mode-bad",
			TriggerType:   TriggerTypeManual,
			ExecutionMode: m,
		}
		err := ValidateJobEnvelope(job)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidExecutionMode), "expected ErrInvalidExecutionMode for %q", m)
	}
}

func TestSchedulerCycle31_Static_JobAdmission_Pipeline_FilterChaining(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	pipeline := NewJobAdmissionPipeline()
	require.NotNil(t, pipeline)
	assert.Equal(t, 0, pipeline.FilterCount())

	// Nil filter add is safe
	pipeline.AddFilter(nil)
	assert.Equal(t, 0, pipeline.FilterCount())

	filter1Ran := false
	pipeline.AddFilter(func(_ context.Context, job *ScheduledJob) error {
		filter1Ran = true
		return nil
	})
	assert.Equal(t, 1, pipeline.FilterCount())

	filter2Ran := false
	pipeline.AddFilter(func(_ context.Context, job *ScheduledJob) error {
		filter2Ran = true
		return nil
	})
	assert.Equal(t, 2, pipeline.FilterCount())

	job := &ScheduledJob{ID: "SCH-chain-1", TriggerType: TriggerTypeManual}
	err := pipeline.Admit(ctx, job)
	require.NoError(t, err)
	assert.True(t, filter1Ran)
	assert.True(t, filter2Ran)
}

func TestSchedulerCycle31_Static_JobAdmission_Pipeline_EarlyExit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	firstRan := false
	secondRan := false

	expectedErr := errors.New("custom filter rejection")
	pipeline := NewJobAdmissionPipeline(
		func(_ context.Context, _ *ScheduledJob) error {
			firstRan = true
			return expectedErr
		},
		func(_ context.Context, _ *ScheduledJob) error {
			secondRan = true
			return nil
		},
	)
	assert.Equal(t, 2, pipeline.FilterCount())

	job := &ScheduledJob{ID: "SCH-exit-early", TriggerType: TriggerTypeManual}
	err := pipeline.Admit(ctx, job)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrAdmissionFilterRejected))
	assert.True(t, firstRan)
	assert.False(t, secondRan, "subsequent filter must not run after rejection")
}

func TestSchedulerCycle31_Static_JobAdmission_BuiltInFilters_PriorityAndMode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	prioFilter := AdmissionFilterRequirePriority(JobPriorityHigh, JobPriorityCritical)
	assert.Error(t, prioFilter(ctx, nil))

	normalJob := &ScheduledJob{ID: "SCH-p1", TriggerType: TriggerTypeManual, Priority: JobPriorityNormal}
	highJob := &ScheduledJob{ID: "SCH-p2", TriggerType: TriggerTypeManual, Priority: JobPriorityHigh}
	assert.Error(t, prioFilter(ctx, normalJob))
	assert.NoError(t, prioFilter(ctx, highJob))

	modeFilter := AdmissionFilterExecutionMode(ExecutionModeOneTime)
	assert.Error(t, modeFilter(ctx, nil))

	reusableJob := &ScheduledJob{ID: "SCH-m1", TriggerType: TriggerTypeManual, ExecutionMode: ExecutionModeReusable}
	oneTimeJob := &ScheduledJob{ID: "SCH-m2", TriggerType: TriggerTypeManual, ExecutionMode: ExecutionModeOneTime}
	assert.Error(t, modeFilter(ctx, reusableJob))
	assert.NoError(t, modeFilter(ctx, oneTimeJob))
}

func TestSchedulerCycle31_Static_JobAdmission_BuiltInFilters_StatusAndEnabled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	nonArchivedFilter := AdmissionFilterNonArchived()
	assert.Error(t, nonArchivedFilter(ctx, nil))

	archivedJob := &ScheduledJob{ID: "SCH-a1", Status: objects.ObjectStatusArchived}
	activeJob := &ScheduledJob{ID: "SCH-a2", Status: objects.ObjectStatusActive}
	assert.Error(t, nonArchivedFilter(ctx, archivedJob))
	assert.NoError(t, nonArchivedFilter(ctx, activeJob))

	enabledFilter := AdmissionFilterEnabled()
	assert.Error(t, enabledFilter(ctx, nil))

	disabledJob1 := &ScheduledJob{ID: "SCH-d1", Enabled: false}
	disabledJob2 := &ScheduledJob{ID: "SCH-d2", Enabled: true, Status: StatusDisabled}
	enabledJob := &ScheduledJob{ID: "SCH-e1", Enabled: true, Status: objects.ObjectStatusActive}

	assert.Error(t, enabledFilter(ctx, disabledJob1))
	assert.Error(t, enabledFilter(ctx, disabledJob2))
	assert.NoError(t, enabledFilter(ctx, enabledJob))
}

func TestSchedulerCycle31_Static_JobAdmission_Predicates_ReloadAndHourglass(t *testing.T) {
	t.Parallel()
	now := time.Now()

	// 1. oneTimeImmediateShouldStartOnReload
	assert.False(t, oneTimeImmediateShouldStartOnReload(nil, false))
	assert.False(t, oneTimeImmediateShouldStartOnReload(&ScheduledJob{ExecutionMode: ExecutionModeOneTime}, true))
	assert.False(t, oneTimeImmediateShouldStartOnReload(&ScheduledJob{
		ExecutionMode: ExecutionModeOneTime,
		LastRunAt:     &now,
	}, false))

	validReloadJob := &ScheduledJob{
		ID:            "SCH-reload-ok",
		ExecutionMode: ExecutionModeOneTime,
		TriggerType:   TriggerTypeImmediate,
		Category:      CategoryManual,
		Enabled:       true,
	}
	assert.True(t, oneTimeImmediateShouldStartOnReload(validReloadJob, false))

	// 2. jobWarrantsAdmissionHourglass
	assert.False(t, jobWarrantsAdmissionHourglass(nil))
	assert.False(t, jobWarrantsAdmissionHourglass(&ScheduledJob{Enabled: false}))

	validHourglassJob := &ScheduledJob{
		ID:              "SCH-hg-ok",
		ExecutionMode:   ExecutionModeOneTime,
		TriggerType:     TriggerTypeImmediate,
		Enabled:         true,
		CallbackOnError: "cmd:notify",
	}
	assert.True(t, jobWarrantsAdmissionHourglass(validHourglassJob))
}

func TestSchedulerCycle31_Static_JobAdmission_SchedulerMethods_AdmissionTracking(t *testing.T) {
	t.Parallel()

	// Safe against nil receiver
	var nilScheduler *Scheduler
	nilScheduler.markOneTimeImmediatePending(nil)
	assert.False(t, nilScheduler.isImmediateDispatchPending("SCH-1"))
	nilScheduler.clearImmediateDispatchPending("SCH-1")
	assert.False(t, nilScheduler.markAdmissionFailed("SCH-1"))
	assert.False(t, nilScheduler.admissionAlreadyFailed("SCH-1"))
	assert.False(t, nilScheduler.isJobOccupied(nil))
	assert.False(t, nilScheduler.AdmitJobFromStorage(context.Background(), "SCH-1"))

	sched := &Scheduler{
		jobs: make(map[string]*ScheduledJob),
	}

	// Immediate dispatch tracking
	testJob := &ScheduledJob{ID: "SCH-track-1", ExecutionMode: ExecutionModeOneTime, TriggerType: TriggerTypeImmediate}
	sched.markOneTimeImmediatePending(testJob)
	assert.True(t, sched.isImmediateDispatchPending(testJob.ID))
	sched.clearImmediateDispatchPending(testJob.ID)
	assert.False(t, sched.isImmediateDispatchPending(testJob.ID))

	// Admission failed tracking
	firstMark := sched.markAdmissionFailed("SCH-fail-1")
	assert.True(t, firstMark)
	secondMark := sched.markAdmissionFailed("SCH-fail-1")
	assert.False(t, secondMark, "duplicate mark must return false")
	assert.True(t, sched.admissionAlreadyFailed("SCH-fail-1"))
}

func TestSchedulerCycle31_Static_JobRegistry_RegisterAndDuplicateRejection(t *testing.T) {
	t.Parallel()

	// Nil registry handling
	var nilReg *JobRegistry
	errNilReg := nilReg.Register(&ScheduledJob{ID: "SCH-1"})
	require.Error(t, errNilReg)

	reg := NewJobRegistry()
	require.NotNil(t, reg)
	assert.Equal(t, 0, reg.Count())

	// Reject nil job
	errNilJob := reg.Register(nil)
	require.Error(t, errNilJob)
	assert.True(t, errors.Is(errNilJob, ErrNilJobEnvelope))

	// Reject empty ID
	errEmptyID := reg.Register(&ScheduledJob{ID: ""})
	require.Error(t, errEmptyID)
	assert.True(t, errors.Is(errEmptyID, ErrEmptyJobID))

	// Register valid job
	job1 := &ScheduledJob{ID: "SCH-reg-1", TriggerType: TriggerTypeManual, Title: "Job 1"}
	require.NoError(t, reg.Register(job1))
	assert.Equal(t, 1, reg.Count())

	// Duplicate registration rejection
	errDup := reg.Register(job1)
	require.Error(t, errDup)
	assert.True(t, errors.Is(errDup, ErrJobAlreadyRegistered))
	assert.Equal(t, 1, reg.Count())
}

func TestSchedulerCycle31_Static_JobRegistry_UnregisterAndLookup(t *testing.T) {
	t.Parallel()

	var nilReg *JobRegistry
	require.Error(t, nilReg.Unregister("SCH-1"))
	j, found := nilReg.Lookup("SCH-1")
	assert.Nil(t, j)
	assert.False(t, found)

	reg := NewJobRegistry()
	job1 := &ScheduledJob{ID: "SCH-reg-1", TriggerType: TriggerTypeManual}
	job2 := &ScheduledJob{ID: "SCH-reg-2", TriggerType: TriggerTypeManual}
	require.NoError(t, reg.Register(job1))
	require.NoError(t, reg.Register(job2))

	// Lookup existing
	lookedUp, ok := reg.Lookup("SCH-reg-1")
	assert.True(t, ok)
	assert.Equal(t, job1.ID, lookedUp.ID)

	// Lookup missing & empty
	_, okMissing := reg.Lookup("SCH-missing")
	assert.False(t, okMissing)
	_, okEmpty := reg.Lookup("")
	assert.False(t, okEmpty)

	// Unregister empty
	require.Error(t, reg.Unregister(""))

	// Unregister missing
	errMissing := reg.Unregister("SCH-non-existent")
	require.Error(t, errMissing)
	assert.True(t, errors.Is(errMissing, ErrJobNotFound))

	// Unregister valid
	require.NoError(t, reg.Unregister("SCH-reg-1"))
	assert.Equal(t, 1, reg.Count())
	_, stillExists := reg.Lookup("SCH-reg-1")
	assert.False(t, stillExists)
}

func TestSchedulerCycle31_Static_JobRegistry_ListJobsCountAndClear(t *testing.T) {
	t.Parallel()

	var nilReg *JobRegistry
	assert.Nil(t, nilReg.ListJobs())
	assert.Equal(t, 0, nilReg.Count())
	nilReg.Clear()

	reg := NewJobRegistry()
	assert.Empty(t, reg.ListJobs())

	jobs := []*ScheduledJob{
		{ID: "SCH-list-1", TriggerType: TriggerTypeManual},
		{ID: "SCH-list-2", TriggerType: TriggerTypeManual},
		{ID: "SCH-list-3", TriggerType: TriggerTypeManual},
	}
	for _, j := range jobs {
		require.NoError(t, reg.Register(j))
	}

	assert.Equal(t, 3, reg.Count())
	listed := reg.ListJobs()
	assert.Len(t, listed, 3)

	reg.Clear()
	assert.Equal(t, 0, reg.Count())
	assert.Empty(t, reg.ListJobs())
}

func TestSchedulerCycle31_Static_JobRegistry_AdmissionPipelineIntegration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	pipeline := NewJobAdmissionPipeline(
		AdmissionFilterEnabled(),
		AdmissionFilterRequirePriority(JobPriorityHigh, JobPriorityCritical),
	)

	reg := NewJobRegistryWithPipeline(pipeline)

	// Job with invalid priority rejected by pipeline
	jobNormal := &ScheduledJob{
		ID:          "SCH-adm-1",
		TriggerType: TriggerTypeManual,
		Priority:    JobPriorityNormal,
		Enabled:     true,
	}
	errNormal := reg.RegisterWithContext(ctx, jobNormal)
	require.Error(t, errNormal)
	assert.True(t, errors.Is(errNormal, ErrAdmissionFilterRejected))

	// Disabled job rejected
	jobDisabled := &ScheduledJob{
		ID:          "SCH-adm-2",
		TriggerType: TriggerTypeManual,
		Priority:    JobPriorityHigh,
		Enabled:     false,
	}
	errDisabled := reg.RegisterWithContext(ctx, jobDisabled)
	require.Error(t, errDisabled)
	assert.True(t, errors.Is(errDisabled, ErrAdmissionFilterRejected))

	// Admitted job accepted
	jobAccepted := &ScheduledJob{
		ID:          "SCH-adm-3",
		TriggerType: TriggerTypeManual,
		Priority:    JobPriorityHigh,
		Enabled:     true,
	}
	require.NoError(t, reg.RegisterWithContext(ctx, jobAccepted))
	assert.Equal(t, 1, reg.Count())

	// Dynamic pipeline clearing
	reg.SetAdmissionPipeline(nil)
	require.NoError(t, reg.Register(jobNormal))
	assert.Equal(t, 2, reg.Count())
}

func TestSchedulerCycle31_Static_JobRegistry_ConcurrentReadWriteInvariants(t *testing.T) {
	t.Parallel()

	reg := NewJobRegistry()
	numWorkers := 8
	iterations := 50
	var wg sync.WaitGroup
	var registerErrors uint64
	var lookupErrors uint64
	var unregisterErrors uint64
	var readerErrors uint64

	// Concurrent register & unregister workers
	for w := 0; w < numWorkers; w++ {
		workerID := w
		wg.Add(1)
		goroutinelabels.NewGoroutine("cycle31_reg_writer", fmt.Sprintf("worker-%d", workerID)).StartSimple(func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				jobID := fmt.Sprintf("SCH-concur-%d-%d", workerID, i)
				job := &ScheduledJob{ID: jobID, TriggerType: TriggerTypeManual}
				if err := reg.Register(job); err != nil {
					atomic.AddUint64(&registerErrors, 1)
				}
				foundJob, found := reg.Lookup(jobID)
				if !found || foundJob == nil {
					atomic.AddUint64(&lookupErrors, 1)
				}
				if err := reg.Unregister(jobID); err != nil {
					atomic.AddUint64(&unregisterErrors, 1)
				}
			}
		})
	}

	// Concurrent read/list workers
	for r := 0; r < 4; r++ {
		readerID := r
		wg.Add(1)
		goroutinelabels.NewGoroutine("cycle31_reg_reader", fmt.Sprintf("reader-%d", readerID)).StartSimple(func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				list := reg.ListJobs()
				count := reg.Count()
				if list == nil || count < 0 {
					atomic.AddUint64(&readerErrors, 1)
				}
			}
		})
	}

	wg.Wait()
	assert.Equal(t, uint64(0), atomic.LoadUint64(&registerErrors))
	assert.Equal(t, uint64(0), atomic.LoadUint64(&lookupErrors))
	assert.Equal(t, uint64(0), atomic.LoadUint64(&unregisterErrors))
	assert.Equal(t, uint64(0), atomic.LoadUint64(&readerErrors))
}

func TestSchedulerCycle31_Static_SchedulerOptions_DefaultsAndApply(t *testing.T) {
	t.Parallel()

	def := DefaultSchedulerOptions()
	require.NotNil(t, def)
	assert.Equal(t, DefaultSchedulerTimeout, def.Timeout)
	assert.Equal(t, DefaultSchedulerTickInterval, def.TickInterval)
	assert.Equal(t, DefaultMaxConcurrency, def.MaxConcurrency)
	assert.True(t, def.AdmissionTimeout > 0)
	require.NoError(t, def.Validate())

	var nilOpts *SchedulerOptions
	assert.Nil(t, nilOpts.Apply(WithTimeout(5*time.Second)))
}

func TestSchedulerCycle31_Static_SchedulerOptions_WithFunctionalOptions(t *testing.T) {
	t.Parallel()

	customTimeout := 45 * time.Second
	customTick := 2 * time.Second
	customAdmission := 5 * time.Minute
	customConcurrency := 25
	customRoot := "/tmp/zqk-cycle31-test"
	pipeline := NewJobAdmissionPipeline()

	opts := NewSchedulerOptions(
		WithTimeout(customTimeout),
		WithTickInterval(customTick),
		WithAdmissionTimeout(customAdmission),
		WithMaxConcurrency(customConcurrency),
		WithProjectRoot(customRoot),
		WithOnlyManualJobs(true),
		WithAdmissionPipeline(pipeline),
	)

	require.NotNil(t, opts)
	assert.Equal(t, customTimeout, opts.Timeout)
	assert.Equal(t, customTick, opts.TickInterval)
	assert.Equal(t, customAdmission, opts.AdmissionTimeout)
	assert.Equal(t, customConcurrency, opts.MaxConcurrency)
	assert.Equal(t, customRoot, opts.ProjectRoot)
	assert.True(t, opts.OnlyManualJobs)
	assert.Equal(t, pipeline, opts.AdmissionPipeline)
	require.NoError(t, opts.Validate())
}

func TestSchedulerCycle31_Static_SchedulerOptions_NilOptionSafety(t *testing.T) {
	t.Parallel()

	// Safe with nil options in constructor
	opts := NewSchedulerOptions(nil, WithTimeout(10*time.Second), nil)
	require.NotNil(t, opts)
	assert.Equal(t, 10*time.Second, opts.Timeout)

	// Safe when calling option func with nil
	WithTimeout(5 * time.Second)(nil)
	WithTickInterval(5 * time.Second)(nil)
	WithAdmissionTimeout(5 * time.Second)(nil)
	WithMaxConcurrency(5)(nil)
	WithProjectRoot("/test")(nil)
	WithOnlyManualJobs(false)(nil)
	WithStorageProvider(nil)(nil)
	WithAdmissionPipeline(nil)(nil)

	// Non-positive values do not overwrite valid defaults
	opts2 := NewSchedulerOptions(
		WithTimeout(0),
		WithTickInterval(-1),
		WithAdmissionTimeout(0),
		WithMaxConcurrency(-5),
	)
	assert.Equal(t, DefaultSchedulerTimeout, opts2.Timeout)
	assert.Equal(t, DefaultSchedulerTickInterval, opts2.TickInterval)
	assert.Equal(t, DefaultMaxConcurrency, opts2.MaxConcurrency)
}

func TestSchedulerCycle31_Static_SchedulerOptions_ValidationRules(t *testing.T) {
	t.Parallel()

	var nilOpts *SchedulerOptions
	require.Error(t, nilOpts.Validate())

	badTimeout := DefaultSchedulerOptions()
	badTimeout.Timeout = 0
	errTimeout := badTimeout.Validate()
	require.Error(t, errTimeout)
	assert.True(t, errors.Is(errTimeout, ErrInvalidSchedulerTimeout))

	badTick := DefaultSchedulerOptions()
	badTick.TickInterval = -1 * time.Second
	errTick := badTick.Validate()
	require.Error(t, errTick)
	assert.True(t, errors.Is(errTick, ErrInvalidTickInterval))

	badAdmission := DefaultSchedulerOptions()
	badAdmission.AdmissionTimeout = 0
	errAdmission := badAdmission.Validate()
	require.Error(t, errAdmission)
	assert.True(t, errors.Is(errAdmission, ErrInvalidAdmissionTimeout))

	badConcurrency := DefaultSchedulerOptions()
	badConcurrency.MaxConcurrency = 0
	errConcurrency := badConcurrency.Validate()
	require.Error(t, errConcurrency)
	assert.True(t, errors.Is(errConcurrency, ErrInvalidMaxConcurrency))
}

func TestSchedulerCycle31_Static_StorageProviderOption(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	sp, err := storagepkg.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	t.Cleanup(func() {
		shutdownErr := sp.Shutdown(context.Background())
		require.NoError(t, shutdownErr)
	})

	opts := NewSchedulerOptions(WithStorageProvider(sp))
	require.NotNil(t, opts)
	assert.Equal(t, sp, opts.StorageProvider)
}
