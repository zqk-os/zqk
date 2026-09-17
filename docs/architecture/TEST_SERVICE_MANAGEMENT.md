# Test Service Management

**Last Verified:** 2026-08-31


## Overview

The test service management system allows tests to automatically spin up required services (like MemGraph) before running tests and clean them up afterward.

## Usage

### Basic Usage

```go
func TestGraphBackendIntegration(t *testing.T) {
    // Skip if graph backend is not enabled
    if os.Getenv("ZQK_GRAPH_ENABLED") != "true" {
        t.Skip("Graph backend not enabled")
    }

    // Setup test environment
    tmpDir, _ := os.MkdirTemp("", "zqk-test-*")
    defer os.RemoveAll(tmpDir)
    
    testconfig.SetupTestEnvironment(tmpDir)
    os.Setenv("ZQK_TEST_ROOT", tmpDir)
    defer os.Unsetenv("ZQK_TEST_ROOT")

    // Setup test services (spins up MemGraph)
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()

    services, err := testconfig.SetupTestServices(ctx)
    if err != nil {
        t.Fatalf("failed to setup test services: %v", err)
    }
    defer services.Cleanup() // Cleanup services after test

    // Run your tests...
}
```

### Service Configuration

Services are configured via `ServiceConfig`:

```go
config := testconfig.ServiceConfig{
    Name:          "memgraph",
    ContainerName: "zqk-test-memgraph",
    Image:         "memgraph/memgraph",
    Ports: map[string]string{
        "7687": "7687", // Bolt port
        "7444": "7444", // HTTP port
    },
    HealthCheck: testconfig.HealthCheck{
        Type:     "port",
        Port:     "7687",
        Interval: 1 * time.Second,
        Timeout:  30 * time.Second,
    },
    StartTimeout: 30 * time.Second,
}
```

### Health Checks

The service manager supports multiple health check types:

1. **Port Check**: Checks if a port is listening
   ```go
   HealthCheck{
       Type: "port",
       Port: "7687",
   }
   ```

2. **HTTP Check**: Checks if an HTTP endpoint responds
   ```go
   HealthCheck{
       Type: "http",
       Port: "7444",
       Path: "/health",
   }
   ```

3. **Command Check**: Runs a command to check health
   ```go
   HealthCheck{
       Type:    "command",
       Command: []string{"docker", "exec", "container", "healthcheck"},
   }
   ```

### Pre-configured Services

Use `GetMemGraphConfig()` for MemGraph:

```go
config := testconfig.GetMemGraphConfig()
manager := testconfig.NewServiceManager()
err := manager.StartService(ctx, config)
```

### Manual Service Management

```go
manager := testconfig.NewServiceManager()

// Start a service
config := testconfig.GetMemGraphConfig()
err := manager.StartService(ctx, config)

// Check if running
running := manager.IsServiceRunning(config.ContainerName)

// Stop a service
err := manager.StopService(ctx, config.ContainerName)
```

## Environment Variables

- **`ZQK_GRAPH_ENABLED`**: Set to `true` to enable graph backend tests
- **`ZQK_TEST_VERBOSE`**: Set to `true` for verbose service management logs
- **`ZQK_TEST_ROOT`**: Test data directory (for test isolation)

## Features

1. **Automatic Service Startup**: Services are started before tests run
2. **Health Checks**: Waits for services to be healthy before proceeding
3. **Automatic Cleanup**: Services are stopped and removed after tests
4. **Docker Integration**: Uses Docker to manage service containers
5. **Timeout Handling**: Configurable timeouts for startup and health checks
6. **Verbose Logging**: Optional verbose mode for debugging

## Best Practices

1. **Always use `defer services.Cleanup()`** to ensure services are stopped
2. **Set appropriate timeouts** for service startup
3. **Use context with timeout** to prevent tests from hanging
4. **Check Docker availability** before running tests that require services
5. **Skip tests gracefully** if services are not available

## Example: Full Test Suite

```go
func TestGraphBackendFullSuite(t *testing.T) {
    if os.Getenv("ZQK_GRAPH_ENABLED") != "true" {
        t.Skip("Graph backend not enabled")
    }

    // Setup
    tmpDir := t.TempDir()
    testconfig.SetupTestEnvironment(tmpDir)
    os.Setenv("ZQK_TEST_ROOT", tmpDir)
    defer os.Unsetenv("ZQK_TEST_ROOT")

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()

    services, err := testconfig.SetupTestServices(ctx)
    if err != nil {
        t.Fatalf("failed to setup services: %v", err)
    }
    defer services.Cleanup()

    // Run test suite
    t.Run("Integration", TestGraphBackendIntegration)
    t.Run("Performance", TestGraphBackendPerformance)
}
```

## Troubleshooting

### Service Won't Start

- Check Docker is running: `docker info`
- Check ports are available: `lsof -i :7687`
- Increase timeout: `StartTimeout: 60 * time.Second`
- Enable verbose logging: `ZQK_TEST_VERBOSE=true`

### Health Check Fails

- Verify health check type matches service
- Check service logs: `docker logs zqk-test-memgraph`
- Increase health check timeout
- Try different health check type

### Services Not Cleaning Up

- Ensure `defer services.Cleanup()` is called
- Manually stop: `docker stop zqk-test-memgraph`
- Manually remove: `docker rm zqk-test-memgraph`

