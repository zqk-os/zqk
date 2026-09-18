package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/httpheaders"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/when"
)

// Context key types for avoiding collisions
//
//nolint:unused // Reserved for future authentication features
type authSubjectKey struct{}

//
//nolint:unused // Reserved for future authentication features
type authPermissionsKey struct{}

// CallbackListenerHandler runs a lightweight HTTP server to receive callbacks and route them
type CallbackListenerHandler struct {
	storage             storagepkg.ObjectStorageProvider
	logger              logging.Logger
	scheduler           SchedulerInterface           // Reference to scheduler for triggering jobs
	authHook            AuthHook                     // Authentication hook for validating requests
	notificationContext NotificationContextInterface // Notification context
	server              *http.Server
	serverMu            sync.RWMutex
	activeJobs          map[string]time.Time // Track active jobs to determine if idle
	activeJobsMu        sync.RWMutex
	lastActivity        time.Time
	lastActivityMu      sync.RWMutex
}

// NewCallbackListenerHandler creates a new callback listener handler
func NewCallbackListenerHandler(storage storagepkg.ObjectStorageProvider, logger logging.Logger, scheduler SchedulerInterface, authHook AuthHook, notificationContext NotificationContextInterface) CallbackListenerHandlerInterface {
	// If no context provided, create a default one
	if notificationContext == nil {
		notificationContext = NewNotificationContext(logger, nil)
	}

	return &CallbackListenerHandler{
		storage:             storage,
		logger:              logger,
		scheduler:           scheduler,
		authHook:            authHook,
		notificationContext: notificationContext,
		activeJobs:          make(map[string]time.Time),
		lastActivity:        time.Now(),
	}
}

// StopNotificationContext stops the notification context if it was created by this handler
// This is used for test cleanup to prevent goroutine leaks
func (h *CallbackListenerHandler) StopNotificationContext() {
	if h.notificationContext != nil {
		h.notificationContext.Stop()
	}
}

// Execute starts the callback listener server
func (h *CallbackListenerHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return RunCallbackListenerViaPipeline(ctx, h, job)
}

// BuildHTTPServer creates an http.Server configured with standard timeouts for callback listener.
func (h *CallbackListenerHandler) BuildHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second, // Prevent Slowloris attacks
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// executeCallbackListenerCore starts the callback listener server.
// Called from RunCallbackListenerViaPipeline NORMALIZE stage.
func (h *CallbackListenerHandler) executeCallbackListenerCore(ctx context.Context, job *ScheduledJob) error {
	// Get listener configuration
	port := job.ListenerPort
	if port == 0 {
		port = 8080 // Default port
	}
	basePath := job.ListenerPath
	if basePath == emptyValue {
		basePath = "/callbacks" // Default path
	}
	idleTimeout := time.Duration(job.IdleShutdownSeconds) * time.Second
	if idleTimeout == 0 {
		idleTimeout = 5 * time.Minute // Default: 5 minutes
	}

	// Default to loopback — never expose auth-free / lightly-auth'd callbacks
	// on all interfaces (REQ-CEF-SEC-001 / CRIT-CEF-SEC-001A).
	addr := loopbackListenAddr(port)

	CallbackListenerLog(h.logger).Info(LogEventCallbackListenerStarted).
		JobID(job.ID).
		String("address", addr).
		String("base_path", basePath).
		String("idle_timeout", idleTimeout.String()).
		Log()

	// Create HTTP server
	mux := http.NewServeMux()

	// Register routes
	h.registerRoutes(mux, basePath, job)

	server := h.BuildHTTPServer(addr, mux)

	if err := concurrency.RunInLockWithLogger(
		&h.serverMu, LockNameCallbackListenerSetServer, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			h.server = server
			return nil
		},
	); err != nil {
		SLog(h.logger).Error("Failed to set callback listener server under lock", err).Log()
	}

	// Start idle shutdown monitor
	idleMonitorCtx, idleCancel := context.WithCancel(ctx)
	defer idleCancel()
	goroutinelabels.NewGoroutine("callback_listener_idle_monitor", fmt.Sprintf("monitoring idle shutdown for job %s", job.ID)).
		StartWithContext(idleMonitorCtx, func(ctx context.Context) error {
			h.monitorIdleShutdown(ctx, idleTimeout, job.ID)
			return nil
		})

	// Start server in goroutine
	// Use context so goroutine can be tracked and cancelled
	serverErr := make(chan error, 1)
	goroutinelabels.NewGoroutine("callback_listener_server", fmt.Sprintf("listening for callbacks on job %s", job.ID)).
		StartWithContext(ctx, func(ctx context.Context) error {
			// ListenAndServe blocks until server is shut down
			// We handle shutdown via shutdownServer() when context is cancelled
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				select {
				case serverErr <- err:
				case <-ctx.Done():
					// Context cancelled, ignore error
				}
			}
			return nil
		})

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		CallbackListenerLog(h.logger).Info(LogEventCallbackListenerShuttingDownContext).
			JobID(job.ID).
			String("reason", "context_cancelled").
			Log()
		h.shutdownServer()
		return nil
	case err := <-serverErr:
		CallbackListenerLog(h.logger).Error(LogEventCallbackListenerServerError, err).
			JobID(job.ID).
			Log()
		return errfmt.Newf("server error").Wrap(err)
	}
}

