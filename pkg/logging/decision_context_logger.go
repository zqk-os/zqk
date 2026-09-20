package logging

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"sync"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// decisionContextLogger implements Logger and uses LoggingDecisionContext
// to make context-aware decisions about routing, filtering, and destinations
type decisionContextLogger struct {
	router      *LogRouter
	decisionCtx *pkgctx.LoggingDecisionContext
	loggingCtx  *pkgctx.LoggingContext
	projectRoot string

	// Cache of dynamically created destinations
	destCache map[string]*destination
	destMu    sync.RWMutex
}

func (dcl *decisionContextLogger) log(level LogLevel, msg string, err error, fields ...Field) {
	// Extract component and operation from fields
	component := ""
	operation := ""
	for _, field := range fields {
		if field.Key == DecisionLogFieldKeyComponent {
			if str, ok := field.Value.(string); ok {
				component = str
			}
		}
		if field.Key == DecisionLogFieldKeyOperation {
			if str, ok := field.Value.(string); ok {
				operation = str
			}
		}
	}

	// Use component from decision context if not in fields
	if component == emptyValue && dcl.decisionCtx != nil {
		component = dcl.decisionCtx.Component
	}
	if operation == emptyValue && dcl.decisionCtx != nil {
		operation = dcl.decisionCtx.Operation
	}

	// Check if we should log this entry
	if dcl.decisionCtx != nil {
		if !dcl.decisionCtx.ShouldLog(int(level), component, operation) {
			return // Filtered out by decision context
		}
	}

	// Get destinations from decision context
	var destinations map[string]*pkgctx.LogDestination
	if dcl.decisionCtx != nil {
		destinations = dcl.decisionCtx.GetDestinations(int(level), dcl.projectRoot)
	}

	// Get existing router destinations
	var routerDests []*destination
	_ = concurrency.WithRLockCtx(
		&dcl.router.mu,
		pkgctx.NewSystemContext(),
		"decision_context_logger_get_router_destinations",
		func() error {
			routerDests = make([]*destination, 0, len(dcl.router.destinations))
			for _, dest := range dcl.router.destinations {
				routerDests = append(routerDests, dest)
			}
			return nil
		},
	)

	// Create entry
	entryFields := make(map[string]any)
	for _, field := range fields {
		entryFields[field.Key] = field.Value
	}
	if err != nil {
		entryFields["error"] = err.Error()
	}

	entry := &LogEntry{
		Timestamp: getCurrentTime(),
		Level:     level,
		Message:   msg,
		Fields:    entryFields,
		Error:     err,
		Context:   pkgctx.NewSystemContext(),
	}

	// Route to decision context destinations
	for name, destConfig := range destinations {
		// Get or create destination
		dest := dcl.getOrCreateDestination(name, destConfig)
		if dest == nil {
			continue
		}

		// Check level
		if level < dest.level {
			continue
		}

		// Format and write
		dcl.writeToDestination(dest, entry)
	}

	// Also route to existing router destinations (for backward compatibility)
	for _, dest := range routerDests {
		if level < dest.level {
			continue
		}

		// Apply decision context filtering
		if dcl.decisionCtx != nil {
			// Skip stdio destinations if MCP server is serving
			if dcl.decisionCtx.MCPCtx != nil && dcl.decisionCtx.MCPCtx.IsServing() {
				if isStdout(dest.writer) || isStderr(dest.writer) {
					continue
				}
			}

			// Skip debug logs to stdout if suppressed
			if level == DebugLevel && isStdout(dest.writer) {
				if dcl.loggingCtx != nil {
					if dcl.loggingCtx.ShouldSuppressDebugToStdout() {
						continue
					}
				}
			}
		}

		dcl.writeToDestination(dest, entry)
	}
}

// destinationCacheKey deduplicates open file writers: two logical destination names
// that resolve to the same FilePath share one cached *destination.
func destinationCacheKey(name string, destConfig *pkgctx.LogDestination) string {
	if destConfig != nil && destConfig.FilePath != emptyValue {
		return "file:" + filepath.Clean(destConfig.FilePath)
	}
	return name
}

