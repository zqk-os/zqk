package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/authcred"
	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/observer"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// prepareReinitialization handles re-initialization logic when server is already initialized
func (s *Server) prepareReinitialization() {
	if !s.initialized.Load() {
		// First initialization - don't clear tools
		s.traceLogf("[MCP_DEBUG] prepareReinitialization: first initialization, keeping existing tools (count=%d)", s.getToolCount())
		return
	}

	// TCP/proxy daemon: a second client's initialize must not wipe tools for live peers
	// (IDE proxy hang). TRACK: core-backlog — per-connection Server.
	if s.multiClient.Load() {
		s.traceLogf("[MCP_DEBUG] prepareReinitialization: multi-client mode, keeping tools (count=%d)", s.getToolCount())
		return
	}

	s.traceLogf("[MCP_DEBUG] Re-initializing: resetting state (was initialized)")
	s.initialized.Store(false)
	s.shutdownRequested.Store(false)
	s.shutdownMu.Lock()
	s.shutdownReason = emptyValue
	s.shutdownMu.Unlock()

	// Clear tools to allow re-registration
	oldToolCount := s.getToolCount()
	_ = concurrency.RunInLock(&s.toolsMu, func() error {
		s.tools = make(map[string]Tool)
		s.toolsCache.Store([]Tool{})     // Clear cache atomically
		s.promptsCache.Store([]Prompt{}) // Clear prompts cache too
		return nil
	})
	s.traceLogf("[MCP_DEBUG] Cleared tools for re-initialization (was %d tools)", oldToolCount)
}

// extractClientInfoFromInitParams extracts all client information from initialization parameters
func extractClientInfoFromInitParams(initParams InitializeParams) map[string]any {
	clientInfo := make(map[string]any)
	clientInfo[clientInfoName] = initParams.ClientInfo.Name
	clientInfo[clientInfoVersion] = initParams.ClientInfo.Version

	// Extract account ID from capabilities
	if id, ok := initParams.Capabilities[clientInfoAccountID].(string); ok {
		clientInfo[clientInfoAccountID] = id
	} else if id, ok := initParams.Capabilities["accountId"].(string); ok {
		clientInfo[clientInfoAccountID] = id
	}

	// Extract session_id from capabilities if present
	if sessionID, ok := initParams.Capabilities[objects.FieldKeySessionID].(string); ok && sessionID != emptyValue {
		clientInfo[clientInfoSessionID] = sessionID
	}

	// Extract authentication credentials
	if keyID, ok := initParams.Capabilities["keystore_key_id"].(string); ok && keyID != emptyValue {
		clientInfo[clientInfoKeystoreKeyID] = keyID
	} else if envKeyID := zqkenv.MCPKeystoreKeyID().Get(); envKeyID != emptyValue {
		clientInfo[clientInfoKeystoreKeyID] = envKeyID
	}
	if username, ok := initParams.Capabilities[objects.FieldKeyUsername].(string); ok && username != emptyValue {
		clientInfo[clientInfoUsername] = username
	}
	if password, ok := initParams.Capabilities["password"].(string); ok && password != emptyValue {
		clientInfo[clientInfoPassword] = password
	}
	if oauthToken, ok := initParams.Capabilities["oauth_token"].(string); ok && oauthToken != emptyValue {
		clientInfo[clientInfoOAuthTok] = oauthToken
	}
	if pat, ok := initParams.Capabilities["personal_access_token"].(string); ok && pat != emptyValue {
		clientInfo[clientInfoPersonalAccessToken] = pat
	}

	// Extract roles and permissions
	if roles, ok := initParams.Capabilities[objects.FieldKeyRoles].([]any); ok {
		clientInfo[clientInfoRoles] = roles
	}
	if perms, ok := initParams.Capabilities[objects.FieldKeyPermissions].([]any); ok {
		clientInfo[clientInfoPermissions] = perms
	}

	return clientInfo
}

