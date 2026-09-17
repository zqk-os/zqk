package scheduler

import (
	"context"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/scheduler/transceiver"
	"github.com/lanceman/zqk/pkg/scheduler/transceiver/types"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

// RunWrapperHandler executes external commands with timeout and retry logic
type RunWrapperHandler struct {
	storage             storagepkg.ObjectStorageProvider
	logger              logging.Logger
	notificationContext NotificationContextInterface
	asyncRouter         *transceiver.AsyncRouter
	processGroupManager ProcessGroupManagerInterface // Manages all subprocesses spawned by this handler
	projectRoot         string                       // Project root for writing job logs
	testCommandDetector TestCommandDetector          // Configurable: detects test commands for notifications/sanitization
	failureCount        int64                        // Track consecutive failures to implement circuit breaking
	scheduler           SchedulerInterface           // Reference to scheduler for job quarantine
}

// BindScheduler wires the owning scheduler for handlers that need dispatch (e.g. envelope tick follow-ups).
func (h *RunWrapperHandler) BindScheduler(s SchedulerInterface) {
	h.scheduler = s
}

// NewRunWrapperHandler creates a new run wrapper handler
func NewRunWrapperHandler(storage storagepkg.ObjectStorageProvider, logger logging.Logger, notificationContext NotificationContextInterface, asyncRouter *transceiver.AsyncRouter) RunWrapperHandlerInterface {
	return NewRunWrapperHandlerWithProjectRoot(storage, logger, notificationContext, asyncRouter, "")
}

// NewRunWrapperHandlerWithProjectRoot creates a new run wrapper handler with explicit project root.
func NewRunWrapperHandlerWithProjectRoot(storage storagepkg.ObjectStorageProvider, logger logging.Logger, notificationContext NotificationContextInterface, asyncRouter *transceiver.AsyncRouter, projectRoot string) RunWrapperHandlerInterface {
	return NewRunWrapperHandlerWithProjectRootAndDetector(storage, logger, notificationContext, asyncRouter, projectRoot, nil)
}

// NewRunWrapperHandlerWithProjectRootAndDetector creates a run wrapper handler with an optional
// pre-built test command detector.
func NewRunWrapperHandlerWithProjectRootAndDetector(storage storagepkg.ObjectStorageProvider, logger logging.Logger, notificationContext NotificationContextInterface, asyncRouter *transceiver.AsyncRouter, projectRoot string, detector TestCommandDetector) RunWrapperHandlerInterface {
	// If no context provided, create a default one
	if notificationContext == nil {
		notificationContext = NewNotificationContext(logger, nil)
	}

	// Create process group manager with 30 second shutdown timeout
	// This allows critical operations to complete, but prevents hanging
	// Note: Handler may be created before command context exists, use system context
	processGroupManager := NewProcessGroupManager(pkgctx.NewSystemContext(), 30*time.Second)

	// If project root not provided, try to get from file-based storage
	if projectRoot == emptyValue {
		if fileStorage, ok := storage.(*storagepkg.FileObjectStorage); ok {
			projectRoot = fileStorage.GetProjectRoot()
		}
	}

	if detector == nil {
		detector = LoadTestCommandDetectorFromStorage(storage)
	}

	return &RunWrapperHandler{
		storage:             storage,
		logger:              logger,
		notificationContext: notificationContext,
		asyncRouter:         asyncRouter,
		processGroupManager: processGroupManager,
		projectRoot:         projectRoot,
		testCommandDetector: detector,
	}
}

// StopNotificationContext stops the notification context if it was created by this handler
// This is used for test cleanup to prevent goroutine leaks
func (h *RunWrapperHandler) StopNotificationContext() {
	if h.notificationContext != nil {
		h.notificationContext.Stop()
	}
}

// routeAsync sends a message through the async router and logs (non-fatal) on failure
//
//nolint:gocritic // Message passed by value to avoid shared mutation across goroutines
func (h *RunWrapperHandler) routeAsync(ctx context.Context, message types.Message) {
	if h.asyncRouter == nil {
		return
	}

	jobID := ""
	if message.Metadata != nil {
		jobID = message.Metadata["job_id"]
	}

	if err := RouteJobMessageAsync(ctx, h.asyncRouter, message); err != nil {
		RunWrapperLog(h.logger).Warn(LogEventRunWrapperRouteJobMessageFailed).
			JobID(jobID).
			WithError(err).
			EventType(message.EventType).
			Log()
	}
}
