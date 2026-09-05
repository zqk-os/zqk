package mcp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"io"
	"os"
	"sync/atomic"
)

// ServeCoordinator coordinates the main serve loop with thread-safe operations
// This manages timeout detection, message reading, and shutdown handling

// TimeoutManager defines the interface for managing server timeouts
type TimeoutManager interface {
	GetServerContext() context.Context
	ResetTimeout()
}

type ServeCoordinator struct {
	timeoutManager        TimeoutManager
	server                *Server
	processor             *MessageProcessor
	lifecycle             *ServerLifecycleBuilder
	loopIterationsTotal   atomic.Int64
	timeoutShutdownsTotal atomic.Int64
}

// GetServeCoordinatorStats returns lifetime counters for loop iterations and timeout shutdowns.
func (sc *ServeCoordinator) GetServeCoordinatorStats() (iterations, timeouts int64) {
	if sc == nil {
		return 0, 0
	}
	return sc.loopIterationsTotal.Load(), sc.timeoutShutdownsTotal.Load()
}

// SetTimeoutManager sets the timeout manager
func (sc *ServeCoordinator) SetTimeoutManager(tm TimeoutManager) {
	sc.timeoutManager = tm
}

// NewServeCoordinator creates a new serve coordinator
func NewServeCoordinator(server *Server, processor *MessageProcessor, lifecycle *ServerLifecycleBuilder) *ServeCoordinator {
	return &ServeCoordinator{
		server:    server,
		processor: processor,
		lifecycle: lifecycle,
	}
}

// ServeLoop runs the main serve loop with proper timeout and error handling
func (sc *ServeCoordinator) ServeLoop() error {
	reader := sc.lifecycle.GetReader()
	writer := sc.lifecycle.GetWriter()
	trace := sc.lifecycle.IsTraceEnabled()
	traceWriter := sc.lifecycle.GetTraceWriter()
	baseCtx := sc.lifecycle.GetBaseContext()

	var readChan <-chan ReadResult

	for {
		// CRITICAL: Check atomic shutdown flag on EACH loop iteration
		// This ensures we exit immediately when shutdown is ordered
		// Use atomic load for lock-free, thread-safe check
		if sc.server.shutdownFlag.Load() == 1 {
			return sc.handleExplicitShutdown()
		}

		sc.loopIterationsTotal.Add(1)

		// Get and validate server context for timeout detection
		currentServerCtx := sc.getValidServerContext(trace, traceWriter)

		// Log waiting state
		sc.logWaitingState()

		// Read message asynchronously (only if not already reading)
		if readChan == nil {
			readChan = sc.processor.ReadMessageAsync(reader)
		}

		// Wait for message, timeout, or shutdown (so we don't block forever on stdin read)
		timeoutChan := sc.getTimeoutChannel(currentServerCtx)
		shutdownChan := sc.server.GetShutdownContext().Done()

		select {
		case readResult := <-readChan:
			readChan = nil // Reset for next iteration

			// Check shutdown again after receiving message
			if sc.server.shutdownFlag.Load() == 1 {
				return sc.handleExplicitShutdown()
			}

			if err := sc.handleReadResult(readResult, currentServerCtx, writer, trace, traceWriter, baseCtx); err != nil {
				// Check if this is EOF (client disconnect) - exit loop
				if errors.Is(err, io.EOF) {
					// Already handled in handleReadError, return to exit the loop cleanly
					return nil
				}
				return err
			}

		case <-timeoutChan:
			if currentServerCtx != nil && currentServerCtx.Err() == context.Canceled {
				if (func() context.Context {
					if sc.timeoutManager != nil {
						return sc.timeoutManager.GetServerContext()
					}
					return nil
				})() != currentServerCtx {
					if trace && traceWriter != nil {
						sc.server.traceLogf("[MCP_DEBUG] Context was replaced (timeout reset), ignoring cancellation")
					}
					continue
				}
			}
			return sc.handleTimeout(currentServerCtx, writer, trace, traceWriter)

		case <-shutdownChan:
			// Shutdown requested while blocked on read; exit without waiting for stdin.
			// The in-flight reader goroutine may remain blocked in read(stdin) until process exit.
			return sc.handleExplicitShutdown()
		}

		// Check for explicit shutdown request (legacy check, atomic flag is primary)
		if sc.server.IsShutdownRequested() {
			return sc.handleExplicitShutdown()
		}
	}
}

