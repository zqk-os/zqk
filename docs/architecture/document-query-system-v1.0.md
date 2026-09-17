# Document Query System v1.0

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-12-25  
**Status**: Design  
**Related**: doc_entry spec, Guided Spec Management, Graph Backend

## Overview

The document query system enables users to discover and search documentation by category, group, and full-text content. This system leverages the `doc_entry` object specification and provides semantic linking for discoverability.

## Query Capabilities

### 1. Group-Based Filtering

Filter documents by their primary group:

```bash
# Find all architecture documents
zqk docman list --group architecture

# Find all process documents
zqk docman list --group process
```

**Available Groups**:
- `project_goals` - Project goal documentation
- `project_specific` - Project-specific documentation
- `tooling` - Tooling and development documentation
- `process` - Process and workflow documentation
- `onboarding` - Onboarding materials
- `design` - Design documents
- `architecture` - Architecture documentation (NEW)
- `other` - Uncategorized documents

### 2. Category-Based Filtering

Filter documents by fine-grained category:

```bash
# Find all validation-related architecture docs
zqk docman list --group architecture --category validation

# Find all graph-backend related docs
zqk docman list --group architecture --category graph-backend
```

**Category Examples**:
- `validation` - Validation and testing documentation
- `graph-backend` - Graph backend implementation
- `semantic-types` - Semantic type definitions
- `migration` - Migration strategies
- `storage` - Storage and persistence
- `cli` - CLI interface documentation

### 3. Full-Text Content Search

Search document content for specific terms:

```bash
# Find documents containing "semantic"
zqk docman search "semantic" --group architecture

# Find documents containing "validation" in any group
zqk docman search "validation"

# Case-insensitive search with multiple terms
zqk docman search "semantic types validation" --group architecture
```

### 4. Combined Queries

Combine filters and search:

```bash
# Find architecture documents with category "validation" containing "semantic"
zqk docman search "semantic" \
  --group architecture \
  --category validation

# Find all documents in architecture group that mention "graph" or "backend"
zqk docman search "graph backend" --group architecture
```

## Implementation Architecture

### Document Indexing

Documents are indexed for full-text search when `content_searchable: true`:

```go
type DocumentIndexer interface {
    IndexDocument(docEntry *DocEntry, content string) error
    Search(query string, filters SearchFilters) ([]DocEntry, error)
    UpdateIndex(docEntry *DocEntry) error
    RemoveFromIndex(docEntryID string) error
}

type SearchFilters struct {
    Group    string   // Filter by group
    Category string   // Filter by category
    GoalRefs []string // Filter by goal references
    WorkstreamRefs []string // Filter by workstream references
}
```

### Graph Backend Integration

For graph backends, documents can be indexed as nodes with full-text search:

```cypher
// Create document node with indexed content
CREATE (d:Document {
    id: "doc-architecture-guided-spec-management",
    path: "docs/architecture/guided-spec-management-v1.0.md",
    group: "architecture",
    category: "spec-management",
    summary: "Interactive, guided workflows for spec creation/update",
    content_searchable: true
})

// Full-text search query
MATCH (d:Document)
WHERE d.group = "architecture"
  AND d.category = "validation"
  AND d.content CONTAINS "semantic"
RETURN d.id, d.path, d.summary
```

### File-Based Search

For file-based backends, use file system search:

```go
func SearchDocuments(query string, filters SearchFilters) ([]DocEntry, error) {
    // Load all doc_entry objects
    entries := loadAllDocEntries()
    
    var results []DocEntry
    
    for _, entry := range entries {
        // Apply filters
        if filters.Group != "" && entry.Group != filters.Group {
            continue
        }
        if filters.Category != "" && entry.Category != filters.Category {
            continue
        }
        
        // Search content if searchable
        if entry.ContentSearchable {
            content, err := readFileContent(entry.Path)
            if err != nil {
                continue
            }
            
            // Simple text search (can be enhanced with fuzzy matching)
            if strings.Contains(strings.ToLower(content), strings.ToLower(query)) {
                results = append(results, entry)
            }
        } else {
            // Search metadata only (path, summary)
            if strings.Contains(strings.ToLower(entry.Path+entry.Summary), strings.ToLower(query)) {
                results = append(results, entry)
            }
        }
    }
    
    return results, nil
}
```

## Query Examples

### Example 1: Find Architecture Documents About Semantic Types

```bash
$ zqk docman search "semantic" --group architecture

Results:
  - docs/architecture/guided-spec-management-v1.0.md
    Group: architecture
    Category: spec-management
    Summary: Interactive, guided workflows for spec creation/update with semantic linking
    
  - .zqk/specs/documentation/criteria-categories-v1.0.md
    Group: architecture
    Category: criteria
    Summary: Defines standard categories for criteria objects
```

### Example 2: Find Validation-Related Architecture Docs

```bash
$ zqk docman list --group architecture --category validation

Results:
  - docs/architecture/instance-validation-v1.0.md
  - docs/architecture/pluggable-validators-v1.0.md
  - docs/architecture/shacl-validation-evaluation-v1.0.md
```

### Example 3: Find Documents Linked to Specific Goal

```bash
$ zqk docman list --goal-ref GOAL-9796

Results:
  - docs/architecture/graphrag-schema-design-v1.0.md
    Group: architecture
    Category: graph-backend
    Summary: GraphRAG schema architecture for the zqk Knowledge Kernel
```

## Document Registration

Documents should be registered as `doc_entry` objects:

```yaml
# Example: .zqk/process/documents/DOC-001.yaml
id: DOC-001
kind: doc_entry
schema_version: "2.0.0"
path: "docs/architecture/guided-spec-management-v1.0.md"
summary: "Interactive, guided workflows for spec creation/update with semantic linking"
group: "architecture"
category: "spec-management"
content_searchable: true
goal_refs: []
workstream_refs: []
milestone_refs: []
requirement_refs: []
```

## CLI Commands

### List Documents

```bash
zqk docman list
  --group <group>          # Filter by group
  --category <category>    # Filter by category
  --goal-ref <goal-id>     # Filter by goal reference
  --workstream-ref <ws-id> # Filter by workstream reference
  --format <format>        # Output format (table, json, yaml)
```

### Search Documents

```bash
zqk docman search <query>
  --group <group>          # Filter by group
  --category <category>    # Filter by category
  --case-sensitive         # Case-sensitive search
  --fuzzy                  # Fuzzy matching
  --limit <n>              # Limit results
  --format <format>        # Output format
```

### Register Document

```bash
zqk docman register <path>
  --group <group>          # Document group
  --category <category>    # Document category
  --summary <summary>      # Document summary
  --goal-ref <goal-id>     # Link to goal
  --workstream-ref <ws-id> # Link to workstream
  --content-searchable     # Enable content indexing (default: true)
  --non-interactive        # Non-interactive mode
```

## Benefits

1. **Discoverability**: Easy to find relevant documentation
2. **Traceability**: Link documents to goals, workstreams, requirements
3. **Organization**: Clear categorization and grouping
4. **Search**: Full-text search across document content
5. **Semantic Linking**: Documents linked to system objects
6. **Consistency**: Unified interface for all documentation

## Related Documentation

- [doc_entry Object Specification](../_internal/object_specs/doc_entry.yaml)
- [Guided Spec Management](./guided-spec-management-v1.0.md) - Similar semantic linking approach
- [Graph Backend Architecture](./pluggable-graph-backend-interface-v1.0.md) - Graph storage for documents

