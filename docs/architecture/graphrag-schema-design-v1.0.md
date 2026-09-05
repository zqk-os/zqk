# GraphRAG Schema Design v1.0 - Three-Layer Architecture

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2025-12-24  
**Status:** Design Complete  
**Related:** BLI-238, BLI-239, BLI-240, MIL-035, System Ontology v1.0

## Overview

This document defines the GraphRAG schema architecture for the zqk Knowledge Kernel, implementing a three-layer architecture that enables graph-based knowledge retrieval and reasoning. The schema supports all project object types defined in the System Ontology v1.0 and provides the foundation for multi-hop queries, semantic search, and episodic memory.

## Architecture Principles

1. **Three-Layer Separation**: Document, Entity, and Relationship layers provide distinct abstraction levels
2. **Ontology-Driven**: Schema directly maps to System Ontology v1.0 object types and relationships
3. **Multi-Hop Reasoning**: Supports complex graph traversals across object relationships
4. **Semantic Overlay**: Vector embeddings enable semantic search alongside structural queries
5. **Backend Agnostic**: Schema design supports MemGraph, Neo4j, and RDF implementations

## Three-Layer Architecture

### Layer 1: Document Layer

The Document Layer stores the original source documents and their chunked representations. This layer provides the raw content that will be processed into entities and relationships.

#### Node Type: `Document`

```cypher
CREATE CONSTRAINT IF NOT EXISTS FOR (d:Document) REQUIRE d.id IS UNIQUE;

// Document node structure
CREATE (d:Document {
  id: "doc_backlog_item_BLI-237",
  type: "backlog_item",
  source_path: "docs/process/backlog/BLI-237.yaml",
  content: "<full YAML content>",
  chunk_id: "chunk_001",
  chunk_index: 0,
  chunk_text: "<extracted text from YAML>",
  metadata: {
    created_at: "2025-12-24T05:45:00Z",
    updated_at: "2025-12-24T05:47:33Z",
    schema_version: "2.0.0"
  }
})
```

**Properties:**
- `id`: Unique document identifier (format: `doc_{object_type}_{object_id}`)
- `type`: Object type (e.g., `backlog_item`, `milestone`, `goal`)
- `source_path`: File path in source repository
- `content`: Full original content (YAML/JSON)
- `chunk_id`: Identifier for this chunk (if document is chunked)
- `chunk_index`: Position of chunk in document
- `chunk_text`: Extracted text content for embedding
- `metadata`: Additional metadata (timestamps, schema version, etc.)

**Purpose:**
- Preserves original source material
- Enables document-level retrieval
- Provides context for entity extraction
- Supports provenance and audit trails

### Layer 2: Entity Layer

The Entity Layer extracts and represents all object instances from the System Ontology as graph nodes. Each object type becomes a node type, and each object instance becomes a node.

#### Node Types (from System Ontology)

```cypher
// Primary Planning Objects
CREATE CONSTRAINT IF NOT EXISTS FOR (g:Goal) REQUIRE g.id IS UNIQUE;
CREATE CONSTRAINT IF NOT EXISTS FOR (m:Milestone) REQUIRE m.id IS UNIQUE;
CREATE CONSTRAINT IF NOT EXISTS FOR (w:Workstream) REQUIRE w.id IS UNIQUE;
CREATE CONSTRAINT IF NOT EXISTS FOR (p:PriorityPlan) REQUIRE p.id IS UNIQUE;
CREATE CONSTRAINT IF NOT EXISTS FOR (b:BacklogItem) REQUIRE b.id IS UNIQUE;

// Specification Objects
CREATE CONSTRAINT IF NOT EXISTS FOR (r:Requirement) REQUIRE r.id IS UNIQUE;
CREATE CONSTRAINT IF NOT EXISTS FOR (c:Criteria) REQUIRE c.id IS UNIQUE;
CREATE CONSTRAINT IF NOT EXISTS FOR (t:TestCase) REQUIRE t.id IS UNIQUE;

// Strategic Objects
CREATE CONSTRAINT IF NOT EXISTS FOR (rd:Roadmap) REQUIRE rd.id IS UNIQUE;
CREATE CONSTRAINT IF NOT EXISTS FOR (ms:Mission) REQUIRE ms.id IS UNIQUE;
CREATE CONSTRAINT IF NOT EXISTS FOR (v:Vision) REQUIRE v.id IS UNIQUE;

// Supporting Objects
CREATE CONSTRAINT IF NOT EXISTS FOR (d:Decision) REQUIRE d.id IS UNIQUE;
CREATE CONSTRAINT IF NOT EXISTS FOR (comp:Component) REQUIRE comp.id IS UNIQUE;
CREATE CONSTRAINT IF NOT EXISTS FOR (a:Account) REQUIRE a.id IS UNIQUE;
```

