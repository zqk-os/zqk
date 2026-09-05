# Pluggable Graph Backend Interface Architecture v1.0

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2025-12-24  
**Status:** Design Complete  
**Related:** BLI-239, BLI-237, BLI-238, BLI-240, BLI-624, MIL-035

## Overview

This document defines the pluggable graph backend interface architecture for the zqk Knowledge Kernel. The interface abstracts backend-specific details while providing a unified API for graph operations across multiple backend implementations (MemGraph, Neo4j, RDF stores).

## Architecture Principles

1. **Backend Agnostic**: Interface abstracts all backend-specific operations
2. **Extensible**: Easy to add new backend implementations
3. **Type Safe**: Strong typing for nodes, edges, and queries
4. **Transaction Support**: ACID transactions where supported
5. **Connection Pooling**: Efficient connection management
6. **Error Handling**: Comprehensive error types and retry logic
7. **Query Abstraction**: Unified query interface supporting Cypher, SPARQL, and native queries

## Core Interfaces

### GraphProvider Interface

The `GraphProvider` interface is the entry point for backend implementations. It provides factory methods for creating connections and querying backend capabilities.

```go
type GraphProvider interface {
    // Create a new connection to the graph backend
    Connect(ctx context.Context, config ConnectionConfig) (GraphConnection, error)
    
    // Check if the provider supports a specific feature
    SupportsFeature(feature Feature) bool
    
    // Get provider capabilities and metadata
    GetCapabilities() Capabilities
    
    // Get provider name and version
    GetProviderInfo() ProviderInfo
}

type ProviderInfo struct {
    Name        string
    Version     string
    Description string
}

type Capabilities struct {
    Features          []Feature
    QueryLanguages    []QueryLanguage
    TransactionSupport bool
    VectorSearch      bool
    MultiHopTraversal bool
    BatchOperations   bool
    StreamingQueries bool
}

type Feature string

const (
    FeatureCypherQuery      Feature = "cypher_query"
    FeatureSPARQLQuery      Feature = "sparql_query"
    FeatureVectorSearch     Feature = "vector_search"
    FeatureMultiHopTraversal Feature = "multi_hop_traversal"
    FeatureTransactions     Feature = "transactions"
    FeatureBatchOperations  Feature = "batch_operations"
    FeatureStreamingQueries Feature = "streaming_queries"
    FeatureGraphAlgorithms  Feature = "graph_algorithms"
)

type QueryLanguage string

const (
    QueryLanguageCypher QueryLanguage = "cypher"
    QueryLanguageSPARQL QueryLanguage = "sparql"
    QueryLanguageGremlin QueryLanguage = "gremlin"
    QueryLanguageNative QueryLanguage = "native"
)
```

### GraphConnection Interface

The `GraphConnection` interface represents an active connection to a graph backend. It provides methods for CRUD operations, queries, and transaction management.

```go
type GraphConnection interface {
    // Node operations
    CreateNode(ctx context.Context, node Node) error
    GetNode(ctx context.Context, id string, labels []string) (*Node, error)
    UpdateNode(ctx context.Context, id string, updates NodeUpdates) error
    DeleteNode(ctx context.Context, id string, labels []string) error
    ListNodes(ctx context.Context, filter NodeFilter) ([]*Node, error)
    
    // Edge operations
    CreateEdge(ctx context.Context, edge Edge) error
    GetEdge(ctx context.Context, fromID, toID string, edgeType string) (*Edge, error)
    UpdateEdge(ctx context.Context, fromID, toID string, edgeType string, updates EdgeUpdates) error
    DeleteEdge(ctx context.Context, fromID, toID string, edgeType string) error
    ListEdges(ctx context.Context, filter EdgeFilter) ([]*Edge, error)
    
    // Query operations
    ExecuteQuery(ctx context.Context, query Query) (*QueryResult, error)
    ExecuteVectorQuery(ctx context.Context, query VectorQuery) (*QueryResult, error)
    ExecuteTraversal(ctx context.Context, traversal TraversalQuery) (*QueryResult, error)
    
    // Transaction management
    BeginTransaction(ctx context.Context) (GraphTransaction, error)
    
    // Connection management
    HealthCheck(ctx context.Context) error
    Close() error
    
    // Batch operations
    ExecuteBatch(ctx context.Context, operations []Operation) (*BatchResult, error)
}

type Node struct {
    ID      string
    Labels  []string
    Properties map[string]any
}

type Edge struct {
    FromID     string
    ToID       string
    Type       string
    Properties map[string]any
}

type Query struct {
    Language QueryLanguage
    Query    string
    Params   map[string]any
}

type VectorQuery struct {
    Query        string
    Vector       []float32
    Limit        int
    Threshold    float32
    Filter       NodeFilter
}

type TraversalQuery struct {
    StartNodeID  string
    Relationship string
    Direction    Direction
    MaxDepth     int
    Filter       NodeFilter
}

type Direction string

const (
    DirectionOutgoing Direction = "outgoing"
    DirectionIncoming Direction = "incoming"
    DirectionBoth     Direction = "both"
)
```