// resolveAccountIDFromRegistry attempts to resolve account ID from agent registry by client name
func (s *Server) resolveAccountIDFromRegistry(clientName string, clientInfo map[string]any) string {
	if s.config == nil || len(s.config.MCPServer.Security.AgentRegistry) == 0 {
		return ""
	}

	// Try common patterns. Prefer ACC-* / canonicalized legacy account:username.
	// TRACK: follow-up in kernel backlog
	possibleAccountIDs := []string{
		clientName,
		strings.ToLower(clientName),
		fmt.Sprintf("%s%s", AccountIDPrefix, clientName),
		fmt.Sprintf("%s%s", AccountIDPrefix, strings.ToLower(clientName)),
	}
	if canon := authcred.CanonicalAccountID(s.GetProjectRoot(), "account:"+strings.ToLower(clientName)); canon != "" {
		possibleAccountIDs = append([]string{canon}, possibleAccountIDs...)
	}

	for _, possibleID := range possibleAccountIDs {
		agentConfig, ok := s.config.MCPServer.Security.AgentRegistry[possibleID]
		if !ok {
			continue
		}

		accountID := agentConfig.AccountID
		if accountID == emptyValue {
			accountID = possibleID
		}
		if canon := authcred.CanonicalAccountID(s.GetProjectRoot(), accountID); canon != "" {
			accountID = canon
		}

		clientInfo[clientInfoAccountID] = accountID

		// Set roles from registry
		if len(agentConfig.Roles) > 0 {
			rolesAny := make([]any, len(agentConfig.Roles))
			for i, role := range agentConfig.Roles {
				rolesAny[i] = role
			}
			clientInfo[clientInfoRoles] = rolesAny
		}

		return accountID
	}

	return ""
}

// setupClientID extracts or generates client ID and sets it on the server
func (s *Server) setupClientID(initParams InitializeParams) string {
	var clientID string
	if id, ok := initParams.Capabilities[objects.FieldKeyClientID].(string); ok && id != emptyValue {
		clientID = id
	} else {
		rolePrefix := DefaultClientIDPrefix
		clientID = fmt.Sprintf("%s%d_%d", rolePrefix, time.Now().UnixNano(), time.Now().Unix())
	}

	_ = concurrency.RunInLockWithLogger(
		&s.clientIDMu, LockNameMcpServerSetClientId, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			s.clientID = clientID
			return nil
		},
	)

	return clientID
}

// recordConnectionEvent records the connect event for a new connection
func (s *Server) recordConnectionEvent(sequenceID, clientID string, initParams InitializeParams) {
	var isNewConnection bool
	_ = concurrency.RunInRLock(&s.sequenceIDMu, func() error {
		isNewConnection = s.currentSequenceID == emptyValue
		return nil
	})

	if isNewConnection {
		connectClientID := clientID
		if connectClientID == emptyValue {
			connectClientID = fmt.Sprintf("%s%d", ConnectingClientIDPrefix, time.Now().UnixNano())
		}
		s.recordClientEvent(sequenceID, connectClientID, "connect", map[string]any{
			objects.FieldKeyClientName: initParams.ClientInfo.Name,
			"client_version":           initParams.ClientInfo.Version,
		})
	}
}

