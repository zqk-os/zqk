package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// LogLevel represents the severity level of a log message
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// Log delivery method constants
const (
	logDeliveryMethodTraceFile         = "trace_file"
	logDeliveryMethodTraceFileFallback = "trace_file_fallback"
	logDeliveryMethodMCPProtocol       = "mcp_protocol"
	logDeliveryMethodQueue             = "queue"
	logDeliveryMethodNone              = "none"
)

// Log error type constants
const (
	logErrorTypeMarshalFailed    = "marshal_failed"
	logErrorTypeWriteFailed      = "write_failed"
	logErrorTypeTimeout          = "timeout"
	logErrorTypeQueueFull        = "queue_full"
	logErrorTypeQueueUnavailable = "queue_unavailable"
)

// LogMetricsBuilder provides a fluent API for building log message metrics
type LogMetricsBuilder struct {
	metrics map[string]any
}

// NewLogMetricsBuilder creates a new log metrics builder with common fields
func NewLogMetricsBuilder(level LogLevel, message string, fields map[string]any, startTime time.Time) *LogMetricsBuilder {
	return &LogMetricsBuilder{
		metrics: map[string]any{
			"level":          string(level),
			"message_length": len(message),
			"fields_count":   len(fields),
			"duration_ns":    time.Since(startTime).Nanoseconds(),
		},
	}
}

// TransportReady sets whether transport was ready
func (b *LogMetricsBuilder) TransportReady(ready bool) *LogMetricsBuilder {
	b.metrics["transport_ready"] = ready
	return b
}

// DeliveryMethod sets the delivery method
func (b *LogMetricsBuilder) DeliveryMethod(method string) *LogMetricsBuilder {
	b.metrics["delivery_method"] = method
	return b
}

// Success sets whether delivery succeeded
func (b *LogMetricsBuilder) Success(success bool) *LogMetricsBuilder {
	b.metrics["success"] = success
	return b
}

// Error sets the error type
func (b *LogMetricsBuilder) Error(errorType string) *LogMetricsBuilder {
	b.metrics["error"] = errorType
	return b
}

// Build returns the built metrics map
func (b *LogMetricsBuilder) Build() map[string]any {
	return b.metrics
}

