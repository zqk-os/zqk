# Commit and Timeout Semantics

## Auto-Commit vs Explicit Transactions

### Auto-Commit (Default Behavior)

**All operations outside of explicit transactions are automatically committed.**

```go
// These operations are immediately committed
conn.CreateNode(ctx, node)      // ✅ Auto-committed
conn.UpdateNode(ctx, id, updates) // ✅ Auto-committed
conn.CreateEdge(ctx, edge)       // ✅ Auto-committed
```

**Key Points**:
- Each operation is atomic on its own
- No explicit commit needed
- Simple and intuitive for single operations
- Matches common database behavior (e.g., PostgreSQL autocommit mode)

### Explicit Transactions

**Operations within a transaction are NOT auto-committed. You must explicitly commit or rollback.**

```go
// Start a transaction
tx, err := conn.BeginTransaction(ctx)
if err != nil {
    return err
}

// These operations are NOT committed yet
tx.CreateNode(ctx, node1)  // ⏳ Pending
tx.CreateNode(ctx, node2)  // ⏳ Pending
tx.CreateEdge(ctx, edge)   // ⏳ Pending

// Explicitly commit all operations together
err = tx.Commit(ctx)  // ✅ All operations now committed atomically
// OR
err = tx.Rollback(ctx)  // ❌ All operations discarded
```

**Key Points**:
- Transactions provide atomicity for multiple operations
- All operations in a transaction succeed together or fail together
- Must explicitly call `Commit()` or `Rollback()`
- If you forget to commit/rollback and return the connection to the pool, it's automatically rolled back (safety mechanism)

### Safety Mechanism: Auto-Rollback on Connection Return

**If a connection is returned to the pool with an open transaction, it is automatically rolled back.**

This is a **safety mechanism**, not the primary commit path. The intent is to prevent:
- Accidental data loss from forgotten transactions
- Connections with open transactions being reused incorrectly
- Data inconsistency from partial transactions

```go
tx, _ := conn.BeginTransaction(ctx)
tx.CreateNode(ctx, node)
// Oops! Forgot to commit...

pool.ReturnConnection(conn)  // ⚠️ Transaction is automatically rolled back
// The CreateNode operation is discarded
```

**Best Practice**: Always explicitly commit or rollback transactions before returning the connection.

## Timeout Handling

### Context-Based Timeouts

All operations respect `context.Context` deadlines. This provides:
- Per-operation timeout control
- Cancellation support
- Integration with Go's standard timeout patterns

```go
// Set timeout for a specific operation
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

err := conn.CreateNode(ctx, node)  // Will timeout after 5 seconds
```

### Connection Configuration Timeouts

`ConnectionConfig` provides default timeouts:

```go
config := ConnectionConfig{
    ConnectTimeout: 10 * time.Second,  // Timeout for establishing connections
    QueryTimeout:   30 * time.Second,  // Default timeout for queries
    PoolTimeout:    5 * time.Second,  // Timeout for acquiring connection from pool
}
```

**Timeout Precedence**:
1. Context deadline (if set) - highest priority
2. ConnectionConfig.QueryTimeout - default for operations
3. No timeout - operations may hang indefinitely (not recommended)

## Retry Handling

### Retry Configuration

Retry logic is configurable via `RetryConfig`:

```go
retryConfig := &RetryConfig{
    MaxAttempts:   3,                    // Try up to 3 times
    InitialDelay:  100 * time.Millisecond, // Wait 100ms before first retry
    MaxDelay:      5 * time.Second,       // Never wait more than 5s
    BackoffFactor: 2.0,                   // Double delay each retry
    RetryableErrors: []ErrorCode{
        ErrorCodeConnectionFailed,
        ErrorCodeTimeout,
    },
}
```

### Retryable Errors

By default, only transient errors are retried:
- `ErrorCodeConnectionFailed` - Connection lost
- `ErrorCodeTimeout` - Operation timed out
- `ErrorCodeDeadlineExceeded` - Context deadline exceeded

**Non-retryable errors** (returned immediately):
- `ErrorCodeNodeNotFound` - Data doesn't exist
- `ErrorCodeValidationFailed` - Invalid input
- `ErrorCodeTransactionFailed` - Transaction error

### Using Retry

```go
// Retry is automatically applied to operations if RetryConfig is set in ConnectionConfig
config := ConnectionConfig{
    RetryConfig: DefaultRetryConfig(),
}

pool, _ := provider.CreatePool(ctx, config)

// This operation will automatically retry on transient failures
err := pool.Execute(ctx, func(conn GraphConnection) error {
    return conn.CreateNode(ctx, node)
})
```

## Summary

| Scenario | Commit Behavior | Timeout | Retry |
|----------|----------------|---------|-------|
| Single operation outside transaction | ✅ Auto-committed | Context or QueryTimeout | If configured |
| Multiple operations in transaction | ⏳ Pending until `Commit()` | Context or QueryTimeout | If configured |
| Connection returned with open transaction | ❌ Auto-rolled back (safety) | N/A | N/A |
| Operation with context timeout | ✅ Auto-committed (if succeeds) | Context deadline | If configured |
| Operation with retry config | ✅ Auto-committed (if succeeds) | Context or QueryTimeout | Per RetryConfig |

## Best Practices

1. **Use auto-commit for simple operations** - No transaction needed
2. **Use explicit transactions for multi-operation atomicity** - Always commit or rollback explicitly
3. **Set timeouts** - Use context timeouts or ConnectionConfig defaults
4. **Configure retry for transient failures** - Use RetryConfig for network/database issues
5. **Never return connections with open transactions** - Always commit or rollback first (safety mechanism will catch it, but it's better to be explicit)

