package mcp

import (
	"reflect"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

// createTestServerConfigForRegistry creates a ServerConfig for testing agent registry
func createTestServerConfigForRegistry(requireAccountID bool, registry map[string]AgentConfig) *ServerConfig {
	config := &ServerConfig{}
	config.MCPServer.Security.RequireAccountID = requireAccountID
	config.MCPServer.Security.AgentRegistry = registry
	return config
}

func TestValidateAgentRegistrationWithClientInfo(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		clientID    string
		accountID   string
		clientName  string
		config      *ServerConfig
		wantConfig  *AgentConfig
		wantError   bool
		errorSubstr string
	}{
		{
			name:       "nil config allows all",
			clientID:   "test-client",
			accountID:  "test-account",
			clientName: "test-client",
			config:     nil,
			wantConfig: nil,
			wantError:  false,
		},
		{
			name:       "empty registry allows all",
			clientID:   "test-client",
			accountID:  "test-account",
			clientName: "test-client",
			config:     createTestServerConfigForRegistry(false, map[string]AgentConfig{}),
			wantConfig: nil,
			wantError:  false,
		},
		{
			name:       "found by account_id",
			clientID:   "test-client",
			accountID:  "ACC-TEST-AGENT",
			clientName: "test-client",
			config: createTestServerConfigForRegistry(false, map[string]AgentConfig{
				"ACC-TEST-AGENT": {
					AccountID: "ACC-TEST-AGENT",
					Roles:     []string{"test_agent"},
					Profile:   "ai-agent",
				},
			}),
			wantConfig: &AgentConfig{
				AccountID: "ACC-TEST-AGENT",
				Roles:     []string{"test_agent"},
				Profile:   "ai-agent",
			},
			wantError: false,
		},
		{
			name:       "found by client_id when account_id not found",
			clientID:   "client:test_agent",
			accountID:  "",
			clientName: "test-client",
			config: createTestServerConfigForRegistry(false, map[string]AgentConfig{
				"client:test_agent": {
					AccountID: "ACC-TEST-AGENT",
					Roles:     []string{"test_agent"},
					Profile:   "ai-agent",
				},
			}),
			wantConfig: &AgentConfig{
				AccountID: "ACC-TEST-AGENT",
				Roles:     []string{"test_agent"},
				Profile:   "ai-agent",
			},
			wantError: false,
		},
		{
			name:       "require_account_id rejects non-human client without account_id",
			clientID:   "ai-agent-client",
			accountID:  "",
			clientName: "ai-agent",
			config: createTestServerConfigForRegistry(true, map[string]AgentConfig{
				"ACC-TEST-AGENT": {
					AccountID: "ACC-TEST-AGENT",
					Roles:     []string{"test_agent"},
				},
			}),
			wantConfig:  nil,
			wantError:   true,
			errorSubstr: "account_id is required",
		},
		{
			name:       "require_account_id allows human client without account_id",
			clientID:   "ide-seat-01",
			accountID:  "",
			clientName: "ide-seat-01",
			config: createTestServerConfigForRegistry(true, map[string]AgentConfig{
				"ACC-TEST-AGENT": {
					AccountID: "ACC-TEST-AGENT",
					Roles:     []string{"test_agent"},
				},
			}),
			wantConfig: nil,
			wantError:  false,
		},
		{
			name:       "unregistered agent allowed when not in registry",
			clientID:   "unregistered-client",
			accountID:  "",
			clientName: "unregistered-client",
			config: createTestServerConfigForRegistry(false, map[string]AgentConfig{
				"account:other_agent": {
					AccountID: "account:other_agent",
					Roles:     []string{"other_agent"},
				},
			}),
			wantConfig: nil,
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotConfig, err := validateAgentRegistrationWithClientInfo(tt.clientID, tt.accountID, tt.clientName, tt.config)

			if tt.wantError {
				if err == nil {
					t.Errorf("validateAgentRegistrationWithClientInfo() expected error but got none")
					return
				}
				if tt.errorSubstr != emptyValue && !strings.Contains(err.Error(), tt.errorSubstr) {
					t.Errorf("validateAgentRegistrationWithClientInfo() error = %v, want error containing %q", err, tt.errorSubstr)
				}
			} else {
				if err != nil {
					t.Errorf("validateAgentRegistrationWithClientInfo() unexpected error = %v", err)
					return
				}
			}

			if !reflect.DeepEqual(gotConfig, tt.wantConfig) {
				t.Errorf("validateAgentRegistrationWithClientInfo() gotConfig = %v, want %v", gotConfig, tt.wantConfig)
			}
		})
	}
}

