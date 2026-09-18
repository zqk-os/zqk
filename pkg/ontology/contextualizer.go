package ontology

import (
	"context"
	"errors"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/when"
)

// Contextualizer maps external data to ZQK ontology layers.
type Contextualizer struct {
	manager OntologyManager
}

// NewContextualizer creates a new contextualizer.
func NewContextualizer(manager OntologyManager) *Contextualizer {
	return &Contextualizer{manager: manager}
}

// Contextualize maps the external data representation to the active ontology layer.
func (c *Contextualizer) Contextualize(ctx context.Context, externalData map[string]any) (map[string]any, error) {
	kind := objects.GetString(externalData, "kind")
	id := objects.GetString(externalData, "id")

	// Use when.When for conditional logic
	var result map[string]any
	var err error

	when.When(func() bool { return kind != "" && id != "" }).Then(func() {
		result = map[string]any{
			"mapped_kind":         kind,
			"mapped_id":           id,
			objects.FieldKeyLayer: "active_layer", // Placeholder for actual layer logic
		}
	}).OrElse(func() {
		err = errors.New(ConstMissingRequiredFieldsKindOrID)
	}).Run()

	return result, err
}