// SendLogMessage sends a log message via the MCP logging channel
// This routes logs through the MCP protocol instead of stdout/stderr
// The client can display these logs in its UI
func (s *Server) SendLogMessage(level LogLevel, message string, fields map[string]any) error {
	// GUARD 0: Check atomic shutdown flag FIRST (fastest check, before ANY work)
	// This is the PRIMARY guard - if shutdown is ordered, we never try to acquire locks
	// CRITICAL: This check happens before ANY other operations, including time.Now()
	if s.shutdownFlag.Load() == 1 {
		// Shutdown ordered - only write to trace file, skip all other operations
		var traceWriter io.Writer
		_ = concurrency.RunInRLockWithLogger(
			&s.traceWriterMu, LockNameMcpLoggingTraceWriterShutdown, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				traceWriter = s.traceWriter
				return nil
			},
		)

		if traceWriter != nil {
			formattedMessage := fmt.Sprintf("[MCP_%s] %s", strings.ToUpper(string(level)), message)
			if len(fields) > 0 {
				fieldsJSON, _ := json.Marshal(fields) //nolint:errcheck // Marshal errors are non-critical for logging
				formattedMessage += fmt.Sprintf(" %s", string(fieldsJSON))
			}
			writeTraceMessage(traceWriter, formattedMessage)
		}
		return nil // Exit immediately, don't try to use queue or acquire any locks
	}

	startTime := time.Now()

	// GUARD 1: Check shutdown context (defensive, handles race conditions)
	// Sometimes shutdownFlag might not be set yet, but context is cancelled
	// During shutdown, we must avoid ALL lock acquisitions (clientsMu, transportMu, etc.)
	// This check must happen before ANY other operations to prevent deadlock
	// If shutdown context is cancelled, return immediately and only write to trace file
	select {
	case <-s.shutdownCtx.Done():
		// Shutdown ordered - ONLY write to trace file, skip ALL lock acquisitions
		// This is the critical path that prevents deadlock
		var traceWriter io.Writer
		_ = concurrency.RunInRLock(&s.traceWriterMu, func() error {
			traceWriter = s.traceWriter
			return nil
		})

		if traceWriter != nil {
			formattedMessage := fmt.Sprintf("[MCP_%s] %s", strings.ToUpper(string(level)), message)
			if len(fields) > 0 {
				fieldsJSON, _ := json.Marshal(fields) //nolint:errcheck // Marshal errors are non-critical for logging
				formattedMessage += fmt.Sprintf(" %s", string(fieldsJSON))
			}
			writeTraceMessage(traceWriter, formattedMessage)
		}
		// Skip ALL other operations during shutdown - no queue access, no transport access, no event recording
		return nil
	default:
		// Shutdown context not cancelled yet - continue with normal flow
		// But double-check with atomic flag (lock-free, thread-safe)
		if s.shutdownFlag.Load() == 1 {
			// Shutdown in progress - only write to trace file, skip all other operations
			var traceWriter io.Writer
			_ = concurrency.RunInRLockWithLogger(
				&s.traceWriterMu, LockNameMcpLoggingTraceWriterShutdownFlag, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					traceWriter = s.traceWriter
					return nil
				},
			)

			if traceWriter != nil {
				formattedMessage := fmt.Sprintf("[MCP_%s] %s", strings.ToUpper(string(level)), message)
				if len(fields) > 0 {
					fieldsJSON, _ := json.Marshal(fields) //nolint:errcheck // Marshal errors are non-critical for logging
					formattedMessage += fmt.Sprintf(" %s", string(fieldsJSON))
				}
				writeTraceMessage(traceWriter, formattedMessage)
			}
			// Skip ALL other operations during shutdown
			return nil
		}
	}

	// CRITICAL: Final shutdown check immediately before acquiring transportMu lock
	// This prevents any lock acquisition during shutdown, even for read locks
	// Quadruple redundancy: we've done work since the last check, so shutdown may have been ordered
	if s.shutdownFlag.Load() == 1 {
		// Shutdown ordered - only write to trace file, skip all other operations
		traceWriter := s.getTraceWriter()
		if traceWriter != nil {
			formattedMessage := fmt.Sprintf("[MCP_%s] %s", strings.ToUpper(string(level)), message)
			if len(fields) > 0 {
				fieldsJSON, _ := json.Marshal(fields) //nolint:errcheck // Marshal errors are non-critical for logging
				formattedMessage += fmt.Sprintf(" %s", string(fieldsJSON))
			}
			writeTraceMessage(traceWriter, formattedMessage)
		}
		return nil // Exit immediately, don't acquire any locks
	}
	select {
	case <-s.shutdownCtx.Done():
		// Shutdown ordered - only write to trace file, skip all other operations
		traceWriter := s.getTraceWriter()
		if traceWriter != nil {
			formattedMessage := fmt.Sprintf("[MCP_%s] %s", strings.ToUpper(string(level)), message)
			if len(fields) > 0 {
				fieldsJSON, _ := json.Marshal(fields) //nolint:errcheck // Marshal errors are non-critical for logging
				formattedMessage += fmt.Sprintf(" %s", string(fieldsJSON))
			}
			writeTraceMessage(traceWriter, formattedMessage)
		}
		return nil // Exit immediately, don't acquire any locks
	default:
	}

	// Get transport writer and format atomically
	// We need to check and use them while holding the lock to avoid race conditions
	var writer *bufio.Writer
	var format *MessageFormat
	var transportReady bool
	_ = concurrency.RunInRLockWithLogger(
		&s.transportMu, LockNameMcpLoggingGetTransport, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			writer = s.transportWriter
			format = s.transportFormat
			transportReady = writer != nil && format != nil
			return nil
		},
	)

	if !transportReady {
		// Transport not ready - fallback to trace file so logs aren't lost
		traceWriter := s.getTraceWriter()
		if traceWriter != nil {
			formattedMessage := fmt.Sprintf("[MCP_%s] %s", strings.ToUpper(string(level)), message)
			if len(fields) > 0 {
				fieldsJSON, _ := json.Marshal(fields) //nolint:errcheck // Marshal errors are non-critical for logging
				formattedMessage += fmt.Sprintf(" %s", string(fieldsJSON))
			}
			writeTraceMessage(traceWriter, formattedMessage)
		}
		// Record log_message event with delivery metrics
		eventCtx := s.getClientEventContext()
		eventCtx.RecordEvent("log_message", NewLogMetricsBuilder(level, message, fields, startTime).
			TransportReady(false).
			DeliveryMethod(logDeliveryMethodTraceFile).
			Success(true).
			Build())
		return nil
	}

	// Build log message params
	params := map[string]any{
		"level":   string(level),
		"message": message,
	}

	// Add fields if provided
	if len(fields) > 0 {
		params["data"] = fields
	}

	// Create JSON-RPC notification for log message
	notification := map[string]any{
		jsonrpcFieldJSONRPC: JSONRPCVersion,
		jsonrpcFieldMethod:  notificationMethodLogMessage,
		jsonrpcFieldParams:  params,
	}

	// Marshal to JSON
	data, err := json.Marshal(notification)
	if err != nil {
		// Record log_message event with metrics for marshal failure
		eventCtx := s.getClientEventContext()
		eventCtx.RecordEvent("log_message", NewLogMetricsBuilder(level, message, fields, startTime).
			TransportReady(true).
			DeliveryMethod(logDeliveryMethodNone).
			Success(false).
			Error(logErrorTypeMarshalFailed).
			Build())
		return errfmt.Newf("failed to marshal log notification").Wrap(err)
	}

	// CRITICAL: Check shutdown context AGAIN before trying to find queue
	// Shutdown may have been ordered between the initial check and this point
	// If shutdown is ordered, NEVER try to find queue - just write to trace file and return
	// This prevents any lock acquisition attempts during shutdown
	select {
	case <-s.shutdownCtx.Done():
		// Shutdown ordered - skip queue entirely, only write to trace file
		traceWriter := s.getTraceWriter()
		if traceWriter != nil {
			formattedMessage := fmt.Sprintf("[MCP_%s] %s", strings.ToUpper(string(level)), message)
			if len(fields) > 0 {
				fieldsJSON, _ := json.Marshal(fields) //nolint:errcheck // Marshal errors are non-critical for logging
				formattedMessage += fmt.Sprintf(" %s", string(fieldsJSON))
			}
			writeTraceMessage(traceWriter, formattedMessage)
		}
		return nil // Exit immediately, don't try to use queue or acquire any locks
	default:
		// Double-check with atomic flag (lock-free, thread-safe)
		if s.shutdownFlag.Load() == 1 {
			// Shutdown in progress - skip queue, only write to trace file
			traceWriter := s.getTraceWriter()
			if traceWriter != nil {
				formattedMessage := fmt.Sprintf("[MCP_%s] %s", strings.ToUpper(string(level)), message)
				if len(fields) > 0 {
					fieldsJSON, _ := json.Marshal(fields) //nolint:errcheck // Marshal errors are non-critical for logging
					formattedMessage += fmt.Sprintf(" %s", string(fieldsJSON))
				}
				writeTraceMessage(traceWriter, formattedMessage)
			}
			return nil // Exit immediately, don't try to use queue
		}
	}

	// CRITICAL: Final shutdown check immediately before calling findClientQueue()
	// Even though we checked above, shutdown may have been ordered in the meantime
	// This is the last chance to avoid lock acquisition
	select {
	case <-s.shutdownCtx.Done():
		// Shutdown ordered just now - skip queue, only write to trace file
		traceWriter := s.getTraceWriter()
		if traceWriter != nil {
			formattedMessage := fmt.Sprintf("[MCP_%s] %s", strings.ToUpper(string(level)), message)
			if len(fields) > 0 {
				fieldsJSON, _ := json.Marshal(fields) //nolint:errcheck // Marshal errors are non-critical for logging
				formattedMessage += fmt.Sprintf(" %s", string(fieldsJSON))
			}
			writeTraceMessage(traceWriter, formattedMessage)
		}
		return nil // Exit immediately, don't try to use queue
	default:
		// Continue - shutdown not ordered yet
	}

	// CRITICAL: Final shutdown check RIGHT BEFORE calling findClientQueue()
	// This is the absolute last chance to avoid any lock acquisition
	// Even TryRLock() should be avoided during shutdown to prevent any potential issues
	// CRITICAL: Check BOTH shutdown flag AND context - double redundancy for 99.9999% reliability
	if s.shutdownFlag.Load() == 1 {
		// Shutdown ordered - skip queue entirely, only write to trace file
		traceWriter := s.getTraceWriter()
		if traceWriter != nil {
			formattedMessage := fmt.Sprintf("[MCP_%s] %s", strings.ToUpper(string(level)), message)
			if len(fields) > 0 {
				fieldsJSON, _ := json.Marshal(fields) //nolint:errcheck // Marshal errors are non-critical for logging
				formattedMessage += fmt.Sprintf(" %s", string(fieldsJSON))
			}
			writeTraceMessage(traceWriter, formattedMessage)
		}
		return nil // Exit immediately, don't call findClientQueue() at all
	}
	select {
	case <-s.shutdownCtx.Done():
		// Shutdown ordered at the last possible moment - skip queue entirely
		traceWriter := s.getTraceWriter()
		if traceWriter != nil {
			formattedMessage := fmt.Sprintf("[MCP_%s] %s", strings.ToUpper(string(level)), message)
			if len(fields) > 0 {
				fieldsJSON, _ := json.Marshal(fields) //nolint:errcheck // Marshal errors are non-critical for logging
				formattedMessage += fmt.Sprintf(" %s", string(fieldsJSON))
			}
			writeTraceMessage(traceWriter, formattedMessage)
		}
		return nil // Exit immediately, don't call findClientQueue() at all
	default:
		// Continue - shutdown not ordered yet
	}

	// CRITICAL: One more atomic check immediately before findClientQueue()
	// This is the absolute final guard - triple redundancy for 99.9999% reliability
	if s.shutdownFlag.Load() == 1 {
		// Shutdown ordered just now - skip queue entirely
		traceWriter := s.getTraceWriter()
		if traceWriter != nil {
			formattedMessage := fmt.Sprintf("[MCP_%s] %s", strings.ToUpper(string(level)), message)
			if len(fields) > 0 {
				fieldsJSON, _ := json.Marshal(fields) //nolint:errcheck // Marshal errors are non-critical for logging
				formattedMessage += fmt.Sprintf(" %s", string(fieldsJSON))
			}
			writeTraceMessage(traceWriter, formattedMessage)
		}
		return nil // Exit immediately, don't call findClientQueue() at all
	}

	// Try to use message queue if available (provides backpressure relief)
	// Use safe abstraction to avoid deadlock during shutdown
	// NOTE: findClientQueue uses TryRLock() so it won't block, but we've already checked shutdown above
	// CRITICAL: Even though TryRLock() is non-blocking, we avoid calling it during shutdown
	// to prevent any potential race conditions or edge cases
	queue := s.findClientQueue()

	if queue != nil {
		// Determine priority from log level
		priority := "normal"
		switch level {
		case LogLevelError:
			priority = "high"
		case LogLevelWarn:
			priority = "normal"
		case LogLevelInfo, LogLevelDebug:
			priority = "low"
		}
		// Enqueue message (non-blocking)
		dropped := !queue.Enqueue(data, format, priority, nil)
		if dropped {
			// Queue full, fall back to file logging
			if s.getTraceWriter() != nil {
				logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
				entry := logging.Fluent(logger).Warn(fmt.Sprintf("Message queue full, falling back to file log: %s", message)).
					EmitComponent("mcp_server").
					TraceMarker().
					MCPLogLevel(string(level)).
					Reason("message_queue_full")
				for k, v := range fields {
					entry = entry.String(k, fmt.Sprintf("%v", v))
				}
				entry.Log()
			}
			// Record log_message event with metrics for queue full
			eventCtx := s.getClientEventContext()
			eventCtx.RecordEvent("log_message", NewLogMetricsBuilder(level, message, fields, startTime).
				TransportReady(true).
				DeliveryMethod(logDeliveryMethodTraceFileFallback).
				Success(false).
				Error(logErrorTypeQueueFull).
				Build())
			// Record dropped message in metrics
			if s.mcpMetrics != nil {
				s.mcpMetrics.RecordLogMessage(true)
			}
			return nil
		}
		// Successfully queued
		eventCtx := s.getClientEventContext()
		eventCtx.RecordEvent("log_message", NewLogMetricsBuilder(level, message, fields, startTime).
			TransportReady(true).
			DeliveryMethod(logDeliveryMethodQueue).
			Success(true).
			Build())
		// Record successful message in metrics
		if s.mcpMetrics != nil {
			s.mcpMetrics.RecordLogMessage(false)
		}
		return nil
	}

	// CRITICAL: Never fall back to direct write - this causes interleaving with response writes
	// Direct writes to the transport writer can corrupt the JSON-RPC stream
	// If queue is not available, just drop the message and log to trace file
	// This ensures all writes go through the message queue, which serializes them properly
	if s.getTraceWriter() != nil {
		logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
		entry := logging.Fluent(logger).Debug(fmt.Sprintf("Message queue unavailable, logging to trace file only: %s", message)).
			EmitComponent("mcp_server").
			TraceMarker().
			MCPLogLevel(string(level)).
			Reason("message_queue_unavailable")
		for k, v := range fields {
			entry = entry.String(k, fmt.Sprintf("%v", v))
		}
		entry.Log()
	}
	// Record log_message event with metrics for queue unavailable
	eventCtx := s.getClientEventContext()
	eventCtx.RecordEvent("log_message", NewLogMetricsBuilder(level, message, fields, startTime).
		TransportReady(true).
		DeliveryMethod(logDeliveryMethodTraceFileFallback).
		Success(false).
		Error(logErrorTypeQueueUnavailable).
		Build())
	// Don't return error - logging failures shouldn't crash the server
	// The message has been logged via trace file, which is sufficient
	return nil
}

// SendLogDebug sends a debug log message
func (s *Server) SendLogDebug(message string, fields map[string]any) error {
	return s.SendLogMessage(LogLevelDebug, message, fields)
}

// SendLogInfo sends an info log message
func (s *Server) SendLogInfo(message string, fields map[string]any) error {
	return s.SendLogMessage(LogLevelInfo, message, fields)
}

// SendLogWarn sends a warning log message
func (s *Server) SendLogWarn(message string, fields map[string]any) error {
	return s.SendLogMessage(LogLevelWarn, message, fields)
}

// SendLogError sends an error log message
func (s *Server) SendLogError(message string, fields map[string]any) error {
	return s.SendLogMessage(LogLevelError, message, fields)
}

// SendProgressLog sends a progress update via logging channel
// Useful for long-running operations
func (s *Server) SendProgressLog(operation string, progress float64, message string) error {
	fields := map[string]any{
		objects.FieldKeyOperation: operation,
		"progress":                progress,
	}
	return s.SendLogInfo(message, fields)
}
