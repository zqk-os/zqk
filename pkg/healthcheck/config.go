package healthcheck

// MonitorConfig is the persisted config for one monitor (e.g. enabled/disabled).
type MonitorConfig struct {
	Enabled bool `json:"enabled"`
}

// Config is the root structure for the health monitors config file.
type Config struct {
	Monitors map[string]MonitorConfig `json:"monitors"`
}
