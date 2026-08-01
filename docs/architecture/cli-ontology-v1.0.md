# CLI Ontology Specification v1.0

**Version:** 1.0.0  
**Created:** 2025-12-24  
**Status:** Design Complete  
**Related:** BLI-243, BLI-237, MIL-035

## Overview

This document establishes a formal CLI ontology specification that enables both AI agents and humans to interpret CLI structure, commands, and outputs. The ontology is machine-readable and human-understandable, supporting automated agent workflows while remaining accessible for direct human use.

## Design Principles

1. **Machine-Readable**: CLI structure and semantics are formally defined and queryable
2. **Human-Friendly**: Commands are intuitive and follow common CLI conventions
3. **Progressive Disclosure**: Complex operations can be simplified for common use cases
4. **Consistency**: Commands follow consistent patterns and naming conventions
5. **Extensibility**: New commands can be added without breaking existing patterns

## Command Structure

### Standardized Command Form

**Core Pattern**: `zqk <verb> <kind> [arguments] [flags]`

Where:
- **`<verb>`**: Action (list, create, get, update, delete)
- **`<kind>`**: Object type (backlog-item, goal, milestone, workstream, etc.)
- **`[arguments]`**: Object-specific arguments (ID, filters, etc.)
- **`[flags]`**: Common flags (--format, --context, --dry-run, etc.)

### Command Hierarchy

```
zqk
├── generic commands (standardized CRUD operations)
│   ├── list <kind> [filters] [flags]
│   ├── create <kind> [flags]
│   ├── get <id> [flags]
│   ├── update <id> [fields] [flags]
│   └── delete <id> [flags]
│
├── domain commands (grouped specializations - map to generic form)
│   ├── backlog <verb> [arguments] [flags]
│   │   ├── backlog list [filters]      → list backlog-item
│   │   ├── backlog create [flags]      → create backlog-item
│   │   ├── backlog get <id>            → get <id>
│   │   ├── backlog update <id> [fields] → update <id>
│   │   └── backlog delete <id>         → delete <id>
│   │
│   ├── goal <verb> [arguments] [flags]
│   │   ├── goal list [filters]         → list goal
│   │   ├── goal create [flags]         → create goal
│   │   ├── goal get <id>               → get <id>
│   │   ├── goal update <id> [fields]    → update <id>
│   │   └── goal delete <id>             → delete <id>
│   │
│   ├── milestone <verb> [arguments] [flags]
│   │   ├── milestone list [filters]    → list milestone
│   │   ├── milestone create [flags]    → create milestone
│   │   ├── milestone get <id>          → get <id>
│   │   ├── milestone update <id> [fields] → update <id>
│   │   └── milestone delete <id>       → delete <id>
│   │
│   ├── workstream <verb> [arguments] [flags]
│   │   └── (same pattern as above)
│   │
│   └── priority-plan <verb> [arguments] [flags]
│       └── (same pattern as above)
│
├── system commands (system operations)
│   ├── init [flags]
│   ├── status
│   ├── check [object] [flags]
│   ├── validate [object] [flags]
│   └── sync
│
└── utility commands (helper operations)
    ├── help [command]
    ├── version
    └── docman [subcommand]
```

### Command Organization Principles

1. **Single Standard Form**: All CRUD operations follow `zqk <verb> <kind>` pattern
2. **Grouped Specializations**: Domain commands are syntactic sugar that map to generic form
3. **Namespace Resolution**: Domain namespaces (backlog, goal, etc.) map to object kinds (backlog-item, goal, etc.)
4. **Consistent Flags**: Common flags work across all commands (--format, --context, --dry-run)
5. **Context-Aware**: Commands respect context profiles (ai-agent, human, debug, dry-run)

### Command Pattern

**Generic Form**: `zqk <verb> <kind> [arguments] [flags]`

**Specialized Form**: `zqk <namespace> <verb> [arguments] [flags]`

**Mapping**: Specialized form maps to generic form via namespace-to-kind resolution

**Components**:
- **Verb**: Primary action (list, create, get, update, delete)
- **Kind/Namespace**: Object type (backlog-item) or domain namespace (backlog)
- **Arguments**: Required parameters (object ID, filters, etc.)
- **Flags**: Optional parameters (--format, --context, --dry-run, etc.)

