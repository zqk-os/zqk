package logging

import (
	"context"
	"io"
	"os"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// progressLogger implements ProgressLogger interface
type progressLogger struct {
	logger            Logger
	progressFormatter ProgressFormatter
	writer            io.Writer
}

// LoggerWriter is an interface that allows extracting the writer from a logger
// This is used to get the writer from the underlying logger implementation
type LoggerWriter interface {
	GetWriter() io.Writer
}

// NewProgressLogger creates a new progress logger
// The progress logger uses a separate formatter for progress output
// and writes to the same writer as the underlying logger
func NewProgressLogger(logger Logger, progressFormatter ProgressFormatter) ProgressLogger {
	// Try to extract writer from logger
	var writer io.Writer = os.Stderr // Default to stderr for progress

	// Try to get writer from logger if it implements LoggerWriter
	if lw, ok := logger.(LoggerWriter); ok {
		writer = lw.GetWriter()
	}

	return &progressLogger{
		logger:            logger,
		progressFormatter: progressFormatter,
		writer:            writer,
	}
}

// NewProgressLoggerWithWriter creates a new progress logger with an explicit writer
// This is useful for testing or when you want to specify a custom writer
func NewProgressLoggerWithWriter(logger Logger, progressFormatter ProgressFormatter, writer io.Writer) ProgressLogger {
	return &progressLogger{
		logger:            logger,
		progressFormatter: progressFormatter,
		writer:            writer,
	}
}

// Progress logs a progress update
func (pl *progressLogger) Progress(operationID string, progress int, message string, fields ...Field) {
	// Convert fields to map for formatter
	fieldsMap := make(map[string]any)
	for _, field := range fields {
		fieldsMap[field.Key] = field.Value
	}

	// Format progress update
	formatted, err := pl.progressFormatter.FormatProgress(operationID, progress, message, fieldsMap)
	if err != nil {
		// Fallback to simple log if formatting fails
		Fluent(pl.logger).Warn("Failed to format progress update").
			String("operation_id", operationID).
			String("error", err.Error()).
			Log()
		return
	}

	// Write progress output
	// If formatter supports overwrite, use \r for line overwriting
	if pl.progressFormatter.SupportsOverwrite() {
		// Use \r to overwrite the same line
		pl.writeWithOverwrite(formatted)
	} else {
		// Write as new line (for JSON, each update is separate)
		pl.write(formatted)
	}
}

// Status logs a status change
func (pl *progressLogger) Status(operationID, oldStatus, newStatus string, fields ...Field) {
	// Convert fields to map for formatter
	fieldsMap := make(map[string]any)
	for _, field := range fields {
		fieldsMap[field.Key] = field.Value
	}

	// Format status change
	formatted, err := pl.progressFormatter.FormatStatus(operationID, oldStatus, newStatus, fieldsMap)
	if err != nil {
		// Fallback to simple log if formatting fails
		Fluent(pl.logger).Warn("Failed to format status update").
			String("operation_id", operationID).
			String("error", err.Error()).
			Log()
		return
	}

	// Status changes are always written as new lines (not overwritten)
	pl.write(formatted)
}

// Stream streams a real-time event
func (pl *progressLogger) Stream(ctx context.Context, eventType string, data any, fields ...Field) error {
	// Convert fields to map for formatter
	fieldsMap := make(map[string]any)
	for _, field := range fields {
		fieldsMap[field.Key] = field.Value
	}

	// Format stream event
	formatted, err := pl.progressFormatter.FormatStreamEvent(eventType, data, fieldsMap)
	if err != nil {
		return errfmt.Newf("failed to format stream event").Wrap(err)
	}

	// Stream events are always written as new lines
	pl.write(formatted)
	return nil
}

// write writes formatted output to the writer
func (pl *progressLogger) write(data []byte) {
	// CRITICAL: Check if MCP server is serving or we're in MCP mode
	// In MCP mode, stdout is reserved for JSON-RPC protocol
	// Progress output should go to stderr in MCP mode
	// Use the isMCPMode function from context_logger.go
	if isMCPServerServing() || (zqkenv.MCPAccountID().Get() != emptyValue) {
		// In MCP mode, always use stderr for progress (even if writer is stdout)
		if pl.writer == os.Stdout {
			_ , _ = os.Stderr.Write(data)
			return
		}
	}

	// Normal mode: use configured writer
	//nolint:errcheck // Progress output errors are non-critical
	_, _ = pl.writer.Write(data)
}

// writeWithOverwrite writes formatted output with \r for line overwriting
func (pl *progressLogger) writeWithOverwrite(data []byte) {
	// Prepend \r to overwrite the current line
	overwriteData := append([]byte("\r"), data...)
	pl.write(overwriteData)
}
