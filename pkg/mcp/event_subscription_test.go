package mcp

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/logging"
)

// mockEventSubscriber is a test implementation of EventSubscriber
type mockEventSubscriber struct {
	id         string
	eventTypes []EventType
	events     []*Event
	mu         sync.RWMutex
	active     bool
	shouldFail bool // If true, SendEvent returns false
}

func newMockEventSubscriber(id string, eventTypes []EventType) *mockEventSubscriber {
	return &mockEventSubscriber{
		id:         id,
		eventTypes: eventTypes,
		events:     make([]*Event, 0),
		active:     true,
	}
}

func (m *mockEventSubscriber) ID() string {
	return m.id
}

func (m *mockEventSubscriber) EventTypes() []EventType {
	var eventTypes []EventType
	_ = concurrency.RunInRLockWithLogger(
		&m.mu, LockNameMockEventSubscriberEventTypes, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			eventTypes = m.eventTypes
			return nil
		},
	)
	return eventTypes
}

func (m *mockEventSubscriber) SendEvent(event *Event) bool {
	var success bool
	_ = concurrency.RunInLockWithLogger(
		&m.mu, LockNameMockEventSubscriberSendEvent, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if !m.active || m.shouldFail {
				success = false
				return nil
			}

			m.events = append(m.events, event)
			success = true
			return nil
		},
	)
	return success
}

func (m *mockEventSubscriber) IsActive() bool {
	var active bool
	_ = concurrency.RunInRLockWithLogger(
		&m.mu, LockNameMockEventSubscriberIsActive, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			active = m.active
			return nil
		},
	)
	return active
}

func (m *mockEventSubscriber) SetActive(active bool) {
	_ = concurrency.RunInLockWithLogger(
		&m.mu, LockNameMockEventSubscriberSetActive, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			m.active = active
			return nil
		},
	)
}

func (m *mockEventSubscriber) SetShouldFail(shouldFail bool) {
	_ = concurrency.RunInLockWithLogger(
		&m.mu, LockNameMockEventSubscriberSetShouldFail, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			m.shouldFail = shouldFail
			return nil
		},
	)
}

func (m *mockEventSubscriber) GetEvents() []*Event {
	var result []*Event
	_ = concurrency.RunInRLockWithLogger(
		&m.mu, LockNameMockEventSubscriberGetEvents, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			result = make([]*Event, len(m.events))
			copy(result, m.events)
			return nil
		},
	)
	return result
}

func (m *mockEventSubscriber) ClearEvents() {
	_ = concurrency.RunInLockWithLogger(
		&m.mu, LockNameMockEventSubscriberClearEvents, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			m.events = make([]*Event, 0)
			return nil
		},
	)
}

// TestEventEmitter_Subscribe tests basic subscription functionality
func TestEventEmitter_Subscribe(t *testing.T) {
	t.Parallel()
	emitter := NewEventEmitter(100)

	subscriber := newMockEventSubscriber("sub1", []EventType{EventTypeLogDebug, EventTypeLogInfo})

	subID := emitter.Subscribe(subscriber)

	if subID != subscriber.ID() {
		t.Errorf("Expected subscription ID %s, got %s", subscriber.ID(), subID)
	}

	if emitter.GetSubscriberCount() != 1 {
		t.Errorf("Expected 1 subscriber, got %d", emitter.GetSubscriberCount())
	}

	if emitter.GetSubscriberCountByType(EventTypeLogDebug) != 1 {
		t.Errorf("Expected 1 subscriber for log.debug, got %d", emitter.GetSubscriberCountByType(EventTypeLogDebug))
	}
}

// TestEventEmitter_Unsubscribe tests unsubscription
func TestEventEmitter_Unsubscribe(t *testing.T) {
	t.Parallel()
	emitter := NewEventEmitter(100)

	subscriber := newMockEventSubscriber("sub1", []EventType{EventTypeLogDebug})
	emitter.Subscribe(subscriber)

	if emitter.GetSubscriberCount() != 1 {
		t.Errorf("Expected 1 subscriber before unsubscribe, got %d", emitter.GetSubscriberCount())
	}

	emitter.Unsubscribe(subscriber.ID())

	if emitter.GetSubscriberCount() != 0 {
		t.Errorf("Expected 0 subscribers after unsubscribe, got %d", emitter.GetSubscriberCount())
	}
}