// getValidServerContext gets and validates the server context for timeout detection
func (sc *ServeCoordinator) getValidServerContext(trace bool, traceWriter io.Writer) context.Context {
	currentServerCtx := (func() context.Context {
		if sc.timeoutManager != nil {
			return sc.timeoutManager.GetServerContext()
		}
		return nil
	})()

	iter := sc.loopIterationsTotal.Load()
	if currentServerCtx != nil {
		if err := currentServerCtx.Err(); err != nil {
			// Context is already cancelled - from previous timeout
			if trace && traceWriter != nil && iter == 1 {
				sc.server.traceLogf("[MCP_DEBUG] Server context is cancelled (from previous timeout), ignoring until reset")
			}
			currentServerCtx = nil // Treat as no context
		}
	}

	// Log loop iteration for debugging
	if trace && traceWriter != nil && iter%100 == 0 {
		sc.server.traceLogf("[MCP_DEBUG] Loop iteration %d, checking for timeout...", iter)
	}

	return currentServerCtx
}

// getTimeoutChannel gets the timeout channel from server context
func (sc *ServeCoordinator) getTimeoutChannel(serverCtx context.Context) <-chan struct{} {
	if serverCtx != nil {
		return serverCtx.Done()
	}
	return nil // nil channel never triggers
}

// logWaitingState logs that we're waiting for a message
// CRITICAL: Use traceLogf instead of SendLogDebug to prevent interleaving with responses
// SendLogDebug queues messages that can interleave with JSON-RPC responses, causing parsing errors
func (sc *ServeCoordinator) logWaitingState() {
	sc.server.traceLogf("[MCP_DEBUG] Waiting for message from client: event=waiting_for_message")
}

// handleReadResult handles the result of a message read operation
func (sc *ServeCoordinator) handleReadResult(result ReadResult, serverCtx context.Context, writer *bufio.Writer, trace bool, traceWriter io.Writer, baseCtx context.Context) error {
	// Reset timeout on client activity
	if sc.timeoutManager != nil {
		sc.timeoutManager.ResetTimeout()
	}

	// Log read result
	sc.logReadResult(result.Err)

	// Handle read errors
	if result.Err != nil {
		return sc.handleReadError(result.Err, serverCtx, writer, trace, traceWriter)
	}

	// Process message
	return sc.processor.ProcessMessage(
		serverCtx,
		result.Msg,
		result.Format,
		writer,
		baseCtx,
		sc.server.operationTracker,
	)
}

// logReadResult logs the result of a message read
// CRITICAL: Use traceLogf instead of SendLogDebug to prevent interleaving with responses
// SendLogDebug queues messages that can interleave with JSON-RPC responses, causing parsing errors
func (sc *ServeCoordinator) logReadResult(readErr error) {
	if readErr != nil {
		sc.server.traceLogf("[MCP_DEBUG] Message read completed with error: error=%v event=message_read_error", readErr)
	} else {
		sc.server.traceLogf("[MCP_DEBUG] Message read completed successfully, timeout reset: event=message_read_success")
	}
}

// handleReadError handles read errors with proper shutdown logic
func (sc *ServeCoordinator) handleReadError(readErr error, serverCtx context.Context, writer *bufio.Writer, trace bool, traceWriter io.Writer) error {
	// Check for context cancellation (idle timeout)
	if serverCtx != nil && errors.Is(readErr, serverCtx.Err()) {
		return sc.handleIdleTimeout(serverCtx, writer, trace, traceWriter, "during read")
	}

	// Check for EOF (client disconnect)
	if errors.Is(readErr, io.EOF) {
		_ = sc.handleClientDisconnect(writer) //nolint:errcheck // Best effort - cleanup on error
		return io.EOF                         // Return EOF to signal continue in ServeLoop
	}

	// TCP/proxy clients often drop with reset/broken-pipe rather than clean EOF.
	// Treat those as per-connection disconnect — do not shut down the whole daemon
	// (proxy + multi-provider share one TCP listener).
	// TRACK: [REDACTED-ID] — TCP path still shares one Server; disconnect
	// resets global init state (handleClientDisconnect). Fine for single IDE proxy; multi-conn needs isolation.
	if isBrokenPipeError(readErr) {
		sc.server.traceLogf("[MCP_INFO] Client transport closed (%v) — ending connection only", readErr)
		_ = sc.handleClientDisconnect(writer) //nolint:errcheck // Best effort - cleanup on error
		return io.EOF
	}

	// Check for context errors
	if errors.Is(readErr, context.DeadlineExceeded) || errors.Is(readErr, context.Canceled) {
		return sc.handleContextError(readErr, writer, trace, traceWriter)
	}

	// Real error - shutdown
	eventCtx := sc.server.getClientEventContext()
	eventCtx.RecordEvent("disconnect", map[string]any{
		objects.FieldKeyReason: "read_error",
		"error":                readErr.Error(),
	})

	realErr := errfmt.Newf("failed to read MCP request").Wrap(readErr)
	logTraceError("Failed to read MCP request", realErr)
	if sc.endConnectionOnly(fmt.Sprintf("read error: %v", realErr), writer) {
		return io.EOF
	}
	sc.server.shutdownSequence(fmt.Sprintf("read error: %v", realErr))
	return realErr
}

