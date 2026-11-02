package scheduler

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	admissionReasonTimeout    = "admission_timeout"
	admissionReasonNotInCache = "not_in_cache_after_retries"
	admissionErrorPrefix      = "scheduler admission failed: "
	lockNameAdmissionScanJobs = "scheduler.admission_scan_jobs"
)

// oneTimeImmediateShouldStartOnReload is true when a periodic ReloadJobs must call
// scheduleImmediateJob instead of only registerTriggeredJob.
//
// SCH-run-* / category=testing stay register-only so reload cannot flood the pool
// (the original mem-explosion guard). Callback-bearing CLI one-shots that were
// never queued (LastRunAt unset, not pending) must start or the error callback never runs.
func oneTimeImmediateShouldStartOnReload(job *ScheduledJob, pending bool) bool {
	if job == nil || pending || job.LastRunAt != nil {
		return false
	}
	if !job.Enabled || job.Status == StatusDisabled || job.Status == objects.ObjectStatusArchived {
		return false
	}
	if job.ExecutionMode != ExecutionModeOneTime {
		return false
	}
	if job.TriggerType != TriggerTypeImmediate {
		return false
	}
	if IsTestBundleJob(job.ID) || job.Category == CategoryTesting {
		return false
	}
	if job.CallbackOnError != emptyValue || job.CallbackOnCompletion != emptyValue {
		return true
	}
	if job.Category == CategoryManual || job.Category == CategoryUser {
		return true
	}
	return job.Priority == JobPriorityHigh || job.Priority == JobPriorityCritical
}

func jobWarrantsAdmissionHourglass(job *ScheduledJob) bool {
	if job == nil || !job.Enabled || job.LastRunAt != nil || job.Status == StatusDisabled || job.Status == objects.ObjectStatusArchived {
		return false
	}
	if job.IsRunning() {
		return false
	}
	if job.ExecutionMode != ExecutionModeOneTime || job.TriggerType != TriggerTypeImmediate {
		return false
	}
	if IsTestBundleJob(job.ID) || job.Category == CategoryTesting {
		return false
	}
	return job.CallbackOnError != emptyValue || job.CallbackOnCompletion != emptyValue
}

func (s *Scheduler) markOneTimeImmediatePending(job *ScheduledJob) {
	if s == nil || job == nil {
		return
	}
	if job.ExecutionMode != ExecutionModeOneTime || job.TriggerType != TriggerTypeImmediate {
		return
	}
	s.immediateDispatchPending.Store(job.ID, struct{}{})
}

func (s *Scheduler) isImmediateDispatchPending(jobID string) bool {
	if s == nil || jobID == emptyValue {
		return false
	}
	_, ok := s.immediateDispatchPending.Load(jobID)
	return ok
}

func (s *Scheduler) clearImmediateDispatchPending(jobID string) {
	if s == nil || jobID == emptyValue {
		return
	}
	s.immediateDispatchPending.Delete(jobID)
}

func (s *Scheduler) markAdmissionFailed(jobID string) bool {
	if s == nil || jobID == emptyValue {
		return false
	}
	_, loaded := s.admissionFailed.LoadOrStore(jobID, struct{}{})
	return !loaded
}

func (s *Scheduler) admissionAlreadyFailed(jobID string) bool {
	if s == nil || jobID == emptyValue {
		return false
	}
	_, ok := s.admissionFailed.Load(jobID)
	return ok
}

// FailJobAdmission invokes callback_on_error and disables the job so a late start cannot double-commit.
// Used when the trigger queue exhausts cache-miss retries for a CLI one-shot.
func (s *Scheduler) FailJobAdmission(ctx context.Context, jobID, reason string) {
	if s == nil || jobID == emptyValue {
		return
	}
	job, ok := s.lookupJobInCache(jobID)
	if !ok || job == nil {
		job = s.readJobForAdmission(ctx, jobID)
	}
	if job == nil {
		SchedulerTriggerQueueLog(s.logger).Warn(LogEventSchedulerTriggerQueueAdmissionFailed).
			JobID(jobID).
			String("reason", reason).
			String("detail", "job not in cache or storage").
			Log()
		if s.storage != nil && s.secCtx != nil {
			_ = s.storage.Update(pkgctx.WithCacheInvalidate(ctx, jobID), s.secCtx, jobID, map[string]any{
				objects.FieldKeyEnabled: false,
				objects.FieldKeyStatus:  StatusDisabled,
			})
		}
		return
	}
	s.failJobAdmission(ctx, job, reason)
}

