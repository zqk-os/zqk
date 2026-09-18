package utility

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// testEventLogger is a test logger that captures log calls
type testEventLogger struct {
	mu        sync.Mutex
	infoLogs  []testLogEntry
	warnLogs  []testLogEntry
	errorLogs []testLogEntry
	debugLogs []testLogEntry
	fatalLogs []testLogEntry

	// Callbacks for async event notification
	onInfoCallback  func(testLogEntry)
	onWarnCallback  func(testLogEntry)
	onErrorCallback func(testLogEntry)
}

type testLogEntry struct {
	message string
	fields  []logging.Field
	err     error
}

// Implement logging.Logger interface
func (t *testEventLogger) Debug(msg string, fields ...logging.Field) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.debugLogs = append(t.debugLogs, testLogEntry{message: msg, fields: fields})
}

func (t *testEventLogger) Info(msg string, fields ...logging.Field) {
	t.mu.Lock()
	entry := testLogEntry{message: msg, fields: fields}
	t.infoLogs = append(t.infoLogs, entry)
	callback := t.onInfoCallback
	t.mu.Unlock()

	if callback != nil {
		callback(entry)
	}
}

func (t *testEventLogger) Warn(msg string, fields ...logging.Field) {
	t.mu.Lock()
	entry := testLogEntry{message: msg, fields: fields}
	t.warnLogs = append(t.warnLogs, entry)
	callback := t.onWarnCallback
	t.mu.Unlock()

	if callback != nil {
		callback(entry)
	}
}

func (t *testEventLogger) Error(msg string, err error, fields ...logging.Field) {
	t.mu.Lock()
	entry := testLogEntry{message: msg, err: err, fields: fields}
	t.errorLogs = append(t.errorLogs, entry)
	callback := t.onErrorCallback
	t.mu.Unlock()

	if callback != nil {
		callback(entry)
	}
}

func (t *testEventLogger) Fatal(msg string, err error, fields ...logging.Field) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.fatalLogs = append(t.fatalLogs, testLogEntry{message: msg, err: err, fields: fields})
}

func (t *testEventLogger) WithFields(fields ...logging.Field) logging.Logger {
	// Return self for simplicity in tests
	return t
}

func (t *testEventLogger) WithContext(ctx context.Context) logging.Logger {
	// Return self for simplicity in tests
	return t
}

func (t *testEventLogger) WithObjectRef(kind, id string) logging.Logger {
	// Return self for simplicity in tests
	return t
}

func (t *testEventLogger) getInfoLogs() []testLogEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]testLogEntry, len(t.infoLogs))
	copy(result, t.infoLogs)
	return result
}

func (t *testEventLogger) getWarnLogs() []testLogEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]testLogEntry, len(t.warnLogs))
	copy(result, t.warnLogs)
	return result
}

func (t *testEventLogger) getErrorLogs() []testLogEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]testLogEntry, len(t.errorLogs))
	copy(result, t.errorLogs)
	return result
}

// reset clears all log entries (thread-safe)
func (t *testEventLogger) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.infoLogs = nil
	t.warnLogs = nil
	t.errorLogs = nil
	t.debugLogs = nil
	t.fatalLogs = nil
	t.onInfoCallback = nil
	t.onWarnCallback = nil
	t.onErrorCallback = nil
}

