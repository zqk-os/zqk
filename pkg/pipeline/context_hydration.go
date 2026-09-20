package pipeline

import (
	"context"
)

// ContextHydrationPlugin implements PipelinePlugin for context hydration.
type ContextHydrationPlugin struct{}

func (p *ContextHydrationPlugin) Name() string {
	return "ContextHydration"
}

func (p *ContextHydrationPlugin) Execute(ctx context.Context, payload map[string]any) (map[string]any, error) {
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["hydrated"] = true
	return payload, nil
}

func (p *ContextHydrationPlugin) Validate(ctx context.Context, output map[string]any) error {
	return nil
}
