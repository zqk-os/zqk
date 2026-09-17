package mcp

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// ServerConfig represents MCP server configuration loaded from .zqk/mcp/config.yaml
type ServerConfig struct {
	MCPServer struct {
		// ServerInfo externalizes server name and version (BLI-958)
		ServerInfo struct {
			Name    string `yaml:"name"`    // Server name (default: brand prefix)
			Version string `yaml:"version"` // Server version (default: "0.1.0")
		} `yaml:"server_info"`
		Trace struct {
			Enabled bool   `yaml:"enabled"`
			File    string `yaml:"file"`
			Rolling struct {
				Enabled  *bool `yaml:"enabled"`   // Enable rolling (nil = default true, false = disabled, true = enabled)
				MaxSize  int64 `yaml:"max_size"`  // Max file size in bytes before rotation (default: 10MB)
				MaxFiles int   `yaml:"max_files"` // Max number of rotated files to retain (default: 5)
			} `yaml:"rolling"`
		} `yaml:"trace"`
		// IdleTimeout is the maximum time without client activity before the server gracefully shuts down
		// Format: "5m", "10m", "30m", "1h", etc. (Go time.Duration format)
		// Default: "5m" (5 minutes)
		// Set to "0" to disable idle timeout (server runs indefinitely)
		IdleTimeout      string `yaml:"idle_timeout"`
		RegisterCLITools *bool  `yaml:"register_cli_tools"` // If nil/not set: true (default, backward compat). If false: only built-in tools. If true: register CLI tools.
		// Tools allowlist: when non-empty, only these tool names are exposed (e.g. to stay under IDE's 40/80 tool limit).
		// Tool names are the MCP names (e.g. zqk_object_list, zqk_system_status). Empty = expose all registered tools.
		Tools struct {
			Allowlist []string `yaml:"allowlist"`
			AliasMode *bool    `yaml:"alias_mode"` // When true, only built-in/alias tools are registered (no per-command CLI tools). Reduces tool count while keeping functionality.
		} `yaml:"tools"`
		ExposedCommands []string `yaml:"exposed_commands"`
		BlockedCommands []string `yaml:"blocked_commands"`
		WriteOperations []string `yaml:"write_operations"`
		// Async configuration for parallel operations
		Async struct {
			MaxConcurrent int    `yaml:"max_concurrent"` // Maximum concurrent operations (default: 10)
			Timeout       string `yaml:"timeout"`        // Operation timeout in Go duration format (default: 90s)
		} `yaml:"async"`
		// Event emitter configuration
		Events struct {
			BufferSize int `yaml:"buffer_size"` // Event buffer size per subscriber (default: 100)
		} `yaml:"events"`
		// MaxClients limits concurrent client connections (0 = unlimited). Prevents resource exhaustion in multi-client scenarios.
		MaxClients int `yaml:"max_clients"`
		// RateLimit (BLI-645): request rate limiting per window
		RateLimit struct {
			Enabled           bool `yaml:"enabled"`             // Enable rate limiting (default: false)
			RequestsPerMinute int  `yaml:"requests_per_minute"` // Max requests per minute (default: 60)
			PerAccount        bool `yaml:"per_account"`         // When true, limit per account_id; when false, global limit
		} `yaml:"rate_limit"`
		// Message queue configuration for backpressure relief
		MessageQueue struct {
			MaxQueueSize  int    `yaml:"max_queue_size"` // Maximum messages in queue (default: 1000)
			DropWhenFull  bool   `yaml:"drop_when_full"` // Drop messages when queue is full (default: true)
			FlushInterval string `yaml:"flush_interval"` // How often to flush (Go duration, default: "0" = immediate)
			WriteTimeout  string `yaml:"write_timeout"`  // Write timeout (Go duration, default: "5s")
		} `yaml:"message_queue"`
		// Resource URI scheme configuration
		Resources struct {
			URISchemeRules []ResourceURISchemeRuleConfig `yaml:"uri_scheme_rules"` // Custom URI scheme rules (optional, uses defaults if not specified)
		} `yaml:"resources"`
		// Error handling configuration
		ErrorHandling struct {
			CriticalErrorCodes []int `yaml:"critical_error_codes"` // Error codes that should trigger critical error notifications (optional, uses defaults if not specified)
		} `yaml:"error_handling"`
		// CLI bridge configuration
		CLI struct {
			PositionalArgumentNames []string `yaml:"positional_argument_names"` // Names of arguments that should be passed as positional args (default: ["id", "ids", "kind", "kinds", "path"])
			CommandGroups           []string `yaml:"command_groups"`            // Command group names used for path normalization (default: ["object", "system", "reports", "graph", "keystore", "scheduler"])
		} `yaml:"cli"`
		Security struct {
			RequireWritePermission bool     `yaml:"require_write_permission"`
			DefaultContext         string   `yaml:"default_context"`
			DefaultFormat          string   `yaml:"default_format"`
			AllowedFormats         []string `yaml:"allowed_formats"`       // Restrict clients to specific formats (empty = all allowed)
			PermissionOperations   []string `yaml:"permission_operations"` // Valid permission operations (default: read, write, delete, execute) (BLI-958)
			// Authentication strategies
			AuthStrategies []string `yaml:"auth_strategies"` // List of enabled auth strategy IDs (empty = use all enabled strategies)
			// Role enforcement for AI agents
			EnforcedRole        string   `yaml:"enforced_role"`         // Force all AI agents to use this role (overrides agent-provided roles)
			AllowedRoles        []string `yaml:"allowed_roles"`         // Whitelist of allowed roles (empty = all allowed)
			ValidateRoles       bool     `yaml:"validate_roles"`        // Validate roles against system role objects (requires storage)
			EnforceAccountRoles bool     `yaml:"enforce_account_roles"` // If account_id provided, enforce roles match account's assigned roles
			// Agent registry - pre-registered agents with expected roles
			RequireAccountID bool                   `yaml:"require_account_id"` // Require agents to provide account_id
			AgentRegistry    map[string]AgentConfig `yaml:"agent_registry"`     // Map of agent identifiers to expected roles/profiles
		} `yaml:"security"`
	} `yaml:"mcp_server"`
}

