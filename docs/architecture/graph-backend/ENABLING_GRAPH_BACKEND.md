# Enabling the Graph Backend

This guide explains how to enable and configure the graph backend for zqk.

## Prerequisites

1. **MemGraph** must be installed and running
2. **Connection details** (host, port, credentials if needed)

## Quick Start

### 1. Start MemGraph

#### Option A: Using Docker (Recommended)

```bash
docker run -it -p 7687:7687 -p 7444:7444 memgraph/memgraph
```

This starts MemGraph on:
- **Bolt port**: `7687` (for database connections)
- **HTTP port**: `7444` (for web interface)

#### Option B: Using Docker Compose

Create a `docker-compose.yml`:

```yaml
version: '3.8'
services:
  memgraph:
    image: memgraph/memgraph
    ports:
      - "7687:7687"
      - "7444:7444"
    volumes:
      - memgraph_data:/var/lib/memgraph
    environment:
      - MEMGRAPH_LOG_LEVEL=INFO

volumes:
  memgraph_data:
```

Then run:
```bash
docker-compose up -d
```

#### Option C: Native Installation

See [MemGraph Installation Guide](https://memgraph.com/docs/getting-started/installation)

### 2. Verify MemGraph is Running

```bash
# Check if port 7687 is listening
lsof -i :7687

# Or test connection with curl (if HTTP port is accessible)
curl http://localhost:7444/health
```

### 3. Enable Graph Backend in zqk

Set the environment variable:

```bash
export ZQK_GRAPH_ENABLED=true
```

Optional configuration (defaults shown):

```bash
export ZQK_GRAPH_HOST=localhost      # Default: localhost
export ZQK_GRAPH_PORT=7687           # Default: 7687
export ZQK_GRAPH_USERNAME=""         # Optional
export ZQK_GRAPH_PASSWORD=""         # Optional
export ZQK_GRAPH_DATABASE=""        # Optional
export ZQK_GRAPH_POOL_SIZE=10       # Default: 10
```

### 4. Verify Graph Backend is Enabled

```bash
# Run a test command
./zqk internal list --format json

# Or run performance tests
go test ./pkg/zqkcli -run TestPerformanceComparison -v
```

## Configuration Details

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ZQK_GRAPH_ENABLED` | `false` | Set to `true` to enable graph backend |
| `ZQK_GRAPH_HOST` | `localhost` | MemGraph server hostname |
| `ZQK_GRAPH_PORT` | `7687` | MemGraph Bolt port |
| `ZQK_GRAPH_USERNAME` | `""` | Database username (if required) |
| `ZQK_GRAPH_PASSWORD` | `""` | Database password (if required) |
| `ZQK_GRAPH_DATABASE` | `""` | Database name (if required) |
| `ZQK_GRAPH_POOL_SIZE` | `10` | Connection pool size |

### Connection Pool

The graph backend uses a connection pool to manage database connections efficiently:
- **Default pool size**: 10 connections
- **Automatic connection management**: Connections are acquired and released automatically
- **Retry logic**: Built-in retry with exponential backoff for transient failures

## Troubleshooting

### Connection Refused

**Error**: `dial tcp [::1]:7687: connect: connection refused`

**Solution**:
1. Verify MemGraph is running: `docker ps` or `lsof -i :7687`
2. Check firewall settings
3. Verify port number matches configuration

### Timeout Errors

**Error**: `TransactionExecutionLimit: timeout`

**Solution**:
1. Check MemGraph logs for issues
2. Verify network connectivity
3. Increase timeout settings if needed
4. Check if MemGraph is under heavy load

### Authentication Errors

**Error**: Authentication failed

**Solution**:
1. Verify `ZQK_GRAPH_USERNAME` and `ZQK_GRAPH_PASSWORD` are correct
2. Check MemGraph authentication settings
3. For local development, MemGraph may not require authentication

## Testing Graph Backend

### Run Performance Tests

```bash
export ZQK_GRAPH_ENABLED=true
go test ./pkg/zqkcli -run TestPerformanceComparison -v
```

This will test both file and graph backends and compare performance.

### Test Graph Output Formats

```bash
export ZQK_GRAPH_ENABLED=true
go test ./pkg/zqkcli -run TestGraphBackend -v
```

### Manual Testing

```bash
# Enable graph backend
export ZQK_GRAPH_ENABLED=true

# Create an object (will use graph backend)
./zqk object create backlog_item --file item.yaml

# List objects (will query from graph)
./zqk object list backlog_item --format json

# Get object (will read from graph)
./zqk object get BLI-001
```

## Switching Between Backends

The system automatically selects the backend based on `ZQK_GRAPH_ENABLED`:

- **File Backend** (default): `ZQK_GRAPH_ENABLED` not set or `false`
- **Graph Backend**: `ZQK_GRAPH_ENABLED=true` and MemGraph is running

If graph backend is enabled but MemGraph is unavailable, the system will:
1. Attempt to connect to MemGraph
2. If connection fails, fall back to file backend
3. Log a warning about the fallback

## Production Considerations

1. **Connection Pooling**: Adjust `ZQK_GRAPH_POOL_SIZE` based on expected load
2. **Security**: Use authentication in production (`ZQK_GRAPH_USERNAME`/`ZQK_GRAPH_PASSWORD`)
3. **Monitoring**: Monitor connection pool stats and query performance
4. **Backup**: Ensure MemGraph data is backed up regularly
5. **High Availability**: Consider MemGraph clustering for production

## Next Steps

- See [Graph Backend Architecture](../graph-backend/ARCHITECTURE_DECISIONS.md)
- Review [Performance Testing](../graph-backend/PERFORMANCE.md)
- Check [Migration Guide](../migration-strategy-file-to-graph-v1.0.md)

