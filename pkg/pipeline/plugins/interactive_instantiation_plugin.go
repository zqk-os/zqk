package plugins

import (
	"context"
	"fmt"

	"github.com/zqk-os/zqk/pkg/interactive"
	"github.com/zqk-os/zqk/pkg/objects"
)

// InteractiveInstantiationPlugin pauses the pipeline to solicit interactive object creation
// via the ambient environment when missing fields are detected.
type InteractiveInstantiationPlugin struct {
	fieldRegistry *objects.FieldRegistry
}

// NewInteractiveInstantiationPlugin creates a new InteractiveInstantiationPlugin.
func NewInteractiveInstantiationPlugin(registry *objects.FieldRegistry) *InteractiveInstantiationPlugin {
	if registry == nil {
		registry = objects.GetGlobalFieldRegistry()
	}
	return &InteractiveInstantiationPlugin{
		fieldRegistry: registry,
	}
}

// Name returns the unique string identifier for the plugin.
func (p *InteractiveInstantiationPlugin) Name() string {
	return "InteractiveInstantiationPlugin"
}

// Execute enriches the pipeline payload or delegates to an interactive wizard loop
// if ambient context cannot auto-hydrate all required fields.
func (p *InteractiveInstantiationPlugin) Execute(ctx context.Context, payload map[string]any) (map[string]any, error) {
	if payload == nil {
		payload = make(map[string]any)
	}

	// For now, if interactive instantiation is triggered, we simulate
	// hydration of missing fields using the ambient environment.
	kind, _ := payload[objects.FieldKeyKind].(string)
	if kind == "" {
		return payload, nil
	}

	// This is a placeholder for actual interaction logic that would
	// pause execution and dispatch a UI event or CLI wizard.
	generator := interactive.NewTemplateGenerator(p.fieldRegistry)
	loop := interactive.NewTemplateLoop(generator)

	// Example: Try to process the loop automatically using payload as initial values
	loopState, err := loop.ProcessLoop(kind, payload)
	if err != nil {
		return nil, fmt.Errorf("interactive instantiation loop failed: %w", err)
	}

	if loopState.IsComplete {
		payload["interactive_instantiation"] = "complete"
		payload["hydrated_values"] = loopState.ProvidedValues
	} else {
		payload["interactive_instantiation"] = "pending_user_input"
		payload["missing_fields"] = loopState.MissingRequired
	}

	return payload, nil
}

// Validate ensures the output of this plugin satisfies the next stage's requirements.
func (p *InteractiveInstantiationPlugin) Validate(ctx context.Context, output map[string]any) error {
	status, _ := output["interactive_instantiation"].(string)
	if status == "pending_user_input" {
		return fmt.Errorf("cannot proceed: interactive instantiation is waiting for user input")
	}
	return nil
}