#### Example: Backlog Item Entity

```cypher
// Backlog Item node with all properties
CREATE (b:BacklogItem {
  id: "BLI-237",
  kind: "backlog_item",
  title: "Define Comprehensive System Ontology - All Objects and Relationships",
  status: "in_progress",
  priority: "critical",
  priority_tier: "P0",
  category: "Architecture",
  estimated_effort: "3-5 days",
  description: "Create complete system ontology defining all object types...",
  context: "Define comprehensive system ontology covering all object types...",
  created_at: "2025-12-24T05:45:00Z",
  updated_at: "2025-12-24T05:47:33Z",
  schema_version: "2.0.0",
  source_type: "internal",
  origin_project: "lanceettl",
  origin_system: "zqk",
  
  // Embedding for semantic search
  embedding: [0.12, -0.98, 0.45, ...],  // Vector embedding of title + description + context
  
  // Reference fields (stored as properties, also create edges)
  priority_plan_ref: "PRI-207",
  milestone_refs: ["MIL-035"],
  goal_refs: [],
  requirement_refs: []
})
```

**Entity Properties:**
- **Core Identity**: `id`, `kind`, `title`
- **Lifecycle**: `status`, `created_at`, `updated_at`
- **Classification**: `priority`, `priority_tier`, `category`
- **Content**: `description`, `context`, `notes`
- **Metadata**: `schema_version`, `source_type`, `origin_project`, `origin_system`
- **Semantic**: `embedding` (vector representation for semantic search)
- **References**: `*_ref` and `*_refs` fields (also create edges in Relationship Layer)

**Purpose:**
- Represents all project objects as graph nodes
- Enables structural queries and traversals
- Provides semantic search via embeddings
- Maintains object properties and metadata

### Layer 3: Relationship Layer

The Relationship Layer defines explicit edges between entities, representing the relationships from the System Ontology. Each `*_ref` and `*_refs` field creates one or more edges.

#### Edge Types (from System Ontology)

```cypher
// Primary relationship types
BELONGS_TO      // Backlog item belongs to priority plan
IMPLEMENTS       // Backlog item implements milestone
SUPPORTS         // Backlog item supports goal
DEFINES          // Requirement defines milestone
VALIDATES        // Test case validates requirement
COMPLETES        // Criteria completes milestone
ORGANIZES        // Priority plan organizes backlog items
CONTAINS         // Workstream contains priority plans and milestones
ACHIEVES         // Milestone achieves goal
OWNS             // Account owns workstream
LINKS_TO         // Generic linking relationship
DEPENDS_ON       // Dependency relationship
BLOCKS           // Blocking relationship
RELATES_TO       // General relationship
```

#### Example: Relationship Edges

```cypher
// Backlog item belongs to priority plan
MATCH (b:BacklogItem {id: "BLI-237"}), (p:PriorityPlan {id: "PRI-207"})
CREATE (b)-[:BELONGS_TO {
  created_at: "2025-12-24T05:45:00Z",
  updated_at: "2025-12-24T05:47:33Z",
  source_field: "priority_plan_ref"
}]->(p)

// Backlog item implements milestone
MATCH (b:BacklogItem {id: "BLI-237"}), (m:Milestone {id: "MIL-035"})
CREATE (b)-[:IMPLEMENTS {
  created_at: "2025-12-24T05:45:00Z",
  updated_at: "2025-12-24T05:47:33Z",
  source_field: "milestone_refs[0]"
}]->(m)

// Priority plan organizes backlog items (reverse direction for query efficiency)
MATCH (p:PriorityPlan {id: "PRI-207"}), (b:BacklogItem {id: "BLI-237"})
CREATE (p)-[:ORGANIZES {
  created_at: "2025-12-24T05:45:00Z",
  updated_at: "2025-12-24T05:47:33Z",
  priority_tier: "P0"
}]->(b)

// Workstream contains priority plan
MATCH (w:Workstream {id: "WS-007"}), (p:PriorityPlan {id: "PRI-207"})
CREATE (w)-[:CONTAINS {
  created_at: "2025-12-23T21:00:00Z",
  updated_at: "2025-12-24T04:20:30Z",
  source_field: "workstream_ref"
}]->(p)

// Milestone achieves goal
MATCH (m:Milestone {id: "MIL-035"}), (g:Goal {id: "GOAL-6369"})
CREATE (m)-[:ACHIEVES {
  created_at: "2025-12-24T05:23:50Z",
  updated_at: "2025-12-24T05:23:50Z",
  source_field: "goal_refs[0]"
}]->(g)
```

