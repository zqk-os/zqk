# Observer Agent Onboarding Guide (BLI-812)

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2026-02-20  
**Status**: Active  
**Related**: [Observer Agent Architecture v1.0](./observer-agent-architecture-v1.0.md), ROL-001 (Observer Agent Role)

## Purpose

This guide helps developers and AI agents onboard to the Observer Agent: run extraction, populate the knowledge kernel graph, and subscribe to observer events.

## Prerequisites

- **Role**: Observer Agent role is defined as **ROL-001** (`observer_agent`) with permissions: `read:*`, `read:code`, `read:ast`, `write:graph_node`, `write:graph_edge`, `read:graph_node`, `read:graph_edge`, `write:account`.
- **Go**: The observer’s AST parsing currently supports **Go** via `pkg/observer` (Go `go/ast`). Tree-sitter or other languages can be added later.
- **Graph (for populate)**: Set `ZQK_GRAPH_ENABLED=true` and have a running graph backend (e.g. MemGraph) when using `observer populate`.

## Commands

From the repository root:

| Command | Description |
|--------|-------------|
| `zqk observer extract --dir .` | Extract code entities (functions, types, methods) from Go files under `--dir`. Output is JSON (entities + errors). |
| `zqk observer populate --dir . [--extract-id RUN_ID]` | Extract entities, then create graph nodes (CodeEntity, SourceFile) and edges (CONTAINS, METHOD_OF) in the knowledge kernel. Requires graph backend. |

- **Default dir**: `.` (current directory).
- **extract-id**: Optional; defaults to current UTC timestamp for versioning the batch.

## Event Emission (BLI-813)

The observer emits lifecycle events that subscribers can use for logging or integration:

| Event | When |
|-------|------|
| `observer.extract_started` | Before walking the directory for extract. |
| `observer.extract_completed` | After extract succeeded (includes entity count and duration). |
| `observer.extract_failed` | After extract failed (includes error and duration). |
| `observer.populate_started` | Before graph population. |
| `observer.populate_completed` | After populate succeeded (includes nodes/edges created). |
| `observer.populate_failed` | After populate failed. |

In code, subscribe via `observer.DefaultEmitter.Subscribe(fn)`. Each event includes `Type`, `Dir`, `ExtractID`, `EntityCount`, `Error`, `Duration`, `Timestamp`, and optional `Payload`.

## Code Layout

- **`pkg/observer`**: Extractor interface, Go extractor, entity model, graph population (nodes/edges), event types and emitter.
- **`cmd/zqk/observer`**: CLI `observer` with subcommands `extract` and `populate`.

## Testing

- Unit tests: `go test ./pkg/observer/...`
- Integration: Run `zqk observer extract --dir ./pkg/observer` and assert JSON output contains entities.

## Next Steps

- **Automatic agent registration (BLI-857–BLI-860)**: Observer can subscribe to MCP connection events and create accounts for new agents; see planning docs for REQ-9007 and CRIT-9024–9027.
