# Interactive Creation Architecture Analysis

**Version**: 1.0.0  
**Created**: 2026-01-08  
**Status**: Analysis  
**Purpose**: Analyze the architecture to identify the correct abstraction layer and resolve import cycles

## Problem Statement

We need to integrate MCP elicitation with streaming template loops, but we're hitting an import cycle:
- `pkg/mcp` (protocol layer) wants to use `cmd/zqk/system` (command layer)
- This creates a dependency from protocol → command, which is architecturally wrong

**Root Cause**: The import cycle detection tells us we haven't defined the correct abstraction layer.

## Current Dependency Analysis

### Current Structure

```
pkg/mcp (Protocol Layer)
  └─ Needs: Template generation, field validation, session management

cmd/zqk/system (Command Layer)  
  └─ Contains: StreamingTemplateGenerator, StreamingTemplateLoop, InteractiveSessionManager
  └─ Dependencies: pkg/objects, pkg/validation (shared packages)
```

### The Cross-Cutting Concern

Interactive object creation involves logic that spans multiple layers:

1. **Template Generation** (spec-driven, reusable)
   - Uses: `pkg/objects.FieldRegistry`, `pkg/objects.LifecycleLoader`, `pkg/objects.SpecLoader`
   - Not protocol-specific (could be used by CLI, MCP, or other interfaces)
   - Not command-specific (generic template generation logic)

2. **Field Validation** (spec-driven, reusable)
   - Uses: Field definitions from specs
   - Not protocol-specific
   - Not command-specific

3. **Session Management** (state tracking, reusable)
   - Backend-agnostic (in-memory, could be extended to persistent)
   - Not protocol-specific
   - Not command-specific

4. **Conversion Utilities** (data transformation, reusable)
   - FieldTokenInfo → ElicitationParam conversion
   - Protocol-aware but conversion logic is reusable

## Dependency Hierarchy Analysis

### What Dependencies Does Template Logic Need?

From `cmd/zqk/system/streaming_template.go`:
```go
import (
    "github.com/lanceman/zqk/pkg/objects"  // FieldRegistry, LifecycleLoader, SpecLoader
)
```

**Key Insight**: Template logic only depends on `pkg/objects` (shared package), not on command-specific logic.

### What Dependencies Does MCP Need?

MCP handlers need:
- Protocol types (`ElicitationError`, `ElicitationParam`) - in `pkg/mcp`
- Template generation logic - currently in `cmd/zqk/system` (WRONG)
- Conversion logic - currently in `cmd/zqk/system` (WRONG)

**Key Insight**: MCP shouldn't depend on command layer. It should depend on shared packages.

## Correct Architecture

### Shared/Reusable Logic

The template generation, field validation, and session management logic is **cross-cutting** and should live in a **shared package** that:
- Can be used by `pkg/mcp` (protocol layer)
- Can be used by `cmd/zqk/system` (command layer)
- Can be used by future interfaces (REST API, GraphQL, etc.)

### Proposed Package Structure

**Option 1: `pkg/interactive`** (Recommended)
- Focus: Interactive creation workflows
- Contains: Template generation, session management, conversion utilities
- Dependencies: `pkg/objects`, `pkg/validation` (shared packages only)
- No dependencies on: `pkg/mcp`, `cmd/zqk/system`

**Option 2: `pkg/templates`**
- Focus: Template generation and management
- More narrow scope (templates only, not full interactive workflows)

**Option 3: Extend `pkg/objects`**
- Would add interactive/template logic to objects package
- But this mixes concerns (object definitions vs. interactive creation)

### Dependency Flow (Corrected)

```
┌─────────────────────────────────────────┐
│  pkg/interactive (NEW - Shared Layer)   │
│  - Template generation                  │
│  - Field validation                     │
│  - Session management                   │
│  - Conversion utilities                 │
│  Dependencies: pkg/objects, pkg/validation │
└─────────────────────────────────────────┘
            ↑                    ↑
            │                    │
    ┌───────┴──────┐    ┌────────┴────────┐
    │              │    │                 │
┌───┴──────────┐  │    │  ┌──────────────┴──────┐
│ pkg/mcp      │  │    │  │ cmd/zqk/system     │
│ - Tool       │  │    │  │ - MCP integration    │
│   handlers   │  │    │  │ - CLI integration    │
│ - Protocol   │  │    │  │ - Command logic      │
│ Dependencies:│  │    │  │ Dependencies:        │
│ pkg/interactive│ │    │  │ pkg/interactive,     │
│ pkg/objects  │  │    │  │ pkg/mcp (if needed)  │
└──────────────┘  │    │  └──────────────────────┘
                  │    │
                  │    ┌──────────────────────┐
                  │    │ cmd/zqk/object     │
                  │    │ - Object creation    │
                  │    │ - Validation         │
                  └────┴──────────────────────┘
```

## Implementation Strategy

### Step 1: Create `pkg/interactive` Package

Move from `cmd/zqk/system` to `pkg/interactive`:
- `StreamingTemplateGenerator` → `pkg/interactive.TemplateGenerator`
- `StreamingTemplateLoop` → `pkg/interactive.TemplateLoop`
- `InteractiveSessionManager` → `pkg/interactive.SessionManager`
- Conversion utilities → `pkg/interactive.ConvertToElicitationParams`

### Step 2: Update Dependencies

- `pkg/interactive` depends on:
  - `pkg/objects` (FieldRegistry, SpecLoader, LifecycleLoader)
  - `pkg/validation` (field validation)
  - Standard library only

- `pkg/mcp` can depend on:
  - `pkg/interactive` (shared logic)
  - `pkg/objects` (if needed)
  - Standard library

- `cmd/zqk/system` can depend on:
  - `pkg/interactive` (shared logic)
  - `pkg/mcp` (protocol layer)
  - All command packages

This creates a clean dependency hierarchy with no cycles.

## Key Principles

1. **Protocol Layer Independence**: `pkg/mcp` should not depend on command-specific logic
2. **Shared Logic Extraction**: Cross-cutting concerns belong in shared packages
3. **Dependency Direction**: Command layer can depend on protocol layer, not vice versa
4. **Context-Aware Abstraction**: If contexts demand divergent behavior, create context-specific implementations that share a common interface

## Benefits

1. **No Import Cycles**: Clean dependency hierarchy
2. **Reusability**: Template logic can be used by multiple interfaces
3. **Testability**: Shared logic can be tested independently
4. **Maintainability**: Clear separation of concerns
5. **Extensibility**: Easy to add new interfaces (REST, GraphQL, etc.)

## Migration Path

1. Create `pkg/interactive` package structure
2. Move template generation logic from `cmd/zqk/system`
3. Move session management logic from `cmd/zqk/system`
4. Move conversion utilities from `cmd/zqk/system`
5. Update `cmd/zqk/system` to use `pkg/interactive`
6. Create MCP handlers in `pkg/mcp` that use `pkg/interactive`
7. Update tests to use new package structure

## Context-Specific Considerations

If different contexts (MCP, CLI, REST) require divergent behavior:
- Core logic stays in `pkg/interactive` (shared)
- Context-specific wrappers in their respective packages
- Use interfaces for context-specific behavior
- Example: MCP uses `ElicitationError`, CLI uses `fmt.Errorf`, but both use the same template generation logic
