package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
)

// Server represents an MCP server instance
type Server struct {
	tools             map[string]Tool
	toolsMu           sync.RWMutex // Protects tools map for concurrent access
	toolsCache        atomic.Value // Cached immutable []Tool slice (lock-free reads)
	resources         map[string]Resource
	resourcesMu       sync.RWMutex // Protects resources map for concurrent access
	resourcesCache    atomic.Value // Cached immutable []Resource slice (lock-free reads)
	prompts           map[string]Prompt
	promptsMu         sync.RWMutex             // Protects prompts map for concurrent access
	promptsCache      atomic.Value             // Cached immutable []Prompt slice (lock-free reads)
	schemaHandlers    map[string]SchemaHandler // Schema URI pattern -> handler function
	schemaHandlersMu  sync.RWMutex             // Protects schemaHandlers map for concurrent access
	roots             []string
	shutdownRequested atomic.Bool
	shutdownReason    string
	shutdownFlag      atomic.Int32 // Atomic boolean: 0 = running, 1 = shutting down (thread-safe, lock-free)
	shutdownMu        sync.Mutex   // Only used for shutdownReason and shutdownRequested
	// multiClient is set by ServeTCP/TLS: one process serves many connections; disconnect must not reset shared tool/init state.
	multiClient atomic.Bool
	// Shutdown context and hook manager for graceful shutdown coordination
	// Components register hooks to be notified when shutdown is ordered
	// The shutdown context is cancelled when shutdown is ordered, allowing
	// components to check ctx.Done() and exit gracefully
	shutdownCtx         context.Context
	shutdownCancel      context.CancelFunc
	shutdownHookManager *ShutdownHookManager
	initialized         atomic.Bool
	// CLI bridge support
	rootCommand             any                              // *cobra.Command (avoid import cycle)
	inProcessCLIRunner      InProcessCLIRunner               // optional: run CLI in-process via dispatch (no subprocess)
	secCtxMu                sync.RWMutex
	secCtx                  any                              // *pkgctx.SecurityContext (avoid import cycle)
	requestsHandledTotal    atomic.Int64                     // Total requests handled by server
	serverErrorsTotal       atomic.Int64                     // Total server errors encountered
	shutdownsInitiatedTotal atomic.Int64                     // Total shutdowns initiated by server
	initCtx                 *pkgctx.CliInitializationContext // CLI initialization context (bundles project root and other state)
	cliToolPrefix           string                           // Prefix for CLI tools (e.g., "cli_")
	// Active serving state tracking removed - no longer needed
	// Async operation management
	operationTracker *OperationTracker
	asyncConfigMu    sync.RWMutex
	asyncConfig      AsyncHandlerConfig
	// Security configuration
	configMu sync.RWMutex
	config   *ServerConfig
	// Permission cache for spec-based access control
	permissionCache any // *PermissionCache (avoid import cycle)
	// Spec access control for format permission checking
	specAccessControl any // *SpecAccessControl (avoid import cycle)
	// Storage provider for loading MCP specs from system objects
	storageProvider StorageProvider // StorageProvider interface (avoids import cycle)
	// Allowed formats for this client (restricts output format)
	allowedFormats []string // Formats this client is allowed to use (empty = all allowed)
	// Event emitter for notifications/subscriptions
	eventEmitterMu sync.RWMutex
	eventEmitter   *EventEmitter
	// Transport context for event notifications (set during Serve)
	transportWriter *bufio.Writer
	transportFormat *MessageFormat
	transportMu     sync.RWMutex
	// Trace writer for logging outgoing notifications (set during Serve)
	traceWriter   io.Writer
	traceWriterMu sync.RWMutex
	// Trace file closer for client-specific trace files (needs to be closed on client disconnect)
	traceCloser   io.Closer
	traceCloserMu sync.RWMutex
	// Resource URI scheme resolver for configurable resource URI schemes
	resourceURISchemeResolver *ResourceURISchemeResolver
	// MIME adapter registry for extracting metadata from different content types
	mimeAdapterRegistry *ResourceMIMEAdapterRegistry
	// Client ID for notification routing (established on notifications/initialized)
	clientID   string
	clientIDMu sync.RWMutex
	// Client registry for routing messages to specific clients (future: multi-client support)
	// Currently stdio MCP is one-to-one, but this structure supports future expansion
	clients   map[string]*ClientConnection
	clientsMu sync.RWMutex
	// writerQueues: per-TCP-connection message queues (multi-client). Keyed by the
	// connection's bufio.Writer so RPC responses never share a dead clientID queue.
	writerQueues   map[*bufio.Writer]*MessageQueue
	writerQueuesMu sync.Mutex
	// writerSubscriptions: events/subscribe IDs bound to a connection Writer.
	// Cleared on disconnect so mcp_subscribers does not accumulate across reconnects.
	// TRACK: honest live subscriber count.
	writerSubscriptions   map[*bufio.Writer][]string
	writerSubscriptionsMu sync.Mutex
	// maxClients is from config (0 = unlimited). Enforced in registerNewClient to prevent resource exhaustion.
	maxClients int
	// Client metrics for tracking connection sequences
	clientMetricsStore  *ClientMetricsStore
	compressionTicker   *time.Ticker       // Ticker for periodic metrics compression (must be stopped on shutdown)
	compressionCtx      context.Context    // Context for compression goroutine lifecycle
	compressionCancel   context.CancelFunc // Cancel function for compression goroutine
	compressionTickerMu sync.Mutex         // Protects compressionTicker access
	currentSequenceID   string             // Current connection sequence ID
	sequenceIDMu        sync.RWMutex
	// Current MCP session object ID (e.g. MCP-001) for disconnect callback; set on init/session create, cleared on disconnect
	currentSessionID   string
	currentSessionIDMu sync.RWMutex
	// In-memory session activity lease (last_activity persist is throttled; see TouchSessionLastActivity)
	sessionActivityMu     sync.Mutex
	sessionLastActivityAt time.Time
	sessionLastPersistAt  time.Time
	// MCP protocol metrics
	mcpMetrics *MCPMetrics
	// Rate limiter for request throttling (BLI-645); nil when disabled
	rateLimiterMu sync.RWMutex
	rateLimiter   interface{ Allow(key string) bool }
	// Process group manager for tracking and controlling all goroutines and subprocesses
	// This ensures all spawned processes can be tracked and controlled during shutdown
	processGroupManager *ProcessGroupManager
	// Structural metrics enforcement per POL-DES-001
	// Dynamic metrics registry will be integrated in future release
	// - Track active client connections
	// - Implement connection cleanup for inactive clients

	// MCP Spec Diagnostics
	mcpSpecProvenance string
	mcpSpecConfigured atomic.Int32
	mcpSpecDefault    atomic.Int32

	serverInitOnce sync.Once
}