// waitForInfoLog waits for an info log matching the predicate, with timeout
func (t *testEventLogger) waitForInfoLog(predicate func(testLogEntry) bool, timeout time.Duration) bool {
	done := make(chan bool, 1)

	// Set callback
	t.mu.Lock()
	t.onInfoCallback = func(entry testLogEntry) {
		if predicate(entry) {
			select {
			case done <- true:
			default:
			}
		}
	}
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		t.onInfoCallback = nil
		t.mu.Unlock()
	}()

	// Check existing logs first
	t.mu.Lock()
	for _, entry := range t.infoLogs {
		if predicate(entry) {
			t.mu.Unlock()
			return true
		}
	}
	t.mu.Unlock()

	// Wait for new log or timeout
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// waitForWarnLog waits for a warn log matching the predicate, with timeout
func (t *testEventLogger) waitForWarnLog(predicate func(testLogEntry) bool, timeout time.Duration) bool {
	done := make(chan bool, 1)

	// Set callback
	t.mu.Lock()
	t.onWarnCallback = func(entry testLogEntry) {
		if predicate(entry) {
			select {
			case done <- true:
			default:
			}
		}
	}
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		t.onWarnCallback = nil
		t.mu.Unlock()
	}()

	// Check existing logs first
	t.mu.Lock()
	for _, entry := range t.warnLogs {
		if predicate(entry) {
			t.mu.Unlock()
			return true
		}
	}
	t.mu.Unlock()

	// Wait for new log or timeout
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// waitForErrorLog waits for an error log matching the predicate, with timeout
func (t *testEventLogger) waitForErrorLog(predicate func(testLogEntry) bool, timeout time.Duration) bool {
	done := make(chan bool, 1)

	// Set callback
	t.mu.Lock()
	t.onErrorCallback = func(entry testLogEntry) {
		if predicate(entry) {
			select {
			case done <- true:
			default:
			}
		}
	}
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		t.onErrorCallback = nil
		t.mu.Unlock()
	}()

	// Check existing logs first
	t.mu.Lock()
	for _, entry := range t.errorLogs {
		if predicate(entry) {
			t.mu.Unlock()
			return true
		}
	}
	t.mu.Unlock()

	// Wait for new log or timeout
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// TestScenarioBuilderEventSubscriber_HandleEvent tests that the subscriber correctly handles events
func TestScenarioBuilderEventSubscriber_HandleEvent(t *testing.T) {
	t.Parallel()
	testLogger := &testEventLogger{}
	subscriber := &ScenarioBuilderEventSubscriber{
		id:      "test-subscriber",
		logger:  testLogger,
		profile: ScenarioBuilderProfileName,
	}
	subscriber.active.Store(true)

	tests := []struct {
		name           string
		event          *coordination.OperationalEvent
		expectInfoLog  bool
		expectWarnLog  bool
		expectErrorLog bool
		expectedMsg    string
	}{
		{
			name: "progress event with kind and count",
			event: &coordination.OperationalEvent{
				OperationType: ScenarioBuilderProfileName,
				Status:        scenarioBuilderStatusProgress,
				Metadata: map[string]any{
					objects.FieldKeyKind: "backlog_item",
					"count":              5,
				},
			},
			expectInfoLog: true,
			expectedMsg:   "Creating 5 backlog_item objects...",
		},
		{
			name: "progress event with message",
			event: &coordination.OperationalEvent{
				OperationType: ScenarioBuilderProfileName,
				Status:        scenarioBuilderStatusProgress,
				Metadata: map[string]any{
					scenarioBuilderMetadataEvent: "Building scenario from data file",
				},
			},
			expectInfoLog: true,
			expectedMsg:   "Building scenario from data file",
		},
		{
			name: "warning event",
			event: &coordination.OperationalEvent{
				OperationType: ScenarioBuilderProfileName,
				Status:        scenarioBuilderStatusWarning,
				Metadata: map[string]any{
					scenarioBuilderMetadataEvent: "Failed to prepare object",
					objects.FieldKeyKind:         "backlog_item",
				},
			},
			expectWarnLog: true,
			expectedMsg:   "Failed to prepare object",
		},
		{
			name: "error event",
			event: &coordination.OperationalEvent{
				OperationType: ScenarioBuilderProfileName,
				Status:        scenarioBuilderStatusError,
				Error:         "object creation failed",
				Metadata: map[string]any{
					scenarioBuilderMetadataEvent: "Failed to create object",
					objects.FieldKeyKind:         "backlog_item",
				},
			},
			expectErrorLog: true,
			expectedMsg:    "Failed to create object",
		},
		{
			name: "complete event",
			event: &coordination.OperationalEvent{
				OperationType: ScenarioBuilderProfileName,
				Status:        scenarioBuilderStatusComplete,
			},
			expectInfoLog: true,
			expectedMsg:   "Scenario builder completed",
		},
		{
			name: "non-scenario_builder event (should be ignored)",
			event: &coordination.OperationalEvent{
				OperationType: "other_operation",
				Status:        scenarioBuilderStatusProgress,
				Metadata: map[string]any{
					scenarioBuilderMetadataEvent: "Some other event",
				},
			},
			expectInfoLog:  false,
			expectWarnLog:  false,
			expectErrorLog: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset logger
			testLogger.reset()

			// Handle event (synchronous, no need to wait)
			err := subscriber.HandleEvent(tt.event)
			if err != nil {
				t.Errorf("HandleEvent() returned error: %v", err)
			}

			// Verify logs (HandleEvent is synchronous, so logs should be immediate)
			infoLogs := testLogger.getInfoLogs()
			warnLogs := testLogger.getWarnLogs()
			errorLogs := testLogger.getErrorLogs()

			if tt.expectInfoLog {
				if len(infoLogs) == 0 {
					t.Error("Expected info log but none found")
				} else if tt.expectedMsg != emptyValue {
					found := false
					for _, log := range infoLogs {
						if log.message == tt.expectedMsg {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Expected info log with message '%s', but got: %v", tt.expectedMsg, infoLogs)
					}
				}
			} else if len(infoLogs) > 0 {
				t.Errorf("Unexpected info logs: %v", infoLogs)
			}

			if tt.expectWarnLog {
				if len(warnLogs) == 0 {
					t.Error("Expected warn log but none found")
				} else if tt.expectedMsg != emptyValue {
					found := false
					for _, log := range warnLogs {
						if log.message == tt.expectedMsg {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Expected warn log with message '%s', but got: %v", tt.expectedMsg, warnLogs)
					}
				}
			} else if len(warnLogs) > 0 {
				t.Errorf("Unexpected warn logs: %v", warnLogs)
			}

			if tt.expectErrorLog {
				if len(errorLogs) == 0 {
					t.Error("Expected error log but none found")
				} else if tt.expectedMsg != emptyValue {
					found := false
					for _, log := range errorLogs {
						if log.message == tt.expectedMsg {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Expected error log with message '%s', but got: %v", tt.expectedMsg, errorLogs)
					}
				}
			} else if len(errorLogs) > 0 {
				t.Errorf("Unexpected error logs: %v", errorLogs)
			}
		})
	}
}

// TestScenarioBuilderEventSubscriber_Integration tests that events flow through coordinator to subscriber
func TestScenarioBuilderEventSubscriber_Integration(t *testing.T) {
	t.Parallel()
	// Create coordinator
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Create test logger
	testLogger := &testEventLogger{}

	// Create subscriber with test logger
	subscriber := &ScenarioBuilderEventSubscriber{
		id:      "test-subscriber",
		logger:  testLogger,
		profile: ScenarioBuilderProfileName,
	}
	subscriber.active.Store(true)

	// Subscribe to coordinator
	coordinator.Subscribe(subscriber)

	// Create scenario builder with coordinator
	sb := &ScenarioBuilder{
		coordinator: coordinator,
		logger:      logging.NewEventLogger(pkgctx.NewSystemContext()),
	}

	// Test 1: Emit progress event
	sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		"Creating 3 backlog_item objects...",
		map[string]any{
			objects.FieldKeyKind: "backlog_item",
			"count":              3,
		})

	// Wait for async event processing using callback
	if !testLogger.waitForInfoLog(func(entry testLogEntry) bool {
		return entry.message == "Creating 3 backlog_item objects..."
	}, 2*time.Second) {
		infoLogs := testLogger.getInfoLogs()
		t.Error("Subscriber did not receive progress event")
		t.Errorf("Expected progress log not found. Got: %v", infoLogs)
		return
	}

	// Verify subscriber received and logged the event
	infoLogs := testLogger.getInfoLogs()
	if len(infoLogs) == 0 {
		t.Error("Subscriber did not receive progress event")
	} else {
		found := false
		for _, log := range infoLogs {
			if log.message == "Creating 3 backlog_item objects..." {
				found = true
				// Verify fields
				hasKind := false
				hasCount := false
				for _, field := range log.fields {
					if field.Key == "kind" && field.Value == "backlog_item" {
						hasKind = true
					}
					if field.Key == "count" && field.Value == 3 {
						hasCount = true
					}
				}
				if !hasKind {
					t.Error("Progress log missing 'kind' field")
				}
				if !hasCount {
					t.Error("Progress log missing 'count' field")
				}
				break
			}
		}
		if !found {
			t.Errorf("Expected progress log not found. Got: %v", infoLogs)
		}
	}

	// Reset logger
	testLogger.reset()

	// Test 2: Emit warning event
	sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
		"Failed to prepare object",
		map[string]any{
			objects.FieldKeyKind: "backlog_item",
			"index":              0,
			"error":              fmt.Errorf("validation failed"),
		})

	// Wait for "Failed to prepare object" warning using callback
	if !testLogger.waitForWarnLog(func(entry testLogEntry) bool {
		return entry.message == "Failed to prepare object"
	}, 2*time.Second) {
		warnLogs := testLogger.getWarnLogs()
		t.Errorf("Expected warning log 'Failed to prepare object' not found. Got: %v", warnLogs)
	}

	// Reset logger
	testLogger.reset()

	// Test 3: Emit error event
	sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusError,
		"Failed to create object",
		map[string]any{
			objects.FieldKeyKind: "backlog_item",
			"index":              0,
			"error":              fmt.Errorf("storage error"),
		})

	// Wait for "Failed to create object" error using callback
	if !testLogger.waitForErrorLog(func(entry testLogEntry) bool {
		return entry.message == "Failed to create object" && entry.err != nil
	}, 2*time.Second) {
		errorLogs := testLogger.getErrorLogs()
		t.Errorf("Expected error log 'Failed to create object' not found. Got: %v", errorLogs)
	}

	// Reset logger
	testLogger.reset()

	// Test 4: Emit complete event
	sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusComplete,
		"Scenario build complete: 10 total objects",
		map[string]any{
			"total_objects": 10,
			"total_kinds":   3,
		})

	// Wait for "Scenario builder completed" event using callback
	if !testLogger.waitForInfoLog(func(entry testLogEntry) bool {
		return entry.message == "Scenario builder completed"
	}, 2*time.Second) {
		infoLogs := testLogger.getInfoLogs()
		t.Errorf("Expected complete log 'Scenario builder completed' not found. Got: %v", infoLogs)
	}
}

