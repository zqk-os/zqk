package coordination

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/metrics"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
)

// setupCoordinatorTest creates a test environment with all routers configured
// Returns the audit router for callback setup (can be nil if not needed)
func setupCoordinatorTest(t *testing.T) (coordinator *Coordinator, fileStorage storage.ObjectStorageProvider, auditRouter *StorageAuditRouter, cleanup func()) {
	envOpts := defaultCoordinationTestEnvOptions()
	envOpts.NoopStorageMetrics = false // assert on global storage metrics in this package
	testRoot, fileStorage, coreCleanup := setupCoordinationCompleteTestEnvironment(t, &envOpts)

	storage.BuildPathAliasCacheForProject(testRoot)

	// Create metrics pipeline
	pipeline := metrics.NewMetricPipeline(fileStorage)

	// Create audit router with callback support for test verification
	auditRouter = NewStorageAuditRouter(testRoot, fileStorage)

	// Create coordinator with all routers
	config := CoordinatorConfig{
		LoggingRouter:     &DefaultLoggingRouter{},
		AuditRouter:       auditRouter,
		MetricsRouter:     NewMetricPipelineRouter(pipeline),
		OperationalRouter: &DefaultOperationalRouter{},
	}

	coordinator = NewCoordinator(config)

	// Return cleanup function that shuts down background goroutines.
	// Run storage test cleanup first so test storage drains its hash registries before global
	// shutdown; otherwise in-flight audit creates can see "hash registry context cancelled".
	cleanupFunc := func() {
		_ = testkit.RunNamedTestSteps(context.Background(), "coordination.integration_cleanup",
			testkit.NamedTestStep{Name: "TEARDOWN_TEST_ENV", Fn: func() error {
				if coreCleanup != nil {
					coreCleanup()
				}
				return nil
			}},
			testkit.NamedTestStep{Name: "SHUTDOWN_GLOBAL_COORDINATOR", Fn: func() error {
				if shutdownCoordinator := storage.GetGlobalShutdownCoordinator(); shutdownCoordinator != nil {
					_ = shutdownCoordinator.InitiateShutdown() //nolint:errcheck // Test cleanup
				}
				return nil
			}},
			testkit.NamedTestStep{Name: "DRAIN_IO_QUEUE_MANAGER", Fn: func() error {
				if ioQueueManager := storage.GetGlobalIOQueueManager(pkgctx.NewSystemContext()); ioQueueManager != nil {
					_ = ioQueueManager.InitiateShutdown() //nolint:errcheck // Test cleanup
					ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 2*time.Second)
					_ = ioQueueManager.Drain(ctx) //nolint:errcheck // Test cleanup - timeout is acceptable
					cancel()
				}
				return nil
			}},
			testkit.NamedTestStep{Name: "SHUTDOWN_AUDIT_BUFFER", Fn: func() error {
				if buffer := storage.GetGlobalAuditEventBuffer(); buffer != nil {
					_ = buffer.Shutdown() //nolint:errcheck // Test cleanup
				}
				return nil
			}},
			testkit.NamedTestStep{Name: "STOP_METRIC_SAMPLERS", Fn: func() error {
				if pipeline != nil && pipeline.GetSamplerRegistry() != nil {
					_ = pipeline.GetSamplerRegistry().StopAll() //nolint:errcheck // Test cleanup
				}
				return nil
			}},
		)
	}

	return coordinator, fileStorage, auditRouter, cleanupFunc
}

// auditEventResult represents the result of an audit event creation
type auditEventResult struct {
	eventType   string
	operationID string
	err         error
}