func TestIsHumanClient(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		clientName  string
		clientID    string
		want        bool
		description string
	}{
		{
			name:        "ide-seat-01 in name",
			clientName:  "ide-seat-01",
			clientID:    "some-id",
			want:        true,
			description: "Should detect ide-seat-01 in client name",
		},
		{
			name:        "ide-seat-01 in id",
			clientName:  "some-name",
			clientID:    "ide-seat-01",
			want:        true,
			description: "Should detect ide-seat-01 in client ID",
		},
		{
			name:        "vscode in name",
			clientName:  "vscode",
			clientID:    "some-id",
			want:        true,
			description: "Should detect vscode in client name",
		},
		{
			name:        "ide in name",
			clientName:  "ide",
			clientID:    "some-id",
			want:        true,
			description: "Should detect ide in client name",
		},
		{
			name:        "code in name",
			clientName:  "code",
			clientID:    "some-id",
			want:        true,
			description: "Should detect code in client name",
		},
		{
			name:        "neovim in name",
			clientName:  "neovim",
			clientID:    "some-id",
			want:        true,
			description: "Should detect neovim in client name",
		},
		{
			name:        "vim in name",
			clientName:  "vim",
			clientID:    "some-id",
			want:        true,
			description: "Should detect vim in client name",
		},
		{
			name:        "emacs in name",
			clientName:  "emacs",
			clientID:    "some-id",
			want:        true,
			description: "Should detect emacs in client name",
		},
		{
			name:        "intellij in name",
			clientName:  "intellij",
			clientID:    "some-id",
			want:        true,
			description: "Should detect intellij in client name",
		},
		{
			name:        "idea in name",
			clientName:  "idea",
			clientID:    "some-id",
			want:        true,
			description: "Should detect idea in client name",
		},
		{
			name:        "sublime in name",
			clientName:  "sublime",
			clientID:    "some-id",
			want:        true,
			description: "Should detect sublime in client name",
		},
		{
			name:        "atom in name",
			clientName:  "atom",
			clientID:    "some-id",
			want:        true,
			description: "Should detect atom in client name",
		},
		{
			name:        "case insensitive - IDE-SEAT",
			clientName:  "IDE-SEAT",
			clientID:    "some-id",
			want:        true,
			description: "Should be case insensitive",
		},
		{
			name:        "case insensitive - IDE-VSCode",
			clientName:  "IDE-VSCode",
			clientID:    "some-id",
			want:        true,
			description: "Should be case insensitive",
		},
		{
			name:        "partial match - my-ide-editor",
			clientName:  "my-ide-editor",
			clientID:    "some-id",
			want:        true,
			description: "Should match partial strings",
		},
		{
			name:        "ai agent not human",
			clientName:  "ai-agent",
			clientID:    "ai-agent-client",
			want:        false,
			description: "Should not detect AI agent as human",
		},
		{
			name:        "unknown client not human",
			clientName:  "unknown-client",
			clientID:    "unknown-id",
			want:        false,
			description: "Should not detect unknown client as human",
		},
		{
			name:        "empty strings not human",
			clientName:  "",
			clientID:    "",
			want:        false,
			description: "Should not detect empty strings as human",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHumanClient(tt.clientName, tt.clientID)
			if got != tt.want {
				t.Errorf("isHumanClient(%q, %q) = %v, want %v - %s", tt.clientName, tt.clientID, got, tt.want, tt.description)
			}
		})
	}
}

