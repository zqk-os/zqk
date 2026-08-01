# CLI Architecture & Semantic Structure

**Purpose:** Comprehensive documentation of the CLI architecture, command patterns, and semantic structure for improved discoverability, maintainability, and graphRAG understanding.

## Command Organization

The CLI is organized into **topical command groups** that reflect the system's ontology:

### Core Command Groups

1. **`object`** - Object operations (CRUD, query, and management)
   - Works across all object kinds
   - Common operations: create, get, list, update, delete, count
   - Relationship traversal: related, path, neighbors
   - Specialized: bulk (batch operations), template, move
   - Kind-based: `<kind> fields` (explore fields for a specific kind)

2. **`internal`** - Internal and built-in object management (admin only)
   - Privileged access to immutable objects
   - Same CRUD operations as `object` but with admin privileges
   - Used for system objects, specs, lifecycles, templates

3. **`system`** - System-level operations
   - Health checks, validation, maintenance
   - Status, sync, check, metrics
   - System configuration and diagnostics

4. **`utility`** - Helper operations
   - Version, migration, validation
   - Utility functions for common tasks

5. **`automation`** - Automation and integration operations
   - Git hooks, CI/CD, scripts
   - Lint bypass audit, docman sync

6. **`semantic`** - Semantic operations and maturity assessment
   - Ontology management
   - Semantic maturity assessment

7. **`docman`** - Documentation management
   - Discover and register markdown files
   - Documentation registration

8. **`scheduler`** - Scheduler operations
   - Activity tracking, history
   - Test failure analysis

9. **`reports`** - Reporting operations
   - PCS, EDD, blockers reports

## Command Patterns

### Standard CRUD Pattern

All CRUD operations follow a consistent pattern:

```go
// 1. Command Definition
func NewCreateCmd() *cobra.Command {
    helpBuilder := clipkg.DynamicHelpBuilder(...)
    cmd := &cobra.Command{Use: "create <kind>", RunE: runCreate}
    helpBuilder.ApplyToCommand(cmd)
    cli.AddCommonFlags(cmd)
    // Add command-specific flags
    return cmd
}

// 2. Command Execution
func runCreate(cmd *cobra.Command, args []string) error {
    // a. Create processor (handles context, storage, security, logging)
    proc, err := cli.NewProcessor(cmd)
    
    // b. Load data (from file, data flag, or stdin)
    objData, filePath, err := clipkg.LoadObjectData(cmd, proc.Logger())
    
    // c. Validate (kind matching, etc.)
    if err := clipkg.EnsureKindMatches(objData, kind, proc.Logger()); err != nil {
        return err
    }
    
    // d. Handle dry-run
    handled, result, err := clipkg.HandleDryRun(cmd, objData, kind, proc.Logger(), "object")
    if handled && result != nil {
        return cli.FormatOutput(cmd, result)
    }
    
    // e. Perform operation
    if err := proc.Storage().Create(...); err != nil {
        return err
    }
    
    // f. Cleanup and output
    clipkg.CleanupSourceFile(cmd, filePath, proc.Logger())
    msg := clipkg.FormatCreateSuccessMessage(...)
    return cli.WriteOutput(cmd, []byte(msg))
}
```

### Bulk Operation Pattern

Bulk operations follow a similar pattern but handle multiple objects:

```go
func runBulkCreate(cmd *cobra.Command, args []string) error {
    // Load objects from file (YAML array)
    // Normalize and validate all objects
    // Perform bulk operation
    // Output results using outputBulkResult helper
}
```

### Query Operation Pattern

List and count operations follow a query pattern:

```go
func runList(cmd *cobra.Command, args []string) error {
    // Parse query flags (filters, sorting, pagination)
    queryFlags, err := clipkg.ParseQueryFlags(cmd, logger, parseFilterString)
    
    // Build storage filter
    filter := storage.ListFilter{Kind: kind, Filters: queryFlags.Filters, ...}
    
    // Execute query
    result, err := proc.Storage().List(..., filter)
    
    // Format output (JSON, YAML, or Table)
    return cli.FormatOutput(cmd, result)
}
```

## Output Formatting Strategy

### Format Hierarchy

1. **Structured Data** → Use `cli.FormatOutput(cmd, data)`
   - Automatically respects `--format` flag
   - Supports JSON, YAML, Table formats
   - Used for: list results, get results, bulk results, count results

2. **Simple Messages** → Use `cli.WriteOutput(cmd, []byte(msg))`
   - For success messages, error messages, simple text
   - Examples: "Object created successfully", "No objects found"

3. **Complex Custom Output** → Use `cli.WriteOutput` with custom formatting
   - For specialized output (e.g., system check results, reports)
   - Should still respect `--format` flag when possible

### Format Detection

The system automatically detects the output format based on:
- `--format` flag (explicit)
- `--output` flag (file output)
- Terminal context (interactive vs. script)

## Data Flow Patterns

### Create/Update Data Flow

```
Command Args/Flags
    ↓
LoadObjectData / BuildUpdatesMap
    ↓
Validate (kind matching, normalization)
    ↓
Handle Dry-Run (if enabled)
    ↓
Storage Operation (Create/Update)
    ↓
Cleanup (file removal if needed)
    ↓
Success Message
```

### Query Data Flow