// TestCoordinator_MultiChannelRouting tests that events are routed to all enabled channels
func TestCoordinator_MultiChannelRouting(t *testing.T) {
	// Not parallel: setupCoordinatorTest shares global audit metrics/buffer/ID machinery with other tests;
	// parallel runs caused flaky List vs metrics (events created in peer temp dirs).
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - requires full system shutdown")
	}
	coordinator, fileStorage, auditRouter, cleanup := setupCoordinatorTest(t)
	defer cleanup()

	// Set up callback channel to verify async audit event creation
	auditEventChan := make(chan auditEventResult, 1)
	auditRouter.SetCallback(func(eventType, operationID string, err error) {
		auditEventChan <- auditEventResult{
			eventType:   eventType,
			operationID: operationID,
			err:         err,
		}
	})

	// Create test subscribers to capture operational events
	subscriber1 := &testEventSubscriber{
		id:         "sub-1",
		eventTypes: []string{"operation.start", "operation.complete"},
		active:     true,
		events:     make([]*OperationalEvent, 0),
		mu:         sync.Mutex{},
	}

	subscriber2 := &testEventSubscriber{
		id:         "sub-2",
		eventTypes: []string{"operation.complete"},
		active:     true,
		events:     make([]*OperationalEvent, 0),
		mu:         sync.Mutex{},
	}

	coordinator.Subscribe(subscriber1)
	coordinator.Subscribe(subscriber2)

	// Create event context with all channels enabled
	eventCtx := NewEventContext("test-op-1", "test_operation", "start").
		WithEventData(&EventData{
			LoggingFields: []LoggingField{
				{Key: "event", Value: "test_start"},
				{Key: "operation_id", Value: "test-op-1"},
			},
			AuditMetadata: map[string]any{
				objects.FieldKeyEventType: "system_config_change", // Use valid enum value from audit_event spec
				objects.FieldKeyOperation: "Test operation: start",
				objects.FieldKeySeverity:  "low",
			},
			MetricsData: map[string]any{
				objects.FieldKeyOperation: "test_operation",
				objects.FieldKeyStatus:    "start",
			},
		}).
		WithChannels(true, true, true, true) // Enable all channels

	ctx := pkgctx.NewSystemContext()

	// Emit event
	err := coordinator.Emit(ctx, eventCtx)
	if err != nil {
		t.Errorf("Emit() returned error: %v", err)
	}

	// Wait for audit event callback (async completion verification)
	select {
	case result := <-auditEventChan:
		if result.err != nil {
			// Log the actual error for debugging
			t.Errorf("Audit event creation failed: %v (event_type=%s, operation_id=%s)",
				result.err, result.eventType, result.operationID)
		} else {
			t.Logf("Audit event callback fired: event_type=%s, operation_id=%s", result.eventType, result.operationID)
		}
	case <-time.After(5 * time.Second):
		t.Error("Timeout waiting for audit event callback - event may not have been created")
	}

	// Check audit metrics to verify event creation was attempted
	metricsCollector := storage.GetGlobalAuditMetricsCollector()
	snapshot := metricsCollector.GetSnapshot()
	t.Logf("Audit metrics: created=%d, failed=%d, validated=%d, skipped=%d",
		snapshot.EventsCreated, snapshot.EventsFailed, snapshot.EventsValidated, snapshot.EventsSkipped)

	// Verify audit event was actually created in storage
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{
		Kind:  "audit_event",
		Limit: 10,
	}

	// Flush buffer to ensure any buffered events are written
	buffer := storage.GetGlobalAuditEventBuffer()
	if err := buffer.Flush(); err != nil {
		t.Logf("Warning: Failed to flush buffer: %v", err)
	}

	// Give storage and async routers time: callback fires when CreateAuditEventWithBuilder returns;
	// operational events are emitted in a separate goroutine, so allow time for delivery.
	time.Sleep(500 * time.Millisecond)

	result, err := fileStorage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		t.Errorf("Failed to list audit events: %v", err)
	} else {
		found := false
		for _, event := range result.Objects {
			eventType, _ := event[objects.FieldKeyEventType].(string)
			operation, _ := event[objects.FieldKeyOperation].(string)
			// Check for either the explicit event_type or operation matching
			if (eventType != emptyValue && eventType == "system_config_change") ||
				(operation != emptyValue && strings.Contains(operation, "Test operation")) {
				found = true
				t.Logf("Found audit event in storage: event_type=%s, operation=%s, id=%v", eventType, operation, event[objects.FieldKeyID])
				break
			}
		}
		if !found {
			// If callback fired but event not found, and metrics show no creation, something failed silently
			if snapshot.EventsCreated == 0 && snapshot.EventsFailed == 0 {
				t.Errorf("Audit event callback fired but metrics show no creation attempt. This suggests silent failure in CreateAuditEventWithBuilder")
			} else if snapshot.EventsFailed > 0 {
				t.Errorf("Audit event creation failed (metrics: failed=%d). Callback fired but event not created.", snapshot.EventsFailed)
			} else {
				t.Errorf("Audit event callback fired but event not found in storage. Found %d events total, metrics: created=%d", len(result.Objects), snapshot.EventsCreated)
			}
			for i, event := range result.Objects {
				t.Logf("Event %d: id=%v, event_type=%v, operation=%v", i, event[objects.FieldKeyID], event[objects.FieldKeyEventType], event[objects.FieldKeyOperation])
			}
		}
	}

	// Verify operational events: Check subscribers received events
	subscriber1.mu.Lock()
	sub1Count := len(subscriber1.events)
	subscriber1.mu.Unlock()

	if sub1Count == 0 {
		t.Error("Subscriber 1 did not receive operational event")
	}

	subscriber2.mu.Lock()
	sub2Count := len(subscriber2.events)
	subscriber2.mu.Unlock()

	// Subscriber 2 only listens to "operation.complete", so should not receive "operation.start"
	if sub2Count != 0 {
		t.Errorf("Subscriber 2 received %d events, expected 0 (only listens to operation.complete)", sub2Count)
	}
}

