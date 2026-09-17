package bldr_trait_v1

import (
	"github.com/lanceman/zqk/pkg/specbuilder/trait_builders"
)

// StatusReactiveBuilder builds the status_reactive trait at version v1_0_0.
type StatusReactiveBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewStatusReactiveBuilder creates a new builder for status_reactive trait version v1_0_0.
func NewStatusReactiveBuilder() *StatusReactiveBuilder {
	builder := &StatusReactiveBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("status_reactive", "v1_0_0"),
	}

	builder.
		SetDescription("Object-level admission for status-event listeners. A catalyst status save publishes one event to each outbound ref (listener stubs). Kinds with this trait may run the generic interpreter. Kinds without it are a no-op. The listener updates only itself; a self-update is a new catalyst. TRACK: BLI-REDACTED.\\n").
		SetCategory("behavior").
		SetObjectLevel(true).
		SetFieldLevel(false)

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewStatusReactiveBuilder())
}
