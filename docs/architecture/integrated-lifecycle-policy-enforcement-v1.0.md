# Integrated Lifecycle and Policy Enforcement Architecture

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-12-30  
**Status**: Active  
**Related**: BLI-656

## Overview

This document describes the architecture for integrated lifecycle and policy enforcement that eliminates the need for external scripts and triggers. The system provides configurable aspects with intelligent defaults and strategies that allow users to quickly establish effective policies.

**Design exam for occupancy and hops:** [LIFECYCLE_STATE_MACHINE_RUBRIC.md](./LIFECYCLE_STATE_MACHINE_RUBRIC.md). Policies are the membrane backbone; object lifecycles are the occupancy machines those policies gate. Do not treat a thin policy status ladder as “no state work.”

## Problem Statement

Currently, lifecycle reminders and policy enforcement rely on external scripts, cron jobs, or manual triggers. This creates maintenance overhead, potential gaps in enforcement, and requires users to set up and maintain external infrastructure.

## Solution Architecture

### Core Principles

1. **Integrated Enforcement**: Lifecycle and policy enforcement built into the core system
2. **Event-Driven**: Triggered by natural system events (object updates, status changes, command execution)
3. **Non-Blocking**: All evaluations run asynchronously to avoid performance impact
4. **Configurable**: Intelligent defaults with full customization options
5. **Self-Contained**: No external scheduler dependencies

## Architecture Components

### 1. Lifecycle Hooks

**Location**: `pkg/storage/object_storage_file.go`

Lifecycle hooks are integrated into the storage layer and trigger automatically on object status changes:

```go
func executeLifecycleHook(ctx context.Context, kind, fromState, toState string, objectData map[string]interface{}) error
```

**Key Features**:
- Triggers on status transitions (detected in `Update` method)
- Async execution (non-blocking)
- Integrates with scheduler for job triggering
- Respects lifecycle context from operation context

**Integration Points**:
- Object creation/update operations
- Status transitions
- Command execution context

### 2. Scheduler System

**Location**: `pkg/scheduler/scheduler.go`

The scheduler provides background evaluation without external dependencies:

**Job Types**:
- **Timer-Based**: Periodic evaluation (e.g., daily system checks)
- **Lifecycle-Triggered**: Event-driven evaluation on status changes
- **Immediate**: One-time jobs (e.g., cache pre-warming on startup)

**Key Features**:
- Integrated scheduler (no external cron/systemd)
- Job definitions as objects (`scheduler_job` kind)
- Configurable schedules (cron format)
- Max runtime limits
- Enable/disable per job

**Example Jobs**:
- SCH-001: Cache Pre-Warming (Every 6 Hours)
- SCH-002: Lifecycle Check on Backlog Item Activation
- SCH-003: Daily System Check
- SCH-004: Daily Audit Event Aggregation
- SCH-005: Lifecycle Check on Backlog Item Completion
- SCH-006: Lifecycle Check on Milestone Completion
- SCH-007: Initial Cache Pre-Warming

### 3. Global Scheduler Registry

**Location**: `pkg/scheduler/scheduler.go`, `pkg/storage/object_storage_file.go`

The global scheduler registry allows the storage layer to trigger scheduler jobs without creating circular dependencies:

```go
// In scheduler package
var globalSchedulerRegistry *Scheduler
func GetGlobalScheduler() *Scheduler

// In storage package
var getGlobalSchedulerFunc func() interface{}
func SetGlobalSchedulerGetter(getter func() interface{})
```

**Benefits**:
- Decouples storage from scheduler
- Enables lifecycle hooks to trigger jobs
- No circular dependencies

### 4. Policy Enforcement Integration

**Integration Points**:

1. **Pre-Commit Hooks**: Existing mechanism (POL-CODE-004)
   - Enforces blocking policies before commits
   - Validates documentation registration (POL-DOC-001)

2. **Object Validation Pipeline**: Integrated into object operations
   - Validates policies during create/update
   - Respects `enforcement.automated` flag

3. **Command Execution Context**: After CLI operations
   - Provides immediate feedback
   - Non-blocking warnings

4. **Periodic Evaluation**: Via scheduler jobs
   - Comprehensive policy compliance checks
   - Catches violations that may have been missed

### 5. Configuration Strategy

**Primary**: Structured Object Configuration

Configuration is stored as objects in the system:
- **Scheduler Jobs**: `scheduler_job` objects define evaluation schedules
- **Policies**: `policy` objects define enforcement behavior
- **Templates**: `policy_template` objects define quick start strategies

**Intelligent Defaults**:
- Default evaluation intervals based on job type
- Default policy templates for common scenarios
- Sensible defaults for quick start

**Quick Start Strategies**:
- **Minimal**: Essential policies only (POL-CODE-002, POL-CODE-004)
- **Standard**: Balanced policies (code quality, documentation, workflow)
- **Strict**: Comprehensive policies (all categories)
- **Custom**: User-defined configuration

### 6. Evaluation Results Storage

**Multi-Tier Approach**:

1. **In-Memory Cache**: Fast access for recent results
   - Cached in scheduler/check command context
   - Evicted based on memory pressure

2. **Object Storage**: Persistent storage as `evaluation_result` objects
   - Stored in `.zqk/process/evaluations/` directory
   - Versioned and queryable via object system
   - Supports historical analysis

**Query Interface**:
- Via object system: `zqk object list evaluation_result --filter <criteria>`
- Graph backend enables advanced queries
- Filtering by object ID, policy ID, date range, status

