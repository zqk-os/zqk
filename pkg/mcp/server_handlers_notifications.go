package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/logging"
)

// handleNotificationInitialized handles the notifications/initialized notification
// This is the trigger to establish a unique client ID for notification routing
// Notifications are fire-and-forget, so we return nil and handle internally
func (s *Server) handleNotificationInitialized(_ context.Context, _ string, params json.RawMessage) (any, error) {
	// Log client initialization - this is a critical state change
	// Extract any params if provided (some clients send initialization data)
	var initParams map[string]any
	if len(params) > 0 {
		//nolint:errcheck // Ignore errors - params may be empty or malformed
		_ = json.Unmarshal(params, &initParams)
	}

	// Record event (before client ID is set, so we'll use the connecting ID)
	eventCtx := s.getClientEventContext()
	clientID := eventCtx.GetClientID()
	if clientID == emptyValue {
		clientID = fmt.Sprintf("connecting_%d", time.Now().UnixNano())
	}
	eventCtx.RecordEventWithClientID("notification_initialized", clientID, map[string]any{})

	// Update client ID with role prefix if not already set with role
	// ClientID should already be set from initialize, but we can add role prefix if needed
	// IMPORTANT: Only add role prefix to generated client IDs (starting with "client_"),
	// not to client-provided IDs from initialize capabilities
	var wasNewClient bool
	var currentClientID string
	_ = concurrency.RunInLockWithLogger(
		&s.clientIDMu, LockNameMcpServerNotificationInitialized, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			wasNewClient = s.clientID == emptyValue
			currentClientID = s.clientID

			// Only add role prefix if:
			// 1. Client ID exists
			// 2. Client ID was generated (starts with "client_") - not provided by client
			// 3. Client ID doesn't already have a role prefix (doesn't contain "_client_")
			// This preserves client-provided IDs from initialize capabilities
			if currentClientID != emptyValue && strings.HasPrefix(currentClientID, "client_") && !strings.Contains(currentClientID, "_client_") {
				var rolePrefix string
				if s.secCtx != nil {
					if secCtx, ok := s.secCtx.(interface{ GetRoles() []string }); ok {
						roles := secCtx.GetRoles()
						if len(roles) > 0 {
							rolePrefix = strings.Join(roles, "_") + "_"
						}
					}
				}
				if rolePrefix != emptyValue {
					// Update clientID with role prefix (only for generated IDs)
					s.clientID = fmt.Sprintf("%s%s", rolePrefix, currentClientID)
				}
			} else if currentClientID == emptyValue {
				// Fallback: generate if somehow not set (shouldn't happen after initialize fix)
				var rolePrefix string
				if s.secCtx != nil {
					if secCtx, ok := s.secCtx.(interface{ GetRoles() []string }); ok {
						roles := secCtx.GetRoles()
						if len(roles) > 0 {
							rolePrefix = strings.Join(roles, "_") + "_"
						}
					}
				}
				if rolePrefix == emptyValue {
					rolePrefix = DefaultClientIDPrefix
				} else {
					rolePrefix += DefaultClientIDPrefix
				}
				s.clientID = fmt.Sprintf("%s%d_%d", rolePrefix, time.Now().UnixNano(), time.Now().Unix())
			}
			clientID = s.clientID
			return nil
		},
	)

	// Flush buffered events now that clientID is available
	// Get sequenceID for flushing buffered events
	sequenceID := s.getOrCreateSequenceID()
	if s.clientMetricsStore != nil {
		_ = s.clientMetricsStore.UpdateClientID(sequenceID, clientID) //nolint:errcheck // Non-critical
	}

	// Get client ID with role prefix for logging (may differ from stored clientID if roles changed)
	clientIDWithRole := s.getClientIDWithRole()
	logClientID := clientIDWithRole
	if logClientID == emptyValue {
		logClientID = clientID // Fallback to stored clientID if getClientIDWithRole returns empty
	}

	// Switch to client-specific trace file for new clients
	// This creates separate trace logs per client for easier debugging in multi-agent scenarios
	if wasNewClient {
		// Check if trace logging is enabled before creating client-specific trace file
		config := s.config
		traceEnabled := false
		if config != nil {
			traceEnabled = config.MCPServer.Trace.Enabled
		}
		// Also check environment variable
		if traceEnv := os.Getenv(zqkenv.MCPTrace()); traceEnv != emptyValue {
			traceEnabled = traceEnv != "0" && traceEnv != "false" && traceEnv != "off" && traceEnv != "no"
		}
		if traceEnabled {
			s.switchToClientSpecificTrace(logClientID)
		}
	}

	// Log notifications are now sent via callback after initialize response is written
	// This is handled in message_processor.go sendResponse() for "initialize" method
	// No need for delayed goroutine - callback is invoked exactly when response is written

	// Client is now ready - we can route notifications to this client ID
	// This ID can be used in events/subscribe and other notification routing

	// Send welcome message to client (non-blocking, errors are ignored)
	// This prompts the client to introduce itself
	// CRITICAL: Use process group manager to track this goroutine
	// This ensures it can be controlled during shutdown
	_, _ = s.processGroupManager.SpawnGoroutine(
		"welcome-message",
		"Welcome Message Sender",
		"Sends welcome message to client after initialization",
		false, // Not critical - can be cancelled during shutdown
		func(ctx context.Context) {
			// Small delay to ensure transport writer is set from the main serve loop
			select {
			case <-ctx.Done():
				return // Shutdown ordered, exit immediately
			case <-time.After(DefaultLogNotificationTimeout):
				// Continue after delay
			}

			// Try sending with retry (transport might not be ready immediately)
			maxRetries := DefaultNotificationMaxRetries
			for i := 0; i < maxRetries; i++ {
				// CRITICAL: Check shutdown context on EACH loop iteration
				// Process group manager provides the context, which is cancelled on shutdown
				select {
				case <-ctx.Done():
					return // Shutdown ordered, exit immediately
				default:
					// Continue with retry
				}

				// CRITICAL: Final shutdown check immediately before SendMessageToClient
				// This prevents any lock acquisition during shutdown
				// Check both atomic flag and context for maximum reliability
				if s.shutdownFlag.Load() == 1 {
					return // Shutdown ordered, exit immediately
				}
				select {
				case <-ctx.Done():
					return // Shutdown ordered, exit immediately
				default:
				}

				err := s.SendMessageToClient(
					"Welcome! Please introduce yourself and let me know how I can help you today.",
					"welcome",
					"medium",
				)
				if err == nil {
					// Success - message sent
					clientIDWithRole := s.getClientIDWithRole()
					logClientID := clientIDWithRole
					if logClientID == emptyValue {
						logClientID = clientID // Fallback to stored clientID
					}
					if s.getTraceWriter() != nil {
						s.traceLogf("[MCP_INFO] ← Welcome message sent to client: client_id=%s", logClientID)
					}
					return // Success, exit goroutine
				}
				// Log error to trace if available (for debugging)
				s.traceLogf("[MCP_WARN] ✗ Welcome message send attempt %d/%d failed: %v", i+1, maxRetries, err)
				if i < maxRetries-1 {
					// Wait before retry, but check shutdown context
					select {
					case <-ctx.Done():
						return // Shutdown ordered during wait, exit immediately
					case <-time.After(200 * time.Millisecond):
						// Continue retry
					}
				}
			}
		},
	)

	// No response needed for notifications - fire-and-forget per JSON-RPC 2.0 spec
	// Return special sentinel value to indicate no response should be sent
	return nil, &NotificationSentinel{}
}

