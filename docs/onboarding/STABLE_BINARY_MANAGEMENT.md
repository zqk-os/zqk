# Stable Binary Management Guide

This guide explains how to manage the stable MCP binary and when to update it.

## Quick Reference

### When to Update

**Must Update**:
- MCP protocol changes
- Bug fixes in MCP or exposed commands
- Security fixes
- New exposed commands added

**Should Update**:
- Performance improvements
- Logging improvements
- Compatibility improvements

**No Update Needed**:
- Non-MCP code changes
- Unexposed command changes
- Development-only features

### Update Process

```bash
# Use the update script
./scripts/update-stable-binary.sh

# Or manually:
go build -o bin/zqk-stable ./cmd/zqk
# Update version in .zqk/mcp/config.yaml
# Test MCP server
# Restart MCP server
```

## Decision Framework

See [MCP Binary Stability Decision Framework](../process/architecture/mcp-binary-stability-decision-framework.md) for detailed guidance.

## Version Management

Versions are stored in:
- `.zqk/mcp/config.yaml` - `binary_version` field
- Change history: see docs/architecture/architecture/mcp/ or git history

## Testing

After updating:
1. Test MCP server startup
2. Test all exposed commands
3. Test JSON-RPC communication
4. Monitor for issues

## Rollback

If issues occur:
1. Restore backup binary (created by update script)
2. Update config to previous version
3. Restart MCP server
4. Investigate issue
