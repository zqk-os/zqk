package logging

import (
	"bytes"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestLogRouter_MultipleDestinations tests routing to multiple destinations
func TestLogRouter_MultipleDestinations(t *testing.T) {
	t.Parallel()
	var stdoutBuf, stderrBuf bytes.Buffer

	router := NewLogRouter()
	router.AddDestination("stdout", &stdoutBuf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	router.AddDestination("stderr", &stderrBuf, ErrorLevel, NewTextFormatter(pkgctx.NewSystemContext()))

	logger := router.GetLogger()

	logger.Info("info message")
	logger.Error("error message", nil)

	// Info should go to stdout
	if !strings.Contains(stdoutBuf.String(), "info message") {
		t.Error("Expected info message in stdout")
	}

	// Error should go to both stdout and stderr
	if !strings.Contains(stdoutBuf.String(), "error message") {
		t.Error("Expected error message in stdout")
	}
	if !strings.Contains(stderrBuf.String(), "error message") {
		t.Error("Expected error message in stderr")
	}
}

// TestLogRouter_LevelFiltering tests that destinations respect log levels
func TestLogRouter_LevelFiltering(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer

	router := NewLogRouter()
	router.AddDestination("output", &buf, WarnLevel, NewTextFormatter(pkgctx.NewSystemContext())) // Only WARN and above

	logger := router.GetLogger()

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message", nil)

	output := buf.String()

	// Debug and Info should not appear
	if strings.Contains(output, "debug message") {
		t.Error("Debug message should not appear at WarnLevel")
	}
	if strings.Contains(output, "info message") {
		t.Error("Info message should not appear at WarnLevel")
	}

	// Warn and Error should appear
	if !strings.Contains(output, "warn message") {
		t.Error("Warn message should appear")
	}
	if !strings.Contains(output, "error message") {
		t.Error("Error message should appear")
	}
}

// TestLogRouter_DifferentFormatters tests different formatters per destination
func TestLogRouter_DifferentFormatters(t *testing.T) {
	t.Parallel()
	var jsonBuf, textBuf bytes.Buffer

	router := NewLogRouter()
	router.AddDestination("json", &jsonBuf, InfoLevel, NewJSONFormatter(pkgctx.NewSystemContext()))
	router.AddDestination("text", &textBuf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))

	logger := router.GetLogger()
	logger.Info("test message", String("key", "value"))

	// JSON destination should have JSON output
	jsonOutput := jsonBuf.String()
	if !strings.Contains(jsonOutput, "\"message\":\"test message\"") {
		t.Error("Expected JSON format in json destination")
	}

	// Text destination should have text output
	textOutput := textBuf.String()
	if !strings.Contains(textOutput, "[info]") || !strings.Contains(textOutput, "test message") {
		t.Error("Expected text format in text destination")
	}
}

// TestLogRouter_FileDestination tests file destination
func TestLogRouter_FileDestination(t *testing.T) {
	t.Parallel()
	tmpFile, err := fileutil.CreateTemp("", "test-log-*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer fileutil.Remove(tmpFile.Name())
	tmpFile.Close()

	router := NewLogRouter()
	err = router.AddFileDestination("file", tmpFile.Name(), InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	if err != nil {
		t.Fatalf("Failed to add file destination: %v", err)
	}

	logger := router.GetLogger()
	logger.Info("file message")

	// Close router to flush file
	router.Close()

	// Read file and verify
	data, err := fileutil.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(data), "file message") {
		t.Error("Expected message in log file")
	}
}

// TestLogRouter_StdoutStderr tests standard output destinations
func TestLogRouter_StdoutStderr(t *testing.T) {
	t.Parallel()
	router := NewLogRouter()
	router.AddStdoutDestination(InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	router.AddStderrDestination(ErrorLevel, NewTextFormatter(pkgctx.NewSystemContext()))

	logger := router.GetLogger()

	// Info should go to stdout
	logger.Info("stdout message")

	// Error should go to stderr
	logger.Error("stderr message", nil)

	// Router should work (we can't easily test stdout/stderr in unit tests,
	// but we can verify the router doesn't crash)
	router.Close()
}

// TestLogRouter_RemoveDestination tests removing destinations
func TestLogRouter_RemoveDestination(t *testing.T) {
	t.Parallel()
	var buf1, buf2 bytes.Buffer

	router := NewLogRouter()
	router.AddDestination("dest1", &buf1, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	router.AddDestination("dest2", &buf2, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))

	logger := router.GetLogger()
	logger.Info("message 1")

	// Remove dest1
	router.RemoveDestination("dest1")

	logger.Info("message 2")

	// dest1 should have message 1 but not message 2
	if !strings.Contains(buf1.String(), "message 1") {
		t.Error("Expected message 1 in dest1")
	}
	if strings.Contains(buf1.String(), "message 2") {
		t.Error("Expected message 2 NOT in dest1 after removal")
	}

	// dest2 should have both messages
	if !strings.Contains(buf2.String(), "message 1") {
		t.Error("Expected message 1 in dest2")
	}
	if !strings.Contains(buf2.String(), "message 2") {
		t.Error("Expected message 2 in dest2")
	}
}
