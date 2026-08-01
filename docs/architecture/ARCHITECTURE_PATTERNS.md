# Architecture Patterns Library

**Version**: 1.0.0  
**Created**: 2025-01-02  
**Status**: Active  
**Purpose**: Central reference for established architecture patterns and anti-patterns

## Overview

This document serves as the authoritative source for architecture patterns in zqk. **Before implementing new functionality, AI agents MUST consult this document and follow established patterns.**

## Related Documents

- **[Design Patterns Library](./DESIGN_PATTERNS.md)**: Comprehensive catalog of design patterns (Handler, Middleware, Event Emitter, Context Objects, Builder, Strategy, Adapter) used for request/response and pub/sub interactions.
- **[Code Evaluation and Refactoring Policy](./CODE_EVALUATION_POLICY.md)**: Policy for evaluating code quality, identifying refactoring opportunities, and applying design patterns to improve maintainability, readability, efficiency, accuracy, and observability.

## Pattern Discovery Process

### Step 1: Check Existing Patterns

Before implementing new functionality, query for existing patterns:

```bash
# Query architecture documents
zqk object list doc_entry --filter group=architecture --filter category=cli

# Query for specific patterns
zqk object list doc_entry --filter group=architecture --filter "title~bridge"
```

### Step 2: Review Related Architecture Documents

Check the [Architecture README](../../docs/architecture/README.md) for relevant documents.

### Step 3: Follow Established Patterns

If a pattern exists, **use it**. Do not create new patterns without explicit approval.

## Established Patterns

### Pattern 1: CLI Bridge for MCP Exposure

**Problem**: Exposing functionality via MCP without duplicating code.

**Solution**: Create CLI commands, not separate MCP bridges.

**Pattern**:
1. Implement functionality as CLI commands
2. CLI bridge automatically exposes commands as MCP tools
3. No separate MCP bridge needed

**Example**: Metrics tools (`zqk reports pcs`, `zqk reports edd`, `zqk reports blockers`) are exposed via CLI bridge, not a separate metrics bridge.

**Documentation**: [MCP CLI Bridge Architecture v1.0](./mcp-cli-bridge-v1.0.md)

**Anti-Pattern**: Creating separate bridge packages (e.g., `pkg/mcp/bridge/metrics_bridge.go`) when CLI commands would suffice.

**Note**: `pkg/mcp/interactive_elicitation.go` is type conversion only (interactive.FieldTokenInfo → MCP ElicitationParam), not a tool bridge; interactive tools are exposed via CLI bridge.

### Pattern 2: Context-Driven Bootstrap

**Problem**: Initializing systems based on available context.

**Solution**: Bootstrap from context, not manual registration.

**Pattern**:
1. Check context for available resources
2. Automatically discover and register based on context
3. No manual registration required

**Example**: CLI bridge automatically discovers CLI commands and registers them as MCP tools based on security context.

**Documentation**: [MCP CLI Bridge Architecture v1.0](./mcp-cli-bridge-v1.0.md)

**Anti-Pattern**: Manual registration of tools that could be discovered automatically.

### Pattern 3: Single Context Principle

**Problem**: Functions needing multiple context objects.

**Solution**: Use a single merged context, derive variations as needed.

**Pattern**:
1. Load single merged context
2. Derive variations using `Derive()` or convenience methods
3. Never pass multiple context objects

**Example**: `ctx.WithFormat("json")` instead of passing separate format context.

**Documentation**: [Context Architecture](../../internal/cli/context/ARCHITECTURE.md)

**Anti-Pattern**: Functions with signatures like `func(fmtCtx, storageCtx, secCtx)`.

### Pattern 4: CLI as Normative Path

**Problem**: Direct file edits bypassing system integrity.

**Solution**: Always use CLI for object operations.

**Pattern**:
1. Use `zqk object create/update/delete` for all object operations
2. Never edit YAML files directly
3. CLI maintains integrity hashes, cache, audit trails

