package mcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/metrics"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/dispatch"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/mcp"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/spf13/cobra"
)

// specLoaderAdapter adapts objects.SpecLoader to mcp.SpecLoader interface
type specLoaderAdapter struct {
	loader *objects.SpecLoader
}

// LoadSpecWithInheritance implements mcp.SpecLoader interface
func (a *specLoaderAdapter) LoadSpecWithInheritance(filename string) (mcp.Spec, error) {
	spec, err := a.loader.LoadSpecWithInheritance(filename)
	if err != nil {
		return nil, err
	}
	return &specAdapter{spec: spec}, nil
}

// specAdapter adapts objects.Spec to mcp.Spec interface
type specAdapter struct {
	spec *objects.Spec
}

// GetResolvedFields implements mcp.Spec interface
func (a *specAdapter) GetResolvedFields() map[string]any {
	return a.spec.ResolvedFields
}

// loggerAdapter adapts pkg/logging.Logger to mcp.Logger interface
type loggerAdapter struct {
	logger logging.Logger
}

// Debug implements mcp.Logger interface
func (la *loggerAdapter) Debug(msg string, fields ...mcp.LogField) {
	logFields := make([]logging.Field, len(fields))
	for i, f := range fields {
		logFields[i] = logging.Field{Key: f.Key, Value: f.Value}
	}
	entry := logging.Fluent(la.logger).Debug(msg)
	if len(logFields) > 0 {
		entry = entry.WithFields(logFields...)
	}
	entry.Log()
}

// NewMCPCmd creates the MCP command
func NewMCPCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"MCP server operations",
		"Manage and run the Model Context Protocol (MCP) server",
	).
		ExcludeCommonFlags()

	mcpCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewMcpCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use: "mcp",
	})

	helpBuilder.ApplyToCommand(mcpCmd)

	mcpCmd.AddCommand(NewServeCmd())
	mcpCmd.AddCommand(NewProxyCmd())
	mcpCmd.AddCommand(NewListToolsCmd())
	mcpCmd.AddCommand(NewInstallCmd())
	mcpCmd.AddCommand(NewDaemonCmd())

	return mcpCmd
}

var (
	tcpAddr string
	tlsCert string
	tlsKey  string
)

// NewServeCmd creates the serve command
func NewServeCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Start the MCP server",
		"Start the MCP server that exposes CLI commands via the Model Context Protocol.",
		"The server reads from stdin and writes to stdout using JSON-RPC protocol, or over TCP if specified.",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewMcpServeCommandBuilder(), &cobra.Command{
		Use: "serve",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// The timeout hook will read the MCP config directly via getMCPIdleTimeout()
			return nil
		},
		RunE: runServe,
	})

	cmd.Flags().StringVar(&tcpAddr, "tcp", "", "Start a TCP server at the given address instead of stdio (e.g., :8080)")
	cmd.Flags().StringVar(&tlsCert, "tls-cert", "", "Path to TLS certificate file (enables TLS if provided)")
	cmd.Flags().StringVar(&tlsKey, "tls-key", "", "Path to TLS key file")

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	return cmd
}

