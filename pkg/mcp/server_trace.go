package mcp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"github.com/zqk-os/zqk/pkg/zqktime"

	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
)

func (s *Server) getTraceWriter() io.Writer {
	var writer io.Writer
	_ = concurrency.RunInRLockWithLogger(
		&s.traceWriterMu, LockNameMcpServerGetTraceWriter, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			writer = s.traceWriter
			return nil
		},
	)
	return writer
}

// writeTraceMessage writes a formatted message to a trace writer via WriteString(Sprintf).
// POL-CODE-007: all output through logging/trace framework, no direct Print* to stdio.
func writeTraceMessage(writer io.Writer, message string) {
	if writer == nil {
		return
	}
	timestamp := zqktime.NowLayoutUTC(zqktime.LayoutDateTimeMillis)
	formatted := fmt.Sprintf("[%s] %s\n", timestamp, message)
	_, _ = writer.Write([]byte(formatted))
}

func (s *Server) traceLogf(format string, args ...any) {
	traceWriter := s.getTraceWriter()
	if traceWriter == nil {
		// If trace writer is nil, try to write ERROR and WARN messages to stderr as fallback
		message := fmt.Sprintf(format, args...)
		if !strings.HasPrefix(message, "[MCP_ERROR]") && !strings.HasPrefix(message, "[MCP_WARN]") {
			return
		}
		logger := getSystemLogger()
		if strings.HasPrefix(message, "[MCP_ERROR]") {
			logging.Fluent(logger).Error(message, nil).
				EmitComponent("mcp_trace").
				Log()
		} else {
			logging.Fluent(logger).Warn(message).
				EmitComponent("mcp_trace").
				Log()
		}
		return
	}

	message := fmt.Sprintf(format, args...)

	// When writing to stderr (trace disabled), filter out DEBUG, INFO, and TRACE messages
	// Only ERROR and WARN messages should appear in stderr to avoid confusion
	if traceWriter == os.Stderr {
		// Check message prefix for log level
		if strings.HasPrefix(message, "[MCP_DEBUG]") || strings.HasPrefix(message, "[MCP_INFO]") || strings.HasPrefix(message, "[MCP_TRACE]") {
			// Suppress DEBUG, INFO, and TRACE messages when writing to stderr
			return
		}
		// Allow ERROR and WARN messages through to stderr
	}

	writeTraceMessage(traceWriter, message)
}
func isMCPTraceEnabled(config *ServerConfig) bool {
	// Check environment variable first (highest precedence)
	value := zqkenv.MCPTrace().Get()
	if value != emptyValue {
		switch strings.ToLower(value) {
		case "0", "false", "off", "no":
			return false
		default:
			return true
		}
	}

	// Fall back to config if env var not set
	if config != nil {
		return config.MCPServer.Trace.Enabled
	}

	return false
}

// systemLogger is a cached logger instance for trace error logging
// Initialized once using sync.Once to avoid repeated logger creation
var (
	systemLogger     logging.Logger
	systemLoggerOnce sync.Once
)

// getSystemLogger returns the cached system logger, initializing it once
func getSystemLogger() logging.Logger {
	systemLoggerOnce.Do(func() {
		systemLogger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	})
	return systemLogger
}

// logTraceError logs trace-related errors using the logging framework
// The logging framework automatically respects MCPServerContext.IsServing() to prevent stderr pollution
func logTraceError(msg string, err error, keyvals ...string) {
	// Use cached logger to avoid repeated initialization
	logger := getSystemLogger()

	// Convert keyvals to logging fields
	fields := make([]logging.Field, 0, len(keyvals)/2+1)
	for i := 0; i < len(keyvals); i += 2 {
		if i+1 < len(keyvals) {
			fields = append(fields, logging.String(keyvals[i], keyvals[i+1]))
		}
	}

	logging.Fluent(logger).Error(msg, err).WithFields(fields...).Log()
}