**Documentation**: [CLI Normative Path v1.0](./cli-normative-path-v1.0.md)

**Anti-Pattern**: Direct file edits requiring `--force` flags.

### Pattern 5: Storage Provider Abstraction

**Problem**: Code depending on specific storage backends.

**Solution**: Use `ObjectStorageProvider` interface, not concrete implementations.

**Pattern**:
1. Code depends on `storage.ObjectStorageProvider` interface
2. Storage factory selects backend (file or graph)
3. No direct imports of `FileObjectStorage` or `GraphStorage`

**Documentation**: [Object Storage Provider v1.0](./object-storage-provider-v1.0.md)

**Anti-Pattern**: Direct imports of `pkg/storage/object_storage_file.go`.

### Pattern 6: Cross-Process File Locking

**Problem**: Multiple processes modifying shared file-based resources causing race conditions and data corruption.

**Solution**: Use `FileLock` utility for all shared file-based resources.

**Pattern**:
1. Use `storage.FileLock` for any shared file-based resource
2. Acquire lock before read-modify-write operations
3. Use timeout to prevent indefinite blocking
4. Lock file naming: `{resource_file}.lock`

**Documentation**: [Cross-Process File Locking Pattern](./shared-resource-locking.md)

**Anti-Pattern**: Direct file writes without locking, causing race conditions.

### Pattern 7: Event-Driven Webhooks over Synchronous Polling

**Problem**: Long-running external API calls (e.g., media generation) block CLI processes and consume idle resources.

**Solution**: Use async job dispatches with webhook callbacks instead of blocking loops or polling.

**Pattern**:
1. Dispatch external jobs with a webhook callback URL pointing to the Tool Pod Mesh or MCP server.
2. CLI process exits immediately, returning a tracking `scheduler_job` or `media_job` reference.
3. The mesh listener waits for the webhook callback to update the job state to `COMPLETED`.
4. Subsequent operations trigger off the completion event rather than a blocking `time.Sleep` loop.

**Anti-Pattern**: Using `time.Sleep` loops inside `cmd/zqk` to wait 10+ minutes for an external provider.

## Anti-Patterns to Avoid

### Anti-Pattern 1: Duplicate Architecture

**Symptom**: Creating separate systems when existing patterns would work.

**Example**: Creating metrics bridge when CLI bridge already handles MCP exposure.

**Prevention**: Always check for existing patterns before implementing.

### Anti-Pattern 2: Import Cycles

**Symptom**: Circular dependencies between packages.

**Example**: `pkg/mcp` importing `pkg/metrics` which imports `pkg/storage` which imports `pkg/mcp`.

**Prevention**: Use interfaces, bridge patterns, or separate packages that can import both.

### Anti-Pattern 3: Manual Registration

**Symptom**: Manually registering tools/commands when automatic discovery exists.

**Example**: Calling `RegisterMetricsTools()` when CLI bridge auto-discovers commands.

**Prevention**: Use context-driven bootstrap patterns.

### Anti-Pattern 4: Direct File Edits

**Symptom**: Editing YAML files directly, requiring `--force` flags.

**Example**: Using `vim` to edit backlog items instead of `zqk object update`.

**Prevention**: Always use CLI for object operations.

## Pre-Implementation Checklist

Before implementing new functionality, verify:

- [ ] Checked existing architecture patterns
- [ ] Reviewed related architecture documents
- [ ] Confirmed no duplicate patterns exist
- [ ] Verified no import cycles will be created
- [ ] Ensured CLI commands will be used (if MCP exposure needed)
- [ ] Confirmed storage provider abstraction is used
- [ ] Verified single context principle is followed
- [ ] Checked that CLI will be used for object operations
- [ ] Verified file locking is used for shared resources (if applicable)

## Pattern Discovery Commands

```bash
# List all architecture documents
zqk object list doc_entry --filter group=architecture

# Search for specific patterns
zqk object list doc_entry --filter group=architecture --filter "title~bridge"
zqk object list doc_entry --filter group=architecture --filter "title~cli"
zqk object list doc_entry --filter group=architecture --filter "title~storage"

# Query architecture README
cat docs/architecture/README.md
```

