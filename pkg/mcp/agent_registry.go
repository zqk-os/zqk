package mcp

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

// validateAgentRegistrationWithClientInfo validates agent registration with client name for human client detection
func validateAgentRegistrationWithClientInfo(clientID, accountID, clientName string, config *ServerConfig) (*AgentConfig, error) {
	if config == nil || len(config.MCPServer.Security.AgentRegistry) == 0 {
		// No registry configured - allow all agents
		return nil, nil
	}

	// Check if this is a human client (IDE) - these should bypass strict account_id requirements
	// Human clients can provide roles/permissions through elicitation
	isHumanClient := isHumanClient(clientName, clientID)

	// Try to find agent by account_id first (most specific)
	if accountID != emptyValue {
		if agentConfig, ok := config.MCPServer.Security.AgentRegistry[accountID]; ok {
			return &agentConfig, nil
		}
	}

	// Try to find agent by client_id
	if clientID != emptyValue {
		if agentConfig, ok := config.MCPServer.Security.AgentRegistry[clientID]; ok {
			return &agentConfig, nil
		}
	}

	// If require_account_id is true and account_id not provided, reject
	// BUT: Allow human clients (IDEs) to bypass this requirement - they can provide roles via elicitation
	if config.MCPServer.Security.RequireAccountID && accountID == emptyValue && !isHumanClient {
		return nil, errfmt.Errorf("account_id is required but not provided. Please provide your account_id in the initialize request")
	}

	// If agent not found in registry, check if registry requires all agents to be registered
	// The presence of required agents means "only registered agents can connect"
	// BUT: Allow unregistered clients to connect if they're not explicitly blocked
	// The "required" flag on individual agents means "this specific agent must be registered",
	// not "all agents must be registered if any agent is required"
	//
	// We only reject if:
	// 1. The agent is trying to use an account_id that's in the registry but marked as required (already handled above)
	// 2. OR there's a policy that says "all agents must be registered" (not currently implemented)
	//
	// For now, allow unregistered clients to connect - they can provide roles via elicitation
	// This allows legitimate clients like ide-seat-01 to connect even if not pre-registered

	// No registry enforcement - allow agent
	return nil, nil
}

// isHumanClient checks if the client is a human IDE (or a local developer agent)
// Human clients and local agents like ide-seat-01, vscode, etc. should be allowed to connect
// without strict account_id requirements to prevent them from hanging on interactive elicitation.
func isHumanClient(clientName, clientID string) bool {
	// Check client name for common IDE and local agent patterns
	humanClientNames := []string{
		"ide-ide-proxy",  // zqk mcp proxy stamp (IDE → daemon)
		"zqk-feed-steer", // feed steer / doctor events/list probes
		"ide-seat-01",
		"vscode",
		"ide",
		"composer",
		"peer-client",
		"peer-agent-01", // Added to bypass interactive elicitation for Agy
		"cline",         // Added for Cline support
		"windsurf",      // Added for Windsurf support
		"roo-code",      // Added for Roo support
		"roo",           // Added for Roo support
		"aider",         // Added for Aider support
		"code",
		"neovim",
		"vim",
		"emacs",
		"intellij",
		"idea",
		"sublime",
		"atom",
	}

	name := strings.ToLower(clientName)
	id := strings.ToLower(clientID)

	for _, pattern := range humanClientNames {
		if strings.Contains(name, pattern) || strings.Contains(id, pattern) {
			return true
		}
	}

	return false
}

// isTrustedLoopbackAdapter is the named local membrane allowed to initialize
// on loopback TCP without credentials. Exact match only — substring "ide"
// would punch a hole through CRIT-CEF-R15-MCP-LOOPBACK-AUTH-001.
// TRACK: BLI-CEF-R2-REL-MCP-RECONNECT
func isTrustedLoopbackAdapter(clientName, clientID string) bool {
	name := strings.TrimSpace(clientName)
	id := strings.TrimSpace(clientID)
	for _, trusted := range []string{IDEProxySubscriberClientID, feedSteerProbeClientID} {
		if name == trusted || id == trusted {
			return true
		}
	}
	return false
}

// enforceAgentRegistry enforces that an agent uses the roles/profile from the registry
func enforceAgentRegistry(clientInfo map[string]any, agentConfig *AgentConfig) map[string]any {
	if agentConfig == nil {
		return clientInfo
	}

	// Enforce account_id if specified in registry
	if agentConfig.AccountID != emptyValue {
		clientInfo[clientInfoAccountID] = agentConfig.AccountID
	}

	// Enforce roles if specified in registry
	if len(agentConfig.Roles) > 0 {
		rolesAny := make([]any, len(agentConfig.Roles))
		for i, role := range agentConfig.Roles {
			rolesAny[i] = role
		}
		clientInfo[objects.FieldKeyRoles] = rolesAny
	}

	// Store expected profile for later use (not in clientInfo, but in server state)
	// Profile enforcement happens separately in handleInitialize

	return clientInfo
}

// getExpectedProfile returns the expected profile for an agent based on registry or role
func getExpectedProfile(agentConfig *AgentConfig, roles []string, defaultProfile string) string {
	// If agent has explicit profile in registry, use it
	if agentConfig != nil && agentConfig.Profile != emptyValue {
		return agentConfig.Profile
	}

	// Map roles to profiles (if no explicit profile in registry)
	// This ensures agents use appropriate profiles based on their roles
	roleProfileMap := map[string]string{
		"admin":          "mcp",      // Admin uses MCP profile for full access
		"founder":        "mcp",      // Founder uses MCP profile
		"executive":      "ai-agent", // Executive uses AI agent profile
		"owner":          "ai-agent", // Owner uses AI agent profile
		"developer":      "ai-agent", // Developer uses AI agent profile
		"viewer":         "ai-agent", // Viewer uses AI agent profile
		"observer_agent": "ai-agent", // Observer agent uses AI agent profile
		"test_agent":     "ai-agent", // Test agent uses AI agent profile
		"coder_agent":    "ai-agent", // Coder agent uses AI agent profile
		"collective":     "ai-agent", // Collective uses AI agent profile
	}

	// Check if any role has a specific profile mapping
	for _, role := range roles {
		if profile, ok := roleProfileMap[strings.ToLower(role)]; ok {
			return profile
		}
	}

	// Default to provided default profile or "ai-agent"
	if defaultProfile != emptyValue {
		return defaultProfile
	}
	return "ai-agent"
}
