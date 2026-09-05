# Graph-Based Spec Storage v1.0

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2025-12-24  
**Status:** Design Complete  
**Related:** REQ-018, CRIT-8195, BLI-622

## Overview

Object specifications can be stored in the graph backend as nodes, enabling dynamic loading of ID patterns, validation rules, and other spec metadata. This supports the transition from file-based to graph-based storage while maintaining consistency.

## Graph Spec Node Structure

### Node Label
- Primary: `ObjectSpec`
- Alternative: `Spec` (for compatibility)

### Node Properties

```cypher
CREATE (spec:ObjectSpec {
  id: "spec_backlog_item",
  ontology: "backlog_item",
  id_template: "BLI-{sequence}",
  id_prefixes: ["BLI-"],
  fields: {
    id: {
      validation: {
        pattern: "^BLI-\\d{3,}$"
      }
    }
  }
})
```

**Required Properties:**
- `ontology`: Object kind (e.g., `"backlog_item"`, `"priority_plan"`)

**Optional Properties:**
- `id_template`: Template string (e.g., `"BLI-{sequence}"`)
- `id_prefixes`: Array of valid prefixes (e.g., `["PRI-", "PRIO-"]`)
- `fields.id.validation.pattern`: Regex pattern for ID validation

## Loading Priority

The ID validator uses the following priority order:

1. **Graph Backend** (if enabled and available)
   - Queries for `ObjectSpec` or `Spec` labeled nodes
   - Falls back to querying by `type: "object_spec"` property
   - Parses node properties to extract ID patterns

2. **File-Based Specs** (if graph unavailable)
   - Reads from `docs/process/_internal/object_specs/*.yaml`
   - Extracts `id_template` and `fields.id.validation.pattern`

3. **Default Patterns** (if both unavailable)
   - Hardcoded fallback patterns
   - Ensures system always has validation rules

## ID Pattern Extraction

The validator extracts patterns from graph nodes in this order:

1. **Direct `id_prefixes` property**: `["PRI-", "PRIO-"]`
2. **From `id_template`**: `"BLI-{sequence}"` → `["BLI-"]`
3. **From `fields.id.validation.pattern`**: Extracts prefixes from regex
4. **Inferred from `ontology`**: Uses kind-to-prefix mapping

## Example Graph Spec Nodes

### Backlog Item Spec
```cypher
CREATE (spec:ObjectSpec {
  id: "spec_backlog_item",
  ontology: "backlog_item",
  id_template: "BLI-{sequence}",
  id_prefixes: ["BLI-"]
})
```

### Priority Plan Spec (Multiple Prefixes)
```cypher
CREATE (spec:ObjectSpec {
  id: "spec_priority_plan",
  ontology: "priority_plan",
  id_prefixes: ["PRI-", "PRIO-"]
})
```

### Requirement Spec (With Pattern)
```cypher
CREATE (spec:ObjectSpec {
  id: "spec_requirement",
  ontology: "requirement",
  id_prefixes: ["REQ-", "REQU-"],
  fields: {
    id: {
      validation: {
        pattern: "^(REQ|REQU)-\\d{3,}$"
      }
    }
  }
})
```

## Migration Strategy

When migrating from file-based to graph-based specs:

1. **Phase 1**: Keep file-based specs, add graph support
2. **Phase 2**: Load specs into graph during migration
3. **Phase 3**: Prefer graph-based specs, keep files as backup
4. **Phase 4**: Graph-only (files become read-only archive)

## Benefits

1. **Dynamic Updates**: Specs can be updated in graph without code changes
2. **Consistency**: Same validator works for both file and graph backends
3. **Performance**: Graph queries are fast, patterns cached in memory
4. **Flexibility**: Easy to add new object types by creating spec nodes
5. **Backward Compatible**: Falls back to files if graph unavailable

## Implementation

The `pkg/validation` package provides:
- `NewIDValidatorWithGraph()`: Creates validator with graph support
- `SetGraphConnection()`: Adds graph connection to existing validator
- `LoadPatterns()`: Automatically tries graph → files → defaults

## Related

- [Hash Registry Design](../architecture/hash-registry-design-v1.0.md) - Consistent pattern for registry storage
- [GraphRAG Schema Design](../architecture/graphrag-schema-design-v1.0.md) - Graph schema architecture
- [System Ontology](../ontology/system-ontology-v1.0.md) - Object type definitions

