# Graph Backend Architecture Decisions

## Connection Pooling Architecture

### Decision: Pool-First Design

**Problem**: Initial design had `GraphProvider.Connect()` returning a single `GraphConnection`, which doesn't scale well and doesn't match common Go patterns.

**Solution**: Refactored to pool-first design:
- `GraphProvider.CreatePool()` returns a `ConnectionPool`
- Pool manages connections internally
- Users acquire connections from pool, use them, then return them
- Convenience method `Execute()` handles acquire/release automatically

**Benefits**:
- Matches Go's `database/sql` pattern
- Better resource management
- Automatic connection lifecycle
- Vendor-specific implementations hidden behind interface

**Example Usage**:
```go
pool, err := provider.CreatePool(ctx, config)
defer pool.Close()

// Option 1: Manual connection management
conn, err := pool.GetConnection(ctx)
defer pool.ReturnConnection(conn)
err = conn.CreateNode(ctx, node)

// Option 2: Convenience method (recommended)
err = pool.Execute(ctx, func(conn GraphConnection) error {
    return conn.CreateNode(ctx, node)
})
```

## Transaction Management

### Decision: Explicit Transaction Lifecycle

**Problem**: Unclear what happens when `Close()` is called on a connection with an open transaction.

**Solution**:
- Transactions must be explicitly committed or rolled back
- `GraphConnection.HasOpenTransaction()` and `GetOpenTransaction()` allow checking state
- `ConnectionPool.ReturnConnection()` automatically rolls back any open transactions
- `ConnectionPool.Close()` rolls back all open transactions before closing

**Benefits**:
- Prevents accidental data loss
- Clear lifecycle management
- Safe connection return to pool

### Decision: Nested Transaction Support

**Problem**: Need to support nested transactions (savepoints) for complex operations, but not all backends support them.

**Solution**:
- `GraphTransaction.BeginNestedTransaction()` for savepoints
- `GraphTransaction.SupportsNestedTransactions()` to check capability
- Returns error if not supported (graceful degradation)
- Nested transactions can be committed/rolled back independently

**Benefits**:
- Future-proof design
- Graceful degradation for backends without savepoint support
- Enables complex transaction patterns

## Vendor-Specific Implementations

### Decision: Hidden Implementation Details

**Problem**: Exposing vendor-specific connection types makes it harder to swap backends.

**Solution**:
- Vendor-specific connection types are internal to their packages
- Only `GraphConnection` interface is exposed
- Pool wraps vendor connections and returns interface
- Implementation details hidden from consumers

**Structure**:
```
pkg/graph/
├── provider/          # Public interfaces
│   ├── interfaces.go  # GraphProvider, ConnectionPool, GraphConnection
│   └── types.go       # Node, Edge, Query, etc.
├── memgraph/          # MemGraph implementation (internal)
│   ├── pool.go        # MemGraphConnectionPool (implements ConnectionPool)
│   ├── connection.go  # memgraphConnection (implements GraphConnection, internal)
│   └── transaction.go # memgraphTransaction (implements GraphTransaction, internal)
└── neo4j/             # Neo4j implementation (internal)
    └── ...
```

## Auto-Commit vs Explicit Transactions

### Decision: Auto-Commit by Default, Explicit Transactions for Multi-Operation Atomicity

**Problem**: Need to clarify when operations are committed and how transactions work.

**Solution**:
- **Auto-Commit Mode (Default)**: All operations outside of explicit transactions are automatically committed
  - `CreateNode()`, `UpdateNode()`, `CreateEdge()`, etc. are immediately committed
  - Each operation is atomic on its own
  - No explicit commit needed for single operations

- **Explicit Transactions**: For multi-operation atomicity
  - Call `BeginTransaction()` to start a transaction
  - All operations within the transaction are NOT auto-committed
  - Must explicitly call `Commit()` to persist or `Rollback()` to discard
  - If connection is returned to pool with open transaction, it's rolled back (safety mechanism)

**Benefits**:
- Simple operations don't require transaction management
- Complex multi-operation workflows can use explicit transactions
- Safety mechanism prevents accidental data loss

**Example**:
```go
// Auto-commit: Single operation is immediately committed
err = conn.CreateNode(ctx, node)

// Explicit transaction: Multiple operations are atomic
tx, _ := conn.BeginTransaction(ctx)
tx.CreateNode(ctx, node1)
tx.CreateNode(ctx, node2)
tx.CreateEdge(ctx, edge)
err = tx.Commit(ctx) // All operations committed together, or none if error
```

## Timeout and Retry Handling

### Decision: Context-Based Timeouts with Configurable Retry Logic

**Problem**: Need to handle timeouts and transient failures gracefully.

**Solution**:
- **Context-Based Timeouts**: All operations respect `context.Context` deadlines
  - `ConnectionConfig.QueryTimeout` provides default timeout
  - Can be overridden per-operation via context
  - Pool acquisition respects `ConnectionConfig.PoolTimeout`

- **Retry Configuration**: Configurable retry logic for transient failures
  - `RetryConfig` with exponential backoff
  - Retries only on retryable errors (timeouts, connection failures)
  - Configurable per-pool or per-operation

**Benefits**:
- Prevents operations from hanging indefinitely
- Handles transient network/database issues automatically
- Configurable for different use cases

**Example**:
```go
config := ConnectionConfig{
    QueryTimeout: 30 * time.Second,
    RetryConfig: &RetryConfig{
        MaxAttempts: 3,
        InitialDelay: 100 * time.Millisecond,
        BackoffFactor: 2.0,
    },
}

// Operation respects context timeout
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
err = conn.CreateNode(ctx, node)
```

## Migration Path

### Backward Compatibility

The original `GraphProvider.Connect()` method can be kept as a convenience that:
1. Creates a pool with MaxConns=1
2. Gets a connection
3. Returns a wrapper that closes the pool on connection close

This allows gradual migration while maintaining backward compatibility.