// registerRoutes registers HTTP routes for the callback listener
func (h *CallbackListenerHandler) registerRoutes(mux *http.ServeMux, basePath string, job *ScheduledJob) {
	// Default routes
	routes := map[string]string{
		"/job/complete": "job_complete",
		"/job/error":    "job_error",
		"/job/status":   "job_status",
		"/trigger":      "trigger_job",
		"/event":        "emit_event",
	}

	// Override with configured route handlers
	if len(job.RouteHandlers) > 0 {
		for route, handler := range job.RouteHandlers {
			routes[route] = handler
		}
	}

	// Register each route
	for route, handlerType := range routes {
		fullPath := basePath + route
		CallbackListenerLog(h.logger).Debug(LogEventCallbackListenerRegisteringRoute).
			String("path", fullPath).
			String("handler", handlerType).
			Log()

		mux.HandleFunc(fullPath, func(w http.ResponseWriter, r *http.Request) {
			h.handleCallback(w, r, handlerType, job)
		})
	}

	// Health check endpoint (no auth required)
	mux.HandleFunc(basePath+"/health", func(w http.ResponseWriter, r *http.Request) {
		h.handleHealthCheck(w, r)
	})
}

// authenticateRequest authenticates the incoming request using the auth hook
//
//nolint:unused // Reserved for future authentication features
func (h *CallbackListenerHandler) authenticateRequest(w http.ResponseWriter, r *http.Request) bool {
	if h.authHook == nil {
		// No auth hook configured - allow request (for testing)
		return true
	}

	ctx := r.Context()
	authenticated, subject, permissions, err := h.authHook.Authenticate(ctx, r)

	if err != nil {
		CallbackListenerLog(h.logger).Warn(LogEventCallbackListenerAuthError).
			WithFields(append([]logging.Field{logging.String("method", r.Method), logging.PathField(r.URL.Path)}, logErrField(err)...)...).
			Log()
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return false
	}

	if !authenticated {
		CallbackListenerLog(h.logger).Warn(LogEventCallbackListenerUnauthenticatedRequest).
			String("method", r.Method).
			String("path", r.URL.Path).
			String("remote_addr", r.RemoteAddr).
			Log()
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}

	// Log successful authentication
	CallbackListenerLog(h.logger).Debug(LogEventCallbackListenerAuthenticatedRequest).
		String("subject", subject).
		String("permissions", fmt.Sprintf("%v", permissions)).
		String("path", r.URL.Path).
		Log()

	// Store subject and permissions in request context for use by handlers
	// (could be used for authorization checks in specific handlers)
	ctx = context.WithValue(ctx, authSubjectKey{}, subject)
	ctx = context.WithValue(ctx, authPermissionsKey{}, permissions)
	*r = *r.WithContext(ctx)

	return true
}

