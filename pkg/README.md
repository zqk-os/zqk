# Packages Directory

**Status**: Active  
**Last Updated**: AUTO-GENERATED - Do not edit manually

This directory contains Go packages for the ZQK project. Each package is a modular component with a defined boundary and contract.

**⚠️ This README is auto-generated. To update it, run:**
```bash
./scripts/open-core/generate-code-readme-index.sh pkg
```

## Packages Index

| Package | Import Path | Files | Tests | Subpackages | README | Description |
|---------|-------------|-------|-------|-------------|--------|-------------|
| [accumulator](./accumulator/) | `github.com/zqk-os/zqk/pkg/accumulator` | 4+2 | 2 | - | ❌ - | Aggregation and time-windowed rollups for execution metrics, status events, and telemetry. |
| [adapters](./adapters/) | `github.com/zqk-os/zqk/pkg/adapters` | 4+1 | 1 | antigravity, gemini, +6 more | ❌ - | The vendor-neutral factory for host/IDE message delivery. Kernel code depends on [MessageDelivery] and [Resolve] only. Concrete... |
| [agent](./agent/) | `github.com/zqk-os/zqk/pkg/agent` | 1+1 | 1 | - | ❌ - | Cryptographic utilities, key management, and security identity primitives for agents. |
| [agentclaim](./agentclaim/) | `github.com/zqk-os/zqk/pkg/agentclaim` | 6+8 | 8 | - | ❌ - | Agent work claiming, cadence check-ins, lease acquisition, and write gating. |
| [agentdelivery](./agentdelivery/) | `github.com/zqk-os/zqk/pkg/agentdelivery` | 8+4 | 4 | adapter | ❌ - | Host-neutral delivery of messages, steers, and task notifications to agent environments. |
| [agentfeed](./agentfeed/) | `github.com/zqk-os/zqk/pkg/agentfeed` | 20+19 | 19 | bridge, httpapi | ❌ - | Bidirectional agent message feed, event streaming, human-in-the-loop correspondence, and HTTP API. |
| [agentidle](./agentidle/) | `github.com/zqk-os/zqk/pkg/agentidle` | 2+3 | 3 | - | ❌ - | Agent idle state detection, persistence, sleep management, and reactive wakeup triggers. |
| [agentonboard](./agentonboard/) | `github.com/zqk-os/zqk/pkg/agentonboard` | 7+4 | 4 | - | ❌ - | Workspace-to-kernel synchronization and agent seating for first-contact initialization. |
| [agentorch](./agentorch/) | `github.com/zqk-os/zqk/pkg/agentorch` | 1+1 | 1 | - | ❌ - | Process orchestration and multi-agent coordination across execution lifecycles. |
| [agentpack](./agentpack/) | `github.com/zqk-os/zqk/pkg/agentpack` | 1+0 | 0 | - | ❌ - | Domain object pack loader and runtime registration for agent entities. |
| [agentprompt](./agentprompt/) | `github.com/zqk-os/zqk/pkg/agentprompt` | 11+11 | 11 | - | ❌ - | Prompt assembly, template rendering, and contextual prompt injection for agent sessions. |
| [agentrules](./agentrules/) | `github.com/zqk-os/zqk/pkg/agentrules` | 1+5 | 5 | - | ❌ - | Parsing, validation, and enforcement of agent behavioral rules, directives, and constraints. |
| [aliases](./aliases/) | `github.com/zqk-os/zqk/pkg/aliases` | 1+1 | 1 | - | ❌ - | Command and object alias resolution, shorthand mapping, and synonym expansion. |
| [ambience](./ambience/) | `github.com/zqk-os/zqk/pkg/ambience` | 4+4 | 4 | - | ❌ - | Background ambient intelligence, contextual awareness, and environment state tracking. |
| [ambient](./ambient/) | `github.com/zqk-os/zqk/pkg/ambient` | 17+15 | 15 | - | ❌ - | Filesystem watcher daemon, ambient change detection, and reactive event triggering. |
| [appledouble](./appledouble/) | `github.com/zqk-os/zqk/pkg/appledouble` | 1+1 | 1 | - | ❌ - | Sanitization and handling of AppleDouble and macOS resource fork metadata files. |
| [architecture](./architecture/) | `github.com/zqk-os/zqk/pkg/architecture` | 1+1 | 1 | - | ❌ - | Codebase architectural boundaries, layering rules, and static AST governance checks. |
| [audit](./audit/) | `github.com/zqk-os/zqk/pkg/audit` | 2+2 | 2 | - | ❌ - | High-volume audit log streaming, event persistence, and validation policies. |
| [authcred](./authcred/) | `github.com/zqk-os/zqk/pkg/authcred` | 16+14 | 14 | - | ❌ - | Secure credential storage, token management, and authentication provider integration. |
| [batchaf](./batchaf/) | `github.com/zqk-os/zqk/pkg/batchaf` | 1+2 | 2 | - | ❌ - | Batch aggregation and resolution for orphaned requirements, goals, and backlog items. |
| [brand](./brand/) | `github.com/zqk-os/zqk/pkg/brand` | 2+4 | 4 | substitution | ❌ - | Product branding, executable names, channel flags, and environment variable namespace configuration. |
| [bridge](./bridge/) | `github.com/zqk-os/zqk/pkg/bridge` | 3+3 | 3 | engine, impl | ❌ - | Bidirectional communication bridge connecting IDEs and external tools with the kernel. |
| [bufferpool](./bufferpool/) | `github.com/zqk-os/zqk/pkg/bufferpool` | 1+1 | 1 | - | ❌ - | Reusable memory buffer pools for high-throughput zero-allocation I/O operations. |
| [cef](./cef/) | `github.com/zqk-os/zqk/pkg/cef` | 2+3 | 3 | - | ❌ - | Critical Evidence Framework (CEF) evaluation, evidence lift tracking, and proof aggregation. |
| [circuitbreaker](./circuitbreaker/) | `github.com/zqk-os/zqk/pkg/circuitbreaker` | 5+4 | 4 | - | ❌ - | Fault tolerance circuit breaker pattern preventing cascade failures in distributed calls. |
| [cleanup](./cleanup/) | `github.com/zqk-os/zqk/pkg/cleanup` | 1+1 | 1 | - | ❌ - | Workspace cleanup, orphaned temp file purging, and transient artifact garbage collection. |
| [cli](./cli/) | `github.com/zqk-os/zqk/pkg/cli` | 50+33 | 33 | bldr_cli_cmd_v1, commands, +3 more | ✅ [README](./cli/README.md) | This package provides the command-line interface infrastructure for zqk, following the spec-driven builder pattern established... |
| [clihooks](./clihooks/) | `github.com/zqk-os/zqk/pkg/clihooks` | 2+2 | 2 | - | ❌ - | CLI execution hooks, pre/post-command intercepts, and telemetry emission. |
| [closureevidence](./closureevidence/) | `github.com/zqk-os/zqk/pkg/closureevidence` | 1+1 | 1 | - | ❌ - | Evidence collection and cryptographic verification for backlog item closure. |
| [community](./community/) | `github.com/zqk-os/zqk/pkg/community` | 16+47 | 47 | - | ❌ - | Open-source community distribution, telemetry anonymization, docs portal, and packaging. |
| [concurrency](./concurrency/) | `github.com/zqk-os/zqk/pkg/concurrency` | 11+11 | 11 | - | ✅ [README](./concurrency/README.md) | This package provides standardized concurrency primitives and patterns to ensure consistent, safe, and observable concurrent op... |
| [config](./config/) | `github.com/zqk-os/zqk/pkg/config` | 10+8 | 8 | - | ❌ - | Kernel configuration loading, YAML parsing, environment overrides, and schema validation. |
| [context](./context/) | `github.com/zqk-os/zqk/pkg/context` | 15+6 | 6 | - | ❌ - | Context management, security principal propagation, and request-scoped state. |
| [contextevents](./contextevents/) | `github.com/zqk-os/zqk/pkg/contextevents` | 3+1 | 1 | - | ❌ - | Event dispatching and subscription scoped to execution contexts. |
| [contractchange](./contractchange/) | `github.com/zqk-os/zqk/pkg/contractchange` | 2+2 | 2 | - | ❌ - | Tracking and auditing schema and contract mutations across kernel versions. |
| [convergence](./convergence/) | `github.com/zqk-os/zqk/pkg/convergence` | 3+3 | 3 | - | ❌ - | Target state convergence controllers, admission checks, and milestone progression. |
| [convergerollup](./convergerollup/) | `github.com/zqk-os/zqk/pkg/convergerollup` | 8+8 | 8 | - | ❌ - | Rollup reporting and outcome aggregation for convergence evaluation ticks. |
| [coordination](./coordination/) | `github.com/zqk-os/zqk/pkg/coordination` | 15+10 | 10 | - | ✅ [README](./coordination/README.md) | The coordination package provides the central event coordination system for the entire codebase. It serves as the "spinal cord"... |
| [crypto](./crypto/) | `github.com/zqk-os/zqk/pkg/crypto` | 1+2 | 2 | - | ❌ - | Ed25519 cryptographic signing, token validation, and artifact provenance verification. |
| [daemon](./daemon/) | `github.com/zqk-os/zqk/pkg/daemon` | 0+0 | 0 | overseer, singleton | ❌ - | Background daemon supervisor, process lifecycle management, and health monitoring. |
| [datacell](./datacell/) | `github.com/zqk-os/zqk/pkg/datacell` | 22+22 | 22 | - | ❌ - | Data cell coordinator membrane around stream storage and content-addressable storage nuclei. |
| [datacellregistry](./datacellregistry/) | `github.com/zqk-os/zqk/pkg/datacellregistry` | 2+2 | 2 | - | ❌ - | Registry and discovery catalog for active project data cells. |
| [decisionpack](./decisionpack/) | `github.com/zqk-os/zqk/pkg/decisionpack` | 1+0 | 0 | - | ❌ - | Decision domain pack registering decision object lifecycles, options, and rationale. |
| [diagnostics](./diagnostics/) | `github.com/zqk-os/zqk/pkg/diagnostics` | 9+8 | 8 | - | ❌ - | Runtime diagnostics, stack dumps, memory profiling, and system health checks. |
| [diskusage](./diskusage/) | `github.com/zqk-os/zqk/pkg/diskusage` | 2+2 | 2 | - | ❌ - | Storage footprint calculation, quota monitoring, and disk capacity reporting. |
| [dispatch](./dispatch/) | `github.com/zqk-os/zqk/pkg/dispatch` | 4+3 | 3 | - | ❌ - | Thread-safe execution dispatching, worker routing, and context propagation. |
| [displaypack](./displaypack/) | `github.com/zqk-os/zqk/pkg/displaypack` | 1+0 | 0 | - | ❌ - | Display and visual formatting pack for terminal rendering and UI output. |
| [dna](./dna/) | `github.com/zqk-os/zqk/pkg/dna` | 6+6 | 6 | pm | ❌ - | Dna component and domain abstractions for ZQK Core. |
| [docman](./docman/) | `github.com/zqk-os/zqk/pkg/docman` | 5+7 | 7 | - | ❌ - | Documentation discovery, frontmatter verification, and index management. |
| [domain](./domain/) | `github.com/zqk-os/zqk/pkg/domain` | 0+0 | 0 | organizational | ❌ - | Domain-driven design entity models, value objects, and organizational boundaries. |
| [drifthotspots](./drifthotspots/) | `github.com/zqk-os/zqk/pkg/drifthotspots` | 4+4 | 4 | - | ❌ - | Detection and heatmapping of code and specification drift across the codebase. |
| [economy](./economy/) | `github.com/zqk-os/zqk/pkg/economy` | 2+2 | 2 | - | ❌ - | Agent token economy, resource budgeting, compute metering, and credit tracking. |
| [entitlements](./entitlements/) | `github.com/zqk-os/zqk/pkg/entitlements` | 1+3 | 3 | - | ❌ - | Role-based feature access control, license entitlement gates, and tier verification. |
| [envelope](./envelope/) | `github.com/zqk-os/zqk/pkg/envelope` | 1+1 | 1 | - | ❌ - | Standardized message envelope format for inter-process and network communication. |
| [errfmt](./errfmt/) | `github.com/zqk-os/zqk/pkg/errfmt` | 1+3 | 3 | - | ❌ - | Structured error formatting, actionable error reporting, and troubleshooting suggestions. |
| [events](./events/) | `github.com/zqk-os/zqk/pkg/events` | 1+2 | 2 | - | ❌ - | Event bus, overlay event streams, and asynchronous event pub/sub routing. |
| [evolution](./evolution/) | `github.com/zqk-os/zqk/pkg/evolution` | 5+5 | 5 | - | ❌ - | Schema migration, data model version evolution, and compatibility adapters. |
| [evolutionpack](./evolutionpack/) | `github.com/zqk-os/zqk/pkg/evolutionpack` | 1+0 | 0 | - | ❌ - | Model evolution pack managing version upgrade chains and deprecation schedules. |
| [execwrap](./execwrap/) | `github.com/zqk-os/zqk/pkg/execwrap` | 1+1 | 1 | - | ❌ - | Context-aware, safe execution wrappers around operating system commands. |
| [featureflags](./featureflags/) | `github.com/zqk-os/zqk/pkg/featureflags` | 2+3 | 3 | - | ❌ - | Dynamic runtime feature toggles and progressive rollout switches. |
| [federation](./federation/) | `github.com/zqk-os/zqk/pkg/federation` | 5+2 | 2 | meshbroker | ❌ - | Cross-kernel mesh federation, remote project peering, and distributed sync. |
| [filter](./filter/) | `github.com/zqk-os/zqk/pkg/filter` | 2+2 | 2 | - | ❌ - | Lexical filter expressions, object predicate evaluation, and search query compilation. |
| [fitness](./fitness/) | `github.com/zqk-os/zqk/pkg/fitness` | 4+2 | 2 | - | ❌ - | Architecture fitness functions and automated compliance scoring. |
| [functional](./functional/) | `github.com/zqk-os/zqk/pkg/functional` | 3+1 | 1 | - | ✅ [README](./functional/README.md) | A fluent, functional-style Go API for monadic error handling, safe map operations, and telemetry-integrated execution with auto... |
| [gantt](./gantt/) | `github.com/zqk-os/zqk/pkg/gantt` | 2+3 | 3 | - | ❌ - | Gantt timeline generation, SVG visual rendering, and roadmap scheduling displays. |
| [git](./git/) | `github.com/zqk-os/zqk/pkg/git` | 7+8 | 8 | - | ❌ - | Git operations wrapper, branch inspection, worktree management, and status reporting. |
| [gitconstants](./gitconstants/) | `github.com/zqk-os/zqk/pkg/gitconstants` | 1+1 | 1 | - | ❌ - | Centralized Git command, subcommand, flag, and option constant definitions. |
| [gitevidence](./gitevidence/) | `github.com/zqk-os/zqk/pkg/gitevidence` | 1+1 | 1 | - | ❌ - | Git commit evidence verification, trunk tip freshness, and branch validation gates. |
| [goroutinelabels](./goroutinelabels/) | `github.com/zqk-os/zqk/pkg/goroutinelabels` | 5+4 | 4 | - | ✅ [README](./goroutinelabels/README.md) | The GoroutineBuilder provides a fluent API for creating goroutines with consistent patterns and best practices. It ensures all... |
| [gotestparse](./gotestparse/) | `github.com/zqk-os/zqk/pkg/gotestparse` | 1+2 | 2 | - | ❌ - | Streaming parser and event normalizer for go test terminal output and JSON test events. |
| [graph](./graph/) | `github.com/zqk-os/zqk/pkg/graph` | 4+4 | 4 | databook, memgraph, +4 more | ✅ [README](./graph/README.md) | This package implements the pluggable graph backend interface for the zqk knowledge kernel. |
| [grooming](./grooming/) | `github.com/zqk-os/zqk/pkg/grooming` | 0+2 | 2 | - | ❌ - | Automated backlog grooming, criteria validation, and orphaned object detection. |
| [handslapper](./handslapper/) | `github.com/zqk-os/zqk/pkg/handslapper` | 2+2 | 2 | - | ❌ - | Path containment enforcement, directory traversal prevention, and boundary guards. |
| [healthcheck](./healthcheck/) | `github.com/zqk-os/zqk/pkg/healthcheck` | 3+10 | 10 | monitors | ❌ - | Subsystem health monitors, liveness/readiness probes, and diagnostic suites. |
| [hive](./hive/) | `github.com/zqk-os/zqk/pkg/hive` | 1+1 | 1 | capability, inbox, media | ❌ - | Multi-agent hive primitives, shared memory structures, and collective decision making. |
| [hivemind](./hivemind/) | `github.com/zqk-os/zqk/pkg/hivemind` | 3+4 | 4 | bridge, indexer, +2 more | ❌ - | Distributed agent knowledge graph, activity logging, and shared context storage. |
| [hostload](./hostload/) | `github.com/zqk-os/zqk/pkg/hostload` | 8+9 | 9 | - | ❌ - | Host machine CPU, memory, and I/O load monitoring for dynamic worker scheduling. |
| [httpheaders](./httpheaders/) | `github.com/zqk-os/zqk/pkg/httpheaders` | 1+1 | 1 | - | ❌ - | HTTP header constants, canonical header names, and utility parsers. |
| [idebridge](./idebridge/) | `github.com/zqk-os/zqk/pkg/idebridge` | 3+3 | 3 | - | ❌ - | IDE communication protocol bridge, editor command integration, and live sync. |
| [idehooks](./idehooks/) | `github.com/zqk-os/zqk/pkg/idehooks` | 1+1 | 1 | - | ❌ - | IDE lifecycle event hooks, workspace notification triggers, and editor bindings. |
| [inbox](./inbox/) | `github.com/zqk-os/zqk/pkg/inbox` | 1+1 | 1 | - | ❌ - | Agent message inbox, asynchronous task queueing, and notification delivery. |
| [infrastructure](./infrastructure/) | `github.com/zqk-os/zqk/pkg/infrastructure` | 4+2 | 2 | crypto, hts | ❌ - | Core infrastructure components, hardware abstraction, and cryptographic storage. |
| [ingestion](./ingestion/) | `github.com/zqk-os/zqk/pkg/ingestion` | 0+0 | 0 | adapters, translation | ❌ - | Data ingestion pipeline, file format adapters, and document translation. |
| [integrity](./integrity/) | `github.com/zqk-os/zqk/pkg/integrity` | 4+3 | 3 | - | ❌ - | Kernel state integrity verification, hash validation, and tamper detection. |
| [interactionpolicy](./interactionpolicy/) | `github.com/zqk-os/zqk/pkg/interactionpolicy` | 8+6 | 6 | - | ❌ - | User-agent interaction constraints, confirmation prompts, and safe intervention rules. |
| [interactive](./interactive/) | `github.com/zqk-os/zqk/pkg/interactive` | 7+5 | 5 | - | ❌ - | Interactive terminal UI components, prompts, select menus, and survey wizards. |
| [interfacepack](./interfacepack/) | `github.com/zqk-os/zqk/pkg/interfacepack` | 1+0 | 0 | - | ❌ - | Interface contract and API boundary specifications pack. |
| [kernel](./kernel/) | `github.com/zqk-os/zqk/pkg/kernel` | 4+3 | 3 | intake, mutation, +2 more | ❌ - | Knowledge Kernel bootstrap sequence, runtime lifecycle, and core state coordination. |
| [kernelcas](./kernelcas/) | `github.com/zqk-os/zqk/pkg/kernelcas` | 5+4 | 4 | compose | ❌ - | Kernel Content-Addressable Storage (CAS) integration and object hash trees. |
| [kindnames](./kindnames/) | `github.com/zqk-os/zqk/pkg/kindnames` | 2+3 | 3 | - | ❌ - | Canonical naming constants and normalization rules for kernel object kinds. |
| [kindsynonyms](./kindsynonyms/) | `github.com/zqk-os/zqk/pkg/kindsynonyms` | 1+1 | 1 | - | ❌ - | Synonym mapping, alias resolution, and plural-to-singular kind translation. |
| [librarypack](./librarypack/) | `github.com/zqk-os/zqk/pkg/librarypack` | 1+0 | 0 | - | ❌ - | Reusable component library pack registering shared traits and behaviors. |
| [license](./license/) | `github.com/zqk-os/zqk/pkg/license` | 1+1 | 1 | - | ❌ - | Open-source license compliance, copyright header verification, and SPDX tracking. |
| [lifecycle](./lifecycle/) | `github.com/zqk-os/zqk/pkg/lifecycle` | 13+17 | 17 | - | ❌ - | Kernel object lifecycle state machine, phase transitions, and validation rules. |
| [llm](./llm/) | `github.com/zqk-os/zqk/pkg/llm` | 14+13 | 13 | semanticcache | ❌ - | LLM client abstractions, prompt completion handlers, and model provider routing. |
| [loader](./loader/) | `github.com/zqk-os/zqk/pkg/loader` | 3+4 | 4 | - | ✅ [README](./loader/README.md) | Abstract **component loader** pattern: shared state as atomics, callback on state change, configurable timeouts (default config... |
| [localci](./localci/) | `github.com/zqk-os/zqk/pkg/localci` | 4+2 | 2 | - | ❌ - | Studio-local CI checkout and demote in pure Go. Production must not shell out to scripts/local-ci-*.sh. p |
| [lockhealth](./lockhealth/) | `github.com/zqk-os/zqk/pkg/lockhealth` | 1+1 | 1 | - | ❌ - | Verified stale-lock cleanup primitives with fail-closed semantics, usable across subsystems (scheduler, file lock strategies, s... |
| [logging](./logging/) | `github.com/zqk-os/zqk/pkg/logging` | 21+14 | 14 | - | ✅ [README](./logging/README.md) | This package provides structured logging with context-aware routing, MCP protocol protection, and multi-destination support. |
| [maintenance](./maintenance/) | `github.com/zqk-os/zqk/pkg/maintenance` | 1+2 | 2 | - | ❌ - | Scheduled maintenance tasks, garbage collection, and database compaction. |
| [mcp](./mcp/) | `github.com/zqk-os/zqk/pkg/mcp` | 139+121 | 121 | ideadapter, mcp_helpers, testing | ✅ [README](./mcp/README.md) | This package provides a complete MCP server implementation that enables AI assistants and other MCP clients to interact with th... |
| [mesh](./mesh/) | `github.com/zqk-os/zqk/pkg/mesh` | 10+12 | 12 | multimodal, sync | ❌ - | Peer-to-peer agent mesh networking, ambient signal exchange, and distributed sync. |
| [metricpack](./metricpack/) | `github.com/zqk-os/zqk/pkg/metricpack` | 1+0 | 0 | - | ❌ - | System and agent metric definitions pack for performance tracking. |
| [metrics](./metrics/) | `github.com/zqk-os/zqk/pkg/metrics` | 25+17 | 17 | tsdb | ❌ - | Lock operation names for RunInLockWithLogger / RunInRLockWithLogger. CONSTANTS_AND_DRY_INVENTORY_PLAN Phase A. p |
| [metricsrecording](./metricsrecording/) | `github.com/zqk-os/zqk/pkg/metricsrecording` | 2+2 | 2 | - | ❌ - | Centralizes whether test runs should record metrics (storage counters, pipeline sampling, metric object creation). Production b... |
| [migration](./migration/) | `github.com/zqk-os/zqk/pkg/migration` | 10+7 | 7 | detector, exporter, +6 more | ✅ [README](./migration/README.md) | This package implements the file-based to graph backend migration tools as defined in the Migration Strategy v1.0. |
| [mutation](./mutation/) | `github.com/zqk-os/zqk/pkg/mutation` | 7+10 | 10 | - | ❌ - | Atomic kernel object mutation, version incrementing, and change event publishing. |
| [nildecode](./nildecode/) | `github.com/zqk-os/zqk/pkg/nildecode` | 1+1 | 1 | - | ❌ - | Generic helpers for type assertions with non-zero checks. Kept separate from pkg/pipeline so low-level packages (e.g. pkg/objec... |
| [objectcreate](./objectcreate/) | `github.com/zqk-os/zqk/pkg/objectcreate` | 0+1 | 1 | - | ❌ - | Knowledge Kernel object creation helpers and schema validation. |
| [objectget](./objectget/) | `github.com/zqk-os/zqk/pkg/objectget` | 8+4 | 4 | - | ❌ - | Object retrieval, lazy loading, dereferencing, and relationship expansion. |
| [objectidcache](./objectidcache/) | `github.com/zqk-os/zqk/pkg/objectidcache` | 11+5 | 5 | - | ❌ - | The ObjectIDCache implementation extracted from cmd/zqk/system. CLI audit, coordinator events, cache-item strategy, and project... |
| [objectrecord](./objectrecord/) | `github.com/zqk-os/zqk/pkg/objectrecord` | 1+2 | 2 | - | ❌ - | Objectrecord component and domain abstractions for ZQK Core. |
| [objects](./objects/) | `github.com/zqk-os/zqk/pkg/objects` | 88+94 | 94 | koi | ❌ - | Kernel object graph repository, schema registration, and persistence adapters. |
| [observability](./observability/) | `github.com/zqk-os/zqk/pkg/observability` | 1+2 | 2 | - | ❌ - | Telemetry, distributed tracing spans, and operational visibility. |
| [observer](./observer/) | `github.com/zqk-os/zqk/pkg/observer` | 9+8 | 8 | - | ❌ - | Lock operation names for RunInLockWithLogger / RunInRLockWithLogger. CONSTANTS_AND_DRY_INVENTORY_PLAN Phase A. p |
| [ontology](./ontology/) | `github.com/zqk-os/zqk/pkg/ontology` | 5+3 | 3 | - | ❌ - | Ontological relationship validation, semantic modeling, and hierarchy graphs. |
| [opencore](./opencore/) | `github.com/zqk-os/zqk/pkg/opencore` | 2+2 | 2 | - | ❌ - | Open core boundary enforcement, feature segregation, and open-source distribution. |
| [operational](./operational/) | `github.com/zqk-os/zqk/pkg/operational` | 6+4 | 4 | - | ❌ - | Operational congruence reporting: disk vs index vs internal counts, disparity detection, and hooks for metrics and alerts. p |
| [orchestration](./orchestration/) | `github.com/zqk-os/zqk/pkg/orchestration` | 14+9 | 9 | adversarial, intent, +4 more | ❌ - | Orchestration component and domain abstractions for ZQK Core. |
| [orgpack](./orgpack/) | `github.com/zqk-os/zqk/pkg/orgpack` | 1+0 | 0 | - | ❌ - | Organizational hierarchy and team structure pack. |
| [osmosis](./osmosis/) | `github.com/zqk-os/zqk/pkg/osmosis` | 0+0 | 0 | github | ❌ - | Bi-directional state osmosis between local workspace and Knowledge Kernel graph. |
| [osslaunch](./osslaunch/) | `github.com/zqk-os/zqk/pkg/osslaunch` | 0+1 | 1 | - | ❌ - | Open-source launch automation, release readiness gates, and pre-flight checks. |
| [outputtypes](./outputtypes/) | `github.com/zqk-os/zqk/pkg/outputtypes` | 1+1 | 1 | - | ❌ - | Standardized CLI output format types, table formatters, and serialization. |
| [packrecord](./packrecord/) | `github.com/zqk-os/zqk/pkg/packrecord` | 1+1 | 1 | - | ❌ - | Verifies an uploaded spec pack and records its specs so those kinds load as typed objects. Spec-only packs need no rebuild. p |
| [paths](./paths/) | `github.com/zqk-os/zqk/pkg/paths` | 21+26 | 26 | - | ❌ - | Canonical project directory paths, file location resolvers, and path safety. |
| [pipeline](./pipeline/) | `github.com/zqk-os/zqk/pkg/pipeline` | 24+19 | 19 | convergence, plugins, sentinel | ✅ [README](./pipeline/README.md) | This package provides a robust builder and runtime for the standardized data pipeline lifecycle, enabling structured, multi-sta... |
| [pipelinepack](./pipelinepack/) | `github.com/zqk-os/zqk/pkg/pipelinepack` | 1+0 | 0 | - | ❌ - | Pipeline and task workflow step pack. |
| [pm](./pm/) | `github.com/zqk-os/zqk/pkg/pm` | 2+3 | 3 | - | ❌ - | Program and project management models, milestones, and deliverables. |
| [policy](./policy/) | `github.com/zqk-os/zqk/pkg/policy` | 2+1 | 1 | - | ❌ - | Kernel policy definitions, enforcement hooks, and rule evaluation. |
| [policyinterrupt](./policyinterrupt/) | `github.com/zqk-os/zqk/pkg/policyinterrupt` | 1+1 | 1 | - | ❌ - | Emergency policy interrupts, execution halting, and safety interlocks. |
| [precommit](./precommit/) | `github.com/zqk-os/zqk/pkg/precommit` | 1+2 | 2 | - | ❌ - | Types and logic for the pre-commit hook that reads background-check results. Category files are written by scheduler jobs (lint... |
| [predicate](./predicate/) | `github.com/zqk-os/zqk/pkg/predicate` | 2+2 | 2 | - | ❌ - | Predicate component and domain abstractions for ZQK Core. |
| [primaryorch](./primaryorch/) | `github.com/zqk-os/zqk/pkg/primaryorch` | 3+4 | 4 | - | ❌ - | Primary agent orchestration engine, session coordination, and run tracking. |
| [process](./process/) | `github.com/zqk-os/zqk/pkg/process` | 2+2 | 2 | - | ❌ - | Operating system process management, PID tracking, and graceful signal handling. |
| [processhygiene](./processhygiene/) | `github.com/zqk-os/zqk/pkg/processhygiene` | 6+15 | 15 | - | ❌ - | Workspace hygiene auditing, dead-code detection, and repository cleanliness. |
| [processing](./processing/) | `github.com/zqk-os/zqk/pkg/processing` | 2+3 | 3 | - | ❌ - | Processing component and domain abstractions for ZQK Core. |
| [projecttemp](./projecttemp/) | `github.com/zqk-os/zqk/pkg/projecttemp` | 3+1 | 1 | - | ❌ - | Hosts isolated temp-project teardown helpers that must not import pkg/storage (storage imports pkg/validation and other consume... |
| [proxy](./proxy/) | `github.com/zqk-os/zqk/pkg/proxy` | 1+1 | 1 | - | ❌ - | MCP and network proxy service routing, authentication, and payload forwarding. |
| [qapack](./qapack/) | `github.com/zqk-os/zqk/pkg/qapack` | 1+0 | 0 | - | ❌ - | Quality assurance, test coverage, and verification pack. |
| [quality](./quality/) | `github.com/zqk-os/zqk/pkg/quality` | 12+17 | 17 | - | ❌ - | Quality component and domain abstractions for ZQK Core. |
| [quick](./quick/) | `github.com/zqk-os/zqk/pkg/quick` | 1+1 | 1 | - | ❌ - | Parsing and helpers for one-click creation of system objects from text or files. p |
| [relay](./relay/) | `github.com/zqk-os/zqk/pkg/relay` | 2+2 | 2 | - | ❌ - | Message relay, inter-process communication, and agent event forwarding. |
| [releasegate](./releasegate/) | `github.com/zqk-os/zqk/pkg/releasegate` | 2+3 | 3 | - | ❌ - | Verification gates and multi-platform compilation tests for release candidates. p |
| [releasepack](./releasepack/) | `github.com/zqk-os/zqk/pkg/releasepack` | 1+0 | 0 | - | ❌ - | Release gates, artifact generation, and deployment pack. |
| [reports](./reports/) | `github.com/zqk-os/zqk/pkg/reports` | 1+1 | 1 | - | ❌ - | Reports component and domain abstractions for ZQK Core. |
| [reqharness](./reqharness/) | `github.com/zqk-os/zqk/pkg/reqharness` | 1+1 | 1 | - | ❌ - | The test-requirements verification harness. It closes the loop between a requirement (kernel object shape: id, claim, test_crit... |
| [resourcehygiene](./resourcehygiene/) | `github.com/zqk-os/zqk/pkg/resourcehygiene` | 5+2 | 2 | - | ❌ - | Resourcehygiene component and domain abstractions for ZQK Core. |
| [rollback](./rollback/) | `github.com/zqk-os/zqk/pkg/rollback` | 6+3 | 3 | - | ❌ - | A reusable snapshot-and-rollback pattern for lifecycle transitions, maintenance tasks, and other operations. See docs/architect... |
| [rollup](./rollup/) | `github.com/zqk-os/zqk/pkg/rollup` | 3+2 | 2 | - | ❌ - | Rollup component and domain abstractions for ZQK Core. |
| [runtime](./runtime/) | `github.com/zqk-os/zqk/pkg/runtime` | 4+4 | 4 | - | ❌ - | Runtime environment detection, OS capabilities, and hardware specs. |
| [safepath](./safepath/) | `github.com/zqk-os/zqk/pkg/safepath` | 1+1 | 1 | - | ❌ - | Builds filesystem paths confined under a root directory to mitigate directory traversal when joining untrusted or external segm... |
| [scenario](./scenario/) | `github.com/zqk-os/zqk/pkg/scenario` | 6+7 | 7 | - | ❌ - | Convergence lifecycle end-to-end tests (storage-backed CVS updates). Coverage: measure → BuildSuggestedConvergenceSessionFields... |
| [scheduler](./scheduler/) | `github.com/zqk-os/zqk/pkg/scheduler` | 201+249 | 249 | clusterstatus, hostservice, transceiver | ❌ - | Distributed job scheduler, cron execution, maintenance tasks, and test scans. |
| [screencap](./screencap/) | `github.com/zqk-os/zqk/pkg/screencap` | 2+3 | 3 | - | ❌ - | Headless browser automation and visual screenshot capture engine. |
| [search](./search/) | `github.com/zqk-os/zqk/pkg/search` | 6+2 | 2 | - | ❌ - | Search component and domain abstractions for ZQK Core. |
| [seatworker](./seatworker/) | `github.com/zqk-os/zqk/pkg/seatworker` | 1+2 | 2 | - | ❌ - | Installs OS supervisor units for `agent seat-worker`. Production must not exec scripts/mesh/install-seat-workers.sh. p |
| [security](./security/) | `github.com/zqk-os/zqk/pkg/security` | 4+3 | 3 | secretpatterns | ❌ - | Security policies, credential isolation, and capability verification. |
| [semantic](./semantic/) | `github.com/zqk-os/zqk/pkg/semantic` | 5+7 | 7 | graph, translator | ❌ - | Semantic component and domain abstractions for ZQK Core. |
| [service](./service/) | `github.com/zqk-os/zqk/pkg/service` | 6+3 | 3 | - | ❌ - | Core service lifecycle, daemon startup, and background worker orchestration. |
| [shellcmd](./shellcmd/) | `github.com/zqk-os/zqk/pkg/shellcmd` | 1+1 | 1 | - | ❌ - | Turns a configured command string into an argv for execution. Scheduler callbacks and router actions are authored as shell one-... |
| [shockwave](./shockwave/) | `github.com/zqk-os/zqk/pkg/shockwave` | 4+7 | 7 | - | ❌ - | Event shockwave propagation, reactive state invalidation, and dependent notifications. |
| [shovelready](./shovelready/) | `github.com/zqk-os/zqk/pkg/shovelready` | 1+2 | 2 | - | ❌ - | Shovel-ready work discovery, execution scoring, and backlog ranking. |
| [skill](./skill/) | `github.com/zqk-os/zqk/pkg/skill` | 2+2 | 2 | - | ❌ - | Agent skill model definitions, capability declarations, and tool bindings. |
| [skills](./skills/) | `github.com/zqk-os/zqk/pkg/skills` | 0+0 | 0 | breeding, mutation | ❌ - | Skill loading, registry resolution, and execution sandboxing. |
| [specbuilder](./specbuilder/) | `github.com/zqk-os/zqk/pkg/specbuilder` | 3+3 | 3 | adapters, api_builders, +20 more | ✅ [README](./specbuilder/README.md) | This package provides the core infrastructure for the Spec-Driven Builder Pattern, a reusable pattern for generating artifacts... |
| [specialization](./specialization/) | `github.com/zqk-os/zqk/pkg/specialization` | 6+4 | 4 | - | ❌ - | Agent specialization roles, domain competence, and persona assignment. |
| [specorigination](./specorigination/) | `github.com/zqk-os/zqk/pkg/specorigination` | 6+5 | 5 | - | ❌ - | Stable stage names for spec origination pipelines. Wire these with pkg/pipeline.Builder.AddStage using pipeline kind PipelineKi... |
| [stampmemo](./stampmemo/) | `github.com/zqk-os/zqk/pkg/stampmemo` | 6+1 | 1 | - | ❌ - | A stamp-invalidated memo: the key is a stable identity, the stamp is the generation. There is no size limit, LRU, or TTL. Maps... |
| [stewardbase](./stewardbase/) | `github.com/zqk-os/zqk/pkg/stewardbase` | 1+1 | 1 | - | ❌ - | Base interfaces and common abstractions for stream storage stewards. |
| [storage](./storage/) | `github.com/zqk-os/zqk/pkg/storage` | 329+315 | 315 | audit, binary, +12 more | ✅ [README](./storage/README.md) | This package provides a unified storage abstraction layer for zqk, supporting both file-based and graph-based storage backends... |
| [storagetesting](./storagetesting/) | `github.com/zqk-os/zqk/pkg/storagetesting` | 1+1 | 1 | - | ❌ - | Holds the minimal interfaces and option structs shared by [github.com/zqk-os/zqk/pkg/testing.SetupCompleteTestEnvironment] and... |
| [strutil](./strutil/) | `github.com/zqk-os/zqk/pkg/strutil` | 2+2 | 2 | - | ❌ - | String manipulation, token extraction, and text formatting utilities. |
| [studio](./studio/) | `github.com/zqk-os/zqk/pkg/studio` | 3+2 | 2 | components | ❌ - | Visual Studio Web UI backend, asset serving, and interactive dashboard APIs. |
| [supervision](./supervision/) | `github.com/zqk-os/zqk/pkg/supervision` | 4+2 | 2 | - | ❌ - | Process supervision trees, worker restarts, and crash resilience. |
| [supply](./supply/) | `github.com/zqk-os/zqk/pkg/supply` | 1+5 | 5 | - | ❌ - | Dependency supply chain validation, vendor verification, and bill of materials. |
| [swarm](./swarm/) | `github.com/zqk-os/zqk/pkg/swarm` | 17+18 | 18 | metabolism, pack, remote | ❌ - | Multi-agent swarm coordination, task allocation, and consensus protocols. |
| [swarminit](./swarminit/) | `github.com/zqk-os/zqk/pkg/swarminit` | 6+7 | 7 | - | ❌ - | Runs configurable mesh bring-up recipes stored as kernel pipeline (PIP-*) objects. It is a mesh ops runner, not the kernel CAS... |
| [system](./system/) | `github.com/zqk-os/zqk/pkg/system` | 1+1 | 1 | - | ❌ - | System-level diagnostics, host environment inspection, and OS capabilities. |
| [systemcheck](./systemcheck/) | `github.com/zqk-os/zqk/pkg/systemcheck` | 5+3 | 3 | policy | ❌ - | Holds shared types and helpers for zqk system check / validation surfaces that used to live only in cmd/zqk/system (F-ARCH-001)... |
| [systemcheckwake](./systemcheckwake/) | `github.com/zqk-os/zqk/pkg/systemcheckwake` | 1+2 | 2 | - | ❌ - | Evaluates system-check summaries and optionally wakes a mesh seat. Opt-in only via `zqk system check --notify [agent-id]` — nev... |
| [systempeel](./systempeel/) | `github.com/zqk-os/zqk/pkg/systempeel` | 1+1 | 1 | - | ❌ - | Layered system abstraction peeling and kernel introspection tools. |
| [tde](./tde/) | `github.com/zqk-os/zqk/pkg/tde` | 5+4 | 4 | - | ❌ - | Tde component and domain abstractions for ZQK Core. |
| [tdval](./tdval/) | `github.com/zqk-os/zqk/pkg/tdval` | 1+1 | 1 | - | ❌ - | Test-driven validation engines, acceptance criteria gates, and verification suites. |
| [telemetry](./telemetry/) | `github.com/zqk-os/zqk/pkg/telemetry` | 10+9 | 9 | - | ✅ [README](./telemetry/README.md) | This package provides telemetry tracking, diagnostics hooks, and daemon synchronization capabilities to monitor Knowledge Kerne... |
| [testdiscovery](./testdiscovery/) | `github.com/zqk-os/zqk/pkg/testdiscovery` | 7+8 | 8 | - | ❌ - | Import ( "context" "os" "path/filepath" "strings" "testing" "time" "github.com/zqk-os/zqk/pkg/objects" "github.com/zqk-os/zqk/p... |
| [testenvroot](./testenvroot/) | `github.com/zqk-os/zqk/pkg/testenvroot` | 4+5 | 5 | - | ❌ - | A minimal test project layout (.zqk/process + test-settings) without importing pkg/testing (import-cycle hygiene for packages l... |
| [testing](./testing/) | `github.com/zqk-os/zqk/pkg/testing` | 11+1 | 1 | - | ✅ [README](./testing/README.md) | This package provides test configuration support to isolate test data from actual project data. |
| [testkit](./testkit/) | `github.com/zqk-os/zqk/pkg/testkit` | 18+14 | 14 | dummy_policy | ❌ - | Reusable test helpers intended for extraction into a shared Go testing library later. It composes storage/CAS/audit teardown us... |
| [testrunner](./testrunner/) | `github.com/zqk-os/zqk/pkg/testrunner` | 6+7 | 7 | - | ❌ - | Automated test runner execution, timeout management, and report generation. |
| [testservices](./testservices/) | `github.com/zqk-os/zqk/pkg/testservices` | 1+3 | 3 | - | ❌ - | Manages optional test-side services (e.g. MemGraph via Docker) without pulling in the full pkg/testing surface. p |
| [tpm](./tpm/) | `github.com/zqk-os/zqk/pkg/tpm` | 1+2 | 2 | - | ❌ - | Technical Program Management scheduling, Gantt tracking, and priority plans. |
| [tracing](./tracing/) | `github.com/zqk-os/zqk/pkg/tracing` | 1+1 | 1 | - | ❌ - | OpenTelemetry and distributed trace context propagation. |
| [translation](./translation/) | `github.com/zqk-os/zqk/pkg/translation` | 10+4 | 4 | - | ✅ [README](./translation/README.md) | Translates imported ontology/schema formats (RDF/OWL, JSON Schema, etc.) into zqk-domain structures for traceability and downst... |
| [transport](./transport/) | `github.com/zqk-os/zqk/pkg/transport` | 3+4 | 4 | - | ❌ - | Transport component and domain abstractions for ZQK Core. |
| [traversal](./traversal/) | `github.com/zqk-os/zqk/pkg/traversal` | 5+6 | 6 | - | ❌ - | Knowledge graph traversal algorithms, depth-bounded search, and cycle detection. |
| [tray](./tray/) | `github.com/zqk-os/zqk/pkg/tray` | 3+3 | 3 | - | ❌ - | Loads named shortcuts ("Tray") that expand to zqk argv lists. Default entries are embedded; merge with .zqk/tray.yaml (see Load... |
| [utils](./utils/) | `github.com/zqk-os/zqk/pkg/utils` | 0+0 | 0 | chunking, fileutil, +3 more | ❌ - | Generic cross-cutting utility functions and data structures. |
| [validation](./validation/) | `github.com/zqk-os/zqk/pkg/validation` | 67+98 | 98 | qa, scenario | ✅ [README](./validation/README.md) | This package provides object validation for zqk: instance validation (schema, lifecycle, semantic types), ID validation (prefix... |
| [vds](./vds/) | `github.com/zqk-os/zqk/pkg/vds` | 10+6 | 6 | - | ❌ - | Verifiable Decomposition Spine (VDS) verification, traceability, and done-gates. |
| [verification](./verification/) | `github.com/zqk-os/zqk/pkg/verification` | 1+2 | 2 | - | ❌ - | Formal verification of kernel constraints, schemas, and invariants. |
| [vet](./vet/) | `github.com/zqk-os/zqk/pkg/vet` | 13+5 | 5 | - | ❌ - | Code quality vetting, static analysis rules, and repository linting. |
| [vitality](./vitality/) | `github.com/zqk-os/zqk/pkg/vitality` | 1+1 | 1 | - | ❌ - | System and agent vitality metrics, heartbeat monitoring, and health scoring. |
| [vocabularypack](./vocabularypack/) | `github.com/zqk-os/zqk/pkg/vocabularypack` | 1+0 | 0 | - | ❌ - | Canonical domain terminology and glossary definitions pack. |
| [walutil](./walutil/) | `github.com/zqk-os/zqk/pkg/walutil` | 3+3 | 3 | - | ❌ - | Walutil component and domain abstractions for ZQK Core. |
| [when](./when/) | `github.com/zqk-os/zqk/pkg/when` | 5+4 | 4 | - | ❌ - | When component and domain abstractions for ZQK Core. |
| [workflow](./workflow/) | `github.com/zqk-os/zqk/pkg/workflow` | 2+1 | 1 | whatsnext | ❌ - | Workflow execution engine, what's next recommendation, and step evaluation. |
| [workflowpack](./workflowpack/) | `github.com/zqk-os/zqk/pkg/workflowpack` | 1+0 | 0 | - | ❌ - | Standard workflow step execution and sequence pack. |
| [workpack](./workpack/) | `github.com/zqk-os/zqk/pkg/workpack` | 3+2 | 2 | - | ❌ - | The included planning and verification pack. The composition root links it by default. A root that passes the build tag zqk_omi... |
| [wsobs](./wsobs/) | `github.com/zqk-os/zqk/pkg/wsobs` | 1+2 | 2 | - | ❌ - | Wsobs component and domain abstractions for ZQK Core. |
| [zqkcli](./zqkcli/) | `github.com/zqk-os/zqk/pkg/zqkcli` | 30+20 | 20 | - | ❌ - | Cobra command integration, flag binding, and CLI command dispatch. |
| [zqkdev](./zqkdev/) | `github.com/zqk-os/zqk/pkg/zqkdev` | 13+2 | 2 | - | ❌ - | Developer tooling, local test fixtures, and environment setup aids. |
| [zqkenv](./zqkenv/) | `github.com/zqk-os/zqk/pkg/zqkenv` | 9+11 | 11 | agentguard | ❌ - | Environment variable configuration, test mode detection, and runtime switches. |
| [zqksession](./zqksession/) | `github.com/zqk-os/zqk/pkg/zqksession` | 4+4 | 4 | - | ❌ - | Manages persisted ZQK session lifecycle state. p |
| [zqktime](./zqktime/) | `github.com/zqk-os/zqk/pkg/zqktime` | 1+3 | 3 | - | ❌ - | Deterministic UTC time helpers and RFC3339 compliance enforcement. |

## Package Structure

```
pkg/
├── accumulator/          # Aggregation and time-windowed rollups for execution metrics,
├── adapters/          # The vendor-neutral factory for host/IDE message delivery. Ke
│   └── antigravity/
│   └── gemini/
│   └── golang/
│   └── macos/
│   └── ollama/
│   └── openai/
│   └── qwen/
│   └── sync/
├── agent/          # Cryptographic utilities, key management, and security identi
├── agentclaim/          # Agent work claiming, cadence check-ins, lease acquisition, a
├── agentdelivery/          # Host-neutral delivery of messages, steers, and task notifica
│   └── adapter/
├── agentfeed/          # Bidirectional agent message feed, event streaming, human-in-
│   └── bridge/
│   └── httpapi/
├── agentidle/          # Agent idle state detection, persistence, sleep management, a
├── agentonboard/          # Workspace-to-kernel synchronization and agent seating for fi
├── agentorch/          # Process orchestration and multi-agent coordination across ex
├── agentpack/          # Domain object pack loader and runtime registration for agent
├── agentprompt/          # Prompt assembly, template rendering, and contextual prompt i
├── agentrules/          # Parsing, validation, and enforcement of agent behavioral rul
├── aliases/          # Command and object alias resolution, shorthand mapping, and 
├── ambience/          # Background ambient intelligence, contextual awareness, and e
├── ambient/          # Filesystem watcher daemon, ambient change detection, and rea
├── appledouble/          # Sanitization and handling of AppleDouble and macOS resource 
├── architecture/          # Codebase architectural boundaries, layering rules, and stati
├── audit/          # High-volume audit log streaming, event persistence, and vali
├── authcred/          # Secure credential storage, token management, and authenticat
├── batchaf/          # Batch aggregation and resolution for orphaned requirements, 
├── brand/          # Product branding, executable names, channel flags, and envir
│   └── substitution/
├── bridge/          # Bidirectional communication bridge connecting IDEs and exter
│   └── engine/
│   └── impl/
├── bufferpool/          # Reusable memory buffer pools for high-throughput zero-alloca
├── cef/          # Critical Evidence Framework (CEF) evaluation, evidence lift 
├── circuitbreaker/          # Fault tolerance circuit breaker pattern preventing cascade f
├── cleanup/          # Workspace cleanup, orphaned temp file purging, and transient
├── cli/          # This package provides the command-line interface infrastruct
│   └── bldr_cli_cmd_v1/
│   └── commands/
│   └── flagutil/
│   └── printer/
│   └── ux/
├── clihooks/          # CLI execution hooks, pre/post-command intercepts, and teleme
├── closureevidence/          # Evidence collection and cryptographic verification for backl
├── community/          # Open-source community distribution, telemetry anonymization,
├── concurrency/          # This package provides standardized concurrency primitives an
├── config/          # Kernel configuration loading, YAML parsing, environment over
├── context/          # Context management, security principal propagation, and requ
├── contextevents/          # Event dispatching and subscription scoped to execution conte
├── contractchange/          # Tracking and auditing schema and contract mutations across k
├── convergence/          # Target state convergence controllers, admission checks, and 
├── convergerollup/          # Rollup reporting and outcome aggregation for convergence eva
├── coordination/          # The coordination package provides the central event coordina
├── crypto/          # Ed25519 cryptographic signing, token validation, and artifac
├── daemon/          # Background daemon supervisor, process lifecycle management, 
│   └── overseer/
│   └── singleton/
├── datacell/          # Data cell coordinator membrane around stream storage and con
├── datacellregistry/          # Registry and discovery catalog for active project data cells
├── decisionpack/          # Decision domain pack registering decision object lifecycles,
├── diagnostics/          # Runtime diagnostics, stack dumps, memory profiling, and syst
├── diskusage/          # Storage footprint calculation, quota monitoring, and disk ca
├── dispatch/          # Thread-safe execution dispatching, worker routing, and conte
├── displaypack/          # Display and visual formatting pack for terminal rendering an
├── dna/          # Dna component and domain abstractions for ZQK Core.
│   └── pm/
├── docman/          # Documentation discovery, frontmatter verification, and index
├── domain/          # Domain-driven design entity models, value objects, and organ
│   └── organizational/
├── drifthotspots/          # Detection and heatmapping of code and specification drift ac
├── economy/          # Agent token economy, resource budgeting, compute metering, a
├── entitlements/          # Role-based feature access control, license entitlement gates
├── envelope/          # Standardized message envelope format for inter-process and n
├── errfmt/          # Structured error formatting, actionable error reporting, and
├── events/          # Event bus, overlay event streams, and asynchronous event pub
├── evolution/          # Schema migration, data model version evolution, and compatib
├── evolutionpack/          # Model evolution pack managing version upgrade chains and dep
├── execwrap/          # Context-aware, safe execution wrappers around operating syst
├── featureflags/          # Dynamic runtime feature toggles and progressive rollout swit
├── federation/          # Cross-kernel mesh federation, remote project peering, and di
│   └── meshbroker/
├── filter/          # Lexical filter expressions, object predicate evaluation, and
├── fitness/          # Architecture fitness functions and automated compliance scor
├── functional/          # A fluent, functional-style Go API for monadic error handling
├── gantt/          # Gantt timeline generation, SVG visual rendering, and roadmap
├── git/          # Git operations wrapper, branch inspection, worktree manageme
├── gitconstants/          # Centralized Git command, subcommand, flag, and option consta
├── gitevidence/          # Git commit evidence verification, trunk tip freshness, and b
├── goroutinelabels/          # The GoroutineBuilder provides a fluent API for creating goro
├── gotestparse/          # Streaming parser and event normalizer for go test terminal o
├── graph/          # This package implements the pluggable graph backend interfac
│   └── databook/
│   └── memgraph/
│   └── provider/
│   └── rpcpool/
│   └── schema/
│   └── sync/
├── grooming/          # Automated backlog grooming, criteria validation, and orphane
├── handslapper/          # Path containment enforcement, directory traversal prevention
├── healthcheck/          # Subsystem health monitors, liveness/readiness probes, and di
│   └── monitors/
├── hive/          # Multi-agent hive primitives, shared memory structures, and c
│   └── capability/
│   └── inbox/
│   └── media/
├── hivemind/          # Distributed agent knowledge graph, activity logging, and sha
│   └── bridge/
│   └── indexer/
│   └── providers/
│   └── sync/
├── hostload/          # Host machine CPU, memory, and I/O load monitoring for dynami
├── httpheaders/          # HTTP header constants, canonical header names, and utility p
├── idebridge/          # IDE communication protocol bridge, editor command integratio
├── idehooks/          # IDE lifecycle event hooks, workspace notification triggers, 
├── inbox/          # Agent message inbox, asynchronous task queueing, and notific
├── infrastructure/          # Core infrastructure components, hardware abstraction, and cr
│   └── crypto/
│   └── hts/
├── ingestion/          # Data ingestion pipeline, file format adapters, and document 
│   └── adapters/
│   └── translation/
├── integrity/          # Kernel state integrity verification, hash validation, and ta
├── interactionpolicy/          # User-agent interaction constraints, confirmation prompts, an
├── interactive/          # Interactive terminal UI components, prompts, select menus, a
├── interfacepack/          # Interface contract and API boundary specifications pack.
├── kernel/          # Knowledge Kernel bootstrap sequence, runtime lifecycle, and 
│   └── intake/
│   └── mutation/
│   └── steward/
│   └── verification/
├── kernelcas/          # Kernel Content-Addressable Storage (CAS) integration and obj
│   └── compose/
├── kindnames/          # Canonical naming constants and normalization rules for kerne
├── kindsynonyms/          # Synonym mapping, alias resolution, and plural-to-singular ki
├── librarypack/          # Reusable component library pack registering shared traits an
├── license/          # Open-source license compliance, copyright header verificatio
├── lifecycle/          # Kernel object lifecycle state machine, phase transitions, an
├── llm/          # LLM client abstractions, prompt completion handlers, and mod
│   └── semanticcache/
├── loader/          # Abstract **component loader** pattern: shared state as atomi
├── localci/          # Studio-local CI checkout and demote in pure Go. Production m
├── lockhealth/          # Verified stale-lock cleanup primitives with fail-closed sema
├── logging/          # This package provides structured logging with context-aware 
├── maintenance/          # Scheduled maintenance tasks, garbage collection, and databas
├── mcp/          # This package provides a complete MCP server implementation t
│   └── ideadapter/
│   └── mcp_helpers/
│   └── testing/
├── mesh/          # Peer-to-peer agent mesh networking, ambient signal exchange,
│   └── multimodal/
│   └── sync/
├── metricpack/          # System and agent metric definitions pack for performance tra
├── metrics/          # Lock operation names for RunInLockWithLogger / RunInRLockWit
│   └── tsdb/
├── metricsrecording/          # Centralizes whether test runs should record metrics (storage
├── migration/          # This package implements the file-based to graph backend migr
│   └── detector/
│   └── exporter/
│   └── parser/
│   └── reporter/
│   └── resolver/
│   └── scanner/
│   └── validator/
│   └── writer/
├── mutation/          # Atomic kernel object mutation, version incrementing, and cha
├── nildecode/          # Generic helpers for type assertions with non-zero checks. Ke
├── objectcreate/          # Knowledge Kernel object creation helpers and schema validati
├── objectget/          # Object retrieval, lazy loading, dereferencing, and relations
├── objectidcache/          # The ObjectIDCache implementation extracted from cmd/zqk/syst
├── objectrecord/          # Objectrecord component and domain abstractions for ZQK Core.
├── objects/          # Kernel object graph repository, schema registration, and per
│   └── koi/
├── observability/          # Telemetry, distributed tracing spans, and operational visibi
├── observer/          # Lock operation names for RunInLockWithLogger / RunInRLockWit
├── ontology/          # Ontological relationship validation, semantic modeling, and 
├── opencore/          # Open core boundary enforcement, feature segregation, and ope
├── operational/          # Operational congruence reporting: disk vs index vs internal 
├── orchestration/          # Orchestration component and domain abstractions for ZQK Core
│   └── adversarial/
│   └── intent/
│   └── monitor/
│   └── reporting/
│   └── service/
│   └── ticker/
├── orgpack/          # Organizational hierarchy and team structure pack.
├── osmosis/          # Bi-directional state osmosis between local workspace and Kno
│   └── github/
├── osslaunch/          # Open-source launch automation, release readiness gates, and 
├── outputtypes/          # Standardized CLI output format types, table formatters, and 
├── packrecord/          # Verifies an uploaded spec pack and records its specs so thos
├── paths/          # Canonical project directory paths, file location resolvers, 
├── pipeline/          # This package provides a robust builder and runtime for the s
│   └── convergence/
│   └── plugins/
│   └── sentinel/
├── pipelinepack/          # Pipeline and task workflow step pack.
├── pm/          # Program and project management models, milestones, and deliv
├── policy/          # Kernel policy definitions, enforcement hooks, and rule evalu
├── policyinterrupt/          # Emergency policy interrupts, execution halting, and safety i
├── precommit/          # Types and logic for the pre-commit hook that reads backgroun
├── predicate/          # Predicate component and domain abstractions for ZQK Core.
├── primaryorch/          # Primary agent orchestration engine, session coordination, an
├── process/          # Operating system process management, PID tracking, and grace
├── processhygiene/          # Workspace hygiene auditing, dead-code detection, and reposit
├── processing/          # Processing component and domain abstractions for ZQK Core.
├── projecttemp/          # Hosts isolated temp-project teardown helpers that must not i
├── proxy/          # MCP and network proxy service routing, authentication, and p
├── qapack/          # Quality assurance, test coverage, and verification pack.
├── quality/          # Quality component and domain abstractions for ZQK Core.
├── quick/          # Parsing and helpers for one-click creation of system objects
├── relay/          # Message relay, inter-process communication, and agent event 
├── releasegate/          # Verification gates and multi-platform compilation tests for 
├── releasepack/          # Release gates, artifact generation, and deployment pack.
├── reports/          # Reports component and domain abstractions for ZQK Core.
├── reqharness/          # The test-requirements verification harness. It closes the lo
├── resourcehygiene/          # Resourcehygiene component and domain abstractions for ZQK Co
├── rollback/          # A reusable snapshot-and-rollback pattern for lifecycle trans
├── rollup/          # Rollup component and domain abstractions for ZQK Core.
├── runtime/          # Runtime environment detection, OS capabilities, and hardware
├── safepath/          # Builds filesystem paths confined under a root directory to m
├── scenario/          # Convergence lifecycle end-to-end tests (storage-backed CVS u
├── scheduler/          # Distributed job scheduler, cron execution, maintenance tasks
│   └── clusterstatus/
│   └── hostservice/
│   └── transceiver/
├── screencap/          # Headless browser automation and visual screenshot capture en
├── search/          # Search component and domain abstractions for ZQK Core.
├── seatworker/          # Installs OS supervisor units for `agent seat-worker`. Produc
├── security/          # Security policies, credential isolation, and capability veri
│   └── secretpatterns/
├── semantic/          # Semantic component and domain abstractions for ZQK Core.
│   └── graph/
│   └── translator/
├── service/          # Core service lifecycle, daemon startup, and background worke
├── shellcmd/          # Turns a configured command string into an argv for execution
├── shockwave/          # Event shockwave propagation, reactive state invalidation, an
├── shovelready/          # Shovel-ready work discovery, execution scoring, and backlog 
├── skill/          # Agent skill model definitions, capability declarations, and 
├── skills/          # Skill loading, registry resolution, and execution sandboxing
│   └── breeding/
│   └── mutation/
├── specbuilder/          # This package provides the core infrastructure for the Spec-D
│   └── adapters/
│   └── api_builders/
│   └── bldr_config_v1/
│   └── bldr_instance_v1/
│   └── bldr_lifecycle_v1/
│   └── bldr_profile_v1/
│   └── bldr_routing_v1/
│   └── bldr_trait_v1/
│   └── bldr_v2/
│   └── bootstrap/
│   └── builders/
│   └── cli_builders/
│   └── config_builders/
│   └── core/
│   └── generators/
│   └── instance_builders/
│   └── lifecycle_builders/
│   └── profile_builders/
│   └── registry/
│   └── routing_builders/
│   └── trait_builders/
│   └── yaml/
├── specialization/          # Agent specialization roles, domain competence, and persona a
├── specorigination/          # Stable stage names for spec origination pipelines. Wire thes
├── stampmemo/          # A stamp-invalidated memo: the key is a stable identity, the 
├── stewardbase/          # Base interfaces and common abstractions for stream storage s
├── storage/          # This package provides a unified storage abstraction layer fo
│   └── audit/
│   └── binary/
│   └── cas/
│   └── core/
│   └── crud/
│   └── file/
│   └── filecas/
│   └── graph/
│   └── id_generation/
│   └── locknames/
│   └── migration/
│   └── signedurl/
│   └── systemcheck/
│   └── wal/
├── storagetesting/          # Holds the minimal interfaces and option structs shared by [g
├── strutil/          # String manipulation, token extraction, and text formatting u
├── studio/          # Visual Studio Web UI backend, asset serving, and interactive
│   └── components/
├── supervision/          # Process supervision trees, worker restarts, and crash resili
├── supply/          # Dependency supply chain validation, vendor verification, and
├── swarm/          # Multi-agent swarm coordination, task allocation, and consens
│   └── metabolism/
│   └── pack/
│   └── remote/
├── swarminit/          # Runs configurable mesh bring-up recipes stored as kernel pip
├── system/          # System-level diagnostics, host environment inspection, and O
├── systemcheck/          # Holds shared types and helpers for zqk system check / valida
│   └── policy/
├── systemcheckwake/          # Evaluates system-check summaries and optionally wakes a mesh
├── systempeel/          # Layered system abstraction peeling and kernel introspection 
├── tde/          # Tde component and domain abstractions for ZQK Core.
├── tdval/          # Test-driven validation engines, acceptance criteria gates, a
├── telemetry/          # This package provides telemetry tracking, diagnostics hooks,
├── testdiscovery/          # Import ( "context" "os" "path/filepath" "strings" "testing" 
├── testenvroot/          # A minimal test project layout (.zqk/process + test-settings)
├── testing/          # This package provides test configuration support to isolate 
├── testkit/          # Reusable test helpers intended for extraction into a shared 
│   └── dummy_policy/
├── testrunner/          # Automated test runner execution, timeout management, and rep
├── testservices/          # Manages optional test-side services (e.g. MemGraph via Docke
├── tpm/          # Technical Program Management scheduling, Gantt tracking, and
├── tracing/          # OpenTelemetry and distributed trace context propagation.
├── translation/          # Translates imported ontology/schema formats (RDF/OWL, JSON S
├── transport/          # Transport component and domain abstractions for ZQK Core.
├── traversal/          # Knowledge graph traversal algorithms, depth-bounded search, 
├── tray/          # Loads named shortcuts ("Tray") that expand to zqk argv lists
├── utils/          # Generic cross-cutting utility functions and data structures.
│   └── chunking/
│   └── fileutil/
│   └── mcp_harness/
│   └── sortutil/
│   └── syscallutil/
├── validation/          # This package provides object validation for zqk: instance va
│   └── qa/
│   └── scenario/
├── vds/          # Verifiable Decomposition Spine (VDS) verification, traceabil
├── verification/          # Formal verification of kernel constraints, schemas, and inva
├── vet/          # Code quality vetting, static analysis rules, and repository 
├── vitality/          # System and agent vitality metrics, heartbeat monitoring, and
├── vocabularypack/          # Canonical domain terminology and glossary definitions pack.
├── walutil/          # Walutil component and domain abstractions for ZQK Core.
├── when/          # When component and domain abstractions for ZQK Core.
├── workflow/          # Workflow execution engine, what's next recommendation, and s
│   └── whatsnext/
├── workflowpack/          # Standard workflow step execution and sequence pack.
├── workpack/          # The included planning and verification pack. The composition
├── wsobs/          # Wsobs component and domain abstractions for ZQK Core.
├── zqkcli/          # Cobra command integration, flag binding, and CLI command dis
├── zqkdev/          # Developer tooling, local test fixtures, and environment setu
├── zqkenv/          # Environment variable configuration, test mode detection, and
│   └── agentguard/
├── zqksession/          # Manages persisted ZQK session lifecycle state. p
├── zqktime/          # Deterministic UTC time helpers and RFC3339 compliance enforc
```

## Usage

Import packages using their canonical import path:

```go
import "github.com/zqk-os/zqk/pkg/agentfeed"
```

## Related Documentation

- [Project README](../README.md) - Project overview
- [Architecture Docs](../docs/architecture/) - System architecture
- [Verifiable Decomposition Spine](../docs/architecture/VERIFIABLE_DECOMPOSITION_SPINE.md) - Done-gates and contracts

---

*This README was auto-generated. Packages are discovered dynamically from the file system.*
