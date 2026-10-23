package scheduler

// TRACK: BLI-1790209618294719000-90cdbe5f — Scheduler ForTest seams for studio facade alias
// (unblocks JobTriggerQueue / HandlerFactory / DataCellEnvelopeTick peels).

import (
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
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
