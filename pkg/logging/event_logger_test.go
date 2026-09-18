package logging

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

func TestEventLogger_LogSpecLoad(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, DebugLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	ctx := pkgctx.NewSystemContext()
	eventLogger := &EventLogger{logger: logger, ctx: ctx}

	// Test successful load
	eventLogger.LogSpecLoad("specs/test.yaml", nil)
	output := buf.String()
	if !contains(output, "spec_load") {
		t.Error("Expected 'spec_load' event in output")
	}
	if !contains(output, "test.yaml") {
		t.Error("Expected spec file name in output")
	}

	buf.Reset()

	// Test failed load
	err := fmt.Errorf("test error")
	eventLogger.LogSpecLoad("specs/test.yaml", err)
	output = buf.String()
	if !contains(output, "Failed to load spec") {
		t.Error("Expected error message in output")
	}
}

func TestEventLogger_LogSpecValidation(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, DebugLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	ctx := pkgctx.NewSystemContext()
	eventLogger := &EventLogger{logger: logger, ctx: ctx}

	// Test validation with errors
	errors := []ValidationError{
		{Field: "field1", MissingItem: "purpose", CriteriaRef: "CRIT-123"},
		{Field: "field2", MissingItem: "system_usage", CriteriaRef: "CRIT-124"},
	}
	eventLogger.LogSpecValidation("specs/test.yaml", errors)
	output := buf.String()
	if !contains(output, "spec_validation") {
		t.Error("Expected 'spec_validation' event in output")
	}
	if !contains(output, "error_count") {
		t.Error("Expected error count in output")
	}

	buf.Reset()

	// Test validation without errors
	eventLogger.LogSpecValidation("specs/test.yaml", nil)
	output = buf.String()
	if !contains(output, "Spec validation passed") {
		t.Error("Expected validation passed message")
	}
}

func TestEventLogger_LogCheckResult(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, DebugLevel, NewJSONFormatter(pkgctx.NewSystemContext()))
	ctx := pkgctx.NewSystemContext()
	eventLogger := &EventLogger{logger: logger, ctx: ctx}

	tierCounts := map[int]int{
		1: 2,  // 2 blocking issues
		2: 5,  // 5 warnings
		3: 10, // 10 informational
	}

	eventLogger.LogCheckResult("backlog_item", 20, 17, tierCounts)
	output := buf.String()
	if !contains(output, "check_result") {
		t.Error("Expected 'check_result' event in output")
	}
	if !contains(output, "backlog_item") {
		t.Error("Expected kind in output")
	}
	if !contains(output, "object_count") {
		t.Error("Expected object_count in output")
	}
}

func TestEventLogger_LogObjectCheck(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, DebugLevel, NewJSONFormatter(pkgctx.NewSystemContext()))
	ctx := pkgctx.NewSystemContext()
	eventLogger := &EventLogger{logger: logger, ctx: ctx}

	issues := []CheckIssue{
		{Tier: 1, Category: "registration", Message: "Missing id"},
		{Tier: 2, Category: "lifecycle", Message: "Invalid status"},
	}

	eventLogger.LogObjectCheck("BLI-123", "backlog_item", issues)
	output := buf.String()
	if !contains(output, "object_check") {
		t.Error("Expected 'object_check' event in output")
	}
	if !contains(output, "BLI-123") {
		t.Error("Expected object ID in output")
	}
}

func TestEventLogger_LogFileOperation(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, DebugLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	ctx := pkgctx.NewSystemContext()
	eventLogger := &EventLogger{logger: logger, ctx: ctx}

	eventLogger.LogFileOperation("read", "/path/to/file.yaml", nil)
	output := buf.String()
	if !contains(output, "file_operation") {
		t.Error("Expected 'file_operation' event in output")
	}
	if !contains(output, "read") {
		t.Error("Expected operation type in output")
	}
	if !contains(output, "file.yaml") {
		t.Error("Expected file name in output")
	}
}

func TestEventLogger_LogHashOperation(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, DebugLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	ctx := pkgctx.NewSystemContext()
	eventLogger := &EventLogger{logger: logger, ctx: ctx}

	hash := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	eventLogger.LogHashOperation("verify", "backlog_item", "BLI-123.yaml", hash, nil)
	output := buf.String()
	if !contains(output, "hash_operation") {
		t.Error("Expected 'hash_operation' event in output")
	}
	if !contains(output, "verify") {
		t.Error("Expected operation type in output")
	}
	if !contains(output, "backlog_item") {
		t.Error("Expected kind in output")
	}
}

func TestEventLogger_WithObjectRef(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, DebugLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	ctx := pkgctx.NewSystemContext()
	eventLogger := &EventLogger{logger: logger, ctx: ctx}

	eventLogger = eventLogger.WithObjectRef("backlog_item", "BLI-123")
	eventLogger.LogInfo("Test message")
	output := buf.String()
	if !contains(output, "BLI-123") {
		t.Error("Expected object reference in output")
	}
}

func TestNewEventLogger(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(pkgctx.NewSystemContext(), contextKey("profile"), "ai-agent")
	eventLogger := NewEventLogger(ctx)
	if eventLogger == nil {
		t.Error("Expected non-nil EventLogger")
		return
	}
	if eventLogger.ctx != ctx {
		t.Error("Expected context to be set")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || substr == emptyValue ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsMiddle(s, substr))))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
