package mcp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// MessageProcessor coordinates message reading, processing, and response handling
// This provides thread-safe operations and clear separation of concerns
type MessageProcessor struct {
	server                 *Server
	handler                Handler
	transport              Transport
	messagesProcessedTotal atomic.Int64
	processErrorsTotal     atomic.Int64
}

// GetMessageProcessorStats returns lifetime counters for messages processed and process errors.
func (mp *MessageProcessor) GetMessageProcessorStats() (processed, errors int64) {
	if mp == nil {
		return 0, 0
	}
	return mp.messagesProcessedTotal.Load(), mp.processErrorsTotal.Load()
}

// NewMessageProcessor creates a new message processor
func NewMessageProcessor(server *Server, handler Handler, transport Transport) *MessageProcessor {
	return &MessageProcessor{
		server:    server,
		handler:   handler,
		transport: transport,
	}
}

// ReadResult represents the result of a message read operation
type ReadResult struct {
	Msg    []byte
	Format *MessageFormat
	Err    error
}

// ReadMessageAsync reads a message asynchronously with panic recovery
func (mp *MessageProcessor) ReadMessageAsync(reader *bufio.Reader) <-chan ReadResult {
	resultChan := make(chan ReadResult, 1)
	goroutinelabels.NewGoroutine("mcp_message_reader", "reading MCP message asynchronously").
		WithPanicHandler(func(r any) {
			stackTrace := string(debug.Stack())
			err := errfmt.Errorf("panic in transport.ReadMessage: %v\n\nStack trace:\n%s", r, stackTrace)
			resultChan <- ReadResult{Msg: nil, Format: nil, Err: err}
		}).
		StartSimple(func() {
			msg, format, err := mp.transport.ReadMessage(reader)
			resultChan <- ReadResult{Msg: msg, Format: format, Err: err}
		})
	return resultChan
}

// ProcessMessage processes a received message and returns a response
func (mp *MessageProcessor) ProcessMessage(ctx context.Context, msg []byte, format *MessageFormat, writer *bufio.Writer, baseCtx context.Context, operationTracker *OperationTracker) error {
	mp.messagesProcessedTotal.Add(1)
	err := mp.doProcessMessage(ctx, msg, format, writer, baseCtx, operationTracker)
	if err != nil {
		mp.processErrorsTotal.Add(1)
	}
	return err
}

func (mp *MessageProcessor) doProcessMessage(ctx context.Context, msg []byte, format *MessageFormat, writer *bufio.Writer, baseCtx context.Context, operationTracker *OperationTracker) error {
	// Update transport state (thread-safe)
	_ = concurrency.RunInLockWithLogger(
		&mp.server.transportMu, LockNameMcpMessageProcessorUpdateTransport, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			mp.server.transportFormat = format
			mp.server.transportWriter = writer
			return nil
		},
	)

	// Update client connection (thread-safe)
	mp.updateClientConnection(writer, format)

	// Unmarshal request
	req, parseErr := UnmarshalRequest(msg)
	if parseErr != nil {
		return mp.handleParseError(parseErr, format, writer)
	}

	// Create request-specific context with tracking
	reqID := fmt.Sprintf("%v", req.ID)
	reqCtx, cancel := operationTracker.StartOperation(reqID, req.Method, baseCtx)
	defer cancel()

	// Handle notification vs request
	// Notifications have null ID
	if req.ID == nil {
		return mp.handleNotification(reqCtx, req)
	}

	return mp.handleRequest(reqCtx, req, format, writer, operationTracker)
}

// updateClientConnection updates client connection state (thread-safe)
func (mp *MessageProcessor) updateClientConnection(writer *bufio.Writer, format *MessageFormat) {
	// Check if shutdown is in progress - if so, skip client connection update to avoid deadlock
	if mp.server.isShuttingDown() {
		return // Don't update client connections during shutdown
	}

	var currentClientID string
	_ = concurrency.RunInRLockWithLogger(
		&mp.server.clientIDMu, LockNameMcpMessageProcessorUpdateClientConnection, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			currentClientID = mp.server.clientID
			return nil
		},
	)

	if currentClientID == emptyValue {
		return
	}

	// For write operations, we still need direct lock access (write lock)
	// But we check shutdown first to avoid deadlock
	if mp.server.isShuttingDown() {
		return
	}

	_ = concurrency.RunInLockWithLogger(
		&mp.server.clientsMu, LockNameMcpMessageProcessorUpdateClientConnectionWrite, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Final shutdown check after acquiring write lock
			if mp.server.isShuttingDown() {
				return nil
			}

			if client, exists := mp.server.clients[currentClientID]; exists {
				// Update existing client connection
				client.Writer = writer
				client.Format = format
				client.LastSeen = time.Now()
				mp.ensureClientQueue(client, writer, format)
			} else {
				// Register new client connection
				mp.registerNewClient(currentClientID, writer, format)
			}
			return nil
		},
	)
}