// handleCallback processes incoming callback requests
func (h *CallbackListenerHandler) handleCallback(w http.ResponseWriter, r *http.Request, handlerType string, job *ScheduledJob) {
	// Update last activity
	if err := concurrency.RunInLockWithLogger(
		&h.lastActivityMu, LockNameCallbackListenerUpdateActivity, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			h.lastActivity = time.Now()
			return nil
		},
	); err != nil {
		SLog(h.logger).Debug("Failed to update last activity under lock").WithError(err).Log()
	}

	// Parse request body
	var payload map[string]any
	if r.Body != nil {
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&payload); err != nil {
			CallbackListenerLog(h.logger).Warn(LogEventCallbackListenerParsePayloadFailed).
				WithFields(append([]logging.Field{logging.JobIDField(job.ID), logging.HandlerField(handlerType)}, logErrField(err)...)...).
				Log()
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}
	}

	CallbackListenerLog(h.logger).Info(LogEventCallbackListenerReceivedCallback).
		JobID(job.ID).
		String("handler", handlerType).
		String("method", r.Method).
		String("path", r.URL.Path).
		Log()

	// Route to appropriate handler
	switch handlerType {
	case "job_complete":
		h.handleJobComplete(w, r, payload, job)
	case "job_error":
		h.handleJobError(w, r, payload, job)
	case "job_status":
		h.handleJobStatus(w, r, payload, job)
	case "trigger_job":
		h.handleTriggerJob(w, r, payload, job)
	case "emit_event":
		h.handleEmitEvent(w, r, payload, job)
	default:
		CallbackListenerLog(h.logger).Warn(LogEventCallbackListenerUnknownHandlerType).
			JobID(job.ID).
			String("handler", handlerType).
			Log()
		http.Error(w, "Unknown handler type", http.StatusBadRequest)
		return
	}

	// Mark activity
	h.updateActivity(payload)
}

// handleJobComplete handles job completion callbacks
func (h *CallbackListenerHandler) handleJobComplete(w http.ResponseWriter, _ *http.Request, payload map[string]any, job *ScheduledJob) {
	jobID, _ := payload["job_id"].(string)
	if jobID == emptyValue {
		http.Error(w, "job_id required", http.StatusBadRequest)
		return
	}

	CallbackListenerLog(h.logger).Info(LogEventCallbackListenerJobCompleteReceived).
		String("listener_job_id", job.ID).
		String("target_job_id", jobID).
		Log()

	// Extract duration and metadata from payload
	var duration time.Duration
	if durSec, ok := payload["duration"].(float64); ok {
		duration = time.Duration(durSec) * time.Second
	}
	jobType, _ := payload[objects.FieldKeyJobType].(string)
	category, _ := payload[objects.FieldKeyCategory].(string)

	// Display user notification
	notif := CreateJobNotification(
		jobID,
		jobType,
		category,
		"completed",
		PriorityMedium,
		duration,
		nil,
		payload,
	)
	h.notificationContext.Notify(notif)

	// Could trigger lifecycle transitions, update job status, etc.
	// For now, just acknowledge
	w.WriteHeader(http.StatusOK)
	//nolint:errcheck // HTTP response encoding errors are non-critical
	_ = json.NewEncoder(w).Encode(map[string]any{
		objects.FieldKeyStatus: "acknowledged",
		"job_id":               jobID,
		"message":              "Job completion callback received",
	})
}

// handleJobError handles job error callbacks
func (h *CallbackListenerHandler) handleJobError(w http.ResponseWriter, _ *http.Request, payload map[string]any, job *ScheduledJob) {
	jobID, _ := payload["job_id"].(string)
	if jobID == emptyValue {
		http.Error(w, "job_id required", http.StatusBadRequest)
		return
	}

	CallbackListenerLog(h.logger).Info(LogEventCallbackListenerJobErrorReceived).
		String("listener_job_id", job.ID).
		String("target_job_id", jobID).
		Log()

	// Extract error and metadata from payload
	var err error
	if errStr, ok := payload["error"].(string); ok && errStr != emptyValue {
		err = errfmt.Errorf("%s", errStr) // Use constant format string
	}
	jobType, _ := payload[objects.FieldKeyJobType].(string)
	category, _ := payload[objects.FieldKeyCategory].(string)

	// Display user notification (high priority for errors)
	notif := CreateJobNotification(
		jobID,
		jobType,
		category,
		"failed",
		PriorityHigh,
		0, // Duration not always available in error callbacks
		err,
		payload,
	)
	h.notificationContext.Notify(notif)

	// Could trigger error handling workflows, notifications, etc.
	w.WriteHeader(http.StatusOK)
	//nolint:errcheck // HTTP response encoding errors are non-critical
	_ = json.NewEncoder(w).Encode(map[string]any{
		objects.FieldKeyStatus: "acknowledged",
		"job_id":               jobID,
		"message":              "Job error callback received",
	})
}

