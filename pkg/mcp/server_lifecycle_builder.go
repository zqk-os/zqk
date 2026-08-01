package mcp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/lanceman/zqk/pkg/circuitbreaker"
	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// ServerLifecycleBuilder provides a fluent API for building and managing server lifecycle
// This manages thread-safety concerns and provides logical structure for server operations
type ServerLifecycleBuilder struct {
	server      *Server
	config      *ServerConfig
	trace       bool
	traceWriter io.Writer
	traceCloser io.Closer
	reader      *bufio.Reader
	writer      *bufio.Writer
	transport   Transport
	handler     Handler
	baseCtx     context.Context
	mcpCtx      *pkgctx.MCPServerContext
	initialized bool
}

// NewServerLifecycleBuilder creates a new server lifecycle builder
func NewServerLifecycleBuilder(server *Server) *ServerLifecycleBuilder {
	return &ServerLifecycleBuilder{
		server:      server,
		reader:      bufio.NewReader(os.Stdin),
		writer:      bufio.NewWriter(os.Stdout),
		initialized: false,
	}
}

// WithReaderAndWriter overrides the default stdio reader and writer
func (b *ServerLifecycleBuilder) WithReaderAndWriter(r io.Reader, w io.Writer) *ServerLifecycleBuilder {
	b.reader = bufio.NewReader(r)
	b.writer = bufio.NewWriter(w)
	return b
}

// LoadConfig loads and applies MCP configuration
func (b *ServerLifecycleBuilder) LoadConfig() *ServerLifecycleBuilder {
	config, _ := LoadMCPConfig(b.server.GetProjectRoot()) // Ignore errors - use defaults
	b.config = config
	b.server.config = config // Store config for security enforcement
	return b
}

// ApplyAsyncConfig applies async configuration from config file
func (b *ServerLifecycleBuilder) ApplyAsyncConfig() *ServerLifecycleBuilder {
	if b.config != nil && b.config.MCPServer.Async.MaxConcurrent > 0 {
		b.server.asyncConfig.MaxConcurrent = b.config.MCPServer.Async.MaxConcurrent
	}
	if b.config != nil && b.config.MCPServer.Async.Timeout != emptyValue {
		if timeout, err := time.ParseDuration(b.config.MCPServer.Async.Timeout); err == nil {
			b.server.asyncConfig.Timeout = timeout
			// Log timeout configuration for debugging
			if b.server.getTraceWriter() != nil {
				b.server.traceLogf("[MCP_INFO] Async operation timeout configured: %v", timeout)
			}
		} else {
			// Log error if timeout parsing fails
			if b.server.getTraceWriter() != nil {
				b.server.traceLogf("[MCP_WARN] Failed to parse async timeout '%s': %v, using default", b.config.MCPServer.Async.Timeout, err)
			}
		}
	} else {
		// Log default timeout if not configured
		if b.server.getTraceWriter() != nil {
			b.server.traceLogf("[MCP_DEBUG] Using default async operation timeout: %v", b.server.asyncConfig.Timeout)
		}
	}
	return b
}

// ApplyEventEmitterConfig applies event emitter configuration from config file
func (b *ServerLifecycleBuilder) ApplyEventEmitterConfig() *ServerLifecycleBuilder {
	if b.config != nil && b.config.MCPServer.Events.BufferSize > 0 {
		if b.server.eventEmitter == nil || b.server.eventEmitter.GetBufferSize() != b.config.MCPServer.Events.BufferSize {
			b.server.eventEmitter = NewEventEmitter(b.config.MCPServer.Events.BufferSize)
		}
	}
	return b
}

// ApplyRateLimitConfig applies rate limit configuration (ITEM-645). When enabled, creates a fixed-window limiter.
func (b *ServerLifecycleBuilder) ApplyRateLimitConfig() *ServerLifecycleBuilder {
	if b.config == nil || !b.config.MCPServer.RateLimit.Enabled {
		b.server.rateLimiter = nil
		return b
	}
	rpm := b.config.MCPServer.RateLimit.RequestsPerMinute
	if rpm <= 0 {
		rpm = 60
	}
	b.server.rateLimiter = circuitbreaker.NewFixedWindowLimiter(rpm, time.Minute)
	return b
}

