# Import Tracking Architecture

**Last Verified:** 2026-08-31


**Status**: Active

## Architecture
The `import_tracking` object tracks ontology and schema imports (RDF/OWL, JSON Schema, OpenAPI, Cypher) from external formats into the Zen Quantum Kernel.

### Core Concepts
- **Tracking ID**: Unique ID for the import batch.
- **Source Format**: The original format of the data (e.g., `rdf_owl`, `json_schema`).
- **Domain Registry**: Reference to the generated `domain_registry` object.
- **Status**: The status of the import process (e.g., `pending`, `completed`, `failed`).
