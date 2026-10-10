package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/zqk-os/zqk/pkg/objects"
)

// Client info field name constants
const (
	clientInfoName                = objects.FieldKeyName
	clientInfoVersion             = objects.FieldKeyVersion
	clientInfoAccountID           = objects.FieldKeyAccountID
	clientInfoSessionID           = "session_id"
	clientInfoKeystoreKeyID       = "keystore_key_id"
	clientInfoUsername            = objects.FieldKeyUsername
	clientInfoPassword            = "password"
	clientInfoOAuthTok            = "oauth_token"
	clientInfoPersonalAccessToken = "personal_access_token"
	clientInfoRoles               = objects.FieldKeyRoles
	clientInfoPermissions         = objects.FieldKeyPermissions
)

// Object field name constants (alias objects.FieldKey* so drift scans do not duplicate system keys).
const (
	fieldKind          = objects.FieldKeyKind
	fieldID            = objects.FieldKeyID
	fieldStatus        = objects.FieldKeyStatus
	fieldCreatedAt     = objects.FieldKeyCreatedAt
	fieldCreatedBy     = objects.FieldKeyCreatedBy
	fieldUpdatedAt     = objects.FieldKeyUpdatedAt
	fieldUpdatedBy     = objects.FieldKeyUpdatedBy
	fieldSchemaVersion = objects.FieldKeySchemaVersion
	fieldNamespaceID   = objects.FieldKeyNamespaceID
	fieldOriginProject = objects.FieldKeyOriginProject
	fieldOriginSystem  = objects.FieldKeyOriginSystem
)

// MCP session field name constants
const (
	sessionFieldClientID             = "client_id"
	sessionFieldClientName           = "client_name"
	sessionFieldAuthenticationStatus = "authentication_status"
	sessionFieldLastActivity         = "last_activity"
)

// MCP session authentication_status values (not object lifecycle ObjectStatus*).
const (
	mcpSessionAuthPending       = "pending_authentication"
	mcpSessionAuthAuthenticated = "authenticated"
)

// Authentication strategy name constants
const (
	authStrategyKeystore            = "keystore"
	authStrategyUsernamePassword    = "username_password"
	authStrategyOAuth               = "oauth"
	authStrategyPersonalAccessToken = "personal_access_token"
)

// Capability name constants
const (
	capabilityAllowedFormats = "allowed_formats"
)

// setupHandlers sets up all MCP method handlers using the MethodRouter
func (s *Server) setupHandlers() *MethodRouter {
	router := NewMethodRouter()

	// Register all MCP method handlers
	router.RegisterFunc("initialize", s.handleInitialize)
	router.RegisterFunc("ping", s.handlePing)
	router.RegisterFunc("tools/list", s.handleToolsList)
	router.RegisterFunc("resources/list", s.handleResourcesList)
	router.RegisterFunc("resources/get", s.handleResourcesGet)
	router.RegisterFunc("prompts/list", s.handlePromptsList)
	router.RegisterFunc("prompts/get", s.handlePromptsGet)
	router.RegisterFunc("roots/list", s.handleRootsList)
	router.RegisterFunc("tools/call", s.handleToolsCall)
	router.RegisterFunc("shutdown", s.handleShutdown)
	router.RegisterFunc("system/diagnostics", s.handleSystemDiagnostics)

	// Event subscription methods
	router.RegisterFunc("events/subscribe", s.handleEventsSubscribe)
	router.RegisterFunc("events/unsubscribe", s.handleEventsUnsubscribe)
	router.RegisterFunc("events/list", s.handleEventsList)

	// Notifications (fire-and-forget, no response)
	router.RegisterFunc("notifications/initialized", s.handleNotificationInitialized)
	router.RegisterFunc("notifications/cancelled", s.handleNotificationCancelled)
	// IPC / mesh wake: inbound notifications/event (PublishDaemonEvent) → EventEmitter.
	// Must not return MethodNotFound (-32601); that logged noise and paired with Writer
	// rebind bugs that dropped IDE's MCP session (core-backlog).
	router.RegisterFunc(notificationMethodEvent, s.handleNotificationEvent)
	router.RegisterFunc(notificationMethodMessage, s.handleNotificationMessage)

	return router
}

