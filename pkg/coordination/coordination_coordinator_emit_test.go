package coordination

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type testCycle151Subscriber struct {
	id         string
	eventTypes []string
	active     bool
	received   atomic.Int32
	errToRet   error
}

func (s *testCycle151Subscriber) ID() string {
	return s.id
}

func (s *testCycle151Subscriber) EventTypes() []string {
	return s.eventTypes
}

func (s *testCycle151Subscriber) IsActive() bool {
	return s.active
}

func (s *testCycle151Subscriber) HandleEvent(event *OperationalEvent) error {
	s.received.Add(1)
	return s.errToRet
}

type testCycle151ErrorRouter struct {
	errToReturn error
	calledCount atomic.Int32
}

func (r *testCycle151ErrorRouter) Emit(_ context.Context, _ *EventContext) error {
	r.calledCount.Add(1)
	return r.errToReturn
}

type testCycle151CustomCoordinator struct {
	emitCalls        atomic.Int32
	subscribeCalls   atomic.Int32
	unsubscribeCalls atomic.Int32
}

func (c *testCycle151CustomCoordinator) Emit(_ context.Context, _ *EventContext) error {
	c.emitCalls.Add(1)
	return nil
}

func (c *testCycle151CustomCoordinator) Subscribe(_ OperationalEventSubscriber) string {
	c.subscribeCalls.Add(1)
	return "custom-sub-id"
}

func (c *testCycle151CustomCoordinator) Unsubscribe(_ string) {
	c.unsubscribeCalls.Add(1)
}

func TestCycle151_Coordinator_InitializationLifecycleAndTeardown(t *testing.T) {
	coord := NewCoordinator(CoordinatorConfig{})
	if coord == nil {
		t.Fatalf("expected non-nil coordinator")
	}

	if err := coord.Drain(0); err != nil {
		t.Fatalf("unexpected error on Drain(0): %v", err)
	}

	if err := coord.Drain(50 * time.Millisecond); err != nil {
		t.Fatalf("unexpected error on Drain(50ms): %v", err)
	}

	coord.Wait()

	ResetGlobalCoordinator()
	g1 := GetCoordinator()
	if g1 == nil {
		t.Fatalf("expected non-nil global coordinator")
	}

	custom := NewCoordinator(CoordinatorConfig{})
	SetGlobalCoordinator(custom)
	if GetCoordinator() != custom {
		t.Fatalf("expected custom coordinator to be set as global")
	}

	if err := DrainGlobalCoordinator(50 * time.Millisecond); err != nil {
		t.Fatalf("unexpected error on DrainGlobalCoordinator: %v", err)
	}

	ResetGlobalCoordinator()

	metricsSink := noopCoordinatorEmitMetrics{}
	metricsSink.RecordStage(context.Background(), "stage1", "id1", time.Millisecond, nil)
	metricsSink.RecordStageWithBuckets(context.Background(), "stage1", "id1", time.Millisecond, nil, nil)
}

func TestCycle151_Coordinator_EmitPartitionAndIdempotencyKeys(t *testing.T) {
	if got := coordinatorEmitPartitionKey(nil); got != "" {
		t.Fatalf("expected empty partition key for nil context, got %q", got)
	}

	ev := &EventContext{OperationType: "task_exec", Status: "running"}
	if got := coordinatorEmitPartitionKey(ev); got != "task_exec:running" {
		t.Fatalf("expected task_exec:running, got %q", got)
	}

	if got := coordinatorEmitIdempotencyKey(nil); got != "" {
		t.Fatalf("expected empty idempotency key for nil context, got %q", got)
	}

	evWithOp := &EventContext{OperationID: "op-101", CorrelationID: "corr-101"}
	if got := coordinatorEmitIdempotencyKey(evWithOp); got != "op-101" {
		t.Fatalf("expected op-101, got %q", got)
	}

	evWithCorr := &EventContext{CorrelationID: "corr-202"}
	if got := coordinatorEmitIdempotencyKey(evWithCorr); got != "corr-202" {
		t.Fatalf("expected corr-202, got %q", got)
	}

	evEmpty := &EventContext{}
	if got := coordinatorEmitIdempotencyKey(evEmpty); got != "" {
		t.Fatalf("expected empty idempotency key, got %q", got)
	}
}

