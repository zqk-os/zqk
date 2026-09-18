package coordination

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// EventCoordinator is the central coordinator that routes events to all channels
// This is the "spinal cord" of the system - all events pass through here
type EventCoordinator interface {
	// Emit routes an event to all appropriate channels based on the EventContext
	// This is the main entry point for event emission
	Emit(ctx context.Context, eventCtx *EventContext) error

	// Subscribe adds a subscriber for operational events
	// Returns the subscriber ID
	Subscribe(subscriber OperationalEventSubscriber) string

	// Unsubscribe removes a subscriber
	Unsubscribe(subscriberID string)
}

// Coordinator is the default implementation of EventCoordinator
type coordinatorState struct {
	subscribers map[string]OperationalEventSubscriber
	typeIndex   map[string][]string
}

type Coordinator struct {
	state atomic.Pointer[coordinatorState]
	mu    sync.Mutex // For writing updates

	// Channel routers
	loggingRouter     LoggingRouter
	auditRouter       AuditRouter
	metricsRouter     MetricsRouter
	operationalRouter OperationalRouter
}

// LoggingRouter routes events to the logging channel
type LoggingRouter interface {
	Emit(ctx context.Context, eventCtx *EventContext) error
}

// AuditRouter routes events to the audit channel
type AuditRouter interface {
	Emit(ctx context.Context, eventCtx *EventContext) error
}

// MetricsRouter routes events to the metrics channel
type MetricsRouter interface {
	Emit(ctx context.Context, eventCtx *EventContext) error
}

// OperationalRouter routes events to operational subscribers
type OperationalRouter interface {
	Emit(ctx context.Context, event *OperationalEvent, subscribers []OperationalEventSubscriber) error
}

// CoordinatorConfig configures a Coordinator
type CoordinatorConfig struct {
	LoggingRouter     LoggingRouter
	AuditRouter       AuditRouter
	MetricsRouter     MetricsRouter
	OperationalRouter OperationalRouter
}

// NewCoordinator creates a new Coordinator with the specified routers
func NewCoordinator(config CoordinatorConfig) *Coordinator {
	if config.LoggingRouter == nil {
		config.LoggingRouter = &DefaultLoggingRouter{}
	}
	if config.AuditRouter == nil {
		config.AuditRouter = &DefaultAuditRouter{}
	}
	if config.MetricsRouter == nil {
		config.MetricsRouter = &DefaultMetricsRouter{}
	}
	if config.OperationalRouter == nil {
		config.OperationalRouter = &DefaultOperationalRouter{}
	}

	c := &Coordinator{
		loggingRouter:     config.LoggingRouter,
		auditRouter:       config.AuditRouter,
		metricsRouter:     config.MetricsRouter,
		operationalRouter: config.OperationalRouter,
	}
	c.state.Store(&coordinatorState{
		subscribers: make(map[string]OperationalEventSubscriber),
		typeIndex:   make(map[string][]string),
	})
	return c
}

// Emit routes an event to all appropriate channels.
// Stages are composed with pkg/pipeline (see coordinator_emit_pipeline.go); router timing uses
// runCoordinatorRouterSync / runCoordinatorRouterAsync so context.WithTimeout cancellation matches
// where work runs (coordinator_emit_stage.go).
func (c *Coordinator) Emit(ctx context.Context, eventCtx *EventContext) error {
	if eventCtx == nil {
		return errfmt.Errorf("event context cannot be nil")
	}

	// Use event context's Go context if available, otherwise use provided context
	emitCtx := ctx
	if eventCtx.Ctx != nil {
		emitCtx = eventCtx.Ctx
	}

	pctx := &pipeline.Context{
		Ctx:            emitCtx,
		IdempotencyKey: coordinatorEmitIdempotencyKey(eventCtx),
		PartitionKey:   coordinatorEmitPartitionKey(eventCtx),
		Outcome:        make(map[string]any),
	}
	pl := c.buildEmitPipeline(eventCtx)
	_, err := pl.Run(pctx, eventCtx)
	return err
}