func (dcl *decisionContextLogger) getOrCreateDestination(name string, destConfig *pkgctx.LogDestination) *destination {
	cacheKey := destinationCacheKey(name, destConfig)
	// Check cache first
	var cached *destination
	var exists bool
	_ = concurrency.WithRLockCtx(
		&dcl.destMu,
		pkgctx.NewSystemContext(),
		"decision_logger_get_dest_check",
		func() error {
			var ok bool
			cached, ok = dcl.destCache[cacheKey]
			exists = ok
			return nil
		},
	)

	if exists {
		return cached
	}

	// Create new destination (I/O outside lock)
	var dest *destination
	// Determine writer based on destination type
	if destConfig.FilePath != emptyValue {
		// File destination - create file and destination with rolling policy
		var writer io.Writer
		var closer io.Closer

		// Get rolling policy from destination config (if available)
		var policy *RollingPolicy
		if destConfig.RollingPolicy != nil {
			// Convert from pkg/context.RollingPolicy to pkg/logging.RollingPolicy
			policy = &RollingPolicy{
				Strategy:    destConfig.RollingPolicy.Strategy,
				MaxSize:     destConfig.RollingPolicy.MaxSize,
				MaxFiles:    destConfig.RollingPolicy.MaxFiles,
				MaxAge:      destConfig.RollingPolicy.MaxAge,
				RotateEvery: destConfig.RollingPolicy.RotateEvery,
			}
		} else {
			// Use default rolling policy for file destinations
			defaultPolicy := DefaultRollingPolicy()
			policy = &defaultPolicy
		}

		// Create rolling writer
		factory := NewRollingWriterFactory()
		rollingWriter, err := factory.CreateWriter(destConfig.FilePath, *policy)
		if err != nil {
			return nil
		}

		// Wrap with buffered writer to minimize I/O context switches
		bufferedConfig := DefaultBufferedWriterConfig()
		bufferedWriter := NewBufferedWriter(rollingWriter, bufferedConfig)

		writer = bufferedWriter
		closer = rollingWriter // Close the underlying rolling writer, buffered writer will flush

		// Determine formatter based on config
		var formatter Formatter
		// Get context from decision context for formatter (use operation context if available)
		ctx := pkgctx.NewSystemContext()
		if dcl.decisionCtx != nil && dcl.decisionCtx.OperationCtx != nil {
			ctx = dcl.decisionCtx.OperationCtx
		}
		switch destConfig.Formatter {
		case "json":
			formatter = NewJSONFormatter(ctx)
		case "text":
			formatter = NewTextFormatter(ctx)
		case "compact":
			formatter = NewCompactFormatter(ctx)
		default:
			formatter = NewJSONFormatter(ctx) // Default to JSON for file destinations
		}

		dest = &destination{
			name:         name,
			writer:       writer,
			buffered:     bufferedWriter,
			level:        LogLevel(destConfig.Level),
			formatter:    formatter,
			closer:       closer,
			writeMu:      sync.Mutex{},
			useBuffering: true, // File destinations use buffering
		}
	} else if destConfig.Writer != emptyValue {
		// Stdio destination
		var writer io.Writer
		switch destConfig.Writer {
		case "stdout":
			writer = os.Stdout
		case "stderr":
			writer = os.Stderr
		default:
			return nil
		}

		// Determine formatter
		// Get context from decision context for formatter (use operation context if available)
		ctx := pkgctx.NewSystemContext()
		if dcl.decisionCtx != nil && dcl.decisionCtx.OperationCtx != nil {
			ctx = dcl.decisionCtx.OperationCtx
		}
		var formatter Formatter
		switch destConfig.Formatter {
		case "json":
			formatter = NewJSONFormatter(ctx)
		case "text":
			formatter = NewTextFormatter(ctx)
		case "compact":
			formatter = NewCompactFormatter(ctx)
		default:
			formatter = NewTextFormatter(ctx) // Default to text for stdio
		}

		dest = &destination{
			name:      name,
			writer:    writer,
			level:     LogLevel(destConfig.Level),
			formatter: formatter,
		}
	} else {
		// No destination configured (or MCP channel destination which is handled separately)
		return nil
	}

	// Copy-in: Update cache with lock
	var cachedAfterCreate *destination
	var existsAfterCreate bool
	_ = concurrency.WithLockCtx(
		&dcl.destMu,
		pkgctx.NewSystemContext(),
		"decision_logger_get_dest_cache",
		func() error {
			// Double-check cache after acquiring write lock
			var ok bool
			cachedAfterCreate, ok = dcl.destCache[cacheKey]
			existsAfterCreate = ok
			if !existsAfterCreate {
				// Initialize cache if needed
				if dcl.destCache == nil {
					dcl.destCache = make(map[string]*destination)
				}
				// Cache the destination
				dcl.destCache[cacheKey] = dest
			}
			return nil
		},
	)

	if existsAfterCreate {
		// Another goroutine created it - close our writer if it's a file
		if dest != nil && dest.closer != nil {
			dest.closer.Close() //nolint:gosec
		}
		return cachedAfterCreate
	}

	return dest
}

