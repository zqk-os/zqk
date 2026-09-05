# CLI Standardization and Dry-Run Context v1.0

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-12-25  
**Status**: Design  
**Related**: CLI Ontology v1.0, Context System

## Overview

The zqk CLI needs standardized command forms and a dry-run context system to improve usability and enable safe, impact-aware operations. Unlike the legacy CLI that supports multiple command permutations, we standardize on a single form while allowing specialized commands to be grouped differently.

## Design Principles

1. **Single Command Form**: One standard pattern for all CRUD operations
2. **Context-Aware**: Commands respect context profiles (ai-agent, dry-run, etc.)
3. **Impact Visibility**: Dry-run shows what would change without making changes
4. **Grouped Specializations**: Domain-specific commands are grouped but follow same pattern
5. **Persistence Abstraction**: Commands work with unified persistence layer

## Standardized Command Form

### Core Pattern

**Standard Form**: `zqk <verb> <kind> [arguments] [flags]`

Where:
- `<verb>`: Action (list, create, get, update, delete)
- `<kind>`: Object type (backlog-item, goal, milestone, etc.)
- `[arguments]`: Object-specific arguments (ID, filters, etc.)
- `[flags]`: Common flags (--format, --context, --dry-run, etc.)

### Examples

```bash
# List operations
zqk object list backlog-item [--filter ...]
zqk object list goal [--filter ...]
zqk object list milestone [--filter ...]

# Create operations
zqk create backlog-item [--title ...] [--description ...]
zqk create goal [--title ...] [--target ...]

# Get operations
zqk object get BLI-237
zqk object get GOAL-6369

# Update operations
zqk update BLI-237 [--field ...]
zqk update GOAL-6369 [--field ...]

# Delete operations
zqk delete BLI-237 [--force]
zqk delete GOAL-6369 [--archive]
```

### Grouped Specializations

Domain-specific commands are grouped under a namespace but follow the same pattern:

```bash
# Backlog-specific commands (grouped for convenience)
zqk backlog list [--filter ...]        # Same as: zqk object list backlog-item
zqk backlog create [--title ...]       # Same as: zqk create backlog-item
zqk backlog get BLI-237                # Same as: zqk object get BLI-237
zqk backlog update BLI-237 [--field]  # Same as: zqk update BLI-237

# Goal-specific commands
zqk goal list [--filter ...]           # Same as: zqk object list goal
zqk goal create [--title ...]         # Same as: zqk create goal
zqk goal get GOAL-6369                 # Same as: zqk object get GOAL-6369

# Milestone-specific commands
zqk milestone list [--filter ...]      # Same as: zqk object list milestone
zqk milestone create [--title ...]     # Same as: zqk create milestone
```

**Key Point**: Specialized commands are syntactic sugar - they map to the same underlying operations as the generic form.

## Context System

### Context Profiles

Context profiles provide pre-configured settings for different use cases:

```bash
# AI Agent context (JSON output, machine-readable)
zqk object list backlog-item --context ai-agent

# Human context (table output, human-friendly)
zqk object list backlog-item --context human

# Debug context (YAML output, verbose)
zqk object list backlog-item --context debug

# Dry-run context (no writes, show impacts)
zqk update BLI-237 --context dry-run --field status=complete
```

### Context Precedence

Context is applied in order of precedence:

1. **Command Flags** (highest precedence)
2. **Project Config** (`.zqk/config.yaml`)
3. **User Config** (`~/.zqk/config.yaml`)
4. **System Defaults** (lowest precedence)

### Context Properties

Each context profile sets:

| Property | ai-agent | human | debug | dry-run |
|----------|----------|-------|-------|---------|
| `format` | json | table | yaml | table/json |
| `verbose` | false | false | true | true |
| `quiet` | false | false | false | false |
| `write_enabled` | true | true | true | **false** |
| `show_impacts` | false | false | false | **true** |

## Dry-Run Context

### Purpose

Dry-run context short-circuits all write operations and shows what would change, enabling safe exploration and impact analysis.

### Behavior

When `--context dry-run` is specified:

1. **No Writes**: All write operations (create, update, delete) are skipped
2. **Impact Analysis**: Shows what would be created, updated, or deleted
3. **Validation**: Still validates inputs as if operation would proceed
4. **Dependencies**: Shows what other objects would be affected
5. **Output**: Uses appropriate format based on other context settings

### Examples

```bash
# Dry-run create
zqk create backlog-item \
  --title "New Feature" \
  --context dry-run

# Output:
# DRY-RUN: Would create backlog-item
#   ID: BLI-XXX (auto-generated)
#   Title: New Feature
#   Status: exploring
#   Would be saved to: docs/process/backlog/BLI-XXX.yaml
#   Would update: priority plan PRI-208 (if linked)

# Dry-run update
zqk update BLI-237 \
  --field status=complete \
  --context dry-run

# Output:
# DRY-RUN: Would update backlog-item BLI-237
#   Current: status=in_progress
#   Would change to: status=complete
#   Would trigger: milestone progress update (MIL-035)
#   Would update: priority plan progress (PRI-208)

# Dry-run delete
zqk delete BLI-999 \
  --context dry-run

# Output:
# DRY-RUN: Would delete backlog-item BLI-999
#   Object: BLI-999 - "Old Feature"
#   Would remove: 1 file (docs/process/backlog/BLI-999.yaml)
#   Would update: 2 objects that reference this item
#     - REQ-018: Remove BLI-999 from requirement_refs
#     - MIL-035: Remove BLI-999 from backlog_item_refs
```

