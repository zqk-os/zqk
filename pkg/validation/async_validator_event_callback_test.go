package validation

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestAsyncValidator_EventCallback tests that event callbacks are invoked correctly
func TestAsyncValidator_EventCallback(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 1, time.Hour)

	// Track events received via callback
	var mu sync.Mutex
	events := make([]eventRecord, 0)

	callback := func(eventType, objectID, message string, fields map[string]any, severity string) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, eventRecord{
			eventType: eventType,
			objectID:  objectID,
			message:   message,
			fields:    fields,
			severity:  severity,
		})
	}

	validator.SetEventCallback(callback)

	if err := validator.Start(); err != nil {
		t.Fatalf("failed to start validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup

	// Test 1: File read error should trigger callback
	testFile := filepath.Join(testRoot, "nonexistent.yaml")
	_ = validator.Enqueue("TEST-001", "test_object", testFile, 1)

	// Wait for validation to complete (file read error should be immediate)
	timeout := time.After(5 * time.Second)
	tick := time.Tick(100 * time.Millisecond)
	fileReadErrorFound := false
	for !fileReadErrorFound {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for file read error event")
		case <-tick:
			mu.Lock()
			for _, event := range events {
				if event.eventType == "file_read_error" {
					fileReadErrorFound = true
					if event.objectID != "TEST-001" {
						t.Errorf("Expected objectID TEST-001, got %s", event.objectID)
					}
					if event.severity != "medium" {
						t.Errorf("Expected severity medium, got %s", event.severity)
					}
					if filePath := objects.GetString(event.fields, objects.FieldKeyFilePath); filePath != testFile {
						t.Errorf("Expected file_path %s, got %v", testFile, event.fields[objects.FieldKeyFilePath])
					}
					if _, ok := event.fields["error"]; !ok {
						t.Error("Expected error field in event fields")
					}
				}
			}
			mu.Unlock()
		}
	}

	// Test 2: Semaphore full warning (if we can trigger it)
	// This is harder to test deterministically, but we can verify the callback is set
	mu.Lock()
	initialEventCount := len(events)
	mu.Unlock()

	// Verify callback was called
	if initialEventCount == 0 {
		t.Error("Expected at least one event callback to be invoked")
	}

	// Verify event structure
	mu.Lock()
	fileReadEvent := events[0]
	mu.Unlock()

	if fileReadEvent.eventType != "file_read_error" {
		t.Errorf("Expected eventType file_read_error, got %s", fileReadEvent.eventType)
	}
	if fileReadEvent.message == emptyValue {
		t.Error("Expected non-empty message in event")
	}
}

// TestAsyncValidator_EventCallbackNil tests that nil callback doesn't cause panics
func TestAsyncValidator_EventCallbackNil(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 1, time.Hour)

	// Set nil callback - should not panic
	validator.SetEventCallback(nil)

	if err := validator.Start(); err != nil {
		t.Fatalf("failed to start validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup

	// Enqueue a task that will fail (file read error)
	testFile := filepath.Join(testRoot, "nonexistent.yaml")
	_ = validator.Enqueue("TEST-001", "test_object", testFile, 1)

	// Wait a bit - should not panic even with nil callback
	time.Sleep(500 * time.Millisecond)
}

// TestAsyncValidator_EventCallbackConcurrent tests that callback is thread-safe
func TestAsyncValidator_EventCallbackConcurrent(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 4, time.Hour)

	// Track events with concurrent-safe counter
	var mu sync.Mutex
	eventCount := 0

	callback := func(eventType, objectID, message string, fields map[string]any, severity string) {
		mu.Lock()
		eventCount++
		mu.Unlock()
	}

	validator.SetEventCallback(callback)

	if err := validator.Start(); err != nil {
		t.Fatalf("failed to start validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup

	// Enqueue multiple tasks concurrently
	for i := 0; i < 10; i++ {
		testFile := filepath.Join(testRoot, "nonexistent.yaml")
		_ = validator.Enqueue("TEST-"+string(rune(i)), "test_object", testFile, 1)
	}

	// Wait for all validations to complete
	timeout := time.After(5 * time.Second)
	tick := time.Tick(100 * time.Millisecond)
	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for events")
		case <-tick:
			mu.Lock()
			count := eventCount
			mu.Unlock()
			if count >= 10 {
				// All events received
				return
			}
		}
	}
}

// TestAsyncValidator_SetEventCallback tests that SetEventCallback updates the callback
func TestAsyncValidator_SetEventCallback(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 1, time.Hour)

	// Set first callback
	var mu sync.Mutex
	firstCallbackCalled := false
	firstCallback := func(eventType, objectID, message string, fields map[string]any, severity string) {
		mu.Lock()
		firstCallbackCalled = true
		mu.Unlock()
	}

	validator.SetEventCallback(firstCallback)

	// Replace with second callback
	secondCallbackCalled := false
	secondCallback := func(eventType, objectID, message string, fields map[string]any, severity string) {
		mu.Lock()
		secondCallbackCalled = true
		mu.Unlock()
	}

	validator.SetEventCallback(secondCallback)

	if err := validator.Start(); err != nil {
		t.Fatalf("failed to start validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup

	// Trigger an event
	testFile := filepath.Join(testRoot, "nonexistent.yaml")
	_ = validator.Enqueue("TEST-001", "test_object", testFile, 1)

	// Wait for event
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if firstCallbackCalled {
		t.Error("First callback should not have been called after replacement")
	}
	if !secondCallbackCalled {
		t.Error("Second callback should have been called")
	}
}

// eventRecord represents a recorded event for testing
type eventRecord struct {
	eventType string
	objectID  string
	message   string
	fields    map[string]any
	severity  string
}
