package profile_builders

// This file provides a global registry and auto-registration mechanism
// Builders register themselves from version packages via init() functions in those packages

var globalRegistry *VersionedProfileBuilderRegistry

func init() {
	globalRegistry = NewVersionedProfileBuilderRegistry()
	// Builders from bldr_profile_v1 package register themselves via their init() functions
	// Note: bldr_profile_v1 package must be imported (e.g., in tests or main) to trigger init()
}

// RegisterBuilder registers a builder (called from version package init() functions)
func RegisterBuilder(builder ProfileBuilder) {
	globalRegistry.Register(builder)
}

// GetGlobalRegistry returns the global builder registry
func GetGlobalRegistry() *VersionedProfileBuilderRegistry {
	return globalRegistry
}