// handleAuthenticationFlow processes authentication and account resolution
// Returns updated clientInfo and accountID, or an error if authentication fails
func (s *Server) handleAuthenticationFlow(ctx context.Context, clientID string, clientInfo map[string]any, initParams InitializeParams) (map[string]any, string, error) {
	accountID, _ := clientInfo[clientInfoAccountID].(string)

	// In multiClient (TCP/network) mode, do NOT resolve accounts from registry without credentials
	if !s.multiClient.Load() && accountID == emptyValue {
		accountID = s.resolveAccountIDFromRegistry(initParams.ClientInfo.Name, clientInfo)
	}

	// Validate agent registration
	agentConfig, err := validateAgentRegistrationWithClientInfo(clientID, accountID, initParams.ClientInfo.Name, s.config)
	if err != nil {
		return nil, "", errfmt.Newf("agent registration validation failed").Wrap(err)
	}

	// Enforce agent registry
	if agentConfig != nil {
		clientInfo = enforceAgentRegistry(clientInfo, agentConfig)
		if enforcedAccountID, ok := clientInfo[clientInfoAccountID].(string); ok && enforcedAccountID != accountID {
			accountID = enforcedAccountID
		}
	}

	// Handle authentication if no account ID
	if accountID == emptyValue {
		// Check if valid session object is provided (stdio only)
		if !s.multiClient.Load() && clientID != emptyValue {
			if s.storageProvider != nil {
				systemSecCtx := pkgctx.NewSystemSecurityContext()
				sessionObj, readErr := s.storageProvider.Read(ctx, systemSecCtx, clientID)
				if readErr == nil && sessionObj != nil {
					k, _ := sessionObj[objects.FieldKeyKind].(string)
					if k == objects.KindMcpSession || k == "session" {
						if accID, exists := sessionObj[objects.FieldKeyAccountID].(string); exists && accID != "" {
							accountID = accID
							clientInfo[clientInfoAccountID] = accountID

							var projectRoot string
							if s.initCtx != nil {
								projectRoot = s.initCtx.ProjectRoot
							} else {
								projectRoot = s.GetProjectRoot()
							}

							if projectRoot != emptyValue {
								accountObj, loadErr := s.loadAccountObject(accountID, projectRoot)
								if loadErr == nil && accountObj != nil {
									roles := ExtractRolesFromAccount(accountObj)
									permissions := ExtractPermissionsFromAccount(accountObj)

									if len(roles) > 0 {
										rolesAny := make([]any, len(roles))
										for i, r := range roles {
											rolesAny[i] = r
										}
										clientInfo[clientInfoRoles] = rolesAny
									}
									if len(permissions) > 0 {
										permsAny := make([]any, len(permissions))
										for i, p := range permissions {
											permsAny[i] = p
										}
										clientInfo[clientInfoPermissions] = permsAny
									}
								}
							}
							return clientInfo, accountID, nil
						}
					}
				}
			}
		}

		// Check if system account (permitted only in local stdio mode)
		if !s.multiClient.Load() && clientID != emptyValue && (strings.HasPrefix(clientID, SystemAccountID) || strings.HasPrefix(clientID, SystemAccountPrefix)) {
			accountID = SystemAccountID
			clientInfo[clientInfoAccountID] = accountID
			return clientInfo, accountID, nil
		}

		// Check if human client without credentials (permitted only in local stdio mode)
		if !s.multiClient.Load() && isHumanClient(initParams.ClientInfo.Name, clientID) && !hasCredentials(clientInfo) {
			accountID = SystemAccountID
			clientInfo[clientInfoAccountID] = accountID
			return clientInfo, accountID, nil
		}

		// Handle credential-based authentication
		if hasCredentials(clientInfo) {
			return s.handleCredentialAuthentication(ctx, clientInfo)
		}

		// Named loopback membranes (ide-adapter / feed-steer) on TCP. Exact
		// client name only — not isHumanClient (that list is substring-based).
		if s.multiClient.Load() && isTrustedLoopbackAdapter(initParams.ClientInfo.Name, clientID) {
			accountID = SystemAccountID
			clientInfo[clientInfoAccountID] = accountID
			return clientInfo, accountID, nil
		}

		// In multiClient (TCP/network) mode, uncredentialed callers are rejected fail-closed (CRIT-CEF-R15-MCP-LOOPBACK-AUTH-001)
		if s.multiClient.Load() {
			return nil, "", &JSONRPCError{
				Code:    Unauthenticated,
				Message: "Authentication required: loopback/TCP MCP listener requires verified credentials or mTLS (CRIT-CEF-R15-MCP-LOOPBACK-AUTH-001)",
			}
		}

		// Elicit authentication for interactive stdio sessions
		return s.eliciteAuthentication(ctx, clientID, clientInfo, initParams)
	}

	// If accountID was already supplied on multiClient connection without credentials, require credentials
	if s.multiClient.Load() && !hasCredentials(clientInfo) {
		if isTrustedLoopbackAdapter(initParams.ClientInfo.Name, clientID) {
			return clientInfo, accountID, nil
		}
		return nil, "", &JSONRPCError{
			Code:    Unauthenticated,
			Message: "Authentication required: loopback/TCP MCP listener requires verified credentials for account (CRIT-CEF-R15-MCP-LOOPBACK-AUTH-001)",
		}
	}

	return clientInfo, accountID, nil
}

// handleCredentialAuthentication validates credentials and resolves account
func (s *Server) handleCredentialAuthentication(ctx context.Context, clientInfo map[string]any) (map[string]any, string, error) {
	resolvedAccountID, resolvedRoles, resolvedPerms, err := s.validateCredentialsAndResolveAccount(ctx, clientInfo)
	if err != nil {
		clientID := s.getClientIDWithRole()
		if clientID == emptyValue {
			_ = concurrency.RunInRLockWithLogger(
				&s.clientIDMu, LockNameMcpServerGetClientIdError, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					clientID = s.clientID
					return nil
				},
			)
		}
		_ = s.SendCriticalError( //nolint:errcheck // Best effort - error notification
			err,
			"error",
			"authentication",
			"Authentication failed",
			[]string{
				"Verify your credentials are correct (keystore key, username/password, etc.)",
				"Check that the account exists and is active",
				"Review authentication configuration in .zqk/mcp/config.yaml",
				"Check system logs for detailed error information",
			},
			map[string]any{
				objects.FieldKeyOperation: "authentication",
				"has_keystore_key":        clientInfo[clientInfoKeystoreKeyID] != nil,
				"has_username":            clientInfo[clientInfoUsername] != nil,
			},
			clientID,
			false,
		)
		return nil, "", errfmt.Newf("authentication failed").Wrap(err)
	}

	accountID := resolvedAccountID
	clientInfo[clientInfoAccountID] = accountID
	if len(resolvedRoles) > 0 {
		rolesAny := make([]any, len(resolvedRoles))
		for i, r := range resolvedRoles {
			rolesAny[i] = r
		}
		clientInfo[clientInfoRoles] = rolesAny
	}
	if len(resolvedPerms) > 0 {
		permsAny := make([]any, len(resolvedPerms))
		for i, p := range resolvedPerms {
			permsAny[i] = p
		}
		clientInfo[clientInfoPermissions] = permsAny
	}

	return clientInfo, accountID, nil
}