```
Command Args/Flags
    ↓
ParseQueryFlags (filters, sorting, pagination)
    ↓
Build Storage Filter
    ↓
Storage Query (List/Count)
    ↓
Format Output (JSON/YAML/Table)
```

## Error Handling Patterns

### Standard Error Pattern

```go
// ✅ Good - provides context, wraps error, and adds actionable suggestion (EnhanceError, BLI-659)
if err != nil {
    proc.Logger().LogError("Failed to create object", err,
        logging.String("kind", kind),
        logging.String("id", id))
    return cli.EnhanceError(cmd, fmt.Errorf("failed to create object: %w", err))
}

// ❌ Bad - loses context
if err != nil {
    return err
}
```

### Error Message Standards

1. **Action-oriented**: "Failed to create object" not "Error occurred"
2. **Context-rich**: Include relevant identifiers (kind, id, etc.)
3. **Chain errors**: Use `%w` for error wrapping
4. **User-friendly**: Provide actionable information when possible

## Flag Patterns

### Common Flags (via `cli.AddCommonFlags`)

- `--format`: Output format (json, yaml, table)
- `--output`: Output file path
- `--verbose`: Verbose logging
- `--quiet`: Quiet mode
- `--timeout`: Operation timeout

### CRUD-Specific Flags

**Create:**
- `--file`: Path to YAML file
- `--data`: Inline YAML data
- `--dry-run`: Show what would be created
- `--force`: Force creation (update if exists)
- `--relaxed`: Relax integrity constraints
- `--keep-file`: Don't remove source file after creation

**Update:**
- `--file`: Path to YAML file with updates
- `--data`: Inline YAML updates
- `--field`: Update single field (can be used multiple times)
- `--all`: Update all objects of a kind
- `--kind`: Object kind (required with --all)
- `--filter`: Filter objects (with --all)
- `--add-missing-fields`: Automatically add missing fields
- `--expected-updated-at`: Optimistic locking timestamp

**List:**
- `--filter`: Filter by field (can be used multiple times)
- `--sort-by`: Field to sort by
- `--sort-asc`: Sort ascending (default: true)
- `--offset`: Pagination offset
- `--limit`: Pagination limit
- `--group-by`: Group results by field
- `--group-limit`: Limit per group
- `--count`: Show count only

**Delete:**
- `--cascade`: Delete dependents
- `--dry-run`: Show what would be deleted

**Bulk:**
- `--file`: Path to YAML file (array of objects/updates)
- `--ids`: Comma-separated list of IDs
- `--force`: Force operation

## Semantic Structure

### Command Semantics

Commands are organized to reflect the system's semantic model:

- **Objects** are the primary entities (backlog items, goals, milestones, etc.)
- **Operations** are actions on objects (create, read, update, delete, query)
- **Relationships** connect objects (related, path, neighbors)
- **Kinds** define object types (backlog_item, goal, milestone, etc.)

### Discovery Patterns

Users can discover commands through:

1. **Topical Groups**: Commands organized by domain (object, system, etc.)
2. **Operation Types**: CRUD operations consistent across groups
3. **Help Text**: Dynamic help with examples and descriptions
4. **Tab Completion**: Context-aware command completion

### GraphRAG Representation

The CLI structure enables graphRAG to understand:

- **Command Hierarchy**: Top-level groups → subcommands → operations
- **Operation Patterns**: Consistent CRUD patterns across command groups
- **Data Flow**: How data moves through the system
- **Relationships**: How commands relate to each other and to objects
- **Capabilities**: What operations are available for different object types

## Best Practices

### 1. Use Shared Utilities

Always prefer shared utilities from `pkg/cli` over custom implementations:

```go
// ✅ Good
ids, err := clipkg.LoadIDsFromFlags(cmd, logger)

// ❌ Bad
idsStr, _ := cmd.Flags().GetString("ids")
ids := strings.Split(idsStr, ",")
```

### 2. Consistent Output Formatting

Use `FormatOutput` for structured data:

```go
// ✅ Good
return cli.FormatOutput(cmd, result)

// ❌ Bad (unless simple message)
return cli.WriteOutput(cmd, []byte(fmt.Sprintf("%+v", result)))
```

### 3. Comprehensive Help Text

Use `DynamicHelpBuilder` with examples:

```go
helpBuilder := clipkg.DynamicHelpBuilder(
    "Short description",
    "Long description",
    "",
    "Additional details...",
).
    AddExample("Example title", "%s command example").
    ExcludeFlags("format", "output", "verbose", "quiet")
```

### 4. Error Context

Always provide context in errors:

```go
// ✅ Good
return fmt.Errorf("failed to create %s object: %w", kind, err)

// ❌ Bad
return err
```

### 5. Logging

Use structured logging with relevant fields:

```go
proc.Logger().LogInfo("Object created",
    logging.String("id", id),
    logging.String("kind", kind))
```

## Migration Guide

When migrating commands to use shared patterns:

1. **Replace hardcoded help** with `DynamicHelpBuilder`
2. **Extract data loading** to shared utilities
3. **Standardize output** to use `FormatOutput`
4. **Use shared helpers** for CRUD operations
5. **Extract common patterns** to `pkg/cli` utilities

## Related Documentation

- `pkg/cli/README.md`: Detailed utility documentation
- `DESIGN_IMPROVEMENTS.md`: Ongoing improvements and opportunities
- `cmd/zqk/README.md`: Command organization details