### GraphTransaction Interface

The `GraphTransaction` interface provides transaction support for backends that support ACID transactions.

```go
type GraphTransaction interface {
    // Transaction operations (same as GraphConnection but within transaction)
    CreateNode(ctx context.Context, node Node) error
    GetNode(ctx context.Context, id string, labels []string) (*Node, error)
    UpdateNode(ctx context.Context, id string, updates NodeUpdates) error
    DeleteNode(ctx context.Context, id string, labels []string) error
    
    CreateEdge(ctx context.Context, edge Edge) error
    GetEdge(ctx context.Context, fromID, toID string, edgeType string) (*Edge, error)
    UpdateEdge(ctx context.Context, fromID, toID string, edgeType string, updates EdgeUpdates) error
    DeleteEdge(ctx context.Context, fromID, toID string, edgeType string) error
    
    ExecuteQuery(ctx context.Context, query Query) (*QueryResult, error)
    
    // Transaction control
    Commit(ctx context.Context) error
    Rollback(ctx context.Context) error
}
```

## Backend Implementations

### MemGraphProvider

MemGraph implementation using native Cypher queries and vector search extensions.

```go
type MemGraphProvider struct {
    config MemGraphConfig
}

type MemGraphConfig struct {
    Host     string
    Port     int
    Username string
    Password string
    Database string
    PoolSize int
}

func (p *MemGraphProvider) Connect(ctx context.Context, config ConnectionConfig) (GraphConnection, error) {
    // Create MemGraph connection using memgraph-go client
    // Configure connection pooling
    // Return MemGraphConnection
}

func (p *MemGraphProvider) SupportsFeature(feature Feature) bool {
    switch feature {
    case FeatureCypherQuery, FeatureVectorSearch, FeatureMultiHopTraversal,
         FeatureTransactions, FeatureBatchOperations:
        return true
    default:
        return false
    }
}

type MemGraphConnection struct {
    client *memgraph.Client
    pool   *connection.Pool
}

func (c *MemGraphConnection) ExecuteVectorQuery(ctx context.Context, query VectorQuery) (*QueryResult, error) {
    // Use MemGraph's vector search extensions
    // Execute similarity search query
    // Return results
}
```

**Key Features:**
- Native Cypher query support
- Vector similarity search via extensions
- Transaction support
- Connection pooling
- Batch operations

### Neo4jProvider

Neo4j implementation using Bolt protocol and APOC/GDS integration.

```go
type Neo4jProvider struct {
    config Neo4jConfig
}

type Neo4jConfig struct {
    URI      string
    Username string
    Password string
    Database string
    PoolSize int
}

func (p *Neo4jProvider) Connect(ctx context.Context, config ConnectionConfig) (GraphConnection, error) {
    // Create Neo4j driver using neo4j-go-driver
    // Configure connection pooling
    // Return Neo4jConnection
}

func (p *Neo4jProvider) SupportsFeature(feature Feature) bool {
    switch feature {
    case FeatureCypherQuery, FeatureMultiHopTraversal,
         FeatureTransactions, FeatureGraphAlgorithms:
        return true
    case FeatureVectorSearch:
        // Depends on Neo4j version and plugins
        return p.config.VectorSearchEnabled
    default:
        return false
    }
}

type Neo4jConnection struct {
    driver neo4j.Driver
    pool   *neo4j.SessionPool
}

func (c *Neo4jConnection) ExecuteTraversal(ctx context.Context, traversal TraversalQuery) (*QueryResult, error) {
    // Use Neo4j GDS (Graph Data Science) library for advanced traversals
    // Execute traversal query
    // Return results
}
```

**Key Features:**
- Cypher via Bolt protocol
- APOC (Awesome Procedures on Cypher) integration
- GDS (Graph Data Science) algorithms
- Transaction support
- Connection pooling
- Optional vector search (via plugins)

