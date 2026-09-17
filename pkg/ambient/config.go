package ambient

import "github.com/lanceman/zqk/pkg/zqkenv"

// Config defines the configuration for the Ambience Engine.
type Config struct {
	Enabled bool
}

// LoadConfig reads the environment variables to determine if the
// Ambience Engine should be activated.
func LoadConfig() Config {
	return Config{
		Enabled: zqkenv.EnableAmbientWatcher().Get() == "1",
	}
}

// ResolveService returns the appropriate Service implementation based on the config.
func ResolveService(cfg Config) Service {
	if !cfg.Enabled {
		return NewNoopService()
	}
	// TODO: Return the actual fsnotify watcher service when implemented.
	return NewNoopService() // Fallback until implemented
}
