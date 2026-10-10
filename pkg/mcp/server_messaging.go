package mcp

import (
	"bufio"
	"encoding/json"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

func (s *Server) SendMessageToClient(message, messageType, priority string) error {
	return s.SendMessageToClientByID("", message, messageType, priority)
}
func (s *Server) SendMessageToClientByID(clientID, message, messageType, priority string) error {
	// GUARD 0: Check atomic shutdown flag at the very start (before any work)
	// This ensures we exit immediately if shutdown is ordered
	// CRITICAL: This is the FIRST check - before any allocations or function calls
	if s.shutdownFlag.Load() == 1 {
		return nil // Shutdown in progress, exit immediately
	}

	// GUARD 0.5: Double-check shutdown context (defensive, handles race conditions)
	// Sometimes shutdownFlag might not be set yet, but context is cancelled
	select {
	case <-s.shutdownCtx.Done():
		return nil // Shutdown ordered, exit immediately
	default:
	}

	// Record server-sent notification event using context object
	eventCtx := s.getClientEventContext()
	if clientID == emptyValue {
		clientID = eventCtx.GetClientID()
	}
	if clientID != emptyValue {
		eventCtx.RecordEventWithClientID("notification_sent", clientID, map[string]any{
			"message_type":           messageType,
			objects.FieldKeyPriority: priority,
			"message_len":            len(message),
		})
	}

	// Create event for subscribers
	event := &Event{
		Type:      EventTypeActionRequired,
		Timestamp: time.Now(),
		Message:   message,
		Fields: map[string]any{
			"message_type": messageType,
		},
		Priority: priority,
		Severity: "info",
	}

	// Emit event for subscribers
	if s.eventEmitter != nil {
		s.eventEmitter.Emit(event)
	}

	// Check shutdown context FIRST (fast path, no lock needed)
	// This prevents any lock acquisition during shutdown
	select {
	case <-s.shutdownCtx.Done():
		// Shutdown ordered, don't try to send messages - return immediately
		return nil
	default:
		// Continue - shutdown not ordered yet
	}

	// Route to specific client or default to current connection
	var writer *bufio.Writer
	var format *MessageFormat

	// CRITICAL: Check shutdown flag AGAIN before any client lookup
	// This is quadruple redundancy for 99.9999% reliability
	// We've done work (event recording, event emission) since the last check,
	// so shutdown may have been ordered during that time
	if s.shutdownFlag.Load() == 1 {
		return nil // Shutdown in progress, exit immediately
	}
	select {
	case <-s.shutdownCtx.Done():
		return nil // Shutdown ordered, exit immediately
	default:
	}

	// Use safe abstraction to find client with proper shutdown handling
	if clientID != emptyValue {
		// CRITICAL: Final check immediately before findClientByID (which uses TryRLock)
		// This is the absolute last chance to avoid any lock acquisition
		if s.shutdownFlag.Load() == 1 {
			return nil // Shutdown in progress, exit immediately
		}
		select {
		case <-s.shutdownCtx.Done():
			return nil // Shutdown ordered, exit immediately
		default:
		}
		client, exists := s.findClientByID(clientID)
		if exists {
			writer = client.Writer
			format = client.Format
		}
	}

	// Fall back to current connection if clientID not found or empty
	if writer == nil || format == nil {
		_ = concurrency.RunInRLockWithLogger(
			&s.transportMu, LockNameMcpServerGetTransportFallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				writer = s.transportWriter
				format = s.transportFormat
				return nil
			},
		)
	}

	if writer == nil || format == nil {
		// Transport not ready yet - event was emitted, that's enough
		return nil
	}

	// Create JSON-RPC notification
	notification := map[string]any{
		jsonrpcFieldJSONRPC: JSONRPCVersion,
		jsonrpcFieldMethod:  notificationMethodMessage,
		jsonrpcFieldParams: map[string]any{
			"message":                message,
			"message_type":           messageType,
			objects.FieldKeyPriority: priority,
			"timestamp":              zqktime.NowRFC3339UTC(),
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(notification)
	if err != nil {
		return errfmt.Newf("failed to marshal notification").Wrap(err)
	}

	// Use message queue if available, otherwise fall back to direct write
	// CRITICAL: Check shutdown context before ANY lock acquisition to prevent deadlock
	// If shutdown ordered, skip queue entirely and return immediately
	select {
	case <-s.shutdownCtx.Done():
		// Shutdown ordered, don't try to send messages at all
		// Queues are stopped and we shouldn't acquire any locks
		return nil
	default:
		// Continue - shutdown not ordered yet
	}

	// Try to use client-specific queue if clientID provided
	if clientID != emptyValue {
		// GUARD 1: Check shutdown context before trying to find client
		select {
		case <-s.shutdownCtx.Done():
			return nil // Shutdown ordered, exit immediately
		default:
		}

		// GUARD 2: Double-check with atomic flag (lock-free, thread-safe)
		if s.shutdownFlag.Load() == 1 {
			return nil // Shutdown in progress, exit immediately
		}

		// Use safe abstraction to find client queue (has shutdown checks built-in)
		// findClientByID uses withClientsReadLock which has multiple guards
		client, exists := s.findClientByID(clientID)
		if exists && client.Queue != nil {
			// GUARD 3: Re-check shutdown context before using queue (shutdown may have been ordered)
			select {
			case <-s.shutdownCtx.Done():
				return nil // Shutdown ordered, exit immediately
			default:
			}

			// GUARD 4: Final atomic check before using queue (lock-free)
			if s.shutdownFlag.Load() == 1 {
				return nil // Shutdown started, exit immediately
			}

			queue := client.Queue
			// Determine priority from messageType/priority
			msgPriority := "normal"
			if priority == "high" || priority == "critical" {
				msgPriority = priority
			} else if messageType == "error" || messageType == "critical" {
				msgPriority = "high"
			}
			// Enqueue message (non-blocking)
			if !queue.Enqueue(data, format, msgPriority, nil) {
				// Message was dropped due to queue being full
				// This is expected behavior when client is slow
				return nil // Don't return error - dropping is intentional backpressure
			}
			return nil
		}
	}

	// Fall back to direct write for default client or if queue not available
	// This maintains backward compatibility

	// GUARD 5: Check shutdown context again before trying default queue
	// Shutdown may have been ordered while we were processing client-specific queue
	select {
	case <-s.shutdownCtx.Done():
		// Shutdown ordered, don't try to send messages
		return nil
	default:
	}

	// GUARD 6: Double-check with atomic flag (lock-free, thread-safe)
	if s.shutdownFlag.Load() == 1 {
		return nil // Shutdown in progress, exit immediately
	}

	// Try to use default client queue if available (uses safe abstraction with shutdown checks)
	// findClientQueue uses withClientsReadLock which has multiple guards
	defaultQueue := s.findClientQueue()

	// GUARD 7: Final shutdown context check before using queue
	select {
	case <-s.shutdownCtx.Done():
		return nil // Shutdown ordered, exit immediately
	default:
	}

	// GUARD 8: Final atomic check before using queue (lock-free)
	if s.shutdownFlag.Load() == 1 {
		return nil // Shutdown started, exit immediately
	}

	if defaultQueue != nil {
		msgPriority := "normal"
		if priority == "high" || priority == "critical" {
			msgPriority = priority
		} else if messageType == "error" || messageType == "critical" {
			msgPriority = "high"
		}
		dropped := !defaultQueue.Enqueue(data, format, msgPriority, nil)
		if dropped {
			// Queue full, message dropped - this is expected backpressure behavior
			// Metrics are updated via queue callback
			return nil
		}
		// Message queued successfully - metrics updated via queue callback
		return nil
	}

	// CRITICAL: Never fall back to direct write - this causes interleaving with response writes
	// Direct writes to the transport writer can corrupt the JSON-RPC stream
	// If queue is not available, just drop the message (notifications are non-critical)
	// This ensures all writes go through the message queue, which serializes them properly
	s.traceLogf("[MCP_DEBUG] Message queue unavailable for notification, message dropped (non-critical)")
	return nil
}

// canSendNotifications checks if notifications can be sent
// Notifications require the server to be initialized and transport to be ready
func (s *Server) canSendNotifications() bool {
	if !s.initialized.Load() {
		return false
	}
	var ready bool
	_ = concurrency.RunInRLockWithLogger(
		&s.transportMu, LockNameMcpServerIsTransportReady, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			ready = s.transportWriter != nil && s.transportFormat != nil
			return nil
		},
	)
	return ready
}

// getQueueConfig returns the queue configuration from config or defaults
func (s *Server) getQueueConfig() QueueConfig {
	if s.config == nil {
		return DefaultQueueConfig()
	}

	config := DefaultQueueConfig()

	// Load from config file if provided
	if s.config.MCPServer.MessageQueue.MaxQueueSize > 0 {
		config.MaxQueueSize = s.config.MCPServer.MessageQueue.MaxQueueSize
	}
	config.DropWhenFull = s.config.MCPServer.MessageQueue.DropWhenFull
	// If not set in config, use default (true)
	if !s.config.MCPServer.MessageQueue.DropWhenFull && s.config.MCPServer.MessageQueue.MaxQueueSize == 0 {
		// Only use default if nothing was configured
		config.DropWhenFull = true
	}

	if s.config.MCPServer.MessageQueue.FlushInterval != emptyValue {
		if interval, err := time.ParseDuration(s.config.MCPServer.MessageQueue.FlushInterval); err == nil {
			config.FlushInterval = interval
		}
	}

	if s.config.MCPServer.MessageQueue.WriteTimeout != emptyValue {
		if timeout, err := time.ParseDuration(s.config.MCPServer.MessageQueue.WriteTimeout); err == nil {
			config.WriteTimeout = timeout
		}
	}

	return config
}

// BroadcastNotification broadcasts a JSON-RPC notification to all connected clients.
// It uses non-blocking enqueue on each client's message queue to prevent slow clients
// from blocking the server or other clients.
// Returns the number of clients to which the notification was successfully dispatched.
func (s *Server) BroadcastNotification(method string, params any) int {
	if s == nil || s.shutdownFlag.Load() == 1 {
		return 0
	}
	select {
	case <-s.shutdownCtx.Done():
		return 0
	default:
	}

	notification := map[string]any{
		jsonrpcFieldJSONRPC: JSONRPCVersion,
		jsonrpcFieldMethod:  method,
		jsonrpcFieldParams:  params,
	}

	data, err := json.Marshal(notification)
	if err != nil {
		return 0
	}

	sent := 0

	// 1. Broadcast to all active multi-client connections
	s.withClientsReadLock(func() bool {
		for _, client := range s.clients {
			if client != nil && client.Queue != nil && client.Format != nil {
				if client.Queue.Enqueue(data, client.Format, "high", nil) {
					sent++
				}
			}
		}
		return true
	})

	// 2. If no multi-client recipients or stdio transport is active, deliver to transportWriter
	if sent == 0 {
		var writer *bufio.Writer
		var format *MessageFormat
		_ = concurrency.RunInRLockWithLogger(
			&s.transportMu, LockNameMcpServerGetTransportFallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				writer = s.transportWriter
				format = s.transportFormat
				return nil
			},
		)
		if writer != nil && format != nil {
			q := s.queueForWriter(writer, format)
			if q != nil && q.Enqueue(data, format, "high", nil) {
				sent++
			} else {
				transport := NewDefaultTransport()
				if err := transport.WriteMessage(writer, data, format); err == nil {
					sent++
				}
			}
		}
	}

	return sent
}

// BroadcastMessage sends a notifications/message JSON-RPC notification to all connected clients.
func (s *Server) BroadcastMessage(message, messageType, priority string) int {
	params := map[string]any{
		"message":                message,
		"message_type":           messageType,
		objects.FieldKeyPriority: priority,
		"timestamp":              zqktime.NowRFC3339UTC(),
	}
	return s.BroadcastNotification(notificationMethodMessage, params)
}
