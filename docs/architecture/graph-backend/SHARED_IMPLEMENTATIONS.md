# Shared Implementation Patterns

## Overview

To keep code DRY across different graph backend providers, we've extracted common logic into shared base implementations.

## What's Shared

### 1. Connection Pool Management (`BasePool`)

**Location**: `pkg/graph/provider/base_pool.go`

**What it provides**:
- Connection acquisition with timeout support
- Connection release with transaction safety (auto-rollback)
- Pool statistics tracking
- Execute() convenience method
- Pre-population support

**What providers implement**:
- Connection creation logic (provider-specific)
- Connection configuration (provider-specific)

**Usage**:
```go
type MyProviderPool struct {
    *provider.BasePool
    config MyProviderConfig
}

func NewMyProviderPool(config MyProviderConfig) (*MyProviderPool, error) {
    pool := &MyProviderPool{
        BasePool: provider.NewBasePool(connConfig, maxSize),
        config:   config,
    }
    return pool, nil
}

func (p *MyProviderPool) createConnection() (provider.ConnectionWrapper, error) {
    // Provider-specific connection creation
    return &myConnection{...}, nil
}

func (p *MyProviderPool) GetConnection(ctx context.Context) (provider.GraphConnection, error) {
    return p.BasePool.GetConnection(ctx, p.createConnection)
}
```

### 2. Connection State Management (`BaseConnection`)

**Location**: `pkg/graph/provider/base_connection.go`

**What it provides**:
- Transaction state tracking (HasOpenTransaction, GetOpenTransaction)
- Transaction state updates (SetOpenTransaction, ClearTransaction)
- Connection reset logic (Reset)

**What providers implement**:
- Provider-specific connection logic (query execution, etc.)
- Provider-specific reset logic (if needed)

**Usage**:
```go
type myConnection struct {
    *provider.BaseConnection // Embedded for transaction state
    client *MyProviderClient
}

func (c *myConnection) BeginTransaction(ctx context.Context) (provider.GraphTransaction, error) {
    tx, err := c.client.BeginTx()
    if err != nil {
        return nil, err
    }
    
    wrapper := &myTransaction{tx: tx, parent: c}
    c.SetOpenTransaction(wrapper) // Track transaction state
    return wrapper, nil
}
```

### 3. Retry Logic (`Retry`)

**Location**: `pkg/graph/provider/retry.go`

**What it provides**:
- Exponential backoff retry logic
- Configurable retry attempts and delays
- Retryable error detection

**Usage**:
```go
err := provider.Retry(ctx, retryConfig, func() error {
    return conn.CreateNode(ctx, node)
})
```

### 4. Error Types (`GraphError`, `ErrorCode`)

**Location**: `pkg/graph/provider/interfaces.go`

**What it provides**:
- Standardized error types across all providers
- Error code constants
- Retryable error detection

**Usage**:
```go
return &provider.GraphError{
    Code:    provider.ErrorCodeConnectionFailed,
    Message: "failed to connect to database",
    Cause:   err,
    Backend:  "memgraph",
}
```

## What's Provider-Specific

### Must Implement Per Provider

1. **Connection Creation**
   - How to connect to the database
   - Provider-specific client initialization
   - Connection validation

2. **Query Execution**
   - Cypher (MemGraph, Neo4j)
   - SPARQL (RDF stores)
   - Native queries
   - Query result parsing

3. **Transaction Implementation**
   - How transactions work in the provider
   - Savepoint support (for nested transactions)
   - Commit/rollback semantics

4. **Node/Edge Operations**
   - How to create/update/delete nodes
   - How to create/update/delete edges
   - Provider-specific optimizations

5. **Connection Cleanup**
   - Closing provider-specific resources
   - Connection health checks

## Example: Adding a New Provider

```go
// 1. Define provider config
type Neo4jConfig struct {
    URI      string
    Username string
    Password string
}

// 2. Create pool embedding BasePool
type Neo4jConnectionPool struct {
    *provider.BasePool
    config Neo4jConfig
}

func NewNeo4jConnectionPool(config Neo4jConfig) (*Neo4jConnectionPool, error) {
    pool := &Neo4jConnectionPool{
        BasePool: provider.NewBasePool(connConfig, maxSize),
        config:   config,
    }
    return pool, nil
}

// 3. Create connection embedding BaseConnection
type neo4jConnection struct {
    *provider.BaseConnection
    driver neo4j.Driver
    session neo4j.Session
}

// 4. Implement provider-specific methods
func (c *neo4jConnection) CreateNode(ctx context.Context, node provider.Node) error {
    // Neo4j-specific implementation
    query := "CREATE (n:Label $props)"
    _, err := c.session.Run(ctx, query, map[string]any{"props": node.Properties})
    return err
}
```

## Benefits

1. **DRY**: Common logic written once, reused everywhere
2. **Consistency**: All providers behave the same way for pool management
3. **Maintainability**: Fix bugs once, all providers benefit
4. **Testability**: Base implementations can be tested independently
5. **Speed**: New providers can be added quickly by focusing on provider-specific logic

## Migration Path

Existing providers (like MemGraph) can be gradually refactored to use base implementations:

1. Embed `BaseConnection` in connection struct
2. Embed `BasePool` in pool struct
3. Implement `ConnectionWrapper` interface
4. Delegate pool operations to `BasePool`
5. Remove duplicate code

