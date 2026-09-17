# Translation Manifest Architecture

**Last Verified:** 2026-08-31


**Status**: Active
**Requirement**: [REDACTED-ID]

## Architecture
To avoid hardcoding translation logic for every external schema (OpenAPI, CRD, JSON Schema), ZQK utilizes a declarative `TranslationManifest`. This manifest dictates how fields in an external representation map to properties in a ZQK `ontology.Class`.

### Core Concepts
- **Manifest**: A set of declarative rules that describe the source format and destination class.
- **Field Mapping**: A rule linking an external JSON path to an internal ontology property name.
- **Transformer**: Optional transformation rules (e.g., string-to-int, date parsing).

### Interface & Structs
```go
package translator

// TranslationManifest defines how to map external data to an internal class.
type TranslationManifest struct {
	SourceFormat string            `json:"source_format"`
	TargetClass  string            `json:"target_class"`
	Mappings     map[string]string `json:"mappings"` // map[externalPath]internalProperty
}

// ManifestDrivenTranslator implements Translator using a TranslationManifest.
type ManifestDrivenTranslator struct {
	manifest TranslationManifest
}

func (m *ManifestDrivenTranslator) TranslateToInternal(className string, externalData []byte) (map[string]any, error) {
	// ... logic to parse externalData as JSON and apply m.manifest.Mappings
}
```

### Future Considerations
- Support for complex JSONPath mappings.
- Support for nested object resolution within the manifest.
