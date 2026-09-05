# Semantic Bridge System Architecture

**Last Verified:** 2026-08-31


## Vision
The Semantic Bridge System (SBS) provides a bi-directional translation engine mapping external schema formats (OpenAPI, JSON Schema, Protobuf) into ZQK's internal ontology. This ensures seamless integration of external data structures into our graph-based kernel.

## Architecture

### 1. Core Interfaces
The system centers on the `SemanticTranslator` interface, ensuring modularity for different schema providers.

```go
package bridge

import (
    "context"
    "github.com/zqk/pkg/ontology" // Conceptual
)

// SemanticTranslator defines the contract for translating external schemas.
type SemanticTranslator interface {
    // Translate converts a raw external schema into an internal ontology object.
    Translate(ctx context.Context, input []byte) (ontology.Object, error)
    
    // GetSupportedFormats returns the list of formats the translator handles.
    GetSupportedFormats() []string
}
```

### 2. TranslationManifest (ZQK Object)
To maintain traceability and configuration, we introduce the `TranslationManifest` object kind.

*   **Kind**: `TranslationManifest`
*   **Fields**:
    *   `SourceURL`: URI of the external schema.
    *   `TranslatorID`: Identifier for the registered `SemanticTranslator`.
    *   `TargetKind`: The ZQK object kind produced by this translation.
    *   `MappingRules`: Configuration for mapping fields to ontology properties.
    *   `LastSync`: Timestamp of the last translation execution.

### 3. Workflow & Integration

#### Registration
1.  New translators are registered via `zqk bridge register --name <id> --type <kind>`.
2.  A `TranslationManifest` is created to link the external schema to a ZQK `Requirement` or `Capability`.

#### Translation Flow
1.  **Trigger**: An event or CLI command (`zqk bridge translate <manifest-id>`) invokes the translation service.
2.  **Execution**: The service fetches the schema, identifies the appropriate `SemanticTranslator`, and executes `Translate()`.
3.  **Ontology Mapping**: The output is validated against internal ontologies.
4.  **Journaling**: Every translation event is logged as a `change_journal_entry` with a reference to the `TranslationManifest` and the newly created/updated object ID.
5.  **Asynchronous Handling**: The translation process runs as a background task, emitting status events to the system bus.

## Error Handling & Logging
*   Use `github.com/zqk/pkg/errfmt` for structured, context-aware error wrapping.
*   Log translation results via ZQK's standard structured logging, ensuring all logs include the `manifest_id` for observability.