// TestEventEmitter_Emit tests event emission to subscribers
func TestEventEmitter_Emit(t *testing.T) {
	t.Parallel()
	emitter := NewEventEmitter(100)

	subscriber := newMockEventSubscriber("sub1", []EventType{EventTypeLogDebug})
	emitter.Subscribe(subscriber)

	event := &Event{
		Type:    EventTypeLogDebug,
		Message: "Test debug message",
		Fields:  map[string]any{"key": "value"},
	}

	sentCount := emitter.Emit(event)

	if sentCount != 1 {
		t.Errorf("Expected 1 event sent, got %d", sentCount)
	}

	events := subscriber.GetEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event received, got %d", len(events))
	}

	if events[0].Type != EventTypeLogDebug {
		t.Errorf("Expected event type log.debug, got %s", events[0].Type)
	}

	if events[0].Message != "Test debug message" {
		t.Errorf("Expected message 'Test debug message', got '%s'", events[0].Message)
	}
}

// TestEventEmitter_Broadcast_MultipleSubscribersReceiveSameEvent verifies that when one event is emitted,
// all subscribers subscribed to that event type receive it (broadcast semantics).
func TestEventEmitter_Broadcast_MultipleSubscribersReceiveSameEvent(t *testing.T) {
	t.Parallel()
	emitter := NewEventEmitter(100)

	agentA := newMockEventSubscriber("agent_a", []EventType{EventTypeToolStarted})
	agentB := newMockEventSubscriber("agent_b", []EventType{EventTypeToolStarted})
	emitter.Subscribe(agentA)
	emitter.Subscribe(agentB)

	event := &Event{
		Type:    EventTypeToolStarted,
		Message: "Tool execution started: object list",
		Fields:  map[string]any{"tool": "object list"},
	}
	sentCount := emitter.Emit(event)
	if sentCount != 2 {
		t.Errorf("Expected 2 events sent (broadcast), got %d", sentCount)
	}

	for _, sub := range []*mockEventSubscriber{agentA, agentB} {
		events := sub.GetEvents()
		if len(events) != 1 {
			t.Errorf("Expected subscriber %s to receive 1 event, got %d", sub.ID(), len(events))
		} else if events[0].Type != EventTypeToolStarted || events[0].Message != event.Message {
			t.Errorf("Subscriber %s: expected same event, got %+v", sub.ID(), events[0])
		}
	}
}

// TestEventEmitter_ClientToClient_TwoAgentsReceiveOperationalEvent verifies agent-to-agent notification:
// two clients (agents) subscribed to the same operational event type both receive the event when it is emitted.
func TestEventEmitter_ClientToClient_TwoAgentsReceiveOperationalEvent(t *testing.T) {
	t.Parallel()
	emitter := NewEventEmitter(100)

	client1 := newMockEventSubscriber("client_agent_1", []EventType{EventTypeToolCompleted})
	client2 := newMockEventSubscriber("client_agent_2", []EventType{EventTypeToolCompleted})
	emitter.Subscribe(client1)
	emitter.Subscribe(client2)

	operationalEvent := &Event{
		Type:    EventTypeToolCompleted,
		Message: "object_list completed",
		Fields:  map[string]any{objects.FieldKeyOperation: "object_list", objects.FieldKeyStatus: objects.ObjectStatusComplete},
	}
	n := emitter.Emit(operationalEvent)
	if n != 2 {
		t.Errorf("Expected both clients to receive event (client-to-client), got %d", n)
	}

	c1Events := client1.GetEvents()
	c2Events := client2.GetEvents()
	if len(c1Events) != 1 || len(c2Events) != 1 {
		t.Errorf("Each client should receive 1 event: client1=%d, client2=%d", len(c1Events), len(c2Events))
	}
	if len(c1Events) == 1 && c1Events[0].Fields[objects.FieldKeyOperation] != "object_list" {
		t.Errorf("client1: expected operation=object_list, got %v", c1Events[0].Fields[objects.FieldKeyOperation])
	}
	if len(c2Events) == 1 && c2Events[0].Fields[objects.FieldKeyOperation] != "object_list" {
		t.Errorf("client2: expected operation=object_list, got %v", c2Events[0].Fields[objects.FieldKeyOperation])
	}
}