// rollingTraceConfigFromServerConfig returns rolling enabled flag and config with defaults applied.
// config may be nil (e.g. trace path only from env); rolling stays enabled with default sizes.
func rollingTraceConfigFromServerConfig(config *ServerConfig) (RollingTraceConfig, bool) {
	rollingEnabled := true
	if config != nil {
		if config.MCPServer.Trace.Rolling.Enabled != nil && !*config.MCPServer.Trace.Rolling.Enabled {
			rollingEnabled = false
		} else if config.MCPServer.Trace.Rolling.MaxSize > 0 || config.MCPServer.Trace.Rolling.MaxFiles > 0 {
			rollingEnabled = true
		}
	}
	rollingConfig := RollingTraceConfig{}
	if config != nil {
		rollingConfig.MaxSize = config.MCPServer.Trace.Rolling.MaxSize
		rollingConfig.MaxFiles = config.MCPServer.Trace.Rolling.MaxFiles
	}
	def := DefaultRollingTraceConfig()
	if rollingConfig.MaxSize <= 0 {
		rollingConfig.MaxSize = def.MaxSize
	}
	if rollingConfig.MaxFiles <= 0 {
		rollingConfig.MaxFiles = def.MaxFiles
	}
	return rollingConfig, rollingEnabled
}

// openTraceWriter opens a trace file writer or returns stderr
// Environment variable ZQK_MCP_TRACE_FILE takes precedence over config
// Relative paths are resolved relative to projectRoot (unless path is absolute)
func openTraceWriter(config *ServerConfig, projectRoot string) (io.Writer, io.Closer) {
	// Check environment variable first (highest precedence)
	path := zqkenv.MCPTraceFile().Get()

	// Fall back to config if env var not set
	if path == emptyValue && config != nil {
		path = config.MCPServer.Trace.File
	}

	// If still no path, write to stderr
	if path == emptyValue {
		return os.Stderr, nil
	}

	// Resolve relative paths relative to project root
	// Absolute paths (starting with /) are used as-is
	if !filepath.IsAbs(path) && projectRoot != emptyValue {
		path = filepath.Join(projectRoot, path)
	}

	rollingConfig, rollingEnabled := rollingTraceConfigFromServerConfig(config)
	if rollingEnabled {
		rollingWriter, err := NewRollingTraceWriter(path, rollingConfig)
		if err != nil {
			// Log error using helper that respects MCPServerContext.IsServing()
			logTraceError("Failed to create rolling trace writer", err, "trace_file", path)
			// Use logging framework to comply with POL-CODE-007
			logger := getSystemLogger()
			logging.Fluent(logger).Error("Failed to create rolling trace writer", err).
				TraceFile(path).
				EmitComponent("mcp_trace_init").
				Log()
			return os.Stderr, nil
		}
		// Verify the file was actually created
		if _, statErr := fileutil.Stat(path); statErr != nil {
			logTraceError("Trace file was not created after rolling writer initialization", statErr, "trace_file", path)
			logger := getSystemLogger()
			logging.Fluent(logger).Error("Trace file was not created after rolling writer initialization", statErr).
				TraceFile(path).
				EmitComponent("mcp_trace_init").
				Log()
		}
		return rollingWriter, rollingWriter
	}

	// Create directory if needed
	if err := fileutil.EnsureDir(filepath.Dir(path)); err != nil {
		// Log error using helper that respects MCPServerContext.IsServing()
		// This ensures errors are suppressed during active MCP serving to prevent stderr pollution
		logTraceError("Failed to create trace directory", err, "directory", filepath.Dir(path), "trace_file", path)
		return os.Stderr, nil
	}

	// Open trace file (append mode) - non-rolling
	f, err := fileutil.OpenFile(path, fileutil.O_CREATE|fileutil.O_WRONLY|fileutil.O_APPEND, paths.FilePerm600)
	if err != nil {
		// Log error using helper that respects MCPServerContext.IsServing()
		// This ensures errors are suppressed during active MCP serving to prevent stderr pollution
		logTraceError("Failed to open trace file", err, "trace_file", path)
		// Use logging framework to comply with POL-CODE-007
		logger := getSystemLogger()
		logging.Fluent(logger).Error("Failed to open trace file", err).
			TraceFile(path).
			EmitComponent("mcp_trace_init").
			Log()
		return os.Stderr, nil
	}
	// Verify the file was actually created and is writable
	if stat, statErr := f.Stat(); statErr != nil {
		_ = f.Close()
		logTraceError("Trace file stat failed after opening", statErr, "trace_file", path)
		logger := getSystemLogger()
		logging.Fluent(logger).Error("Trace file stat failed after opening", statErr).
			TraceFile(path).
			EmitComponent("mcp_trace_init").
			Log()
		return os.Stderr, nil
	} else {
		// Log successful trace file creation using logging framework to comply with POL-CODE-007
		logger := getSystemLogger()
		logging.Fluent(logger).Info("Trace logging initialized").
			TraceFile(path).
			Int("size", int(stat.Size())).
			EmitComponent("mcp_trace_init").
			Log()
	}
	return f, f
}

