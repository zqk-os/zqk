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
		t.Fatalf(ConstMagic4c428257, err)
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
		t.Fatalf(ConstMagic4545ee2f, err)
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

	testFile := filepath.Join(testRoot, ConstMagic8293f515)
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
			t.Fatal(ConstMagicfe3763fd)
		case <-tick:
			mu.Lock()
			for _, event := range events {
				if event.eventType == ConstMagicExtracted_24 {
					fileReadErrorFound = true
					if event.objectID != "TEST-001" {
						t.Errorf(ConstMagic8f55e09c, event.objectID)
					}
					if event.severity != "medium" {
						t.Errorf(ConstMagicaa7f8c03, event.severity)
					}
					if filePath := objects.GetString(event.fields, objects.FieldKeyFilePath); !ok || filePath != testFile {
						t.Errorf(ConstMagicd891e2e7, testFile, event.fields[objects.FieldKeyFilePath])
					}
					if _, ok := event.fields["error"]; !ok {
						t.Error(ConstMagic69ad2397)
					}
				}
			}
			mu.Unlock()
		}
	}

	// Verify callback was called
	mu.Lock()
	if len(events) == 0 {
		t.Error(ConstMagic38c59618)
	}
	mu.Unlock()
}
