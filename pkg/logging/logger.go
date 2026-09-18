package logging

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"reflect"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/when"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const emptyValue = ""

// isMCPServerServing checks if the MCP server is actively serving requests
// Uses the MCPServerContext object following the context pattern
func isMCPServerServing() bool {
	return pkgctx.GetMCPServerContext().IsServing()
}

// isMCPSubprocessMode checks if we're running in MCP subprocess mode
// MCP subprocess mode is detected when ZQK_MCP_ACCOUNT_ID is set
func isMCPSubprocessMode() bool {
	return zqkenv.MCPAccountID().Get() != emptyValue
}

// shouldSuppressStdioForMCPProtocol determines if stdio output should be suppressed
// to protect the JSON-RPC protocol stream. Returns true if:
// - MCP server is actively serving (stdout reserved for JSON-RPC)
// - Running in MCP subprocess mode (stdout reserved for JSON-RPC responses)
// File destinations are never suppressed - only stdio is protected.
func shouldSuppressStdioForMCPProtocol(writer io.Writer) bool {
	if !isStdio(writer) {
		return false // File destinations are never suppressed
	}
	return isMCPServerServing() || isMCPSubprocessMode()
}

// selectWriterForLogLevel selects the appropriate writer for a given log level,
// ensuring stdout is never polluted with debug logs, errors, or MCP protocol violations.
func selectWriterForLogLevel(level LogLevel, writer io.Writer) io.Writer {
	if shouldSuppressStdioForMCPProtocol(writer) {
		return nil // Signal to suppress (caller should return early)
	}
	// Redirect to stderr when destination is stdout and this is diagnostic/protocol-sensitive output
	redirectToStderr := level == DebugLevel || level == ErrorLevel || level == WarnLevel || isMCPSubprocessMode()
	if isStdout(writer) && redirectToStderr {
		return os.Stderr
	}
	return writer
}

// LogLevel represents the severity of a log entry
type LogLevel int

const (
	DebugLevel LogLevel = iota
	InfoLevel
	WarnLevel
	ErrorLevel
	FatalLevel
)

func (l LogLevel) String() string {
	switch l {
	case DebugLevel:
		return "debug"
	case InfoLevel:
		return "info"
	case WarnLevel:
		return "warn"
	case ErrorLevel:
		return "error"
	case FatalLevel:
		return "fatal"
	default:
		return "unknown"
	}
}

// ParseLevel parses a log level string ("debug", "info", "warn", "error", "fatal") to LogLevel.
// Case-insensitive. Returns (level, true) on success, or (InfoLevel, false) for unknown/empty.
func ParseLevel(s string) (LogLevel, bool) {
	switch {
	case s == emptyValue:
		return InfoLevel, false
	case strings.EqualFold(s, "debug"):
		return DebugLevel, true
	case strings.EqualFold(s, "info"):
		return InfoLevel, true
	case strings.EqualFold(s, "warn"), strings.EqualFold(s, "warning"):
		return WarnLevel, true
	case strings.EqualFold(s, "error"):
		return ErrorLevel, true
	case strings.EqualFold(s, "fatal"):
		return FatalLevel, true
	default:
		return InfoLevel, false
	}
}

// Field represents a structured log field
type Field struct {
	Key   string
	Value any
}

// Field helpers
func String(key, value string) Field {
	return Field{Key: key, Value: value}
}

func Int(key string, value int) Field {
	return Field{Key: key, Value: value}
}

func Bool(key string, value bool) Field {
	return Field{Key: key, Value: value}
}

func Error(err error) Field {
	return Field{Key: "error", Value: err.Error()}
}

// ErrField returns a slice of one error field for structured logs. Returns nil if err is nil.
// Use as logging.ErrField(err)... in Warn/Error log calls for consistent error logging.
func ErrField(err error) []Field {
	if err == nil {
		return nil
	}
	return []Field{Error(err)}
}

func ObjectRef(kind, id string) Field {
	return Field{Key: "object_ref", Value: map[string]string{
		objectFieldKeyKind: kind,
		objectFieldKeyID:   id,
	}}
}

func Timestamp(t time.Time) Field {
	return Field{Key: "timestamp", Value: zqktime.FormatRFC3339UTC(t)}
}

// Formatter formats log entries for output
type Formatter interface {
	Format(entry *LogEntry) ([]byte, error)
}

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time
	Level     LogLevel
	Message   string
	Fields    map[string]any
	Error     error
	Context   context.Context
}

// Logger provides structured logging
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, err error, fields ...Field)
	Fatal(msg string, err error, fields ...Field)

	WithFields(fields ...Field) Logger
	WithContext(ctx context.Context) Logger
	WithObjectRef(kind, id string) Logger
}