func TestCycle151_Coordinator_EmitPipelineExecutionAndShortCircuiting(t *testing.T) {
	coord := NewCoordinator(CoordinatorConfig{})

	if err := coord.Emit(context.Background(), nil); err == nil {
		t.Fatalf("expected error on Emit with nil event context")
	}

	if err := coord.EmitOperationalSync(context.Background(), nil); err == nil {
		t.Fatalf("expected error on EmitOperationalSync with nil event context")
	}

	noOpEv := &EventContext{EmitOperational: false}
	if err := coord.EmitOperationalSync(context.Background(), noOpEv); err != nil {
		t.Fatalf("unexpected error when EmitOperational is false: %v", err)
	}

	sub := &testCycle151Subscriber{id: "sub-sync-1", active: true}
	if subID := coord.Subscribe(sub); subID == "" {
		t.Fatalf("expected non-empty subscriber id")
	}
	opEv := NewEventContext("op-sync", "sync-type", OperationStatusStart).
		WithChannels(false, false, false, true)
	if err := coord.EmitOperationalSync(context.Background(), opEv); err != nil {
		t.Fatalf("unexpected error on EmitOperationalSync: %v", err)
	}
	if sub.received.Load() != 1 {
		t.Fatalf("expected subscriber to receive 1 event, got %d", sub.received.Load())
	}
}

func TestCycle151_Coordinator_EmitPipeline_BestEffortResilience(t *testing.T) {
	expectedErr := errors.New("pipeline stage failed")

	logErrRouter := &testCycle151ErrorRouter{errToReturn: expectedErr}
	cLogErr := NewCoordinator(CoordinatorConfig{LoggingRouter: logErrRouter})
	evLog := NewEventContext("op-log", "type-log", objects.ObjectStatusCompleted).
		WithChannels(true, false, false, false)
	if err := cLogErr.Emit(context.Background(), evLog); err != nil {
		t.Fatalf("expected coordinator emit to remain resilient on logging router failure, got: %v", err)
	}
	if logErrRouter.calledCount.Load() != 1 {
		t.Fatalf("expected logging router to be invoked 1 time, got %d", logErrRouter.calledCount.Load())
	}

	auditErrRouter := &testCycle151ErrorRouter{errToReturn: expectedErr}
	cAuditErr := NewCoordinator(CoordinatorConfig{AuditRouter: auditErrRouter})
	evAudit := NewEventContext("op-aud", "type-aud", OperationStatusStart).
		WithChannels(false, true, false, false)
	if err := cAuditErr.Emit(context.Background(), evAudit); err != nil {
		t.Fatalf("expected coordinator emit to remain resilient on audit router failure, got: %v", err)
	}
	if err := cAuditErr.Drain(100 * time.Millisecond); err != nil {
		t.Fatalf("drain error: %v", err)
	}
	if auditErrRouter.calledCount.Load() != 1 {
		t.Fatalf("expected audit router to be invoked 1 time, got %d", auditErrRouter.calledCount.Load())
	}

	metricsErrRouter := &testCycle151ErrorRouter{errToReturn: expectedErr}
	cMetricsErr := NewCoordinator(CoordinatorConfig{MetricsRouter: metricsErrRouter})
	evMetrics := NewEventContext("op-met", "type-met", OperationStatusStart).
		WithChannels(false, false, true, false)
	if err := cMetricsErr.Emit(context.Background(), evMetrics); err != nil {
		t.Fatalf("expected coordinator emit to remain resilient on metrics router failure, got: %v", err)
	}
}