// openClientSpecificTraceWriter opens a client-specific trace file writer
// The filename is based on the client ID (with role prefix) for easy identification
// Format: mcp-trace-<client-id-with-role>.log
// Returns the writer and closer, or stderr if file creation fails
func (s *Server) openClientSpecificTraceWriter(clientIDWithRole string, config *ServerConfig, projectRoot string) (io.Writer, io.Closer) {
	// Get base trace file path from config or env var
	basePath := zqkenv.MCPTraceFile().Get()
	if basePath == emptyValue && config != nil {
		basePath = config.MCPServer.Trace.File
	}

	// If no base path configured, use default directory
	if basePath == emptyValue {
		if projectRoot != emptyValue {
			basePath = filepath.Join(projectRoot, paths.ProjectDataDir, paths.MCPDir, paths.MCPLogsDir, "mcp-trace.log")
		} else {
			// Fallback to temp dir to avoid polluting the current working directory (e.g., during tests)
			basePath = filepath.Join(os.TempDir(), "zqk-mcp-logs", "mcp-trace.log")
		}
	}

	// Extract directory from base path
	baseDir := filepath.Dir(basePath)
	if !filepath.IsAbs(baseDir) && projectRoot != emptyValue {
		baseDir = filepath.Join(projectRoot, baseDir)
	}

	// Sanitize client ID for use in filename (replace invalid characters)
	// Keep only alphanumeric, underscore, and hyphen
	sanitizedClientID := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_' // Replace invalid chars with underscore
	}, clientIDWithRole)

	// Create client-specific filename: mcp-trace-<client-id>.log
	clientTraceFile := fmt.Sprintf("mcp-trace-%s.log", sanitizedClientID)
	clientTracePath := filepath.Join(baseDir, clientTraceFile)

	rollingConfig, rollingEnabled := rollingTraceConfigFromServerConfig(config)
	if rollingEnabled {
		rollingWriter, err := NewRollingTraceWriter(clientTracePath, rollingConfig)
		if err != nil {
			logTraceError("Failed to create rolling client trace writer", err, "trace_file", clientTracePath)
			return os.Stderr, nil
		}
		return rollingWriter, rollingWriter
	}

	// Create directory if needed
	if err := fileutil.EnsureDir(baseDir); err != nil {
		logTraceError("Failed to create client trace directory", err, "directory", baseDir, "trace_file", clientTracePath)
		return os.Stderr, nil
	}

	// Open client-specific trace file (append mode) - non-rolling
	f, err := fileutil.OpenFile(clientTracePath, fileutil.O_CREATE|fileutil.O_WRONLY|fileutil.O_APPEND, paths.FilePerm600)
	if err != nil {
		logTraceError("Failed to open client-specific trace file", err, "trace_file", clientTracePath)
		return os.Stderr, nil
	}

	return f, f
}

// switchToClientSpecificTrace switches the trace writer to a client-specific trace file
// This is called when a client initializes to create separate trace logs per client
func (s *Server) switchToClientSpecificTrace(clientIDWithRole string) {
	// Get current config and project root
	config := s.config
	projectRoot := s.GetProjectRoot()

	// Close existing client-specific trace file if any
	_ = concurrency.RunInLockWithLogger(
		&s.traceCloserMu, LockNameMcpServerCloseTraceFile, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if s.traceCloser != nil {
				_ = s.traceCloser.Close() // Ignore errors on close
				s.traceCloser = nil
			}
			return nil
		},
	)

	// Open new client-specific trace file
	clientTraceWriter, clientTraceCloser := s.openClientSpecificTraceWriter(clientIDWithRole, config, projectRoot)

	// Update trace writer atomically
	_ = concurrency.RunInLockWithLogger(
		&s.traceWriterMu, LockNameMcpServerUpdateTraceWriter, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			s.traceWriter = clientTraceWriter
			return nil
		},
	)

	// Store closer for cleanup
	_ = concurrency.RunInLockWithLogger(
		&s.traceCloserMu, LockNameMcpServerStoreTraceCloser, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			s.traceCloser = clientTraceCloser
			return nil
		},
	)
}
