package logging

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// LogRouter routes logs to multiple destinations with different formatters and levels
type LogRouter struct {
	destinations map[string]*destination
	mu           sync.RWMutex
}

type destination struct {
	name         string
	writer       io.Writer
	buffered     *BufferedWriter // Buffered writer wrapper (nil if not buffered)
	level        LogLevel
	formatter    Formatter
	closer       io.Closer  // For file writers that need closing
	writeMu      sync.Mutex // Mutex for serializing writes to file destinations
	useBuffering bool       // Whether to use buffering for this destination
	mu           sync.RWMutex
}

// NewLogRouter creates a new log router
func NewLogRouter() *LogRouter {
	return &LogRouter{
		destinations: make(map[string]*destination),
	}
}

// AddDestination adds a destination to the router
// CRITICAL FIX: Check if destination exists first (with read lock) to avoid unnecessary write lock contention
// This prevents deadlocks when multiple goroutines try to add the same destination concurrently
// If destination already exists, update it if the new level is more permissive (lower numeric value)
func (lr *LogRouter) AddDestination(name string, writer io.Writer, level LogLevel, formatter Formatter) {
	// Fast path: Check if destination already exists (read lock)
	var existingDest *destination
	var exists bool
	var existingLevel LogLevel
	_ = concurrency.WithRLockCtx(
		&lr.mu,
		pkgctx.NewSystemContext(),
		"log_router_add_dest_check",
		func() error {
			var ok bool
			existingDest, ok = lr.destinations[name]
			exists = ok
			if exists {
				// Destination exists - check if we need to update the level
				// Use more permissive level (lower numeric value = higher priority)
				// InfoLevel (1) is more permissive than ErrorLevel (3)
				existingLevel = existingDest.level
			}
			return nil
		},
	)

	if exists {
		// If new level is more permissive (lower value), update the destination
		if level < existingLevel {
			_ = concurrency.WithLockCtx(
				&lr.mu,
				pkgctx.NewSystemContext(),
				"log_router_add_dest_update",
				func() error {
					// Double-check after acquiring write lock
					if dest, stillExists := lr.destinations[name]; stillExists {
						dest.mu.Lock()
						dest.level = level
						// Also update formatter and writer if they differ (for consistency)
						if dest.formatter != formatter {
							dest.formatter = formatter
						}
						if dest.writer != writer {
							dest.writer = writer
						}
						dest.mu.Unlock()
					}
					return nil
				},
			)
		}
		return
	}

	// Slow path: Destination doesn't exist, need to add it (write lock)
	_ = concurrency.WithLockCtx(
		&lr.mu,
		pkgctx.NewSystemContext(),
		"log_router_add_dest",
		func() error {
			// Double-check after acquiring write lock (another goroutine might have added it)
			if _, exists := lr.destinations[name]; !exists {
				lr.destinations[name] = &destination{
					name:      name,
					writer:    writer,
					level:     level,
					formatter: formatter,
				}
			}
			return nil
		},
	)
}

// AddFileDestination adds a file destination
// CRITICAL FIX: Check if destination exists first (with read lock) to avoid unnecessary write lock contention
func (lr *LogRouter) AddFileDestination(name, filePath string, level LogLevel, formatter Formatter) error {
	return lr.AddFileDestinationWithPolicy(name, filePath, level, formatter, nil)
}