## Object Commands

### Create Command

**Pattern**: `zqk create <kind> [flags]`

**Semantics**:
- Creates a new object of the specified kind
- Object data provided via flags or stdin
- Returns created object ID

**Flags**:
- `--title <string>`: Object title
- `--description <string>`: Object description
- `--from-file <path>`: Load object from YAML file
- `--interactive`: Interactive creation mode
- `--dry-run`: Validate without creating

**Example**:
```bash
zqk create backlog-item \
  --title "Implement Graph Backend" \
  --description "Design and implement graph database backend" \
  --priority high \
  --category Architecture
```

### List Command

**Pattern**: `zqk object list <kind> [filters] [flags]`

**Semantics**:
- Lists objects of the specified kind
- Supports filtering by various criteria
- Returns list of object IDs or summaries

**Filters**:
- `--status <status>`: Filter by status
- `--priority <priority>`: Filter by priority
- `--category <category>`: Filter by category
- `--workstream <id>`: Filter by workstream
- `--milestone <id>`: Filter by milestone

**Flags**:
- `--format <format>`: Output format (table, json, yaml)
- `--output <path>`: Write output to file
- `--limit <n>`: Limit number of results
- `--sort <field>`: Sort by field

**Example**:
```bash
zqk object list backlog-item \
  --status in_progress \
  --priority high \
  --format table
```

### Get Command

**Pattern**: `zqk object get <id> [flags]`

**Semantics**:
- Retrieves a single object by ID
- Returns full object representation
- Supports multiple output formats

**Flags**:
- `--format <format>`: Output format (yaml, json, markdown)
- `--output <path>`: Write output to file
- `--include-related`: Include related objects
- `--include-history`: Include change history

**Example**:
```bash
zqk object get BLI-237 --format markdown
```

### Update Command

**Pattern**: `zqk update <id> [fields] [flags]`

**Semantics**:
- Updates an existing object
- Fields can be specified via flags or file
- Returns updated object

**Flags**:
- `--field <key=value>`: Update specific field
- `--from-file <path>`: Load updates from file
- `--interactive`: Interactive update mode
- `--dry-run`: Validate without updating

**Example**:
```bash
zqk update BLI-237 \
  --field status=complete \
  --field updated_by="account:ai-assistant"
```

### Delete Command

**Pattern**: `zqk delete <id> [flags]`

**Semantics**:
- Deletes an object (or marks as archived)
- Requires confirmation unless --force specified
- Returns deletion confirmation

**Flags**:
- `--force`: Skip confirmation
- `--archive`: Archive instead of delete
- `--dry-run`: Validate without deleting

**Example**:
```bash
zqk delete BLI-999 --archive
```

## Domain Commands

### Backlog Commands

**Pattern**: `zqk backlog <subcommand> [arguments] [flags]`

**Subcommands**:
- `list [filters]`: List backlog items
- `create [flags]`: Create backlog item
- `update <id> [fields]`: Update backlog item
- `promote <id>`: Promote to planned status
- `archive <id>`: Archive backlog item
- `set-priority <id> <priority>`: Set priority

**Example**:
```bash
zqk backlog promote BLI-237
zqk backlog set-priority BLI-238 critical
```

### Goal Commands

**Pattern**: `zqk goal <subcommand> [arguments] [flags]`

**Subcommands**:
- `list [filters]`: List goals
- `create [flags]`: Create goal
- `update <id> [fields]`: Update goal
- `link-metric <id> <metric-id>`: Link metric template
- `set-target <id> <target>`: Set target value

**Example**:
```bash
zqk goal create \
  --title "Complete Graph Backend" \
  --metric-template-ref METRIC-001 \
  --target "100%"
```

### Milestone Commands

**Pattern**: `zqk milestone <subcommand> [arguments] [flags]`

**Subcommands**:
- `list [filters]`: List milestones
- `create [flags]`: Create milestone
- `update <id> [fields]`: Update milestone
- `set-target-date <id> <date>`: Set target date
- `link-items <id> <item-ids>`: Link items to milestone