func (s *Scheduler) readJobForAdmission(ctx context.Context, jobID string) *ScheduledJob {
	if s.storage == nil || s.secCtx == nil {
		return nil
	}
	raw, err := s.storage.Read(ctx, s.secCtx, jobID)
	if err != nil || raw == nil {
		return nil
	}
	if s.jobLoader == nil {
		return nil
	}
	job, hydErr := s.jobLoader.HydrateJob(raw)
	if hydErr != nil || job == nil {
		return nil
	}
	return job
}

// AdmitJobFromStorage hydrates a job by id via storage.Read and registers it in s.jobs.
// List/CAS-index lag after cross-process `scheduler submit` leaves JobInCache false while
// object get / Read already succeed (FailJobAdmission already used this path to disable).
// Returns true when the job is in cache and eligible to trigger.
// read-your-writes with pending visibility for scheduler_job (CAS_LIST_GET_CONSISTENCY.md).
func (s *Scheduler) AdmitJobFromStorage(ctx context.Context, jobID string) bool {
	if s == nil || jobID == emptyValue {
		return false
	}
	if job, ok := s.lookupJobInCache(jobID); ok && job != nil {
		return job.Enabled && job.Status != objects.ObjectStatusArchived && job.Status != StatusDisabled
	}
	job := s.readJobForAdmission(ctx, jobID)
	if job == nil || !job.Enabled || job.Status == objects.ObjectStatusArchived || job.Status == StatusDisabled {
		return false
	}
	if job.AllowParallelExecution {
		job.ConcurrentAllowed = true
	} else {
		job.ConcurrentAllowed = s.isConcurrentAllowed(job.JobType, job.Category)
	}
	_ = concurrency.RunInLockWithLogger(
		&s.jobsMu, LockNameSchedulerAdmitJobFromStorage, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if existing, ok := s.jobs[jobID]; ok && existing != nil {
				return nil
			}
			s.jobs[jobID] = job
			return s.registerTriggeredJob(job)
		},
	)
	if _, ok := s.lookupJobInCache(jobID); !ok {
		return false
	}
	SchedulerTriggerQueueLog(s.logger).Info(LogEventSchedulerTriggerQueueAdmittedFromStorage).
		JobID(jobID).
		Log()
	s.EmitTriggerQueueEvent(map[string]any{
		objects.FieldKeyEventType: "trigger_queue_admitted_from_storage",
		triggerQueueKeyMessage:    "Admitted job via storage.Read after List/cache miss",
		triggerQueueKeyJobID:      jobID,
	})
	return true
}

func (s *Scheduler) failJobAdmission(ctx context.Context, job *ScheduledJob, reason string) {
	if job == nil {
		return
	}
	if !s.markAdmissionFailed(job.ID) {
		return
	}
	s.clearImmediateDispatchPending(job.ID)
	errMsg := admissionErrorPrefix + reason
	payload := map[string]any{
		"outcome":              "error",
		"error":                errMsg,
		objects.FieldKeyReason: reason,
	}
	if job.CallbackOnError != emptyValue {
		InvokeJobCallback(ctx, s.logger, s.asyncRouter, job, callbackTypeError, payload)
	} else if job.CallbackOnCompletion != emptyValue {
		// Seat is still waiting; completion callback with error outcome clears inflight locks.
		InvokeJobCallback(ctx, s.logger, s.asyncRouter, job, callbackTypeCompletion, payload)
	}
	job.Enabled = false
	job.Status = StatusDisabled
	s.disableJobInStorage(ctx, job)
	SchedulerJobManagementLog(s.logger).Warn(LogEventSchedulerJobMgmtAdmissionTimeout).
		JobID(job.ID).
		String("reason", reason).
		Log()
	s.EmitTriggerQueueEvent(map[string]any{
		objects.FieldKeyEventType: "trigger_queue_admission_failed",
		triggerQueueKeyMessage:    errMsg,
		triggerQueueKeyJobID:      job.ID,
		objects.FieldKeyReason:    reason,
	})
}