func TestEnforceAgentRegistry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		clientInfo  map[string]any
		agentConfig *AgentConfig
		want        map[string]any
		description string
	}{
		{
			name:        "nil agentConfig returns original",
			clientInfo:  map[string]any{sessionFieldClientID: "test"},
			agentConfig: nil,
			want:        map[string]any{sessionFieldClientID: "test"},
			description: "Should return original clientInfo when agentConfig is nil",
		},
		{
			name:       "enforce account_id",
			clientInfo: map[string]any{sessionFieldClientID: "test"},
			agentConfig: &AgentConfig{
				AccountID: "ACC-TEST-AGENT",
			},
			want: map[string]any{
				sessionFieldClientID: "test",
				clientInfoAccountID:  "ACC-TEST-AGENT",
			},
			description: "Should enforce account_id from registry",
		},
		{
			name:       "enforce roles",
			clientInfo: map[string]any{sessionFieldClientID: "test"},
			agentConfig: &AgentConfig{
				Roles: []string{"test_agent", "developer"},
			},
			want: map[string]any{
				sessionFieldClientID:  "test",
				objects.FieldKeyRoles: []any{"test_agent", "developer"},
			},
			description: "Should enforce roles from registry",
		},
		{
			name:       "enforce both account_id and roles",
			clientInfo: map[string]any{sessionFieldClientID: "test", "existing": "value"},
			agentConfig: &AgentConfig{
				AccountID: "ACC-TEST-AGENT",
				Roles:     []string{"test_agent"},
			},
			want: map[string]any{
				sessionFieldClientID:  "test",
				"existing":            "value",
				clientInfoAccountID:   "ACC-TEST-AGENT",
				objects.FieldKeyRoles: []any{"test_agent"},
			},
			description: "Should enforce both account_id and roles",
		},
		{
			name:       "override existing account_id",
			clientInfo: map[string]any{clientInfoAccountID: "wrong", sessionFieldClientID: "test"},
			agentConfig: &AgentConfig{
				AccountID: "ACC-TEST-AGENT",
			},
			want: map[string]any{
				clientInfoAccountID:  "ACC-TEST-AGENT",
				sessionFieldClientID: "test",
			},
			description: "Should override existing account_id",
		},
		{
			name:       "override existing roles",
			clientInfo: map[string]any{objects.FieldKeyRoles: []any{"wrong"}, sessionFieldClientID: "test"},
			agentConfig: &AgentConfig{
				Roles: []string{"test_agent"},
			},
			want: map[string]any{
				objects.FieldKeyRoles: []any{"test_agent"},
				sessionFieldClientID:  "test",
			},
			description: "Should override existing roles",
		},
		{
			name:       "empty roles slice",
			clientInfo: map[string]any{sessionFieldClientID: "test"},
			agentConfig: &AgentConfig{
				Roles: []string{},
			},
			want: map[string]any{
				sessionFieldClientID: "test",
			},
			description: "Should not add roles when empty slice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := enforceAgentRegistry(tt.clientInfo, tt.agentConfig)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("enforceAgentRegistry() = %v, want %v - %s", got, tt.want, tt.description)
			}
		})
	}
}