// handleNotificationCancelled handles the notifications/cancelled notification.
// If requestId is present, the in-flight operation is cancelled so the server stops
// blocking on that request (e.g. tool call subprocess is killed and handler returns).
func (s *Server) handleNotificationCancelled(_ context.Context, _ string, params json.RawMessage) (any, error) {
	var cancelParams CancelledParams
	//nolint:errcheck // Unmarshal errors ignored intentionally
	_ = json.Unmarshal(params, &cancelParams) // Ignore unmarshal errors - params may be empty

	// Record event using context object
	eventCtx := s.getClientEventContext()
	eventCtx.RecordEvent("notification_cancelled", map[string]any{
		objects.FieldKeyReason: cancelParams.Reason,
	})

	// Cancel in-flight operation by request ID so we stop blocking (subprocess killed, handler returns)
	if cancelParams.RequestID != nil {
		reqID := fmt.Sprintf("%v", cancelParams.RequestID)
		if s.operationTracker != nil && s.operationTracker.CancelOperation(reqID) {
			s.traceLogf("[MCP_DEBUG] Cancelled in-flight operation: request_id=%s reason=%s", reqID, cancelParams.Reason)
		}
	}

	if strings.EqualFold(cancelParams.Reason, "client_shutdown") {
		s.RequestShutdown("client requested shutdown via cancellation")
	}

	// No response needed for notifications
	return nil, &NotificationSentinel{}
}