// emitLoggingRouter, emitAuditRouter, and emitMetricsRouter delegate to the configured routers.
// They are named methods (not anonymous funcs inside buildEmitPipeline) so pkg/logging.EventLogger
// caller attribution via runtime.Caller(3) in LogError/LogInfo records this file and a stable line,
// instead of a pipeline stage closure in coordinator_emit_pipeline.go (misleading for scheduler_job logs).
func (c *Coordinator) emitLoggingRouter(ctx context.Context, ev *EventContext) error {
	return c.loggingRouter.Emit(ctx, ev)
}

func (c *Coordinator) emitAuditRouter(ctx context.Context, ev *EventContext) error {
	return c.auditRouter.Emit(ctx, ev)
}

func (c *Coordinator) emitMetricsRouter(ctx context.Context, ev *EventContext) error {
	return c.metricsRouter.Emit(ctx, ev)
}

// EmitOperationalSync emits an operational event synchronously (blocks until subscribers process it)
// This is used for CLI subscribers that need immediate feedback
func (c *Coordinator) EmitOperationalSync(ctx context.Context, eventCtx *EventContext) error {
	if eventCtx == nil {
		return errfmt.Errorf("event context cannot be nil")
	}

	if !eventCtx.EmitOperational {
		return nil
	}

	// Use event context's Go context if available, otherwise use provided context
	emitCtx := ctx
	if eventCtx.Ctx != nil {
		emitCtx = eventCtx.Ctx
	}

	// Emit synchronously by calling subscribers directly (bypass async router)
	c.emitOperationalEventSync(emitCtx, eventCtx)
	return nil
}

// emitOperationalEventSync emits an operational event synchronously to subscribers
// This bypasses the async router to ensure immediate delivery for CLI feedback
func (c *Coordinator) emitOperationalEventSync(_ context.Context, eventCtx *EventContext) {
	operationalEvent := NewOperationalEvent(eventCtx)

	// Find relevant subscribers (same logic as async version)
	var subscribers []OperationalEventSubscriber
	state := c.state.Load()

	subscriberIDs, exists := state.typeIndex[operationalEvent.Type]
	if !exists {
		subscriberIDs = []string{}
	}

	for _, id := range subscriberIDs {
		if sub, exists := state.subscribers[id]; exists && sub.IsActive() {
			subscribers = append(subscribers, sub)
		}
	}

	for _, sub := range state.subscribers {
		if sub.IsActive() {
			if len(sub.EventTypes()) == 0 {
				subscribers = append(subscribers, sub)
			}
		}
	}

	// Call subscribers synchronously (no goroutines, no router)
	// This ensures immediate delivery for CLI feedback
	for _, subscriber := range subscribers {
		_ = subscriber.HandleEvent(operationalEvent) //nolint:errcheck // Synchronous, best-effort
	}
}

// emitOperationalEvent emits an operational event to relevant subscribers
func (c *Coordinator) emitOperationalEvent(ctx context.Context, eventCtx *EventContext) {
	operationalEvent := NewOperationalEvent(eventCtx)

	// Find relevant subscribers
	var subscribers []OperationalEventSubscriber
	state := c.state.Load()

	subscriberIDs, exists := state.typeIndex[operationalEvent.Type]
	if !exists {
		subscriberIDs = []string{}
	}

	for _, id := range subscriberIDs {
		if sub, exists := state.subscribers[id]; exists && sub.IsActive() {
			subscribers = append(subscribers, sub)
		}
	}

	for _, sub := range state.subscribers {
		if sub.IsActive() {
			if len(sub.EventTypes()) == 0 {
				subscribers = append(subscribers, sub)
			}
		}
	}

	// Route to operational router
	_ = c.operationalRouter.Emit(ctx, operationalEvent, subscribers) //nolint:errcheck // Async, best-effort
}