func (dcl *decisionContextLogger) writeToDestination(dest *destination, entry *LogEntry) {
	// Format entry
	formatted, formatErr := dest.formatter.Format(entry)
	if formatErr != nil {
		// Fallback to simple text format
		timestamp := zqktime.FormatRFC3339UTC(entry.Timestamp)
		formatted = []byte(fmt.Sprintf("[%s] %s: %s\n", timestamp, entry.Level.String(), entry.Message))
	}

	// For file destinations, add newline for JSONL format (one JSON object per line)
	// This matches the behavior in router.go for consistent JSONL output
	if !isStdout(dest.writer) && !isStderr(dest.writer) {
		if len(formatted) > 0 && formatted[len(formatted)-1] != '\n' {
			// Formatter doesn't include newline (e.g., JSONFormatter) - add one for JSONL
			formatted = append(formatted, '\n')
		}
	}

	// For file destinations, add newline for JSONL format (one JSON object per line)
	// This makes the file easier to parse and ensures each entry is on its own line
	// However, if the formatter already added a newline (e.g., TextFormatter), don't add another
	var toWrite []byte
	if !isStdout(dest.writer) && !isStderr(dest.writer) {
		// File destination - ensure newline for JSONL format
		if len(formatted) > 0 && formatted[len(formatted)-1] == '\n' {
			// Formatter already includes newline - use as-is
			toWrite = formatted
		} else {
			// Formatter doesn't include newline (e.g., JSONFormatter) - add one
			toWrite = make([]byte, 0, len(formatted)+1)
			toWrite = append(toWrite, formatted...)
			toWrite = append(toWrite, '\n')
		}
	} else {
		// Stdout/stderr - write without newline (may be part of structured output)
		toWrite = formatted
	}

	// Write to destination with level-aware buffering
	if dest.buffered != nil {
		// Use level-aware write for immediate flush on error/fatal
		//nolint:errcheck // Intentional error ignored
		_, _ = dest.buffered.WriteWithLevel(toWrite, entry.Level)
	} else {
		// Direct write for non-buffered destinations (stdio, etc.)
		//nolint:errcheck // Intentional error ignored
		_, _ = dest.writer.Write(toWrite)
	}
}

func (dcl *decisionContextLogger) Debug(msg string, fields ...Field) {
	dcl.log(DebugLevel, msg, nil, fields...)
}

func (dcl *decisionContextLogger) Info(msg string, fields ...Field) {
	dcl.log(InfoLevel, msg, nil, fields...)
}

func (dcl *decisionContextLogger) Warn(msg string, fields ...Field) {
	dcl.log(WarnLevel, msg, nil, fields...)
}

func (dcl *decisionContextLogger) Error(msg string, err error, fields ...Field) {
	dcl.log(ErrorLevel, msg, err, fields...)
}

func (dcl *decisionContextLogger) Fatal(msg string, err error, fields ...Field) {
	dcl.log(FatalLevel, msg, err, fields...)
	// TRACK: [Fatal logging error]
	panic(fmt.Sprintf("FATAL: %s: %v", msg, err))
}

func (dcl *decisionContextLogger) WithFields(fields ...Field) Logger {
	// For decision context logger, fields are passed per-call
	return dcl
}

func (dcl *decisionContextLogger) WithContext(ctx context.Context) Logger {
	// Update operation context
	if dcl.decisionCtx != nil {
		dcl.decisionCtx = dcl.decisionCtx.WithOperationContext(ctx)
	}
	return dcl
}

func (dcl *decisionContextLogger) WithObjectRef(kind, id string) Logger {
	// Object refs are passed as fields per-call
	return dcl
}

// Close closes all dynamically created destinations
func (dcl *decisionContextLogger) Close() error {
	var destinationsCopy map[string]*destination
	_ = concurrency.WithLockCtx(
		&dcl.destMu,
		pkgctx.NewSystemContext(),
		"decision_logger_close_copy",
		func() error {
			destinationsCopy = make(map[string]*destination, len(dcl.destCache))
			maps.Copy(destinationsCopy, dcl.destCache)
			return nil
		},
	)

	// Close destinations outside lock to avoid holding lock during I/O
	var errs []error
	for _, dest := range destinationsCopy {
		if dest.closer != nil {
			if err := dest.closer.Close(); err != nil {
				errs = append(errs, fmt.Errorf("failed to close destination %s: %w", dest.name, err)) //nolint:gosec
			}
		}
	}

	if len(errs) > 0 {
		return errfmt.Errorf("errors closing destinations: %v", errs)
	}
	return nil
}
