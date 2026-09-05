package bridge

import (
	"context"
)

// SemanticTranslator defines the contract for translating external schemas.
type SemanticTranslator interface {
	// Translate converts a raw external schema into an internal ontology object.
	Translate(ctx context.Context, input []byte) (map[string]any, error)

	// GetSupportedFormats returns the list of formats the translator handles.
	GetSupportedFormats() []string
}