**Example**:
```bash
zqk milestone create \
  --title "Phase 1 Complete" \
  --target-date "2025-12-31"
```

### Workstream Commands

**Pattern**: `zqk workstream <subcommand> [arguments] [flags]`

**Subcommands**:
- `list [filters]`: List workstreams
- `create [flags]`: Create workstream
- `update <id> [fields]`: Update workstream
- `link-goal <id> <goal-id>`: Link goal to workstream
- `status <id>`: Show workstream status

**Example**:
```bash
zqk workstream status WS-007
```

### Priority Plan Commands

**Pattern**: `zqk priority-plan <subcommand> [arguments] [flags]`

**Subcommands**:
- `list [filters]`: List priority plans
- `create [flags]`: Create priority plan
- `current`: Show current priority plan
- `apply <id>`: Apply priority plan
- `generate`: Generate priority plan from backlog

**Example**:
```bash
zqk priority-plan current
zqk priority-plan apply PRI-207
```

## System Commands

### Init Command

**Pattern**: `zqk init [flags]`

**Semantics**:
- Initializes a new zqk project
- Creates directory structure
- Sets up configuration files

**Flags**:
- `--project-name <name>`: Project name
- `--template <template>`: Use template
- `--force`: Overwrite existing files

**Example**:
```bash
zqk init --project-name myproject
```

### Status Command

**Pattern**: `zqk status [flags]`

**Semantics**:
- Shows system status and health
- Displays current priority plan
- Shows recent activity

**Flags**:
- `--verbose`: Detailed status
- `--format <format>`: Output format

**Example**:
```bash
zqk status --verbose
```

### Validate Command

**Pattern**: `zqk validate [object] [flags]`

**Semantics**:
- Validates object(s) against schema
- Checks references and relationships
- Reports validation errors

**Flags**:
- `--all`: Validate all objects
- `--kind <kind>`: Validate objects of kind
- `--fix`: Attempt to fix errors

**Example**:
```bash
zqk validate BLI-237
zqk validate --all --kind backlog_item
```

### Sync Command

**Pattern**: `zqk sync [flags]`

**Semantics**:
- Synchronizes local state with remote repository
- Pulls changes from remote
- Pushes local changes

**Flags**:
- `--pull`: Pull from remote
- `--push`: Push to remote
- `--verify`: Verify signatures

**Example**:
```bash
zqk sync --pull --verify
```

## Utility Commands

### Help Command

**Pattern**: `zqk help [command]`

**Semantics**:
- Shows help for command or general help
- Provides usage examples
- Lists available flags

**Example**:
```bash
zqk help
zqk help backlog
```

### Version Command

**Pattern**: `zqk version [flags]`

**Semantics**:
- Shows zqk version information
- Displays build metadata

**Example**:
```bash
zqk version
```

### Docman Commands

**Pattern**: `zqk docman <subcommand> [arguments] [flags]`

**Subcommands**:
- `list [filters]`: List documents
- `register <path> [flags]`: Register document
- `get <path>`: Get document
- `index`: Rebuild document index

**Example**:
```bash
zqk docman register docs/architecture/design.md \
  --goal-ref GOAL-001 \
  --workstream-ref WS-007
```

## Flag Definitions

### Common Flags

**Output Format**:
- `--format <format>`: Output format (table, json, yaml, markdown)
- `--output <path>`: Write output to file
- `--quiet`: Suppress non-essential output
- `--verbose`: Verbose output

**Filtering**:
- `--status <status>`: Filter by status
- `--priority <priority>`: Filter by priority
- `--category <category>`: Filter by category
- `--workstream <id>`: Filter by workstream
- `--milestone <id>`: Filter by milestone

**Validation**:
- `--dry-run`: Validate without executing
- `--force`: Skip confirmation
- `--interactive`: Interactive mode

### Flag Semantics

**Format Values**:
- `table`: Human-readable table format
- `json`: JSON format (machine-readable)
- `yaml`: YAML format (machine-readable)
- `markdown`: Markdown format (human-readable)

**Status Values**:
- `exploring`: Initial exploration
- `validated`: Validated and ready
- `planned`: Planned for execution
- `in_progress`: Currently in progress
- `complete`: Completed
- `archived`: Archived

