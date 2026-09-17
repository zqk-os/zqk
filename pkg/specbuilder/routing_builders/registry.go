package routing_builders

// This file provides a global registry and auto-registration mechanism
// Builders register themselves from version packages via init() functions in those packages

var globalRegistry *VersionedRoutingRuleBuilderRegistry

func init() {
	globalRegistry = NewVersionedRoutingRuleBuilderRegistry()
	// Generated builders in bldr_routing_v1 register via init() when that package is imported.
	// The package may be empty until generate-routing-builders runs against a rules YAML file.
}

// RegisterBuilder registers a builder (called from version package init() functions)
func RegisterBuilder(builder RoutingRuleBuilder) {
	globalRegistry.Register(builder)
}

// GetGlobalRegistry returns the global builder registry
func GetGlobalRegistry() *VersionedRoutingRuleBuilderRegistry {
	return globalRegistry
}
