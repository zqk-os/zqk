package config

// resetForTesting resets the config singleton so tests can get a fresh load.
func resetForTesting() {
	globalConfig = nil
	rootConfigs.Reset()
}