## Related Documentation

- [MCP CLI Bridge Architecture v1.0](./mcp-cli-bridge-v1.0.md)
- [CLI Normative Path v1.0](./cli-normative-path-v1.0.md)
- [Context Architecture](../../internal/cli/context/ARCHITECTURE.md)
- [Object Storage Provider v1.0](./object-storage-provider-v1.0.md)
- [AI Agent Onboarding](../../onboarding/AI_AGENT_ONBOARDING.md)

---

*This document should be consulted before any new implementation. If a pattern doesn't exist, document it here before implementing.*

---

### Pattern 8: Decoupled Semantic Verification

**Problem**: AI image and video models often hallucinate consistency across generated frames and require purely objective QA. Subjective "black box" model evaluations can easily miss spatial or temporal anomalies.

**Solution**: Decouple the observation of an artifact from the evaluation of the artifact.
1. `DescribeImage` (Observation): Use a vision model to explicitly extract a literal, objective string representation of the artifact, strictly forbidding any hallucination or intent-assumption.
2. `SemanticCompare` (Evaluation): Generate dense vector embeddings for both the observed text description and the expected narrative prompt. Calculate the cosine similarity (dot product) to produce a pure, mathematical alignment score.

**Benefit**: This transforms a subjective model check into a deterministic, testable pipeline, preventing hallucinated continuity bugs from slipping through the QC pipeline.

### Pattern 9: Three-Channel Event-Driven Shockwave Propagation

**Problem**: Handling object transitions, cascading dependencies, and custom user integrations across a distributed graph-state kernel without causing high memory overhead or blocking foreground execution.

**Solution**: Structure the event bus / coordination channel into three distinct subscriber channels:
1. **Mandatory Channels** (Core/System): Executes system-critical logic (e.g. database referential integrity, core dependency status propagation like `backlog_item` -> `priority_plan`). These are essential and run immediately.
2. **Optional Channels** (Integrations): Handles auxiliary systems, tracing, and metrics collection (e.g. file lock loggers, CPU metrics aggregation). If these fail or run slowly, they do not disrupt the core lifecycle.
3. **User-Defined Channels** (Custom/Ad-hoc): Allows future user-defined rules and triggers. These can listen for completed aggregations of many object transitions (e.g. "when all requirements for milestone X are verified") to transition larger, complex processes based on simple semantic definitions.

**Benefits**:
- **On-Demand Loading**: Avoids verbose instructions being retained in memory. Details are loaded on-demand from storage only when triggered by transitions.
- **Low Overhead**: Emitted events propagate one level at a time in lightweight asynchronous pipelines, keeping status updates fast and non-blocking.

### Pattern 10: Event-Driven Neurons (Shockwave Topology)

**Problem**: Hardcoding agent components (e.g., routing, inference, map-reduce) to specific network mediums or synchronous, blocking execution flows causes massive bottlenecks in a distributed AI mesh.
 
**Solution**: Design all multi-agent interaction nodes as "Event-Driven Neurons". 
1. **Thread-Safe & Non-Blocking**: Components must be strictly asynchronous (goroutines, channels) and thread-safe (`sync.Map`, atomic bitmasks).
2. **Channel Agnostic**: Components must expose clean `interface{}` abstractions so they can be configured to operate on *any* medium (HTTP, MCP, standard out, memory pipes) purely via semantic vocabulary definitions (like the `shockwave_router` object spec).
3. **Common Efficiency**: Reuse common ZQK components (e.g., pipeline builder, `logging.Fluent`) rather than inventing bespoke communication pipelines for each new node type.

**Benefits**:
- Neurons can scale infinitely and fail gracefully without hanging the primary orchestrator.
- You can hot-swap the underlying network channel (e.g. from local IPC to distributed cloud webhooks) by just changing the semantic configuration, not the Go logic.