// TestEventEmitter_Emit_Filtering tests that subscribers only receive events they're interested in
func TestEventEmitter_Emit_Filtering(t *testing.T) {
	t.Parallel()
	emitter := NewEventEmitter(100)

	sub1 := newMockEventSubscriber("sub1", []EventType{EventTypeLogDebug})
	sub2 := newMockEventSubscriber("sub2", []EventType{EventTypeLogInfo})

	emitter.Subscribe(sub1)
	emitter.Subscribe(sub2)

	// Emit debug event
	debugEvent := &Event{
		Type:    EventTypeLogDebug,
		Message: "Debug message",
	}
	emitter.Emit(debugEvent)

	// Emit info event
	infoEvent := &Event{
		Type:    EventTypeLogInfo,
		Message: "Info message",
	}
	emitter.Emit(infoEvent)

	// Check sub1 only received debug event
	sub1Events := sub1.GetEvents()
	if len(sub1Events) != 1 {
		t.Errorf("Expected sub1 to receive 1 event, got %d", len(sub1Events))
	}
	if sub1Events[0].Type != EventTypeLogDebug {
		t.Errorf("Expected sub1 to receive log.debug, got %s", sub1Events[0].Type)
	}

	// Check sub2 only received info event
	sub2Events := sub2.GetEvents()
	if len(sub2Events) != 1 {
		t.Errorf("Expected sub2 to receive 1 event, got %d", len(sub2Events))
	}
	if sub2Events[0].Type != EventTypeLogInfo {
		t.Errorf("Expected sub2 to receive log.info, got %s", sub2Events[0].Type)
	}
}

// TestEventEmitter_Emit_InactiveSubscriber tests cleanup of inactive subscribers
func TestEventEmitter_Emit_InactiveSubscriber(t *testing.T) {
	t.Parallel()
	emitter := NewEventEmitter(100)

	subscriber := newMockEventSubscriber("sub1", []EventType{EventTypeLogDebug})
	emitter.Subscribe(subscriber)

	// Make subscriber inactive
	subscriber.SetActive(false)

	event := &Event{
		Type:    EventTypeLogDebug,
		Message: "Test message",
	}

	sentCount := emitter.Emit(event)

	if sentCount != 0 {
		t.Errorf("Expected 0 events sent to inactive subscriber, got %d", sentCount)
	}

	if emitter.GetSubscriberCount() != 0 {
		t.Errorf("Expected inactive subscriber omitted from count immediately, got %d", emitter.GetSubscriberCount())
	}

	// Give cleanup goroutine time to run
	time.Sleep(10 * time.Millisecond)

	if emitter.GetSubscriberCount() != 0 {
		t.Errorf("Expected inactive subscriber to be cleaned up, but count is %d", emitter.GetSubscriberCount())
	}
}

// TestEventEmitter_Emit_FailingSubscriber tests cleanup when SendEvent returns false
func TestEventEmitter_Emit_FailingSubscriber(t *testing.T) {
	t.Parallel()
	emitter := NewEventEmitter(100)

	subscriber := newMockEventSubscriber("sub1", []EventType{EventTypeLogDebug})
	emitter.Subscribe(subscriber)

	// Make subscriber fail
	subscriber.SetShouldFail(true)

	event := &Event{
		Type:    EventTypeLogDebug,
		Message: "Test message",
	}

	sentCount := emitter.Emit(event)

	if sentCount != 0 {
		t.Errorf("Expected 0 events sent to failing subscriber, got %d", sentCount)
	}

	// Give cleanup goroutine time to run
	time.Sleep(10 * time.Millisecond)

	if emitter.GetSubscriberCount() != 0 {
		t.Errorf("Expected failing subscriber to be cleaned up, but count is %d", emitter.GetSubscriberCount())
	}
}

