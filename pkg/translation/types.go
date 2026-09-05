package translation

// Translator converts source-format ontology/schema content into zqk-domain structures.
// Implementations exist per format (RDF/OWL, JSON Schema, etc.) — see registry.
type Translator interface {
	// SourceFormat returns the format key this translator handles (e.g. "rdf_owl", "turtle", "jsonld").
	SourceFormat() string
	// Translate parses raw input and produces zqk-domain objects (e.g. domain_registry, object_spec-like).
	// Callers may persist results via storage or use for validation/traceability.
	Translate(raw []byte, opts *TranslateOptions) (*TranslateResult, error)
}

// TranslateOptions optional parameters for Translate.
type TranslateOptions struct {
	SourceFile string // Optional: path or identifier for diagnostics.
}

// TranslateResult holds the output of a translation run.
type TranslateResult struct {
	Objects  []TranslatedObject // Domain objects derived from the source.
	Specs    []map[string]any   // Object-spec-like structures (schema_version, ontology, extends, description, fields) for downstream spec generation.
	Warnings []string           // Non-fatal issues (e.g. unsupported triples).
}

// TranslatedObject is a single zqk-domain object produced by a translator.
type TranslatedObject struct {
	Kind string         // Object kind (e.g. "domain_registry").
	Data map[string]any // Field set; must include id and kind (see pkg/objects field key constants) for persistence.
}
