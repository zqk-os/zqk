package plugins

import (
	"context"
	"fmt"
	"strings"
)

// GraphExclusiveStatePlugin enforces that agent state and task tracking
// is recorded exclusively in the ZQK graph or .zqk-state/system-state.csnap,
// rejecting paths associated with external brain reliance like .gemini or .cursor.
type GraphExclusiveStatePlugin struct {
	restrictedPaths []string
}

// NewGraphExclusiveStatePlugin creates a new GraphExclusiveStatePlugin.
func NewGraphExclusiveStatePlugin() *GraphExclusiveStatePlugin {
	return &GraphExclusiveStatePlugin{
		restrictedPaths: []string{".gemini", ".cursor", ".claude"},
	}
}

// Name returns the unique string identifier for the plugin.
func (p *GraphExclusiveStatePlugin) Name() string {
	return "GraphExclusiveStatePlugin"
}

// Execute scans the payload for restricted paths.
func (p *GraphExclusiveStatePlugin) Execute(ctx context.Context, payload map[string]any) (map[string]any, error) {
	if payload == nil {
		payload = make(map[string]any)
	}

	if err := p.checkPayload(payload, ""); err != nil {
		return nil, fmt.Errorf("state tracking violation: %w", err)
	}

	payload["graph_exclusive_state_enforced"] = true
	return payload, nil
}

func (p *GraphExclusiveStatePlugin) checkPayload(payload map[string]any, prefix string) error {
	for k, v := range payload {
		fullKey := k
		if prefix != "" {
			fullKey = prefix + "." + k
		}

		switch val := v.(type) {
		case string:
			for _, rp := range p.restrictedPaths {
				if strings.Contains(val, rp) {
					return fmt.Errorf("field %q contains restricted external path %q", fullKey, rp)
				}
			}
		case map[string]any:
			if err := p.checkPayload(val, fullKey); err != nil {
				return err
			}
		}
	}
	return nil
}

// Validate ensures the output of this plugin satisfies the next stage's requirements.
func (p *GraphExclusiveStatePlugin) Validate(ctx context.Context, output map[string]any) error {
	if _, ok := output["graph_exclusive_state_enforced"]; !ok {
		return fmt.Errorf("graph exclusive state enforcement failed: marker missing")
	}
	return nil
}