// endConnectionOnly ends one TCP session without killing the shared daemon.
// Returns true when the caller should treat this as connection teardown (io.EOF).
func (sc *ServeCoordinator) endConnectionOnly(reason string, writer *bufio.Writer) bool {
	if sc == nil || sc.server == nil || !sc.server.multiClient.Load() {
		return false
	}
	sc.server.traceLogf("[MCP_INFO] Ending TCP connection only (multi-client): %s", reason)
	MarkSessionDisconnected(sc.server)
	sc.server.releaseConnectionWriter(writer)
	if writer != nil {
		_ = writer.Flush()
	}
	return true
}

// handleIdleTimeout handles idle timeout during read
func (sc *ServeCoordinator) handleIdleTimeout(serverCtx context.Context, writer *bufio.Writer, trace bool, traceWriter io.Writer, contextStr string) error {
	sc.timeoutShutdownsTotal.Add(1)
	timeoutDuration := sc.getTimeoutDuration()
	reason := fmt.Sprintf("idle timeout %s (configured: %s): %s", contextStr, timeoutDuration, serverCtx.Err().Error())

	sc.server.traceLogf("[MCP_INFO] Shutdown triggered: %s", reason)

	MarkSessionDisconnected(sc.server)

	eventCtx := sc.server.getClientEventContext()
	eventCtx.RecordEvent("disconnect", map[string]any{
		objects.FieldKeyReason: "idle_timeout",
	})

	if sc.endConnectionOnly(reason, writer) {
		return io.EOF
	}

	// Send notification to client
	if writer != nil {
		_ = sc.server.SendMessageToClient( //nolint:errcheck
			"Server shutting down due to idle timeout. Re-initialization will be required on next request.",
			"server_shutdown",
			"low",
		)
	}

	_ = sc.server.SendLogInfo("MCP server graceful shutdown: idle timeout during read", //nolint:errcheck
		map[string]any{
			objects.FieldKeyReason: serverCtx.Err().Error(),
			"project_root":         sc.server.GetProjectRoot(),
		})

	if trace {
		_ = sc.server.SendLogDebug("Graceful shutdown: context cancelled during read", //nolint:errcheck
			map[string]any{
				objects.FieldKeyReason: serverCtx.Err().Error(),
				logMapKeyComponent:     "mcp_server",
			})
	}

	writer.Flush()
	sc.server.shutdownSequence(reason)
	return nil // Clean exit
}

// handleClientDisconnect handles client disconnect (EOF)
func (sc *ServeCoordinator) handleClientDisconnect(writer *bufio.Writer) error {
	// Mark session disconnected so retention can archive/delete it (best-effort, non-blocking)
	MarkSessionDisconnected(sc.server)

	eventCtx := sc.server.getClientEventContext()
	eventCtx.RecordEvent("disconnect", map[string]any{
		objects.FieldKeyReason: "EOF",
	})

	// Always drop event subscriptions for this connection Writer first so
	// events/list subscriberCount cannot retain reconnect ghosts.
	sc.server.unsubscribeWriterSubscriptions(writer)

	// Stdio: one client — reset so the next process can re-initialize.
	// TCP/proxy daemon: many clients share one Server — clearing tools/init here
	// races the live IDE proxy and looks like a hang (tools never list).
	// Still release this connection's Writer so reconnect / ephemeral dials do not
	// enqueue JSON-RPC onto a closed TCP socket (30s adapter timeout → Not connected).
	if sc.server.multiClient.Load() {
		sc.server.traceLogf("[MCP_INFO] Client disconnected (EOF) — multi-client mode, leaving shared server state intact")
		sc.server.releaseConnectionWriter(writer)
	} else {
		sc.server.traceLogf("[MCP_INFO] Client disconnected (EOF) - resetting state for re-initialization")
		sc.resetServerState()
	}

	if writer != nil {
		writer.Flush()
	}
	// Continue serving loop instead of returning - allows new client to connect
	// Return a sentinel error that ServeLoop will handle by continuing
	return nil // Continue loop (handled specially in ServeLoop)
}