**Priority Values**:
- `critical`: Critical priority
- `high`: High priority
- `medium`: Medium priority
- `low`: Low priority

## Context Propagation

### Command Context

Commands maintain context across invocations:

1. **Current Priority Plan**: Commands operate in context of current priority plan
2. **Current Workstream**: Commands can be scoped to workstream
3. **Current Milestone**: Commands can be scoped to milestone
4. **Project Root**: Commands operate relative to project root

### Context Flags

- `--priority-plan <id>`: Set priority plan context
- `--workstream <id>`: Set workstream context
- `--milestone <id>`: Set milestone context
- `--project-root <path>`: Set project root

## Operation Mappings

### System Ontology Mappings

Commands map to system ontology operations:

| Command | Ontology Operation | Object Kind |
|---------|-------------------|-------------|
| `create backlog-item` | Create | `backlog_item` |
| `list backlog-item` | Query | `backlog_item` |
| `get BLI-237` | Read | `backlog_item` |
| `update BLI-237` | Update | `backlog_item` |
| `delete BLI-237` | Delete | `backlog_item` |

### Relationship Mappings

Commands can create relationships:

| Command | Relationship Type | Example |
|---------|------------------|---------|
| `backlog link-milestone` | `BELONGS_TO` | BLI-237 → MIL-035 |
| `goal link-workstream` | `SUPPORTS` | GOAL-001 → WS-007 |
| `milestone link-items` | `CONTAINS` | MIL-035 → [BLI-237, BLI-238] |

## Progressive Disclosure

### Simple Commands

For common use cases, commands can be simplified:

```bash
# Simple creation
zqk backlog create "Implement Graph Backend"

# Simple update
zqk backlog complete BLI-237

# Simple listing
zqk backlog in-progress
```

### Advanced Commands

For complex operations, full command syntax:

```bash
# Advanced creation
zqk create backlog-item \
  --title "Implement Graph Backend" \
  --description "..." \
  --priority high \
  --category Architecture \
  --milestone-ref MIL-035 \
  --priority-plan-ref PRI-207

# Advanced update
zqk update BLI-237 \
  --field status=complete \
  --field updated_by="account:ai-assistant" \
  --field completed_at="2025-12-24T12:00:00Z"
```

## Error Handling

### Error Types

1. **Validation Errors**: Invalid input or state
2. **Not Found Errors**: Object doesn't exist
3. **Permission Errors**: Insufficient permissions
4. **Conflict Errors**: Concurrent modification conflicts
5. **System Errors**: Internal system errors

### Error Format

Errors follow consistent format:

```json
{
  "error": {
    "type": "validation_error",
    "message": "Invalid status transition",
    "code": "INVALID_STATUS_TRANSITION",
    "details": {
      "current_status": "planned",
      "requested_status": "complete",
      "allowed_transitions": ["in_progress"]
    }
  }
}
```

### Error Handling Patterns

**Interactive Mode**:
- Prompts for correction
- Suggests alternatives
- Provides context

**Non-Interactive Mode**:
- Returns error code
- Provides error message
- Exits with non-zero code

## Machine-Readable Ontology

### CLI Ontology Schema

The CLI ontology is defined in machine-readable format:

```yaml
cli_ontology:
  version: "1.0.0"
  command_structure:
    standard_form: "zqk <verb> <kind> [arguments] [flags]"
    specialized_form: "zqk <namespace> <verb> [arguments] [flags]"
    mapping_principle: "Specialized commands map to generic form via namespace-to-kind resolution"
  
  verbs:
    - name: list
      description: "List objects of specified kind"
      pattern: "zqk object list <kind> [filters] [flags]"
      specialized_pattern: "zqk <namespace> list [filters] [flags]"
      read_only: true
      
    - name: create
      description: "Create a new object"
      pattern: "zqk create <kind> [flags]"
      specialized_pattern: "zqk <namespace> create [flags]"
      read_only: false
      
    - name: get
      description: "Get a single object by ID"
      pattern: "zqk object get <id> [flags]"
      specialized_pattern: "zqk <namespace> get <id> [flags]"
      read_only: true
      
    - name: update
      description: "Update an existing object"
      pattern: "zqk update <id> [fields] [flags]"
      specialized_pattern: "zqk <namespace> update <id> [fields] [flags]"
      read_only: false
      
    - name: delete
      description: "Delete an object"
      pattern: "zqk delete <id> [flags]"
      specialized_pattern: "zqk <namespace> delete <id> [flags]"
      read_only: false
  
  namespaces:
    - name: backlog
      kind: backlog-item
      description: "Backlog item operations"
      
    - name: goal
      kind: goal
      description: "Goal operations"
      
    - name: milestone
      kind: milestone
      description: "Milestone operations"
      
    - name: workstream
      kind: workstream
      description: "Workstream operations"
      
    - name: priority-plan
      kind: priority_plan
      description: "Priority plan operations"
  
  context_profiles:
    - name: ai-agent
      format: json
      verbose: false
      write_enabled: true
      description: "Machine-readable output for AI agents"
      
    - name: human
      format: table
      verbose: false
      write_enabled: true
      description: "Human-friendly table output"
      
    - name: debug
      format: yaml
      verbose: true
      write_enabled: true
      description: "Debugging with full details"
      
    - name: dry-run
      format: table
      verbose: true
      write_enabled: false
      description: "Show impacts without making changes"
  
  common_flags:
    - name: context
      type: enum
      values: [ai-agent, human, debug, dry-run]
      description: "Context profile for output and behavior"
      
    - name: format
      type: enum
      values: [json, yaml, table, markdown]
      description: "Output format"
      
    - name: verbose
      type: boolean
      description: "Enable verbose output"
      
    - name: quiet
      type: boolean
      description: "Suppress non-essential output"
      
    - name: dry-run
      type: boolean
      description: "Short-circuit writes and show impacts (alternative to --context dry-run)"
  
  examples:
    - command: "zqk object list backlog-item --status in_progress"
      description: "Generic form: List backlog items with in_progress status"
      
    - command: "zqk backlog list --status in_progress"
      description: "Specialized form: Same as above, using backlog namespace"
      
    - command: "zqk update BLI-237 --field status=complete --context dry-run"
      description: "Dry-run update: Show impacts without making changes"
      
    - command: "zqk create goal --title 'New Goal' --context ai-agent"
      description: "Create with AI agent context (JSON output)"
```

### Query Interface

Agents can query CLI ontology:

```bash
# Get command definition
zqk ontology command create

# List all commands
zqk ontology commands

# Get flag definitions
zqk ontology flags create
```

## Implementation Structure

### Command Organization

```
zqk/
├── cmd/
│   ├── root.go              # Root command
│   ├── create.go            # Create command
│   ├── list.go              # List command
│   ├── get.go               # Get command
│   ├── update.go            # Update command
│   ├── delete.go            # Delete command
│   ├── backlog.go           # Backlog commands
│   ├── goal.go              # Goal commands
│   ├── milestone.go         # Milestone commands
│   ├── workstream.go        # Workstream commands
│   ├── priority-plan.go     # Priority plan commands
│   ├── init.go              # Init command
│   ├── status.go            # Status command
│   ├── validate.go          # Validate command
│   ├── sync.go              # Sync command
│   └── docman.go            # Docman commands
│
├── internal/
│   ├── ontology/            # CLI ontology definitions
│   ├── parser/              # Command parser
│   ├── validator/           # Input validator
│   └── formatter/           # Output formatter
```

## Related Documents

- **System Ontology v1.0**: `docs/architecture/ontology/system-ontology-v1.0.md`
- **Knowledge Kernel Separation v1.0**: `docs/architecture/architecture/knowledge-kernel-separation-v1.0.md`
- **Backlog Item**: BLI-243 (Establish CLI Ontology for AI Agent and Human Interpretation)
- **Milestone**: MIL-035 (Knowledge Kernel Graph Structure)

## Next Steps

1. ✅ **Complete**: CLI Ontology Specification v1.0
2. **Next**: Implement CLI ontology schema (Phase 2)
3. **Next**: Deploy command structure
4. **Next**: Create machine-readable ontology definitions

---

**Status**: Design Complete - Ready for Implementation  
**Approved By**: Architecture Review  
**Last Updated**: 2025-12-24