**Edge Properties:**
- `created_at`: When relationship was created
- `updated_at`: When relationship was last updated
- `source_field`: Which field in source object created this edge
- Relationship-specific properties (e.g., `priority_tier` for ORGANIZES)

**Purpose:**
- Enables graph traversal and multi-hop queries
- Maintains explicit relationship semantics
- Supports bidirectional traversal
- Provides relationship metadata and provenance

## Cross-Layer Connections

### Document → Entity Links

```cypher
// Link document chunks to entities
MATCH (d:Document {id: "doc_backlog_item_BLI-237"}), (b:BacklogItem {id: "BLI-237"})
CREATE (d)-[:EXTRACTS_TO]->(b)
```

**Purpose:** Trace entities back to source documents for provenance

### Entity → Document Links

```cypher
// Link entities to source documents
MATCH (b:BacklogItem {id: "BLI-237"}), (d:Document {id: "doc_backlog_item_BLI-237"})
CREATE (b)-[:SOURCED_FROM]->(d)
```

**Purpose:** Enable document retrieval from entities

## Schema Implementation Patterns

### Pattern 1: Reference Field to Edge Mapping

Every `*_ref` and `*_refs` field in object specifications creates edges:

```yaml
# From backlog_item.yaml
priority_plan_ref: PRI-207        # Creates: BELONGS_TO edge
milestone_refs: [MIL-035]         # Creates: IMPLEMENTS edge(s)
goal_refs: [GOAL-6369]            # Creates: SUPPORTS edge(s)
requirement_refs: [REQ-001]       # Creates: RELATES_TO edge(s)
```

**Mapping Rules:**
- Single reference (`*_ref`) → Single edge
- Multiple references (`*_refs`) → Multiple edges (one per reference)
- Edge type determined by relationship semantics (see Edge Types above)

### Pattern 2: Bidirectional Edges

Some relationships benefit from bidirectional edges for query efficiency:

```cypher
// Forward: Backlog item belongs to priority plan
(b:BacklogItem)-[:BELONGS_TO]->(p:PriorityPlan)

// Reverse: Priority plan organizes backlog items
(p:PriorityPlan)-[:ORGANIZES]->(b:BacklogItem)
```

**Purpose:** Enable efficient queries from either direction

### Pattern 3: Semantic Embeddings

All entity nodes include vector embeddings for semantic search:

```cypher
// Embedding generation strategy
embedding = embed(title + " " + description + " " + context)

// Semantic search query
MATCH (b:BacklogItem)
WHERE vector.similarity(b.embedding, $query_embedding) > 0.7
RETURN b
```

**Purpose:** Enable semantic search alongside structural queries

## Query Patterns

### Multi-Hop Query: Find all backlog items for a goal

```cypher
// Query: Find all backlog items that support goal GOAL-6369
MATCH (g:Goal {id: "GOAL-6369"})<-[:ACHIEVES]-(m:Milestone)<-[:IMPLEMENTS]-(b:BacklogItem)
RETURN b.id, b.title, b.status, b.priority_tier
ORDER BY b.priority_tier, b.id
```

### Multi-Hop Query: Find all test cases for a milestone

```cypher
// Query: Find all test cases that validate requirements for milestone MIL-035
MATCH (m:Milestone {id: "MIL-035"})<-[:DEFINES]-(r:Requirement)<-[:VALIDATES]-(t:TestCase)
RETURN t.id, t.title, t.status, t.category
```

### Semantic + Structural Query: Find similar backlog items

```cypher
// Query: Find backlog items semantically similar to BLI-237 that are in the same priority plan
MATCH (b1:BacklogItem {id: "BLI-237"})
MATCH (b1)-[:BELONGS_TO]->(p:PriorityPlan)<-[:BELONGS_TO]-(b2:BacklogItem)
WHERE b1.id <> b2.id
  AND vector.similarity(b1.embedding, b2.embedding) > 0.75
RETURN b2.id, b2.title, b2.status
ORDER BY vector.similarity(b1.embedding, b2.embedding) DESC
```

### Graph Traversal: Find dependency chain

```cypher
// Query: Find all objects that depend on milestone MIL-035 (backlog items, requirements, etc.)
MATCH path = (m:Milestone {id: "MIL-035"})<-[:IMPLEMENTS|DEFINES|COMPLETES*]-(dependent)
RETURN path, dependent.id, dependent.kind
```

## Backend Implementation Considerations

### MemGraph Implementation

```cypher
// MemGraph supports Cypher queries natively
// Use MemGraph's vector search extensions for semantic search
CREATE INDEX ON :BacklogItem(embedding) USING VECTOR INDEX;

// Query with vector similarity
MATCH (b:BacklogItem)
WHERE vector.similarity(b.embedding, $query_vector) > 0.7
RETURN b
```

