# Context Package

**Status**: Active  
**Purpose**: Context management for the CLI with flexible processing modes

## Overview

This package provides context management for the CLI with support for multiple processing modes (sequential, hierarchical, hybrid). The context system follows a **Single Context Principle**: everything should be derivable from a single starting context.

## Quick Reference

- **Single Context Principle**: Functions receive one fully-processed context object
- **Derivation**: Use `Derive()` or convenience methods for variations
- **Processing Modes**: Sequential, Hierarchical, Hybrid
- **Precedence**: System → User → Project → Command (lowest to highest)

### Basic Usage

```go
// Get single processed context (already merged)
proc, err := cli.NewProcessor(cmd)
ctx := proc.Context()

// Derive variations as needed
jsonCtx := ctx.WithFormat("json")
verboseCtx := ctx.WithVerbose(true)
```

## Related Documentation

**Architecture Principles:**
- **Context Architecture v1.0**: Single Context Principle and command execution context isolation.
- **Context Patterns v1.0**: Context building patterns, processing modes, and profile resolution.

## Package Structure

```
internal/cli/context/
├── context.go          # Core context types and interfaces
├── builder.go          # Context builder for flexible assembly
├── processor.go        # Context processing logic
└── README.md          # This file (index only)
```

## Processing Modes

The context system supports three processing modes:

1. **Sequential**: Contexts processed in order (System → User → Project → Command)
2. **Hierarchical**: Tree structure with inheritance (parent → child)
3. **Hybrid**: Combination of sequential and hierarchical

See context processing mode specifications for detailed examples.

## Context Precedence

Default precedence (lowest to highest):
1. System Defaults
2. User Config (`~/.zqk/config/config.yaml`)
3. Project Config (`config/zqk.yaml`)
4. Command Flags

## Context Profiles

Pre-configured profiles:
- `ai-agent`: JSON output, verbose off
- `human`: Table output, verbose off
- `debug`: YAML output, verbose on
- `pagination`: Max page size 5 items
