package scheduler

import (
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestHandlerFactory_CreateHandler(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer env.Cleanup()

	storage := env.Storage.(storagepkg.ObjectStorageProvider)
	specLoader := env.SpecLoader
	lifecycleLoader := env.LifecycleLoader
	testRoot := env.TestRoot
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	notificationContext := NewNotificationContext(logger, nil)
	metrics := NewDefaultSchedulerMetricsCollector()

	factory := NewHandlerFactory(
		storage,
		specLoader,
		lifecycleLoader,
		testRoot,
		logger,
		notificationContext,
		nil, // asyncRouter
		metrics,
		nil, // objectIDCacheBuilder
	)

	tests := []struct {
		name    string
		jobType string
		wantNil bool
	}{
		{
			name:    "cache_prewarm handler",
			jobType: "cache_prewarm",
			wantNil: false,
		},
		{
			name:    "lifecycle_check handler",
			jobType: "lifecycle_check",
			wantNil: false,
		},
		{
			name:    "run_wrapper handler",
			jobType: "run_wrapper",
			wantNil: false,
		},
		{
			name:    "object_validation handler",
			jobType: "object_validation",
			wantNil: false,
		},
		{
			name:    "scheduler_events_aggregation handler",
			jobType: JobTypeSchedulerEventsAggregation,
			wantNil: false,
		},
		{
			name:    "convergence_session_tick handler",
			jobType: JobTypeConvergenceSessionTick,
			wantNil: false,
		},
		{
			name:    "data_cell_envelope_tick handler",
			jobType: JobTypeDataCellEnvelopeTick,
			wantNil: false,
		},
		{
			name:    "unknown job type returns NoOpHandler",
			jobType: "unknown_type",
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &ScheduledJob{
				ID:      "TEST-JOB",
				JobType: tt.jobType,
			}

			handler := factory.CreateHandler(job)
			if (handler == nil) != tt.wantNil {
				t.Errorf("CreateHandler() = %v, want nil=%v", handler, tt.wantNil)
			}

			// Verify metrics were recorded (give async time if needed)
			time.Sleep(10 * time.Millisecond)
			snapshot := metrics.GetMetrics()
			if snapshot.Handlers.Created == 0 && !tt.wantNil {
				t.Error("Expected handler creation to be recorded in metrics")
			}
		})
	}
}

func TestHandlerFactory_CreateMetricsCollectionHandler(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer env.Cleanup()

	factory := NewHandlerFactory(
		env.Storage.(storagepkg.ObjectStorageProvider),
		env.SpecLoader,
		env.LifecycleLoader,
		env.TestRoot,
		logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
		NewNotificationContext(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)), nil),
		nil,
		NewDefaultSchedulerMetricsCollector(),
		nil, // objectIDCacheBuilder
	)

	tests := []struct {
		name  string
		title string
	}{
		{
			name:  "File lock metrics from title",
			title: "File Lock Metrics Collection",
		},
		{
			name:  "Lock metrics from title",
			title: "Lock Metrics",
		},
		{
			name:  "Default metrics collection",
			title: "Some Other Metrics",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &ScheduledJob{
				ID:      "TEST-JOB",
				JobType: JobTypeMetricsCollection,
				Title:   tt.title,
			}

			handler := factory.CreateHandler(job)
			if handler == nil {
				t.Error("CreateHandler() returned nil for metrics_collection")
			}
		})
	}
}
