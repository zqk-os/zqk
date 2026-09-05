# Service Management

**Last Verified:** 2026-08-31


## Overview

The zqk CLI includes service management capabilities to start, stop, and monitor external services required by the system (e.g., MemGraph for the graph backend).

## Commands

### List Services

```bash
zqk system service list (PRUNED)
```

Lists all available services that can be managed.

### Start Service

```bash
zqk system service start (PRUNED) <service>
```

Starts a service. Currently supports:
- `memgraph` - MemGraph graph database

### Stop Service

```bash
zqk system service stop (PRUNED) <service>
```

Stops a running service.

### Check Status

```bash
zqk system service status (PRUNED) <service>
```

Checks if a service is running.

## Authentication

Service management supports multiple authentication sources with the following precedence:

1. **Service-specific environment variables** (highest priority)
   - `ZQK_<SERVICE>_USERNAME` (e.g., `ZQK_MEMGRAPH_USERNAME`)
   - `ZQK_<SERVICE>_PASSWORD` (e.g., `ZQK_MEMGRAPH_PASSWORD`)

2. **Graph backend environment variables** (for MemGraph)
   - `ZQK_GRAPH_USERNAME`
   - `ZQK_GRAPH_PASSWORD`

3. **Service defaults** (if configured)
   - Some services may have default authentication configured

4. **No authentication** (default)
   - Most services (like MemGraph) don't require authentication by default

### Examples

```bash
# Start MemGraph with authentication from environment
export ZQK_MEMGRAPH_USERNAME=admin
export ZQK_MEMGRAPH_PASSWORD=secret
zqk system service start memgraph (PRUNED)

# Or use graph backend variables
export ZQK_GRAPH_USERNAME=admin
export ZQK_GRAPH_PASSWORD=secret
zqk system service start memgraph (PRUNED)

# Start without authentication (default)
zqk system service start memgraph (PRUNED)
```

## Security Integration

The service management system is designed to integrate with zqk security schemes:

- **Future**: Integration with security context for authenticated operations
- **Future**: Role-based access control (e.g., only admins can start/stop services)
- **Current**: Environment variable-based authentication
- **Current**: Default no-auth for development

## Service Configuration

Each service has a configuration that defines:
- Container/image name
- Port mappings
- Environment variables
- Volume mounts
- Authentication requirements

### MemGraph Configuration

- **Container name**: `zqk-memgraph`
- **Image**: `memgraph/memgraph`
- **Ports**:
  - `7687` (Bolt protocol)
  - `7444` (HTTP API)
- **Authentication**: Optional (via environment variables)

## Output Formats

All service commands support multiple output formats:

```bash
# Table format (default)
zqk system service status memgraph (PRUNED)

# JSON format
zqk system service status memgraph --format json (PRUNED)

# YAML format
zqk system service status memgraph --format yaml (PRUNED)
```

## Error Handling

The service management system provides clear error messages:

- **Docker not available**: Prompts user to start Docker
- **Service already running**: Informational message, no error
- **Service not running**: Informational message when stopping
- **Connection failures**: Detailed error messages with troubleshooting hints

## Future Enhancements

- Integration with security context for authenticated operations
- Support for additional services (Neo4j, PostgreSQL, etc.)
- Service health monitoring
- Automatic service restart on failure
- Service configuration files
- Service discovery
- Multi-environment support (dev, staging, prod)