// AgentConfig defines the expected configuration for a registered agent
type AgentConfig struct {
	AccountID string   `yaml:"account_id"` // Expected account ID (must match)
	Roles     []string `yaml:"roles"`      // Expected roles (agent must use these)
	Profile   string   `yaml:"profile"`    // Expected profile (e.g., "ai-agent", "mcp")
	Required  bool     `yaml:"required"`   // If true, agent must be in registry to connect
}

// LoadMCPConfig loads MCP server configuration from .zqk/mcp/config.yaml
// Returns default config if file doesn't exist or can't be read
func LoadMCPConfig(projectRoot string) (*ServerConfig, error) {
	configPath := paths.MCPConfigPath(projectRoot)

	// Check if config file exists
	if _, err := fileutil.Stat(configPath); fileutil.IsNotExist(err) {
		// Return default config with safe defaults for greenfield (no init) scenarios
		defaultAliasMode := true
		cfg := &ServerConfig{}
		cfg.MCPServer.Tools.AliasMode = &defaultAliasMode
		return cfg, nil
	}

	// Read config file
	data, err := fileutil.ReadFile(configPath)
	if err != nil {
		return &ServerConfig{}, errfmt.Newf("failed to read config file").Wrap(err)
	}

	var cfg ServerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return &ServerConfig{}, errfmt.Newf("failed to parse config file").Wrap(err)
	}

	// Expand substitutions in config values
	configDir := filepath.Dir(configPath)
	expandConfigSubstitutions(&cfg, projectRoot, configDir)

	return &cfg, nil
}

// expandConfigSubstitutions expands variable substitutions in config values
// Supported variables:
//   - ${MCP_CONFIG_DIR} - directory containing config.yaml (.zqk/mcp)
//   - ${PROJECT_ROOT} - project root directory
//   - ${VAR_NAME} - environment variables
func expandConfigSubstitutions(config *ServerConfig, projectRoot, configDir string) {
	if config.MCPServer.Trace.File != emptyValue {
		config.MCPServer.Trace.File = expandPathSubstitutions(config.MCPServer.Trace.File, projectRoot, configDir)
	}
}

// expandPathSubstitutions expands variable substitutions in a path string
// Supported variables:
//   - ${MCP_CONFIG_DIR} - directory containing config.yaml (.zqk/mcp)
//   - ${PROJECT_ROOT} - project root directory
//   - ${VAR_NAME} - environment variables (e.g., ${HOME}, ${USER})
func expandPathSubstitutions(path, projectRoot, configDir string) string {
	path = strings.ReplaceAll(path, "${MCP_CONFIG_DIR}", configDir)
	path = strings.ReplaceAll(path, "$MCP_CONFIG_DIR", configDir)
	path = strings.ReplaceAll(path, "${PROJECT_ROOT}", projectRoot)
	path = strings.ReplaceAll(path, "$PROJECT_ROOT", projectRoot)
	path = expandEnvVars(path)
	return path
}

// expandEnvVars expands environment variable references in a string
// Supports both ${VAR} and $VAR syntax
func expandEnvVars(s string) string {
	for {
		start := strings.Index(s, "${")
		if start == -1 {
			break
		}
		end := strings.Index(s[start:], "}")
		if end == -1 {
			break
		}
		end += start
		varName := s[start+2 : end]
		varValue := os.Getenv(varName)
		s = s[:start] + varValue + s[end+1:]
	}

	start := 0
	for {
		nextStart := strings.Index(s[start:], "$")
		if nextStart == -1 {
			break
		}
		start += nextStart
		if start > 0 && s[start-1] == '{' {
			start++
			continue
		}
		end := start + 1
		for end < len(s) && ((s[end] >= 'a' && s[end] <= 'z') || (s[end] >= 'A' && s[end] <= 'Z') || (s[end] >= '0' && s[end] <= '9') || s[end] == '_') {
			end++
		}
		if end > start+1 {
			varName := s[start+1 : end]
			varValue := os.Getenv(varName)
			s = s[:start] + varValue + s[end:]
			start += len(varValue)
		} else {
			start++
		}
	}
	return s
}

// SaveMCPConfig saves the MCP server configuration back to .zqk/mcp/config.yaml
func SaveMCPConfig(projectRoot string, config *ServerConfig) error {
	configPath := paths.MCPConfigPath(projectRoot)
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	return fileutil.WriteStandardFile(configPath, data)
}
