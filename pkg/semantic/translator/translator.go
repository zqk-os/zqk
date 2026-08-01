package translator

import (
	"errors"

	"github.com/lanceman/zqk/pkg/ontology"
)

var (
	ErrUnsupportedFormat = errors.New("unsupported external format")
	ErrValidationFailed  = errors.New("translation failed internal validation")
)

// Translator defines the contract for converting a specific external format.
type Translator interface {
	TranslateToInternal(className string, externalData []byte) (map[string]any, error)
	TranslateToExternal(className string, internalData map[string]any) ([]byte, error)
	SupportedFormat() string
}

// Engine manages translators and orchestrates the translation and validation pipeline.
type Engine interface {
	RegisterTranslator(t Translator) error
	GetTranslator(format string) (Translator, error)
	TranslateAndValidate(format string, className string, externalData []byte) (map[string]any, error)
}

type defaultEngine struct {
	translators map[string]Translator
	ontologyMgr ontology.OntologyManager
}

// NewEngine creates a new Semantic Translation Engine.
func NewEngine(mgr ontology.OntologyManager) Engine {
	return &defaultEngine{
		translators: make(map[string]Translator),
		ontologyMgr: mgr,
	}
}

func (e *defaultEngine) RegisterTranslator(t Translator) error {
	e.translators[t.SupportedFormat()] = t
	return nil
}

func (e *defaultEngine) GetTranslator(format string) (Translator, error) {
	t, ok := e.translators[format]
	if !ok {
		return nil, ErrUnsupportedFormat
	}
	return t, nil
}

func (e *defaultEngine) TranslateAndValidate(format string, className string, externalData []byte) (map[string]any, error) {
	t, err := e.GetTranslator(format)
	if err != nil {
		return nil, err
	}

	internalData, err := t.TranslateToInternal(className, externalData)
	if err != nil {
		return nil, err
	}

	// Validate against the ontology layer
	if err := e.ontologyMgr.ValidateInstance(className, internalData); err != nil {
		return nil, ErrValidationFailed
	}

	return internalData, nil
}
