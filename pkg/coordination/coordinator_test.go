package coordination

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestCoordinator_Emit(t *testing.T) {
	t.Parallel()
	coordinator := NewCoordinator(CoordinatorConfig{})

	eventCtx := NewEventContext("test-op-1", "test_operation", "start").
		WithEventData(&EventData{
			LoggingFields: []LoggingField{
				{Key: "event", Value: "test_start"},
				{Key: "operation", Value: "test_operation"},
			},
			AuditMetadata: map[string]any{
				objects.FieldKeyEventType: "test_start",
				objects.FieldKeyOperation: "Test operation started",
			},
			MetricsData: map[string]any{
				objects.FieldKeyOperation: "test_operation",
			},
		})

	// Emit should not panic
	err := coordinator.Emit(pkgctx.NewSystemContext(), eventCtx)
	if err != nil {
		t.Errorf("Emit() returned error: %v", err)
	}
}

func TestCoordinator_Subscribe(t *testing.T) {
	t.Parallel()
	coordinator := NewCoordinator(CoordinatorConfig{})

	subscriber := &testSubscriber{
		id:         "test-sub-1",
		eventTypes: []string{"operation.start"},
		active:     true,
	}

	subscriberID := coordinator.Subscribe(subscriber)
	if subscriberID != "test-sub-1" {
		t.Errorf("Subscribe() returned wrong ID: got %s, want test-sub-1", subscriberID)
	}

	// Unsubscribe
	coordinator.Unsubscribe(subscriberID)

	// Subscribe again should work
	subscriberID2 := coordinator.Subscribe(subscriber)
	if subscriberID2 != "test-sub-1" {
		t.Errorf("Subscribe() after unsubscribe returned wrong ID: got %s, want test-sub-1", subscriberID2)
	}
}

func TestEventContext_WithMethods(t *testing.T) {
	t.Parallel()
	ec := NewEventContext("op-1", "test_op", "start")

	ec = ec.WithDuration(5 * time.Second).
		WithDependencies([]string{"dep-1", "dep-2"}).
		WithTriggers([]string{"trigger-1"}).
		WithCorrelationID("corr-123")

	if ec.Duration != 5*time.Second {
		t.Errorf("Duration not set correctly")
	}
	if len(ec.Dependencies) != 2 {
		t.Errorf("Dependencies not set correctly")
	}
	if len(ec.Triggers) != 1 {
		t.Errorf("Triggers not set correctly")
	}
	if ec.CorrelationID != "corr-123" {
		t.Errorf("CorrelationID not set correctly")
	}
}

func TestStorageAuditRouter_Emit(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("Skipping test in short mode - creates FileObjectStorage with background goroutines")
	}
	// Create a temporary directory for storage
	tmpDir := t.TempDir()

	ensureObjectSpecsForCoordTest(t, tmpDir)

	// Use test storage so hash registries are not registered with global manager;
	// otherwise a later test's cleanup can cancel them and cause "hash registry context cancelled".
	fileStorage, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create file storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, fileStorage)
	t.Cleanup(func() {
		if err := storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, fileStorage)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	router := NewStorageAuditRouter(tmpDir, fileStorage)

	eventCtx := NewEventContext("test-op", "test_operation", "complete").
		WithEventData(&EventData{
			AuditMetadata: map[string]any{
				objects.FieldKeyEventType:  "test_complete",
				objects.FieldKeyOperation:  "Test operation completed",
				objects.FieldKeySeverity:   "low",
				objects.FieldKeyTargetKind: "test_object",
			},
		})

	// Emit should not panic (best-effort, may fail if project structure is incomplete)
	err = router.Emit(pkgctx.NewSystemContext(), eventCtx)
	// Error is acceptable for best-effort audit events
	_ = err
}

func TestMetricPipelineRouter_Emit(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("Skipping test in short mode - creates FileObjectStorage with background goroutines")
	}
	tmpDir := t.TempDir()

	// Create minimal project structure (.zqk/process directory)
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	fileStorage, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create file storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, fileStorage)

	pipeline := metrics.NewMetricPipeline(fileStorage)
	router := NewMetricPipelineRouter(pipeline)

	t.Cleanup(func() {
		if err := storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, fileStorage)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})
	t.Cleanup(func() {
		if pipeline != nil && pipeline.GetSamplerRegistry() != nil {
			_ = pipeline.GetSamplerRegistry().StopAll() //nolint:errcheck // Test cleanup
		}
		buffer := storage.GetGlobalAuditEventBuffer()
		if buffer != nil {
			_ = buffer.Shutdown() //nolint:errcheck // Test cleanup
		}
	})

	eventCtx := NewEventContext("test-op", "test_operation", "complete").
		WithEventData(&EventData{
			MetricsData: map[string]any{
				objects.FieldKeyKind:            "test",
				objects.FieldKeyOperation:       "test_operation",
				objects.FieldKeyDurationSeconds: 1.5,
			},
		})

	// Emit should not panic
	err = router.Emit(pkgctx.NewSystemContext(), eventCtx)
	// Error is acceptable for best-effort metrics
	_ = err
}