// GetServerStats returns lifetime counters for requests handled, server errors, and shutdowns initiated.
func (s *Server) GetServerStats() (requestsHandled, serverErrors, shutdownsInitiated int64) {
	if s == nil {
		return 0, 0, 0
	}
	return s.requestsHandledTotal.Load(), s.serverErrorsTotal.Load(), s.shutdownsInitiatedTotal.Load()
}

// Provenance values reported by GetMCPSpecDiagnostics, describing where the
// active MCP specs were loaded from.
const (
	MCPSpecProvenanceKernelStorage     = "kernel_storage"
	MCPSpecProvenanceFallbackFile      = "fallback_file"
	MCPSpecProvenanceBootstrapFallback = "bootstrap_fallback"
	MCPSpecProvenanceNone              = "none"
)

// SetMCPSpecDiagnostics sets the diagnostics for the loaded MCP specs.
func (s *Server) SetMCPSpecDiagnostics(provenance string, configured, defaultCount int) {
	s.mcpSpecProvenance = provenance
	s.mcpSpecConfigured.Store(int32(configured)) //nolint:gosec
	s.mcpSpecDefault.Store(int32(defaultCount))  //nolint:gosec
}

// GetMCPSpecDiagnostics returns the diagnostics for the loaded MCP specs.
func (s *Server) GetMCPSpecDiagnostics() (string, int, int) {
	return s.mcpSpecProvenance, int(s.mcpSpecConfigured.Load()), int(s.mcpSpecDefault.Load())
}