// Subscribe adds a subscriber for operational events
func (c *Coordinator) Subscribe(subscriber OperationalEventSubscriber) string {
	if subscriber == nil {
		return ""
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	oldState := c.state.Load()
	newState := &coordinatorState{
		subscribers: make(map[string]OperationalEventSubscriber, len(oldState.subscribers)+1),
		typeIndex:   make(map[string][]string, len(oldState.typeIndex)),
	}

	for k, v := range oldState.subscribers {
		newState.subscribers[k] = v
	}
	for k, v := range oldState.typeIndex {
		newSlice := make([]string, len(v))
		copy(newSlice, v)
		newState.typeIndex[k] = newSlice
	}

	subscriberID := subscriber.ID()
	newState.subscribers[subscriberID] = subscriber

	eventTypes := subscriber.EventTypes()
	if len(eventTypes) > 0 {
		for _, eventType := range eventTypes {
			newState.typeIndex[eventType] = append(newState.typeIndex[eventType], subscriberID)
		}
	}

	c.state.Store(newState)

	return subscriberID
}

// hasSubscribers returns true if at least one subscriber is registered (used to run operational delivery sync)
func (c *Coordinator) hasSubscribers() bool {
	return len(c.state.Load().subscribers) > 0
}

// Unsubscribe removes a subscriber
func (c *Coordinator) Unsubscribe(subscriberID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	oldState := c.state.Load()
	if _, exists := oldState.subscribers[subscriberID]; !exists {
		return
	}

	newState := &coordinatorState{
		subscribers: make(map[string]OperationalEventSubscriber, len(oldState.subscribers)-1),
		typeIndex:   make(map[string][]string, len(oldState.typeIndex)),
	}

	for k, v := range oldState.subscribers {
		if k != subscriberID {
			newState.subscribers[k] = v
		}
	}

	for k, v := range oldState.typeIndex {
		var newSlice []string
		for _, id := range v {
			if id != subscriberID {
				newSlice = append(newSlice, id)
			}
		}
		if len(newSlice) > 0 {
			newState.typeIndex[k] = newSlice
		}
	}

	c.state.Store(newState)
}

// DefaultLoggingRouter is the default implementation of LoggingRouter
// Uses pkg/logging.EventLogger
type DefaultLoggingRouter struct{}

func (r *DefaultLoggingRouter) Emit(ctx context.Context, eventCtx *EventContext) error {
	if eventCtx.EventData == nil || len(eventCtx.EventData.LoggingFields) == 0 {
		return nil
	}

	eventLogger := logging.NewEventLogger(ctx)

	// Convert LoggingField to logging.Field
	fields := make([]logging.Field, 0, len(eventCtx.EventData.LoggingFields))
	for _, field := range eventCtx.EventData.LoggingFields {
		fields = append(fields, logging.Field{
			Key:   field.Key,
			Value: field.Value,
		})
	}

	// Log based on status
	// Prefer "message" (progress/status text) then "event"; otherwise use operation type
	message := eventCtx.OperationType
	var eventMsg string
	for _, field := range fields {
		if field.Value == nil {
			continue
		}
		if msg, ok := field.Value.(string); ok && msg != emptyValue {
			if field.Key == "message" {
				message = msg
				break
			}
			if field.Key == "event" {
				eventMsg = msg
			}
		}
	}
	if message == eventCtx.OperationType && eventMsg != emptyValue {
		message = eventMsg
	}

	if eventCtx.Status == OperationStatusError && eventCtx.Error != nil {
		// TRACK: storage.IsExpectedObjectGetMiss — do not import storage (cycle).
		if expectedObjectGetMiss(eventCtx.Error) {
			eventLogger.LogWarning(message, fields...)
		} else {
			eventLogger.LogError(message, eventCtx.Error, fields...)
		}
	} else {
		level := resolveLogLevel(eventCtx, message, fields)
		switch level {
		case logging.DebugLevel:
			eventLogger.LogDebug(message, fields...)
		case logging.WarnLevel:
			eventLogger.LogWarning(message, fields...)
		case logging.ErrorLevel:
			eventLogger.LogError(message, nil, fields...)
		default:
			eventLogger.LogInfo(message, fields...)
		}
	}

	return nil
}

func resolveLogLevel(eventCtx *EventContext, message string, fields []logging.Field) logging.LogLevel {
	// 1. Check field-level explicit overrides
	for _, field := range fields {
		if field.Key == "level" || field.Key == "log_level" {
			if s, ok := field.Value.(string); ok {
				switch strings.ToLower(s) {
				case "debug":
					return logging.DebugLevel
				case "info":
					return logging.InfoLevel
				case "warn", "warning":
					return logging.WarnLevel
				case "error":
					return logging.ErrorLevel
				}
			}
		}
	}

	// 2. Check EventContext explicit level
	if eventCtx != nil && eventCtx.Level != "" {
		switch strings.ToLower(eventCtx.Level) {
		case "debug":
			return logging.DebugLevel
		case "info":
			return logging.InfoLevel
		case "warn", "warning":
			return logging.WarnLevel
		case "error":
			return logging.ErrorLevel
		}
	}

	if eventCtx == nil {
		return logging.InfoLevel
	}

	// 3. Dynamic Environment overrides to easily switch log levels without code changes
	// Set ZQK_INFO_OPERATIONS=queue_shutdown,system_check or ZQK_INFO_OPERATIONS=* to force info
	if infoOps := zqkenv.Get("ZQK_INFO_OPERATIONS").OrDefault(""); infoOps != "" {
		for _, op := range strings.Split(infoOps, ",") {
			op = strings.TrimSpace(op)
			if op == "*" || strings.EqualFold(op, eventCtx.OperationType) {
				return logging.InfoLevel
			}
		}
	}
	// Set ZQK_DEBUG_OPERATIONS=queue_shutdown,system_check or ZQK_DEBUG_OPERATIONS=* to force debug
	if debugOps := zqkenv.Get("ZQK_DEBUG_OPERATIONS").OrDefault(""); debugOps != "" {
		for _, op := range strings.Split(debugOps, ",") {
			op = strings.TrimSpace(op)
			if op == "*" || strings.EqualFold(op, eventCtx.OperationType) {
				return logging.DebugLevel
			}
		}
	}

	// 4. Default progress / intermediate steps to Debug
	if eventCtx.Status == OperationStatusProgress || eventCtx.Status == "spec" || eventCtx.Status == "lifecycle" {
		return logging.DebugLevel
	}
	if eventCtx.OperationType == "queue_shutdown" {
		return logging.DebugLevel
	}

	return logging.InfoLevel
}

// expectedObjectGetMiss mirrors storage.IsExpectedObjectGetMiss without importing
// pkg/storage (coordination ↔ storage cycle via unified_metrics_collector_example).
// TRACK: storage.IsExpectedObjectGetMiss — keep these needles in sync.
func expectedObjectGetMiss(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "object not found") ||
		strings.Contains(msg, "id not found") ||
		strings.Contains(msg, "could not infer kind") ||
		strings.Contains(msg, "account acc-system not found") ||
		strings.Contains(msg, "missing token in")
}