// handleJobStatus handles incremental status updates
func (h *CallbackListenerHandler) handleJobStatus(w http.ResponseWriter, _ *http.Request, payload map[string]any, job *ScheduledJob) {
	jobID, _ := payload["job_id"].(string)
	status, _ := payload[objects.FieldKeyStatus].(string)

	CallbackListenerLog(h.logger).Debug(LogEventCallbackListenerJobStatusReceived).
		String("listener_job_id", job.ID).
		String("target_job_id", jobID).
		String(objects.FieldKeyStatus, status).
		Log()

	// Update activity tracking
	if jobID != emptyValue {
		if err := concurrency.RunInLockWithLogger(
			&h.activeJobsMu, LockNameCallbackListenerTrackJob, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				h.activeJobs[jobID] = time.Now()
				return nil
			},
		); err != nil {
			SLog(h.logger).Debug("Failed to track active job under lock").WithError(err).Log()
		}
	}

	w.WriteHeader(http.StatusOK)
	//nolint:errcheck // HTTP response encoding errors are non-critical
	_ = json.NewEncoder(w).Encode(map[string]any{
		objects.FieldKeyStatus: "acknowledged",
		"job_id":               jobID,
		"message":              "Status update received",
	})
}

// handleTriggerJob handles job trigger requests
func (h *CallbackListenerHandler) handleTriggerJob(w http.ResponseWriter, r *http.Request, payload map[string]any, job *ScheduledJob) {
	jobID, _ := payload["job_id"].(string)
	if jobID == emptyValue {
		http.Error(w, "job_id required", http.StatusBadRequest)
		return
	}

	CallbackListenerLog(h.logger).Info(LogEventCallbackListenerTriggerReceived).
		String("listener_job_id", job.ID).
		String("target_job_id", jobID).
		Log()

	// Trigger the job via scheduler
	ctx := r.Context()
	if err := h.scheduler.TriggerJob(ctx, jobID); err != nil {
		CallbackListenerLog(h.logger).Warn(LogEventCallbackListenerTriggerJobFailed).
			WithFields(append([]logging.Field{logging.String("listener_job_id", job.ID), logging.String("target_job_id", jobID)}, logErrField(err)...)...).
			Log()
		http.Error(w, "Failed to trigger job: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Mark activity
	h.updateActivity(payload)

	w.WriteHeader(http.StatusOK)
	//nolint:errcheck // HTTP response encoding errors are non-critical
	_ = json.NewEncoder(w).Encode(map[string]any{
		objects.FieldKeyStatus: "triggered",
		"job_id":               jobID,
		"message":              "Job triggered successfully",
	})
}

// handleEmitEvent handles event emission requests
func (h *CallbackListenerHandler) handleEmitEvent(w http.ResponseWriter, r *http.Request, payload map[string]any, job *ScheduledJob) {
	eventType, _ := payload[objects.FieldKeyEventType].(string)
	if eventType == emptyValue {
		http.Error(w, "event_type required", http.StatusBadRequest)
		return
	}

	CallbackListenerLog(h.logger).Info(LogEventCallbackListenerEmitEventReceived).
		String("listener_job_id", job.ID).
		EventType(eventType).
		Log()

	// Could trigger event-based jobs, workflows, etc.
	// For now, trigger jobs that match event filters
	ctx := r.Context()
	// TriggerJobByEvent signature: (ctx, eventType, eventFilter, payload)
	// For now, use eventType as the filter
	if err := h.scheduler.TriggerJobByEvent(ctx, eventType, eventType, payload); err != nil {
		CallbackListenerLog(h.logger).Warn(LogEventCallbackListenerEmitEventReceived).
			String("reason", "failed to trigger jobs by event").
			WithError(err).
			Log()
	}

	w.WriteHeader(http.StatusOK)
	//nolint:errcheck // HTTP response encoding errors are non-critical
	_ = json.NewEncoder(w).Encode(map[string]any{
		objects.FieldKeyStatus:    "emitted",
		objects.FieldKeyEventType: eventType,
		"message":                 "Event emitted successfully",
	})
}

// handleHealthCheck handles health check requests
func (h *CallbackListenerHandler) handleHealthCheck(w http.ResponseWriter, _ *http.Request) {
	var activeCount int
	var lastActivity time.Time
	if err := concurrency.RunInRLockWithLogger(
		&h.activeJobsMu, LockNameCallbackListenerHealthCheckActive, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			activeCount = len(h.activeJobs)
			return nil
		},
	); err != nil {
		SLog(h.logger).Debug("Failed to check active jobs under rlock").WithError(err).Log()
	}
	if err := concurrency.RunInRLockWithLogger(
		&h.lastActivityMu, LockNameCallbackListenerHealthCheckActivity, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			lastActivity = h.lastActivity
			return nil
		},
	); err != nil {
		SLog(h.logger).Debug("Failed to check last activity under rlock").WithError(err).Log()
	}

	w.Header().Set(httpheaders.ContentType, "application/json")
	w.WriteHeader(http.StatusOK)
	//nolint:errcheck // HTTP response encoding errors are non-critical
	_ = json.NewEncoder(w).Encode(map[string]any{
		objects.FieldKeyStatus:       "healthy",
		"active_jobs":                activeCount,
		objects.FieldKeyLastActivity: lastActivity.Format(time.RFC3339),
	})
}

