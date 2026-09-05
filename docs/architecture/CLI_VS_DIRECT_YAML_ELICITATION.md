# CLI vs Direct YAML Elicitation

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2025-12-31  
**Status:** Active  
**Purpose:** Identify what operations MUST use CLI vs what can be done via direct YAML, and create backlog items for gaps

## Problem Statement

AI agents prefer to manipulate YAML files directly instead of using CLI/MCP tools. This causes:
- Hash registry sync issues
- Missing audit trails
- Cache inconsistencies
- Validation bypass
- System integrity violations

**Goal**: Ensure AI agents use CLI/MCP for all object operations, and only use direct YAML for operations the CLI doesn't support.

## Elicitation Process

### Step 1: Identify Current CLI Capabilities

**Question**: What object operations does the CLI currently support?

**Answer**: Check via:
```bash
zqk object --help
zqk system --help
zqk automation --help
```

**Current CLI Commands** (as of 2025-12-31):
- `zqk object create <id> --file <file.yaml>` - Create object from file
- `zqk object create <id> --field <field>=<value>` - Create object with fields
- `zqk object update <id> --file <file.yaml>` - Update object from file
- `zqk object update <id> --field <field>=<value>` - Update object field
- `zqk object delete <id>` - Delete object
- `zqk object get <id>` - Get object
- `zqk object list` - List objects
- `zqk object count` - Count objects
- `zqk object neighbors <id>` - Get object neighbors
- `zqk object related <id>` - Get related objects

### Step 2: Identify MCP Tool Availability

**Question**: What MCP tools are available for object operations?

**Answer**: MCP server exposes CLI commands as tools:
- `zqk_object_create` - Create object (if write_operations enabled)
- `zqk_object_update` - Update object (if write_operations enabled)
- `zqk_object_delete` - Delete object (if write_operations enabled)
- `zqk_object_get` - Get object
- `zqk_object_list` - List objects
- `zqk_object_count` - Count objects
- `zqk_object_neighbors` - Get neighbors
- `zqk_object_related` - Get related objects

**Note**: Write operations require:
1. Role with write permissions
2. `write_operations` enabled in MCP config
3. Command in `exposed_commands` or `write_operations` list

### Step 3: Identify Direct YAML Use Cases

**Question**: What operations currently require direct YAML manipulation?

**Answer**: Analyze common AI agent workflows:

#### Currently Requiring Direct YAML (GAPS):

1. **Complex Field Updates**
   - **Current**: `--field` only supports simple values (strings, numbers, booleans)
   - **Gap**: No support for nested objects, arrays with complex values via `--field`
   - **Workaround**: Use `--file` with full object (works but verbose)
   - **Backlog Item**: BLI-COMPLEX-FIELD-001
   - **Priority**: High (common operation)

2. **Object Renaming**
   - **Current**: No rename command
   - **Gap**: Must delete and recreate (loses history, breaks references)
   - **Workaround**: Manual file move + hash registry update (risky)
   - **Backlog Item**: BLI-OBJECT-RENAME-001
   - **Priority**: High (needed for corrections)

3. **Object Moving (Directory Changes)**
   - **Current**: No move command
   - **Gap**: Must delete and recreate in new location (loses history)
   - **Workaround**: Manual file move + hash registry update (risky)
   - **Backlog Item**: BLI-OBJECT-MOVE-001
   - **Priority**: Medium (organization changes)

4. **Reference Management**
   - **Current**: Must manually manage reference arrays via `--field` with full array
   - **Gap**: No add/remove reference commands (e.g., `--add-ref`, `--remove-ref`)
   - **Workaround**: Update entire `*_refs` array (error-prone)
   - **Backlog Item**: BLI-REFERENCE-MANAGE-001
   - **Priority**: High (very common operation)

5. **Conditional Updates**
   - **Current**: No conditional update logic
   - **Gap**: Must read, modify, update manually (multiple steps)
   - **Workaround**: Use `--file` with modified object
   - **Backlog Item**: BLI-CONDITIONAL-UPDATE-001
   - **Priority**: Medium (advanced feature)

6. **Metadata-Only Operations**
   - **Current**: All operations update `updated_at` and `updated_by`
   - **Gap**: No way to update metadata without touching content
   - **Workaround**: Direct YAML edit (bypasses CLI)
   - **Backlog Item**: BLI-METADATA-ONLY-001
   - **Priority**: Low (edge case)

#### Already Supported (No Gap):

1. **Bulk Object Creation** ✅
   - **CLI Command**: `zqk object bulk create <kind> --file <items.yaml>`
   - **Status**: Available
   - **Note**: Use this instead of creating files individually

2. **Template-Based Creation** ✅
   - **CLI Command**: `zqk object template <kind>`
   - **Status**: Available
   - **Note**: Use this to generate templates, then create via CLI

3. **Schema Inspection** ✅
   - **CLI Command**: `zqk object fields <kind>`
   - **Status**: Available
   - **Note**: Use this to see available fields before creating

### Step 4: Identify Valid Direct YAML Use Cases