// TestEventEmitter_Cleanup tests manual cleanup of inactive subscribers
func TestEventEmitter_Cleanup(t *testing.T) {
	t.Parallel()
	emitter := NewEventEmitter(100)

	sub1 := newMockEventSubscriber("sub1", []EventType{EventTypeLogDebug})
	sub2 := newMockEventSubscriber("sub2", []EventType{EventTypeLogInfo})

	emitter.Subscribe(sub1)
	emitter.Subscribe(sub2)

	// Make one inactive
	sub1.SetActive(false)

	emitter.Cleanup()

	if emitter.GetSubscriberCount() != 1 {
		t.Errorf("Expected 1 active subscriber after cleanup, got %d", emitter.GetSubscriberCount())
	}
}

// TestMCPEventSubscriber tests MCPEventSubscriber functionality
func TestMCPEventSubscriber(t *testing.T) {
	t.Parallel()
	var receivedData []byte
	var writeErr error

	writeFunc := func(data []byte) error {
		receivedData = data
		return writeErr
	}

	subscriber := NewMCPEventSubscriber(
		"test-sub",
		[]EventType{EventTypeLogDebug},
		writeFunc,
		5*time.Minute,
	)

	if subscriber.ID() != "test-sub" {
		t.Errorf("Expected ID 'test-sub', got '%s'", subscriber.ID())
	}

	if !subscriber.IsActive() {
		t.Error("Expected subscriber to be active initially")
	}

	event := &Event{
		Type:    EventTypeLogDebug,
		Message: "Test message",
		Fields:  map[string]any{"key": "value"},
	}

	success := subscriber.SendEvent(event)

	if !success {
		t.Error("Expected SendEvent to succeed")
	}

	if receivedData == nil {
		t.Error("Expected data to be written")
	}

	// Verify JSON-RPC notification format
	var notification map[string]any
	if err := json.Unmarshal(receivedData, &notification); err != nil {
		t.Errorf("Failed to unmarshal notification: %v", err)
	}

	if notification["jsonrpc"] != "2.0" {
		t.Errorf("Expected jsonrpc '2.0', got '%v'", notification["jsonrpc"])
	}

	if notification[objects.FieldKeyMethod] != "notifications/event" {
		t.Errorf("Expected method 'notifications/event', got '%v'", notification[objects.FieldKeyMethod])
	}
}