// eliciteAuthentication creates an elicitation error for missing authentication
func (s *Server) eliciteAuthentication(ctx context.Context, clientID string, clientInfo map[string]any, initParams InitializeParams) (map[string]any, string, error) {
	shouldElicitAccountID := s.config != nil && s.config.MCPServer.Security.RequireAccountID && len(s.config.MCPServer.Security.AgentRegistry) > 0
	isHumanClient := isHumanClient(initParams.ClientInfo.Name, clientID)

	if shouldElicitAccountID && !isHumanClient {
		return s.eliciteAccountID(ctx, clientID, clientInfo, initParams)
	}

	return s.eliciteCredentials(ctx, clientID, clientInfo, initParams)
}

func (s *Server) prepareAuthSession(ctx context.Context, clientID string, initParams InitializeParams) string {
	sessionID, sessionErr := s.createAuthenticationSession(ctx, clientID, initParams.ClientInfo.Name, "")
	if sessionErr != nil {
		logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
		logging.Fluent(logger).Warn("Failed to create authentication session").
			WithError(sessionErr).
			EmitComponent("mcp_server").
			String("client_id", clientID).
			Log()
	}
	if sessionID != emptyValue && !isHumanClient(initParams.ClientInfo.Name, clientID) {
		observer.NotifyAgentConnection(ctx, observer.AgentConnectionInfo{
			ClientID:   clientID,
			ClientName: initParams.ClientInfo.Name,
			Version:    initParams.ClientInfo.Version,
		})
	}
	return sessionID
}

func (s *Server) attachSessionToElicitationData(sessionID string, elicitationData map[string]any) {
	if sessionID != emptyValue {
		elicitationData[clientInfoSessionID] = sessionID
		elicitationData["authentication_required"] = true
	}
}

// eliciteAccountID creates an elicitation error for account ID
func (s *Server) eliciteAccountID(ctx context.Context, clientID string, clientInfo map[string]any, initParams InitializeParams) (map[string]any, string, error) {
	availableAccountIDs := make([]any, 0)
	if s.config != nil {
		for accountKey, agentConfig := range s.config.MCPServer.Security.AgentRegistry {
			displayAccountID := agentConfig.AccountID
			if displayAccountID == emptyValue {
				displayAccountID = accountKey
			}
			availableAccountIDs = append(availableAccountIDs, displayAccountID)
		}
	}

	sessionID := s.prepareAuthSession(ctx, clientID, initParams)

	message := "Account ID is required. Please provide your account_id to access the system."
	if len(availableAccountIDs) > 0 {
		message = fmt.Sprintf("Account ID is required. Please provide your account_id. Available accounts: %v", availableAccountIDs)
	}
	if sessionID != emptyValue {
		message = fmt.Sprintf("%s Session created: %s.", message, sessionID)
		s.SetCurrentSessionID(sessionID)
	}

	elicitationParams := []ElicitationParam{
		ElicitParamWithChoices(
			clientInfoAccountID,
			"Your account ID (e.g., ACC-…). This determines your role and permissions.",
			"string",
			true,
			availableAccountIDs,
		),
	}

	// Add credential alternatives
	enabledStrategies := s.enabledAuthStrategiesOrDefault(ctx, clientID)

	if enabledStrategies[authStrategyKeystore] {
		elicitationParams = append(elicitationParams, ElicitParam(
			clientInfoKeystoreKeyID,
			"Alternative: Keystore key ID from local vault (e.g., KEY-001). Use this if you have a key established in the keystore instead of account_id.",
			"string",
			false,
		))
	}
	if enabledStrategies[authStrategyUsernamePassword] {
		elicitationParams = append(elicitationParams,
			ElicitParam(clientInfoUsername, "Alternative: Your username or email address for authentication (required if using username/password instead of account_id)", "string", false),
			ElicitParam(clientInfoPassword, "Alternative: Your password (required if using username/password authentication instead of account_id)", "string", false),
		)
	}

	elicitationData := map[string]any{
		"require_account_id": true,
		"available_accounts": availableAccountIDs,
	}
	s.attachSessionToElicitationData(sessionID, elicitationData)

	return nil, "", NewElicitationErrorWithData(message, elicitationParams, elicitationData)
}

