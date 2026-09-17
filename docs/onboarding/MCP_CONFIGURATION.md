# MCP Configuration Guide

This guide explains how to configure the MCP server to use a stable binary and manage command exposure.

## Configuration Location

The MCP configuration is located at: `.zqk/mcp/config.yaml`

## Stable Binary Setup

### 1. Create Stable Binary Directory

```bash
mkdir -p bin
```

### 2. Build or Install Stable Binary

The stable binary should be a versioned, tested release separate from the development binary:

```bash
# Example: Copy current binary as stable (for initial setup)
cp ./zqk ./bin/zqk-stable

# Or build a specific version
go build -o bin/zqk-stable-v1.0.0 ./cmd/zqk
```

### 3. Configure Binary Path

Set the environment variable or update `.zqk/mcp/config.yaml`:

```bash
export ZQK_STABLE_BINARY_PATH=./bin/zqk-stable
```

## Command Exposure

### View Current Configuration

```bash
cat .zqk/mcp/config.yaml
```

### Adding a New Command

1. Edit `.zqk/mcp/config.yaml`
2. Add command to `exposed_commands` list
3. Verify stable binary includes the command
4. Test via MCP
5. Commit configuration change

### Enabling Write Operations

1. Edit `.zqk/mcp/config.yaml`
2. Add command to `write_operations` list
3. Ensure security permissions are appropriate
4. Test thoroughly
5. Update policy documentation if needed

## Compatibility Checking

### Verify Binary Compatibility

```bash
# Check if binary exists
test -f ./bin/zqk-stable && echo "Binary found" || echo "Binary missing"

# Check binary version
./bin/zqk-stable version
```

### Check Schema Compatibility

The stable binary must support at least the `min_schema_version` specified in the config (currently 2.0.0).

## Agent Session Management

### Between Sessions

Agents should verify MCP configuration:

```bash
# Check configuration exists
test -f .zqk/mcp/config.yaml && echo "Config found" || echo "Config missing"

# Check binary exists
test -f $(grep stable_binary_path .zqk/mcp/config.yaml | cut -d: -f2 | tr -d ' "') && echo "Binary found" || echo "Binary missing"
```

### Binary Updates

When updating the stable binary:

1. **Stop MCP server** (if running)
2. **Backup current binary**: `cp ./bin/zqk-stable ./bin/zqk-stable.backup`
3. **Install new binary**: `cp <new-binary> ./bin/zqk-stable`
4. **Update version in config**: Edit `binary_version` in `.zqk/mcp/config.yaml`
5. **Verify compatibility**: Test exposed commands
6. **Restart MCP server**
7. **Test all exposed commands**

## Policy Reference

See [POL-ARCH-002](../process/policies/POL-ARCH-002.yaml) for complete policy details.

## Related Documentation

- [MCP Binary Stability Architecture](../process/architecture/mcp-binary-stability-v1.0.md)
- [MCP CLI Bridge Architecture](../process/architecture/mcp-cli-bridge-v1.0.md)
