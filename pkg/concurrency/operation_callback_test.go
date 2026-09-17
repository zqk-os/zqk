package concurrency

import (
	"maps"
	"sync"
	"testing"
	"time"
)

// RecordingOperationCallback is a test helper that records all callback invocations
// This allows tests to verify that callbacks are called correctly without requiring
// coordinator or storage dependencies (avoiding import cycles)
type RecordingOperationCallback struct {
	mu sync.RWMutex

	// Recorded events
	StartEvents    []StartEvent
	ProgressEvents []ProgressEvent
	CompleteEvents []CompleteEvent
	ErrorEvents    []ErrorEvent
	CancelEvents   []CancelEvent
}

// StartEvent records an OnStart invocation
type StartEvent struct {
	OperationID string
	Metadata    map[string]any
}

// ProgressEvent records an OnProgress invocation
type ProgressEvent struct {
	OperationID string
	Progress    int
	Total       int
	Message     string
}

// CompleteEvent records an OnComplete invocation
type CompleteEvent struct {
	OperationID string
	Result      any
	Duration    time.Duration
}

// ErrorEvent records an OnError invocation
type ErrorEvent struct {
	OperationID string
	Error       error
}

// CancelEvent records an OnCancel invocation
type CancelEvent struct {
	OperationID string
	Reason      string
}

// NewRecordingOperationCallback creates a new recording callback for tests
func NewRecordingOperationCallback() *RecordingOperationCallback {
	return &RecordingOperationCallback{
		StartEvents:    make([]StartEvent, 0),
		ProgressEvents: make([]ProgressEvent, 0),
		CompleteEvents: make([]CompleteEvent, 0),
		ErrorEvents:    make([]ErrorEvent, 0),
		CancelEvents:   make([]CancelEvent, 0),
	}
}

// OnStart records the start event
func (r *RecordingOperationCallback) OnStart(operationID string, metadata map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.StartEvents = append(r.StartEvents, StartEvent{
		OperationID: operationID,
		Metadata:    copyMetadata(metadata),
	})
}

// OnProgress records the progress event
func (r *RecordingOperationCallback) OnProgress(operationID string, progress int, total int, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ProgressEvents = append(r.ProgressEvents, ProgressEvent{
		OperationID: operationID,
		Progress:    progress,
		Total:       total,
		Message:     message,
	})
}

// OnComplete records the complete event
func (r *RecordingOperationCallback) OnComplete(operationID string, result any, duration time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.CompleteEvents = append(r.CompleteEvents, CompleteEvent{
		OperationID: operationID,
		Result:      result,
		Duration:    duration,
	})
}

// OnError records the error event
func (r *RecordingOperationCallback) OnError(operationID string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ErrorEvents = append(r.ErrorEvents, ErrorEvent{
		OperationID: operationID,
		Error:       err,
	})
}

// OnCancel records the cancel event
func (r *RecordingOperationCallback) OnCancel(operationID string, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.CancelEvents = append(r.CancelEvents, CancelEvent{
		OperationID: operationID,
		Reason:      reason,
	})
}

// GetStartEvents returns all recorded start events (thread-safe)
func (r *RecordingOperationCallback) GetStartEvents() []StartEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]StartEvent, len(r.StartEvents))
	copy(result, r.StartEvents)
	return result
}

// GetProgressEvents returns all recorded progress events (thread-safe)
func (r *RecordingOperationCallback) GetProgressEvents() []ProgressEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]ProgressEvent, len(r.ProgressEvents))
	copy(result, r.ProgressEvents)
	return result
}

// GetCompleteEvents returns all recorded complete events (thread-safe)
func (r *RecordingOperationCallback) GetCompleteEvents() []CompleteEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]CompleteEvent, len(r.CompleteEvents))
	copy(result, r.CompleteEvents)
	return result
}

// GetErrorEvents returns all recorded error events (thread-safe)
func (r *RecordingOperationCallback) GetErrorEvents() []ErrorEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]ErrorEvent, len(r.ErrorEvents))
	copy(result, r.ErrorEvents)
	return result
}

// GetCancelEvents returns all recorded cancel events (thread-safe)
func (r *RecordingOperationCallback) GetCancelEvents() []CancelEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]CancelEvent, len(r.CancelEvents))
	copy(result, r.CancelEvents)
	return result
}

// Reset clears all recorded events
func (r *RecordingOperationCallback) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.StartEvents = r.StartEvents[:0]
	r.ProgressEvents = r.ProgressEvents[:0]
	r.CompleteEvents = r.CompleteEvents[:0]
	r.ErrorEvents = r.ErrorEvents[:0]
	r.CancelEvents = r.CancelEvents[:0]
}

// AssertStartEventCount is a test helper that asserts the number of start events
func (r *RecordingOperationCallback) AssertStartEventCount(t *testing.T, expected int) {
	t.Helper()
	events := r.GetStartEvents()
	if len(events) != expected {
		t.Errorf("Expected %d start events, got %d", expected, len(events))
	}
}

// AssertProgressEventCount is a test helper that asserts the number of progress events
func (r *RecordingOperationCallback) AssertProgressEventCount(t *testing.T, expected int) {
	t.Helper()
	events := r.GetProgressEvents()
	if len(events) != expected {
		t.Errorf("Expected %d progress events, got %d", expected, len(events))
	}
}

// AssertCompleteEventCount is a test helper that asserts the number of complete events
func (r *RecordingOperationCallback) AssertCompleteEventCount(t *testing.T, expected int) {
	t.Helper()
	events := r.GetCompleteEvents()
	if len(events) != expected {
		t.Errorf("Expected %d complete events, got %d", expected, len(events))
	}
}

// AssertErrorEventCount is a test helper that asserts the number of error events
func (r *RecordingOperationCallback) AssertErrorEventCount(t *testing.T, expected int) {
	t.Helper()
	events := r.GetErrorEvents()
	if len(events) != expected {
		t.Errorf("Expected %d error events, got %d", expected, len(events))
	}
}

// AssertCancelEventCount is a test helper that asserts the number of cancel events
func (r *RecordingOperationCallback) AssertCancelEventCount(t *testing.T, expected int) {
	t.Helper()
	events := r.GetCancelEvents()
	if len(events) != expected {
		t.Errorf("Expected %d cancel events, got %d", expected, len(events))
	}
}

// copyMetadata creates a deep copy of metadata map to prevent test races
func copyMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	result := make(map[string]any, len(metadata))
	maps.Copy(result, metadata)
	return result
}
