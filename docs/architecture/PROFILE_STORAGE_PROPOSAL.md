# Profile Storage Proposal

## Current State

Profiles are currently **hardcoded** in switch statements:
- `internal/cli/context/context.go` - `applyProfile()` function
- `internal/cli/context/processor.go` - `applyProfile()` function

Profiles can be **selected** in config files, but **definitions** are not stored.

## Proposed Structure

Store profile definitions in config files (user or project level) under a `profiles:` section:

### User Config (`~/.zqk/config.yaml`)

```yaml
# Profile selection (current)
profile: ai-agent

# Profile definitions (proposed)
profiles:
  ai-agent:
    format: json
    verbose: false
    quiet: false
    flags:
      format: json
      verbose: false
    description: "Machine-readable output for AI agents"
    
  human:
    format: table
    verbose: false
    quiet: false
    flags:
      format: table
    description: "Human-friendly table output"
    
  debug:
    format: yaml
    verbose: true
    quiet: false
    flags:
      format: yaml
      verbose: true
    description: "Debugging with full details"
    
  mcp:
    format: json
    verbose: false
    quiet: false
    flags:
      format: json
      context: mcp
    description: "MCP context: JSON output, logs to stderr"
    
  # Custom profile with command variants
  my-custom-profile:
    format: json
    verbose: true
    flags:
      format: json
      verbose: true
      output: /tmp/results.json
    commands:
      aliases:
        "list": "object list --format json"
        "get": "object get --format json"
    description: "Custom profile for my workflow"
```

### Project Config (`.zqk/config.yaml`)

```yaml
# Project-specific profiles can override or extend user profiles
profiles:
  # Override ai-agent for this project
  ai-agent:
    format: json
    verbose: false
    flags:
      format: json
      priority_plan: PRI-210  # Project-specific default
    
  # Project-specific profile
  project-review:
    format: table
    verbose: true
    flags:
      format: table
      verbose: true
      priority_plan: PRI-210
      workstream: WS-007
    commands:
      aliases:
        "review": "system check --format table --verbose"
    description: "Profile for project review sessions"
```

## Benefits

1. **Extensibility**: Users can define custom profiles without code changes
2. **Project-specific**: Different profiles per project
3. **Flag bundles**: Profiles can define common flag combinations
4. **Command variants**: Profiles can define command aliases/shortcuts
5. **Maintainability**: No need to hardcode profiles in switch statements
6. **Documentation**: Profile descriptions help users understand their purpose

## Implementation Considerations

1. **Precedence**: Project profiles override user profiles, which override system defaults
2. **Validation**: Validate profile definitions on load
3. **Backward compatibility**: Keep hardcoded profiles as fallback defaults
4. **Profile inheritance**: Allow profiles to extend other profiles
5. **Flag merging**: Merge profile flags with command-line flags (command-line takes precedence)

## Storage Location Priority

1. **Project config** (`.zqk/config.yaml`) - Highest precedence for project-specific profiles
2. **User config** (`~/.zqk/config.yaml`) - User-level custom profiles
3. **System defaults** (hardcoded) - Fallback for built-in profiles

## Example Usage

```bash
# Use a profile defined in config
zqk object list --context my-custom-profile

# Profile automatically applies flags and command variants
zqk object list  # Uses alias from profile if defined
```

