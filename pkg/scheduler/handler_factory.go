package scheduler

import (
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/scheduler/transceiver"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// HandlerFactory creates job handlers with proper dependency injection
type HandlerFactory struct {
	storage              storagepkg.ObjectStorageProvider
	specLoader           *objects.SpecLoader
	lifecycleLoader      *objects.LifecycleLoader
	projectRoot          string
	objectIDCacheBuilder ObjectIDCacheBuilder // optional; for cache_prewarm (BLI-954)
	validationScanner    ValidationScanner    // optional; for cache_prewarm Tier 4 background validation scan
	logger               logging.Logger
	notificationContext  NotificationContextInterface
	asyncRouter          *transceiver.AsyncRouter
	metrics              SchedulerMetricsCollector

	cachedTestCommandDetectorMu sync.Mutex
	cachedTestCommandDetector   TestCommandDetector // reused for all run_wrapper handlers to avoid repeated List(test_command_rule)

	// scheduler is set after [NewScheduler] via [HandlerFactory.BindScheduler] so data_cell_envelope_tick
	// can optionally trigger follow-up jobs (resolved envelope tokens → job_type).
	scheduler *Scheduler
}

// BindScheduler wires the owning scheduler for handlers that need dispatch (e.g. envelope tick follow-ups).
func (f *HandlerFactory) BindScheduler(s SchedulerInterface) {
	if f == nil {
		return
	}
	if sc, ok := s.(*Scheduler); ok {
		f.scheduler = sc
	}
}

// SetValidationScanner sets the optional ValidationScanner used by cache_prewarm Tier 4
// to enqueue all cached objects for background validation after the object ID cache is built.
func (f *HandlerFactory) SetValidationScanner(scanner ValidationScanner) {
	f.validationScanner = scanner
}

// NewHandlerFactory creates a new handler factory.
// objectIDCacheBuilder is optional; pass nil and object ID cache will be built during next system check.
func NewHandlerFactory(
	storage storagepkg.ObjectStorageProvider,
	specLoader *objects.SpecLoader,
	lifecycleLoader *objects.LifecycleLoader,
	projectRoot string,
	logger logging.Logger,
	notificationContext NotificationContextInterface,
	asyncRouter *transceiver.AsyncRouter,
	metrics SchedulerMetricsCollector,
	objectIDCacheBuilder ObjectIDCacheBuilder,
) HandlerFactoryInterface {
	return &HandlerFactory{
		storage:              storage,
		specLoader:           specLoader,
		lifecycleLoader:      lifecycleLoader,
		projectRoot:          projectRoot,
		objectIDCacheBuilder: objectIDCacheBuilder,
		logger:               logger,
		notificationContext:  notificationContext,
		asyncRouter:          asyncRouter,
		metrics:              metrics,
	}
}

// CreateHandler creates the appropriate handler for a job type
func (f *HandlerFactory) CreateHandler(job *ScheduledJob) JobHandler {
	var handler JobHandler

	if builder := resolveJobTypeHandlerBuilder(job.JobType); builder != nil {
		handler = builder(f, job)
	} else {
		handler = NewNoOpHandler()
	}

	// Record metrics
	if handler != nil {
		if f.metrics != nil {
			f.metrics.RecordHandlerCreated(job.JobType)
		}
		// Bind scheduler if handler supports it (e.g., RunWrapperHandler)
		type schedulerBinder interface {
			BindScheduler(s *Scheduler)
		}
		if binder, ok := handler.(schedulerBinder); ok {
			binder.BindScheduler(f.scheduler)
		}
	}

	return handler
}

// getCachedTestCommandDetector returns a shared TestCommandDetector, loading from storage once and reusing for all run_wrapper handlers.
func (f *HandlerFactory) getCachedTestCommandDetector() TestCommandDetector {
	f.cachedTestCommandDetectorMu.Lock()
	defer f.cachedTestCommandDetectorMu.Unlock()
	if f.cachedTestCommandDetector != nil {
		return f.cachedTestCommandDetector
	}
	f.cachedTestCommandDetector = LoadTestCommandDetectorFromStorage(f.storage)
	return f.cachedTestCommandDetector
}

// createMetricsCollectionHandler creates a metrics collection handler based on job metadata
func (f *HandlerFactory) createMetricsCollectionHandler(job *ScheduledJob) JobHandler {
	// Check job title or description to determine which metrics to collect
	// For now, assume file lock metrics if title contains "file lock" or "lock"
	if job.Title != emptyValue {
		title := strings.ToLower(job.Title)
		if strings.Contains(title, "file lock") || strings.Contains(title, "lock") {
			return NewFileLockMetricsCollectionHandler(f.storage)
		}
	}
	// Default: file lock metrics (can be extended for other metric types)
	return NewFileLockMetricsCollectionHandler(f.storage)
}