func TestCycle151_Coordinator_ResolveLogLevel_FieldOverrides(t *testing.T) {
	baseEv := NewEventContext("op1", "test_type", OperationStatusStart)

	tests := []struct {
		name     string
		fields   []logging.Field
		expected logging.LogLevel
	}{
		{"debug_level", []logging.Field{{Key: "level", Value: "debug"}}, logging.DebugLevel},
		{"info_level", []logging.Field{{Key: "level", Value: "info"}}, logging.InfoLevel},
		{"warn_level", []logging.Field{{Key: "level", Value: "warn"}}, logging.WarnLevel},
		{"warning_level", []logging.Field{{Key: "level", Value: "warning"}}, logging.WarnLevel},
		{"error_level", []logging.Field{{Key: "level", Value: "error"}}, logging.ErrorLevel},
		{"log_level_debug", []logging.Field{{Key: "log_level", Value: "debug"}}, logging.DebugLevel},
		{"non_string_value", []logging.Field{{Key: "level", Value: 42}}, logging.InfoLevel},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lvl := resolveLogLevel(baseEv, "test msg", tc.fields)
			if lvl != tc.expected {
				t.Fatalf("expected %v, got %v", tc.expected, lvl)
			}
		})
	}
}

func TestCycle151_Coordinator_ResolveLogLevel_ContextAndDefaults(t *testing.T) {
	if lvl := resolveLogLevel(nil, "msg", nil); lvl != logging.InfoLevel {
		t.Fatalf("expected InfoLevel for nil event context, got %v", lvl)
	}

	evDebug := &EventContext{Level: "debug"}
	if lvl := resolveLogLevel(evDebug, "msg", nil); lvl != logging.DebugLevel {
		t.Fatalf("expected DebugLevel for Level=debug, got %v", lvl)
	}

	evWarn := &EventContext{Level: "warn"}
	if lvl := resolveLogLevel(evWarn, "msg", nil); lvl != logging.WarnLevel {
		t.Fatalf("expected WarnLevel for Level=warn, got %v", lvl)
	}

	evWarning := &EventContext{Level: "warning"}
	if lvl := resolveLogLevel(evWarning, "msg", nil); lvl != logging.WarnLevel {
		t.Fatalf("expected WarnLevel for Level=warning, got %v", lvl)
	}

	evError := &EventContext{Level: "error"}
	if lvl := resolveLogLevel(evError, "msg", nil); lvl != logging.ErrorLevel {
		t.Fatalf("expected ErrorLevel for Level=error, got %v", lvl)
	}

	evProgress := &EventContext{Status: OperationStatusProgress}
	if lvl := resolveLogLevel(evProgress, "msg", nil); lvl != logging.DebugLevel {
		t.Fatalf("expected DebugLevel for status progress, got %v", lvl)
	}

	evSpec := &EventContext{Status: "spec"}
	if lvl := resolveLogLevel(evSpec, "msg", nil); lvl != logging.DebugLevel {
		t.Fatalf("expected DebugLevel for status spec, got %v", lvl)
	}

	evLifecycle := &EventContext{Status: "lifecycle"}
	if lvl := resolveLogLevel(evLifecycle, "msg", nil); lvl != logging.DebugLevel {
		t.Fatalf("expected DebugLevel for status lifecycle, got %v", lvl)
	}

	evShutdown := &EventContext{OperationType: "queue_shutdown"}
	if lvl := resolveLogLevel(evShutdown, "msg", nil); lvl != logging.DebugLevel {
		t.Fatalf("expected DebugLevel for queue_shutdown, got %v", lvl)
	}
}

