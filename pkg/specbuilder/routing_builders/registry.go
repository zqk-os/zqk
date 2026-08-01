package routing_builders

// This file provides a global registry and auto-registration mechanism
// Builders register themselves from version packages via init() functions in those packages

var globalRegistry *VersionedRoutingRuleBuilderRegistry

func init() {
	globalRegistry = NewVersionedRoutingRuleBuilderRegistry()
	// Builders from bldr_routing_v1 package register themselves via their init() functions
	// Note: bldr_routing_v1 package must be imported (e.g., in tests or main) to trigger init()
}

// RegisterBuilder registers a builder (called from version package init() functions)
func RegisterBuilder(builder RoutingRuleBuilder) {
	globalRegistry.Register(builder)
}

// GetGlobalRegistry returns the global builder registry
func GetGlobalRegistry() *VersionedRoutingRuleBuilderRegistry {
	return globalRegistry
}
