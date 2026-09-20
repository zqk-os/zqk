package translation

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

// Engine converts external artifacts into ZQK native objects.
type Engine interface {
	Translate(ctx context.Context, externalData []byte, format, sourceName string) (map[string]any, error)
}

// DefaultEngine is the default implementation.
type DefaultEngine struct{}

func NewDefaultEngine() *DefaultEngine {
	return &DefaultEngine{}
}

func (e *DefaultEngine) Translate(ctx context.Context, externalData []byte, format, sourceName string) (map[string]any, error) {
	var parsed map[string]any

	format = strings.ToLower(format)
	if format == "json" {
		if err := json.Unmarshal(externalData, &parsed); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
	} else if format == "yaml" || format == "yml" {
		if err := yaml.Unmarshal(externalData, &parsed); err != nil {
			return nil, fmt.Errorf("failed to parse YAML: %w", err)
		}
	} else {
		return nil, fmt.Errorf("unsupported format: %s", format)
	}

	// Add default object fields to make it a valid ZQK object if not present
	if _, ok := parsed[objects.FieldKeyKind]; !ok {
		// Simple heuristic: if it has "instructions", maybe it's a skill
		if _, hasInst := parsed[objects.FieldKeyInstructions]; hasInst {
			parsed[objects.FieldKeyKind] = "skill"
		} else {
			parsed[objects.FieldKeyKind] = "tool" // default to tool for 3rd party agents
		}
	}

	// Set a generic ID or name if missing
	if _, ok := parsed[objects.FieldKeyID]; !ok {
		name := filepath.Base(sourceName)
		name = strings.TrimSuffix(name, filepath.Ext(name))
		parsed[objects.FieldKeyID] = "ext-" + name
	}

	return parsed, nil
}