func TestCycle151_Coordinator_ResolveLogLevel_EnvOverrides(t *testing.T) {
	t.Setenv("ZQK_INFO_OPERATIONS", "special_op")
	evInfo := &EventContext{OperationType: "special_op", Status: OperationStatusProgress}
	if lvl := resolveLogLevel(evInfo, "msg", nil); lvl != logging.InfoLevel {
		t.Fatalf("expected InfoLevel override via ZQK_INFO_OPERATIONS, got %v", lvl)
	}

	t.Setenv("ZQK_INFO_OPERATIONS", "*")
	evWildcardInfo := &EventContext{OperationType: "another_op", Status: OperationStatusProgress}
	if lvl := resolveLogLevel(evWildcardInfo, "msg", nil); lvl != logging.InfoLevel {
		t.Fatalf("expected InfoLevel wildcard override, got %v", lvl)
	}

	t.Setenv("ZQK_INFO_OPERATIONS", "")
	t.Setenv("ZQK_DEBUG_OPERATIONS", "target_debug")
	evDebug := &EventContext{OperationType: "target_debug", Status: OperationStatusStart}
	if lvl := resolveLogLevel(evDebug, "msg", nil); lvl != logging.DebugLevel {
		t.Fatalf("expected DebugLevel override via ZQK_DEBUG_OPERATIONS, got %v", lvl)
	}

	t.Setenv("ZQK_DEBUG_OPERATIONS", "*")
	evWildcardDebug := &EventContext{OperationType: "normal_op", Status: OperationStatusStart}
	if lvl := resolveLogLevel(evWildcardDebug, "msg", nil); lvl != logging.DebugLevel {
		t.Fatalf("expected DebugLevel wildcard override, got %v", lvl)
	}
}

func TestCycle151_SubscriberTopologies_WildcardAndRouting(t *testing.T) {
	coord := NewCoordinator(CoordinatorConfig{})

	if id := coord.Subscribe(nil); id != "" {
		t.Fatalf("expected empty string when subscribing nil subscriber")
	}

	subTopic := &testCycle151Subscriber{id: "sub-topic", eventTypes: []string{"operation.start"}, active: true}
	subWildcard := &testCycle151Subscriber{id: "sub-wildcard", eventTypes: nil, active: true}
	subInactive := &testCycle151Subscriber{id: "sub-inactive", eventTypes: []string{"operation.start"}, active: false}

	if id := coord.Subscribe(subTopic); id == "" {
		t.Fatalf("expected non-empty subscriber id for subTopic")
	}
	if id := coord.Subscribe(subWildcard); id == "" {
		t.Fatalf("expected non-empty subscriber id for subWildcard")
	}
	if id := coord.Subscribe(subInactive); id == "" {
		t.Fatalf("expected non-empty subscriber id for subInactive")
	}

	if !coord.hasSubscribers() {
		t.Fatalf("expected hasSubscribers to be true")
	}

	evStart := NewEventContext("op-start-1", "test_routing", OperationStatusStart).
		WithChannels(false, false, false, true)
	if err := coord.Emit(context.Background(), evStart); err != nil {
		t.Fatalf("unexpected error emitting start event: %v", err)
	}

	if err := coord.Drain(500 * time.Millisecond); err != nil {
		t.Fatalf("drain error: %v", err)
	}

	if subTopic.received.Load() != 1 {
		t.Fatalf("expected subTopic to receive 1 event, got %d", subTopic.received.Load())
	}
	if subWildcard.received.Load() != 1 {
		t.Fatalf("expected subWildcard to receive 1 event, got %d", subWildcard.received.Load())
	}
	if subInactive.received.Load() != 0 {
		t.Fatalf("expected inactive subscriber to receive 0 events, got %d", subInactive.received.Load())
	}

	coord.Unsubscribe("non-existent-sub")
	coord.Unsubscribe(subTopic.ID())
	coord.Unsubscribe(subWildcard.ID())
	coord.Unsubscribe(subInactive.ID())

	if coord.hasSubscribers() {
		t.Fatalf("expected hasSubscribers to be false after unregistering all")
	}
}

