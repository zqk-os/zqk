package community

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

var (
	// ErrBlankWorkerID is returned when a worker identifier is blank.
	ErrBlankWorkerID = errors.New("supervisor: worker identifier cannot be blank")
	// ErrWorkerNotFound is returned when querying a non-existent worker.
	ErrWorkerNotFound = errors.New("supervisor: worker not found")
	// ErrWorkerQuarantined is returned when attempting operations on a quarantined worker.
	ErrWorkerQuarantined = errors.New("supervisor: worker is quarantined due to crash loop")
	// ErrInvalidSpec is returned when a worker spec violates boundary invariants.
	ErrInvalidSpec = errors.New("supervisor: invalid worker process specification")
)

// WorkerStatus represents the operational lifecycle state of a supervised worker.
type WorkerStatus string

const (
	WorkerStatusHealthy     WorkerStatus = "healthy"
	WorkerStatusDegraded    WorkerStatus = "degraded"
	WorkerStatusRestarting  WorkerStatus = "restarting"
	WorkerStatusQuarantined WorkerStatus = "quarantined"
)

// WorkerProcessSpec defines supervision parameters for a worker process.
type WorkerProcessSpec struct {
	WorkerID         string        `json:"worker_id"`
	BinaryPath       string        `json:"binary_path"`
	HeartbeatTimeout time.Duration `json:"heartbeat_timeout"`
	MaxCrashLimit    int           `json:"max_crash_limit"`
	BaseBackoff      time.Duration `json:"base_backoff"`
	MaxBackoff       time.Duration `json:"max_backoff"`
}

// Validate checks spec invariant boundaries.
func (spec *WorkerProcessSpec) Validate() error {
	if spec == nil {
		return fmt.Errorf("%w: spec cannot be nil", ErrInvalidSpec)
	}
	if err := strutil.ErrorOnBlank(spec.WorkerID, "%w: worker_id is required", ErrBlankWorkerID); err != nil {
		return err
	}
	if err := strutil.ErrorOnBlank(spec.BinaryPath, "%w: binary_path is required", ErrInvalidSpec); err != nil {
		return err
	}
	if spec.HeartbeatTimeout <= 0 {
		return fmt.Errorf("%w: heartbeat_timeout must be positive", ErrInvalidSpec)
	}
	if spec.MaxCrashLimit <= 0 {
		return fmt.Errorf("%w: max_crash_limit must be positive", ErrInvalidSpec)
	}
	if spec.BaseBackoff <= 0 {
		return fmt.Errorf("%w: base_backoff must be positive", ErrInvalidSpec)
	}
	if spec.MaxBackoff < spec.BaseBackoff {
		return fmt.Errorf("%w: max_backoff cannot be less than base_backoff", ErrInvalidSpec)
	}
	return nil
}

// SupervisedWorker tracks state and restart history for a supervised worker.
type SupervisedWorker struct {
	Spec          WorkerProcessSpec `json:"spec"`
	Status        WorkerStatus      `json:"status"`
	LastHeartbeat time.Time         `json:"last_heartbeat"`
	CrashCount    int               `json:"crash_count"`
	RestartCount  int               `json:"restart_count"`
	NextRestartAt time.Time         `json:"next_restart_at"`
}

// WorkerSupervisor manages health and self-healing for worker processes.
type WorkerSupervisor struct {
	mu      sync.RWMutex
	workers map[string]*SupervisedWorker
}

// NewWorkerSupervisor creates a new WorkerSupervisor instance.
func NewWorkerSupervisor() *WorkerSupervisor {
	return &WorkerSupervisor{
		workers: make(map[string]*SupervisedWorker),
	}
}

// RegisterWorker registers a new worker process spec under supervision.
func (s *WorkerSupervisor) RegisterWorker(spec WorkerProcessSpec) error {
	if err := spec.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.workers[spec.WorkerID] = &SupervisedWorker{
		Spec:          spec,
		Status:        WorkerStatusHealthy,
		LastHeartbeat: time.Now(),
	}
	return nil
}

// RecordHeartbeat refreshes the last heartbeat timestamp for a worker.
func (s *WorkerSupervisor) RecordHeartbeat(workerID string) error {
	if err := strutil.ErrorOnBlank(workerID, "%w: worker_id is required", ErrBlankWorkerID); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	w, exists := s.workers[workerID]
	if !exists {
		return ErrWorkerNotFound
	}
	if w.Status == WorkerStatusQuarantined {
		return ErrWorkerQuarantined
	}

	w.LastHeartbeat = time.Now()
	if w.Status == WorkerStatusDegraded || w.Status == WorkerStatusRestarting {
		w.Status = WorkerStatusHealthy
		w.CrashCount = 0
	}
	return nil
}

// EvaluateHealth inspects all workers against current time, updating status.
func (s *WorkerSupervisor) EvaluateHealth(now time.Time) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var degradedWorkers []string
	for id, w := range s.workers {
		if w.Status == WorkerStatusQuarantined {
			continue
		}
		if now.Sub(w.LastHeartbeat) > w.Spec.HeartbeatTimeout {
			w.Status = WorkerStatusDegraded
			degradedWorkers = append(degradedWorkers, id)
		}
	}
	return degradedWorkers
}

// ReportCrash handles worker crash events and calculates exponential restart backoff.
func (s *WorkerSupervisor) ReportCrash(workerID string, now time.Time) (time.Duration, error) {
	if err := strutil.ErrorOnBlank(workerID, "%w: worker_id is required", ErrBlankWorkerID); err != nil {
		return 0, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	w, exists := s.workers[workerID]
	if !exists {
		return 0, ErrWorkerNotFound
	}
	if w.Status == WorkerStatusQuarantined {
		return 0, ErrWorkerQuarantined
	}

	w.CrashCount++
	if w.CrashCount >= w.Spec.MaxCrashLimit {
		w.Status = WorkerStatusQuarantined
		return 0, ErrWorkerQuarantined
	}

	// Exponential backoff: base * 2^(crashCount - 1)
	multiplier := 1 << (w.CrashCount - 1)
	backoff := w.Spec.BaseBackoff * time.Duration(multiplier)
	if backoff > w.Spec.MaxBackoff {
		backoff = w.Spec.MaxBackoff
	}

	w.Status = WorkerStatusRestarting
	w.RestartCount++
	w.NextRestartAt = now.Add(backoff)
	return backoff, nil
}

// GetWorkerStatus returns a snapshot of a worker's supervised state.
func (s *WorkerSupervisor) GetWorkerStatus(workerID string) (SupervisedWorker, error) {
	if err := strutil.ErrorOnBlank(workerID, "%w: worker_id is required", ErrBlankWorkerID); err != nil {
		return SupervisedWorker{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	w, exists := s.workers[workerID]
	if !exists {
		return SupervisedWorker{}, ErrWorkerNotFound
	}
	return *w, nil
}