// eliciteCredentials creates an elicitation error for credentials
func (s *Server) eliciteCredentials(ctx context.Context, clientID string, clientInfo map[string]any, initParams InitializeParams) (map[string]any, string, error) {
	sessionID := s.prepareAuthSession(ctx, clientID, initParams)

	message := "Authentication required. Please provide your credentials to access the system."
	if sessionID != emptyValue {
		message = fmt.Sprintf("Authentication required. Session created: %s. Please provide your credentials to authenticate.", sessionID)
		s.SetCurrentSessionID(sessionID)
	}

	enabledStrategies := s.enabledAuthStrategiesOrDefault(ctx, clientID)

	elicitationParams := []ElicitationParam{}
	if enabledStrategies[authStrategyKeystore] {
		elicitationParams = append(elicitationParams, ElicitParam(
			clientInfoKeystoreKeyID,
			"Keystore key ID from local vault (e.g., KEY-001). Use this if you have a key established in the keystore.",
			"string",
			false,
		))
	}
	if enabledStrategies[authStrategyUsernamePassword] {
		elicitationParams = append(elicitationParams,
			ElicitParam(clientInfoUsername, "Your username or email address for authentication (required if using username/password)", "string", false),
			ElicitParam(clientInfoPassword, "Your password (required if using username/password authentication)", "string", false),
		)
	}
	if enabledStrategies[authStrategyOAuth] {
		elicitationParams = append(elicitationParams, ElicitParam(
			clientInfoOAuthTok,
			"OAuth 2.0 access token (required if using OAuth authentication)",
			"string",
			false,
		))
	}
	if enabledStrategies[authStrategyPersonalAccessToken] {
		elicitationParams = append(elicitationParams, ElicitParam(
			clientInfoPersonalAccessToken,
			"Personal Access Token (PAT) for programmatic access (required if using PAT authentication)",
			"string",
			false,
		))
	}

	elicitationData := map[string]any{}
	s.attachSessionToElicitationData(sessionID, elicitationData)

	return nil, "", NewElicitationErrorWithData(message, elicitationParams, elicitationData)
}

// enabledAuthStrategiesOrDefault loads auth strategies, falling back to keystore +
// username/password when load fails or nothing is enabled (empty process dir under
// Local CI / minimal fixtures must still produce elicitation parameters).
func (s *Server) enabledAuthStrategiesOrDefault(ctx context.Context, clientID string) map[string]bool {
	defaults := map[string]bool{
		authStrategyKeystore:            true,
		authStrategyUsernamePassword:    true,
		authStrategyOAuth:               false,
		authStrategyPersonalAccessToken: false,
	}
	enabledStrategies, err := s.loadEnabledAuthStrategies(ctx)
	if err != nil {
		logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
		logging.Fluent(logger).Warn("Failed to load auth strategies").
			WithError(err).
			EmitComponent("mcp_server").
			String("client_id", clientID).
			Log()
		return defaults
	}
	for _, on := range enabledStrategies {
		if on {
			return enabledStrategies
		}
	}
	return defaults
}

// enforceRoleAndSecurity applies role enforcement and validates roles
func (s *Server) enforceRoleAndSecurity(clientInfo map[string]any, accountID string) (map[string]any, error) {
	projectRoot := emptyValue
	if s.initCtx != nil {
		projectRoot = s.initCtx.ProjectRoot
	}

	enforcedClientInfo, err := enforceRoleEnforcement(clientInfo, s.config, projectRoot, s)
	if err != nil {
		clientID := s.getClientIDWithRole()
		if clientID == emptyValue {
			_ = concurrency.RunInRLockWithLogger(
				&s.clientIDMu, LockNameMcpServerGetClientIdError, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
				func() error {
					clientID = s.clientID
					return nil
				},
			)
		}
		_ = s.SendCriticalError( //nolint:errcheck // Best effort - error notification
			err,
			"error",
			"authorization",
			"Role enforcement failed during initialization",
			[]string{
				"Check that your account_id references a valid account object",
				"Verify that the account has appropriate roles assigned",
				"Review the MCP server configuration in .zqk/mcp/config.yaml",
				"Check system logs for detailed error information",
			},
			map[string]any{
				clientInfoAccountID:       accountID,
				objects.FieldKeyOperation: "initialize",
			},
			clientID,
			false,
		)
		return nil, errfmt.Newf("role enforcement failed").Wrap(err)
	}

	// Validate roles against system if configured
	if s.config != nil && s.config.MCPServer.Security.ValidateRoles {
		if err := s.validateRolesAgainstSystem(enforcedClientInfo); err != nil {
			return nil, errfmt.Newf("role validation failed").Wrap(err)
		}
	}

	return enforcedClientInfo, nil
}