// ensureClientQueue ensures a client has a message queue
func (mp *MessageProcessor) ensureClientQueue(client *ClientConnection, writer *bufio.Writer, format *MessageFormat) {
	if client.Queue != nil {
		client.Queue.UpdateWriter(writer, format)
	} else {
		queueConfig := mp.server.getQueueConfig()
		queue := NewMessageQueue(writer, format, queueConfig)
		queue.SetMetricsCallback(func(depth, dropped, sent, errors int64) {
			mp.server.mcpMetrics.RecordQueueMetrics(depth, dropped, sent, errors)
		})
		client.Queue = queue
	}
}

// registerNewClient registers a new client connection. Respects server max_clients limit (0 = unlimited).
func (mp *MessageProcessor) registerNewClient(clientID string, writer *bufio.Writer, format *MessageFormat) {
	if mp.server.maxClients > 0 && len(mp.server.clients) >= mp.server.maxClients {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Warn("mcp: max clients reached, not registering new client").
			String("client_id", clientID).
			Int("max_clients", mp.server.maxClients).
			Log()
		return
	}
	queueConfig := mp.server.getQueueConfig()
	queue := NewMessageQueue(writer, format, queueConfig)
	queue.SetMetricsCallback(func(depth, dropped, sent, errors int64) {
		mp.server.mcpMetrics.RecordQueueMetrics(depth, dropped, sent, errors)
	})
	mp.server.clients[clientID] = &ClientConnection{
		ID:       clientID,
		Writer:   writer,
		Format:   format,
		LastSeen: time.Now(),
		Queue:    queue,
	}
}

// handleParseError handles parse errors
// CRITICAL: Must use message queue to prevent interleaving with other writes
func (mp *MessageProcessor) handleParseError(parseErr error, format *MessageFormat, writer *bufio.Writer) error {
	resp := NewErrorResponse(nil, ParseError, "Parse error", nil)
	data, _ := resp.Marshal() //nolint:errcheck

	// CRITICAL: Use message queue to prevent interleaving with log notifications
	// Get current client ID and find their queue
	var currentClientID string
	_ = concurrency.RunInRLockWithLogger(
		&mp.server.clientIDMu, LockNameMcpMessageProcessorGetClientIdParseError, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			currentClientID = mp.server.clientID
			return nil
		},
	)

	var queue *MessageQueue
	if currentClientID != emptyValue {
		// Try to get client-specific queue
		client, exists := mp.server.findClientByID(currentClientID)
		if exists && client.Queue != nil {
			queue = client.Queue
		}
	}

	// If no client-specific queue, try default queue
	if queue == nil {
		queue = mp.server.findClientQueue()
	}

	// If queue is available, use it (ensures proper serialization)
	if queue != nil {
		// Parse error responses are high priority - they must be sent immediately
		if !queue.Enqueue(data, format, "high", nil) {
			// Queue full - this is unexpected, but handle gracefully
			// Fall back to direct write as last resort (parse errors are critical)
			mp.server.traceLogf("[MCP_WARN] Parse error response queue full, falling back to direct write (this may cause interleaving)")
			if writeErr := mp.transport.WriteMessage(writer, data, format); writeErr != nil {
				return errfmt.Newf("failed to write parse error response (direct fallback)").Wrap(writeErr)
			}
		}
		// Response queued successfully
		return nil // Continue serving after parse error
	}

	// No queue available - must write directly (last resort)
	// This should be rare - queue should always be available after client connection is established
	mp.server.traceLogf("[MCP_WARN] No message queue available for parse error response, writing directly (this may cause interleaving)")
	if writeErr := mp.transport.WriteMessage(writer, data, format); writeErr != nil {
		return errfmt.Newf("failed to write parse error response").Wrap(writeErr)
	}
	return nil // Continue serving after parse error
}

