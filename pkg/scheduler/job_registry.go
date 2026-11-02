package scheduler

import (
	"context"
	"errors"
	"sync"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

var (
	ErrJobAlreadyRegistered = errors.New("job already registered")
)

// JobRegistry manages in-memory registration, indexing, and concurrent lookup of ScheduledJobs.
type JobRegistry struct {
	mu                sync.RWMutex
	jobs              map[string]*ScheduledJob
	admissionPipeline *JobAdmissionPipeline
}

// NewJobRegistry creates an initialized JobRegistry.
func NewJobRegistry() *JobRegistry {
	return &JobRegistry{
		jobs: make(map[string]*ScheduledJob),
	}
}

// NewJobRegistryWithPipeline creates a JobRegistry with a custom admission pipeline.
func NewJobRegistryWithPipeline(pipeline *JobAdmissionPipeline) *JobRegistry {
	return &JobRegistry{
		jobs:              make(map[string]*ScheduledJob),
		admissionPipeline: pipeline,
	}
}

// SetAdmissionPipeline updates or clears the admission pipeline for the registry.
func (r *JobRegistry) SetAdmissionPipeline(pipeline *JobAdmissionPipeline) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.admissionPipeline = pipeline
}

// Register registers a job in the registry. It validates the envelope, evaluates admission filters
// if configured, and rejects duplicate registrations.
func (r *JobRegistry) Register(job *ScheduledJob) error {
	return r.RegisterWithContext(context.Background(), job)
}

// RegisterWithContext registers a job with explicit context passed to admission filters.
func (r *JobRegistry) RegisterWithContext(ctx context.Context, job *ScheduledJob) error {
	if r == nil {
		return errfmt.Errorf("job registry is nil")
	}
	if job == nil {
		return ErrNilJobEnvelope
	}
	if job.ID == "" {
		return ErrEmptyJobID
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.jobs[job.ID]; exists {
		return errfmt.Errorf("%w: %s", ErrJobAlreadyRegistered, job.ID)
	}

	if r.admissionPipeline != nil {
		if err := r.admissionPipeline.Admit(ctx, job); err != nil {
			return err
		}
	}

	r.jobs[job.ID] = job
	return nil
}

// Unregister removes a job from the registry by ID. Returns ErrJobNotFound if not present.
func (r *JobRegistry) Unregister(jobID string) error {
	if r == nil {
		return errfmt.Errorf("job registry is nil")
	}
	if jobID == "" {
		return ErrEmptyJobID
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.jobs[jobID]; !exists {
		return errfmt.Errorf("%w: %s", ErrJobNotFound, jobID)
	}
	delete(r.jobs, jobID)
	return nil
}

// Lookup finds a job by ID in the registry.
func (r *JobRegistry) Lookup(jobID string) (*ScheduledJob, bool) {
	if r == nil || jobID == "" {
		return nil, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	job, exists := r.jobs[jobID]
	return job, exists
}

// ListJobs returns a snapshot slice of all registered jobs.
func (r *JobRegistry) ListJobs() []*ScheduledJob {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*ScheduledJob, 0, len(r.jobs))
	for _, job := range r.jobs {
		list = append(list, job)
	}
	return list
}

// Count returns the number of registered jobs.
func (r *JobRegistry) Count() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.jobs)
}

// Clear removes all jobs from the registry.
func (r *JobRegistry) Clear() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs = make(map[string]*ScheduledJob)
}