// validateRolesAgainstSystem validates roles against system role objects
func (s *Server) validateRolesAgainstSystem(clientInfo map[string]any) error {
	var rolesToValidate []string
	switch v := clientInfo[clientInfoRoles].(type) {
	case []any:
		for _, r := range v {
			role, ok := r.(string)
			if !ok {
				continue
			}
			rolesToValidate = append(rolesToValidate, role)
		}
	case string:
		rolesToValidate = strings.Split(v, ",")
		for i, r := range rolesToValidate {
			rolesToValidate[i] = strings.TrimSpace(r)
		}
	}

	if len(rolesToValidate) == 0 {
		return nil
	}

	projectRoot := emptyValue
	if s.initCtx != nil {
		projectRoot = s.initCtx.ProjectRoot
	}

	return validateRolesAgainstSystem(rolesToValidate, projectRoot)
}

// initializeSecurityContext creates and sets the security context from client info
func (s *Server) initializeSecurityContext(ctx context.Context, clientInfo map[string]any) *pkgctx.SecurityContext {
	secCtx := InitializeSecurityContextFromMCP(clientInfo)
	s.SetSecurityContext(secCtx)

	// Track current session ID so we can mark it disconnected on client disconnect
	if sessionID, ok := clientInfo[clientInfoSessionID].(string); ok && sessionID != emptyValue {
		s.SetCurrentSessionID(sessionID)
	}
	// Update authentication session if it exists
	if sessionID, ok := clientInfo[clientInfoSessionID].(string); ok && sessionID != emptyValue {
		goroutinelabels.NewGoroutine("mcp_auth_session_updater", fmt.Sprintf("updating authentication session %s", sessionID)).
			StartSimple(func() {
				if err := s.updateAuthenticationSession(ctx, sessionID, secCtx, clientInfo); err != nil && s.getTraceWriter() != nil {
					logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
					logging.Fluent(logger).Warn("Failed to update authentication session").
						String("session_id", sessionID).
						WithError(err).
						EmitComponent("mcp_server").
						String("trace", "true").
						Log()
				}
			})
	}

	return secCtx
}

// configureClientCapabilities extracts and sets allowed formats from client capabilities
func (s *Server) configureClientCapabilities(initParams InitializeParams) {
	_ = concurrency.RunInLock(&s.clientIDMu, func() error {
		if s.config != nil {
			s.allowedFormats = s.config.MCPServer.Security.AllowedFormats
		}
		return nil
	})

	// Extract from client capabilities
	switch v := initParams.Capabilities[capabilityAllowedFormats].(type) {
	case []any:
		allowedFormats := make([]string, 0, len(v))
		for _, f := range v {
			format, ok := f.(string)
			if !ok {
				continue
			}
			allowedFormats = append(allowedFormats, format)
		}
		if len(allowedFormats) > 0 {
			s.SetAllowedFormats(allowedFormats)
		}
	case string:
		formats := strings.Split(v, ",")
		allowedFormats := make([]string, 0, len(formats))
		for _, f := range formats {
			format := strings.TrimSpace(f)
			if format != emptyValue {
				allowedFormats = append(allowedFormats, format)
			}
		}
		if len(allowedFormats) > 0 {
			s.SetAllowedFormats(allowedFormats)
		}
	}

	// Fall back to config if client didn't specify
	if s.config != nil && len(s.config.MCPServer.Security.AllowedFormats) > 0 {
		s.SetAllowedFormats(s.config.MCPServer.Security.AllowedFormats)
	}
}

func (s *Server) registerCoreBuiltinTools(secCtx *pkgctx.SecurityContext) {
	RegisterGraphTools(s)
	RegisterEchoTool(s)
	RegisterCommonTools(s)
	RegisterInteractiveTools(s)
	RegisterWorkflowTools(s, secCtx)
	RegisterMetricsTools(s)
	RegisterReportTools(s)
	RegisterAgentExecutionTools(s)
	RegisterObserverTools(s)
	RegisterChatInjectTool(s)
	RegisterIdeBridgeTool(s)
}

func (s *Server) shouldBootstrapCLITools() bool {
	shouldRegister := true
	if s.config != nil && s.config.MCPServer.RegisterCLITools != nil {
		shouldRegister = *s.config.MCPServer.RegisterCLITools
	}
	aliasMode := s.config != nil && s.config.MCPServer.Tools.AliasMode != nil && *s.config.MCPServer.Tools.AliasMode
	return s.rootCommand != nil && shouldRegister && !aliasMode
}

