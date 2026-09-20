# CLI Package

This package provides shared utilities and context management for the ZQK CLI.

## Context System

The CLI uses a layered context system with precedence:

1. **System Defaults** (lowest precedence) - Built-in defaults
2. **User Config** (`~/.zqk/config.yaml`) - User-level preferences
3. **Project Config** (`.zqk/config.yaml`) - Project-specific settings
4. **Command Flags** (highest precedence) - Command-line arguments

### Context Profiles

Context profiles provide pre-configured settings for different use cases:

- **`ai-agent`**: Sets format to JSON, verbose off (for machine-readable output)
- **`human`**: Sets format to table, verbose off (human-friendly defaults)
- **`debug`**: Sets format to YAML, verbose on (for debugging)

### Usage

```go
// In command implementation
ctx := cli.GetContext(cmd)
format := ctx.Format // Respects all precedence layers
```

### Configuration Files

#### User Config (`~/.zqk/config.yaml`)
```yaml
format: json
verbose: false
profile: ai-agent
```

#### Project Config (`.zqk/config.yaml`)
```yaml
format: table
priority_plan: PRI-208
workstream: WS-007
```

### Command Flags

```bash
# Use ai-agent profile (sets format to JSON)
zqk check --context ai-agent

# Override format even with profile
zqk check --context ai-agent --format yaml
```

## Helper Functions

- `GetFormat(cmd)` - Get output format respecting context precedence
- `IsVerbose(cmd)` - Check if verbose mode is enabled
- `IsQuiet(cmd)` - Check if quiet mode is enabled
- `AddCommonFlags(cmd)` - Add common flags to a command
- `GetContextFromCommand(cmd, projectRoot)` - Load and apply context