// AddFileDestinationWithPolicy adds a file destination with a rolling policy
// If policy is nil, uses default rolling policy (size-based, 10MB, 5 files)
func (lr *LogRouter) AddFileDestinationWithPolicy(name, filePath string, level LogLevel, formatter Formatter, policy *RollingPolicy) error {
	// Fast path: Check if destination already exists (read lock)
	var exists bool
	_ = concurrency.WithRLock(
		&lr.mu,
		"log_router_check_destination",
		func() error {
			_, exists = lr.destinations[name]
			return nil
		},
	)
	if exists {
		return nil // Destination already exists, no need to add
	}

	// Create writer based on policy (no lock held - file I/O can be slow)
	var closer io.Closer
	var err error

	if policy == nil {
		// Use default rolling policy
		defaultPolicy := DefaultRollingPolicy()
		policy = &defaultPolicy
	}

	factory := NewRollingWriterFactory()
	rollingWriter, err := factory.CreateWriter(filePath, *policy)
	if err != nil {
		return errfmt.Errorf("failed to create rolling writer for log file %s: %w", filePath, err)
	}

	// Wrap with buffered writer to minimize I/O context switches
	// Use default buffering config (64KB buffer, 1s flush interval)
	bufferedConfig := DefaultBufferedWriterConfig()
	bufferedWriter := NewBufferedWriter(rollingWriter, bufferedConfig)
	closer = rollingWriter // Close the underlying rolling writer, buffered writer will flush

	// Slow path: Destination doesn't exist, need to add it (write lock)
	var existsAfterIO bool
	err = concurrency.WithLockCtx(
		&lr.mu,
		pkgctx.NewSystemContext(),
		"log_router_add_file",
		func() error {
			// Double-check after acquiring write lock (another goroutine might have added it)
			_, existsAfterIO = lr.destinations[name]
			if existsAfterIO {
				return nil
			}

			// One BufferedWriter only — a second wrap spawned a second auto-flush goroutine
			// per file destination (thread dump: hundreds of autoFlush Gs).
			lr.destinations[name] = &destination{
				name:         name,
				writer:       bufferedWriter,
				buffered:     bufferedWriter,
				level:        level,
				formatter:    formatter,
				closer:       closer,
				useBuffering: true,
			}
			return nil
		},
	)
	if err != nil {
		_ = bufferedWriter.Close()
		return err
	}

	if existsAfterIO {
		// Another goroutine added it — Close unregisters the unused shared flusher entry.
		_ = bufferedWriter.Close()
		return nil
	}

	return nil
}

// AddStdoutDestination adds stdout as a destination
func (lr *LogRouter) AddStdoutDestination(level LogLevel, formatter Formatter) {
	lr.AddDestination("stdout", os.Stdout, level, formatter)
}

// AddStderrDestination adds stderr as a destination
func (lr *LogRouter) AddStderrDestination(level LogLevel, formatter Formatter) {
	lr.AddDestination("stderr", os.Stderr, level, formatter)
}

// RemoveDestination removes a destination
func (lr *LogRouter) RemoveDestination(name string) {
	_ = concurrency.WithLock(
		&lr.mu,
		"log_router_remove_destination",
		func() error {
			if dest, ok := lr.destinations[name]; ok {
				dest.mu.RLock()
				closer := dest.closer
				dest.mu.RUnlock()
				if closer != nil {
					_ = closer.Close()
				}
			}
			delete(lr.destinations, name)
			return nil
		},
	)
}

// GetLogger returns a logger that routes to all destinations
func (lr *LogRouter) GetLogger() Logger {
	return &routerLogger{router: lr, loggingCtx: nil}
}

// GetLoggerWithLoggingContext returns a logger that routes to all destinations
// and uses the provided LoggingContext to determine routing behavior
func (lr *LogRouter) GetLoggerWithLoggingContext(loggingCtx *pkgctx.LoggingContext) Logger {
	return &routerLogger{router: lr, loggingCtx: loggingCtx}
}

// Close closes all destinations that need closing (e.g., files)
func (lr *LogRouter) Close() error {
	var destinationsCopy map[string]*destination
	_ = concurrency.WithLockCtx(
		&lr.mu,
		pkgctx.NewSystemContext(),
		"log_router_close_copy",
		func() error {
			destinationsCopy = make(map[string]*destination, len(lr.destinations))
			maps.Copy(destinationsCopy, lr.destinations)
			return nil
		},
	)

	// Close destinations outside lock to avoid holding lock during I/O
	var errs []error
	for name, dest := range destinationsCopy {
		dest.mu.RLock()
		buffered := dest.buffered
		closer := dest.closer
		dest.mu.RUnlock()

		// Flush buffered writer first if present
		if buffered != nil {
			if err := buffered.Close(); err != nil {
				errs = append(errs, errfmt.Errorf("failed to close buffered writer for destination %s: %w", name, err))
			}
		}

		// Then close the underlying writer
		if closer != nil {
			if err := closer.Close(); err != nil {
				errs = append(errs, errfmt.Errorf("failed to close destination %s: %w", name, err))
			}
		}
	}

	if len(errs) > 0 {
		return errfmt.Errorf("errors closing destinations: %v", errs)
	}
	return nil
}

// routerLogger implements Logger and routes to all destinations
type routerLogger struct {
	router     *LogRouter
	loggingCtx *pkgctx.LoggingContext // LoggingContext for determining routing behavior
}

