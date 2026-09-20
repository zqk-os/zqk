package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

// TranslationEngine converts external artifacts into ZQK native objects.
type TranslationEngine interface {
	Translate(ctx context.Context, externalData []byte, format, sourceName string) (map[string]any, error)
}

// AgentTranslator implements TranslationEngine for 3rd-party agents.
type AgentTranslator struct{}

// NewAgentTranslator creates a new AgentTranslator.
func NewAgentTranslator() *AgentTranslator {
	return &AgentTranslator{}
}

// Translate converts external JSON/YAML (like Gemini skills or IDE rules) into native ZQK objects.
func (t *AgentTranslator) Translate(ctx context.Context, externalData []byte, format, sourceName string) (map[string]any, error) {
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

	// Determine kind based on structure heuristics
	if _, ok := parsed[objects.FieldKeyKind]; !ok {
		if _, hasRules := parsed["rules"]; hasRules {
			parsed[objects.FieldKeyKind] = "policy"
		} else if _, hasInst := parsed[objects.FieldKeyInstructions]; hasInst {
			parsed[objects.FieldKeyKind] = "tool_spec"
		} else if _, hasSkills := parsed[objects.FieldKeySkills]; hasSkills {
			parsed[objects.FieldKeyKind] = "tool_spec"
		} else {
			parsed[objects.FieldKeyKind] = "tool_spec" // fallback
		}
	}

	// Generate stable ID based on source name
	if _, ok := parsed[objects.FieldKeyID]; !ok {
		name := filepath.Base(sourceName)
		name = strings.TrimSuffix(name, filepath.Ext(name))
		parsed[objects.FieldKeyID] = "ext-" + name
	}

	return parsed, nil
}
