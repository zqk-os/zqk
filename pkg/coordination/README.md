# Coordination Package - Event Coordination System (Spinal Cord)

## Overview

The `coordination` package provides the central event coordination system for the entire codebase. It serves as the "spinal cord" of the system, routing events from all operations to appropriate channels (logging, audit, metrics, operational) and enabling process coordination through event subscriptions.

## Architecture

### Core Concept

```
Operation/Task/Job
    │
    └─── EventCoordinator (Spinal Cord)
            │
            ├─── Logging Channel (EventLogger)
            ├─── Audit Channel (audit_event objects)
            ├─── Metrics Channel (MetricPipeline)
            └─── Operational Channel (Subscribers)
```

### Key Components

1. **EventCoordinator**: Central coordinator that routes events to all channels
2. **EventContext**: Rich context objects with operation metadata and routing decisions
3. **OperationalEventSubscriber**: Interface for subscribers that coordinate processes
4. **ChannelRouter**: Routes events to appropriate channels based on context

## Usage

### Basic Usage

```go
import "github.com/lanceman/zqk/pkg/coordination"

// Get the global coordinator
coordinator := coordination.GetCoordinator()

// Create event context
eventCtx := &coordination.EventContext{
    OperationID:   "snapshot_expand_12345",
    OperationType: "snapshot_expand",
    Status:        "complete",
    // ... event data
    EmitLogging:   true,
    EmitAudit:     true,
    EmitMetrics:   true,
    EmitOperational: true,
}

// Emit event (routes to all enabled channels)
coordinator.Emit(ctx, eventCtx)
```

### Subscribing to Operational Events

```go
subscriber := &MySubscriber{
    id: "my-subscriber",
}

coordinator.Subscribe(subscriber)
```

## Integration

The coordinator integrates with:
- `pkg/logging.EventLogger` - Structured logging
- `pkg/storage` (audit events) - Audit trail
- `pkg/metrics.MetricPipeline` - Metrics collection
- `pkg/mcp.EventEmitter` - Operational events for MCP clients
- `pkg/scheduler` - Process coordination

### Scheduler coordination kernel (PRI-220 / BLI-901)

The scheduler participates in the same coordination spine as the rest of the CLI:

1. **High-level execution lifecycle** — Each job run can use `coordination.NewCoordinatorOperationCallback` (`pkg/scheduler/job_execution.go`) so `OnStart` / completion hooks align with the global operation-callback pattern used elsewhere.
2. **Granular job events** — `emitJobExecutionEventViaCoordinator` (`pkg/scheduler/job_execution_coordination.go`) emits `scheduler_job_started`, `scheduler_job_completed`, and `scheduler_job_failed` through the storage audit router and structured logging fields, keeping audit/metrics/logging consistent with coordinator-shaped metadata.

Together these paths give a single, observable story for “what the scheduler did” without duplicating ad hoc logging at every handler.

## Documentation

For detailed documentation, see:
- [Coordinator Mandatory Migration](../../docs/architecture/coordinator-mandatory-migration-v1.0.md)
- Archived integration documents: [docs/archive/completed-integrations/](../../docs/archive/completed-integrations/)

## Design Principles

1. **Single Source of Truth**: One event emission point, multiple channels
2. **Context-Aware**: Events carry rich context for routing and coordination
3. **Non-Blocking**: All channel emissions are async and non-blocking
4. **Thread-Safe**: Concurrent access from multiple goroutines
5. **Extensible**: Easy to add new channels or subscribers
6. **Observable**: All events are observable programmatically

## Benefits

- **System-Wide Awareness**: Single coordination point for all events
- **Process Coordination**: Subscribers can sequence operations based on events
- **Consistency**: All observability layers use the same event source
- **Efficiency**: Shared event data, no duplication
- **Coordination**: Causal chains can be sequenced automatically