// handleContextError handles context-related errors
func (sc *ServeCoordinator) handleContextError(readErr error, writer *bufio.Writer, trace bool, traceWriter io.Writer) error {
	timeoutDuration := sc.getTimeoutDuration()
	reason := "idle_timeout"
	if errors.Is(readErr, context.Canceled) {
		reason = "cancelled"
	}

	eventCtx := sc.server.getClientEventContext()
	eventCtx.RecordEvent("disconnect", map[string]any{
		objects.FieldKeyReason: reason,
	})

	// Send notification to client
	if writer != nil {
		_ = sc.server.SendMessageToClient( //nolint:errcheck
			"Server shutting down due to idle timeout. Re-initialization will be required on next request.",
			"server_shutdown",
			"low",
		)
	}

	_ = sc.server.SendLogInfo("MCP server graceful shutdown: idle timeout (context error)", //nolint:errcheck
		map[string]any{
			"error":        readErr.Error(),
			"project_root": sc.server.GetProjectRoot(),
		})

	if trace {
		_ = sc.server.SendLogDebug("Graceful shutdown: context error during read", //nolint:errcheck
			map[string]any{
				"error":            readErr.Error(),
				logMapKeyComponent: "mcp_server",
			})
	}

	shutdownReason := fmt.Sprintf("idle timeout (context error, configured: %s): %s", timeoutDuration, readErr.Error())
	sc.server.traceLogf("[MCP_INFO] Shutdown triggered: %s", shutdownReason)

	MarkSessionDisconnected(sc.server)

	if sc.endConnectionOnly(shutdownReason, writer) {
		return io.EOF
	}

	writer.Flush()
	sc.server.shutdownSequence(shutdownReason)
	return nil // Clean exit
}

// handleTimeout handles timeout expiration in serve loop
func (sc *ServeCoordinator) handleTimeout(serverCtx context.Context, writer *bufio.Writer, trace bool, traceWriter io.Writer) error {
	timeoutErr := serverCtx.Err()
	timeoutDuration := sc.getTimeoutDuration()
	reason := fmt.Sprintf("idle timeout in serve loop (configured: %s): %s", timeoutDuration, timeoutErr.Error())

	sc.server.traceLogf("[MCP_INFO] Shutdown triggered: %s", reason)

	MarkSessionDisconnected(sc.server)

	eventCtx := sc.server.getClientEventContext()
	eventCtx.RecordEvent("disconnect", map[string]any{
		objects.FieldKeyReason: "idle_timeout",
	})

	if sc.endConnectionOnly(reason, writer) {
		return io.EOF
	}

	_ = sc.server.SendLogInfo("MCP server graceful shutdown: idle timeout", //nolint:errcheck
		map[string]any{
			objects.FieldKeyReason: timeoutErr.Error(),
			"project_root":         sc.server.GetProjectRoot(),
			"trace":                "timeout_expired_in_serve_loop",
			"loop_iteration":       sc.loopIterationsTotal.Load(),
		})

	if sc.server.canSendNotifications() {
		_ = sc.server.SendMessageToClient( //nolint:errcheck
			"Server shutting down due to idle timeout. Re-initialization will be required on next request.",
			"server_shutdown",
			"low",
		)
	}

	if trace {
		_ = sc.server.SendLogDebug("Graceful shutdown: context cancelled", //nolint:errcheck
			map[string]any{
				objects.FieldKeyReason: timeoutErr.Error(),
				logMapKeyComponent:     "mcp_server",
			})
	}

	writer.Flush()
	sc.server.shutdownSequence(reason)
	return nil // Clean exit
}

// handleExplicitShutdown handles explicit shutdown request
func (sc *ServeCoordinator) handleExplicitShutdown() error {
	MarkSessionDisconnected(sc.server)

	eventCtx := sc.server.getClientEventContext()
	eventCtx.RecordEvent("disconnect", map[string]any{
		objects.FieldKeyReason: "explicit_shutdown",
		"details":              sc.server.shutdownReason,
	})

	sc.server.shutdownSequence(sc.server.shutdownReason)
	return nil
}