// TestScenarioBuilderEventSubscriber_NonScenarioBuilderEvents tests that non-scenario_builder events are ignored
func TestScenarioBuilderEventSubscriber_NonScenarioBuilderEvents(t *testing.T) {
	t.Parallel()
	testLogger := &testEventLogger{}
	subscriber := &ScenarioBuilderEventSubscriber{
		id:      "test-subscriber",
		logger:  testLogger,
		profile: ScenarioBuilderProfileName,
	}
	subscriber.active.Store(true)

	// Create coordinator and subscribe
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	coordinator.Subscribe(subscriber)

	// Emit non-scenario_builder event
	eventCtx := coordination.NewEventContext("test-op-1", "other_operation", scenarioBuilderStatusProgress).
		WithEventData(&coordination.EventData{
			LoggingFields: []coordination.LoggingField{
				{Key: scenarioBuilderMetadataEvent, Value: "Some other operation"},
			},
		}).
		WithChannels(false, false, false, true) // Only operational

	err := coordinator.Emit(pkgctx.NewSystemContext(), eventCtx)
	if err != nil {
		t.Errorf("Emit() returned error: %v", err)
	}

	// Wait for async event processing
	time.Sleep(200 * time.Millisecond)

	// Verify subscriber ignored the event (no logs)
	infoLogs := testLogger.getInfoLogs()
	warnLogs := testLogger.getWarnLogs()
	errorLogs := testLogger.getErrorLogs()

	if len(infoLogs) > 0 || len(warnLogs) > 0 || len(errorLogs) > 0 {
		t.Errorf("Subscriber should ignore non-scenario_builder events, but got logs: info=%v, warn=%v, error=%v",
			infoLogs, warnLogs, errorLogs)
	}
}