// InitializeClientMetrics initializes client metrics store with thread-safe operations
func (b *ServerLifecycleBuilder) InitializeClientMetrics() *ServerLifecycleBuilder {
	if b.server.clientMetricsStore == nil {
		metricsPath := filepath.Join(b.server.GetProjectRoot(), paths.ProjectDataDir, paths.MCPDir, paths.MCPLogsDir, "client-metrics.json")
		// Pass shutdown context from ProcessGroupManager for proper shutdown handling
		shutdownCtx := b.server.processGroupManager.GetShutdownContext()
		store, _ := NewClientMetricsStore(metricsPath, shutdownCtx) //nolint:errcheck // Metrics failures shouldn't block server startup
		b.server.clientMetricsStore = store

		// Start save worker via ProcessGroupManager (CRITICAL - must complete saves during shutdown)
		store.StartSaveWorker(b.server.processGroupManager)

		// Start periodic compression with proper lifecycle management
		compressionCtx, compressionCancel := context.WithCancel(pkgctx.NewSystemContext()) //nolint:gosec // G118: cancel stored on server; stopped with compression lifecycle
		ticker := store.StartPeriodicCompression(compressionCtx, DefaultMetricsCompressionInterval, DefaultMetricsRetentionPeriod)

		// Thread-safe update of compression state
		_ = concurrency.RunInLockWithLogger(
			&b.server.compressionTickerMu, LockNameMcpServerLifecycleSetCompression, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				b.server.compressionTicker = ticker
				b.server.compressionCtx = compressionCtx
				b.server.compressionCancel = compressionCancel
				return nil
			},
		)
	}
	return b
}

// InitializeTraceLogging initializes trace logging with thread-safe operations
func (b *ServerLifecycleBuilder) InitializeTraceLogging() *ServerLifecycleBuilder {
	b.trace = isMCPTraceEnabled(b.config)
	b.traceWriter = io.Writer(os.Stderr)

	if b.trace {
		b.traceWriter, b.traceCloser = openTraceWriter(b.config, b.server.GetProjectRoot())
		if b.traceCloser != nil {
			// Closer will be handled by Cleanup
		}
		// Thread-safe update of trace writer
		_ = concurrency.RunInLockWithLogger(
			&b.server.traceWriterMu, LockNameMcpServerLifecycleSetTraceWriter, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				b.server.traceWriter = b.traceWriter
				return nil
			},
		)

		// Test write to verify trace file is actually writable
		if b.traceWriter != nil && b.traceWriter != os.Stderr {
			testMsg := fmt.Sprintf("[%s] [MCP_INFO] Trace logging initialized successfully\n", zqktime.NowLayoutUTC(zqktime.LayoutDateTimeMillis))
			if _, err := b.traceWriter.Write([]byte(testMsg)); err != nil {
				// Write failed - use logging framework to comply with POLICY-CODE-007
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				logging.Fluent(logger).Error("Trace file write test failed, falling back to stderr", err).
					EmitComponent("mcp_trace_init").
					Log()
				_ = concurrency.RunInLockWithLogger(
					&b.server.traceWriterMu, LockNameMcpServerLifecycleFallbackTraceWriter, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
					func() error {
						b.server.traceWriter = os.Stderr
						return nil
					},
				)
			}
		}
	} else {
		// Even when trace is disabled, set stderr as the writer so traceLogf doesn't return early
		// This ensures ERROR and WARN messages still get written to stderr
		_ = concurrency.RunInLockWithLogger(
			&b.server.traceWriterMu, LockNameMcpServerLifecycleSetStderrTrace, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				b.server.traceWriter = os.Stderr
				return nil
			},
		)
	}
	return b
}

// MarkServing marks the server as actively serving with proper context management
func (b *ServerLifecycleBuilder) MarkServing() *ServerLifecycleBuilder {
	mcpCtx := pkgctx.GetMCPServerContext()
	mcpCtx.SetServing(true)
	b.mcpCtx = mcpCtx
	return b
}

