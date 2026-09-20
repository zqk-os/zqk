# Telemetry Package

This package provides telemetry tracking, diagnostics hooks, and daemon synchronization capabilities to monitor Knowledge Kernel execution, latency, and performance drift.

## Overview

- **Tracker**: Provides standard interfaces for recording executions, rate limits, caching, and IPC latency metrics (`telemetry.go`).
- **Hooks**: Supports lifecycle hooks for tool executions, prompt tokens, and agent events.
- **Drift Diagnostics**: Tracks Ghost Drift parameters and captures MTTR metrics (`drift.go`).
- **Daemon Synchronization**: Connects metrics with background daemon listeners (`daemon.go`).

## Usage

```go
import "github.com/zqk-os/zqk/pkg/telemetry"

// Initialize tracker
tracker := telemetry.NewTracker(logger)

// Record basic execution
tracker.RecordExecution(
	ctx,
	"my_tool",
	[]string{"arg1", "arg2"},
	50 * time.Millisecond,
	0, // exit code
	nil,
)

// Record latency
tracker.RecordIPCLatency(ctx, "read_socket", 2 * time.Millisecond)

// Track token usage
tracker.RecordAgentTokenUsage(ctx, "agent-1", 100, 250)
```

## Architecture

- **Structured Logging Backend**: The `DefaultTracker` uses ZQK's `logging.Fluent` to emit structured logs (`osmosis_execution`, `ipc_latency`, `agent_token_usage`) that can be parsed downstream.
- **Pluggability**: Designed around a core `Tracker` interface, enabling easy swapping of backends (e.g., in-memory recording for testing, or OpenTelemetry in production).

## Related

- [Project README](../../README.md) - Project overview
- [Architecture Docs](../../docs/architecture/) - System architecture