// TestMCPEventSubscriber_IdleTimeout tests idle timeout functionality
func TestMCPEventSubscriber_IdleTimeout(t *testing.T) {
	t.Parallel()
	writeFunc := func(data []byte) error {
		return nil
	}

	subscriber := NewMCPEventSubscriber(
		"test-sub",
		[]EventType{EventTypeLogDebug},
		writeFunc,
		100*time.Millisecond, // Short timeout for testing
	)

	if !subscriber.IsActive() {
		t.Error("Expected subscriber to be active initially")
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	for subscriber.IsActive() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if subscriber.IsActive() {
		t.Error("Expected subscriber to be inactive after timeout")
	}

	// Try to send event - should fail
	event := &Event{
		Type:    EventTypeLogDebug,
		Message: "Test message",
	}

	if subscriber.SendEvent(event) {
		t.Error("Expected SendEvent to fail after timeout")
	}
}

// TestMCPEventSubscriber_WriteFailure tests handling of write failures
func TestMCPEventSubscriber_WriteFailure(t *testing.T) {
	t.Parallel()
	writeFunc := func(data []byte) error {
		return &testError{message: "write failed"}
	}

	subscriber := NewMCPEventSubscriber(
		"test-sub",
		[]EventType{EventTypeLogDebug},
		writeFunc,
		5*time.Minute,
	)

	event := &Event{
		Type:    EventTypeLogDebug,
		Message: "Test message",
	}

	success := subscriber.SendEvent(event)

	if success {
		t.Error("Expected SendEvent to fail when write fails")
	}

	if subscriber.IsActive() {
		t.Error("Expected subscriber to be inactive after write failure")
	}
}

// TestLoggerEventAdapter tests LoggerEventAdapter functionality
func TestLoggerEventAdapter(t *testing.T) {
	t.Parallel()
	emitter := NewEventEmitter(100)

	// Create a mock logger
	mockLogger := &mockLogger{
		debugCalls: make([]logCall, 0),
	}

	adapter := NewLoggerEventAdapter(mockLogger, emitter)

	// Subscribe to log events
	subscriber := newMockEventSubscriber("log-sub", []EventType{EventTypeLogDebug})
	emitter.Subscribe(subscriber)

	// Call Debug
	adapter.Debug("Test debug message", LogField{Key: "key", Value: "value"})

	// Verify logger was called
	if len(mockLogger.debugCalls) != 1 {
		t.Errorf("Expected 1 debug call to logger, got %d", len(mockLogger.debugCalls))
	}

	// Verify event was emitted
	events := subscriber.GetEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event emitted, got %d", len(events))
	}

	if events[0].Type != EventTypeLogDebug {
		t.Errorf("Expected event type log.debug, got %s", events[0].Type)
	}

	if events[0].Message != "Test debug message" {
		t.Errorf("Expected message 'Test debug message', got '%s'", events[0].Message)
	}

	if events[0].Fields["key"] != "value" {
		t.Errorf("Expected field key='value', got '%v'", events[0].Fields["key"])
	}
}

// TestLoggerEventAdapter_Disabled tests disabled event emission
func TestLoggerEventAdapter_Disabled(t *testing.T) {
	t.Parallel()
	emitter := NewEventEmitter(100)
	mockLogger := &mockLogger{debugCalls: make([]logCall, 0)}
	adapter := NewLoggerEventAdapter(mockLogger, emitter)

	// Disable event emission
	adapter.SetEnabled(false)

	// Subscribe to events
	subscriber := newMockEventSubscriber("log-sub", []EventType{EventTypeLogDebug})
	emitter.Subscribe(subscriber)

	// Call Debug
	adapter.Debug("Test message")

	// Verify logger was called
	if len(mockLogger.debugCalls) != 1 {
		t.Errorf("Expected logger to be called even when events disabled")
	}

	// Verify event was NOT emitted
	events := subscriber.GetEvents()
	if len(events) != 0 {
		t.Errorf("Expected 0 events when disabled, got %d", len(events))
	}
}

// TestEventEmitter_MultipleSubscribers tests multiple subscribers receiving same event
func TestEventEmitter_MultipleSubscribers(t *testing.T) {
	t.Parallel()
	emitter := NewEventEmitter(100)

	sub1 := newMockEventSubscriber("sub1", []EventType{EventTypeToolStarted})
	sub2 := newMockEventSubscriber("sub2", []EventType{EventTypeToolStarted})
	sub3 := newMockEventSubscriber("sub3", []EventType{EventTypeToolStarted})

	emitter.Subscribe(sub1)
	emitter.Subscribe(sub2)
	emitter.Subscribe(sub3)

	event := &Event{
		Type:    EventTypeToolStarted,
		Message: "Tool started",
	}

	sentCount := emitter.Emit(event)

	if sentCount != 3 {
		t.Errorf("Expected 3 events sent, got %d", sentCount)
	}

	// Verify all subscribers received the event
	for i, sub := range []*mockEventSubscriber{sub1, sub2, sub3} {
		events := sub.GetEvents()
		if len(events) != 1 {
			t.Errorf("Expected sub%d to receive 1 event, got %d", i+1, len(events))
		}
	}
}