// LoadMCPSpecs loads and applies MCP specs from storage or file system
// This allows externalizing MCP configuration (prompts, resources, tools)
func (b *ServerLifecycleBuilder) LoadMCPSpecs() *ServerLifecycleBuilder {
	projectRoot := b.server.GetProjectRoot()
	if projectRoot == emptyValue {
		return b // No project root - skip spec loading
	}

	// Try loading from storage first (specs as system objects)
	// This allows specs to be managed via CLI
	if b.server.storageProvider != nil {
		ctx := pkgctx.NewSystemContext()
		secCtx := pkgctx.NewSystemSecurityContext()
		loader := NewStorageMCPSpecLoader(b.server.storageProvider)
		specs, err := loader.LoadMCPSpecs(ctx, secCtx)
		if err == nil && len(specs) > 0 {
			generator := NewMCPSpecGenerator(b.server)
			if err := generator.GenerateFromSpecs(specs); err == nil {
				// Specs loaded from storage successfully
				return b
			}
		}
	}

	// Fallback: Load from file system (.zqk/mcp/specs/)
	specsDir := filepath.Join(projectRoot, paths.ProjectDataDir, "mcp", "specs")
	if _, err := os.Stat(specsDir); err == nil {
		// Specs directory exists - try to load all YAML files
		entries, err := os.ReadDir(specsDir)
		if err == nil {
			loader := NewMCPSpecLoader()
			generator := NewMCPSpecGenerator(b.server)
			for _, entry := range entries {
				if !entry.IsDir() && (filepath.Ext(entry.Name()) == ".yaml" || filepath.Ext(entry.Name()) == ".yml") {
					specPath := filepath.Join(specsDir, entry.Name())
					specs, err := loader.LoadSpecs(specPath)
					if err == nil && len(specs) > 0 {
						if err := generator.GenerateFromSpecs(specs); err != nil {
							// Log warning but don't fail server startup
							// Spec loading is optional enhancement
						}
					}
				}
			}
		}
	}

	return b
}

// SetupTransport sets up the transport layer
func (b *ServerLifecycleBuilder) SetupTransport() *ServerLifecycleBuilder {
	b.transport = NewDefaultTransport()
	return b
}

// SetupHandlers sets up method router and handlers with middleware
func (b *ServerLifecycleBuilder) SetupHandlers() *ServerLifecycleBuilder {
	router := b.server.setupHandlers()
	asyncHandler := NewAsyncHandler(router, b.server.asyncConfig)

	var handler Handler = asyncHandler
	if b.trace {
		handler = TraceMiddleware(func() io.Writer {
			return b.server.getTraceWriter()
		})(handler)
	}

	b.handler = handler
	return b
}

// Build completes the lifecycle setup and returns the configured builder
func (b *ServerLifecycleBuilder) Build() *ServerLifecycleBuilder {
	b.initialized = true
	return b
}

// Cleanup performs cleanup operations (deferred from Serve)
func (b *ServerLifecycleBuilder) Cleanup() {
	if b.traceCloser != nil {
		_ = b.traceCloser.Close() //nolint:errcheck
	}
	if b.mcpCtx != nil {
		b.mcpCtx.SetServing(false)
	}
}

// GetReader returns the input reader
func (b *ServerLifecycleBuilder) GetReader() *bufio.Reader {
	return b.reader
}

// GetWriter returns the output writer
func (b *ServerLifecycleBuilder) GetWriter() *bufio.Writer {
	return b.writer
}

// GetTransport returns the transport
func (b *ServerLifecycleBuilder) GetTransport() Transport {
	return b.transport
}

// GetHandler returns the configured handler
func (b *ServerLifecycleBuilder) GetHandler() Handler {
	return b.handler
}

// GetBaseContext returns the base context (never nil; uses NewSystemContext() if unset)
func (b *ServerLifecycleBuilder) GetBaseContext() context.Context {
	if b.baseCtx != nil {
		return b.baseCtx
	}
	return pkgctx.NewSystemContext()
}

// IsTraceEnabled returns whether trace logging is enabled
func (b *ServerLifecycleBuilder) IsTraceEnabled() bool {
	return b.trace
}

// GetTraceWriter returns the trace writer
func (b *ServerLifecycleBuilder) GetTraceWriter() io.Writer {
	return b.traceWriter
}

// GetConfig returns the loaded config
func (b *ServerLifecycleBuilder) GetConfig() *ServerConfig {
	return b.config
}