// updateActivity updates activity tracking
func (h *CallbackListenerHandler) updateActivity(payload map[string]any) {
	if err := concurrency.RunInLockWithLogger(
		&h.lastActivityMu, LockNameCallbackListenerUpdateActivity, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			h.lastActivity = time.Now()
			return nil
		},
	); err != nil {
		SLog(h.logger).Debug("Failed to update activity under lock").WithError(err).Log()
	}

	// Track active jobs if job_id is present
	if jobID, ok := payload["job_id"].(string); ok && jobID != emptyValue {
		if err := concurrency.RunInLockWithLogger(
			&h.activeJobsMu, LockNameCallbackListenerTrackJob, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				h.activeJobs[jobID] = time.Now()
				return nil
			},
		); err != nil {
			SLog(h.logger).Debug("Failed to track job under lock").WithError(err).Log()
		}
	}
}

// monitorIdleShutdown monitors for idle state and shuts down server if idle
func (h *CallbackListenerHandler) monitorIdleShutdown(ctx context.Context, idleTimeout time.Duration, jobID string) {
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check if idle
			var timeSinceActivity time.Duration
			var activeCount int
			if err := concurrency.RunInRLockWithLogger(
				&h.lastActivityMu, LockNameCallbackListenerIdleCheckActivity, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					timeSinceActivity = time.Since(h.lastActivity)
					return nil
				},
			); err != nil {
				SLog(h.logger).Debug("Failed to check activity under rlock").WithError(err).Log()
			}
			if err := concurrency.RunInRLockWithLogger(
				&h.activeJobsMu, LockNameCallbackListenerIdleCheckJobs, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					activeCount = len(h.activeJobs)
					return nil
				},
			); err != nil {
				SLog(h.logger).Debug("Failed to check active jobs under rlock").WithError(err).Log()
			}

			// If no active jobs and idle timeout exceeded, shutdown
			if activeCount == 0 && timeSinceActivity >= idleTimeout {
				CallbackListenerLog(h.logger).Info(LogEventCallbackListenerIdleShutdown).
					JobID(jobID).
					IdleDuration(timeSinceActivity.String()).
					Log()
				h.shutdownServer()
				return
			}
		}
	}
}

// shutdownServer gracefully shuts down the HTTP server
func (h *CallbackListenerHandler) shutdownServer() {
	var server *http.Server
	if err := concurrency.RunInRLockWithLogger(
		&h.serverMu, LockNameCallbackListenerShutdown, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			server = h.server
			return nil
		},
	); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Debug("Failed to retrieve server for shutdown under rlock").WithError(err).Log()
	}

	if server == nil {
		return
	}

	// Create shutdown context with 5 second timeout
	// Use system context for shutdown timeout
	shutdownCtx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()

	err := server.Shutdown(shutdownCtx)
	when.When(func() bool { return err == nil }).Then(func() {
		CallbackListenerLog(h.logger).Info(LogEventCallbackListenerShutdownGraceful).Log()
	}).OrElse(func() {
		CallbackListenerLog(h.logger).Warn(LogEventCallbackListenerShutdownError).
			WithFields(logErrField(err)...).
			Log()
	}).Run()
}
