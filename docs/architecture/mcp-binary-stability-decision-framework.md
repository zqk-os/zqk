# MCP Binary Stability Decision Framework

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-12-30  
**Status**: Active  
**Related**: POL-ARCH-003, MCP Binary Stability Architecture v1.0

## Overview

This document provides a decision framework for determining when the stable MCP binary should be updated versus when it should remain unchanged. It helps maintain stability while allowing necessary updates.

## Core Principle

**The stable binary should remain unchanged unless there is a compelling reason to update it.**

## Decision Tree

### Question 1: Does this change affect MCP functionality?

**No** → No stable binary update needed
- Changes to non-MCP code paths
- Internal refactoring that doesn't change behavior
- Documentation-only changes
- Test-only changes

**Yes** → Continue to Question 2

### Question 2: What type of change is it?

#### A. Bug Fixes
**Update stable binary?** → **YES** (after verification)
- Fixes to MCP server initialization
- Fixes to JSON-RPC protocol handling
- Fixes to CLI command execution via MCP
- Fixes to command exposure/filtering
- Security fixes

**Process**:
1. Fix bug in development
2. Test thoroughly
3. Build new stable binary
4. Update version in config
5. Test MCP server with new binary
6. Deploy

#### B. New Features
**Update stable binary?** → **MAYBE** (depends on exposure)

**New CLI Commands**:
- If command is NOT in `exposed_commands` → **NO UPDATE** (not exposed via MCP)
- If command IS in `exposed_commands` → **YES UPDATE** (needed for MCP)

**New MCP Features**:
- New MCP protocol support → **YES UPDATE**
- New tool types → **YES UPDATE**
- New configuration options → **YES UPDATE** (if affects behavior)

#### C. Breaking Changes
**Update stable binary?** → **YES** (with migration plan)
- Schema version changes
- Command signature changes
- Protocol changes
- Configuration format changes

**Process**:
1. Document breaking change
2. Update `min_schema_version` in config
3. Build new stable binary
4. Update version
5. Test compatibility
6. Deploy with migration guide

#### D. Performance Improvements
**Update stable binary?** → **MAYBE**
- If affects MCP response times → **YES UPDATE**
- If only affects non-MCP paths → **NO UPDATE**
- If improves stability → **YES UPDATE**

#### E. Logging/Diagnostics
**Update stable binary?** → **MAYBE**
- If fixes logging pollution (stdout/stderr) → **YES UPDATE**
- If adds new diagnostic info → **MAYBE** (if useful for MCP debugging)
- If only affects non-MCP logging → **NO UPDATE**

## Change Categories

### Category 1: Must Update Stable Binary

These changes **always** require a stable binary update:

1. **MCP Protocol Changes**
   - JSON-RPC protocol updates
   - Message format changes
   - Error handling changes

2. **CLI Command Changes (Exposed)**
   - Commands listed in `exposed_commands` in `.zqk/mcp/config.yaml`
   - Command signature changes (new required flags, removed flags)
   - Command behavior changes that affect output format

3. **Security Fixes**
   - Permission/authorization fixes
   - Input validation fixes
   - Security context handling fixes

4. **Bug Fixes (MCP-Critical)**
   - Fixes that prevent MCP server from starting
   - Fixes that break JSON-RPC communication
   - Fixes that corrupt MCP responses

5. **Configuration Changes**
   - Changes to how `.zqk/mcp/config.yaml` is read/used
   - Changes to environment variable handling
   - Changes to binary discovery logic

### Category 2: Should Update Stable Binary

These changes **should** trigger a stable binary update:

1. **Performance Improvements (MCP-Affecting)**
   - Faster command execution
   - Reduced memory usage
   - Better error recovery

2. **New Exposed Commands**
   - Commands added to `exposed_commands` list
   - New write operations enabled

3. **Logging Improvements**
   - Fixes to stdout/stderr routing
   - Better error messages
   - Diagnostic improvements

4. **Compatibility Improvements**
   - Better schema version handling
   - Improved backward compatibility
   - Better error messages for incompatibilities

### Category 3: No Update Needed

These changes **do not** require a stable binary update:

1. **Non-MCP Code Changes**
   - Internal refactoring
   - Test-only changes
   - Documentation-only changes

2. **Unexposed Commands**
   - Commands not in `exposed_commands` list
   - Commands in `blocked_commands` list

3. **Development-Only Features**
   - Debug-only code paths
   - Development tools
   - Build system changes

4. **Non-Functional Changes**
   - Code formatting
   - Comment updates
   - Variable renaming (internal)

