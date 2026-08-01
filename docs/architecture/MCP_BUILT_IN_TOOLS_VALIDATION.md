# MCP Built-In Tools Validation

**Version:** 1.0.0  
**Created:** 2025-12-31  
**Status:** Active  
**Purpose:** Validation checklist for built-in MCP tools

## Expected Built-In Tools (9 total)

### Graph Tools (3)
1. ✅ `zqk_graph_traversal` (also `graph_traversal`)
2. ✅ `zqk_resolve_references` (also `resolve_references`)
3. ✅ `zqk_state_aware_query` (also `state_aware_query`)

### Common CLI Tools (5)
4. ✅ `zqk_object_list` (also `object_list`)
5. ✅ `zqk_object_get` (also `object_get`)
6. ✅ `zqk_object_count` (also `object_count`)
7. ✅ `zqk_system_status` (also `system_status`)
8. ✅ `zqk_system_check` (also `system_check`)

### Test/Debug Tools (1)
9. ✅ `zqk_test_echo` (also `test_echo`, `echo`)

## Validation Checklist

### ✅ Registration
- [x] All tools registered in `RegisterAllTools()`
- [x] Graph tools registered in `RegisterGraphTools()`
- [x] Common tools registered in `RegisterCommonTools()`
- [x] Echo tool registered in `RegisterEchoTool()`

### ✅ Handler Implementation
- [x] Graph tools have handlers in `handlers_graph.go`
- [x] Common tools have handlers in `tools_common.go`
- [x] Echo tool has handler in `tools_echo.go`
- [x] All handlers added to switch statement in `server.go`

### ✅ Tool Counting
- [x] Built-in tools map includes all tool names (with and without `zqk_` prefix)
- [x] Counting logic correctly identifies built-in vs CLI tools
- [x] Trace logs show accurate tool counts

### ✅ Backward Compatibility
- [x] All tools support both `zqk_` prefixed and non-prefixed names
- [x] Switch statement handles both name variants

### ✅ Documentation
- [x] All tools documented in `MCP_BUILT_IN_TOOLS.md`
- [x] Tool descriptions include examples
- [x] Parameters documented with types and defaults

## Expected Behavior

### Always Available
- ✅ Built-in tools bypass `exposed_commands` whitelist
- ✅ Built-in tools available regardless of config
- ✅ Built-in tools not filtered by RBAC (except write operations)

### Tool Counts
When MCP server initializes with admin role:
- **Expected**: ~9 built-in tools + ~40-50 CLI tools = ~50 total tools
- **Actual**: Check trace log for "Successfully bootstrapped CLI tools: X total tools (Y CLI + Z built-in)"

### Admin Bypass
- ✅ Admin users see all CLI tools (bypass `exposed_commands` whitelist)
- ✅ Built-in tools always available to all users

## Validation Steps

1. **Check Trace Log**:
   ```
   grep "Successfully bootstrapped" .zqk/mcp/logs/mcp-trace.log
   ```
   Should show: `X total tools (Y CLI + 9 built-in)` (or close to 9)

2. **Verify Tool Names**:
   - All tools should be registered with `zqk_` prefix
   - Switch statement should handle both prefixed and non-prefixed names

3. **Test Tool Execution**:
   - Test each built-in tool via MCP
   - Verify handlers are called correctly
   - Verify CLI commands execute properly

4. **Check Config Bypass**:
   - Remove tools from `exposed_commands` in config
   - Verify built-in tools still available
   - Verify CLI tools filtered correctly

## Known Issues

### Issue: Tool Count Shows 0 Built-In
**Status**: ✅ Fixed  
**Cause**: Counting logic was checking for wrong tool names  
**Fix**: Updated `builtInTools` map to include both `zqk_` prefixed and non-prefixed names

## Next Steps

1. Monitor tool usage patterns
2. Identify additional candidates for built-in tools
3. Quarterly review per POL-MCP-001
4. Update documentation as tools evolve

