# AI Agent Communication Channels

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-12-30  
**Status**: Active  
**Purpose**: Document communication channels and interrupt mechanisms for AI agent coordination

## Overview

zqk supports multiple communication channels for AI agent coordination, enabling distributed collaboration while maintaining system integrity and enabling human oversight.

## Communication Channels

### 1. MCP (Model Context Protocol)

**Purpose**: Standardized protocol for AI assistants to interact with the knowledge kernel

**Implementation**: `pkg/mcp/`

**Features**:
- Graph traversal tools (`graph_traversal`, `resolve_references`, `state_aware_query`)
- CLI bridge (automatic exposure of CLI commands as MCP tools)
- Security context filtering (privilege-based access control)
- JSON-RPC protocol
- **Multi-agent orchestration**: Event subscription system and concurrent request handling enable multiple agents to share a single MCP server instance

**Usage**:
- AI assistants connect via MCP protocol
- Tools are automatically discovered from CLI command tree
- Security context determines available tools
- Multiple agents can coordinate via event subscriptions and operation tracking

**Documentation**: 
- `pkg/mcp/README.md`
- `docs/process/architecture/mcp-cli-bridge-v1.0.md`
- `docs/process/architecture/MCP_MULTI_AGENT_ORCHESTRATION.md` (Multi-agent coordination)

### 2. Event-Driven Architecture

**Purpose**: Decoupled agent communication via event bus

**Architecture**:
- Agents emit events to central bus
- Agents subscribe to relevant events
- Event-driven triggers for lifecycle and policy enforcement

**Event Types**:
- Object lifecycle events (creation, update, status transitions)
- Policy violation events
- System check events
- Metrics events

**Implementation**:
- Lifecycle hooks in storage layer (`pkg/storage/object_storage_file.go`)
- Scheduler job triggers (`pkg/scheduler/scheduler.go`)
- Event-driven evaluation system

**Benefits**:
- No tight coupling between agents
- Scalable (add agents without rewriting workflows)
- Natural integration points

### 3. Git-Based Communication

**Purpose**: Distributed state synchronization via git repository

**Architecture**:
- Kernel state committed to public repository
- Agents clone and pull regularly
- Conflict resolution via git merge
- Authority-based resolution (higher authority wins)

**Workflow**:
1. Agent clones repository
2. Agent makes local changes
3. Agent commits and pushes
4. Other agents pull and merge
5. Conflicts resolved via authority or manual intervention

**Documentation**: See `docs/process/architecture/distributed-kernel-architecture-v1.0.md`

### 4. Notification System

**Purpose**: User-facing notifications for job completion and system events

**Implementation**: `pkg/scheduler/notification_context.go`

**Channels**:
- **Terminal Notifications**: Color-coded terminal output with icons
- **Desktop Notifications**: Platform-native notifications (macOS, Linux, Windows)
- **Event Channel**: For integrations and programmatic access

**Features**:
- Priority-based filtering (critical, high, medium, low)
- Category filtering
- Suppression for specific jobs
- Acknowledgment tracking
- Notification history

**Documentation**: See `docs/process/architecture/scheduler-notifications-v1.0.md`

### 5. Lifecycle Reminders

**Purpose**: Proactive notifications for lifecycle state issues

**Implementation**: Integrated with lifecycle hooks and scheduler

**Triggers**:
- Lifecycle precondition violations
- Policy violations
- System check violations
- Metrics deviations

**Delivery**:
- CLI output
- MCP notifications
- Periodic reports
- Lifecycle reminder objects

## Interrupt Mechanisms

### 1. Context Cancellation

**Purpose**: Graceful cancellation of long-running operations

**Implementation**: Go `context.Context` with cancellation

**Usage**:
- Command execution timeouts (`pkg/cli/timeout_hook.go`)
- Scheduler job timeouts (`pkg/scheduler/scheduler.go`)
- Storage operations with context cancellation

**Example**:
```go
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
defer cancel()
// Operation respects context cancellation
```

### 2. Signal Handling

**Purpose**: OS-level interrupt signals (SIGINT, SIGTERM)

**Implementation**: `pkg/cli/timeout_hook.go`