func runServe(cmd *cobra.Command, args []string) error {
	// Initialize CLI context - bundles all initialization state
	// No more one-off conditional checks - context always provides valid values
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")

	// Create MCP server
	server := mcp.NewServer()

	// NOTE: Tools are registered during initialize handler, not at startup.
	// This avoids blocking startup with slow resource discovery (240+ markdown files).
	// tools/list gracefully handles empty tools if called before initialize.

	// Get root command from the command's context
	// We need to access the root command to bootstrap CLI tools
	rootCmd := cmd.Root()

	// Set root command for CLI tool discovery
	server.SetRootCommand(rootCmd)
	// In-process CLI execution: use dispatch pool so multiple tool calls can run concurrently (pool cap);
	// rootCmd is shared so we serialize actual CLI execution with a mutex.
	cliExecMu := &sync.Mutex{}
	server.SetInProcessCLIRunner(newInProcessCLIRunner(rootCmd, initCtx.GetProjectRoot(), cliExecMu))
	// Start global dispatch pool for concurrent tool execution; stop when serve exits.
	dispatch.StartGlobalPool(context.Background())
	defer func() {
		dispatch.StopGlobalPool()
		dispatch.ClearInstanceContext()
	}()

	// Create security context (system context for MCP server)
	secCtx := pkgctx.GetSecurityContext(cmd.Context())
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	server.SetSecurityContext(secCtx)

	// Set CLI initialization context (always valid, no conditional check needed)
	server.SetCliInitializationContext(initCtx)

	// CRITICAL: Load config BEFORE bootstrapping CLI tools
	// This ensures tools are filtered by config security settings from the start
	// Without this, all commands would be registered when config is nil
	config, _ := mcp.LoadMCPConfig(initCtx.GetProjectRoot()) //nolint:errcheck // Ignore errors - use defaults if config missing
	server.SetConfig(config)

	// Bootstrap CLI tools is intentionally deferred to the Initialize handler
	// where it respects the register_cli_tools configuration flag.

	// Initialize logger for error reporting (before using it)
	// Use MCP-aware logging profile that automatically writes to stderr
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileMCP))

	// Initialize permission cache for spec-based access control
	// Create spec loader and adapter to avoid import cycles
	specLoader := objects.NewSpecLoader("")
	specLoaderAdapter := &specLoaderAdapter{loader: specLoader}

	// Create logger adapter for debug logging in permission system
	loggerAdapter := &loggerAdapter{logger: logger}

	permissionCache, specAccessControl, err := mcp.InitializePermissionCache(server, initCtx, specLoaderAdapter, loggerAdapter)
	if err != nil {
		// Log error but don't fail - permission cache will be built lazily
		logging.Fluent(logger).Warn("Failed to initialize permission cache").
			WithError(err).
			Log()
	} else {
		// Store permission cache and spec access control on server for format permission checking
		// These will be used to create MCPPermissionCheckerAdapter when needed
		_ = permissionCache   // Will be stored via SetPermissionCache
		_ = specAccessControl // Will be stored on server for format permission checking
	}

	// Bootstrap storage (and optionally configs) once via the dispatcher so one instance serves all
	// tool calls and in-process CLI; register in CLI cache so GetObjectStorageForCommand reuses it.
	projectRoot := initCtx.GetProjectRoot()
	var storageProvider storage.ObjectStorageProvider
	if projectRoot != emptyValue {
		bootstrapCtx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 30*time.Second)
		defer cancel()
		item := &dispatch.WorkItem{
			OperationID:   "bootstrap_storage_" + fmt.Sprintf("%d", time.Now().UnixNano()),
			OperationType: "bootstrap_storage",
			ProjectRoot:   projectRoot,
			Profile:       string(pkgctx.ProfileMCP),
		}
		runner := func(ctx context.Context) error {
			factory, err := storage.NewStorageFactory(ctx, projectRoot)
			if err != nil {
				return err
			}
			storageProvider = factory.GetStorage()
			if cli.OnStorageCreated != nil {
				cli.OnStorageCreated(storageProvider)
			}
			cli.RegisterStorageForProjectRoot(projectRoot, storageProvider)
			return nil
		}
		if err := dispatch.Run(bootstrapCtx, item, runner); err != nil {
			logging.Fluent(logger).Warn("Bootstrap storage failed, continuing without storage").
				WithError(err).
				Log()
		}
	}
	if storageProvider != nil {
		server.SetStorageProvider(NewStorageProviderAdapter(storageProvider))
		SetupMCPCoordinatorIntegration(server, projectRoot, storageProvider)
		mcp.RegisterAgentConnectionAccountCreation(server)

		// Subscribe to telemetry/metrics pipeline for synthesis_required events
		metricPipeline := metrics.MetricPipelineForProject(storageProvider, projectRoot)
		metricPipeline.AddSubscriber(func(event map[string]any) {
			eventKind, _ := event["event_kind"].(string)
			if eventKind == "synthesis_required" {
				sessionId, _ := event[objects.FieldKeySessionID].(string)
				rollupStatus, _ := event["rollup_status"].(string)

				// Broadcast via MCP notification
				if eventEmitter := server.GetEventEmitter(); eventEmitter != nil {
					_ = eventEmitter.Emit(&mcp.Event{
						Type:     mcp.EventTypeActionRequired,
						Message:  fmt.Sprintf("Synthesis required for session %s (status: %s)", sessionId, rollupStatus),
						Severity: "info",
						Fields: map[string]any{
							"synthesis_required":      true,
							objects.FieldKeySessionID: sessionId,
							"rollup_status":           rollupStatus,
						},
					})
				}
			}
		})
	}

	// One-stop-shop for expensive components: set instance context so handlers and in-process CLI
	// can access storage and registries from the same container (application container).
	ic := &dispatch.InstanceContext{
		Storage:     storageProvider,
		ProjectRoot: projectRoot,
		Profile:     "mcp",
	}
	if ic.Storage != nil {
		ic.SetRegistry(dispatch.KeySpecLoader, objects.GetGlobalSpecLoader())
		ic.SetRegistry(dispatch.KeyLifecycleLoader, objects.GetGlobalLifecycleLoader())
		ic.SetRegistry(dispatch.KeySynonymResolver, objects.GetGlobalSynonymResolver())
		ic.SetRegistry(dispatch.KeyKindMapper, objects.GetGlobalKindMapper())
		ic.SetRegistry(dispatch.KeyValidatorRegistry, validation.GetGlobalRegistry())
		ic.SetRegistry(dispatch.KeyFieldRegistry, objects.GetGlobalFieldRegistry())
	}
	dispatch.SetInstanceContext(ic)

	// Start serving (reads from stdin, writes to stdout)
	// Note: No startup logging to match legacy behavior (completely silent startup)
	// Optional: at debug level, log PID/PPID once so operators can correlate with ps output (see runbook).
	logging.Fluent(logger).Debug("MCP server process started").
		Int("pid", os.Getpid()).
		Int("ppid", os.Getppid()).
		Log()
	// IMPORTANT: Use explicit "mcp" profile, not environment-based detection.
	// The MCP server process itself should NOT have ZQK_MCP_ACCOUNT_ID set,
	// as that would cause environment variable pollution. Only subprocesses
	// spawned by ExecuteCLICommandViaMCP should have it set.
	//
	// The logging framework will automatically detect when we're actively serving
	// (via pkg/mcp.IsMCPServerServing()) and suppress all output during serving
	// to prevent stdout pollution (Cursor may merge stderr into stdout for stdio-based MCP).

	// Set up OS signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	goroutinelabels.NewGoroutine("mcp_signal_handler", "handling os signals for graceful shutdown").StartSimple(func() {
		sig := <-sigChan
		logging.Fluent(logger).Info("Received OS signal, initiating graceful shutdown").
			String("signal", sig.String()).
			Log()
		server.RequestShutdown(fmt.Sprintf("received OS signal: %v", sig))
	})

	// Wrap Serve() with panic recovery to prevent crashes from polluting stdout
	defer func() {
		if r := recover(); r != nil {
			// Log panic to stderr (not stdout)
			logging.Fluent(logger).Error("MCP server panic", errfmt.Errorf("panic: %v", r)).
				String("project_root", initCtx.GetProjectRoot()).
				Log()
			// Don't re-panic - let the server exit gracefully
			// The error will be returned and handled by cobra
		}
	}()

	if err := runServerMethod(server, initCtx, logger); err != nil {
		// In stdio MCP, EOF and normal disconnections are expected
		// Only exit with error code for truly unrecoverable errors
		// Most errors (EOF, connection closed, context cancellation) should exit cleanly (code 0)
		// This prevents Cursor from thinking the server crashed

		// Check if this is a normal disconnect (EOF)
		if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "EOF") {
			// Normal disconnect - exit cleanly without logging as error
			// This is expected behavior in stdio MCP when client disconnects
			return nil
		}

		// Check if this is a context cancellation (idle timeout) - treat as graceful shutdown
		// Check both error type and error message to catch all variations
		if errors.Is(err, context.DeadlineExceeded) ||
			errors.Is(err, context.Canceled) ||
			strings.Contains(err.Error(), "context canceled") ||
			strings.Contains(err.Error(), "context deadline exceeded") ||
			strings.Contains(err.Error(), "context cancelled") ||
			strings.Contains(err.Error(), "deadline exceeded") {
			// Idle timeout or context cancellation - graceful shutdown, exit cleanly
			// Log to file (not stdout/stderr) - logging framework automatically routes to file during MCP serving
			logging.Fluent(logger).Info("MCP server graceful shutdown: idle timeout detected in runServe").
				String("error", err.Error()).
				String("project_root", initCtx.GetProjectRoot()).
				Log()
			return nil
		}

		// This is a real error, not just a normal disconnect or timeout
		// Log to stderr (not stdout) and exit with error code
		logging.Fluent(logger).Error("MCP server error", err).
			String("project_root", initCtx.GetProjectRoot()).
			Log()
		// Note: os.Exit bypasses defer, but this is intentional for fatal errors
		os.Exit(1) //nolint:gocritic // exitAfterDefer - intentional for fatal errors
	}

	return nil
}

