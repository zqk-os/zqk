package api_builders

// This file provides a global registry and auto-registration mechanism
// Builders register themselves from version packages via init() functions in those packages

var globalRegistry *VersionedAPISpecGenBuilderRegistry

func init() {
	globalRegistry = NewVersionedAPISpecGenBuilderRegistry()
	// Builders from bldr_api_v1 package register themselves via their init() functions
	// Note: bldr_api_v1 package must be imported (e.g., in tests or main) to trigger init()
}

// RegisterBuilder registers a builder (called from version package init() functions)
func RegisterBuilder(builder APISpecGenBuilder) {
	globalRegistry.Register(builder)
}

// GetGlobalRegistry returns the global builder registry
func GetGlobalRegistry() *VersionedAPISpecGenBuilderRegistry {
	return globalRegistry
}
