package scheduler

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage"
)

// createJobAuditEvent creates an audit event for scheduler job execution.
// Job lifecycle audit events are fail-closed (BLI-TDE-AUDIT-FAILCLOSED-001).
func (s *Scheduler) createJobAuditEvent(ctx context.Context, eventType, jobID, jobType, category string, success bool, duration time.Duration, jobErr error) error {
	logger := s.logger
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}

	// Get project root from storage (if file-based storage)
	projectRoot := s.getProjectRoot()
	if projectRoot == emptyValue {
		// Can't create audit event without project root
		SchedulerDaemonLog(logger).Debug(LogEventSchedulerAuditSkippedNoProjectRoot).Log()
		return fmt.Errorf("cannot create audit event %s for job %s: empty project root", eventType, jobID)
	}

	// Get storage provider - use scheduler's storage
	storageProvider := s.storage
	if storageProvider == nil {
		SchedulerDaemonLog(logger).Debug(LogEventSchedulerAuditSkippedNoStorage).Log()
		return fmt.Errorf("cannot create audit event %s for job %s: nil storage provider", eventType, jobID)
	}

	// Emit via coordinator (fail-closed AUD persist before cache dual-write)
	return emitJobExecutionEventViaCoordinator(ctx, projectRoot, storageProvider, eventType, jobID, jobType, category, success, duration, jobErr)
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
