package coordination

import (
	"context"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	"github.com/zqk-os/zqk/pkg/testkit"
)

// TestUnifiedMetricsCollector_Integration tests the unified metrics collector with real components
func TestUnifiedMetricsCollector_Integration(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - requires background goroutine shutdown")
	}
	tmpDir := t.TempDir()
	ensureObjectSpecsForCoordTest(t, tmpDir)

	storageProvider, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, storageProvider)

	metricsPipeline := metrics.NewMetricPipeline(storageProvider)

	coordinator := NewCoordinator(CoordinatorConfig{
		LoggingRouter:     &DefaultLoggingRouter{},
		AuditRouter:       NewStorageAuditRouter(tmpDir, storageProvider),
		MetricsRouter:     NewMetricPipelineRouter(metricsPipeline),
		OperationalRouter: &DefaultOperationalRouter{},
	})

	unifiedCollectorAny := CreateUnifiedMetricsCollector(
		coordinator,
		storageProvider,
		nil,
		false,
		false,
	)
	unifiedCollector := unifiedCollectorAny.(*storage.UnifiedMetricsCollector)

	metricsrecording.EnterAllowRecording()
	t.Cleanup(metricsrecording.LeaveAllowRecording)

	t.Cleanup(func() {
		if err := storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, storageProvider)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})
	t.Cleanup(func() {
		unifiedCollector.Stop()
		if metricsPipeline != nil && metricsPipeline.GetSamplerRegistry() != nil {
			_ = metricsPipeline.GetSamplerRegistry().StopAll() //nolint:errcheck // Test cleanup
		}
		buffer := storage.GetGlobalAuditEventBuffer()
		if buffer != nil {
			_ = buffer.Shutdown() //nolint:errcheck // Test cleanup
		}
	})

	// Set up context and time window
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	windowStart := time.Now().Add(-1 * time.Hour)
	windowEnd := time.Now()

	// Reset metrics first to avoid conflicts from previous test runs
	caspkg.ResetObjectStorageMetrics()
	storage.ResetFileLockMetrics()

	// Create a new audit metrics collector with storage (global one may not have storage)
	auditMetrics := storage.NewAuditMetricsCollector(storageProvider)
	// Reset by collecting with empty window
	_, _ = auditMetrics.CollectAndReset(ctx, secCtx, windowStart.Add(-2*time.Hour), windowStart)

	// Record some CAS metrics
	casMetrics := caspkg.GetObjectStorageMetrics()
	casMetrics.RecordCreate(10*time.Millisecond, nil)
	casMetrics.RecordRead(5*time.Millisecond, nil)
	casMetrics.RecordSetMapping(2*time.Millisecond, nil, false)

	// Record some file lock metrics
	fileLockMetrics := storage.GetFileLockMetrics()
	fileLockMetrics.RecordAcquisition(1 * time.Millisecond)
	fileLockMetrics.RecordContention()

	// Record some audit metrics
	auditMetrics.RecordAuditEventCreation(ctx, "object_creation", true, 3*time.Millisecond, true)

	metricIDs, err := unifiedCollector.CollectAllMetrics(ctx, secCtx, windowStart, windowEnd)
	if err != nil {
		// Some metrics may fail (e.g., if no operations recorded), but we should get at least some
		t.Logf("Some metrics collection failed (expected in some cases): %v", err)
	}

	// Verify metrics were collected (at least some should succeed)
	if len(metricIDs) == 0 {
		t.Error("Expected at least 1 metric ID, got 0")
	}

	// Log what we got
	t.Logf("Collected %d metric IDs: %v", len(metricIDs), metricIDs)

	// Verify metrics objects were created
	for _, metricID := range metricIDs {
		if metricID == emptyValue {
			t.Error("Expected non-empty metric ID")
		}
		// Try to read the metric object
		obj, err := storageProvider.Read(ctx, secCtx, metricID)
		if err != nil {
			t.Errorf("Failed to read metric object %s: %v", metricID, err)
		} else if obj == nil {
			t.Errorf("Metric object %s not found", metricID)
		}
	}
}

// TestUnifiedMetricsCollector_EventEmission tests that events are emitted via coordinator
func TestUnifiedMetricsCollector_EventEmission(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - requires full system shutdown")
	}
	tmpDir := t.TempDir()
	ensureObjectSpecsForCoordTest(t, tmpDir)

	storageProvider, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, storageProvider)

	t.Cleanup(func() {
		if err := storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, storageProvider)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	// Create mock coordinator that tracks emitted events
	mockCoordinator := &mockEventCoordinatorForTest{
		emittedEvents: []*EventContext{},
	}

	// Create adapter
	adapter := NewMetricsCollectorAdapter(mockCoordinator)

	// Create unified collector with adapter
	unifiedCollector := storage.NewUnifiedMetricsCollector(storage.UnifiedMetricsCollectorConfig{
		Storage:             storageProvider,
		EventEmitter:        adapter,
		EnableAsyncCAS:      false,
		EnableAsyncFileLock: false,
		AsyncBufferSize:     100,
	})

	t.Cleanup(func() {
		unifiedCollector.Stop()
		buffer := storage.GetGlobalAuditEventBuffer()
		if buffer != nil {
			_ = buffer.Shutdown() //nolint:errcheck // Test cleanup
		}
	})

	// Set up context and time window
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	windowStart := time.Now().Add(-1 * time.Hour)
	windowEnd := time.Now()

	// Reset metrics first to avoid conflicts
	caspkg.ResetObjectStorageMetrics()
	storage.ResetFileLockMetrics()
	auditMetrics := storage.GetGlobalAuditMetricsCollector()
	_, _ = auditMetrics.CollectAndReset(ctx, secCtx, windowStart.Add(-2*time.Hour), windowStart)

	// Record some metrics
	casMetrics := caspkg.GetObjectStorageMetrics()
	casMetrics.RecordCreate(10*time.Millisecond, nil)

	_, err = unifiedCollector.CollectAllMetrics(ctx, secCtx, windowStart, windowEnd)
	if err != nil {
		// Some metrics may fail, but events should still be emitted
		t.Logf("Some metrics collection failed (expected in some cases): %v", err)
	}

	// Verify events were emitted
	events := mockCoordinator.getEmittedEvents()
	if len(events) == 0 {
		t.Error("Expected events to be emitted via coordinator")
	}

	// Verify event properties
	foundCASEvent := false
	for _, event := range events {
		if event.OperationType == "metric_collection" {
			foundCASEvent = true
			if !event.EmitMetrics {
				t.Error("Expected EmitMetrics to be true")
			}
			if event.EventData == nil || event.EventData.MetricsData == nil {
				t.Error("Expected metrics data to be set")
			}
		}
	}

	if !foundCASEvent {
		t.Error("Expected to find CAS metric collection event")
	}
}

// mockEventCoordinatorForTest is a test implementation that tracks emitted events
type mockEventCoordinatorForTest struct {
	emittedEvents []*EventContext
	mu            sync.Mutex
}

func (m *mockEventCoordinatorForTest) Emit(ctx context.Context, eventCtx *EventContext) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emittedEvents = append(m.emittedEvents, eventCtx)
	return nil
}

func (m *mockEventCoordinatorForTest) getEmittedEvents() []*EventContext {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*EventContext, len(m.emittedEvents))
	copy(result, m.emittedEvents)
	return result
}

func (m *mockEventCoordinatorForTest) Subscribe(subscriber OperationalEventSubscriber) string {
	return "test-subscriber"
}

func (m *mockEventCoordinatorForTest) Unsubscribe(subscriberID string) {
	// No-op
}
