package plugins

import (
	"context"
	"fmt"
)

// ValidationPlugin ensures all stages produce compliant outputs
// by verifying that required fields are present in the payload.
type ValidationPlugin struct {
	requiredFields []string
}

// NewValidationPlugin creates a new ValidationPlugin.
func NewValidationPlugin(requiredFields []string) *ValidationPlugin {
	return &ValidationPlugin{
		requiredFields: requiredFields,
	}
}

// Name returns the unique string identifier for the plugin.
func (p *ValidationPlugin) Name() string {
	return "ValidationPlugin"
}

// Execute enriches the pipeline payload with validation status based on compliance.
func (p *ValidationPlugin) Execute(ctx context.Context, payload map[string]any) (map[string]any, error) {
	if payload == nil {
		payload = make(map[string]any)
	}

	var missing []string
	for _, field := range p.requiredFields {
		if _, ok := payload[field]; !ok {
			missing = append(missing, field)
		}
	}

	if len(missing) > 0 {
		payload["validation_status"] = "failed"
		payload["validation_errors"] = fmt.Sprintf("missing required fields: %v", missing)
	} else {
		payload["validation_status"] = "passed"
	}

	return payload, nil
}

// Validate ensures the output of this plugin satisfies the next stage's requirements.
func (p *ValidationPlugin) Validate(ctx context.Context, output map[string]any) error {
	status, ok := output["validation_status"].(string)
	if !ok || status != "passed" {
		errStr, _ := output["validation_errors"].(string)
		if errStr == "" {
			errStr = "unknown validation error"
		}
		return fmt.Errorf("validation failed: %s", errStr)
	}
	return nil
}