// isJobOccupied returns true if the job is actively running in memory, pending immediate dispatch,
// or holds an active, unexpired JobExecutionLease in JobStateRegistry.
// When occupied, CreatedAt age does not represent "never started" and scanAdmissionTimeouts must not disable it.
func (s *Scheduler) isJobOccupied(job *ScheduledJob) bool {
	if s == nil || job == nil {
		return false
	}
	if job.IsRunning() {
		return true
	}
	if s.isImmediateDispatchPending(job.ID) {
		return true
	}
	if s.stateRegistry != nil {
		lease, err := s.stateRegistry.GetActiveLease(job.ID)
		if err == nil && lease != nil && lease.IsValid(time.Now().UTC()) {
			return true
		}
	}
	return false
}

func (s *Scheduler) scanAdmissionTimeouts(ctx context.Context) {
	if s == nil {
		return
	}
	timeout := getSchedulerAdmissionTimeout()
	now := time.Now().UTC()
	var due []*ScheduledJob
	_ = concurrency.RunInRLockWithLogger(
		&s.jobsMu, lockNameAdmissionScanJobs, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			for _, job := range s.jobs {
				if !jobWarrantsAdmissionHourglass(job) {
					continue
				}
				if s.admissionAlreadyFailed(job.ID) {
					continue
				}
				if s.isJobOccupied(job) {
					// Already accepted onto a pool worker, actively running, or holding active lease; CreatedAt age is not "never started".
					continue
				}
				if job.CreatedAt.IsZero() {
					continue
				}
				if now.Sub(job.CreatedAt) >= timeout {
					due = append(due, job)
				}
			}
			return nil
		},
	)
	for _, job := range due {
		if s.isJobOccupied(job) {
			continue
		}
		s.failJobAdmission(ctx, job, admissionReasonTimeout)
	}
}

var (
	ErrNilJobEnvelope          = errors.New("job envelope is nil")
	ErrEmptyJobID              = errors.New("job ID cannot be empty")
	ErrInvalidJobPriority      = errors.New("invalid job priority: must be normal, high, or critical")
	ErrInvalidTriggerType      = errors.New("invalid trigger type")
	ErrInvalidExecutionMode    = errors.New("invalid execution mode")
	ErrMissingScheduleOption   = errors.New("missing required schedule option")
	ErrAdmissionFilterRejected = errors.New("job admission filter rejected job")
)

// ValidateJobEnvelope validates the structural and semantic bounds of a ScheduledJob.
func ValidateJobEnvelope(job *ScheduledJob) error {
	if job == nil {
		return ErrNilJobEnvelope
	}
	if strings.TrimSpace(job.ID) == "" {
		return ErrEmptyJobID
	}
	if err := validateJobPriority(job.Priority); err != nil {
		return err
	}
	if err := validateJobTrigger(job); err != nil {
		return err
	}
	return validateJobExecutionMode(job.ExecutionMode)
}

func validateJobPriority(priority string) error {
	if priority == "" {
		return nil
	}
	switch priority {
	case JobPriorityNormal, JobPriorityHigh, JobPriorityCritical:
		return nil
	default:
		return errfmt.Errorf("%w: %s", ErrInvalidJobPriority, priority)
	}
}

func validateJobTrigger(job *ScheduledJob) error {
	switch job.TriggerType {
	case TriggerTypeTimer:
		if strings.TrimSpace(job.ScheduleExpr) == "" {
			return errfmt.Errorf("%w: timer requires schedule_expr", ErrMissingScheduleOption)
		}
		if _, err := ParseCronSchedule(job.ScheduleExpr); err != nil {
			return errfmt.Errorf("%w: %v", ErrInvalidCronSchedule, err)
		}
		return nil
	case TriggerTypeManual, TriggerTypeImmediate:
		return nil
	case TriggerTypeWorkflow:
		if strings.TrimSpace(job.WorkflowRef) == "" {
			return errfmt.Errorf("%w: workflow trigger requires workflow_ref", ErrMissingScheduleOption)
		}
		return nil
	case TriggerTypeEvent:
		if strings.TrimSpace(job.EventFilter) == "" {
			return errfmt.Errorf("%w: event trigger requires event_filter", ErrMissingScheduleOption)
		}
		return nil
	case TriggerTypeLifecycle:
		if strings.TrimSpace(job.LifecycleFilter) == "" {
			return errfmt.Errorf("%w: lifecycle trigger requires lifecycle_filter", ErrMissingScheduleOption)
		}
		return nil
	case "":
		return errfmt.Errorf("%w: trigger type is empty", ErrInvalidTriggerType)
	default:
		return errfmt.Errorf("%w: %s", ErrInvalidTriggerType, job.TriggerType)
	}
}