// TestCoordinator_SelectiveChannelRouting tests that events only route to enabled channels
func TestCoordinator_SelectiveChannelRouting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - requires full system shutdown")
	}
	coordinator, fileStorage, auditRouter, cleanup := setupCoordinatorTest(t)
	defer cleanup()

	// Set up callback channel to verify async audit event creation
	auditEventChan := make(chan auditEventResult, 1)
	auditRouter.SetCallback(func(eventType, operationID string, err error) {
		auditEventChan <- auditEventResult{
			eventType:   eventType,
			operationID: operationID,
			err:         err,
		}
	})

	// Get initial count
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	initialFilter := storage.ListFilter{Kind: "audit_event"}
	initialResult, err := fileStorage.List(ctx, secCtx, storageCtx, initialFilter)
	if err != nil {
		t.Fatalf("Failed to list initial audit events: %v", err)
	}
	initialCount := len(initialResult.Objects)

	// Create event with only audit channel enabled
	eventCtx := NewEventContext("test-op-2", "test_operation", "complete").
		WithEventData(&EventData{
			AuditMetadata: map[string]any{
				objects.FieldKeyEventType: "system_config_change", // Use valid enum value from audit_event spec
				objects.FieldKeyOperation: "Test operation: complete",
				objects.FieldKeySeverity:  "low",
			},
		}).
		WithChannels(false, true, false, false) // Only audit enabled

	if err := coordinator.Emit(ctx, eventCtx); err != nil {
		t.Errorf("Emit() returned error: %v", err)
	}

	// Wait for audit event callback (async completion verification)
	select {
	case result := <-auditEventChan:
		if result.err != nil {
			t.Errorf("Audit event creation failed: %v", result.err)
		} else {
			t.Logf("Audit event created successfully: event_type=%s, operation_id=%s", result.eventType, result.operationID)
		}
	case <-time.After(5 * time.Second):
		t.Error("Timeout waiting for audit event callback - event may not have been created")
	}

	// Give storage a moment to write (callback fires before storage write completes)
	time.Sleep(100 * time.Millisecond)

	// Verify audit event was created (list all and check)
	filter := storage.ListFilter{
		Kind: "audit_event",
	}

	result, err := fileStorage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		t.Errorf("Failed to list audit events: %v", err)
	}

	found := false
	for _, event := range result.Objects {
		eventType, _ := event[objects.FieldKeyEventType].(string)
		operation, _ := event[objects.FieldKeyOperation].(string)
		// Check for event matching our operation
		if (eventType != emptyValue && eventType == "system_config_change") ||
			(operation != emptyValue && strings.Contains(operation, "Test operation")) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Audit event was not created when audit channel was enabled. Found %d events", len(result.Objects))
	}

	// Verify count increased by exactly 1
	finalResult, err := fileStorage.List(ctx, secCtx, storageCtx, initialFilter)
	if err != nil {
		t.Fatalf("Failed to list final audit events: %v", err)
	}
	finalCount := len(finalResult.Objects)
	if finalCount != initialCount+1 {
		t.Errorf("Expected audit event count to increase by 1, initial: %d, final: %d", initialCount, finalCount)
	}
}

