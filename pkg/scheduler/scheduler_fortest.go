package scheduler

// Package scheduler provides whitebox testing hooks and facade accessors for storage, logger,
// project root, spec loader, executor, and studio file-lease free-func fields.

import (
	"context"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/scheduler/transceiver"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// StorageForTest exposes the scheduler storage provider for facade package tests.
func (s *Scheduler) StorageForTest() storagepkg.ObjectStorageProvider {
	return s.storage
}

// SetStorageForTest replaces the scheduler storage provider (whitebox / facade tests).
func (s *Scheduler) SetStorageForTest(storage storagepkg.ObjectStorageProvider) {
	s.storage = storage
}

// LoggerForTest exposes the scheduler logger for facade package tests.
func (s *Scheduler) LoggerForTest() logging.Logger {
	return s.logger
}

// SetLoggerForTest replaces the scheduler logger (whitebox / facade tests).
func (s *Scheduler) SetLoggerForTest(logger logging.Logger) {
	s.logger = logger
}

// ProjectRootForTest exposes the scheduler project root for facade package tests.
func (s *Scheduler) ProjectRootForTest() string {
	return s.projectRoot
}

// GetProjectRootForTest exposes getProjectRoot (storage-aware) for facade package tests.
func (s *Scheduler) GetProjectRootForTest() string {
	return s.getProjectRoot()
}

// SpecLoaderForTest exposes the spec loader for facade package tests.
func (s *Scheduler) SpecLoaderForTest() *objects.SpecLoader {
	return s.specLoader
}

// SetExecutorForTest replaces the command executor (whitebox / facade tests).
func (s *Scheduler) SetExecutorForTest(executor CommandExecutor) {
	s.executor = executor
}

// ExecutorForTest exposes the command executor for facade package tests.
func (s *Scheduler) ExecutorForTest() CommandExecutor {
	return s.executor
}

// StateRegistryForTest exposes the job state registry (facade / studio lease free funcs).
func (s *Scheduler) StateRegistryForTest() JobStateRegistryInterface {
	return s.stateRegistry
}

// CoordinationChannelForTest exposes the coordination channel.
func (s *Scheduler) CoordinationChannelForTest() CoordinationChannelInterface {
	return s.coordinationChannel
}

// ProcessGroupManagerForTest exposes the process group manager.
func (s *Scheduler) ProcessGroupManagerForTest() ProcessGroupManagerInterface {
	return s.processGroupManager
}

// AsyncRouterForTest exposes the async transceiver router.
func (s *Scheduler) AsyncRouterForTest() *transceiver.AsyncRouter {
	return s.asyncRouter
}

// CreateJobAuditEventForTest exposes createJobAuditEvent for facade package tests.
func (s *Scheduler) CreateJobAuditEventForTest(ctx context.Context, eventType, jobID, jobType, category string, success bool, duration time.Duration, jobErr error) error {
	return s.createJobAuditEvent(ctx, eventType, jobID, jobType, category, success, duration, jobErr)
}

// DisableJobInStorageForTest exposes disableJobInStorage for facade package tests.
func (s *Scheduler) DisableJobInStorageForTest(ctx context.Context, job *ScheduledJob) {
	s.disableJobInStorage(ctx, job)
}