func TestCycle151_SubscriberTopologies_DefaultOperationalRouterFallback(t *testing.T) {
	sub := &testCycle151Subscriber{id: "sub-fb", active: true}
	ev := NewOperationalEvent(NewEventContext("op-fb", "test", OperationStatusStart))

	opRouterWithNilCoord := &DefaultOperationalRouter{coordinator: nil}
	if err := opRouterWithNilCoord.Emit(context.Background(), ev, []OperationalEventSubscriber{sub}); err != nil {
		t.Fatalf("unexpected error from DefaultOperationalRouter.Emit: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if sub.received.Load() != 1 {
		t.Fatalf("expected sub to receive 1 event via fallback goroutine, got %d", sub.received.Load())
	}
}

func TestCycle151_ProgressHelper_ComprehensiveReporting(t *testing.T) {
	ctx := context.Background()

	nilHelper := NewProgressHelper(nil, "/tmp", "op-nil", "type-nil", "system")
	if err := nilHelper.EmitProgress(ctx, 1, 10, "step 1", nil, false); err != nil {
		t.Fatalf("unexpected error on nil helper EmitProgress: %v", err)
	}
	if err := nilHelper.Drain(10 * time.Millisecond); err != nil {
		t.Fatalf("unexpected error on nil helper Drain: %v", err)
	}

	coord := NewCoordinator(CoordinatorConfig{})
	helper := NewProgressHelper(coord, "/tmp", "op-prog", "type-prog", "human")

	if err := helper.EmitProgress(ctx, 0, 0, "starting", nil, false); err != nil {
		t.Fatalf("unexpected error on EmitProgress zero total: %v", err)
	}

	if err := helper.EmitProgress(ctx, 5, 10, "halfway", map[string]any{"step": "calc"}, true); err != nil {
		t.Fatalf("unexpected error on EmitProgress with audit: %v", err)
	}

	if err := helper.EmitProgressSummary(ctx, "50%", 5, 10, "summary checkpoint", map[string]any{"chk": 1}); err != nil {
		t.Fatalf("unexpected error on EmitProgressSummary: %v", err)
	}

	if err := helper.EmitStatusChange(ctx, "init", "running", "started", map[string]any{"k": "v"}); err != nil {
		t.Fatalf("unexpected error on EmitStatusChange: %v", err)
	}
	if err := helper.EmitStatusChange(ctx, "init", "running", "dup", map[string]any{"k": "v"}); err != nil {
		t.Fatalf("unexpected error on duplicate EmitStatusChange: %v", err)
	}
	if err := helper.EmitStatusChange(ctx, "running", "running", "heartbeat", nil); err != nil {
		t.Fatalf("unexpected error on heartbeat EmitStatusChange: %v", err)
	}

	if err := helper.EmitError(ctx, errors.New("subsystem failed"), "failure msg", map[string]any{"code": 500}); err != nil {
		t.Fatalf("unexpected error on EmitError: %v", err)
	}
	if err := helper.EmitError(ctx, nil, "", nil); err != nil {
		t.Fatalf("unexpected error on EmitError nil: %v", err)
	}

	if err := helper.EmitCompletion(ctx, 150*time.Millisecond, "finished execution", map[string]any{"metrics": 1}); err != nil {
		t.Fatalf("unexpected error on EmitCompletion: %v", err)
	}
	if err := helper.EmitCompletion(ctx, 0, "", nil); err != nil {
		t.Fatalf("unexpected error on EmitCompletion zero: %v", err)
	}

	if err := helper.Drain(50 * time.Millisecond); err != nil {
		t.Fatalf("unexpected error on helper Drain: %v", err)
	}
}

func TestCycle151_OperationCallback_ManagerLifecycle(t *testing.T) {
	ctx := context.Background()

	cbNil := NewCoordinatorOperationCallback(ctx, "", nil, "nil_op", "system")
	cbNil.OnStart("op-nil-1", nil)
	cbNil.OnProgress("op-nil-1", 1, 10, "prog")
	cbNil.OnComplete("op-nil-1", "result", time.Second)
	cbNil.OnError("op-nil-1", errors.New("err"))
	cbNil.OnCancel("op-nil-1", "reason")

	testRoot, fileStorage, cleanup := setupCoordinationCompleteTestEnvironment(t, nil)
	defer cleanup()

	cb := NewCoordinatorOperationCallback(ctx, testRoot, fileStorage, "test_job", "human")
	cb.OnStart("op-job-1", map[string]any{"task": "init"})
	cb.OnProgress("op-job-1", 5, 10, "processing")
	cb.OnComplete("op-job-1", "completed_result", 200*time.Millisecond)
	cb.OnError("op-job-1", errors.New("transient error"))
	cb.OnCancel("op-job-1", "user requested cancel")

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		step := i
		goroutinelabels.NewGoroutine("test_concurrent_op_callback", "concurrent operation callback progress").
			StartSimple(func() {
				defer wg.Done()
				cb.OnProgress("op-concurrent", step, 10, "concurrent progress")
			})
	}
	wg.Wait()
}

