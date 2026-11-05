package scheduler

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/agentclaim"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestJobRegistry_StaticFloorInvariants verifies structural invariants, nil receiver safety,
// duplicate rejection, and bounds validation on JobRegistry.
func TestJobRegistry_StaticFloorInvariants(t *testing.T) {
	t.Parallel()

	// 1. Nil receiver safety
	var nilReg *JobRegistry
	assert.Error(t, nilReg.Register(&ScheduledJob{ID: "SCH-001"}))
	assert.Error(t, nilReg.RegisterWithContext(context.Background(), &ScheduledJob{ID: "SCH-001"}))
	assert.Error(t, nilReg.Unregister("SCH-001"))
	job, found := nilReg.Lookup("SCH-001")
	assert.False(t, found)
	assert.Nil(t, job)
	assert.Nil(t, nilReg.ListJobs())
	assert.Equal(t, 0, nilReg.Count())
	nilReg.Clear()
	nilReg.SetAdmissionPipeline(nil)

	// 2. Initialization invariants
	reg := NewJobRegistry()
	require.NotNil(t, reg)
	assert.Equal(t, 0, reg.Count())
	assert.Empty(t, reg.ListJobs())

	// 3. Register bounds validation
	assert.ErrorIs(t, reg.Register(nil), ErrNilJobEnvelope)
	assert.ErrorIs(t, reg.Register(&ScheduledJob{ID: ""}), ErrEmptyJobID)
	assert.ErrorIs(t, reg.Unregister(""), ErrEmptyJobID)
	assert.ErrorIs(t, reg.Unregister("NON-EXISTENT"), ErrJobNotFound)

	lookupJob, ok := reg.Lookup("")
	assert.False(t, ok)
	assert.Nil(t, lookupJob)

	// 4. Duplicate registration rejection
	validJob := &ScheduledJob{
		ID:            "SCH-REG-001",
		Title:         "Sample Job",
		JobType:       "run_wrapper",
		Category:      CategoryUser,
		TriggerType:   TriggerTypeImmediate,
		ExecutionMode: ExecutionModeOneTime,
		Enabled:       true,
		Command:       "echo",
	}

	err := reg.Register(validJob)
	require.NoError(t, err)
	assert.Equal(t, 1, reg.Count())

	// Re-register same ID must fail
	errDup := reg.Register(validJob)
	assert.ErrorIs(t, errDup, ErrJobAlreadyRegistered)

	// Lookup registered job
	retrieved, exists := reg.Lookup("SCH-REG-001")
	assert.True(t, exists)
	assert.Equal(t, validJob.ID, retrieved.ID)
	assert.Equal(t, validJob.Title, retrieved.Title)

	// Unregister
	errUnreg := reg.Unregister("SCH-REG-001")
	require.NoError(t, errUnreg)
	assert.Equal(t, 0, reg.Count())

	// Unregister again must fail
	assert.ErrorIs(t, reg.Unregister("SCH-REG-001"), ErrJobNotFound)
}

// TestJobRegistry_AdmissionPipelineEnforcement verifies integration between JobRegistry
// and JobAdmissionPipeline, including custom filters, priority, and archived rejection.
func TestJobRegistry_AdmissionPipelineEnforcement(t *testing.T) {
	t.Parallel()

	// Pipeline requiring high priority and enabled status
	pipeline := NewJobAdmissionPipeline(
		AdmissionFilterRequirePriority(JobPriorityHigh, JobPriorityCritical),
		AdmissionFilterNonArchived(),
		AdmissionFilterEnabled(),
	)
	assert.Equal(t, 3, pipeline.FilterCount())

	reg := NewJobRegistryWithPipeline(pipeline)
	require.NotNil(t, reg)

	// Case 1: Job missing required priority -> rejected
	jobNormal := &ScheduledJob{
		ID:            "SCH-NORM",
		Title:         "Normal priority job",
		JobType:       "run_wrapper",
		Category:      CategoryUser,
		TriggerType:   TriggerTypeImmediate,
		ExecutionMode: ExecutionModeOneTime,
		Priority:      JobPriorityNormal,
		Enabled:       true,
		Command:       "echo",
	}
	err := reg.Register(jobNormal)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAdmissionFilterRejected)

	// Case 2: Job with archived status -> rejected
	jobArchived := &ScheduledJob{
		ID:            "SCH-ARCHIVED",
		Title:         "Archived job",
		JobType:       "run_wrapper",
		Category:      CategoryUser,
		TriggerType:   TriggerTypeImmediate,
		ExecutionMode: ExecutionModeOneTime,
		Priority:      JobPriorityHigh,
		Status:        objects.ObjectStatusArchived,
		Enabled:       true,
		Command:       "echo",
	}
	err = reg.Register(jobArchived)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAdmissionFilterRejected)

	// Case 3: Job with disabled flag -> rejected
	jobDisabled := &ScheduledJob{
		ID:            "SCH-DISABLED",
		Title:         "Disabled job",
		JobType:       "run_wrapper",
		Category:      CategoryUser,
		TriggerType:   TriggerTypeImmediate,
		ExecutionMode: ExecutionModeOneTime,
		Priority:      JobPriorityHigh,
		Enabled:       false,
		Command:       "echo",
	}
	err = reg.Register(jobDisabled)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAdmissionFilterRejected)

	// Case 4: Compliant job -> admitted
	jobGood := &ScheduledJob{
		ID:            "SCH-GOOD",
		Title:         "Good job",
		JobType:       "run_wrapper",
		Category:      CategoryUser,
		TriggerType:   TriggerTypeImmediate,
		ExecutionMode: ExecutionModeOneTime,
		Priority:      JobPriorityHigh,
		Enabled:       true,
		Command:       "echo",
	}
	err = reg.Register(jobGood)
	require.NoError(t, err)
	assert.Equal(t, 1, reg.Count())

	// Dynamic clearing of pipeline allows previously blocked jobs
	reg.SetAdmissionPipeline(nil)
	jobNorm2 := &ScheduledJob{
		ID:            "SCH-NORM-2",
		Title:         "Normal priority allowed now",
		JobType:       "run_wrapper",
		Category:      CategoryUser,
		TriggerType:   TriggerTypeImmediate,
		ExecutionMode: ExecutionModeOneTime,
		Priority:      JobPriorityNormal,
		Enabled:       true,
		Command:       "echo",
	}
	err = reg.Register(jobNorm2)
	require.NoError(t, err)
	assert.Equal(t, 2, reg.Count())
}