func TestNewOperationalEvent(t *testing.T) {
	t.Parallel()
	eventCtx := NewEventContext("test-op", "test_operation", "complete").
		WithDuration(5 * time.Second).
		WithEventData(&EventData{
			MetricsData: map[string]any{
				"result": "success",
			},
		})

	operationalEvent := NewOperationalEvent(eventCtx)

	if operationalEvent.Type != "operation.complete" {
		t.Errorf("OperationalEvent.Type = %s, want operation.complete", operationalEvent.Type)
	}
	if operationalEvent.OperationID != "test-op" {
		t.Errorf("OperationalEvent.OperationID = %s, want test-op", operationalEvent.OperationID)
	}
	if operationalEvent.Duration != 5*time.Second {
		t.Errorf("OperationalEvent.Duration not set correctly")
	}
}

func TestNewOperationalEvent_DependencyRefStatus(t *testing.T) {
	t.Parallel()
	eventCtx := NewEventContext("target-1", "lifecycle_dependency_ref", "dependency_ref").
		WithEventData(&EventData{
			MetricsData: map[string]any{"project_root": "/proj", objects.FieldKeyTargetID: "target-1"},
		})
	operationalEvent := NewOperationalEvent(eventCtx)
	if operationalEvent.Type != EventTypeLifecycleDependencyRef {
		t.Errorf("OperationalEvent.Type = %s, want %s", operationalEvent.Type, EventTypeLifecycleDependencyRef)
	}
}

// testSubscriber is a test implementation of OperationalEventSubscriber
type testSubscriber struct {
	id         string
	eventTypes []string
	active     bool
	events     []*OperationalEvent
}

func (ts *testSubscriber) ID() string {
	return ts.id
}

func (ts *testSubscriber) HandleEvent(event *OperationalEvent) error {
	ts.events = append(ts.events, event)
	return nil
}

func (ts *testSubscriber) EventTypes() []string {
	return ts.eventTypes
}

func (ts *testSubscriber) IsActive() bool {
	return ts.active
}

func TestResolveLogLevel_QueueShutdownNoiseSuppression(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		eventCtx      *EventContext
		expectedLevel logging.LogLevel
	}{
		{
			name:          "queue_shutdown operation type",
			eventCtx:      NewEventContext("op-1", "queue_shutdown", OperationStatusComplete),
			expectedLevel: logging.DebugLevel,
		},
		{
			name:          "queue_shutdown operation ID with initiate type",
			eventCtx:      NewEventContext("queue_shutdown", "initiate", OperationStatusComplete),
			expectedLevel: logging.DebugLevel,
		},
		{
			name:          "drain_start event type",
			eventCtx:      NewEventContext("queue_shutdown", "drain_start", OperationStatusProgress),
			expectedLevel: logging.DebugLevel,
		},
		{
			name:          "drain_complete event type",
			eventCtx:      NewEventContext("queue_shutdown", "drain_complete", OperationStatusComplete),
			expectedLevel: logging.DebugLevel,
		},
		{
			name:          "explicit debug level override",
			eventCtx:      NewEventContext("op-2", "arbitrary_operation", OperationStatusComplete).WithLevel("debug"),
			expectedLevel: logging.DebugLevel,
		},
		{
			name:          "standard operation defaults to info",
			eventCtx:      NewEventContext("op-3", "user_action", OperationStatusComplete),
			expectedLevel: logging.InfoLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := resolveLogLevel(tt.eventCtx, "test-msg", nil)
			if level != tt.expectedLevel {
				t.Errorf("resolveLogLevel() = %v, want %v", level, tt.expectedLevel)
			}
		})
	}
}

func TestCoordinator_DrainAndWait(t *testing.T) {
	coordinator := NewCoordinator(CoordinatorConfig{})
	// Fast drain when no async tasks
	if err := coordinator.Drain(100 * time.Millisecond); err != nil {
		t.Fatalf("Drain on empty coordinator failed: %v", err)
	}

	coordinator.runRouterAsync(pkgctx.NewSystemContext(), "test_router", "test_purpose", func(ctx context.Context) {
		time.Sleep(20 * time.Millisecond)
	})

	if err := coordinator.Drain(500 * time.Millisecond); err != nil {
		t.Fatalf("Drain with active router failed: %v", err)
	}
	coordinator.Wait()
}

func TestProgressHelper_Drain(t *testing.T) {
	var nilHelper *ProgressHelper
	if err := nilHelper.Drain(100 * time.Millisecond); err != nil {
		t.Fatalf("Drain on nil helper failed: %v", err)
	}

	coord := NewCoordinator(CoordinatorConfig{})
	helper := NewProgressHelper(coord, t.TempDir(), "op-drain", "test_op", "system")
	if err := helper.Drain(100 * time.Millisecond); err != nil {
		t.Fatalf("Drain on helper failed: %v", err)
	}
}

func TestDrainGlobalCoordinator(t *testing.T) {
	old := GetCoordinator()
	defer SetGlobalCoordinator(old)

	coord := NewCoordinator(CoordinatorConfig{})
	SetGlobalCoordinator(coord)

	if err := DrainGlobalCoordinator(100 * time.Millisecond); err != nil {
		t.Fatalf("DrainGlobalCoordinator failed: %v", err)
	}

	ResetGlobalCoordinator()
	if err := DrainGlobalCoordinator(100 * time.Millisecond); err != nil {
		t.Fatalf("DrainGlobalCoordinator on nil coordinator failed: %v", err)
	}
}