// destinationCloser matches loggers that own OS resources (e.g. decisionContextLogger file sinks).
type destinationCloser interface {
	Close() error
}

// TryCloseLoggerDestinations closes file-backed destinations when logger implements Close.
// Use during shutdown (e.g. AsyncValidator stop) so component log files do not leak FDs.
// No-op for loggers that do not implement Close.
func TryCloseLoggerDestinations(logger Logger) error {
	if logger == nil {
		return nil
	}
	if c, ok := logger.(destinationCloser); ok {
		return c.Close()
	}
	return nil
}

// ProgressLogger provides progress, status, and streaming output
// This is separate from Logger to maintain clear separation between logs and interactive output
type ProgressLogger interface {
	// Progress logs a progress update
	Progress(operationID string, progress int, message string, fields ...Field)

	// Status logs a status change
	Status(operationID string, oldStatus, newStatus string, fields ...Field)

	// Stream streams a real-time event
	Stream(ctx context.Context, eventType string, data any, fields ...Field) error
}

// logger is the default implementation of Logger
type logger struct {
	writer    io.Writer
	level     LogLevel
	formatter Formatter
	fields    map[string]any
	ctx       context.Context
}

// GetWriter returns the writer used by this logger
// This implements LoggerWriter interface for progress logger integration
func (l *logger) GetWriter() io.Writer {
	return l.writer
}

// NewLogger creates a new logger
// For file writers, consider using NewBufferedLogger for better performance
func NewLogger(writer io.Writer, level LogLevel, formatter Formatter) Logger {
	return &logger{
		writer:    writer,
		level:     level,
		formatter: formatter,
		fields:    make(map[string]any),
		ctx:       pkgctx.NewSystemContext(),
	}
}

// NewBufferedLogger creates a new logger with buffered output
// This minimizes I/O context switches by batching writes
func NewBufferedLogger(writer io.Writer, level LogLevel, formatter Formatter) Logger {
	// Only buffer file writers, not stdio
	var bufferedWriter = writer
	if !isStdout(writer) && !isStderr(writer) {
		config := DefaultBufferedWriterConfig()
		bufferedWriter = NewBufferedWriter(writer, config)
	}

	return &logger{
		writer:    bufferedWriter,
		level:     level,
		formatter: formatter,
		fields:    make(map[string]any),
		ctx:       pkgctx.NewSystemContext(),
	}
}

func (l *logger) log(level LogLevel, msg string, err error, fields ...Field) {
	if level < l.level {
		return // Skip if below threshold
	}

	// Merge fields
	entryFields := make(map[string]any)
	maps.Copy(entryFields, l.fields)
	for _, field := range fields {
		entryFields[field.Key] = field.Value
	}

	// Add error if present
	if err != nil {
		entryFields["error"] = err.Error()
	}

	entry := &LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     level,
		Message:   msg,
		Fields:    entryFields,
		Error:     err,
		Context:   l.ctx,
	}

	formatted, err := l.formatter.Format(entry)
	if err != nil {
		// Fallback to simple output if formatter fails
		// CRITICAL: If MCP server is actively serving, suppress fallback output to stdout/stderr only
		// File destinations should still receive fallback output to preserve debugging context
		// IDE may merge stderr into stdout for stdio-based MCP, so even stderr output
		// would pollute the JSON-RPC protocol stream. We must suppress stdio output.
		if isMCPServerServing() && isStdio(l.writer) {
			// MCP server is actively serving and writer is stdout/stderr - suppress fallback output
			// The JSON-RPC protocol stream on stdout must remain completely clean
			// File destinations will still receive formatted output (if formatter succeeds)
			return
		}

		// For subprocesses, use stderr for fallback (subprocess stderr is captured separately)
		// CRITICAL: ALWAYS use stderr for fallback to prevent stdout pollution
		// This is the last line of defense - if JSON formatting fails, we must not pollute stdout
		//
		// ALWAYS use stderr for fallback - never write fallback messages to stdout
		// This ensures that even if the logger was incorrectly configured, fallback messages
		// won't pollute stdout (which is reserved for JSON-RPC protocol in MCP mode)
		//
		// Additional safety: If the logger's writer is stdout, we MUST use stderr for fallback
		// This prevents any possibility of fallback messages appearing on stdout
		var fallbackWriter io.Writer = os.Stderr
		when.When(func() bool { return !isStdout(l.writer) && !isStderr(l.writer) }).Then(func() {
			fallbackWriter = l.writer
		}).Run()

		// Include the formatting error in the fallback message for debugging
		// This helps identify what's causing JSON formatting failures
		// Note: Using fmt.Sprintf + Write instead of fmt.Fprintf to comply with POL-CODE-007
		// This is an emergency fallback when the logging framework itself fails
		fallbackMsg := fmt.Sprintf("[%s] %s: %s (formatting error: %v)\n", level.String(), zqktime.NowRFC3339UTC(), msg, err)
		_, _ = fallbackWriter.Write([]byte(fallbackMsg))
		return
	}

	// Select appropriate writer with automatic MCP protocol protection
	writer := selectWriterForLogLevel(level, l.writer)
	if writer == nil {
		return // Suppressed to protect MCP protocol stream
	}

	// Write to buffer if it's a buffered writer, otherwise direct write
	// Buffered writers will handle level-aware flushing automatically
	when.When(func() bool { _, ok := writer.(*BufferedWriter); return ok }).Then(func() {
		buffered := writer.(*BufferedWriter)
		//nolint:errcheck // Intentional error ignored
		_, _ = buffered.WriteWithLevel(formatted, level)
	}).OrElse(func() {
		//nolint:errcheck // Intentional error ignored
		_, _ = writer.Write(formatted)
	}).Run()
}