// IsInitialized returns true if the server is initialized (lock-free, thread-safe).
func (s *Server) IsInitialized() bool {
	if s == nil {
		return false
	}
	return s.initialized.Load()
}

// SetInitialized sets the initialized state (lock-free, thread-safe).
func (s *Server) SetInitialized(val bool) {
	if s != nil {
		s.initialized.Store(val)
	}
}

// NewServer creates a new MCP server
func NewServer() *Server {
	// Note: NewServer is called before command context exists (MCP server initialization)
	// Use system context for top-level component initialization
	shutdownCtx, shutdownCancel := context.WithCancel(pkgctx.NewSystemContext()) //nolint:gosec // G118: cancel stored on Server; invoked from Shutdown
	// Create process group manager with 5 second shutdown timeout
	// This allows critical operations to complete, but prevents hanging
	processGroupManager := NewProcessGroupManager(shutdownCtx, 5*time.Second)
	server := &Server{
		tools:            make(map[string]Tool),
		resources:        make(map[string]Resource),
		prompts:          make(map[string]Prompt),
		schemaHandlers:   make(map[string]SchemaHandler),
		roots:            []string{},
		cliToolPrefix:    DefaultCLIToolPrefix,
		operationTracker: NewOperationTracker(),
		asyncConfig: AsyncHandlerConfig{
			MaxConcurrent: DefaultMaxConcurrentOperations,
			Timeout:       DefaultOperationTimeout,
		},
		eventEmitter:              NewEventEmitter(DefaultEventBufferSize),
		clients:                   make(map[string]*ClientConnection),
		writerQueues:              make(map[*bufio.Writer]*MessageQueue),
		writerSubscriptions:       make(map[*bufio.Writer][]string),
		mcpMetrics:                NewMCPMetrics(),
		resourceURISchemeResolver: NewResourceURISchemeResolver(), // Will be updated from config if available
		mimeAdapterRegistry:       NewResourceMIMEAdapterRegistry(),
		shutdownCtx:               shutdownCtx,
		shutdownCancel:            shutdownCancel,
		shutdownHookManager:       NewShutdownHookManager(),
		processGroupManager:       processGroupManager,
	}
	return server
}

// SetAsyncConfig configures async operation handling (thread-safe)
func (s *Server) SetAsyncConfig(config AsyncHandlerConfig) {
	s.asyncConfigMu.Lock()
	defer s.asyncConfigMu.Unlock()
	s.asyncConfig = config
}

// GetAsyncConfig returns the async operation configuration (thread-safe)
func (s *Server) GetAsyncConfig() AsyncHandlerConfig {
	s.asyncConfigMu.RLock()
	defer s.asyncConfigMu.RUnlock()
	return s.asyncConfig
}

// SetRootCommand sets the root CLI command for automatic tool discovery
// This enables context-driven CLI command exposure via MCP
func (s *Server) SetRootCommand(rootCommand any) {
	s.rootCommand = rootCommand
}

// InProcessCLIRunner runs a CLI command in-process (same process as the MCP server) via the dispatch loop.
// When set, CLI tool calls use this instead of a subprocess, so progress flows to the Coordinator and
// MCP clients can receive real-time progress. Signature: (ctx, commandPath, cmdArgs) -> (result, error).
type InProcessCLIRunner func(ctx context.Context, commandPath string, cmdArgs []string) (any, error)

// SetInProcessCLIRunner sets the optional in-process CLI runner. When non-nil, executeCLICommandWithContextForServer
// uses it instead of spawning a subprocess. Call from the host (e.g. cmd/zqk) after SetRootCommand so the
// runner can use the same root command and dispatch.Run. See docs/architecture/DISPATCH_LOOP_AND_MCP.md.
func (s *Server) SetInProcessCLIRunner(runner InProcessCLIRunner) {
	s.inProcessCLIRunner = runner
}