// getTimeoutDuration gets the configured timeout duration as a string
func (sc *ServeCoordinator) getTimeoutDuration() string {
	if sc.server.config != nil && sc.server.config.MCPServer.IdleTimeout != emptyValue {
		return sc.server.config.MCPServer.IdleTimeout
	}
	return "unknown"
}

// resetServerState resets server state for re-initialization (thread-safe)
func (sc *ServeCoordinator) resetServerState() {
	sc.server.SetInitialized(false)
	sc.server.shutdownRequested.Store(false)
	sc.server.shutdownMu.Lock()
	sc.server.shutdownReason = emptyValue
	sc.server.shutdownMu.Unlock()

	// Clear client ID
	var oldClientID string
	_ = concurrency.RunInLockWithLogger(
		&sc.server.clientIDMu, LockNameMcpServeCoordinatorClearClientId, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			oldClientID = sc.server.clientID
			sc.server.clientID = emptyValue
			return nil
		},
	)
	sc.server.traceLogf("[MCP_DEBUG] Cleared client ID (was: %s)", oldClientID)

	// Clear sequence ID
	_ = concurrency.RunInLockWithLogger(
		&sc.server.sequenceIDMu, LockNameMcpServeCoordinatorClearSequenceId, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			sc.server.currentSequenceID = emptyValue
			return nil
		},
	)

	// Clear current session ID (disconnect callback may still be running; clear so next connection gets fresh session)
	sc.server.ClearCurrentSessionID()

	// Close client-specific trace file
	_ = concurrency.RunInLockWithLogger(
		&sc.server.traceCloserMu, LockNameMcpServeCoordinatorCloseTraceFile, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if sc.server.traceCloser != nil {
				_ = sc.server.traceCloser.Close() //nolint:errcheck
				sc.server.traceCloser = nil
			}
			return nil
		},
	)

	// Reset trace writer
	sc.resetTraceWriter()

	// Clear tools and prompts
	var oldToolCount int
	_ = concurrency.RunInLockWithLogger(
		&sc.server.toolsMu, LockNameMcpServeCoordinatorClearTools, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			oldToolCount = len(sc.server.tools)
			sc.server.tools = make(map[string]Tool)
			sc.server.prompts = make(map[string]Prompt)
			// Clear caches atomically
			sc.server.toolsCache.Store([]Tool{})
			sc.server.promptsCache.Store([]Prompt{})
			return nil
		},
	)
	sc.server.traceLogf("[MCP_DEBUG] Cleared tools and prompts (was %d tools)", oldToolCount)

	// Re-register built-in tools (need to check if secCtx is available)
	// Do NOT wrap this in toolsMu lock because RegisterTool already acquires it (deadlock otherwise)
	RegisterGraphTools(sc.server)
	RegisterEchoTool(sc.server)
	RegisterCommonTools(sc.server)
	RegisterInteractiveTools(sc.server)
	if sc.server.secCtx != nil {
		if secCtx, ok := sc.server.secCtx.(*pkgctx.SecurityContext); ok {
			RegisterWorkflowTools(sc.server, secCtx)
		}
	}
	RegisterMetricsTools(sc.server)
	RegisterOnboardingPrompts(sc.server)
	var newToolCount int
	_ = concurrency.RunInRLockWithLogger(
		&sc.server.toolsMu, LockNameMcpServeCoordinatorCountTools, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			newToolCount = len(sc.server.tools)
			return nil
		},
	)
	sc.server.traceLogf("[MCP_DEBUG] Re-registered built-in tools (%d tools ready for next client)", newToolCount)
}

// resetTraceWriter resets trace writer to default (thread-safe)
func (sc *ServeCoordinator) resetTraceWriter() {
	_ = concurrency.RunInLockWithLogger(
		&sc.server.traceWriterMu, LockNameMcpServeCoordinatorResetTraceWriter, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			config := sc.server.config
			if sc.lifecycle.IsTraceEnabled() {
				defaultTraceWriter, defaultTraceCloser := openTraceWriter(config, sc.server.GetProjectRoot())
				sc.server.traceWriter = defaultTraceWriter
				if defaultTraceCloser != nil {
					_ = concurrency.RunInLockWithLogger(
						&sc.server.traceCloserMu, LockNameMcpServeCoordinatorSetTraceCloser, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
						func() error {
							sc.server.traceCloser = defaultTraceCloser
							return nil
						},
					)
				}
			} else {
				sc.server.traceWriter = os.Stderr
			}
			return nil
		},
	)
}
