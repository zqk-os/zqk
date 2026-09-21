package config

import "sync"

// resetForTesting resets the config singleton so tests can get a fresh load.
func resetForTesting() {
	globalConfig = nil
	configOnce = sync.Once{}
	rootConfigs.Reset()
}