### RDFProvider

RDF store implementation using SPARQL queries and RDF/OWL semantics.

```go
type RDFProvider struct {
    config RDFConfig
}

type RDFConfig struct {
    Endpoint string
    Username string
    Password string
    Format   RDFFormat
}

type RDFFormat string

const (
    RDFFormatTurtle RDFFormat = "turtle"
    RDFFormatRDFXML RDFFormat = "rdfxml"
    RDFFormatJSONLD RDFFormat = "jsonld"
    RDFFormatN3     RDFFormat = "n3"
)

func (p *RDFProvider) Connect(ctx context.Context, config ConnectionConfig) (GraphConnection, error) {
    // Create SPARQL endpoint connection
    // Return RDFConnection
}

func (p *RDFProvider) SupportsFeature(feature Feature) bool {
    switch feature {
    case FeatureSPARQLQuery:
        return true
    case FeatureVectorSearch, FeatureTransactions:
        // Depends on RDF store implementation
        return p.config.SupportsFeature
    default:
        return false
    }
}

type RDFConnection struct {
    endpoint *sparql.Endpoint
    client   *http.Client
}

func (c *RDFConnection) ExecuteQuery(ctx context.Context, query Query) (*QueryResult, error) {
    // Convert Query to SPARQL
    // Execute SPARQL query
    // Parse results and return
}

func (c *RDFConnection) CreateNode(ctx context.Context, node Node) error {
    // Convert Node to RDF triples
    // Execute SPARQL INSERT query
    // Handle RDF semantics (subjects, predicates, objects)
}
```

**Key Features:**
- SPARQL query support
- RDF/OWL semantics
- Triple store operations
- Semantic reasoning (where supported)
- Optional transaction support (depends on store)

## Connection Management

### Connection Pooling

All providers implement connection pooling for efficient resource management.

```go
type ConnectionPool interface {
    Acquire(ctx context.Context) (GraphConnection, error)
    Release(conn GraphConnection) error
    Close() error
    Stats() PoolStats
}

type PoolStats struct {
    Active    int
    Idle      int
    MaxSize   int
    WaitCount int64
}
```

### Health Checks

All connections support health checks for monitoring and failover.

```go
type HealthStatus struct {
    Status    HealthState
    Latency   time.Duration
    Error     error
    Timestamp time.Time
}

type HealthState string

const (
    HealthStateHealthy   HealthState = "healthy"
    HealthStateDegraded  HealthState = "degraded"
    HealthStateUnhealthy HealthState = "unhealthy"
)
```

## Error Handling

### Error Types

```go
type GraphError struct {
    Code    ErrorCode
    Message string
    Cause   error
    Backend string
}

type ErrorCode string

const (
    ErrorCodeConnectionFailed ErrorCode = "connection_failed"
    ErrorCodeQueryFailed       ErrorCode = "query_failed"
    ErrorCodeNodeNotFound      ErrorCode = "node_not_found"
    ErrorCodeEdgeNotFound      ErrorCode = "edge_not_found"
    ErrorCodeTransactionFailed ErrorCode = "transaction_failed"
    ErrorCodeValidationFailed  ErrorCode = "validation_failed"
    ErrorCodeTimeout           ErrorCode = "timeout"
    ErrorCodeRetryExhausted    ErrorCode = "retry_exhausted"
)
```

### Retry Logic

All operations support configurable retry logic for transient failures.

```go
type RetryConfig struct {
    MaxAttempts int
    InitialDelay time.Duration
    MaxDelay     time.Duration
    BackoffFactor float64
    RetryableErrors []ErrorCode
}

func (c *GraphConnection) ExecuteQueryWithRetry(ctx context.Context, query Query, config RetryConfig) (*QueryResult, error) {
    // Implement exponential backoff retry logic
    // Retry on transient errors
    // Return final result or error
}
```

## Query Abstraction

### Query Builder

A query builder provides a fluent API for constructing queries across backends.

```go
type QueryBuilder interface {
    Match(pattern string) QueryBuilder
    Where(condition string) QueryBuilder
    Return(fields ...string) QueryBuilder
    Limit(n int) QueryBuilder
    OrderBy(field string, direction string) QueryBuilder
    Build() Query
}

// Example usage
query := builder.
    Match("(n:BacklogItem)").
    Where("n.status = $status").
    Return("n.id", "n.title").
    Limit(10).
    Build()
```

### Query Translation

