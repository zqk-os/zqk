package builders

// This file provides a global registry and auto-registration mechanism
// Builders register themselves from version packages via init() functions in those packages

var globalRegistry *VersionedBuilderRegistry

func init() {
	globalRegistry = NewVersionedBuilderRegistry()
	// Builders from version packages (bldr_v2, etc.) register themselves via their init() functions
	// Note: version packages must be imported (e.g., in tests or main) to trigger init()
}

// RegisterBuilder registers a builder (called from version package init() functions)
func RegisterBuilder(builder SpecBuilder) {
	globalRegistry.Register(builder)
}

// GetGlobalRegistry returns the global builder registry
func GetGlobalRegistry() *VersionedBuilderRegistry {
	return globalRegistry
}