func validateJobExecutionMode(mode string) error {
	if mode == "" {
		return nil
	}
	switch mode {
	case ExecutionModeReusable, ExecutionModeOneTime:
		return nil
	default:
		return errfmt.Errorf("%w: %s", ErrInvalidExecutionMode, mode)
	}
}

// JobAdmissionFilter defines a validation or admission policy hook for scheduled jobs.
type JobAdmissionFilter func(ctx context.Context, job *ScheduledJob) error

// JobAdmissionPipeline executes a sequence of admission filters for candidate jobs.
type JobAdmissionPipeline struct {
	mu      sync.RWMutex
	filters []JobAdmissionFilter
}

// NewJobAdmissionPipeline creates a new admission pipeline.
func NewJobAdmissionPipeline(filters ...JobAdmissionFilter) *JobAdmissionPipeline {
	p := &JobAdmissionPipeline{
		filters: make([]JobAdmissionFilter, 0, len(filters)),
	}
	for _, f := range filters {
		if f != nil {
			p.filters = append(p.filters, f)
		}
	}
	return p
}

// AddFilter appends an admission filter to the pipeline.
func (p *JobAdmissionPipeline) AddFilter(filter JobAdmissionFilter) *JobAdmissionPipeline {
	if filter == nil {
		return p
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.filters = append(p.filters, filter)
	return p
}

// FilterCount returns the number of active filters.
func (p *JobAdmissionPipeline) FilterCount() int {
	if p == nil {
		return 0
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.filters)
}

// Admit runs envelope validation and all pipeline filters.
func (p *JobAdmissionPipeline) Admit(ctx context.Context, job *ScheduledJob) error {
	if err := ValidateJobEnvelope(job); err != nil {
		return err
	}
	if p == nil {
		return nil
	}
	p.mu.RLock()
	filters := make([]JobAdmissionFilter, len(p.filters))
	copy(filters, p.filters)
	p.mu.RUnlock()

	for _, filter := range filters {
		if err := filter(ctx, job); err != nil {
			return errfmt.Errorf("%w: %v", ErrAdmissionFilterRejected, err)
		}
	}
	return nil
}

// AdmissionFilterRequirePriority creates an admission filter enforcing allowed priorities.
func AdmissionFilterRequirePriority(allowedPriorities ...string) JobAdmissionFilter {
	allowed := make(map[string]struct{}, len(allowedPriorities))
	for _, p := range allowedPriorities {
		allowed[p] = struct{}{}
	}
	return func(_ context.Context, job *ScheduledJob) error {
		if job == nil {
			return ErrNilJobEnvelope
		}
		if _, ok := allowed[job.Priority]; !ok {
			return errfmt.Errorf("priority %q not in allowed set", job.Priority)
		}
		return nil
	}
}

// AdmissionFilterExecutionMode creates an admission filter restricting execution mode.
func AdmissionFilterExecutionMode(allowedModes ...string) JobAdmissionFilter {
	allowed := make(map[string]struct{}, len(allowedModes))
	for _, m := range allowedModes {
		allowed[m] = struct{}{}
	}
	return func(_ context.Context, job *ScheduledJob) error {
		if job == nil {
			return ErrNilJobEnvelope
		}
		if _, ok := allowed[job.ExecutionMode]; !ok {
			return errfmt.Errorf("execution mode %q not in allowed set", job.ExecutionMode)
		}
		return nil
	}
}

// AdmissionFilterNonArchived creates an admission filter rejecting archived jobs.
func AdmissionFilterNonArchived() JobAdmissionFilter {
	return func(_ context.Context, job *ScheduledJob) error {
		if job == nil {
			return ErrNilJobEnvelope
		}
		if job.Status == objects.ObjectStatusArchived {
			return errfmt.Errorf("cannot admit archived job: status=%s", job.Status)
		}
		return nil
	}
}

// AdmissionFilterEnabled creates an admission filter rejecting disabled jobs.
func AdmissionFilterEnabled() JobAdmissionFilter {
	return func(_ context.Context, job *ScheduledJob) error {
		if job == nil {
			return ErrNilJobEnvelope
		}
		if !job.Enabled || job.Status == StatusDisabled {
			return errfmt.Errorf("cannot admit disabled job")
		}
		return nil
	}
}