### Impact Analysis

Dry-run shows comprehensive impact analysis:

```bash
zqk update BLI-237 --field status=complete --context dry-run

# Impact Analysis:
# Direct Changes:
#   - BLI-237: status (in_progress → complete)
#
# Cascade Effects:
#   - MIL-035: percent_complete (75% → 80%)
#   - PRI-208: progress metrics updated
#   - GOAL-6369: progress metrics updated
#
# Validation:
#   ✓ Status transition valid (in_progress → complete)
#   ✓ All required fields present
#   ✓ No breaking changes detected
#
# Files to Modify:
#   - docs/process/backlog/BLI-237.yaml
#   - docs/process/milestones/MIL-035.yaml
#   - docs/process/priority_plans/PRI-208.yaml
```

## Implementation Architecture

### Command Router

```go
type CommandRouter struct {
    persistence PersistenceProvider
    context     *Context
    validator   Validator
}

func (r *CommandRouter) Execute(verb, kind string, args []string, flags map[string]any) error {
    // Check if dry-run context
    if r.context.Profile == "dry-run" {
        return r.executeDryRun(verb, kind, args, flags)
    }
    
    // Normal execution
    return r.executeNormal(verb, kind, args, flags)
}

func (r *CommandRouter) executeDryRun(verb, kind string, args []string, flags map[string]any) error {
    // Validate inputs
    if err := r.validate(verb, kind, args, flags); err != nil {
        return err
    }
    
    // Analyze impacts
    impacts := r.analyzeImpacts(verb, kind, args, flags)
    
    // Show impacts (no writes)
    return r.showImpacts(impacts)
}
```

### Persistence Abstraction

```go
type PersistenceProvider interface {
    // Read operations (always allowed)
    List(kind string, filters Filters) ([]Object, error)
    Get(id string) (Object, error)
    
    // Write operations (disabled in dry-run)
    Create(obj Object) (string, error)
    Update(id string, updates Updates) error
    Delete(id string, options DeleteOptions) error
    
    // Impact analysis
    AnalyzeCreate(obj Object) (*Impact, error)
    AnalyzeUpdate(id string, updates Updates) (*Impact, error)
    AnalyzeDelete(id string) (*Impact, error)
}
```

### Context-Aware Output

```go
func (r *CommandRouter) formatOutput(data any) (string, error) {
    switch r.context.Format {
    case "json":
        return formatJSON(data)
    case "yaml":
        return formatYAML(data)
    case "table":
        return formatTable(data)
    case "markdown":
        return formatMarkdown(data)
    default:
        return formatTable(data)
    }
}

func (r *CommandRouter) showImpacts(impacts *Impact) error {
    output := map[string]any{
        "dry_run": true,
        "operation": impacts.Operation,
        "direct_changes": impacts.DirectChanges,
        "cascade_effects": impacts.CascadeEffects,
        "validation": impacts.Validation,
        "files_to_modify": impacts.FilesToModify,
    }
    
    return r.formatOutput(output)
}
```

## Command Mapping

### Generic to Specialized

```go
// Generic form
zqk object list backlog-item --status in_progress

// Maps to:
cmd := &ListCommand{
    Kind: "backlog-item",
    Filters: Filters{
        Status: "in_progress",
    },
}

// Specialized form
zqk backlog list --status in_progress

// Maps to same command:
cmd := &ListCommand{
    Kind: "backlog-item",  // Inferred from "backlog" namespace
    Filters: Filters{
        Status: "in_progress",
    },
}
```

### Namespace Resolution

```go
var namespaceMap = map[string]string{
    "backlog": "backlog-item",
    "goal": "goal",
    "milestone": "milestone",
    "workstream": "workstream",
    "priority-plan": "priority_plan",
    "requirement": "requirement",
    "criteria": "criteria",
}

func resolveKind(namespace, verb string) string {
    if kind, ok := namespaceMap[namespace]; ok {
        return kind
    }
    return namespace // Assume namespace is already a kind
}
```

## Benefits

1. **Consistency**: Single command form reduces cognitive load
2. **Discoverability**: Easy to guess command syntax
3. **Safety**: Dry-run enables safe exploration
4. **Impact Awareness**: See what changes before committing
5. **Context-Aware**: Output format adapts to use case
6. **Flexibility**: Specialized commands for convenience without breaking pattern

## Migration Path

### Legacy CLI Compatibility

The legacy CLI supports multiple forms:
- `list [kind] --filter`
- `[kind] list --filter`

Migration strategy:
1. Support both forms initially (with deprecation warning)
2. Standardize on `zqk <verb> <kind>` form
3. Map legacy forms to standard form internally
4. Remove legacy forms in future version

## Related Documentation

- [CLI Ontology v1.0](./cli-ontology-v1.0.md) - Command structure and semantics
- [Context System](../internal/cli/context/README.md) - Context management implementation
- [Persistence Gap Analysis](./spec-persistence-gap-analysis.md) - Unified persistence layer

