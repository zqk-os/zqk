# MCP Binary Stability and Compatibility Architecture v1.0

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-12-31  
**Status**: Active  
**Category**: Architecture & Integration  
**Related**: POL-ARCH-002, MCP CLI Bridge v1.0

## Overview

This document defines the architecture for managing MCP server binary stability, ensuring that the MCP server uses a stable, versioned binary that remains compatible with project artifacts even as development continues. It addresses the challenge of maintaining compatibility between a stable MCP binary and evolving project state.

## Problem Statement

**Challenges**:
1. **Binary Stability**: MCP server needs a stable binary while CLI development continues
2. **Selective Exposure**: Not all CLI commands should be exposed via MCP
3. **Compatibility**: Stable binary must work with current project artifacts
4. **Version Management**: Need to track binary version and schema compatibility
5. **Agent Sessions**: Agents need to know what's available and verify compatibility

## Solution: Stable Binary with Selective Command Exposure

### Architecture Components

1. **Stable Binary**: Versioned, tested binary separate from development version
2. **Configuration File**: Explicit list of exposed commands
3. **Compatibility Checking**: Version and schema validation
4. **Selective Exposure**: Only approved commands are available via MCP
5. **Policy Enforcement**: POL-ARCH-002 governs binary management

### Configuration Structure

**Location**: `.zqk/mcp/config.yaml`

```yaml
mcp_server:
  stable_binary_path: "./bin/zqk-stable"
  binary_version: "1.0.0"
  min_schema_version: "2.0.0"
  exposed_commands:
    - "object list"
    - "object get"
    # ... selective list
  write_operations:
    # Explicitly enabled write commands
  blocked_commands:
    - "system sync"
    # ... explicitly blocked
```

### Binary Selection

**Stable Binary Requirements**:
- Versioned (e.g., `zqk-stable-v1.0.0`)
- Supports minimum schema version (2.0.0)
- Includes all exposed commands
- Tested against current project artifacts
- Separate from development binary (`./zqk`)

**Binary Location**:
- Default: `./bin/zqk-stable`
- Override: `ZQK_STABLE_BINARY_PATH` environment variable
- Versioned: `./bin/zqk-stable-v1.0.0`

### Command Exposure Model

**Three-Tier Model**:

1. **Exposed Commands** (`exposed_commands`):
   - Explicitly listed commands available via MCP
   - Default: Read-only operations only
   - Must exist in stable binary

2. **Write Operations** (`write_operations`):
   - Explicitly enabled write commands
   - Require additional security checks
   - Default: Empty (no write operations)

3. **Blocked Commands** (`blocked_commands`):
   - Explicitly blocked even if they exist
   - Patterns supported (e.g., `"system *"`)
   - Override exposed commands

**Exposure Logic**:
```
Command is available via MCP if:
  - Listed in exposed_commands AND
  - Not listed in blocked_commands AND
  - Exists in stable binary AND
  - Security context permits access
```

### Compatibility Checking

**On Startup**:
1. Verify binary exists and is executable
2. Check binary version matches configuration
3. Verify schema version compatibility
4. Test critical exposed commands
5. Log warnings for non-critical failures

**Compatibility Modes**:
- **Non-blocking** (default): Log warnings, continue with limited functionality
- **Blocking**: Fail startup if incompatible, require binary update

**Schema Evolution**:
- When project schema evolves, update `min_schema_version`
- Verify stable binary supports new schema
- Update binary if necessary
- Test all exposed commands

### Agent Session Management

**Between Sessions**:
1. Agent verifies MCP server is running
2. Checks exposed commands match expectations
3. Verifies compatibility status
4. Reports any warnings

**Binary Updates**:
1. Stop MCP server
2. Backup current binary
3. Install new binary
4. Update configuration
5. Verify compatibility
6. Restart MCP server
7. Test exposed commands

### Security Considerations

**Default Safety**:
- Read-only operations by default
- Write operations require explicit enablement
- Additional security checks for write operations
- Context profile: `ai-agent` (consistent behavior)

**Permission Model**:
- Commands filtered by security context
- Write operations require `write:*` or specific permissions
- Blocked commands never exposed regardless of permissions

## Implementation

### Configuration Loading

```go
type MCPConfig struct {
    StableBinaryPath    string   `yaml:"stable_binary_path"`
    BinaryVersion       string   `yaml:"binary_version"`
    MinSchemaVersion    string   `yaml:"min_schema_version"`
    ExposedCommands     []string `yaml:"exposed_commands"`
    WriteOperations     []string `yaml:"write_operations"`
    BlockedCommands     []string `yaml:"blocked_commands"`
    Compatibility       CompatibilityConfig `yaml:"compatibility"`
    Security            SecurityConfig      `yaml:"security"`
}
```

### Binary Discovery

```go
func findStableBinary(config *MCPConfig) (string, error) {
    // Check environment variable first
    if path := os.Getenv("ZQK_STABLE_BINARY_PATH"); path != "" {
        return path, nil
    }
    
    // Use configured path
    path := expandPath(config.StableBinaryPath)
    
    // Verify exists and executable
    if _, err := os.Stat(path); err != nil {
        return "", fmt.Errorf("stable binary not found: %w", err)
    }
    
    return path, nil
}
```

### Command Filtering

```go
func isCommandExposed(cmdPath string, config *MCPConfig) bool {
    // Check blocked first (highest priority)
    for _, blocked := range config.BlockedCommands {
        if matchesPattern(cmdPath, blocked) {
            return false
        }
    }
    
    // Check exposed list
    for _, exposed := range config.ExposedCommands {
        if matchesPattern(cmdPath, exposed) {
            return true
        }
    }
    
    // Default: not exposed
    return false
}
```

## Benefits

1. **Stability**: MCP uses stable binary, unaffected by development changes
2. **Safety**: Selective exposure prevents accidental misuse
3. **Transparency**: Clear documentation of available commands
4. **Compatibility**: Version checking prevents incompatibility issues
5. **Flexibility**: Can enable/disable commands without code changes

## Limitations

1. **Binary Management**: Requires manual binary updates
2. **Version Tracking**: Must keep binary version in sync with config
3. **Testing**: Must test stable binary against project artifacts
4. **Deployment**: Binary must be deployed separately from development version

## Related Documentation

- [POL-ARCH-002](../policies/POL-ARCH-002.yaml): MCP Binary Stability Policy
- [MCP CLI Bridge v1.0](./mcp-cli-bridge-v1.0.md): CLI bridge architecture
- [MCP Configuration](../.zqk/mcp/config.yaml): Configuration file