## Update Process

### Step 1: Assess Change

Use the decision tree above to determine if update is needed.

### Step 2: If Update Needed

1. **Document the Change**
   - What changed?
   - Why is update needed?
   - What are the risks?

2. **Build New Stable Binary**
   ```bash
   go build -o bin/zqk-stable ./cmd/zqk
   ```

3. **Update Version in Config**
   - Update `binary_version` in `.zqk/mcp/config.yaml`
   - Or set `ZQK_STABLE_VERSION` environment variable

4. **Test Compatibility**
   - Test all exposed commands
   - Test MCP server startup
   - Test JSON-RPC communication
   - Test with current project artifacts

5. **Update Documentation**
   - Update change log
   - Update version history
   - Document any breaking changes

6. **Deploy**
   - Commit stable binary (if versioned)
   - Update configuration
   - Restart MCP server
   - Monitor for issues

### Step 3: If No Update Needed

1. **Continue Development**
   - Changes don't affect stable binary
   - Development binary (`./zqk`) can be updated freely
   - MCP server continues using stable binary

2. **Document Decision**
   - Note why update wasn't needed
   - Track for future reference

## Version Management

### Versioning Strategy

**Option 1: Timestamp-Based**
```
binary_version: "2025-12-30-18:27"
```

**Option 2: Semantic Versioning**
```
binary_version: "1.0.1"  # patch
binary_version: "1.1.0"  # minor
binary_version: "2.0.0"  # major
```

**Option 3: Git Commit Hash**
```
binary_version: "abc1234"
```

**Recommended**: Use semantic versioning with git tags:
- Patch: Bug fixes, minor improvements
- Minor: New features, new exposed commands
- Major: Breaking changes, protocol changes

### Version Storage

Store version in:
1. `.zqk/mcp/config.yaml` - `binary_version` field
2. Environment variable - `ZQK_STABLE_VERSION`
3. Binary metadata - Build-time version info

## Examples

### Example 1: Bug Fix in MCP Server

**Change**: Fix JSON-RPC parsing error  
**Category**: Category 1 (Must Update)  
**Decision**: **UPDATE**

**Process**:
1. Fix bug in `pkg/mcp/server.go`
2. Test fix
3. Build: `go build -o bin/zqk-stable ./cmd/zqk`
4. Update version: `binary_version: "1.0.1"`
5. Test MCP server
6. Deploy

### Example 2: New CLI Command (Not Exposed)

**Change**: Add `zqk system metrics` command  
**Category**: Category 3 (No Update)  
**Decision**: **NO UPDATE**

**Reason**: Command not in `exposed_commands`, so MCP doesn't need it.

### Example 3: New CLI Command (Exposed)

**Change**: Add `zqk object search` command, add to `exposed_commands`  
**Category**: Category 2 (Should Update)  
**Decision**: **UPDATE**

**Process**:
1. Add command to codebase
2. Add to `.zqk/mcp/config.yaml` `exposed_commands`
3. Build new stable binary
4. Update version: `binary_version: "1.1.0"`
5. Test new command via MCP
6. Deploy

### Example 4: Logging Fix

**Change**: Fix logging to use stderr in MCP mode  
**Category**: Category 1 (Must Update)  
**Decision**: **UPDATE**

**Reason**: Fixes critical issue (stdout pollution breaks JSON-RPC).

### Example 5: Internal Refactoring

**Change**: Refactor object storage layer  
**Category**: Category 3 (No Update)  
**Decision**: **NO UPDATE**

**Reason**: Internal change, doesn't affect MCP interface.

## Monitoring

### After Update

Monitor for:
1. MCP server startup errors
2. JSON-RPC communication issues
3. Command execution failures
4. Performance degradation
5. Compatibility issues

### Rollback Plan

If issues occur:
1. Revert to previous stable binary
2. Update config to point to old version
3. Restart MCP server
4. Investigate issue
5. Fix and retry update

## Checklist

Before updating stable binary:

- [ ] Change is in Category 1 or 2
- [ ] All tests pass
- [ ] MCP server starts successfully
- [ ] All exposed commands work
- [ ] JSON-RPC communication works
- [ ] Version updated in config
- [ ] Documentation updated
- [ ] Compatibility verified
- [ ] Rollback plan ready

## Related Documentation

- [POL-ARCH-003](../policies/POL-ARCH-003.yaml): MCP Binary Stability Policy
- [MCP Binary Stability Architecture](./mcp-binary-stability-v1.0.md)
- [MCP Configuration Guide](../../onboarding/MCP_CONFIGURATION.md)
