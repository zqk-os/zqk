package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

func TestTryCloseLoggerDestinations_Nil(t *testing.T) {
	t.Parallel()
	if err := TryCloseLoggerDestinations(nil); err != nil {
		t.Fatal(err)
	}
}

func TestTryCloseLoggerDestinations_DefaultLogger_NoCloseMethod(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := NewLogger(&buf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	if err := TryCloseLoggerDestinations(l); err != nil {
		t.Fatal(err)
	}
}

// TestLogger_LogLevels tests that all log levels work correctly
func TestLogger_LogLevels(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message", nil)

	output := buf.String()

	// Debug should not appear (level is Info)
	if strings.Contains(output, "debug message") {
		t.Error("Debug message should not appear at Info level")
	}

	// Info, Warn, Error should appear
	if !strings.Contains(output, "info message") {
		t.Error("Info message should appear")
	}
	if !strings.Contains(output, "warn message") {
		t.Error("Warn message should appear")
	}
	if !strings.Contains(output, "error message") {
		t.Error("Error message should appear")
	}
}

// TestLogger_WithFields tests structured field logging
func TestLogger_WithFields(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, DebugLevel, NewTextFormatter(pkgctx.NewSystemContext()))

	logger.WithFields(
		String("key1", "value1"),
		Int("key2", 42),
	).Info("test message")

	output := buf.String()
	if !strings.Contains(output, "key1=value1") {
		t.Error("Expected field key1=value1 in output")
	}
	if !strings.Contains(output, "key2=42") {
		t.Error("Expected field key2=42 in output")
	}
}

// TestLogger_JSONOutput tests JSON formatter
func TestLogger_JSONOutput(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, InfoLevel, NewJSONFormatter(pkgctx.NewSystemContext()))

	logger.WithFields(
		String("key", "value"),
		Int("number", 42),
	).Info("test message")

	output := buf.String()
	var logEntry map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if logEntry["level"] != "info" {
		t.Errorf("Expected level=info, got %v", logEntry["level"])
	}
	if logEntry["message"] != "test message" {
		t.Errorf("Expected message='test message', got %v", logEntry["message"])
	}
	if logEntry["key"] != "value" {
		t.Errorf("Expected key='value', got %v", logEntry["key"])
	}
}

// TestLogger_ContextIntegration tests context integration
func TestLogger_ContextIntegration(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))

	ctx := context.WithValue(pkgctx.NewSystemContext(), contextKey("command"), "test")
	logger.WithContext(ctx).Info("test message")

	output := buf.String()
	// Context should be included in output
	if !strings.Contains(output, "test message") {
		t.Error("Expected message in output")
	}
}

// TestLogger_ObjectRef tests object reference logging
func TestLogger_ObjectRef(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, InfoLevel, NewJSONFormatter(pkgctx.NewSystemContext()))

	logger.WithObjectRef("backlog_item", "ITEM-123").Info("test message")

	output := buf.String()
	var logEntry map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	objRef, ok := logEntry[objectFieldKeyObjectRef].(map[string]any)
	if !ok {
		t.Fatal("Expected object_ref field in log entry")
	}
	if objRef[objectFieldKeyKind] != "backlog_item" {
		t.Errorf("Expected kind=backlog_item, got %v", objRef[objectFieldKeyKind])
	}
	if objRef[objectFieldKeyID] != "ITEM-123" {
		t.Errorf("Expected id=ITEM-123, got %v", objRef[objectFieldKeyID])
	}
}

// TestLogger_ErrorLogging tests error logging
func TestLogger_ErrorLogging(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, ErrorLevel, NewTextFormatter(pkgctx.NewSystemContext()))

	err := &TestError{Message: "test error"}
	logger.Error("operation failed", err)

	output := buf.String()
	if !strings.Contains(output, "operation failed") {
		t.Error("Expected error message in output")
	}
	if !strings.Contains(output, "test error") {
		t.Error("Expected error details in output")
	}
}

type TestError struct {
	Message string
}

func (e *TestError) Error() string {
	return e.Message
}