// GetInProcessCLIRunner returns the current in-process CLI runner, if any.
func (s *Server) GetInProcessCLIRunner() InProcessCLIRunner {
	return s.inProcessCLIRunner
}

// GetEventEmitter returns the server's event emitter (thread-safe)
func (s *Server) GetEventEmitter() *EventEmitter {
	s.eventEmitterMu.RLock()
	defer s.eventEmitterMu.RUnlock()
	return s.eventEmitter
}

// SetEventEmitter sets the server's event emitter (thread-safe)
func (s *Server) SetEventEmitter(ee *EventEmitter) {
	s.eventEmitterMu.Lock()
	defer s.eventEmitterMu.Unlock()
	s.eventEmitter = ee
}

// GetRateLimiter returns the rate limiter (thread-safe)
func (s *Server) GetRateLimiter() interface{ Allow(key string) bool } {
	s.rateLimiterMu.RLock()
	defer s.rateLimiterMu.RUnlock()
	return s.rateLimiter
}

// SetRateLimiter sets the rate limiter (thread-safe)
func (s *Server) SetRateLimiter(rl interface{ Allow(key string) bool }) {
	s.rateLimiterMu.Lock()
	defer s.rateLimiterMu.Unlock()
	s.rateLimiter = rl
}

// SetSecurityContext sets the security context for permission-based tool filtering (thread-safe)
func (s *Server) SetSecurityContext(secCtx any) {
	s.secCtxMu.Lock()
	defer s.secCtxMu.Unlock()
	s.secCtx = secCtx
}

// GetSecurityContext returns the security context (thread-safe)
func (s *Server) GetSecurityContext() any {
	s.secCtxMu.RLock()
	defer s.secCtxMu.RUnlock()
	return s.secCtx
}

// SetStorageProvider sets the storage provider for loading MCP specs from system objects
// This allows the server to discover and load specs using existing CLI/storage infrastructure
// instead of hardcoding file paths
func (s *Server) SetStorageProvider(provider StorageProvider) {
	s.storageProvider = provider
}

// getClientIDWithRole returns the client ID prefixed with the role(s) for trace logging
// Format: <role>_<client_id> or <role1>_<role2>_<client_id> for multiple roles
// Falls back to just client_id if no roles are available
// Note: If the clientID already has a role prefix (from generation), it returns it as-is
func (s *Server) getClientIDWithRole() string {
	var clientID string
	_ = concurrency.RunInRLockWithLogger(
		&s.clientIDMu, LockNameMcpServerGetClientId, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			clientID = s.clientID
			return nil
		},
	)

	if clientID == emptyValue {
		return ""
	}

	// Check if clientID already has a role prefix (starts with role_ or contains role_client_)
	// If it does, return it as-is (it was generated with the role prefix)
	if strings.Contains(clientID, "_client_") && !strings.HasPrefix(clientID, "client_") {
		// Already has role prefix, return as-is
		return clientID
	}

	// Get roles from security context
	var roles []string
	if secCtx := s.GetSecurityContext(); secCtx != nil {
		if sc, ok := secCtx.(interface{ GetRoles() []string }); ok {
			roles = sc.GetRoles()
		}
	}

	// If no roles, return client ID as-is
	if len(roles) == 0 {
		return clientID
	}

	// Prefix with role(s) - join multiple roles with underscore
	rolePrefix := strings.Join(roles, "_") + "_"
	return fmt.Sprintf("%s_%s", rolePrefix, clientID)
}

// getRateLimitKey returns the key for rate limiting: account ID when per-account and available, else "global".
func (s *Server) getRateLimitKey(perAccount bool) string {
	if !perAccount {
		return "global"
	}
	secCtx := s.GetSecurityContext()
	if secCtx == nil {
		return "global"
	}
	if sc, ok := secCtx.(*pkgctx.SecurityContext); ok && sc.AccountID != emptyValue {
		return sc.AccountID
	}
	return "global"
}

