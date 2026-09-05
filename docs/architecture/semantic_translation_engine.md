# Semantic Translation Engine

**Last Verified:** 2026-08-31


**Status**: Active
**Requirement**: [REDACTED-ID]

## Architecture
The `pkg/semantic/translator` package provides the bi-directional mapping layer between external data schemas (e.g., OpenAPI, JSON Schema, Protobuf) and the internal ZQK `OntologyManager`.

### Core Concepts
- **External Data**: Raw byte arrays or maps representing external data formats.
- **Internal Instance**: A validated `map[string]any` that conforms to a specific ZQK `ontology.Class`.

### Interfaces
```go
package translator

import "github.com/lanceman/zqk/pkg/ontology"

type Translator interface {
	TranslateToInternal(className string, externalData []byte) (map[string]any, error)
	TranslateToExternal(className string, internalData map[string]any) ([]byte, error)
	SupportedFormat() string
}

type Engine interface {
	RegisterTranslator(t Translator) error
	GetTranslator(format string) (Translator, error)
}
```

### Dependency Injection
The Translation Engine must depend on the `ontology.OntologyManager` interface introduced in Cycle 3 to dynamically validate schemas before completing a `TranslateToInternal` operation.
