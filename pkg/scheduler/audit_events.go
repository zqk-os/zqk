package scheduler

import (
	"context"
	"time"

	"github.com/lanceman/zqk/pkg/storage"
)

// createJobAuditEvent creates an audit event for scheduler job execution
// This is a best-effort operation - failures are logged but don't affect job execution
// Now routes through coordinator for unified event routing
func (s *Scheduler) createJobAuditEvent(ctx context.Context, eventType, jobID, jobType, category string, success bool, duration time.Duration, jobErr error) {
	// Get project root from storage (if file-based storage)
	projectRoot := s.getProjectRoot()
	if projectRoot == emptyValue {
		// Can't create audit event without project root
		SchedulerDaemonLog(s.logger).Debug(LogEventSchedulerAuditSkippedNoProjectRoot).Log()
		return
	}

	// Get storage provider - use scheduler's storage
	storageProvider := s.storage
	if storageProvider == nil {
		SchedulerDaemonLog(s.logger).Debug(LogEventSchedulerAuditSkippedNoStorage).Log()
		return
	}

	// Emit via coordinator (async, non-blocking)
	emitJobExecutionEventViaCoordinator(ctx, projectRoot, storageProvider, eventType, jobID, jobType, category, success, duration, jobErr)
	// Coordinator emits asynchronously, always succeeds
}

// getProjectRoot extracts the project root from the storage provider
// Returns empty string if project root cannot be determined
func (s *Scheduler) getProjectRoot() string {
	if s == nil {
		return ""
	}
	// Prefer file-based storage when present (authoritative for on-disk layout).
	if fileStorage, ok := s.storage.(*storage.FileObjectStorage); ok {
		if r := fileStorage.GetProjectRoot(); r != emptyValue {
			return r
		}
	}
	// Fallback: scheduler often holds projectRoot even when storage is graph-backed or unset in tests.
	return s.projectRoot
}
