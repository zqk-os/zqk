package engine

import (
	"context"
	"fmt"

	"github.com/lanceman/zqk/pkg/bridge"
)

// TranslationEngine orchestrates schema-to-ontology translation.
type TranslationEngine struct {
	translators map[string]bridge.SemanticTranslator
}

// NewTranslationEngine creates a new engine.
func NewTranslationEngine() *TranslationEngine {
	return &TranslationEngine{
		translators: make(map[string]bridge.SemanticTranslator),
	}
}

// RegisterTranslator adds a new schema translator to the engine.
func (e *TranslationEngine) RegisterTranslator(format string, t bridge.SemanticTranslator) {
	e.translators[format] = t
}

// Translate performs the end-to-end translation for a given format and input.
func (e *TranslationEngine) Translate(ctx context.Context, format string, input []byte) (map[string]any, error) {
	translator, ok := e.translators[format]
	if !ok {
		return nil, fmt.Errorf(ConstNoTranslatorFoundForFormatS, format)
	}

	// Translation logic invoked here using the selected translator
	return translator.Translate(ctx, input)
}