// GetCallerAccountID returns the account ID of the current MCP caller, or empty
// if no security context is set. Used by chat and audit tools to stamp agent
// identity on events.
func (s *Server) GetCallerAccountID() string {
	secCtx := s.GetSecurityContext()
	if secCtx == nil {
		return ""
	}
	if sc, ok := secCtx.(*pkgctx.SecurityContext); ok && sc.AccountID != emptyValue {
		return sc.AccountID
	}
	return ""
}

// SetTraceWriter sets the trace writer for the server
func (s *Server) SetTraceWriter(w io.Writer) {
	_ = concurrency.RunInLock(&s.traceWriterMu, func() error {
		s.traceWriter = w
		return nil
	})
}

// SetConfig sets the MCP server configuration and applies resource URI scheme rules
// This should be called before BootstrapCLITools() to ensure proper filtering
func (s *Server) SetConfig(config *ServerConfig) {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	s.config = config

	// Update resource URI scheme resolver with config rules
	if config != nil {
		s.resourceURISchemeResolver = NewResourceURISchemeResolverFromConfig(config)
		s.maxClients = config.MCPServer.MaxClients // 0 = unlimited
	}
}

// GetConfig returns the MCP server configuration (thread-safe)
func (s *Server) GetConfig() *ServerConfig {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	return s.config
}

// GetClientMetricsStore returns the client metrics store (thread-safe)
func (s *Server) GetClientMetricsStore() *ClientMetricsStore {
	s.compressionTickerMu.Lock()
	defer s.compressionTickerMu.Unlock()
	return s.clientMetricsStore
}

// SetProjectRoot sets the project root for CLI command execution
//
// Deprecated: Use SetCliInitializationContext instead
func (s *Server) SetProjectRoot(projectRoot string) {
	// Create a minimal context for backward compatibility
	initCtx := &pkgctx.CliInitializationContext{
		ProjectRoot: projectRoot,
	}
	s.initCtx = initCtx
}

// SetCliInitializationContext sets the CLI initialization context
// This follows the context pattern - always pass context objects, never extract values
func (s *Server) SetCliInitializationContext(initCtx *pkgctx.CliInitializationContext) {
	s.initCtx = initCtx
}

// GetCliInitializationContext returns the CLI initialization context
func (s *Server) GetCliInitializationContext() *pkgctx.CliInitializationContext {
	return s.initCtx
}

// GetProjectRoot returns the project root from the initialization context
// Always returns a valid value (never empty, defaults to ".")
func (s *Server) GetProjectRoot() string {
	if s.initCtx != nil {
		return s.initCtx.GetProjectRoot()
	}
	return "." // Default fallback
}

// SetPermissionCache sets the permission cache for spec-based access control
func (s *Server) SetPermissionCache(cache any) {
	s.permissionCache = cache
}

// SetSpecAccessControl sets the spec access control on the server
func (s *Server) SetSpecAccessControl(sac any) {
	s.specAccessControl = sac
}

// SetAllowedFormats sets the allowed output formats for this client
// If empty, all formats are allowed. If non-empty, only listed formats are allowed.
func (s *Server) SetAllowedFormats(formats []string) {
	s.allowedFormats = formats
}

// GetAllowedFormats returns the allowed output formats for this client
func (s *Server) GetAllowedFormats() []string {
	return s.allowedFormats
}

