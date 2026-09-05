# Import Cycle Resolution: Architectural Evaluation Best Practices

**Version**: 1.0.0  
**Created**: 2026-01-08  
**Status**: Required Reading  
**Purpose**: Guide for resolving import cycles through architectural evaluation and abstraction layer identification

## Overview

Import cycles are not just compilation errors - they are symptoms of architectural issues. When you encounter an import cycle, it indicates that the dependency hierarchy is incorrect and that you haven't defined the proper abstraction layer. This document provides a systematic approach to resolving import cycles through architectural evaluation.

## Core Principle

> **Reusable logic should be shared; specific logic remains package-specific. Import cycle detection should inform you to look at your architecture to figure out what is driving the cross-cutting integration.**

## The Evaluation Process

### Step 1: Identify the Cycle

When you encounter an import cycle, document it clearly:

```
Package A imports Package B
Package B imports Package A
```

**Example:**
- `pkg/mcp` wants to import `cmd/zqk/system`
- `cmd/zqk/system` (via some path) imports `pkg/mcp`

### Step 2: Analyze Dependencies

For each package in the cycle, identify:

1. **What it needs from the other package**
2. **What dependencies the needed code has**
3. **Whether the needed code is protocol-specific, command-specific, or reusable**

**Dependency Analysis Template:**

```markdown
## Package A needs from Package B:
- Component X (what it needs)
  - Dependencies: [list dependencies]
  - Is it reusable? [yes/no - explain why]
  - Is it protocol-specific? [yes/no]
  - Is it command-specific? [yes/no]

## Package B needs from Package A:
- Component Y (what it needs)
  - Dependencies: [list dependencies]
  - Is it reusable? [yes/no - explain why]
  - Is it protocol-specific? [yes/no]
  - Is it command-specific? [yes/no]
```

**Example Analysis:**

```
## pkg/mcp needs from cmd/zqk/system:
- Template generation logic (StreamingTemplateGenerator)
  - Dependencies: pkg/objects (FieldRegistry, SpecLoader, LifecycleLoader)
  - Is it reusable? YES - spec-driven, not protocol-specific
  - Is it protocol-specific? NO
  - Is it command-specific? NO

## cmd/zqk/system needs from pkg/mcp:
- Nothing directly (no cycle on this side)
```

### Step 3: Identify Cross-Cutting Concerns

Look for logic that:
- Is used by multiple packages/layers
- Is not specific to a particular protocol, command, or interface
- Has minimal dependencies (ideally only shared packages)
- Could be used by future interfaces (REST API, GraphQL, etc.)

**Common Cross-Cutting Concerns:**
- Template generation (spec-driven, reusable)
- Field validation (spec-driven, reusable)
- Session management (state tracking, backend-agnostic)
- Data transformation/conversion (not protocol-specific)
- Core business logic (not interface-specific)

**Red Flags (should NOT be shared):**
- Protocol-specific types (e.g., JSON-RPC structures)
- Command-specific execution logic
- Interface-specific handlers
- Infrastructure-specific implementations

### Step 4: Determine the Correct Abstraction Layer

Based on your analysis, determine:

1. **What should be shared?**
   - Reusable logic that spans multiple layers
   - Logic that only depends on shared packages
   - Logic that is not protocol/command/interface-specific

2. **Where should it live?**
   - Create a new shared package if needed (e.g., `pkg/interactive`, `pkg/templates`)
   - Or move to an existing shared package if appropriate
   - Consider naming that reflects the cross-cutting nature

3. **What should remain package-specific?**
   - Protocol-specific handlers
   - Command-specific execution
   - Interface-specific adapters
   - Infrastructure-specific implementations

### Step 5: Design the Dependency Hierarchy

Create a dependency diagram showing:

```
┌─────────────────────────────────────┐
│  Shared Package (NEW or EXISTING)   │
│  - Reusable cross-cutting logic     │
│  Dependencies: shared packages only │
└─────────────────────────────────────┘
            ↑                    ↑
            │                    │
    ┌───────┴──────┐    ┌────────┴────────┐
    │              │    │                 │
┌───┴──────────┐  │    │  ┌──────────────┴──────┐
│ Package A    │  │    │  │ Package B            │
│ - Protocol   │  │    │  │ - Command logic      │
│   handlers   │  │    │  │ - Integration        │
│ Dependencies:│  │    │  │ Dependencies:        │
│ shared pkg   │  │    │  │ shared pkg, Package A│
└──────────────┘  │    │  └──────────────────────┘
```

**Key Rules:**
- Shared packages can depend on other shared packages
- Protocol/command packages can depend on shared packages
- Protocol/command packages should NOT depend on each other directly
- Lower layers can depend on higher layers, but not vice versa

### Step 6: Consider Context-Aware Abstractions

If different contexts (protocols, commands, interfaces) require divergent behavior:

1. **Core logic stays in shared package** (common behavior)
2. **Context-specific wrappers** in their respective packages
3. **Use interfaces** for context-specific behavior
4. **Dependency injection** for context-specific implementations

**Example:**
- Core template generation → `pkg/interactive` (shared)
- MCP elicitation wrapper → `pkg/mcp` (uses core, adds MCP-specific types)
- CLI integration wrapper → `cmd/zqk/system` (uses core, adds CLI-specific execution)

