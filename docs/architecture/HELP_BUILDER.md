# Help Builder Documentation

**Last Verified:** 2026-08-31


The `HelpBuilder` provides a fluent, dynamic builder pattern for creating consistent command help text across the CLI. It automatically discovers flags and subcommands, reducing the need to manually keep help text in sync with command structure.

## Features

### 1. **Auto-Discovery of Flags**
Automatically discovers and documents all flags from a command, including:
- Flag names and shorthand
- Type information
- Default values
- Usage descriptions
- Optional categorization

### 2. **Auto-Discovery of Subcommands**
Automatically lists available subcommands with their short descriptions.

### 3. **Spec-Driven Examples**
Integrates with `cliexamples.Generator` to generate examples from object specs, ensuring examples stay current as specs evolve.

### 4. **Flexible Configuration**
- Exclude specific flags from documentation
- Categorize flags for organized display
- Include or exclude common flags
- Combine manual and auto-generated content

## Usage Examples

### Basic Usage

```go
helpBuilder := cli.StandardHelpBuilder(
    "Short description",
    "Long description with details.",
).
    AddExample("Comment", "%s command --flag value").
    ApplyToCommand(cmd)
```

### Dynamic Help (Recommended) - Multi-line Description

```go
// No backticks needed! Just pass multiple lines
helpBuilder := cli.DynamicHelpBuilder(
    "Short description",
    "First line of description.",
    "Second line of description.",
    "",  // Empty string = blank line
    "More details here.",
    "  - Bullet point 1",
    "  - Bullet point 2",
).
    AddExample("Example comment", "%s command example").
    ExcludeFlags("format", "output"). // Exclude common flags
    ApplyToCommand(cmd)
```

### Building Descriptions Incrementally

```go
helpBuilder := cli.NewHelpBuilder().
    WithShort("Short description").
    AddDescriptionLine("First line").
    AddDescriptionLine("Second line").
    AddDescriptionParagraph("A new paragraph with blank line before it").
    AddDescriptionLine("Another line").
    ApplyToCommand(cmd)
```

### Object Commands with Spec-Driven Examples

```go
helpBuilder := cli.ObjectCommandHelpBuilder(
    "Create an object",
    "Create a new object of the specified kind.",
    "backlog_item", // Kind for spec-driven examples
).
    AddExample("Manual example", "%s create backlog_item --file item.yaml").
    ApplyToCommand(cmd)
```

### Advanced Configuration

```go
helpBuilder := cli.NewHelpBuilder().
    WithShort("Short description").
    WithDescription("Long description").
    WithAutoDiscoverFlags(true).
    WithAutoDiscoverSubcommands(true).
    WithIncludeCommonFlags(false).
    CategorizeFlag("Filtering", "filter").
    CategorizeFlag("Filtering", "sort-by").
    ExcludeFlag("internal-flag").
    AddExample("Example", "%s command").
    AddSection("Notes", "Additional notes here").
    ApplyToCommand(cmd)
```

## API Reference

### Builder Methods

**Description Building:**
- `WithShort(short string)` - Set short description
- `WithDescription(description string)` - Set long description (can be called multiple times)
- `WithDescriptionLines(lines ...string)` - Set description from multiple lines (variadic)
- `AddDescriptionLine(line string)` - Add a single line to description
- `AddDescriptionParagraph(paragraph string)` - Add a paragraph (adds blank line before)

**Examples and Sections:**
- `AddExample(comment, command string)` - Add manual example
- `AddSection(title, content string)` - Add custom section

**Configuration:**
- `WithAutoDiscoverFlags(enabled bool)` - Enable/disable flag discovery
- `WithAutoDiscoverSubcommands(enabled bool)` - Enable/disable subcommand discovery
- `WithIncludeCommonFlags(include bool)` - Include common flags in docs
- `ExcludeFlag(flagName string)` - Exclude a flag from docs
- `ExcludeFlags(flagNames ...string)` - Exclude multiple flags
- `CategorizeFlag(category, flagName string)` - Categorize a flag
- `WithSpecExamples(kind string)` - Enable spec-driven examples for a kind
- `WithTerminalWidth(width int)` - Set terminal width (0 = auto-detect)

**Application:**
- `ApplyToCommand(cmd *cobra.Command)` - Apply to command

### Convenience Functions

- `StandardHelpBuilder(short, descriptionLines...)` - Basic builder (descriptionLines is variadic)
- `DynamicHelpBuilder(short, descriptionLines...)` - Builder with all dynamic features (descriptionLines is variadic)
- `ObjectCommandHelpBuilder(short, kind, descriptionLines...)` - Builder for object commands (descriptionLines is variadic)

All convenience functions accept multiple description lines - just pass them as separate arguments!

## Best Practices

1. **Use Dynamic Help**: Prefer `DynamicHelpBuilder` for commands with flags/subcommands
2. **Exclude Common Flags**: Use `ExcludeFlags()` to hide common flags that cobra shows automatically
3. **Combine Manual and Auto**: Add manual examples for common cases, let auto-discovery handle the rest
4. **Use Spec Examples**: For object commands, use `ObjectCommandHelpBuilder` with a kind for spec-driven examples
5. **Categorize Flags**: Use `CategorizeFlag()` for commands with many flags to improve readability

## Migration Guide

### Before (Manual with Backticks)

```go
cmd := &cobra.Command{
    Use:   "list [kind]",
    Short: "List objects",
    Long: `List objects of a specific kind.

Examples:
  zqk list backlog_item
  zqk list backlog_item --filter status=active`,
}
```

### After (Dynamic - No Backticks!)

```go
helpBuilder := cli.DynamicHelpBuilder(
    "List objects",
    "List objects of a specific kind.",
).
    AddExample("List backlog items", "%s list backlog_item").
    AddExample("With filter", "%s list backlog_item --filter status=active").
    ExcludeFlags("format", "output").
    ApplyToCommand(cmd)
```

### Multi-line Description (No Backticks!)

```go
helpBuilder := cli.DynamicHelpBuilder(
    "Create a new object",
    "Create a new object of the specified kind.",
    "",
    "The object data can be provided via:",
    "  - --file: Path to a YAML file",
    "  - --data: Inline YAML data",
    "  - stdin: YAML data piped from another command",
    "",
    "Note: Source files are automatically removed after successful creation.",
    "Use --keep-file to preserve the source file.",
).
    AddExample("Create from file", "%s create backlog_item --file item.yaml").
    ApplyToCommand(cmd)
```

The dynamic version automatically:
- Discovers and documents all flags
- Lists available subcommands
- Stays in sync as flags are added/removed
- Can generate spec-driven examples