// DefaultAuditRouter is a no-op placeholder
// Use NewStorageAuditRouter from routers.go for actual audit event creation
type DefaultAuditRouter struct{}

func (r *DefaultAuditRouter) Emit(ctx context.Context, eventCtx *EventContext) error {
	// No-op - use StorageAuditRouter for actual implementation
	return nil
}

// DefaultMetricsRouter is a no-op placeholder
// Use NewMetricPipelineRouter from routers.go for actual metrics emission
type DefaultMetricsRouter struct{}

func (r *DefaultMetricsRouter) Emit(ctx context.Context, eventCtx *EventContext) error {
	// No-op - use MetricPipelineRouter for actual implementation
	return nil
}

// DefaultOperationalRouter routes operational events to subscribers
type DefaultOperationalRouter struct{}

func (r *DefaultOperationalRouter) Emit(ctx context.Context, event *OperationalEvent, subscribers []OperationalEventSubscriber) error {
	// Non-blocking, best-effort: one async stage per subscriber (same cancel pairing as Emit operational branch).
	for _, subscriber := range subscribers {
		subRef := subscriber
		sid := subRef.ID()
		runCoordinatorRouterAsync(ctx, "coordination_event_subscriber", fmt.Sprintf("coordination.emit/TRIGGER_subscriber/%s", sid), func(ctx context.Context) {
			_ = subRef.HandleEvent(event) //nolint:errcheck // Async, best-effort
		})
	}
	return nil
}
