# AI Agent CLI-First Workflow

**Version:** 1.0.0  
**Created:** 2025-12-31  
**Status:** Active  
**Purpose:** Mandatory workflow for AI agents to use CLI/MCP instead of direct YAML manipulation

## Core Principle

**ALL object operations MUST use CLI/MCP tools. Direct YAML manipulation is PROHIBITED except for system metadata files.**

## Mandatory Workflow

### Before Any YAML Edit

**STOP and ask yourself:**

1. **Is this an object file?** (in `docs/architecture/{kind}/`)
   - **YES**: ❌ **STOP** - Use CLI/MCP tool instead
   - **NO**: Continue

2. **Is this a system metadata file?** (specs, lifecycles, config)
   - **YES**: ✅ Direct YAML is OK
   - **NO**: Continue

3. **Is this a documentation file?** (Markdown)
   - **YES**: Use `zqk automation docman-sync`
   - **NO**: ❌ **STOP** - Use CLI/MCP

### Object Operations: Use MCP Tools

**For ALL object operations, use MCP tools (preferred) or CLI commands:**

#### Creating Objects

```bash
# ✅ CORRECT: Use MCP tool
zqk_object_create with:
  id: "ROL-011"
  kind: "role"
  fields:
    title: "New Role"
    description: "Role description"
    role_id: "new_role"
    permissions: ["read:*"]

# ✅ ALSO CORRECT: Use CLI command
zqk object create ROL-011 --file role.yaml

# ❌ INCORRECT: Direct file creation
# Writing YAML file directly - PROHIBITED
```

#### Updating Objects

```bash
# ✅ CORRECT: Use MCP tool
zqk_object_update with:
  id: "ROL-011"
  field: "title"
  value: "Updated Title"

# ✅ ALSO CORRECT: Use CLI command
zqk object update ROL-011 --field title="Updated Title"

# ✅ ALSO CORRECT: Complex updates via file
zqk object update ROL-011 --file updates.yaml

# ❌ INCORRECT: Direct file edit
# Editing YAML file directly - PROHIBITED
```

#### Deleting Objects

```bash
# ✅ CORRECT: Use MCP tool
zqk_object_delete with id="ROL-011"

# ✅ ALSO CORRECT: Use CLI command
zqk object delete ROL-011

# ❌ INCORRECT: Direct file deletion
# rm docs/architecture/roles/ROL-011.yaml - PROHIBITED
```

#### Bulk Operations

```bash
# ✅ CORRECT: Use bulk command
zqk object bulk create role --file roles.yaml

# ❌ INCORRECT: Creating multiple files individually
```

#### Template Generation

```bash
# ✅ CORRECT: Use template command
zqk object template role > role-template.yaml
# Edit template, then create via CLI

# ❌ INCORRECT: Copying existing object and editing directly
```

### Checking Available Tools

**Before performing any operation, check what tools are available:**

```bash
# Use MCP tools/list to see available tools
tools/list

# Look for:
# - zqk_object_create (if you have write permissions)
# - zqk_object_update (if you have write permissions)
# - zqk_object_delete (if you have write permissions)
```

### When MCP Tools Are Not Available

**If MCP write tools are not available:**

1. **Check your role and permissions**
   - Use `prompts/get` with name="role_based_access"
   - Verify you have write permissions for the object kind

2. **Check MCP server configuration**
   - Write operations must be enabled in `.zqk/mcp/config.yaml`
   - Commands must be in `write_operations` or `exposed_commands`

3. **Use CLI directly** (if MCP tools unavailable)
   ```bash
   zqk object create <id> --file <file.yaml>
   ```

4. **Request access** (if needed)
   - Create backlog item to enable write operations
   - Document why write access is needed

## Exception Process

**Direct YAML manipulation is ONLY allowed when:**