func TestCycle151_OperationCallback_WithCustomGlobalCoordinator(t *testing.T) {
	ctx := context.Background()
	testRoot, fileStorage, cleanup := setupCoordinationCompleteTestEnvironment(t, nil)
	defer cleanup()

	customCoord := &testCycle151CustomCoordinator{}
	SetGlobalCoordinator(customCoord)
	defer ResetGlobalCoordinator()

	cb := NewCoordinatorOperationCallback(ctx, testRoot, fileStorage, "custom_global_test", "human")
	cb.OnStart("op-custom-1", nil)
	cb.OnProgress("op-custom-1", 5, 10, "progress")
	cb.OnComplete("op-custom-1", "custom_res", 100*time.Millisecond)

	time.Sleep(100 * time.Millisecond)
	if customCoord.emitCalls.Load() == 0 {
		t.Fatalf("expected custom global coordinator to receive Emit calls")
	}
}

func TestCycle151_CLINotifier_AdapterAndHelperComprehensive(t *testing.T) {
	ctx := context.Background()

	nilAdapter := NewCLINotifierAdapter(nil)
	if err := nilAdapter.EmitProgress(ctx, "op1", "type1", 1, 10, "msg", nil, false); err != nil {
		t.Fatalf("unexpected error on nil adapter EmitProgress: %v", err)
	}
	if err := nilAdapter.EmitStatusChange(ctx, "op1", "type1", "old", "new", "msg", nil); err != nil {
		t.Fatalf("unexpected error on nil adapter EmitStatusChange: %v", err)
	}
	if err := nilAdapter.EmitError(ctx, "op1", "type1", errors.New("fail"), "msg", nil); err != nil {
		t.Fatalf("unexpected error on nil adapter EmitError: %v", err)
	}
	if err := nilAdapter.EmitCompletion(ctx, "op1", "type1", time.Second, "msg", nil); err != nil {
		t.Fatalf("unexpected error on nil adapter EmitCompletion: %v", err)
	}

	testRoot, fileStorage, cleanup := setupCoordinationCompleteTestEnvironment(t, nil)
	defer cleanup()

	noop1 := NewCLINotifierWithCoordinator(false, false, "", fileStorage, "op1", "task", "human")
	if _, ok := noop1.(storage.NoopOperationNotifier); !ok {
		t.Fatalf("expected NoopOperationNotifier when projectRoot is empty")
	}

	noop2 := NewCLINotifierWithCoordinator(false, false, testRoot, nil, "op2", "task", "human")
	if _, ok := noop2.(storage.NoopOperationNotifier); !ok {
		t.Fatalf("expected NoopOperationNotifier when storageProvider is nil")
	}

	validNotifier := NewCLINotifierWithCoordinator(false, false, testRoot, fileStorage, "op3", "task", "human")
	if validNotifier == nil {
		t.Fatalf("expected non-nil OperationNotifier")
	}
}