func (rl *routerLogger) log(level LogLevel, msg string, err error, fields ...Field) {
	var destinations []*destination
	_ = concurrency.WithRLockCtx(
		&rl.router.mu,
		pkgctx.NewSystemContext(),
		"router_logger_log_copy",
		func() error {
			destinations = make([]*destination, 0, len(rl.router.destinations))
			for _, dest := range rl.router.destinations {
				destinations = append(destinations, dest)
			}
			return nil
		},
	)

	// Route to each destination that accepts this level
	for _, dest := range destinations {
		dest.mu.RLock()
		destLevel := dest.level
		destWriter := dest.writer
		destBuffered := dest.buffered
		destFormatter := dest.formatter
		destName := dest.name
		dest.mu.RUnlock()

		if level < destLevel {
			continue // Skip if below threshold
		}

		// CRITICAL: Skip stdio destinations when LoggingContext says to suppress (prevents terminal pollution)
		// System/MCP: never write to stdio at any level (file only) so background events (e.g. orphan_cleanup_batch,
		// scheduler, generate-builders completion) don't flood the terminal during build-all or when tailing the main log.
		// Debug: same rules as before (debug logs only to stdio for ai-agent/debug profiles).
		isStdio := isStdout(destWriter) || isStderr(destWriter)
		if isStdio {
			// Use LoggingContext if available (preferred method)
			if rl.loggingCtx != nil {
				profile := rl.loggingCtx.Profile
				// System and MCP profiles: never write to stdio at any level (file-only; build/scheduler logs stay in files)
				if (profile == pkgctx.ProfileSystem || profile == pkgctx.ProfileMCP) && isStdio {
					continue
				}
				// For other profiles, only apply debug-specific suppression below
				if level == DebugLevel {
					if rl.loggingCtx.ShouldSuppressDebugToStdout() {
						continue
					}
					if profile != pkgctx.ProfileAIAgent && profile != pkgctx.ProfileDebug {
						continue
					}
				}
			} else {
				// Fallback: Check destination name for backward compatibility
				if destName == "stdio_system" || destName == "stdio_mcp" ||
					destName == "system" || destName == "mcp" ||
					strings.HasSuffix(destName, "_system") || strings.HasSuffix(destName, "_mcp") {
					continue
				}
				if level == DebugLevel {
					allowsDebugOnStdout := destName == "stdio_ai-agent" || destName == "stdio_debug" ||
						destName == "ai-agent" || destName == "debug" ||
						strings.HasSuffix(destName, "_ai-agent") || strings.HasSuffix(destName, "_debug")
					if !allowsDebugOnStdout {
						continue
					}
				}
			}
		}

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
			Context:   pkgctx.NewSystemContext(), // Context can be enhanced later
		}

		// Format and write
		formatted, formatErr := destFormatter.Format(entry)
		if formatErr != nil {
			// CRITICAL: If MCP server is actively serving, suppress fallback output to stdout/stderr only
			// File destinations should still receive fallback output to preserve debugging context
			// IDE may merge stderr into stdout for stdio-based MCP, so even stderr output
			// would pollute the JSON-RPC protocol stream. We must suppress stdio output.
			if isMCPServerServing() && (isStdout(destWriter) || isStderr(destWriter)) {
				// MCP server is actively serving and destination is stdout/stderr - suppress fallback output
				// The JSON-RPC protocol stream on stdout must remain completely clean
				// File destinations will still receive formatted output (if formatter succeeds)
				continue
			}

			// For subprocesses, use stderr for fallback (subprocess stderr is captured separately)
			// Fallback - ALWAYS use stderr for fallback to prevent stdout pollution
			// CRITICAL: This is the last line of defense - if JSON formatting fails, we must not pollute stdout
			//
			// ALWAYS use stderr for fallback - never write fallback messages to stdout
			// This ensures that even if the destination was incorrectly configured, fallback messages
			// won't pollute stdout (which is reserved for JSON-RPC protocol in MCP mode)
			//
			// Additional safety: If the destination's writer is stdout, we MUST use stderr for fallback
			// This prevents any possibility of fallback messages appearing on stdout
			var fallbackWriter io.Writer = os.Stderr
			if isStdout(destWriter) {
				// Destination is configured for stdout - force fallback to stderr
				fallbackWriter = os.Stderr
			} else if !isStdout(destWriter) && !isStderr(destWriter) {
				// Destination is a file - use it directly for fallback
				// destWriter is io.Writer, which is compatible with fmt.Fprintf
				fallbackWriter = destWriter
			}

			// Include the formatting error in the fallback message for debugging
			// Note: Using fmt.Sprintf + Write instead of fmt.Fprintf to comply with POL-CODE-007
			// This is an emergency fallback when the logging framework itself fails
			fallbackMsg := fmt.Sprintf("[%s] %s: %s (formatting error: %v)\n", level.String(), getCurrentTime().Format("2006-01-02T15:04:05Z07:00"), msg, formatErr)
			_, _ = fallbackWriter.Write([]byte(fallbackMsg))
			continue
		}

		// Protect MCP protocol stream - suppress stdio output automatically
		if shouldSuppressStdioForMCPProtocol(destWriter) {
			continue
		}

		// Select appropriate writer with automatic protection for debug logs and MCP protocol
		writer := selectWriterForLogLevel(level, destWriter)
		if writer == nil {
			continue // Suppressed to protect MCP protocol stream
		}

		// Additional profile-based checks for debug logs
		if level == DebugLevel && isStdout(writer) && rl.loggingCtx != nil {
			profile := rl.loggingCtx.Profile
			if profile == pkgctx.ProfileSystem || profile == pkgctx.ProfileMCP ||
				rl.loggingCtx.ShouldSuppressDebugToStdout() {
				writer = os.Stderr
			} else if profile != pkgctx.ProfileAIAgent && profile != pkgctx.ProfileDebug {
				writer = os.Stderr
			}
		}

		// For file destinations, serialize writes to prevent corruption from concurrent writes
		// Use the destination's mutex if it's a file (not stdout/stderr)
		if !isStdout(destWriter) && !isStderr(destWriter) {
			// Add newline for JSONL format (one JSON object per line)
			// This makes the file easier to parse and ensures each entry is on its own line
			// However, if the formatter already added a newline (e.g., TextFormatter), don't add another
			var toWrite []byte
			if len(formatted) > 0 && formatted[len(formatted)-1] == '\n' {
				// Formatter already includes newline - use as-is
				toWrite = formatted
			} else {
				// Formatter doesn't include newline (e.g., JSONFormatter) - add one
				toWrite = make([]byte, 0, len(formatted)+1)
				toWrite = append(toWrite, formatted...)
				toWrite = append(toWrite, '\n')
			}

			// Use timeout wrapper for writeMu to prevent deadlocks
			_ = concurrency.WithLockCtx(
				&dest.writeMu,
				pkgctx.NewSystemContext(),
				"log_router_write_file",
				func() error {
					// Use buffered writer with level-aware flushing if available
					if destBuffered != nil {
						// Write with level awareness - will auto-flush on error/fatal
						//nolint:errcheck // Intentional error ignored
						_, _ = destBuffered.WriteWithLevel(toWrite, level)
					} else {
						// Fallback to direct write (shouldn't happen for file destinations)
						//nolint:errcheck // Intentional error ignored
						_, _ = writer.Write(toWrite)
					}
					return nil
				},
			)
		} else {
			// For stdout/stderr, write without newline (may be part of structured output)
			// No buffering for stdio to ensure immediate output
			//nolint:errcheck // Intentional error ignored
			_, _ = writer.Write(formatted)
		}
	}
}