1. ✅ **System metadata files** (specs, lifecycles, config)
2. ✅ **Test fixtures** (test data, not production objects)
3. ⚠️ **CLI gap exists** (operation not supported by CLI)
   - **AND** you immediately create a backlog item
   - **AND** you register hash manually after: `zqk system check <id> --auto-fix --force`

### Process for CLI Gap Exception

1. **Identify the gap**: Operation CLI doesn't support
2. **Create backlog item**: Document the gap and required functionality
3. **Perform direct YAML** (if absolutely necessary)
4. **Register hash**: `zqk system check <id> --auto-fix --force`
5. **Document exception**: Reference backlog item in commit message

## Common Operations Reference

### Creating Objects

**MCP Tool** (preferred):
```
zqk_object_create
  id: string
  kind: string
  fields: object
```

**CLI Command**:
```bash
zqk object create <id> --file <file.yaml>
zqk object create <id> --field <field>=<value> [--field ...]
```

**Bulk Create**:
```bash
zqk object bulk create <kind> --file <items.yaml>
```

### Updating Objects

**MCP Tool** (preferred):
```
zqk_object_update
  id: string
  field?: string
  value?: any
  file?: string
```

**CLI Command**:
```bash
zqk object update <id> --field <field>=<value>
zqk object update <id> --file <updates.yaml>
```

**Bulk Update**:
```bash
zqk object bulk update --file <updates.yaml>
```

### Deleting Objects

**MCP Tool** (preferred):
```
zqk_object_delete
  id: string
```

**CLI Command**:
```bash
zqk object delete <id>
```

### Querying Objects

**MCP Tools**:
```
zqk_object_get
zqk_object_list
zqk_object_count
zqk_object_neighbors
zqk_object_related
```

**CLI Commands**:
```bash
zqk object get <id>
zqk object list <kind> [--filter ...]
zqk object count <kind>
zqk object neighbors <id>
zqk object related <id>
```

## Known Gaps (Use Exception Process)

These operations currently require direct YAML or workarounds:

1. **Object Renaming** - Create backlog item: BLI-OBJECT-RENAME-001
2. **Object Moving** - Create backlog item: BLI-OBJECT-MOVE-001
3. **Reference Add/Remove** - Create backlog item: BLI-REFERENCE-MANAGE-001
4. **Complex Field Updates** - Create backlog item: BLI-COMPLEX-FIELD-001
5. **Conditional Updates** - Create backlog item: BLI-CONDITIONAL-UPDATE-001
6. **Metadata-Only Updates** - Create backlog item: BLI-METADATA-ONLY-001

## Enforcement

### Pre-Commit Hook

The pre-commit hook will:
- ✅ Detect files missing from hash registry
- ✅ Block commits with unregistered object files
- ✅ Provide fix instructions

### System Check

System check will:
- ✅ Detect hash registry sync issues
- ✅ Report Tier 2 violations
- ✅ Generate lifecycle reminders

### CI/CD

CI/CD will:
- ✅ Run system check
- ✅ Fail on Tier 2 violations (configurable threshold)
- ✅ Provide actionable feedback

## Benefits

Using CLI/MCP ensures:
- ✅ Hash registry stays synchronized
- ✅ Audit trails are complete
- ✅ Cache remains accurate
- ✅ Validation is performed
- ✅ System integrity is maintained
- ✅ Role-based security is enforced

## Related Documentation

- [CLI vs Direct YAML Elicitation](./CLI_VS_DIRECT_YAML_ELICITATION.md)
- [CLI Normative Path v1.0](./cli-normative-path-v1.0.md)
- [Hash Registry Sync Analysis](./HASH_REGISTRY_SYNC_ANALYSIS.md)
- [MCP Roles and Permissions](./MCP_ROLES_AND_PERMISSIONS.md)
- [AI Agent Onboarding](../onboarding/AI_AGENT_ONBOARDING.md)

---

**Remember**: The CLI is the normative path. Direct YAML is the exception, not the rule.