## Decision Framework

When evaluating whether logic should be shared:

| Question | If YES | If NO |
|----------|--------|-------|
| Is it used by multiple packages? | Consider sharing | Keep package-specific |
| Does it only depend on shared packages? | Good candidate for sharing | May need refactoring first |
| Is it protocol/command/interface-specific? | Keep package-specific | Consider sharing |
| Could future interfaces use it? | Strong candidate for sharing | May stay package-specific |
| Does it have minimal dependencies? | Good candidate for sharing | Evaluate dependency structure |
| Is it core business logic? | Likely should be shared | May be command-specific |

## Real-World Example: Interactive Object Creation

### Problem

Import cycle detected:
```
pkg/mcp → cmd/zqk/system (wants template generation)
cmd/zqk/system → pkg/mcp (via some path)
```

### Analysis

**What `pkg/mcp` needs:**
- Template generation (`StreamingTemplateGenerator`)
- Session management (`InteractiveSessionManager`)
- Conversion utilities

**Dependencies of needed code:**
- Only depends on `pkg/objects` (shared package)
- No dependencies on `pkg/mcp` or command-specific code
- Not protocol-specific
- Not command-specific

**Conclusion:** This is reusable, cross-cutting logic that should be shared.

### Solution

**Created `pkg/interactive`** (shared package):
- Contains template generation, session management, conversion utilities
- Depends only on `pkg/objects`, `pkg/validation` (shared packages)
- Used by both `pkg/mcp` and `cmd/zqk/system`

**Result:**
- No import cycles
- Clean dependency hierarchy
- Reusable across multiple interfaces (MCP, CLI, future REST/GraphQL)

## Key Principles

1. **Import cycles indicate architectural issues** - don't just work around them
2. **Reusable logic belongs in shared packages** - not in protocol/command-specific packages
3. **Analyze dependencies carefully** - understand what code needs and what it depends on
4. **Consider future use cases** - will other interfaces need this logic?
5. **Maintain clean dependency hierarchy** - lower layers can depend on higher layers, not vice versa
6. **Context-aware abstractions** - core logic shared, context-specific wrappers in their packages

## Checklist for Resolution

When resolving an import cycle:

- [ ] Document the cycle clearly (Package A → Package B → Package A)
- [ ] Analyze what each package needs from the other
- [ ] Identify dependencies of the needed code
- [ ] Determine if needed code is reusable or package-specific
- [ ] Identify cross-cutting concerns
- [ ] Design the correct abstraction layer
- [ ] Create/use shared package if needed
- [ ] Verify dependency hierarchy (no cycles, proper direction)
- [ ] Consider context-aware abstractions if behavior diverges
- [ ] Document the decision and rationale

## When to Create a New Shared Package

Create a new shared package (`pkg/<name>`) when:

1. **Clear cross-cutting concern** that doesn't fit existing packages
2. **Used by multiple layers** (protocol, command, interface)
3. **Minimal dependencies** (only shared packages)
4. **Reusable across contexts** (not protocol/command-specific)
5. **Significant enough** to warrant its own package (not just a single function)

**Examples:**
- `pkg/interactive` - Interactive creation workflows
- `pkg/templates` - Template generation (if not part of interactive)
- `pkg/workflows` - Workflow orchestration (if reusable)

## Anti-Patterns to Avoid

1. **Circular workarounds** - Using interfaces or dependency injection to mask cycles without fixing architecture
2. **Over-sharing** - Moving everything to shared packages (not everything needs to be shared)
3. **Under-sharing** - Keeping reusable logic in package-specific locations
4. **Protocol-specific shared code** - Sharing code that's actually protocol-specific
5. **Ignoring the cycle** - Working around it without understanding root cause

## Documentation Requirements

When resolving an import cycle, document:

1. **The cycle** - What packages were involved
2. **The analysis** - What each package needed and why
3. **The decision** - What was moved/shared and why
4. **The rationale** - Why this solution is architecturally correct
5. **The dependency hierarchy** - Diagram showing the corrected structure

## Integration with Onboarding

This process should be followed whenever:

- An import cycle is encountered
- New cross-cutting logic is being added
- Refactoring package structure
- Adding new interfaces/protocols
- Evaluating where new code should live

## Related Documents

- [EFFICIENT_DATA_PROCESSING.md](./EFFICIENT_DATA_PROCESSING.md) - Principles for efficient data processing
- [FIELD_STATE_TRACKING_PRINCIPLES.md](./FIELD_STATE_TRACKING_PRINCIPLES.md) - Principles for field state tracking
- [INTERACTIVE_CREATION_ARCHITECTURE.md](../process/architecture/INTERACTIVE_CREATION_ARCHITECTURE.md) - Example of this process in action

## Summary

Import cycles are architectural signals. When you encounter one:

1. **Stop and analyze** - Don't just work around it
2. **Identify cross-cutting concerns** - What logic spans multiple layers?
3. **Determine correct abstraction** - Where should reusable logic live?
4. **Design dependency hierarchy** - Ensure clean, acyclic dependencies
5. **Document the decision** - Future developers need to understand the rationale

By following this process, you'll create a cleaner, more maintainable architecture that properly separates concerns and enables code reuse across different interfaces and protocols.
