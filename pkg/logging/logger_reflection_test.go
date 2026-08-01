package logging

import (
	"bytes"
	"os"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// TestLogger_ReflectionSafety tests that isStdout and isStderr don't panic
// on non-addressable values (like struct values or interface values)
func TestLogger_ReflectionSafety(t *testing.T) {
	t.Parallel()
	// Test with bytes.Buffer (struct value, not pointer)
	var buf bytes.Buffer
	logger := NewLogger(&buf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))

	// This should not panic - bytes.Buffer is a struct value
	// The isStdout check should handle it safely
	logger.Info("test message")

	// Verify output was written
	if buf.Len() == 0 {
		t.Error("Expected output to be written")
	}

	// Test with os.Stdout (should return true)
	if !isStdout(os.Stdout) {
		t.Error("Expected os.Stdout to be detected as stdout")
	}

	// Test with bytes.Buffer (should return false, not panic)
	if isStdout(&buf) {
		t.Error("Expected bytes.Buffer not to be detected as stdout")
	}

	// Test with os.Stderr (should return true)
	if !isStderr(os.Stderr) {
		t.Error("Expected os.Stderr to be detected as stderr")
	}

	// Test with bytes.Buffer (should return false, not panic)
	if isStderr(&buf) {
		t.Error("Expected bytes.Buffer not to be detected as stderr")
	}
}

// TestLogger_NonAddressableWriter tests that logging works with non-addressable writers
// This specifically tests the reflection safety fix
func TestLogger_NonAddressableWriter(t *testing.T) {
	t.Parallel()
	// Create a logger with a bytes.Buffer (struct value)
	var buf bytes.Buffer
	logger := NewLogger(&buf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))

	// These calls should not panic even though bytes.Buffer is a struct value
	// The isStdout/isStderr functions should handle it safely
	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message", nil)

	// Verify output was written
	if buf.Len() == 0 {
		t.Error("Expected output to be written")
	}
}