// handleNotification handles a notification (no response)
func (mp *MessageProcessor) handleNotification(ctx context.Context, req *JSONRPCRequest) error {
	// CRITICAL: Do NOT call SendLogDebug here - it queues a log notification
	// that can interleave with response writes, causing JSON parsing errors
	// Use traceLogf instead (doesn't go through message queue)
	mp.server.traceLogf("[MCP_DEBUG] Notification received: method=%s", req.Method)

	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				stackTrace := string(debug.Stack())
				panicErr := errfmt.Errorf("panic in notification handler: %v\n\nStack trace:\n%s", r, stackTrace)
				mp.server.traceLogf("[MCP_ERROR] ✗ PANIC in notification handler: method=%s, error=%v", req.Method, panicErr)
				_ = mp.server.SendLogError("Panic in notification handler", //nolint:errcheck
					map[string]any{
						"method":           req.Method,
						"panic":            fmt.Sprintf("%v", r),
						"stack_trace":      stackTrace,
						logMapKeyComponent: "mcp_server",
					})
				err = panicErr
			}
		}()
		_, err = mp.handler.Handle(ctx, req.Method, req.Params)
	}()

	if _, ok := err.(*NotificationSentinel); ok {
		// CRITICAL: Do NOT call SendLogDebug here - it queues a log notification
		// that can interleave with response writes, causing JSON parsing errors
		// Use traceLogf instead (doesn't go through message queue)
		mp.server.traceLogf("[MCP_DEBUG] Notification processed: method=%s state=handled", req.Method)
		return nil
	}

	if err != nil {
		if mp.server.getTraceWriter() != nil {
			mp.server.traceLogf("[MCP_ERROR] ✗ Notification error: method=%s, error=%v", req.Method, err)
		}
	}
	return nil
}

// handleRequest handles a request (needs response)
func (mp *MessageProcessor) handleRequest(ctx context.Context, req *JSONRPCRequest, format *MessageFormat, writer *bufio.Writer, operationTracker *OperationTracker) error {
	// CRITICAL: Do NOT call SendLogDebug here - it queues a log notification
	// that can interleave with the response write, causing JSON parsing errors
	// Use traceLogf instead (doesn't go through message queue)
	mp.server.traceLogf("[MCP_DEBUG] Request received: method=%s id=%v", req.Method, req.ID)

	// ITEM-645: rate limit check before handling
	if mp.server.rateLimiter != nil {
		perAccount := false
		if mp.server.config != nil {
			perAccount = mp.server.config.MCPServer.RateLimit.PerAccount
		}
		key := mp.server.getRateLimitKey(perAccount)
		if !mp.server.rateLimiter.Allow(key) {
			resp := NewErrorResponse(req.ID, RateLimitExceeded, "rate limit exceeded", map[string]any{"retry_after_seconds": 60})
			return mp.sendResponse(resp, format, writer, nil)
		}
	}

	// Extract actor context from _meta in params
	ctx = ExtractActorContext(ctx, req.Params)

	// Execute handler with panic recovery
	var result any
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				stackTrace := string(debug.Stack())
				panicErr := errfmt.Errorf("panic in request handler: %v\n\nStack trace:\n%s", r, stackTrace)
				mp.server.traceLogf("[MCP_ERROR] ✗ PANIC in request handler: method=%s, error=%v", req.Method, panicErr)
				_ = mp.server.SendLogError("Panic in request handler", //nolint:errcheck
					map[string]any{
						"method":           req.Method,
						"request_id":       req.ID,
						"panic":            fmt.Sprintf("%v", r),
						"stack_trace":      stackTrace,
						logMapKeyComponent: "mcp_server",
					})
				err = panicErr
			}
		}()
		result, err = mp.handler.Handle(ctx, req.Method, req.Params)
	}()

	// Handle special error types
	if err != nil {
		if elicitationErr, ok := err.(*ElicitationError); ok {
			_ = mp.handleElicitationError(req, elicitationErr)
			// Convert ElicitationError to JSONRPCError so buildResponse sends it to the client
			err = &JSONRPCError{
				Code:    InvalidParams,
				Message: elicitationErr.Message,
				Data: map[string]any{
					objects.FieldKeyParameters: elicitationErr.Parameters,
					"data":                     elicitationErr.Data,
					"error_type":               "elicitation",
				},
			}
		} else if _, ok := err.(*NotificationSentinel); ok {
			// Shouldn't happen for requests, but handle gracefully
			return nil
		}
	}

	// Build and send response
	resp := mp.buildResponse(req, result, err)

	// Set up callback for initialize response to send log notifications after write
	var onWriteComplete func()
	if req.Method == "initialize" {
		// IMPORTANT: Ensure client connection/queue is created now that clientID is set!
		mp.updateClientConnection(writer, format)

		// Capture values for callback
		clientID := mp.server.getClientIDWithRole()
		if clientID == emptyValue {
			_ = concurrency.RunInRLockWithLogger(
				&mp.server.clientIDMu, LockNameMcpMessageProcessorGetClientIdFallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					clientID = mp.server.clientID
					return nil
				},
			)
		}
		toolCount := mp.server.getToolCount()
		promptCount := mp.server.getPromptCount()

		onWriteComplete = func() {
			// Check shutdown before sending log
			if mp.server.shutdownFlag.Load() == 1 {
				return
			}
			select {
			case <-mp.server.shutdownCtx.Done():
				return
			default:
			}

			// CRITICAL: Small delay to ensure flush is fully complete
			// bufio.Writer.Flush() may return before OS-level write completes
			// This ensures the response is fully written before we enqueue log notifications
			time.Sleep(10 * time.Millisecond)

			// Check shutdown again after delay
			if mp.server.shutdownFlag.Load() == 1 {
				return
			}
			select {
			case <-mp.server.shutdownCtx.Done():
				return
			default:
			}

			// CRITICAL: Do NOT send log notifications from callback - they can interleave with responses
			// Even with the delay, if other responses are already queued, log notifications
			// will be written between them, causing JSON parsing errors
			// Use traceLogf instead (doesn't go through message queue, doesn't pollute stdout)
			mp.server.traceLogf("[MCP_DEBUG] Server initialization complete: initialized=%v tools_count=%d", mp.server.initialized.Load(), toolCount)

			if clientID != emptyValue {
				mp.server.traceLogf("[MCP_INFO] Client initialized and ready: client_id=%s tools=%d prompts=%d", clientID, toolCount, promptCount)
			}

			// NOTE: Log notifications via SendLogDebug/SendLogInfo are disabled to prevent interleaving
			// These were causing "},"level":"... errors when interleaving with responses
			// If log notifications are needed, they should be sent via a separate mechanism
			// that doesn't use the same message queue as responses
		}
	}

	return mp.sendResponse(resp, format, writer, onWriteComplete)
}