// TestJobRegistry_OperationalDynamicsAndConcurrency verifies concurrent operations,
// snapshot consistency, and thread safety of JobRegistry using goroutinelabels.
func TestJobRegistry_OperationalDynamicsAndConcurrency(t *testing.T) {
	t.Parallel()

	reg := NewJobRegistry()
	require.NotNil(t, reg)

	const workerCount = 8
	const jobsPerWorker = 20
	var wg sync.WaitGroup

	// Concurrently register jobs
	for w := 0; w < workerCount; w++ {
		workerID := w
		wg.Add(1)
		goroutinelabels.NewGoroutine("test-job-registry-writer", "concurrent registration").StartSimple(func() {
			defer wg.Done()
			for j := 0; j < jobsPerWorker; j++ {
				jobID := fmt.Sprintf("SCH-CONCUR-%d-%d", workerID, j)
				job := &ScheduledJob{
					ID:            jobID,
					Title:         fmt.Sprintf("Concurrent job %s", jobID),
					JobType:       "run_wrapper",
					Category:      CategoryUser,
					TriggerType:   TriggerTypeImmediate,
					ExecutionMode: ExecutionModeOneTime,
					Enabled:       true,
					Command:       "echo",
				}
				_ = reg.Register(job)
			}
		})
	}
	wg.Wait()

	expectedTotal := workerCount * jobsPerWorker
	assert.Equal(t, expectedTotal, reg.Count())
	list := reg.ListJobs()
	assert.Equal(t, expectedTotal, len(list))

	// Concurrently read and unregister half the jobs
	var readWg sync.WaitGroup
	for w := 0; w < workerCount; w++ {
		workerID := w
		readWg.Add(1)
		goroutinelabels.NewGoroutine("test-job-registry-reader", "concurrent lookup and unregister").StartSimple(func() {
			defer readWg.Done()
			for j := 0; j < jobsPerWorker; j++ {
				jobID := fmt.Sprintf("SCH-CONCUR-%d-%d", workerID, j)
				retrieved, ok := reg.Lookup(jobID)
				if ok {
					assert.Equal(t, jobID, retrieved.ID)
				}
				// Even jobs are unregistered
				if j%2 == 0 {
					_ = reg.Unregister(jobID)
				}
			}
		})
	}
	readWg.Wait()

	// Remaining count must match expected exactly
	remaining := reg.Count()
	assert.Equal(t, expectedTotal/2, remaining)
	assert.Equal(t, remaining, len(reg.ListJobs()))

	// Clear resets everything
	reg.Clear()
	assert.Equal(t, 0, reg.Count())
	assert.Empty(t, reg.ListJobs())
}