// handleInitialize handles the initialize method
func (s *Server) handleInitialize(ctx context.Context, _ string, params json.RawMessage) (any, error) {
	// Debug logging: log initialize call
	s.traceLogf("[MCP_DEBUG] initialize called: current_state=initialized:%v, tools_count=%d",
		s.initialized.Load(), s.getToolCount())

	// Prepare for re-initialization if needed
	s.prepareReinitialization()

	// Parse initialization parameters
	var initParams InitializeParams
	if err := json.Unmarshal(params, &initParams); err != nil {
		return nil, &JSONRPCError{
			Code:    ParseError,
			Message: "Parse error",
		}
	}

	// Setup client ID and sequence tracking
	clientID := s.setupClientID(initParams)
	sequenceID := s.getOrCreateSequenceID()
	s.recordConnectionEvent(sequenceID, clientID, initParams)

	// Extract client information from initialization parameters
	clientInfo := extractClientInfoFromInitParams(initParams)

	// Handle authentication flow (account resolution, credential validation, elicitation)
	clientInfo, accountID, err := s.handleAuthenticationFlow(ctx, clientID, clientInfo, initParams)
	if err != nil {
		{
			var e *ElicitationError
			var e1 *JSONRPCError
			switch {
			case errors.As(err, &e):
				return nil, e
			case errors.As(err, &e1):
				return nil, e1
			default:
				return nil, &JSONRPCError{Code: InvalidParams, Message: err.Error()}
			}
		}
	}

	// Apply role enforcement and security validation
	clientInfo, err = s.enforceRoleAndSecurity(clientInfo, accountID)
	if err != nil {
		return nil, &JSONRPCError{
			Code:    InvalidParams,
			Message: err.Error(),
		}
	}

	// Initialize security context
	secCtx := s.initializeSecurityContext(ctx, clientInfo)

	// Determine expected profile (for future use)
	var rolesForProfile []string
	if rolesAny, ok := clientInfo[clientInfoRoles].([]any); ok {
		for _, r := range rolesAny {
			if role, ok := r.(string); ok {
				rolesForProfile = append(rolesForProfile, role)
			}
		}
	}
	defaultContext := DefaultMCPContext
	if s.config != nil && s.config.MCPServer.Security.DefaultContext != emptyValue {
		defaultContext = s.config.MCPServer.Security.DefaultContext
	}
	agentConfig, _ := validateAgentRegistrationWithClientInfo(clientID, accountID, initParams.ClientInfo.Name, s.config) //nolint:errcheck // Validation errors handled separately
	_ = getExpectedProfile(agentConfig, rolesForProfile, defaultContext)                                                 // Store for future use

	// Configure client capabilities (allowed formats, etc.)
	s.configureClientCapabilities(initParams)

	// Use this client's security context for tool registration so CLI discovery is filtered by role/permissions.
	// Without this, BootstrapCLITools would use the server's default secCtx and all clients would see the same tools.
	s.SetSecurityContext(secCtx)

	// Register tools and resources (built-in + optional CLI; CLI set is filtered by secCtx above)
	s.registerToolsAndResources(secCtx)

	// Finalize initialization and return response
	return s.finalizeInitialization(clientID, sequenceID, initParams)
}

// List handlers moved to server_handlers_list.go
// Tool handler moved to server_handlers_tools.go

// handlePing responds to MCP/proxy liveness pings (proxy uses id __proxy_heartbeat__).
func (s *Server) handlePing(_ context.Context, _ string, _ json.RawMessage) (any, error) {
	return map[string]any{}, nil
}

// handleShutdown handles the shutdown method
func (s *Server) handleShutdown(_ context.Context, _ string, _ json.RawMessage) (any, error) {
	// Record event using context object
	eventCtx := s.getClientEventContext()
	eventCtx.RecordEvent("shutdown_requested", map[string]any{})

	s.RequestShutdown("shutdown requested")
	return map[string]any{
		"message": "Server shutdown requested",
	}, nil
}

// handleSystemDiagnostics handles the system/diagnostics method
func (s *Server) handleSystemDiagnostics(_ context.Context, _ string, _ json.RawMessage) (any, error) {
	provenance, configured, defaultCount := s.GetMCPSpecDiagnostics()
	return map[string]any{
		"mcp_spec": map[string]any{
			"provenance":       provenance,
			"configured_count": configured,
			"default_count":    defaultCount,
		},
	}, nil
}

// Notification handlers moved to server_handlers_notifications.go
// Event handlers moved to server_handlers_events.go
// Authentication handlers moved to server_handlers_auth.go