// TestEventEmitter_ConcurrentAccess tests thread safety
func TestEventEmitter_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	emitter := NewEventEmitter(100)

	// Create multiple subscribers
	subscribers := make([]*mockEventSubscriber, 10)
	for i := 0; i < 10; i++ {
		subscribers[i] = newMockEventSubscriber(
			"sub"+string(rune(i)),
			[]EventType{EventTypeLogDebug},
		)
		emitter.Subscribe(subscribers[i])
	}

	// Concurrent emit
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("mcp_test", "concurrent event emit").StartSimple(func() {
			func(_ int) {
				defer wg.Done()
				event := &Event{
					Type:    EventTypeLogDebug,
					Message: "Concurrent message",
				}
				emitter.Emit(event)
			}(i)
		})
	}

	waitDone := make(chan struct{})
	goroutinelabels.NewGoroutine("mcp_test", "wait concurrent emits").StartSimple(func() {
		wg.Wait()
		close(waitDone)
	})
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for concurrent emits")
	}

	// Verify all subscribers received events
	for i, sub := range subscribers {
		events := sub.GetEvents()
		if len(events) != 100 {
			t.Errorf("Expected sub%d to receive 100 events, got %d", i, len(events))
		}
	}
}

// TestEventEmitter_EmptyEvent tests handling of nil/empty events
func TestEventEmitter_EmptyEvent(t *testing.T) {
	t.Parallel()
	emitter := NewEventEmitter(100)

	subscriber := newMockEventSubscriber("sub1", []EventType{EventTypeLogDebug})
	emitter.Subscribe(subscriber)

	// Emit nil event
	sentCount := emitter.Emit(nil)

	if sentCount != 0 {
		t.Errorf("Expected 0 events sent for nil event, got %d", sentCount)
	}

	events := subscriber.GetEvents()
	if len(events) != 0 {
		t.Errorf("Expected 0 events received for nil event, got %d", len(events))
	}
}

// TestEventEmitter_Timestamp tests automatic timestamp setting
func TestEventEmitter_Timestamp(t *testing.T) {
	t.Parallel()
	emitter := NewEventEmitter(100)

	subscriber := newMockEventSubscriber("sub1", []EventType{EventTypeLogDebug})
	emitter.Subscribe(subscriber)

	event := &Event{
		Type:    EventTypeLogDebug,
		Message: "Test message",
		// No timestamp set
	}

	emitter.Emit(event)

	events := subscriber.GetEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].Timestamp.IsZero() {
		t.Error("Expected timestamp to be set automatically")
	}

	if time.Since(events[0].Timestamp) > time.Second {
		t.Error("Expected timestamp to be recent")
	}
}

// Helper types for testing

type testError struct {
	message string
}

func (e *testError) Error() string {
	return e.message
}

type logCall struct {
	msg    string
	fields []LogField
}

type mockLogger struct {
	debugCalls []logCall
	mu         sync.Mutex
}

func (m *mockLogger) Debug(msg string, fields ...LogField) {
	_ = concurrency.RunInLockWithLogger(
		&m.mu, LockNameMockLoggerDebug, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			m.debugCalls = append(m.debugCalls, logCall{msg: msg, fields: fields})
			return nil
		},
	)
}