// handleElicitationError handles elicitation errors (sent as notifications)
func (mp *MessageProcessor) handleElicitationError(req *JSONRPCRequest, elicitationErr *ElicitationError) error {
	eventCtx := mp.server.getClientEventContext()
	fields := map[string]any{
		"method":  req.Method,
		"message": elicitationErr.Message,
	}
	if elicitationErr.Data != nil {
		if paramCount, ok := elicitationErr.Data["param_count"].(int); ok {
			fields["param_count"] = paramCount
		}
	}
	eventCtx.RecordEvent("elicitation_required", fields)

	if mp.server.getTraceWriter() != nil {
		clientTag := mp.server.getClientIDWithRole()
		if clientTag != emptyValue {
			clientTag = fmt.Sprintf(" client_id=%s", clientTag)
		}
		mp.server.traceLogf("[MCP_TRACE] ← Elicitation sent: method=%s%s", req.Method, clientTag)
	}
	return nil
}

// buildResponse builds a response from result or error
func (mp *MessageProcessor) buildResponse(req *JSONRPCRequest, result any, err error) *JSONRPCResponse {
	if err != nil {
		var code int
		var message string
		var data any

		if jsonrpcErr, ok := err.(*JSONRPCError); ok {
			code = jsonrpcErr.Code
			message = jsonrpcErr.Message
			data = jsonrpcErr.Data
		} else {
			code = InternalError
			message = err.Error()
			data = nil
		}
		return NewErrorResponse(req.ID, code, message, data)
	}

	resp := NewResponse(req.ID)
	resp.Result = result
	return resp
}

