package mcp

import (
	"context"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/logging"
)

// RequestShutdown is the client-facing shutdown request (JSON-RPC shutdown,
// notifications/cancelled client_shutdown, server_shutdown tool).
// On a multi-client TCP/TLS daemon this must NOT arm process shutdown — IDE
// (and other proxies) send shutdown on reload/disconnect; killing the listener
// surfaces as "transport closed" / ECONNREFUSED for every client.
func (s *Server) RequestShutdown(reason string) {
	if s == nil {
		return
	}
	if s.multiClient.Load() {
		s.traceLogf("[MCP_INFO] Ignoring client shutdown on multi-client daemon (connection will end on EOF): %s", reason)
		return
	}
	s.RequestProcessShutdown(reason)
}

// RequestProcessShutdown arms process-wide shutdown (OS signals, operator stop).
// Always takes effect, including multi-client TCP daemons. Runs shutdownSequence
// so the TCP Accept loop wakes (listener close via shutdownCtx) even when no
// ServeLoop is mid-iteration to observe IsShutdownRequested.
func (s *Server) RequestProcessShutdown(reason string) {
	if s == nil {
		return
	}
	if !s.shutdownRequested.CompareAndSwap(false, true) {
		return
	}
	s.shutdownMu.Lock()
	s.shutdownReason = reason
	s.shutdownMu.Unlock()
	s.traceLogf("[MCP_INFO] Process shutdown requested: %s", reason)
	s.shutdownSequence(reason)
}

// shutdownOnConnectionFault shuts down stdio (single-client) servers on write/marshal faults.
// Multi-client TCP/TLS daemons must keep the listener up — one dead IDE proxy must not
// cancel shutdownCtx / close Accept.
func (s *Server) shutdownOnConnectionFault(reason string) {
	if s == nil {
		return
	}
	if s.multiClient.Load() {
		s.traceLogf("[MCP_INFO] Connection fault (multi-client, ending connection only): %s", reason)
		return
	}
	s.shutdownSequence(reason)
}