// TestMCPEventSubscriber_JSONRPCFormat tests JSON-RPC notification format
func TestMCPEventSubscriber_JSONRPCFormat(t *testing.T) {
	t.Parallel()
	var receivedData []byte

	writeFunc := func(data []byte) error {
		receivedData = data
		return nil
	}

	subscriber := NewMCPEventSubscriber(
		"test-sub",
		[]EventType{EventTypePermissionDenied},
		writeFunc,
		5*time.Minute,
	)

	event := &Event{
		Type:      EventTypePermissionDenied,
		Timestamp: time.Now(),
		Message:   "Access denied",
		Fields: map[string]any{
			"user":                "test-user",
			objects.FieldKeyField: "context",
			objects.FieldKeyKind:  "backlog_item",
			"obj_id":              "obj-123",
		},
		Severity: "warn",
	}

	subscriber.SendEvent(event)

	if receivedData == nil {
		t.Fatal("Expected notification data")
	}

	var notification map[string]any
	if err := json.Unmarshal(receivedData, &notification); err != nil {
		t.Fatalf("Failed to unmarshal notification: %v", err)
	}

	// Verify JSON-RPC structure
	if notification["jsonrpc"] != "2.0" {
		t.Errorf("Expected jsonrpc '2.0', got '%v'", notification["jsonrpc"])
	}

	if notification[objects.FieldKeyMethod] != "notifications/event" {
		t.Errorf("Expected method 'notifications/event', got '%v'", notification[objects.FieldKeyMethod])
	}

	// Verify params structure
	params, ok := notification["params"].(map[string]any)
	if !ok {
		t.Fatal("Expected params to be a map")
	}

	eventData, ok := params["event"].(map[string]any)
	if !ok {
		t.Fatal("Expected params.event to be a map")
	}

	// Verify event fields
	if eventData[objects.FieldKeyType] != string(EventTypePermissionDenied) {
		t.Errorf("Expected type 'permission.denied', got '%v'", eventData[objects.FieldKeyType])
	}

	if eventData["message"] != "Access denied" {
		t.Errorf("Expected message 'Access denied', got '%v'", eventData["message"])
	}

	if eventData[objects.FieldKeySeverity] != "warn" {
		t.Errorf("Expected severity 'warn', got '%v'", eventData[objects.FieldKeySeverity])
	}
}

