package ideadapter

import (
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Config controls the IDE-facing adapter and its private daemon session.
type Config struct {
	DaemonTCP                 string        `yaml:"daemon_tcp"`
	AdvertiseElicitation      bool          `yaml:"advertise_elicitation"`
	AutoSubscribeEventTypes   []string      `yaml:"auto_subscribe_event_types"`
	ClientInfoName            string        `yaml:"client_info_name"`
	ClientInfoVersion         string        `yaml:"client_info_version"`
	HeartbeatInterval         time.Duration `yaml:"-"`
	HeartbeatIntervalRaw      string        `yaml:"heartbeat_interval"`
	RequestTimeout            time.Duration `yaml:"-"`
	RequestTimeoutRaw         string        `yaml:"request_timeout"`
	StdioKeepaliveInterval    time.Duration `yaml:"-"`
	StdioKeepaliveIntervalRaw string        `yaml:"stdio_keepalive_interval"`
	ServerName                string        `yaml:"server_name"`
	ServerVersion             string        `yaml:"server_version"`
}

type configFile struct {
	IDEAdapter Config `yaml:"ide_adapter"`
}

var ideAdapterConfigs stampmemo.Table[Config] // keyed by projectRoot; stamp is .zqk/mcp/config.yaml

// DefaultConfig returns studio-safe defaults for the IDE adapter.
func DefaultConfig() Config {
	return Config{
		DaemonTCP:            mcp.DefaultDaemonTCP,
		AdvertiseElicitation: false,
		AutoSubscribeEventTypes: []string{
			string(mcp.EventTypeActionRequired),
		},
		ClientInfoName:            mcp.IDEProxySubscriberClientID,
		ClientInfoVersion:         defaultClientInfoVer,
		HeartbeatInterval:         5 * time.Second,
		HeartbeatIntervalRaw:      defaultHeartbeatRaw,
		RequestTimeout:            30 * time.Second,
		RequestTimeoutRaw:         defaultRequestTimeout,
		StdioKeepaliveInterval:    45 * time.Second,
		StdioKeepaliveIntervalRaw: defaultStdioKeepaliveRaw,
		ServerName:                mcp.GetBrandPrefix(),
		ServerVersion:             defaultServerVersion,
	}
}

// LoadConfig loads ide_adapter from the MCP config file (same path as the daemon).
// Missing file or section yields defaults.
func LoadConfig(projectRoot string) Config {
	path := paths.MCPConfigPath(projectRoot)
	cfg, _ := ideAdapterConfigs.Load(projectRoot, stampmemo.Of(path), func() (Config, error) {
		return readIDEAdapterConfig(path), nil
	})
	return cfg
}

func readIDEAdapterConfig(path string) Config {
	cfg := DefaultConfig()
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return cfg
	}
	var file configFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return cfg
	}
	mergeConfig(&cfg, file.IDEAdapter)
	return cfg
}

func mergeConfig(dst *Config, src Config) {
	if src.DaemonTCP != "" {
		dst.DaemonTCP = src.DaemonTCP
	}
	if src.AdvertiseElicitation {
		dst.AdvertiseElicitation = true
	}
	if len(src.AutoSubscribeEventTypes) > 0 {
		dst.AutoSubscribeEventTypes = append([]string(nil), src.AutoSubscribeEventTypes...)
	}
	if src.ClientInfoName != "" {
		dst.ClientInfoName = src.ClientInfoName
	}
	if src.ClientInfoVersion != "" {
		dst.ClientInfoVersion = src.ClientInfoVersion
	}
	if src.HeartbeatIntervalRaw != "" {
		dst.HeartbeatIntervalRaw = src.HeartbeatIntervalRaw
		if d, err := time.ParseDuration(src.HeartbeatIntervalRaw); err == nil && d > 0 {
			dst.HeartbeatInterval = d
		}
	} else if src.HeartbeatInterval > 0 {
		dst.HeartbeatInterval = src.HeartbeatInterval
	}
	if src.RequestTimeoutRaw != "" {
		dst.RequestTimeoutRaw = src.RequestTimeoutRaw
		if d, err := time.ParseDuration(src.RequestTimeoutRaw); err == nil && d > 0 {
			dst.RequestTimeout = d
		}
	} else if src.RequestTimeout > 0 {
		dst.RequestTimeout = src.RequestTimeout
	}
	if src.StdioKeepaliveIntervalRaw != "" {
		dst.StdioKeepaliveIntervalRaw = src.StdioKeepaliveIntervalRaw
		if d, err := time.ParseDuration(src.StdioKeepaliveIntervalRaw); err == nil {
			// "0" / "0s" disables host hourglass ping.
			dst.StdioKeepaliveInterval = d
		}
	} else if src.StdioKeepaliveInterval > 0 {
		dst.StdioKeepaliveInterval = src.StdioKeepaliveInterval
	}
	if src.ServerName != "" {
		dst.ServerName = src.ServerName
	}
	if src.ServerVersion != "" {
		dst.ServerVersion = src.ServerVersion
	}
}