// TestScenarioBuilderEventSubscriber_ProfileLoading tests that the subscriber loads the scenario_builder profile
func TestScenarioBuilderEventSubscriber_ProfileLoading(t *testing.T) {
	// Test with different profiles
	profiles := []string{string(pkgctx.ProfileHuman), ScenarioBuilderProfileName, string(pkgctx.ProfileAIAgent), string(pkgctx.ProfileDebug)}

	for _, profile := range profiles {
		t.Run(profile, func(t *testing.T) {
			subscriber := NewScenarioBuilderEventSubscriber(profile)
			if subscriber == nil {
				t.Fatal("NewScenarioBuilderEventSubscriber returned nil")
			}
			if subscriber.ID() != "scenario-builder-event-subscriber" {
				t.Errorf("Expected ID 'scenario-builder-event-subscriber', got '%s'", subscriber.ID())
			}
			if !subscriber.IsActive() {
				t.Error("Subscriber should be active by default")
			}
			if subscriber.profile != profile {
				t.Errorf("Expected profile '%s', got '%s'", profile, subscriber.profile)
			}
		})
	}
}

// TestScenarioBuilder_emitCoordinatorEvent tests that emitCoordinatorEvent routes events correctly
func TestScenarioBuilder_emitCoordinatorEvent(t *testing.T) {
	t.Parallel()
	// Create coordinator
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Create test logger
	testLogger := &testEventLogger{}

	// Create subscriber
	subscriber := &ScenarioBuilderEventSubscriber{
		id:      "test-subscriber",
		logger:  testLogger,
		profile: ScenarioBuilderProfileName,
	}
	subscriber.active.Store(true)

	// Subscribe
	coordinator.Subscribe(subscriber)

	// Create scenario builder with coordinator
	sb := &ScenarioBuilder{
		coordinator: coordinator,
		logger:      logging.NewEventLogger(pkgctx.NewSystemContext()),
	}

	tests := []struct {
		name           string
		operationType  string
		status         string
		message        string
		fields         map[string]any
		expectInfoLog  bool
		expectWarnLog  bool
		expectErrorLog bool
	}{
		{
			name:          "progress event",
			operationType: ScenarioBuilderProfileName,
			status:        scenarioBuilderStatusProgress,
			message:       "Test progress message",
			fields:        map[string]any{"count": 5},
			expectInfoLog: true,
		},
		{
			name:          "warning event",
			operationType: ScenarioBuilderProfileName,
			status:        scenarioBuilderStatusWarning,
			message:       "Test warning message",
			fields:        map[string]any{"error": fmt.Errorf("test error")},
			expectWarnLog: true,
		},
		{
			name:           "error event",
			operationType:  ScenarioBuilderProfileName,
			status:         scenarioBuilderStatusError,
			message:        "Test error message",
			fields:         map[string]any{"error": fmt.Errorf("test error")},
			expectErrorLog: true,
		},
		{
			name:          "complete event",
			operationType: ScenarioBuilderProfileName,
			status:        scenarioBuilderStatusComplete,
			message:       "Test complete message",
			fields:        nil,
			expectInfoLog: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset logger
			testLogger.reset()

			// Emit event
			sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), tt.operationType, tt.status, tt.message, tt.fields)

			// Wait for expected logs using callbacks
			if tt.expectInfoLog {
				if !testLogger.waitForInfoLog(func(entry testLogEntry) bool {
					return entry.message == tt.message || entry.message != emptyValue
				}, 2*time.Second) {
					infoLogs := testLogger.getInfoLogs()
					t.Error("Expected info log but none found")
					t.Errorf("Expected message containing '%s', got: %v", tt.message, infoLogs)
				}
			} else {
				// Wait a short time to ensure no info logs appear
				time.Sleep(100 * time.Millisecond)
				infoLogs := testLogger.getInfoLogs()
				if len(infoLogs) > 0 {
					t.Errorf("Unexpected info logs: %v", infoLogs)
				}
			}

			if tt.expectWarnLog {
				if !testLogger.waitForWarnLog(func(entry testLogEntry) bool {
					return entry.message == tt.message || entry.message != emptyValue
				}, 2*time.Second) {
					warnLogs := testLogger.getWarnLogs()
					t.Error("Expected warn log but none found")
					t.Errorf("Expected message containing '%s', got: %v", tt.message, warnLogs)
				}
			} else {
				// Wait a short time to ensure no warn logs appear
				time.Sleep(100 * time.Millisecond)
				warnLogs := testLogger.getWarnLogs()
				if len(warnLogs) > 0 {
					t.Errorf("Unexpected warn logs: %v", warnLogs)
				}
			}

			if tt.expectErrorLog {
				if !testLogger.waitForErrorLog(func(entry testLogEntry) bool {
					return entry.message == tt.message || entry.message != emptyValue
				}, 2*time.Second) {
					errorLogs := testLogger.getErrorLogs()
					t.Error("Expected error log but none found")
					t.Errorf("Expected message containing '%s', got: %v", tt.message, errorLogs)
				}
			} else {
				// Wait a short time to ensure no error logs appear
				time.Sleep(100 * time.Millisecond)
				errorLogs := testLogger.getErrorLogs()
				if len(errorLogs) > 0 {
					t.Errorf("Unexpected error logs: %v", errorLogs)
				}
			}
		})
	}
}

// TestScenarioBuilder_emitCoordinatorEvent_RequiredCoordinator tests that coordinator is required
func TestScenarioBuilder_emitCoordinatorEvent_RequiredCoordinator(t *testing.T) {
	t.Parallel()
	// Create scenario builder without coordinator - should panic
	// Coordinator is now required, so creating without one should panic
	sb := &ScenarioBuilder{
		coordinator: nil,
		logger:      logging.NewEventLogger(pkgctx.NewSystemContext()),
	}

	// Emit event should panic since coordinator is nil
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when coordinator is nil, but no panic occurred")
		} else if !strings.Contains(fmt.Sprintf("%v", r), "coordinator is required") {
			t.Errorf("Expected panic about coordinator being required, got: %v", r)
		}
	}()

	sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusProgress, "Test message", nil)
	t.Error("Should have panicked before reaching this point")
}