// shutdownSequence performs cleanup operations before server shutdown
// This ensures all resources are properly closed and state is saved
// This method is idempotent - it can be called multiple times safely
func (s *Server) shutdownSequence(reason string) {
	// CRITICAL: Use atomic compare-and-swap to ensure shutdown only runs once
	// This is thread-safe and lock-free - multiple goroutines can call this safely
	// If shutdownFlag is already 1, swap returns false and we return early
	if !s.shutdownFlag.CompareAndSwap(0, 1) {
		return // Already shutting down (another goroutine set the flag)
	}

	s.shutdownsInitiatedTotal.Add(1)

	// Store shutdown reason (requires mutex for string assignment)
	_ = concurrency.RunInLock(&s.shutdownMu, func() error {
		s.shutdownReason = reason
		return nil
	})

	// CRITICAL: Cancel shutdown context FIRST to notify all registered hooks
	// This allows components to check ctx.Done() and exit gracefully
	// without needing to acquire locks that might be held by shutdownSequence
	s.shutdownCancel()

	// Notify all registered hooks that shutdown has been ordered
	// Hooks should check the context and exit immediately if they can't finish work
	_ = s.shutdownHookManager.NotifyHooks() //nolint:errcheck // Hook errors are logged but don't block shutdown

	// CRITICAL: Shutdown process group manager to control all tracked goroutines and subprocesses
	// This ensures all spawned processes are properly terminated
	if s.processGroupManager != nil {
		if err := s.processGroupManager.Shutdown(reason); err != nil {
			// Log warning but don't block shutdown - some critical processes may still be running
			s.traceLogf("[MCP_WARN] Process group shutdown had issues: %v", err)
		}
	}

	// CRITICAL: Always write shutdown reason directly to trace file FIRST
	// This ensures the reason is visible even if client has disconnected
	// and SendLogDebug messages don't make it to the trace file
	s.traceLogf("[MCP_INFO] Shutdown initiated: reason=%s", reason)

	// CRITICAL: Do NOT call SendLogDebug here - it would try to acquire clientsMu via findClientQueue()
	// Even though SendLogMessage has guards, there's a race window where shutdownSequence
	// might acquire clientsMu.Lock() while SendLogMessage is trying to call findClientQueue()
	// Instead, write directly to trace file to avoid any lock acquisition during shutdown
	// _ = s.SendLogDebug("Starting shutdown sequence", ...) // DISABLED - causes race condition
	s.traceLogf("[MCP_DEBUG] Starting shutdown sequence: component=mcp_server reason=%s", reason)

	// 1. Stop periodic compression to prevent goroutine leak
	_ = concurrency.RunInLockWithLogger(
		&s.compressionTickerMu, LockNameMcpServerStopCompression, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if s.compressionCancel != nil {
				// Cancel context first - this signals the goroutine to exit
				s.compressionCancel()
				s.compressionCancel = nil
			}
			if s.compressionTicker != nil {
				// Stop ticker as backup (goroutine should already be exiting via context)
				s.compressionTicker.Stop()
				s.compressionTicker = nil
				// CRITICAL: Do NOT call SendLogDebug here - avoid lock acquisition during shutdown
				// Write directly to trace file instead
				s.traceLogf("[MCP_DEBUG] Compression ticker stopped: component=mcp_server")
			}
			s.compressionCtx = nil
			return nil
		},
	)

	// 2. Stop all client message queues and flush remaining messages
	// CRITICAL: Use TryLock() to avoid deadlock - if a goroutine holds RLock (e.g. SendMessageToClient),
	// we must not block on Lock() or shutdown never completes. TryLock with short retries then proceed.
	var clientIDs []string
	const tryLockTimeout = 50 * time.Millisecond
	const tryLockInterval = 2 * time.Millisecond
	deadline := time.Now().Add(tryLockTimeout)
	gotLock := false
	for time.Now().Before(deadline) {
		if s.clientsMu.TryLock() {
			gotLock = true
			for clientID, client := range s.clients {
				if client.Queue != nil {
					clientIDs = append(clientIDs, clientID)
					client.Queue.Stop()
				}
			}
			s.clientsMu.Unlock()
			break
		}
		time.Sleep(tryLockInterval)
	}
	if !gotLock {
		s.traceLogf("[MCP_DEBUG] Shutdown could not acquire clientsMu (skipped stopping queues): reason=%s", reason)
	}

	// CRITICAL: Do NOT call SendLogDebug here - even though we released the lock,
	// SendLogDebug -> SendLogMessage -> findClientQueue() could still race with other operations
	// Write directly to trace file to avoid any potential lock contention
	for _, clientID := range clientIDs {
		s.traceLogf("[MCP_DEBUG] Stopped message queue for client: component=mcp_server client_id=%s", clientID)
	}

	// 3. Save client metrics before shutdown
	if s.clientMetricsStore != nil {
		// Close will perform final save and wait for save worker to finish
		if err := s.clientMetricsStore.Close(); err != nil {
			// CRITICAL: Write directly to trace file - avoid SendLogWarn which could acquire locks
			s.traceLogf("[MCP_WARN] Failed to save client metrics during shutdown: component=mcp_server error=%s", err.Error())
		} else {
			// CRITICAL: Write directly to trace file - avoid SendLogDebug which could acquire locks
			s.traceLogf("[MCP_DEBUG] Client metrics saved during shutdown: component=mcp_server")
		}
	}

	// 3. Cleanup event emitter (remove inactive subscribers)
	if s.eventEmitter != nil {
		s.eventEmitter.Cleanup()
		// CRITICAL: Write directly to trace file - avoid SendLogDebug which could acquire locks
		s.traceLogf("[MCP_DEBUG] Event emitter cleaned up: component=mcp_server")
	}

	// 4. Cancel any pending operations in operation tracker
	if s.operationTracker != nil {
		// Cancel all tracked operations to prevent goroutine leaks
		operations := s.operationTracker.GetOperations()
		for _, op := range operations {
			if op.Cancel != nil {
				op.Cancel()
			}
		}
		// CRITICAL: Write directly to trace file - avoid SendLogDebug which could acquire locks
		s.traceLogf("[MCP_DEBUG] Operation tracker shutdown (operations cancelled): component=mcp_server operations_count=%d", len(operations))
	}

	// 5. Close trace file (if not already closed)
	_ = concurrency.RunInLock(&s.traceCloserMu, func() error {
		if s.traceCloser != nil {
			_ = s.traceCloser.Close() // Best effort during shutdown
			s.traceCloser = nil
		}
		return nil
	})

	// 6. Flush writer if available (check *bufio.Writer explicitly: nil pointer in interface is non-nil interface)
	_ = concurrency.RunInRLock(&s.transportMu, func() error {
		if s.transportWriter != nil {
			if err := s.transportWriter.Flush(); err != nil {
				s.traceLogf("[MCP_WARN] Failed to flush writer during shutdown: component=mcp_server error=%s", err.Error())
			}
		}
		return nil
	})

	// CRITICAL: Write directly to trace file - avoid SendLogInfo which could acquire locks
	s.traceLogf("[MCP_INFO] Shutdown sequence completed: component=mcp_server reason=%s", reason)

	// CRITICAL: Always write shutdown completion directly to trace file
	// This ensures it's visible even if client has disconnected
	s.traceLogf("[MCP_INFO] Shutdown completed: reason=%s", reason)

	// Record shutdown completed as a client event for metrics tracking
	// This allows monitoring systems to track server lifecycle events
	eventCtx := s.getClientEventContext()
	eventCtx.RecordEvent("shutdown_completed", map[string]any{
		objects.FieldKeyReason: reason,
	})
}
func (s *Server) isShuttingDown() bool {
	// CRITICAL: Use atomic load for lock-free, thread-safe check
	// This is the primary shutdown check - fast and non-blocking
	if s.shutdownFlag.Load() == 1 {
		return true
	}
	// Also check context as a secondary indicator (defensive)
	select {
	case <-s.shutdownCtx.Done():
		return true
	default:
		return false
	}
}

// GetShutdownContext returns the shutdown context
// Components should check ctx.Done() instead of trying to acquire locks during shutdown
func (s *Server) GetShutdownContext() context.Context {
	return s.shutdownCtx
}

// GetProcessGroupManager returns the process group manager for subprocess lifecycle (e.g. CLI tool invocations).
// Returns nil if not set. Used so CLI command execution can register subprocesses for deterministic shutdown.
func (s *Server) GetProcessGroupManager() *ProcessGroupManager {
	return s.processGroupManager
}

func (s *Server) RegisterShutdownHook(hook ShutdownHook) {
	s.shutdownHookManager.RegisterHook(hook)
}
