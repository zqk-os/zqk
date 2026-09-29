# Graph Backend Package

This package implements the pluggable graph backend interface for the zqk knowledge kernel.

## Package Structure

```
pkg/graph/
├── provider/          # Core interfaces and shared implementations
│   ├── interfaces.go  # GraphProvider, ConnectionPool, GraphConnection interfaces
│   ├── base_pool.go   # Shared connection pool logic
│   ├── base_connection.go  # Shared connection state management
│   ├── retry.go       # Retry logic with exponential backoff
│   ├── metrics.go     # Metrics collection interface and default implementation
│   ├── metrics_config.go  # Metrics configuration
│   └── health.go      # Health diagnosis and recommendations
├── memgraph/          # MemGraph backend implementation
│   ├── provider.go    # MemGraphProvider
│   ├── pool.go        # MemGraphConnectionPool
│   └── connection.go  # MemGraphConnection
└── storage/           # Storage abstraction layer (future)
```

## Quick Start

```go
import (
    "github.com/zqk-os/zqk/pkg/graph/memgraph"
    "github.com/zqk-os/zqk/pkg/graph/provider"
)

// Create provider
mgProvider := memgraph.NewMemGraphProvider(memgraph.MemGraphConfig{
    Host: "localhost",
    Port: 7687,
})

// Create pool
pool, err := mgProvider.CreatePool(ctx, provider.ConnectionConfig{
    Host: "localhost",
    Port: 7687,
    MaxConns: 10,
})
defer pool.Close()

// Execute operation
err = pool.Execute(ctx, func(conn provider.GraphConnection) error {
    return conn.CreateNode(ctx, provider.Node{
        ID: "BLI-001",
        Labels: []string{"BacklogItem"},
        Properties: map[string]any{
            "title": "Define System Ontology",
        },
    })
})
```

## Documentation

**Core architecture standards are documented in `docs/architecture/`:**
- [Storage Architecture](../../docs/architecture/TIERED_STORAGE_AND_ARCHIVAL_LIFECYCLE.md) - Storage lifecycle and retention
- [Architecture Overview](../../docs/architecture/README.md) - Community SKU architecture overview

## Development Status

**Phase 2 Implementation - Complete**

- ✅ Core interfaces and shared implementations
- ✅ Connection pooling with transaction safety
- ✅ Metrics collection and health diagnosis
- ✅ Retry logic and timeout handling
- ✅ MemGraph client integration - **Complete**
- ✅ CRUD operations implementation - **Complete**
- ✅ Transaction support (Bolt protocol) - **Complete**
- ✅ Vector similarity search - **Complete**
- ✅ Graph traversal queries - **Complete**
- ✅ Batch operations & Swarm Batch Metrics - **Complete**

## Operator Note: Batch Insertion & Swarm Metrics

To verify bulk write batching behavior under high concurrency/swarm load:
- The graph provider automatically collects batch metrics (`TotalBatches`, `TotalItems`, `TotalChunks`, `FallbackSingleCount`, `AvgBatchSize`, `AvgDuration`) via `GetGlobalGraphProviderMetricsCollector()`.
- Metrics are exposed in `MetricsSnapshot.Batch` and logged via metrics JSONL output.
- Chunking automatically merges consecutive `create_node` and `create_edge` operations into bulk `UNWIND` Cypher queries (up to 500 ops per chunk). If operations are non-chunkable, fallback single-operation execution increments `FallbackSingleCount`.

## Testing

**Note: Following TDD principles per project standards.** Tests should be written before implementation.

Test files should follow Go conventions:
- `*_test.go` files alongside implementation
- Test cases should be documented using the prototype CLI's test case objects
- Criteria objects should link tests to requirements for traceability