### Neo4j Implementation

```cypher
// Neo4j with APOC and GDS libraries
// Use APOC for vector operations or integrate with external vector DB
CALL apoc.vector.similarity($query_vector, b.embedding) > 0.7

// Or use Neo4j's native graph algorithms
CALL gds.graph.project('project-graph', '*', '*')
```

### RDF Implementation

```turtle
# RDF representation using System Ontology schemes
@prefix goal: <urn:zqk:goal:> .
@prefix milestone: <urn:zqk:milestone:> .
@prefix backlog_item: <urn:zqk:backlog_item:> .

backlog_item:BLI-237
  a zqk:BacklogItem ;
  zqk:belongsTo priority_plan:PRI-207 ;
  zqk:implements milestone:MIL-035 ;
  zqk:title "Define Comprehensive System Ontology" ;
  zqk:status "in_progress" .
```

## Migration Strategy

### Phase 1: Document Layer Population

1. Read all YAML files from `docs/process/`
2. Create Document nodes for each file
3. Chunk large documents if needed
4. Store original content and metadata

### Phase 2: Entity Layer Extraction

1. Parse YAML files to extract object instances
2. Create Entity nodes for each object (Goal, Milestone, BacklogItem, etc.)
3. Populate all object properties
4. Generate vector embeddings for semantic search
5. Link entities to source documents

### Phase 3: Relationship Layer Construction

1. For each entity, process all `*_ref` and `*_refs` fields
2. Create edges based on reference field semantics
3. Create bidirectional edges where beneficial
4. Populate edge properties (timestamps, source fields)
5. Validate all references exist

### Validation

1. Verify all references resolve to existing entities
2. Check for orphaned entities (no relationships)
3. Validate edge types match System Ontology
4. Ensure document-entity links are complete
5. Test multi-hop queries

## Performance Considerations

### Indexing Strategy

```cypher
// Index on ID fields (already unique constraints)
CREATE INDEX ON :BacklogItem(id);
CREATE INDEX ON :Milestone(id);
CREATE INDEX ON :Goal(id);
// ... for all entity types

// Index on status for filtering
CREATE INDEX ON :BacklogItem(status);
CREATE INDEX ON :Milestone(status);

// Index on priority_tier for sorting
CREATE INDEX ON :BacklogItem(priority_tier);

// Vector index for semantic search
CREATE INDEX ON :BacklogItem(embedding) USING VECTOR INDEX;
```

### Query Optimization

1. **Use specific node types**: `MATCH (b:BacklogItem)` not `MATCH (n)`
2. **Limit traversal depth**: Use `[*1..3]` instead of `[*]`
3. **Filter early**: Add WHERE clauses before traversals
4. **Use relationship types**: `[:BELONGS_TO]` not `[*]`
5. **Project only needed properties**: `RETURN b.id, b.title` not `RETURN b`

## Future Extensions

### Temporal Relationships

```cypher
// Add time-based relationships
CREATE (b1:BacklogItem)-[:BLOCKS {
  created_at: "2025-12-24T10:00:00Z",
  resolved_at: "2025-12-24T15:00:00Z"
}]->(b2:BacklogItem)
```

### Code References

```cypher
// Link code entities to project objects
CREATE (b:BacklogItem {id: "BLI-237"})-[:IMPLEMENTED_BY]->(f:Function {
  id: "func_define_ontology",
  file_path: "pkg/ontology/design.go",
  line_start: 42,
  line_end: 156
})
```

### Semantic Relationships

```cypher
// Add semantic similarity edges
CREATE (b1:BacklogItem)-[:SEMANTICALLY_SIMILAR {
  similarity: 0.87,
  computed_at: "2025-12-24T12:00:00Z"
}]->(b2:BacklogItem)
```

## Related Documents

- **System Ontology v1.0**: `docs/process/ontology/system-ontology-v1.0.md`
- **Pluggable Backend Interface**: BLI-239 (Design Pluggable Graph Backend Interface Architecture)
- **Migration Strategy**: BLI-240 (Design Migration Strategy from File-Based to Graph Backend)
- **Milestone**: MIL-035 (Knowledge Kernel Graph Structure)

## Next Steps

1. ✅ **Complete**: System Ontology v1.0
2. ✅ **Complete**: GraphRAG Schema Design v1.0
3. **Next**: BLI-239 - Design Pluggable Graph Backend Interface Architecture
4. **Next**: BLI-240 - Design Migration Strategy from File-Based to Graph Backend
5. **Next**: BLI-624 - Implement MemGraph Backend Driver (Phase 2)

---

**Status**: Design Complete - Ready for Implementation  
**Approved By**: Architecture Review  
**Implementation Target**: Phase 2 (BLI-624, BLI-623)