For backends that don't support Cypher (e.g., RDF), queries are translated to the native query language.

```go
type QueryTranslator interface {
    Translate(query Query, targetLanguage QueryLanguage) (Query, error)
}

// Example: Translate Cypher to SPARQL
translator := NewSPARQLTranslator()
sparqlQuery, err := translator.Translate(cypherQuery, QueryLanguageSPARQL)
```

## Integration with GraphRAG Schema

The interface integrates with the GraphRAG Schema Design v1.0:

### Document Layer Operations

```go
// Create document node
doc := Node{
    ID: "doc_backlog_item_BLI-237",
    Labels: []string{"Document"},
    Properties: map[string]any{
        "type": "backlog_item",
        "source_path": "docs/process/backlog/BLI-237.yaml",
        "content": yamlContent,
    },
}
conn.CreateNode(ctx, doc)
```

### Entity Layer Operations

```go
// Create entity node
entity := Node{
    ID: "BLI-237",
    Labels: []string{"BacklogItem", "Entity"},
    Properties: map[string]any{
        "title": "Define Comprehensive System Ontology",
        "status": "complete",
        "priority": "critical",
    },
}
conn.CreateNode(ctx, entity)

// Link entity to document
edge := Edge{
    FromID: "doc_backlog_item_BLI-237",
    ToID: "BLI-237",
    Type: "CONTAINS_ENTITY",
    Properties: map[string]any{
        "extracted_at": time.Now(),
    },
}
conn.CreateEdge(ctx, edge)
```

### Relationship Layer Operations

```go
// Create relationship edge
rel := Edge{
    FromID: "BLI-237",
    ToID: "PRI-207",
    Type: "BELONGS_TO_PLAN",
    Properties: map[string]any{
        "priority_tier": "P0",
        "added_at": time.Now(),
    },
}
conn.CreateEdge(ctx, rel)
```

## Implementation Strategy

### Phase 1: Interface Definition (Current)

- Define all core interfaces
- Create type definitions
- Document interface contracts
- Define error types and handling

### Phase 2: MemGraph Implementation (BLI-624)

- Implement MemGraphProvider
- Implement MemGraphConnection
- Implement transaction support
- Add vector search support
- Create unit tests

### Phase 3: Storage Abstraction Layer

- Create Kernel Connector that uses GraphProvider
- Map object operations to graph operations
- Implement caching layer
- Add migration support

### Phase 4: Additional Backends (Future)

- Implement Neo4jProvider
- Implement RDFProvider
- Add backend selection logic
- Create backend comparison tests

## Testing Strategy

### Unit Tests

- Test each interface method independently
- Mock backend connections
- Test error handling and retries
- Test query translation

### Integration Tests

- Test with real backend instances (Docker containers)
- Test transaction behavior
- Test connection pooling
- Test performance characteristics

### Compatibility Tests

- Test query compatibility across backends
- Test feature detection
- Test graceful degradation

## Performance Considerations

### Connection Pooling

- Configurable pool sizes per backend
- Connection lifecycle management
- Health check integration

### Query Optimization

- Query caching where supported
- Prepared statement support
- Batch operation optimization

### Vector Search

- Efficient similarity search
- Index management
- Embedding dimension handling

## Security Considerations

### Authentication

- Support for various auth mechanisms (username/password, tokens, certificates)
- Secure credential storage
- Connection encryption (TLS/SSL)

### Authorization

- Backend-specific authorization support
- Query-level permissions
- Role-based access control (where supported)

## Related Documents

- **System Ontology v1.0**: `docs/process/ontology/system-ontology-v1.0.md`
- **GraphRAG Schema Design v1.0**: `docs/process/architecture/graphrag-schema-design-v1.0.md`
- **Migration Strategy**: BLI-240 (Design Migration Strategy from File-Based to Graph Backend)
- **MemGraph Implementation**: BLI-624 (Implement MemGraph Backend Driver)
- **Milestone**: MIL-035 (Knowledge Kernel Graph Structure)

## Next Steps

1. ✅ **Complete**: Interface Architecture Design
2. **Next**: BLI-624 - Implement MemGraph Backend Driver
3. **Next**: BLI-240 - Design Migration Strategy from File-Based to Graph Backend
4. **Next**: Implement Storage Abstraction Layer (Kernel Connector)
5. **Next**: Create integration tests

---

**Status**: Design Complete - Ready for Implementation  
**Approved By**: Architecture Review  
**Implementation Target**: Phase 2 (BLI-624)