// sendResponse sends a response to the client
// CRITICAL: All writes must go through the message queue to prevent interleaving
// Direct writes can corrupt the JSON-RPC stream when log notifications are also being written
// onWriteComplete is an optional callback invoked after the response is successfully written
func (mp *MessageProcessor) sendResponse(resp *JSONRPCResponse, format *MessageFormat, writer *bufio.Writer, onWriteComplete func()) error {
	data, err := resp.Marshal()
	if err != nil {
		logErr := errfmt.Newf("failed to marshal response").Wrap(err)
		logTraceError("Failed to marshal response", logErr)
		reason := fmt.Sprintf("marshal error: %v", logErr)
		mp.server.traceLogf("[MCP_INFO] Shutdown triggered: %s", reason)
		mp.server.shutdownSequence(reason)
		return logErr
	}

	// CRITICAL: Use message queue to prevent interleaving with log notifications
	// Get current client ID and find their queue
	var currentClientID string
	_ = concurrency.RunInRLockWithLogger(
		&mp.server.clientIDMu, LockNameMcpMessageProcessorGetClientIdParseError, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			currentClientID = mp.server.clientID
			return nil
		},
	)

	var queue *MessageQueue
	if currentClientID != emptyValue {
		// Try to get client-specific queue
		client, exists := mp.server.findClientByID(currentClientID)
		if exists && client.Queue != nil {
			queue = client.Queue
		}
	}

	// If no client-specific queue, try default queue
	if queue == nil {
		queue = mp.server.findClientQueue()
	}

	// If queue is available, use it (ensures proper serialization)
	if queue != nil {
		// Responses are high priority - they must be sent immediately
		// Pass callback to be invoked after write completes
		if !queue.Enqueue(data, format, "high", onWriteComplete) {
			// Queue full - this is unexpected for responses, but handle gracefully
			// Fall back to direct write as last resort (responses are critical)
			mp.server.traceLogf("[MCP_WARN] Response queue full, falling back to direct write (this may cause interleaving)")
			if err := mp.transport.WriteMessage(writer, data, format); err != nil {
				if isBrokenPipeError(err) {
					mp.server.traceLogf("[MCP_DEBUG] Client disconnected during response write: %v", err)
					return err
				}
				return errfmt.Newf("failed to write response (direct fallback)").Wrap(err)
			}
		}
		// Response queued successfully
		// CRITICAL: Do NOT call SendLogDebug here - it queues a log notification
		// that could potentially interleave with the response write
		// Log notifications are non-critical and can be dropped if needed
		// Use traceLogf instead for debugging (doesn't go through message queue)
		mp.server.traceLogf("[MCP_DEBUG] Response queued: method=%v success=%v", resp.ID, resp.Error == nil)
		return nil
	}

	// No queue available - must write directly (last resort)
	// This should be rare - queue should always be available after client connection is established
	mp.server.traceLogf("[MCP_WARN] No message queue available for response, writing directly (this may cause interleaving)")
	if err := mp.transport.WriteMessage(writer, data, format); err != nil {
		// Check if this is a broken pipe or client disconnect - don't shutdown for these
		// Broken pipe errors are expected when client disconnects
		if isBrokenPipeError(err) {
			// Client disconnected - this is expected, just log and continue
			mp.server.traceLogf("[MCP_DEBUG] Client disconnected during response write: %v", err)
			return err // Return error but don't shutdown - let ServeLoop handle it
		}

		logErr := errfmt.Newf("failed to write response").Wrap(err)
		logTraceError("Failed to write response", logErr)
		reason := fmt.Sprintf("write error: %v", logErr)
		mp.server.traceLogf("[MCP_INFO] Shutdown triggered: %s", reason)
		mp.server.shutdownSequence(reason)
		return logErr
	}

	// CRITICAL: Do NOT call SendLogDebug here - direct write path should not queue log notifications
	// that could interfere with the response write
	// Use traceLogf instead for debugging (doesn't go through message queue)
	mp.server.traceLogf("[MCP_DEBUG] Response written directly: method=%v success=%v", resp.ID, resp.Error == nil)

	return nil
}

// isBrokenPipeError checks if an error is a broken pipe or connection error
// These errors are expected when the client disconnects and shouldn't trigger shutdown
func isBrokenPipeError(err error) bool {
	if err == nil {
		return false
	}

	// Check for EOF (client disconnect)
	if errors.Is(err, io.EOF) {
		return true
	}

	// Check for syscall.EPIPE (broken pipe)
	var errno syscall.Errno
	if errors.As(err, &errno) {
		if errno == syscall.EPIPE {
			return true
		}
	}

	// Check error message for common connection error patterns
	errStr := strings.ToLower(err.Error())
	brokenPipePatterns := []string{
		"broken pipe",
		"connection reset",
		"connection refused",
		"use of closed network connection",
		"write: broken pipe",
		"short write: wrote 0 of",
	}

	for _, pattern := range brokenPipePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}