func runServerMethod(server *mcp.Server, initCtx *pkgctx.CliInitializationContext, logger logging.Logger) error {
	if tcpAddr != "" {
		if tlsCert != "" && tlsKey != "" {
			logging.Fluent(logger).Info("Starting MCP server on TCP with TLS").String("addr", tcpAddr).Log()
			return server.ServeTLS(tcpAddr, tlsCert, tlsKey)
		}
		logging.Fluent(logger).Info("Starting MCP server on TCP").String("addr", tcpAddr).Log()
		return server.ServeTCP(tcpAddr)
	}
	return server.Serve()
}

// newInProcessCLIRunner returns an MCP in-process CLI runner that uses the dispatch pool.
// Progress flows to the Coordinator so MCP clients get real-time progress. cliExecMu serializes
// access to the shared rootCmd so only one in-process CLI run executes at a time (pool still allows
// many concurrent tool submissions; the mutex is held only for the actual ExecuteContext).
func newInProcessCLIRunner(rootCmd *cobra.Command, projectRoot string, cliExecMu *sync.Mutex) mcp.InProcessCLIRunner {
	return func(ctx context.Context, commandPath string, cmdArgs []string) (any, error) {
		cliExecMu.Lock()
		defer cliExecMu.Unlock()
		var stdoutBuf, stderrBuf bytes.Buffer
		rootCmd.SetOut(&stdoutBuf)
		rootCmd.SetErr(&stderrBuf)
		rootCmd.SetArgs(cmdArgs)

		opType := strings.ReplaceAll(strings.TrimSpace(commandPath), " ", "_")
		if opType == emptyValue {
			opType = "cli"
		}
		item := &dispatch.WorkItem{
			OperationID:   fmt.Sprintf("%s_%d", opType, time.Now().UnixNano()),
			OperationType: opType,
			ProjectRoot:   projectRoot,
			Profile:       string(pkgctx.ProfileMCP),
		}
		runner := func(execCtx context.Context) error {
			execCtx = pkgctx.WithCommandOutputWriter(execCtx, &stdoutBuf)
			rootCmd.SetContext(execCtx)
			return rootCmd.ExecuteContext(execCtx)
		}
		execErr := dispatch.RunViaPool(ctx, item, runner)

		result := mcp.BuildCLICommandResultFromOutput(commandPath, cmdArgs, stdoutBuf.String(), stderrBuf.String(), execErr)
		return result, execErr
	}
}