func TestGetExpectedProfile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		agentConfig    *AgentConfig
		roles          []string
		defaultProfile string
		want           string
		description    string
	}{
		{
			name:           "explicit profile in registry",
			agentConfig:    &AgentConfig{Profile: "custom-profile"},
			roles:          []string{"developer"},
			defaultProfile: "ai-agent",
			want:           "custom-profile",
			description:    "Should use explicit profile from registry",
		},
		{
			name:           "admin role maps to mcp",
			agentConfig:    nil,
			roles:          []string{"admin"},
			defaultProfile: "ai-agent",
			want:           "mcp",
			description:    "Should map admin role to mcp profile",
		},
		{
			name:           "founder role maps to mcp",
			agentConfig:    nil,
			roles:          []string{"founder"},
			defaultProfile: "ai-agent",
			want:           "mcp",
			description:    "Should map founder role to mcp profile",
		},
		{
			name:           "executive role maps to ai-agent",
			agentConfig:    nil,
			roles:          []string{"executive"},
			defaultProfile: "",
			want:           "ai-agent",
			description:    "Should map executive role to ai-agent profile",
		},
		{
			name:           "owner role maps to ai-agent",
			agentConfig:    nil,
			roles:          []string{"owner"},
			defaultProfile: "",
			want:           "ai-agent",
			description:    "Should map owner role to ai-agent profile",
		},
		{
			name:           "developer role maps to ai-agent",
			agentConfig:    nil,
			roles:          []string{"developer"},
			defaultProfile: "",
			want:           "ai-agent",
			description:    "Should map developer role to ai-agent profile",
		},
		{
			name:           "viewer role maps to ai-agent",
			agentConfig:    nil,
			roles:          []string{"viewer"},
			defaultProfile: "",
			want:           "ai-agent",
			description:    "Should map viewer role to ai-agent profile",
		},
		{
			name:           "observer_agent role maps to ai-agent",
			agentConfig:    nil,
			roles:          []string{"observer_agent"},
			defaultProfile: "",
			want:           "ai-agent",
			description:    "Should map observer_agent role to ai-agent profile",
		},
		{
			name:           "test_agent role maps to ai-agent",
			agentConfig:    nil,
			roles:          []string{"test_agent"},
			defaultProfile: "",
			want:           "ai-agent",
			description:    "Should map test_agent role to ai-agent profile",
		},
		{
			name:           "coder_agent role maps to ai-agent",
			agentConfig:    nil,
			roles:          []string{"coder_agent"},
			defaultProfile: "",
			want:           "ai-agent",
			description:    "Should map coder_agent role to ai-agent profile",
		},
		{
			name:           "collective role maps to ai-agent",
			agentConfig:    nil,
			roles:          []string{"collective"},
			defaultProfile: "",
			want:           "ai-agent",
			description:    "Should map collective role to ai-agent profile",
		},
		{
			name:           "case insensitive role matching",
			agentConfig:    nil,
			roles:          []string{"ADMIN"},
			defaultProfile: "",
			want:           "mcp",
			description:    "Should match roles case insensitively",
		},
		{
			name:           "multiple roles - first match wins",
			agentConfig:    nil,
			roles:          []string{"developer", "admin"},
			defaultProfile: "",
			want:           "ai-agent",
			description:    "Should use first matching role",
		},
		{
			name:           "unknown role uses default profile",
			agentConfig:    nil,
			roles:          []string{"unknown_role"},
			defaultProfile: "custom-default",
			want:           "custom-default",
			description:    "Should use default profile for unknown roles",
		},
		{
			name:           "unknown role no default uses ai-agent",
			agentConfig:    nil,
			roles:          []string{"unknown_role"},
			defaultProfile: "",
			want:           "ai-agent",
			description:    "Should default to ai-agent for unknown roles",
		},
		{
			name:           "empty roles uses default profile",
			agentConfig:    nil,
			roles:          []string{},
			defaultProfile: "custom-default",
			want:           "custom-default",
			description:    "Should use default profile when no roles",
		},
		{
			name:           "nil roles uses default profile",
			agentConfig:    nil,
			roles:          nil,
			defaultProfile: "custom-default",
			want:           "custom-default",
			description:    "Should use default profile when roles is nil",
		},
		{
			name:           "explicit profile overrides role mapping",
			agentConfig:    &AgentConfig{Profile: "mcp"},
			roles:          []string{"developer"},
			defaultProfile: "ai-agent",
			want:           "mcp",
			description:    "Explicit profile should override role-based mapping",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getExpectedProfile(tt.agentConfig, tt.roles, tt.defaultProfile)
			if got != tt.want {
				t.Errorf("getExpectedProfile() = %v, want %v - %s", got, tt.want, tt.description)
			}
		})
	}
}

func TestIsTrustedLoopbackAdapterExactMatchOnly(t *testing.T) {
	t.Parallel()

	if !isTrustedLoopbackAdapter(IDEProxySubscriberClientID, "") {
		t.Fatal("ide-ide-proxy must be trusted")
	}
	if !isTrustedLoopbackAdapter("", feedSteerProbeClientID) {
		t.Fatal("zqk-feed-steer must be trusted")
	}
	for _, name := range []string{"unauthenticated-agent", "ide", "composer", "vscode", "peer-agent-01"} {
		if isTrustedLoopbackAdapter(name, name) {
			t.Fatalf("%q must not punch a TCP auth hole", name)
		}
	}
}
