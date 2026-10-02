package datacell

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var agentChatChannelConfigs stampmemo.Table[AgentChatChannelConfig] // keyed by projectRoot

var errInvalidAgentChatChannelEventsPath = errfmt.Errorf("agent chat channel events path empty")

// AgentChatChannelSchemaVersion is the config file schema_version for the pilot lite file.
const AgentChatChannelSchemaVersion = "1"

// Delivery mode values mirrored from agent_feed / lite config.
const (
	DeliveryModeOff       = "off"
	DeliveryModeLog       = "log"
	DeliveryModeClipboard = "clipboard"
	DeliveryModePaste     = "paste"
	DeliveryModeNotify    = "notify"
)

// AgentChatChannelConfig is the lite-file policy for the agent chat channel pilot (bounded JSON under .zqk/agent-runtime/).
// Steward / IDE integrations read this to enable or filter delivery; the append-only event stream is separate
// ([AgentChatChannelEventsJSONLPath]).
type AgentChatChannelConfig struct {
	SchemaVersion string `json:"schema_version"`
	Enabled       bool   `json:"enabled"`
	// DeliveryMode is optional coarse delivery when Enabled (off|log|clipboard|paste|notify). Empty defaults in hooks to paste.
	DeliveryMode string `json:"delivery_mode,omitempty"`
	// Note is optional operator context (not consumed by machinery by default).
	Note string `json:"note,omitempty"`
	// FeedID is set when this file was materialized from a CAS agent_feed (AGF-*).
	FeedID string `json:"feed_id,omitempty"`
	// MaterializedAt is RFC3339 UTC when materialized from agent_feed; empty for hand-written files.
	MaterializedAt string `json:"materialized_at,omitempty"`
	// ContractSchemaVersion echoes agent_feed.contract_schema_version (JSONL event contract).
	ContractSchemaVersion string `json:"contract_schema_version,omitempty"`
	// ConfigPathOverride and EventsJSONLPathOverride echo CAS when materialized; hooks may use events override for JSONL path.
	ConfigPathOverride      string `json:"config_path_override,omitempty"`
	EventsJSONLPathOverride string `json:"events_jsonl_path_override,omitempty"`
	// ProbeToolAllowlist limits postToolUse JSONL / delivery to these tool_name values (case-insensitive). Empty = no tool filter.
	// Used by .ide/hooks/post-tooluse-probe.sh; optional until CAS materialization wires them.
	ProbeToolAllowlist []string `json:"probe_tool_allowlist,omitempty"`
	// ProbeCommandSubstrings requires at least one substring (case-insensitive) in tool payload / command text. Empty = no substring filter.
	ProbeCommandSubstrings []string `json:"probe_command_substrings,omitempty"`
}

// DefaultAgentChatChannelConfig returns a safe default when no file exists (disabled).
func DefaultAgentChatChannelConfig() AgentChatChannelConfig {
	return AgentChatChannelConfig{
		SchemaVersion: AgentChatChannelSchemaVersion,
		Enabled:       false,
	}
}

// ReadAgentChatChannelConfig loads .zqk/agent-runtime/agent_chat_channel.json when present.
// A missing file returns [DefaultAgentChatChannelConfig] with a nil error.
func ReadAgentChatChannelConfig(projectRoot string) (AgentChatChannelConfig, error) {
	p := AgentChatChannelConfigPath(projectRoot)
	cfg, err := agentChatChannelConfigs.Load(projectRoot, stampmemo.Of(p), func() (AgentChatChannelConfig, error) {
		return readAgentChatChannelConfig(p)
	})
	if err != nil {
		return AgentChatChannelConfig{}, err
	}
	return cloneAgentChatChannelConfig(cfg), nil
}

func readAgentChatChannelConfig(p string) (AgentChatChannelConfig, error) {
	b, err := fileutil.ReadFile(p)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return DefaultAgentChatChannelConfig(), nil
		}
		return AgentChatChannelConfig{}, err
	}
	var c AgentChatChannelConfig
	if err := json.Unmarshal(b, &c); err != nil {
		return AgentChatChannelConfig{}, err
	}
	if c.SchemaVersion == "" {
		c.SchemaVersion = AgentChatChannelSchemaVersion
	}
	return c, nil
}

func cloneAgentChatChannelConfig(c AgentChatChannelConfig) AgentChatChannelConfig {
	c.ProbeToolAllowlist = append([]string(nil), c.ProbeToolAllowlist...)
	c.ProbeCommandSubstrings = append([]string(nil), c.ProbeCommandSubstrings...)
	return c
}

// WriteAgentChatChannelConfig writes the lite file to [AgentChatChannelConfigPath] with 0600.
func WriteAgentChatChannelConfig(projectRoot string, c AgentChatChannelConfig) error {
	p := AgentChatChannelConfigPath(projectRoot)
	if err := fileutil.WriteSecureJSONIndent(p, c); err != nil {
		return err
	}
	agentChatChannelConfigs.Delete(projectRoot)
	return nil
}

// EffectiveAgentChatChannelEventsJSONLPath returns the JSONL path for events: override from lite config when set, else default alias path.
func EffectiveAgentChatChannelEventsJSONLPath(projectRoot string, c AgentChatChannelConfig) string {
	o := strings.TrimSpace(c.EventsJSONLPathOverride)
	if o == "" {
		return AgentChatChannelEventsJSONLPath(projectRoot)
	}
	if filepath.IsAbs(o) {
		return filepath.Clean(o)
	}
	return filepath.Clean(filepath.Join(projectRoot, o))
}

// EnsureAgentChatChannelEventsDir creates the parent directory of the default JSONL event file (e.g. before first append).
func EnsureAgentChatChannelEventsDir(projectRoot string) error {
	p := AgentChatChannelEventsJSONLPath(projectRoot)
	if p == "" {
		return errInvalidAgentChatChannelEventsPath
	}
	if err := fileutil.MkdirAll(filepath.Dir(p), paths.DirPerm755); err != nil {
		return err
	}
	return nil
}

// EnsureAgentChatChannelEventsDirForConfig creates the parent directory for [EffectiveAgentChatChannelEventsJSONLPath].
func EnsureAgentChatChannelEventsDirForConfig(projectRoot string, c AgentChatChannelConfig) error {
	p := EffectiveAgentChatChannelEventsJSONLPath(projectRoot, c)
	if p == "" {
		return errInvalidAgentChatChannelEventsPath
	}
	return fileutil.MkdirAll(filepath.Dir(p), paths.DirPerm755)
}