// TestCoordinator_ErrorEvent tests error event handling
func TestCoordinator_ErrorEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - requires full system shutdown")
	}
	coordinator, fileStorage, auditRouter, cleanup := setupCoordinatorTest(t)
	defer cleanup()

	// Set up callback channel to verify async audit event creation
	auditEventChan := make(chan auditEventResult, 1)
	auditRouter.SetCallback(func(eventType, operationID string, err error) {
		auditEventChan <- auditEventResult{
			eventType:   eventType,
			operationID: operationID,
			err:         err,
		}
	})

	testError := fmt.Errorf("test error: object not found")

	eventCtx := NewEventContext("test-op-3", "test_operation", "error").
		WithError(testError).
		WithEventData(&EventData{
			LoggingFields: []LoggingField{
				{Key: "event", Value: "operation_failed"},
				{Key: "operation_id", Value: "test-op-3"},
			},
			AuditMetadata: map[string]any{
				objects.FieldKeyEventType: "system_config_change", // Use valid enum value from audit_event spec
				objects.FieldKeyOperation: "Test operation: error",
				objects.FieldKeySeverity:  "high",
				"error":                   testError.Error(),
			},
		}).
		WithChannels(true, true, false, false)

	ctx := pkgctx.NewSystemContext()

	err := coordinator.Emit(ctx, eventCtx)
	if err != nil {
		t.Errorf("Emit() returned error: %v", err)
	}

	// Wait for audit event callback (async completion verification)
	select {
	case result := <-auditEventChan:
		if result.err != nil {
			t.Errorf("Audit event creation failed: %v", result.err)
		} else {
			t.Logf("Error audit event created successfully: event_type=%s, operation_id=%s", result.eventType, result.operationID)
		}
	case <-time.After(5 * time.Second):
		t.Error("Timeout waiting for audit event callback - event may not have been created")
	}

	// Give storage a moment to write (callback fires before storage write completes)
	time.Sleep(100 * time.Millisecond)

	// Verify error audit event was created
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{
		Kind: "audit_event",
	}

	result, err := fileStorage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		t.Errorf("Failed to list audit events: %v", err)
	}

	found := false
	for _, event := range result.Objects {
		eventType, _ := event[objects.FieldKeyEventType].(string)
		severity, _ := event[objects.FieldKeySeverity].(string)
		operation, _ := event[objects.FieldKeyOperation].(string)
		// Error events should have high severity and system_config_change event_type
		if (eventType != emptyValue && eventType == "system_config_change") &&
			(severity == "high" || (operation != emptyValue && strings.Contains(operation, "error"))) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Error audit event was not created. Found %d events", len(result.Objects))
		for i, event := range result.Objects {
			t.Logf("Event %d: event_type=%v, severity=%v", i, event[objects.FieldKeyEventType], event[objects.FieldKeySeverity])
		}
	}
}

// TestCoordinator_ConcurrentEmit tests concurrent event emission
func TestCoordinator_ConcurrentEmit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - requires full system shutdown")
	}
	coordinator, _, _, cleanup := setupCoordinatorTest(t)
	defer cleanup()

	concurrency := 50
	var wg sync.WaitGroup
	errors := make(chan error, concurrency)

	ctx := pkgctx.NewSystemContext()

	// Emit events concurrently
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("coordination_test", "concurrent emit").StartSimple(func() {
			func(index int) {
				defer wg.Done()

				eventCtx := NewEventContext(
					fmt.Sprintf("test-op-%d", index),
					"concurrent_operation",
					"start",
				).WithEventData(&EventData{
					LoggingFields: []LoggingField{
						{Key: "index", Value: index},
					},
				}).WithChannels(true, false, false, false) // Only logging to avoid storage contention

				if err := coordinator.Emit(ctx, eventCtx); err != nil {
					errors <- err
				}
			}(i)
		})
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent emit failed: %v", err)
	}
}

// TestCoordinator_ConcurrentSubscribe tests concurrent subscription/unsubscription
func TestCoordinator_ConcurrentSubscribe(t *testing.T) {
	t.Parallel()
	coordinator := NewCoordinator(CoordinatorConfig{})

	concurrency := 20
	var wg sync.WaitGroup

	// Concurrent subscribe/unsubscribe
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("coordination_test", "concurrent subscribe").StartSimple(func() {
			func(index int) {
				defer wg.Done()

				subscriber := &testEventSubscriber{
					id:         fmt.Sprintf("sub-%d", index),
					eventTypes: []string{"operation.start"},
					active:     true,
					events:     make([]*OperationalEvent, 0),
					mu:         sync.Mutex{},
				}

				subID := coordinator.Subscribe(subscriber)
				time.Sleep(10 * time.Millisecond) // Simulate some work
				coordinator.Unsubscribe(subID)
			}(i)
		})
	}

	wg.Wait()
	// Test passes if no panic or race condition
}