// registerToolsAndResources registers all tools and resources for the server
func (s *Server) registerToolsAndResources(secCtx *pkgctx.SecurityContext) {
	// Build permission cache asynchronously
	if s.permissionCache != nil {
		if pc, ok := s.permissionCache.(*PermissionCache); ok {
			goroutinelabels.NewGoroutine("mcp_permission_cache_builder", "building permission cache asynchronously").
				StartSimple(func() {
					_ = pc.BuildPermissionCache(secCtx) //nolint:errcheck
				})
		}
	}

	// Register built-in tools synchronously
	// Log tool count before registration for debugging
	beforeCount := s.getToolCount()
	s.traceLogf("[MCP_DEBUG] registerToolsAndResources: starting with %d tools", beforeCount)

	s.registerCoreBuiltinTools(secCtx)
	RegisterOnboardingPrompts(s)

	// Log tool count after registration for debugging
	afterCount := s.getToolCount()
	s.traceLogf("[MCP_DEBUG] registerToolsAndResources: registered tools, now have %d tools (was %d)", afterCount, beforeCount)

	// Log actual tool names for debugging
	var toolNames []string
	_ = concurrency.RunInRLockWithLogger(
		&s.toolsMu, LockNameMcpServerRegisterToolsDebug, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			toolNames = make([]string, 0, len(s.tools))
			for name := range s.tools {
				toolNames = append(toolNames, name)
			}
			return nil
		},
	)
	s.traceLogf("[MCP_DEBUG] registerToolsAndResources: tool names: %v", toolNames)

	defaultToolCount := s.getToolCount()
	defaultPromptCount := s.getPromptCount()
	defaultResourceCount := s.getResourceCount()

	// Execute resource discovery and CLI tool bootstrapping in parallel
	executor := NewParallelExecutor()

	executor.Execute(func() error {
		if s.storageProvider != nil {
			loader := NewStorageMCPSpecLoader(s.storageProvider)
			specs, err := loader.LoadMCPSpecs(pkgctx.NewSystemContext(), secCtx)
			if err == nil && len(specs) > 0 {
				_ = ApplyMCPSpecs(s, specs)
			}
		}
		return nil
	})

	executor.Execute(func() error {
		RegisterCriticalResources(s)
		return nil
	})

	executor.Execute(func() error {
		DiscoverAdditionalResources(s)
		return nil
	})

	// Bootstrap CLI tools in parallel if enabled (and not in alias_mode: alias mode exposes only built-in tools)
	if s.shouldBootstrapCLITools() {
		executor.Execute(func() error {
			if err := s.BootstrapCLITools(); err != nil {
				if s.getTraceWriter() != nil {
					logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
					logging.Fluent(logger).Error("Failed to bootstrap CLI tools", err).
						EmitComponent("mcp_server").
						String("trace", "true").
						Log()
				}
				return nil // Don't block initialization
			}
			return nil
		})
	}

	// Wait for parallel operations
	// CRITICAL: Errors in parallel operations (like RegisterCriticalResources) should NOT
	// prevent tool registration from completing. Tools are registered synchronously above,
	// so they should always be available even if resource registration fails.
	if err := executor.Wait(); err != nil {
		logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
		// Log all errors (including panics) - don't require trace to be enabled
		allErrors := executor.AllErrors()
		if len(allErrors) > 0 {
			logging.Fluent(logger).Error("Parallel initialization operations failed", err).
				EmitComponent("mcp_server").
				Int("error_count", len(allErrors)).
				Log()
			// Log each error individually for better debugging
			for i, e := range allErrors {
				logging.Fluent(logger).Error(fmt.Sprintf("Initialization error %d/%d", i+1, len(allErrors)), e).
					EmitComponent("mcp_server").
					Log()
			}
		} else {
			logging.Fluent(logger).Warn("Some parallel initialization operations failed").
				WithError(err).
				EmitComponent("mcp_server").
				Log()
		}
	}

	// Apply tools allowlist if configured (trim to curated set for IDE/client limits)
	if s.config != nil && len(s.config.MCPServer.Tools.Allowlist) > 0 {
		beforeAllowlist := s.getToolCount()
		s.ApplyToolsAllowlist(s.config.MCPServer.Tools.Allowlist)
		afterAllowlist := s.getToolCount()
		s.traceLogf("[MCP_DEBUG] registerToolsAndResources: applied tools allowlist, %d -> %d tools", beforeAllowlist, afterAllowlist)
	}

	// CRITICAL: Verify tools are still registered after parallel operations
	// This is a defensive check to ensure parallel operation errors didn't clear tools
	finalToolCount := s.getToolCount()
	s.traceLogf("[MCP_DEBUG] registerToolsAndResources: completed, final tool count=%d", finalToolCount)
	if finalToolCount == 0 {
		s.traceLogf("[MCP_ERROR] CRITICAL: No tools registered after registerToolsAndResources! This should never happen.")
		// Log to system logger as well (doesn't require trace)
		logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
		logging.Fluent(logger).Error("CRITICAL: No tools registered after registerToolsAndResources",
			errfmt.Errorf("tool count is 0 after registration")).
			EmitComponent("mcp_server").
			String("trace", "true").
			Log()
	}

	totalToolCount := s.getToolCount()
	totalPromptCount := s.getPromptCount()
	totalResourceCount := s.getResourceCount()
	provenance := MCPSpecProvenanceBootstrapFallback
	if s.storageProvider != nil {
		provenance = MCPSpecProvenanceKernelStorage
	}

	logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
	logging.Fluent(logger).Info("MCP server resource and tool registration complete").
		Int("default_tools", defaultToolCount).
		Int("configured_tools", totalToolCount-defaultToolCount).
		Int("default_prompts", defaultPromptCount).
		Int("configured_prompts", totalPromptCount-defaultPromptCount).
		Int("default_resources", defaultResourceCount).
		Int("configured_resources", totalResourceCount-defaultResourceCount).
		String("source_provenance", provenance).
		EmitComponent("mcp_server").
		Log()
}

