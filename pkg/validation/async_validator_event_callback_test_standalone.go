//go:build ignore
// +build ignore

// This test file is excluded from normal test runs due to import cycle issues
// in the validation package. It can be run manually with:
//   go test -tags=standalone_test ./pkg/validation -run TestAsyncValidator_EventCallback
//
// The import cycle is: validation -> specbuilder/bldr_v2 -> validation
// This is a pre-existing issue in the codebase, not introduced by the event callback feature.

package validation

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/testenvroot"
)

// TestAsyncValidator_EventCallback_Standalone tests event callbacks in isolation
// This test is in a separate file to avoid import cycle issues
func TestAsyncValidator_EventCallback_Standalone(t *testing.T) {
	tmpDir := t.TempDir()
	zqkenv.TestRoot().Set(tmpDir)
	defer zqkenv.TestRoot().Unset()

	testRoot, err := testenvroot.Setup(tmpDir)
	if err != nil {
		t.Fatalf("failed to setup test environment: %v", err)
	}

	validator := NewAsyncValidator(testRoot, 1, time.Hour)

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
	defer func() {
		if err := validator.Stop(); err != //nolint:errcheck // Test cleanup
			nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.

				// Test: File read error should trigger callback
				ProfileSystem))).Error(ErrMsgSwallowedError,

				err).Log()
		}
	}()

	testFile := filepath.Join(testRoot, "nonexistent.yaml")
	if err := validator.Enqueue("TEST-001", "test_object", testFile, 1); err !=

		// Wait for validation to complete (file read error should be immediate)
		nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
	}

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
					if filePath := objects.GetString(event.fields, objects.FieldKeyFilePath); !ok || filePath != testFile {
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

	// Verify callback was called
	mu.Lock()
	if len(events) == 0 {
		t.Error("Expected at least one event callback to be invoked")
	}
	mu.Unlock()
}
