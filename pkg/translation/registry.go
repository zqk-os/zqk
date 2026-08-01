package translation

import (
	"fmt"
	"sync"

	"github.com/lanceman/zqk/pkg/concurrency"
)

// Registry selects a translator by source format (CRIT-7641).
var (
	registry   = &RegistryImpl{translators: make(map[string]Translator)}
	registryMu sync.RWMutex
)

const emptyValue = ""

// RegistryImpl holds format -> Translator mappings.
type RegistryImpl struct {
	translators map[string]Translator
}

// Register adds a translator for its SourceFormat() key.
// Multiple formats (e.g. "turtle", "rdf_owl") can point to the same translator.
func Register(t Translator) {
	_ = concurrency.RunInLock(&registryMu, func() error {
		key := t.SourceFormat()
		if key == emptyValue {
			return nil
		}
		if registry.translators == nil {
			registry.translators = make(map[string]Translator)
		}
		registry.translators[key] = t
		return nil
	})
}

// Get returns the translator for the given format key, or nil if none.
func Get(formatKey string) Translator {
	var t Translator
	_ = concurrency.RunInRLock(&registryMu, func() error {
		t = registry.translators[formatKey]
		return nil
	})
	return t
}

// TranslateByFormat looks up the translator for formatKey and runs Translate.
// Returns ErrNoTranslator if no translator is registered for formatKey.
func TranslateByFormat(formatKey string, raw []byte, opts *TranslateOptions) (*TranslateResult, error) {
	t := Get(formatKey)
	if t == nil {
		return nil, ErrNoTranslator{Format: formatKey}
	}
	return t.Translate(raw, opts)
}

// ErrNoTranslator is returned when no translator is registered for a format.
type ErrNoTranslator struct {
	Format string
}

func (e ErrNoTranslator) Error() string {
	return fmt.Sprintf("no translator registered for format %q", e.Format)
}

func init() {
	// Register default translators (CRIT-7642: at least one format translator).
	rdf := NewRDFOWLTranslator()
	Register(rdf)
	// Same translator for turtle and rdf_xml/jsonld so import pipeline can use formatKey as-is.
	Register(&rdfOwlAlias{rdfOwlTranslator: rdf.(*rdfOwlTranslator), formatKey: FormatTurtle})
	Register(&rdfOwlAlias{rdfOwlTranslator: rdf.(*rdfOwlTranslator), formatKey: FormatRDFXML})
	Register(&rdfOwlAlias{rdfOwlTranslator: rdf.(*rdfOwlTranslator), formatKey: FormatJSONLD})
	// ITEM-761, ITEM-762, ITEM-763: Cypher, JSON Schema, OpenAPI import.
	Register(NewCypherTranslator())
	Register(NewJSONSchemaTranslator())
	Register(NewOpenAPITranslator())
}

// rdfOwlAlias wraps RDF/OWL translator to register under alternate format keys.
type rdfOwlAlias struct {
	*rdfOwlTranslator
	formatKey string
}

func (a *rdfOwlAlias) SourceFormat() string { return a.formatKey }