// registerToolsOnly registers built-in tools, optionally CLI-discovered tools, and applies
// tools allowlist. Used by ListExposedTools for discovery (no resources, no permission cache).
func (s *Server) registerToolsOnly(secCtx *pkgctx.SecurityContext) {
	s.registerCoreBuiltinTools(secCtx)

	if s.shouldBootstrapCLITools() {
		if err := s.BootstrapCLITools(); err != nil {
			// Don't fail discovery; tool list will be built-in only
			if s.getTraceWriter() != nil {
				logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
				logging.Fluent(logger).Debug("BootstrapCLITools failed during registerToolsOnly").
					WithError(err).
					Log()
			}
		}
	}

	if s.config != nil && len(s.config.MCPServer.Tools.Allowlist) > 0 {
		s.ApplyToolsAllowlist(s.config.MCPServer.Tools.Allowlist)
	}
}

// finalizeInitialization completes initialization and returns the response
func (s *Server) finalizeInitialization(clientID, sequenceID string, initParams InitializeParams) (InitializeResult, error) {
	protocolVersion := negotiateProtocolVersion(initParams.ProtocolVersion)

	toolCountBeforeInit := s.getToolCount()
	s.traceLogf("[MCP_DEBUG] About to set initialized=true: tools_count=%d", toolCountBeforeInit)

	s.initialized.Store(true)
	s.traceLogf("[MCP_INFO] MCP server ready: tools_count=%d", toolCountBeforeInit)

	// Log notifications are now sent via callback after initialize response is written
	// This is handled in message_processor.go sendResponse() for "initialize" method
	// No need for delayed goroutine - callback is invoked exactly when response is written

	// Record initialize event
	toolCountAfterInit := s.getToolCount()
	initializeClientID := clientID
	if initializeClientID == emptyValue {
		_ = concurrency.RunInRLockWithLogger(
			&s.clientIDMu, LockNameMcpServerInitializeGetClientId, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				initializeClientID = s.clientID
				return nil
			},
		)
	}
	if sequenceID != emptyValue {
		s.recordClientEvent(sequenceID, initializeClientID, "initialize", map[string]any{
			"tools_count":              toolCountAfterInit,
			objects.FieldKeyClientName: initParams.ClientInfo.Name,
			"client_version":           initParams.ClientInfo.Version,
		})
	}

	// Get server name and version from config or use defaults (BLI-958)
	serverName := GetBrandPrefix()
	serverVersion := "0.1.0"
	if s.config != nil {
		if s.config.MCPServer.ServerInfo.Name != emptyValue {
			serverName = s.config.MCPServer.ServerInfo.Name
		}
		if s.config.MCPServer.ServerInfo.Version != emptyValue {
			serverVersion = s.config.MCPServer.ServerInfo.Version
		}
	}

	// Advertise elicitation only for non-IDE seats. IDE treats
	// capabilities.elicitation.enabled as an auth gate and hangs on
	// "Authenticating…" even when initialize already succeeded without credentials.
	capabilities := map[string]any{
		"tools":     map[string]any{},
		"prompts":   map[string]any{},
		"resources": map[string]any{},
		"logging": map[string]any{
			"level": "info",
		},
		"roots": map[string]any{},
	}
	if !isHumanClient(initParams.ClientInfo.Name, clientID) {
		capabilities["elicitation"] = map[string]any{
			objects.FieldKeyEnabled: true,
		}
	}

	return InitializeResult{
		ProtocolVersion: protocolVersion,
		Capabilities:    capabilities,
		ServerInfo: struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{
			Name:    serverName,
			Version: serverVersion,
		},
		ClientID: clientID,
	}, nil
}