func (rl *routerLogger) Debug(msg string, fields ...Field) {
	rl.log(DebugLevel, msg, nil, fields...)
}

func (rl *routerLogger) Info(msg string, fields ...Field) {
	rl.log(InfoLevel, msg, nil, fields...)
}

func (rl *routerLogger) Warn(msg string, fields ...Field) {
	rl.log(WarnLevel, msg, nil, fields...)
}

func (rl *routerLogger) Error(msg string, err error, fields ...Field) {
	rl.log(ErrorLevel, msg, err, fields...)
}

func (rl *routerLogger) Fatal(msg string, err error, fields ...Field) {
	rl.log(FatalLevel, msg, err, fields...)
	// TRACK: [Fatal logging error]
	panic(fmt.Sprintf("FATAL: %s: %v", msg, err))
}

func (rl *routerLogger) WithFields(fields ...Field) Logger {
	// For router logger, fields are passed per-call
	// We could create a wrapper, but for simplicity, just return self
	// Callers should pass fields to each log call
	return rl
}

func (rl *routerLogger) WithContext(ctx context.Context) Logger {
	// Context is not stored in router logger
	// Could be enhanced to store context and include in entries
	return rl
}

func (rl *routerLogger) WithObjectRef(kind, id string) Logger {
	// Object refs are passed as fields per-call
	return rl
}

// getCurrentTime returns current time (extracted for testability)
var getCurrentTime = time.Now
