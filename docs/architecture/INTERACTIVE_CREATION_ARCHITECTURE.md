# Interactive Object Creation Architecture

**Version**: 1.0.0  
**Created**: 2026-01-08  
**Status**: Design  
**Purpose**: Define the correct abstraction layers for interactive object creation to avoid import cycles

## Problem Statement

We need to integrate MCP elicitation with streaming template loops, but we're hitting an import cycle:
- `pkg/mcp` (protocol layer) wants to use `cmd/zqk/system` (command layer)
- This creates a dependency from protocol → command, which is architecturally wrong

## Architectural Analysis

### Current Layer Structure

```
┌─────────────────────────────────────┐
│  Protocol Layer (pkg/mcp)           │
│  - MCP protocol handling            │
│  - Tool registration                │
│  - Elicitation error types          │
│  - JSON-RPC communication           │
└─────────────────────────────────────┘
            ↑
            │ (should not depend on)
            │
┌─────────────────────────────────────┐
│  Application Layer (cmd/zqk/...)  │
│  - Business logic                   │
│  - Command implementations          │
│  - System operations                │
└─────────────────────────────────────┘
```

### The Cross-Cutting Concern

Interactive object creation involves:
1. **Template Generation** (spec-driven, reusable)
2. **Field Validation** (spec-driven, reusable)
3. **Session Management** (state tracking, reusable)
4. **Elicitation Logic** (protocol-aware but reusable)
5. **CLI Integration** (application-specific)

These are **cross-cutting concerns** that span multiple layers.

## Correct Architecture

### Shared/Reusable Logic

Interactive creation logic should live in a **shared package** (`pkg/interactive` or `pkg/templates`) because:
- Template generation is reusable (spec-driven, not protocol-specific)
- Field validation is reusable (spec-driven, not protocol-specific)
- Session management is reusable (state tracking, backend-agnostic)
- Conversion logic is reusable (data transformation, not protocol-specific)

### Package-Specific Logic

- **MCP Protocol** (`pkg/mcp`): Handles elicitation errors, tool registration, JSON-RPC
- **Command Layer** (`cmd/zqk/system`): Handles CLI integration, command execution
- **Application Logic** (`cmd/zqk/object`): Handles object creation, validation

### Proposed Structure

```
┌─────────────────────────────────────┐
│  pkg/interactive (NEW)              │
│  - Template generation              │
│  - Field validation                 │
│  - Session management               │
│  - Conversion utilities             │
│  - Core loop logic                  │
└─────────────────────────────────────┘
            ↑                    ↑
            │                    │
    ┌───────┴──────┐    ┌────────┴────────┐
    │              │    │                 │
┌───┴──────────┐  │    │  ┌──────────────┴──────┐
│ pkg/mcp      │  │    │  │ cmd/zqk/system     │
│ - Tool       │  │    │  │ - MCP integration    │
│   handlers   │  │    │  │ - CLI integration    │
│ - Protocol   │  │    │  │ - Command logic      │
└──────────────┘  │    │  └──────────────────────┘
                  │    │
                  │    ┌──────────────────────┐
                  │    │ cmd/zqk/object     │
                  │    │ - Object creation    │
                  │    │ - Validation         │
                  └────┴──────────────────────┘
```

## Migration Strategy

### Step 1: Create Shared Package

Move reusable logic to `pkg/interactive`:
- Template generation (`StreamingTemplateGenerator`)
- Template loop (`StreamingTemplateLoop`)
- Session management (`InteractiveSessionManager`)
- Conversion utilities (FieldTokenInfo → ElicitationParam)

### Step 2: Protocol Integration

MCP handlers in `pkg/mcp`:
- Use `pkg/interactive` for core logic
- Handle protocol-specific concerns (elicitation errors, tool registration)
- Bridge to CLI via existing CLI bridge

### Step 3: Command Integration

Command layer in `cmd/zqk/system`:
- Use `pkg/interactive` for core logic
- Handle CLI-specific concerns (command execution, file handling)
- Can also provide MCP handlers if needed (but should register them, not define them in pkg/mcp)

## Key Principles

1. **Protocol Layer Independence**: `pkg/mcp` should not depend on command-specific logic
2. **Shared Logic Extraction**: Cross-cutting concerns belong in shared packages
3. **Dependency Direction**: Command layer can depend on protocol layer, not vice versa
4. **Context-Aware Abstraction**: If contexts demand divergent behavior, create context-specific implementations that share a common interface

## Implementation Notes

- `pkg/interactive` will depend on:
  - `pkg/objects` (specs, field registry)
  - `pkg/validation` (field validation)
  - Standard library (no internal dependencies)

- `pkg/mcp` can depend on:
  - `pkg/interactive` (shared logic)
  - Standard library
  - Protocol-related packages

- `cmd/zqk/system` can depend on:
  - `pkg/interactive` (shared logic)
  - `pkg/mcp` (protocol layer)
  - All command-related packages

This creates a clean dependency hierarchy with no cycles.