// isStdout checks if a writer is os.Stdout
// Uses reflection to safely check if the writer is the same as os.Stdout
func isStdout(w io.Writer) bool {
	if w == nil {
		return false
	}
	// Use pointer comparison for os.File types
	if f, ok := w.(*fileutil.File); ok {
		return f == os.Stdout
	}
	// For other types, use reflect to check if it's the same value
	// But first check if the value is addressable to avoid panic
	v := reflect.ValueOf(w)
	if v.Kind() == reflect.Ptr {
		// Pointer type - safe to call Pointer()
		return v.Pointer() == reflect.ValueOf(os.Stdout).Pointer()
	}
	if v.CanAddr() {
		// Can get address - safe to call Pointer()
		return v.Addr().Pointer() == reflect.ValueOf(os.Stdout).Pointer()
	}
	// Non-addressable value (e.g., struct value, interface value)
	// Cannot safely compare pointers, so assume it's not stdout
	return false
}

// isStderr checks if a writer is os.Stderr
// Uses reflection to safely check if the writer is the same as os.Stderr
func isStderr(w io.Writer) bool {
	if w == nil {
		return false
	}
	// Use pointer comparison for os.File types
	if f, ok := w.(*fileutil.File); ok {
		return f == os.Stderr
	}
	// For other types, use reflect to check if it's the same value
	// But first check if the value is addressable to avoid panic
	v := reflect.ValueOf(w)
	if v.Kind() == reflect.Ptr {
		// Pointer type - safe to call Pointer()
		return v.Pointer() == reflect.ValueOf(os.Stderr).Pointer()
	}
	if v.CanAddr() {
		// Can get address - safe to call Pointer()
		return v.Addr().Pointer() == reflect.ValueOf(os.Stderr).Pointer()
	}
	// Non-addressable value (e.g., struct value, interface value)
	// Cannot safely compare pointers, so assume it's not stderr
	return false
}

// isStdio checks if a writer is stdout or stderr
// This is used to determine if output should be suppressed during MCP serving
// File destinations are not stdio and should continue to receive logs
func isStdio(w io.Writer) bool {
	return isStdout(w) || isStderr(w)
}

func (l *logger) Debug(msg string, fields ...Field) {
	l.log(DebugLevel, msg, nil, fields...)
}

func (l *logger) Info(msg string, fields ...Field) {
	l.log(InfoLevel, msg, nil, fields...)
}

func (l *logger) Warn(msg string, fields ...Field) {
	l.log(WarnLevel, msg, nil, fields...)
}

func (l *logger) Error(msg string, err error, fields ...Field) {
	l.log(ErrorLevel, msg, err, fields...)
}

func (l *logger) Fatal(msg string, err error, fields ...Field) {
	l.log(FatalLevel, msg, err, fields...)
	// In a real implementation, this would call os.Exit(1)
	// TRACK: [Fatal logging error]
	panic(fmt.Sprintf("FATAL: %s: %v", msg, err))
}

func (l *logger) WithFields(fields ...Field) Logger {
	newFields := make(map[string]any, len(l.fields)+len(fields))
	maps.Copy(newFields, l.fields)
	for _, field := range fields {
		newFields[field.Key] = field.Value
	}
	return &logger{
		writer:    l.writer,
		level:     l.level,
		formatter: l.formatter,
		fields:    newFields,
		ctx:       l.ctx,
	}
}

func (l *logger) WithContext(ctx context.Context) Logger {
	return &logger{
		writer:    l.writer,
		level:     l.level,
		formatter: l.formatter,
		fields:    l.fields,
		ctx:       ctx,
	}
}

func (l *logger) WithObjectRef(kind, id string) Logger {
	return l.WithFields(ObjectRef(kind, id))
}