// TestCoordinator_SubscriberFailure tests that subscriber failures don't block other subscribers
func TestCoordinator_SubscriberFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - requires full system shutdown")
	}
	coordinator, _, _, cleanup := setupCoordinatorTest(t)
	defer cleanup()

	// Create a failing subscriber
	failingSubscriber := &testEventSubscriber{
		id:         "failing-sub",
		eventTypes: []string{"operation.start"},
		active:     true,
		events:     make([]*OperationalEvent, 0),
		mu:         sync.Mutex{},
		shouldFail: true,
	}

	// Create a working subscriber
	workingSubscriber := &testEventSubscriber{
		id:         "working-sub",
		eventTypes: []string{"operation.start"},
		active:     true,
		events:     make([]*OperationalEvent, 0),
		mu:         sync.Mutex{},
	}

	coordinator.Subscribe(failingSubscriber)
	coordinator.Subscribe(workingSubscriber)

	eventCtx := NewEventContext("test-op-4", "test_operation", "start").
		WithEventData(&EventData{
			LoggingFields: []LoggingField{
				{Key: "event", Value: "test_start"},
			},
		}).
		WithChannels(false, false, false, true) // Only operational

	ctx := pkgctx.NewSystemContext()

	// Emit should not fail even if one subscriber fails
	err := coordinator.Emit(ctx, eventCtx)
	if err != nil {
		t.Errorf("Emit() should not fail when subscriber fails: %v", err)
	}

	// Operational events are emitted in a goroutine; allow time for delivery
	time.Sleep(500 * time.Millisecond)

	// Verify working subscriber still received the event
	workingSubscriber.mu.Lock()
	workingCount := len(workingSubscriber.events)
	workingSubscriber.mu.Unlock()

	if workingCount == 0 {
		t.Error("Working subscriber did not receive event when another subscriber failed")
	}
}

// TestCoordinator_EmptyEventData tests events with minimal data
func TestCoordinator_EmptyEventData(t *testing.T) {
	t.Parallel()
	coordinator := NewCoordinator(CoordinatorConfig{})

	eventCtx := NewEventContext("test-op-5", "minimal_operation", "start").
		WithChannels(true, false, false, false)

	ctx := pkgctx.NewSystemContext()

	// Should not panic with minimal event data
	err := coordinator.Emit(ctx, eventCtx)
	if err != nil {
		t.Errorf("Emit() with minimal data returned error: %v", err)
	}
}

// TestCoordinator_MultipleSubscribersSameEventType tests multiple subscribers for same event type
func TestCoordinator_MultipleSubscribersSameEventType(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - requires full system shutdown")
	}
	coordinator, _, _, cleanup := setupCoordinatorTest(t)
	defer cleanup()

	subscriber1 := &testEventSubscriber{
		id:         "sub-a",
		eventTypes: []string{"operation.complete"},
		active:     true,
		events:     make([]*OperationalEvent, 0),
		mu:         sync.Mutex{},
	}

	subscriber2 := &testEventSubscriber{
		id:         "sub-b",
		eventTypes: []string{"operation.complete"},
		active:     true,
		events:     make([]*OperationalEvent, 0),
		mu:         sync.Mutex{},
	}

	coordinator.Subscribe(subscriber1)
	coordinator.Subscribe(subscriber2)

	eventCtx := NewEventContext("test-op-6", "test_operation", "complete").
		WithEventData(&EventData{
			LoggingFields: []LoggingField{
				{Key: "event", Value: "test_complete"},
			},
		}).
		WithChannels(false, false, false, true) // Only operational

	ctx := pkgctx.NewSystemContext()

	err := coordinator.Emit(ctx, eventCtx)
	if err != nil {
		t.Errorf("Emit() returned error: %v", err)
	}

	// Operational events are emitted in a goroutine; allow time for delivery
	time.Sleep(500 * time.Millisecond)

	// Both subscribers should receive the event
	subscriber1.mu.Lock()
	sub1Count := len(subscriber1.events)
	subscriber1.mu.Unlock()

	subscriber2.mu.Lock()
	sub2Count := len(subscriber2.events)
	subscriber2.mu.Unlock()

	if sub1Count == 0 {
		t.Error("Subscriber 1 did not receive event")
	}
	if sub2Count == 0 {
		t.Error("Subscriber 2 did not receive event")
	}
}

// testEventSubscriber is a test implementation of OperationalEventSubscriber
type testEventSubscriber struct {
	id         string
	eventTypes []string
	active     bool
	events     []*OperationalEvent
	mu         sync.Mutex
	shouldFail bool
}

func (tes *testEventSubscriber) ID() string {
	return tes.id
}

func (tes *testEventSubscriber) HandleEvent(event *OperationalEvent) error {
	if tes.shouldFail {
		return fmt.Errorf("simulated subscriber failure")
	}

	tes.mu.Lock()
	defer tes.mu.Unlock()
	tes.events = append(tes.events, event)
	return nil
}

func (tes *testEventSubscriber) EventTypes() []string {
	return tes.eventTypes
}

func (tes *testEventSubscriber) IsActive() bool {
	return tes.active
}