**Question**: What operations SHOULD be done via direct YAML (CLI doesn't handle)?

**Answer**: Very few operations should bypass CLI:

1. **Spec Files** (`docs/process/_internal/object_specs/`)
   - **Reason**: System metadata, not user objects
   - **Action**: CLI doesn't manage these
   - **Status**: ✅ Valid direct YAML

2. **Lifecycle Definitions** (`docs/process/_internal/lifecycles/`)
   - **Reason**: System metadata, not user objects
   - **Action**: CLI doesn't manage these
   - **Status**: ✅ Valid direct YAML

3. **Configuration Files** (`.zqk/config.yaml`, `.zqk/mcp/config.yaml`)
   - **Reason**: System configuration, not objects
   - **Action**: CLI doesn't manage these
   - **Status**: ✅ Valid direct YAML

4. **Documentation Files** (Markdown files in `docs/`)
   - **Reason**: Documentation, not objects (but should be registered)
   - **Action**: Use `zqk automation docman-sync` to register
   - **Status**: ⚠️ Should use docman-sync, not direct YAML

5. **Test Files** (Test fixtures, not object files)
   - **Reason**: Test data, not production objects
   - **Action**: CLI doesn't manage these
   - **Status**: ✅ Valid direct YAML

### Step 5: Create Backlog Items for Gaps

**Action**: Create backlog items for each identified gap to enable CLI-first workflow.

## AI Agent Workflow Requirements

### Mandatory: Use CLI/MCP for Object Operations

**For ALL object operations, use CLI/MCP tools:**

1. **Creating Objects**
   ```bash
   # ✅ CORRECT: Use MCP tool or CLI
   zqk_object_create with id="ROL-011", kind="role", fields={...}
   # OR
   zqk object create ROL-011 --file role.yaml
   
   # ❌ INCORRECT: Direct file creation
   # Writing YAML file directly
   ```

2. **Updating Objects**
   ```bash
   # ✅ CORRECT: Use MCP tool or CLI
   zqk_object_update with id="ROL-011", field="title", value="New Title"
   # OR
   zqk object update ROL-011 --field title="New Title"
   
   # ❌ INCORRECT: Direct file edit
   # Editing YAML file directly
   ```

3. **Deleting Objects**
   ```bash
   # ✅ CORRECT: Use MCP tool or CLI
   zqk_object_delete with id="ROL-011"
   # OR
   zqk object delete ROL-011
   
   # ❌ INCORRECT: Direct file deletion
   # rm docs/process/roles/ROL-011.yaml
   ```

### Exception: Only When CLI Doesn't Support

**Direct YAML manipulation is ONLY allowed when:**
1. CLI doesn't support the operation (see gaps above)
2. Operation is on system metadata (specs, lifecycles, config)
3. Operation is on test fixtures
4. **AND** you immediately create a backlog item to add CLI support

**Process for Exception Cases:**
1. Identify the gap (operation CLI doesn't support)
2. Create backlog item for the gap
3. Perform direct YAML operation (if absolutely necessary)
4. Register hash manually: `zqk system check <id> --auto-fix --force`
5. Document the exception in the backlog item

## Elicitation Checklist

Before editing any YAML file, ask:

- [ ] **Is this an object file?** (in `docs/process/{kind}/`)
  - [ ] **YES**: Use CLI/MCP tool - **STOP, don't edit directly**
  - [ ] **NO**: Continue to next question

- [ ] **Is this a system metadata file?** (specs, lifecycles, config)
  - [ ] **YES**: Direct YAML is OK - **Proceed**
  - [ ] **NO**: Continue to next question

- [ ] **Is this a documentation file?** (Markdown)
  - [ ] **YES**: Use `zqk automation docman-sync` - **Use CLI**
  - [ ] **NO**: Continue to next question

- [ ] **Is this a test fixture?**
  - [ ] **YES**: Direct YAML is OK - **Proceed**
  - [ ] **NO**: **STOP - Use CLI/MCP**

## Backlog Items to Create

Based on gap analysis, create these backlog items:

1. **BLI-BULK-CREATE-001**: Batch object creation command
2. **BLI-COMPLEX-FIELD-001**: Support complex/nested field updates
3. **BLI-OBJECT-RENAME-001**: Object rename command (preserves history)
4. **BLI-OBJECT-MOVE-001**: Object move command (directory changes)
5. **BLI-OBJECT-TEMPLATE-001**: Template-based object creation
6. **BLI-CONDITIONAL-UPDATE-001**: Conditional update logic
7. **BLI-REFERENCE-MANAGE-001**: Reference add/remove commands
8. **BLI-METADATA-ONLY-001**: Metadata-only update operations

## Implementation Priority

### High Priority (Blocks Common Workflows)
1. **BLI-REFERENCE-MANAGE-001** - Reference management is common
2. **BLI-COMPLEX-FIELD-001** - Complex updates are frequent
3. **BLI-OBJECT-RENAME-001** - Renaming is needed for corrections

### Medium Priority (Improves Workflow)
4. **BLI-BULK-CREATE-001** - Batch operations are useful
5. **BLI-OBJECT-MOVE-001** - Organization changes
6. **BLI-OBJECT-TEMPLATE-001** - Faster object creation

### Low Priority (Nice to Have)
7. **BLI-CONDITIONAL-UPDATE-001** - Advanced feature
8. **BLI-METADATA-ONLY-001** - Edge case

## Related Documentation

- [CLI Normative Path v1.0](./cli-normative-path-v1.0.md)
- [Hash Registry Sync Analysis](./HASH_REGISTRY_SYNC_ANALYSIS.md)
- [AI Agent Onboarding](../onboarding/AI_AGENT_ONBOARDING.md)
- [MCP Roles and Permissions](./MCP_ROLES_AND_PERMISSIONS.md)

---

*This elicitation should be performed whenever new gaps are identified or when AI agents find themselves needing to edit YAML directly.*