## Event Flow

### Lifecycle Transition Flow

```
Object Update (status change)
  ↓
Storage Layer detects status change
  ↓
executeLifecycleHook() called
  ↓
Lifecycle context created/updated
  ↓
Scheduler triggered (async, non-blocking)
  ↓
Matching scheduler jobs executed
  ↓
Lifecycle checks / Policy evaluations run
  ↓
Results cached and persisted
```

### Periodic Evaluation Flow

```
Scheduler timer triggers
  ↓
Load enabled scheduler jobs
  ↓
Check schedule_expression (cron)
  ↓
Execute matching jobs
  ↓
Run evaluation handlers
  ↓
Cache and persist results
```

## Performance Characteristics

### Lifecycle Hooks
- **Overhead**: <1ms (async, non-blocking)
- **Execution**: Fire-and-forget goroutine
- **Impact**: No noticeable impact on normal operations

### Scheduler Jobs
- **Execution**: Background, configurable frequency
- **Max Runtime**: Configurable per job (`max_runtime_seconds`)
- **Resource Usage**: Minimal, jobs run opportunistically

### System Checks
- **With Cache Pre-Warming**: ~1-2s
- **Without Cache Pre-Warming**: ~3.5s
- **Cache Pre-Warming**: Reduces check time by ~50%

## Configuration Examples

### Minimal Strategy

```yaml
# Essential scheduler jobs only
- SCH-003: Daily System Check (enabled)
- SCH-001: Cache Pre-Warming (enabled)

# Essential policies only
- POL-CODE-002: Use CLI for Object Operations
- POL-CODE-004: System Check Violations Must Be Resolved
```

### Standard Strategy

```yaml
# Core scheduler jobs
- SCH-001: Cache Pre-Warming (enabled)
- SCH-002: Lifecycle Check on Activation (enabled)
- SCH-003: Daily System Check (enabled)
- SCH-004: Daily Audit Aggregation (enabled)

# Balanced policy set
- All code quality policies
- Documentation policies
- Workflow policies
```

### Strict Strategy

```yaml
# All recommended scheduler jobs enabled
- All SCH-* jobs enabled with frequent schedules

# Comprehensive policy set
- All policies across all categories
- Strict enforcement levels
- Frequent evaluation schedules
```

## Integration Points

### CLI Commands

- `zqk object update` - Triggers lifecycle hooks
- `zqk system check` - Periodic comprehensive evaluation
- `zqk policy init` - Quick start setup (future)
- `zqk policy list-templates` - Template management (future)

### Pre-Commit Hooks

- Documentation registration check (POL-DOC-001)
- System check validation (POL-CODE-004)
- Policy compliance checks

### MCP Integration

- Scheduler job management via MCP tools
- Policy querying and management
- Evaluation result queries

## Design Decisions

### QUE-002: Lifecycle Evaluation Trigger Strategy
**Decision**: Hybrid (Event-Driven + Periodic)
- Event-driven for immediate awareness
- Periodic for comprehensive coverage
- Scheduler provides background evaluation

### QUE-003: Policy Check Event Triggers
**Decision**: Combination with Policy-Specific Triggers
- Multiple trigger points with different policy subsets
- Blocking policies in pre-commit hooks
- Warning policies in command context
- Informational policies in periodic checks

### QUE-004: Background Evaluation Without External Schedulers
**Decision**: Integrated Scheduler with Event-Driven Triggers
- Scheduler runs as part of CLI/MCP operations
- No external cron/systemd required
- Jobs defined as objects in the system

### QUE-005: Configuration Format
**Decision**: Structured Object Configuration
- Configuration stored as objects (versioned, queryable)
- Intelligent defaults built into job creation
- Quick start strategies via templates

### QUE-006: Policy Template Structure
**Decision**: Predefined Policy Sets + Parameterized Templates
- Templates stored as `policy_template` objects
- Quick start strategies as templates
- CLI commands for template application

### QUE-007: Proactive Evaluation vs Performance Balance
**Decision**: Async Non-Blocking with Incremental Evaluation
- All evaluations run asynchronously
- Incremental evaluation (only changed objects)
- Multi-level caching strategy
- Priority-based evaluation

### QUE-008: Quick Policy Establishment UX
**Decision**: Quick Start Commands + Context-Aware Suggestions
- Single command setup: `zqk policy init`
- Template selection and application
- Context-aware policy suggestions
- Progressive disclosure (start minimal, add as needed)

### QUE-009: Evaluation Results Storage
**Decision**: Hybrid (Cache in Memory + Persist to Object Storage)
- In-memory cache for fast access
- Object storage for persistence and querying
- Queryable via object system and graph backend

## Future Enhancements

1. **Policy Template System**: Implement `policy_template` objects and CLI commands
2. **Context-Aware Suggestions**: Analyze project state and suggest policies
3. **Evaluation Result Objects**: Create `evaluation_result` object schema
4. **Metrics Integration**: Automatic metrics reporting on lifecycle events
5. **Advanced Querying**: Graph-based queries for evaluation results

## Related Documents

- BLI-656: Integrated Lifecycle and Policy Enforcement Architecture
- POL-CODE-005: Proactive System Check Monitoring and Awareness
- POL-TRACK-002: AI Metrics Reporting Frequency and Baseline Tracking
- System Check Monitoring architecture
- Scheduler Recommended Jobs documentation