**Features**:
- SIGINT handling for graceful shutdown
- Exit code 130 for interrupted commands
- Metrics tracking for interrupted commands

**Example**:
```go
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, os.Interrupt)
// Command can be interrupted via Ctrl+C
```

### 3. Scheduler Job Cancellation

**Purpose**: Cancel running scheduler jobs

**Implementation**: `pkg/scheduler/scheduler.go`

**Features**:
- Job execution context with cancellation
- Max runtime limits (`max_runtime_seconds`)
- Permission-based cancellation

**Usage**:
- Jobs automatically cancelled on timeout
- Can be cancelled via scheduler API (if implemented)
- Context cancellation propagates to job handlers

### 4. Policy Enforcement Interrupts

**Purpose**: Block operations that violate policies

**Implementation**: Pre-commit hooks, validation pipeline

**Features**:
- Pre-commit hooks block commits with violations
- Object validation pipeline blocks invalid operations
- System check can block operations (Tier 1 violations)

**Example**:
- Pre-commit hook blocks commit if system check fails
- Object update blocked if validation fails
- Policy violations generate lifecycle reminders

### 5. Human-in-the-Loom Protocol

**Purpose**: Human approval for high-stakes operations

**Architecture**: Governor pattern for critical actions

**Features**:
- Decision records for high-stakes actions
- Cryptographic approval required
- Immutable audit trail
- Deontic logic constraints (permissions, obligations, prohibitions)

**Documentation**: See strategic pivot documents on "Human-in-the-Loom" protocol

## Communication Patterns

### Request-Response (Synchronous)
- **MCP Tools**: Request-response via JSON-RPC
- **CLI Commands**: Direct command execution
- **Object Operations**: Create/Read/Update/Delete operations

### Event-Driven (Asynchronous)
- **Lifecycle Hooks**: Fire-and-forget event triggers
- **Scheduler Jobs**: Background job execution
- **Notifications**: Async notification delivery

### Git-Based (Distributed)
- **State Synchronization**: Pull/push workflow
- **Conflict Resolution**: Merge-based resolution
- **Authority-Based**: Higher authority wins in conflicts

## Best Practices

### For AI Agents

1. **Use CLI for Object Operations**: Always use `zqk object` commands, never edit files directly
2. **Respect Context Cancellation**: Check `ctx.Done()` in long-running operations
3. **Subscribe to Relevant Events**: Subscribe to events that affect your domain
4. **Pull Regularly**: Pull from git repository to stay synchronized
5. **Handle Conflicts Gracefully**: Use authority-based resolution or request human intervention

### For System Design

1. **Non-Blocking Operations**: Use async execution for lifecycle hooks
2. **Timeout Protection**: Set reasonable timeouts for all operations
3. **Graceful Degradation**: Handle failures without breaking system
4. **Observability**: Log all communication events for debugging
5. **Security Context**: Always validate permissions before operations

## Future Enhancements

1. **Explicit Interrupt API**: Direct API for interrupting agent operations
2. **Priority-Based Interrupts**: Different interrupt levels (soft, hard, emergency)
3. **Agent-to-Agent Messaging**: Direct messaging between agents
4. **Event Bus Implementation**: Centralized event bus for all events
5. **Conflict Resolution UI**: Interactive conflict resolution interface

## Stability for multi-agent and CLI message bus

The kernel is being stabilized so multi-agent workflows can rely on **predictable status/progress/outcome** and **no silent hangs**. All CLI-triggered operations emit to the same event path (Coordinator); that path is the **CLI message bus** agents can use to observe and coordinate. See **`docs/architecture/STABILITY_FOR_MULTI_AGENT_AND_CLI_MESSAGE_BUS.md`** for how this ties together with role/privileges and agent-to-agent communication.

## Related Documentation

- **Stability for multi-agent and CLI message bus:** `docs/architecture/STABILITY_FOR_MULTI_AGENT_AND_CLI_MESSAGE_BUS.md`
- MCP CLI Bridge Architecture v1.0
- MCP Multi-Agent Orchestration
- Scheduler Notification System v1.0
- Distributed Kernel Architecture v1.0
- Integrated Lifecycle and Policy Enforcement Architecture v1.0
- AI Agent Onboarding Guide