func TestCycle151_MetricsCollectorAdapter_EmitMetricCollectedAndCreate(t *testing.T) {
	ctx := context.Background()

	nilAdapter := NewMetricsCollectorAdapter(nil)
	nilAdapter.EmitMetricCollected(ctx, "storage_write", "metric-1", time.Now().Add(-time.Minute), time.Now(), nil)
	nilAdapter.EmitMetricCollected(ctx, "storage_write", "metric-2", time.Now().Add(-time.Minute), time.Now(), errors.New("metric failure"))

	customCoord := &testCycle151CustomCoordinator{}
	adapter := NewMetricsCollectorAdapter(customCoord)
	adapter.EmitMetricCollected(ctx, "storage_read", "metric-3", time.Now().Add(-time.Minute), time.Now(), nil)
	adapter.EmitMetricCollected(ctx, "storage_read", "metric-4", time.Now().Add(-time.Minute), time.Now(), errors.New("read failed"))

	if customCoord.emitCalls.Load() != 2 {
		t.Fatalf("expected adapter to emit 2 events via coordinator, got %d", customCoord.emitCalls.Load())
	}

	testRoot, fileStorage, cleanup := setupCoordinationCompleteTestEnvironment(t, nil)
	defer cleanup()

	collector := CreateUnifiedMetricsCollector(customCoord, fileStorage, nil, false, false)
	if collector == nil {
		t.Fatalf("expected non-nil unified metrics collector")
	}
	if testRoot == "" {
		t.Fatalf("expected non-empty test root")
	}
}

func TestCycle151_StorageAuditRouter_EdgeCasesAndInference(t *testing.T) {
	testRoot, fileStorage, cleanup := setupCoordinationCompleteTestEnvironment(t, nil)
	defer cleanup()

	router := NewStorageAuditRouter(testRoot, fileStorage)
	if router == nil {
		t.Fatalf("expected non-nil router")
	}

	coord := NewStorageCoordinator(testRoot, fileStorage)
	if coord == nil {
		t.Fatalf("expected non-nil coordinator")
	}

	routerGen := StorageAuditRouterForProject(testRoot, fileStorage)
	if routerGen == nil {
		t.Fatalf("expected non-nil AuditRouter")
	}

	auditRouterDirect := NewAuditRouter(testRoot, fileStorage)
	if auditRouterDirect == nil {
		t.Fatalf("expected non-nil StorageAuditRouter")
	}

	if err := router.Emit(context.Background(), &EventContext{}); err != nil {
		t.Fatalf("unexpected error emitting empty EventContext: %v", err)
	}

	var callbackFired atomic.Bool
	router.SetCallback(func(eventType string, operationID string, err error) {
		callbackFired.Store(true)
	})
	if !router.HasCallback() {
		t.Fatalf("expected HasCallback to return true")
	}

	evCtx := NewEventContext("op-aud-edge", "system_task", OperationStatusStart).
		WithEventData(&EventData{
			AuditMetadata: map[string]any{
				objects.FieldKeyTargetKind: "test_kind",
				objects.FieldKeyTargetID:   "id-100",
				objects.FieldKeyTargetPath: "/path/test",
				objects.FieldKeySessionID:  "sess-200",
			},
		})

	if err := router.Emit(context.Background(), evCtx); err != nil {
		t.Fatalf("unexpected error emitting audit event with inference: %v", err)
	}

	if !callbackFired.Load() {
		t.Fatalf("expected audit callback to fire")
	}
}

func TestCycle151_NewCoordinatorProgressHelper(t *testing.T) {
	ResetGlobalCoordinator()
	ph1 := NewCoordinatorProgressHelper("/tmp", "op1", "type1", "system")
	if ph1 == nil {
		t.Fatalf("expected non-nil progress helper")
	}

	custom := NewCoordinator(CoordinatorConfig{})
	SetGlobalCoordinator(custom)
	defer ResetGlobalCoordinator()

	ph2 := NewCoordinatorProgressHelper("/tmp", "op2", "type2", "human")
	if ph2 == nil {
		t.Fatalf("expected non-nil progress helper")
	}
}