// CheckFormatPermission checks if a format is allowed for the current security context
// This is a helper that can be called before command execution (like dry-run)
// It checks both client-level format restrictions and permission-based restrictions
func (s *Server) CheckFormatPermission(ctx context.Context, format string) (bool, string) {
	// Get permission cache and security context
	permissionCache := s.permissionCache
	secCtx := s.secCtx

	secCtxTyped, ok := nildecode.DecodeNonNilPayload[*pkgctx.SecurityContext](secCtx)
	if !ok {
		return false, "no security context"
	}

	// Check if user is active
	if permissionCache != nil {
		if pc, ok := permissionCache.(*PermissionCache); ok {
			if !pc.IsUserActive(secCtxTyped.AccountID) {
				return false, "user is inactive"
			}
		}
	}

	// Check client-level format restrictions first (most restrictive)
	if len(s.allowedFormats) > 0 {
		if !slices.Contains(s.allowedFormats, format) {
			return false, fmt.Sprintf("format '%s' not allowed for this client (allowed: %v)", format, s.allowedFormats)
		}
	}

	// Admin can use any format (if not restricted by client-level restrictions)
	if slices.Contains(secCtxTyped.Roles, "admin") {
		return true, ""
	}

	// Streaming formats (json-rpc, stream) may require special permissions
	if format == "json-rpc" || format == "stream" {
		// Check for streaming permission
		hasStreamingPerm := slices.Contains(secCtxTyped.Permissions, "stream:*") ||
			slices.Contains(secCtxTyped.Permissions, "format:json-rpc") ||
			slices.Contains(secCtxTyped.Permissions, "format:stream")

		// If no explicit streaming permission, check if user has read:* (default allow)
		if !hasStreamingPerm {
			hasStreamingPerm = slices.Contains(secCtxTyped.Permissions, "read:*")
		}

		if !hasStreamingPerm {
			return false, "streaming format requires stream:* or read:* permission"
		}
	}

	// All other formats are allowed by default (table, json, yaml)
	return true, ""
}

// HandleToolCall is a public wrapper for handleToolCallWithContext
// This allows test harnesses and other code to call tools on the server
func (s *Server) HandleToolCall(ctx context.Context, name string, args map[string]any) (any, error) {
	s.requestsHandledTotal.Add(1)
	ctx = ExtractActorContextFromArgs(ctx, args)
	res, err := s.handleToolCallWithContext(ctx, name, args)
	if err != nil {
		s.serverErrorsTotal.Add(1)
	}
	return res, err
}

// ensureServerInitialized performs server-level lifecycle setup once per server instance.
// This loads configuration, specs, metrics store, and trace logging safely without per-connection races.
func (s *Server) ensureServerInitialized() {
	s.serverInitOnce.Do(func() {
		NewServerLifecycleBuilder(s).
			LoadConfig().
			ApplyAsyncConfig().
			ApplyEventEmitterConfig().
			ApplyRateLimitConfig().
			InitializeClientMetrics().
			InitializeTraceLogging().
			MarkServing().
			LoadMCPSpecs().
			Build()
	})
}

// Serve starts the MCP server and handles requests from stdin
// The server will gracefully shutdown if the context is cancelled (e.g., due to idle timeout)
func (s *Server) Serve() error {
	s.ensureServerInitialized()

	// Build connection-scoped lifecycle for stdio
	lifecycle := NewServerLifecycleBuilder(s).
		SetupTransport().
		SetupHandlers().
		Build()

	defer lifecycle.Cleanup()

	// Create message processor
	processor := NewMessageProcessor(s, lifecycle.GetHandler(), lifecycle.GetTransport())

	// Create serve coordinator
	coordinator := NewServeCoordinator(s, processor, lifecycle)

	// Run main serve loop
	return coordinator.ServeLoop()
}

// IsShutdownRequested returns whether a shutdown has been requested on the server
func (s *Server) IsShutdownRequested() bool {
	return s.shutdownRequested.Load()
}

// GetMCPMetrics returns the MCP protocol metrics
func (s *Server) GetMCPMetrics() *MCPMetrics {
	return s.mcpMetrics
}

// GetMCPMetricsSnapshot returns a snapshot of all MCP metrics
func (s *Server) GetMCPMetricsSnapshot() MetricsSnapshot {
	return s.mcpMetrics.GetSnapshot()
}

// negotiateProtocolVersion negotiates the MCP protocol version
// Echoes back the client's protocol version if provided, otherwise uses default
func negotiateProtocolVersion(clientVersion string) string {
	if clientVersion != emptyValue {
		return clientVersion
	}
	return "2025-06-18" // Default MCP protocol version
}

