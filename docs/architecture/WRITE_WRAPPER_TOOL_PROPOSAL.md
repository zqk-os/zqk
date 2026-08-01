# Write Wrapper Tool Proposal

**Version**: 1.0.0  
**Created**: 2026-01-08  
**Status**: Proposal  
**Purpose**: Create MCP wrapper tools that match `write` tool interface but route through CLI layer

## Problem

Agents prefer the `write` tool (Cursor IDE built-in) because it's simple:
- Single call: `write(file_path, content)`
- Immediate success signal
- No syntax knowledge required

Existing MCP tools like `zqk_object_create` require:
- Structured parameters (id, kind, fields)
- Understanding of object schema
- More complex interface

## Solution: Write-Like Wrapper Tools

Create MCP tools that match the `write` tool interface but route through the CLI layer:

### Tool: `zqk_write_object_file`

**Interface** (matches `write` tool):
```json
{
  "file_path": "docs/architecture/criteria/CRIT-9080.yaml",
  "content": "id: CRIT-9080\nkind: criteria\n..."
}
```

**Behavior**:
1. Detects if file path is in `docs/architecture/{kind}/` (object file)
2. If object file:
   - Parses YAML content to extract object data
   - Detects object kind from path or YAML
   - Routes through `zqk object create` via CLI
   - Ensures proper CAS, validation, audit trails
3. If not object file (system metadata):
   - Falls back to direct write (allowed for specs, configs)

**Advantages**:
- ✅ Same interface as `write` tool (low cognitive load)
- ✅ Routes through CLI layer (proper CAS, validation)
- ✅ Agents can use same pattern as `write`
- ✅ No schema knowledge required (just file path + YAML)

**Implementation**:
- Register as built-in MCP tool
- Handler detects object file patterns
- Parses YAML to extract object data
- Calls `zqk object create` via CLI bridge
- Returns success/error like `write` tool

## Alternative: Enhanced Object Create Tool

Instead of wrapper, enhance `zqk_object_create` to accept file_path + content:

### Tool: `zqk_object_create_from_file`

**Interface**:
```json
{
  "file_path": "docs/architecture/criteria/CRIT-9080.yaml",
  "content": "id: CRIT-9080\nkind: criteria\n...",
  "create_if_exists": false
}
```

**Behavior**:
- Parses YAML content
- Extracts object ID, kind, fields
- Routes through `zqk object create`
- Handles file path validation
- Creates object with proper CAS routing

**Advantages**:
- ✅ More explicit (object creation, not file write)
- ✅ Can validate object file patterns
- ✅ Routes through CLI layer
- ✅ Still simple interface (file_path + content)

**Disadvantages**:
- ❌ Name suggests "create" not "write" (agents might not think to use it)
- ❌ Slightly more complex than pure `write` interface

## Recommendation

**Create `zqk_write_object_file` wrapper tool** that:
1. Matches `write` tool interface exactly (file_path + content)
2. Detects object files automatically
3. Routes through CLI layer for object files
4. Falls back to direct write for system metadata (allowed)

This makes it as easy as `write` but ensures proper object creation.

## Implementation Steps

1. **Create handler function** `HandleWriteObjectFile`
   - Detects object file patterns
   - Parses YAML content
   - Routes through CLI for object files
   - Returns appropriate response

2. **Register as built-in tool**
   - Add to `RegisterCommonTools` or `RegisterWorkflowTools`
   - Register with clear description
   - Emphasize: "Use this instead of write tool for object files"

3. **Update documentation**
   - Update agent onboarding
   - Update workflow guides
   - Emphasize this as preferred method

4. **Tool description**
   ```
   Write object files with proper CLI routing. This tool provides the same 
   interface as the write tool (file_path + content) but routes object files 
   through the CLI layer to ensure proper CAS, validation, and audit trails.
   
   Use this tool instead of the write tool for object files in 
   docs/architecture/{kind}/ directories.
   
   For system metadata files (specs, configs), direct writes are allowed.
   ```

## Questions

1. **Should this tool intercept ALL writes to object directories?**
   - Yes: Ensures all object writes go through CLI
   - No: Allow direct writes but recommend this tool

2. **What about updates?**
   - Create `zqk_write_object_file` that detects create vs update?
   - Or create separate `zqk_update_object_file`?

3. **Error handling?**
   - Should it fail gracefully for non-object files?
   - Or route all writes through this tool?

4. **Tool name?**
   - `zqk_write_object_file` (clear, explicit)
   - `zqk_safe_write` (emphasizes safety)
   - `zqk_write_with_validation` (emphasizes validation)

## Next Steps

1. Design tool interface and behavior
2. Implement handler function
3. Register as built-in tool
4. Update documentation
5. Test with agents