// TestEventEmitter_UpdateEventTypes tests updating subscriber event types
func TestMCPEventSubscriber_UpdateEventTypes(t *testing.T) {
	t.Parallel()
	writeFunc := func(data []byte) error {
		return nil
	}

	subscriber := NewMCPEventSubscriber(
		"test-sub",
		[]EventType{EventTypeLogDebug},
		writeFunc,
		5*time.Minute,
	)

	// Verify initial event types
	types := subscriber.EventTypes()
	if len(types) != 1 || types[0] != EventTypeLogDebug {
		t.Errorf("Expected initial event type [log.debug], got %v", types)
	}

	// Update event types
	subscriber.UpdateEventTypes([]EventType{EventTypeLogInfo, EventTypeLogWarn})

	// Verify updated event types
	types = subscriber.EventTypes()
	if len(types) != 2 {
		t.Errorf("Expected 2 event types after update, got %d", len(types))
	}

	found := false
	for _, et := range types {
		if et == EventTypeLogInfo {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected EventTypeLogInfo in updated types")
	}
}

// TestEventEmitter_Deactivate tests manual deactivation
func TestMCPEventSubscriber_Deactivate(t *testing.T) {
	t.Parallel()
	writeFunc := func(data []byte) error {
		return nil
	}

	subscriber := NewMCPEventSubscriber(
		"test-sub",
		[]EventType{EventTypeLogDebug},
		writeFunc,
		5*time.Minute,
	)

	if !subscriber.IsActive() {
		t.Error("Expected subscriber to be active initially")
	}

	subscriber.Deactivate()

	if subscriber.IsActive() {
		t.Error("Expected subscriber to be inactive after Deactivate()")
	}

	// Try to send event - should fail
	event := &Event{
		Type:    EventTypeLogDebug,
		Message: "Test message",
	}

	if subscriber.SendEvent(event) {
		t.Error("Expected SendEvent to fail after deactivation")
	}
}

func TestEventEmitter_LifetimeCounters(t *testing.T) {
	t.Parallel()
	var eeNil *EventEmitter
	eNil, sNil := eeNil.GetEventEmitterStats()
	if eNil != 0 || sNil != 0 {
		t.Fatalf("expected nil stats (0, 0), got (%d, %d)", eNil, sNil)
	}

	ee := NewEventEmitter(10)
	eInit, sInit := ee.GetEventEmitterStats()
	if eInit != 0 || sInit != 0 {
		t.Fatalf("expected initial stats (0, 0), got (%d, %d)", eInit, sInit)
	}

	sub := newMockEventSubscriber("sub-1", []EventType{EventTypeLogInfo})
	ee.Subscribe(sub)

	ee.Emit(&Event{Type: EventTypeLogInfo, Message: "hello"})

	eAfter, sAfter := ee.GetEventEmitterStats()
	if eAfter != 1 || sAfter != 1 {
		t.Fatalf("expected stats (1, 1), got (%d, %d)", eAfter, sAfter)
	}
}

func TestMCPEventSubscriber_LifetimeCounters(t *testing.T) {
	t.Parallel()
	var subNil *MCPEventSubscriber
	if subNil.GetMCPEventSubscriberStats() != 0 {
		t.Fatalf("expected nil stats 0, got %d", subNil.GetMCPEventSubscriberStats())
	}

	sub := NewMCPEventSubscriber("sub-2", []EventType{EventTypeLogDebug}, func(data []byte) error { return nil }, 5*time.Minute)
	if sub.GetMCPEventSubscriberStats() != 0 {
		t.Fatalf("expected initial stats 0, got %d", sub.GetMCPEventSubscriberStats())
	}

	if !sub.SendEvent(&Event{Type: EventTypeLogDebug, Message: "test"}) {
		t.Fatalf("expected SendEvent to succeed")
	}

	if sub.GetMCPEventSubscriberStats() != 1 {
		t.Fatalf("expected stats 1, got %d", sub.GetMCPEventSubscriberStats())
	}

	sub.Deactivate()
	if sub.IsActive() {
		t.Fatalf("expected subscriber to be inactive after Deactivate()")
	}
}

// TestMCPEventSubscriber_FeedSteerToActionRequired verifies that when a feed steer event occurs,
// action.required is emitted to MCP subscribers to wake IDE/IDE seats natively without terminal paste.
func TestMCPEventSubscriber_FeedSteerToActionRequired(t *testing.T) {
	t.Parallel()
	emitter := NewEventEmitter(10)
	ideSubscriber := newMockEventSubscriber("ide-seat-14182", []EventType{EventTypeActionRequired})
	emitter.Subscribe(ideSubscriber)

	event := &Event{
		Type:      EventTypeActionRequired,
		Timestamp: time.Now().UTC(),
		Message:   "ATTN AGY-2 — steer published to feed",
		Severity:  "info",
		Priority:  "high",
		Fields: map[string]any{
			objects.FieldKeyAgentID: "peer-agent-02",
			"event_id":              "AFE-1785384731284174000-69b3953c",
			"to_agent_id":           "peer-agent-02",
		},
	}

	n := emitter.Emit(event)
	if n != 1 {
		t.Fatalf("expected 1 event delivered to IDE subscriber, got %d", n)
	}

	received := ideSubscriber.GetEvents()
	if len(received) != 1 {
		t.Fatalf("expected 1 event in subscriber buffer, got %d", len(received))
	}
	if received[0].Type != EventTypeActionRequired {
		t.Errorf("expected EventTypeActionRequired, got %s", received[0].Type)
	}
}

// TestEventEmitterAdapter_EmitsTypedEvent verifies that NewEventEmitterAdapter converts map payloads to typed *Event.
func TestEventEmitterAdapter_EmitsTypedEvent(t *testing.T) {
	t.Parallel()
	emitter := NewEventEmitter(10)
	ideSubscriber := newMockEventSubscriber("ide-seat-adapter", []EventType{EventTypeActionRequired})
	emitter.Subscribe(ideSubscriber)

	adapter := NewEventEmitterAdapter(emitter)
	n := adapter.Emit(map[string]any{
		objects.FieldKeyType: "action.required",
		"timestamp":          time.Now().UTC().Format(time.RFC3339),
		"message":            "Steer payload via adapter",
		"fields": map[string]any{
			"event_id": "AFE-adapter-1",
		},
	})
	if n != 1 {
		t.Fatalf("expected 1 event delivered via adapter, got %d", n)
	}

	received := ideSubscriber.GetEvents()
	if len(received) != 1 {
		t.Fatalf("expected 1 event in subscriber buffer, got %d", len(received))
	}
	if received[0].Message != "Steer payload via adapter" {
		t.Errorf("expected message 'Steer payload via adapter', got %q", received[0].Message)
	}
}