// TestHourglass_ElevationAndActions verifies action routing for expired timers,
// deadline escalation risk blocker construction, and reference matching.
func TestHourglass_ElevationAndActions(t *testing.T) {
	t.Parallel()

	// 1. Verify actionForExpiredTimer mapping
	assert.Equal(t, actionWakeOrchestrator, actionForExpiredTimer(agentclaim.TimerTypeCheckin))
	assert.Equal(t, actionWakeOrchestrator, actionForExpiredTimer(agentclaim.TimerTypeSeatHeartbeat))
	assert.Equal(t, actionEscalateDeadline, actionForExpiredTimer(timerTypeDeadline))
	assert.Equal(t, actionKillStuckProcess, actionForExpiredTimer("unknown_timer_type"))
	assert.Equal(t, actionKillStuckProcess, actionForExpiredTimer(""))

	// 2. Risk blocker construction
	blockerID := newMissedDeadlineRiskBlockerID()
	assert.NotEmpty(t, blockerID)
	assert.Contains(t, blockerID, "RIS-")

	blockerMap, err := buildMissedDeadlineRiskBlocker(blockerID, "TSK-001", "task", "Resolve Flaky Test")
	require.NoError(t, err)
	require.NotNil(t, blockerMap)
	assert.Equal(t, blockerID, blockerMap["id"])
	assert.Equal(t, missedDeadlineEscalationTitlePrefix+"Resolve Flaky Test", blockerMap["title"])
	assert.Equal(t, escalationRiskTypeBlocker, blockerMap["risk_type"])
	assert.Equal(t, escalationSeverityHigh, blockerMap["severity"])

	// 3. refsContainID helpers
	assert.True(t, refsContainID([]string{"A", "B", "C"}, "B"))
	assert.False(t, refsContainID([]string{"A", "B", "C"}, "D"))
	assert.True(t, refsContainID([]any{"A", "B", "C"}, "C"))
	assert.False(t, refsContainID([]any{"A", "B", "C"}, "Z"))
	assert.False(t, refsContainID("not_a_slice", "A"))
	assert.False(t, refsContainID(nil, "A"))
}

// TestTriggerParser_ElevationAndRequirements verifies trigger parsing and requirement
// validations across edge cases and negative boundaries.
func TestTriggerParser_ElevationAndRequirements(t *testing.T) {
	t.Parallel()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// 1. ParseTriggerType
	assert.Equal(t, TriggerTypeTimer, ParseTriggerType(TriggerTypeTimer, logger, "JOB-1"))
	assert.Equal(t, TriggerTypeManual, ParseTriggerType(TriggerTypeManual, logger, "JOB-2"))
	assert.Equal(t, TriggerTypeImmediate, ParseTriggerType(TriggerTypeImmediate, logger, "JOB-3"))
	assert.Equal(t, TriggerTypeWorkflow, ParseTriggerType(TriggerTypeWorkflow, logger, "JOB-4"))
	assert.Equal(t, TriggerTypeEvent, ParseTriggerType(TriggerTypeEvent, logger, "JOB-5"))
	assert.Equal(t, TriggerTypeLifecycle, ParseTriggerType(TriggerTypeLifecycle, logger, "JOB-6"))
	assert.Equal(t, "timer", ParseTriggerType("", logger, "JOB-7"))
	assert.Equal(t, "timer", ParseTriggerType("custom_nonexistent", logger, "JOB-8"))

	// 2. ValidateTriggerRequirements
	// Valid timer
	resTimerOk := ValidateTriggerRequirements("timer", "0 * * * *", "", "", "", logger, "J1", nil)
	assert.True(t, resTimerOk.Valid)

	// Invalid timer (empty schedule)
	resTimerBad := ValidateTriggerRequirements("timer", "", "", "", "", logger, "J2", nil)
	assert.False(t, resTimerBad.Valid)
	assert.Contains(t, resTimerBad.Reason, "schedule_expression")

	// Valid workflow
	resWorkflowOk := ValidateTriggerRequirements(TriggerTypeWorkflow, "", "WF-123", "", "", logger, "J3", nil)
	assert.True(t, resWorkflowOk.Valid)

	// Invalid workflow (empty ref)
	resWorkflowBad := ValidateTriggerRequirements(TriggerTypeWorkflow, "", "", "", "", logger, "J4", nil)
	assert.False(t, resWorkflowBad.Valid)

	// Valid event
	resEventOk := ValidateTriggerRequirements(TriggerTypeEvent, "", "", "event.created", "", logger, "J5", nil)
	assert.True(t, resEventOk.Valid)

	// Invalid event (empty filter)
	resEventBad := ValidateTriggerRequirements(TriggerTypeEvent, "", "", "", "", logger, "J6", nil)
	assert.False(t, resEventBad.Valid)

	// Valid lifecycle
	resLifeOk := ValidateTriggerRequirements(TriggerTypeLifecycle, "", "", "", "active->complete", logger, "J7", nil)
	assert.True(t, resLifeOk.Valid)

	// Invalid lifecycle (empty filter)
	resLifeBad := ValidateTriggerRequirements(TriggerTypeLifecycle, "", "", "", "", logger, "J8", nil)
	assert.False(t, resLifeBad.Valid)

	// Manual and Immediate (no extra requirements)
	resManual := ValidateTriggerRequirements(TriggerTypeManual, "", "", "", "", logger, "J9", nil)
	assert.True(t, resManual.Valid)

	resImmediate := ValidateTriggerRequirements(TriggerTypeImmediate, "", "", "", "", logger, "J10", nil)
	assert.True(t, resImmediate.Valid)
}