// getClientEventContext returns a ClientEventContext for the current server state
// This provides a clean way to record events without repeating sequenceID/clientID retrieval
func (s *Server) getClientEventContext() *ClientEventContext {
	return NewClientEventContext(s)
}

// GetCurrentSessionID returns the current MCP session object ID (e.g. MCP-001), or empty if none.
func (s *Server) GetCurrentSessionID() string {
	var id string
	_ = concurrency.RunInRLockWithLogger(
		&s.currentSessionIDMu, LockNameMcpServerGetCurrentSessionId, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			id = s.currentSessionID
			return nil
		},
	)
	return id
}

// SetCurrentSessionID sets the current MCP session object ID (e.g. MCP-001). Call when a session is created or re-established.
func (s *Server) SetCurrentSessionID(sessionID string) {
	if sessionID == emptyValue {
		return
	}
	_ = concurrency.RunInLockWithLogger(
		&s.currentSessionIDMu, LockNameMcpServerSetCurrentSessionId, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			s.currentSessionID = sessionID
			return nil
		},
	)
}

// ClearCurrentSessionID clears the current MCP session object ID. Call on disconnect so retention can archive/delete the session.
func (s *Server) ClearCurrentSessionID() {
	_ = concurrency.RunInLockWithLogger(
		&s.currentSessionIDMu, LockNameMcpServerClearCurrentSessionId, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			s.currentSessionID = emptyValue
			return nil
		},
	)
	_ = concurrency.RunInLockWithLogger(
		&s.sessionActivityMu, LockNameMcpServerSessionActivity, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			s.sessionLastActivityAt = time.Time{}
			s.sessionLastPersistAt = time.Time{}
			return nil
		},
	)
}

// recordClientEvent records a client sequence event for metrics
func (s *Server) recordClientEvent(sequenceID, clientID, eventType string, fields map[string]any) {
	store := s.GetClientMetricsStore()
	if store == nil {
		return // Metrics not initialized
	}
	// Record asynchronously to avoid blocking
	// CRITICAL: Use process group manager to track this goroutine
	// This ensures it can be controlled during shutdown
	_, _ = s.processGroupManager.SpawnGoroutine(
		fmt.Sprintf("metrics-record-%s-%s", clientID, eventType),
		"Metrics Recorder",
		fmt.Sprintf("Records %s event for client %s", eventType, clientID),
		false, // Not critical - can be cancelled during shutdown
		func(ctx context.Context) {
			_ = store.RecordEvent(sequenceID, clientID, eventType, fields) //nolint:errcheck // Metrics recording errors are non-critical
		},
	)
}

// GetConnectedClients returns a list of all currently connected client IDs
func (s *Server) GetConnectedClients() []string {
	var clients []string
	_ = concurrency.RunInRLockWithLogger(
		&s.clientsMu, "mcp_server_get_connected_clients", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			clients = make([]string, 0, len(s.clients))
			for id := range s.clients {
				clients = append(clients, id)
			}
			return nil
		},
	)
	return clients
}

// SendNotificationToClient sends a notification event directly to a specific connected client
func (s *Server) SendNotificationToClient(clientID string, method string, params any) error {
	var conn *ClientConnection
	_ = concurrency.RunInRLockWithLogger(
		&s.clientsMu, "mcp_server_send_notification", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if c, ok := s.clients[clientID]; ok {
				conn = c
			}
			return nil
		},
	)

	if conn == nil {
		return fmt.Errorf("client %s not connected", clientID)
	}

	notification := map[string]any{
		"jsonrpc":              "2.0",
		objects.FieldKeyMethod: method,
		"params":               params,
	}

	msgBytes, err := json.Marshal(notification)
	if err != nil {
		return err
	}

	if !conn.Queue.Enqueue(msgBytes, conn.Format, "high", nil) {
		return fmt.Errorf("client %s message queue full", clientID)
	}

	return nil
}
