# MCP/CLI-First Elicitation Prompt

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2025-12-31  
**Status:** Active  
**Purpose:** Elicitation prompt to identify CLI gaps and ensure AI agents use MCP/CLI instead of direct YAML

## Elicitation Prompt for AI Agents

**Use this prompt when you find yourself wanting to edit YAML files directly:**

---

### Pre-YAML-Edit Elicitation

**Before editing any YAML file, answer these questions:**

1. **What operation are you trying to perform?**
   - [ ] Create a new object
   - [ ] Update an existing object
   - [ ] Delete an object
   - [ ] Rename an object
   - [ ] Move an object
   - [ ] Add/remove a reference
   - [ ] Update complex/nested fields
   - [ ] Other: _______________

2. **What is the file path?**
   - Path: `_________________`
   - Is it in `docs/process/{kind}/`? [ ] Yes [ ] No
   - Is it system metadata? (specs, lifecycles, config) [ ] Yes [ ] No

3. **Can you use MCP tools?**
   - [ ] Yes - Use `zqk_object_create`, `zqk_object_update`, or `zqk_object_delete`
   - [ ] No - MCP tools not available (check `tools/list`)
   - [ ] No - Don't have write permissions

4. **Can you use CLI commands?**
   - [ ] Yes - Use `zqk object create/update/delete`
   - [ ] No - CLI doesn't support this operation
   - [ ] No - Operation requires direct YAML

5. **If CLI doesn't support it, what's missing?**
   - Gap description: `_________________`
   - Create backlog item: BLI-XXX
   - Use exception process (see below)

---

## Decision Matrix

| Operation | MCP Tool | CLI Command | Direct YAML |
|-----------|----------|-------------|-------------|
| Create object | ✅ `zqk_object_create` | ✅ `zqk object create <kind> --file` | ❌ PROHIBITED |
| Update object | ✅ `zqk_object_update` | ✅ `zqk object update <id> --field` | ❌ PROHIBITED |
| Delete object | ✅ `zqk_object_delete` | ✅ `zqk object delete <id>` | ❌ PROHIBITED |
| Bulk create | ❌ Not available | ✅ `zqk object bulk create` | ❌ PROHIBITED |
| Template | ❌ Not available | ✅ `zqk object template <kind>` | ❌ PROHIBITED |
| Rename object | ❌ Not available | ❌ Not available | ⚠️ Exception (BLI-851) |
| Move object | ❌ Not available | ❌ Not available | ⚠️ Exception (BLI-852) |
| Add reference | ❌ Not available | ❌ Not available | ⚠️ Exception (BLI-853) |
| Remove reference | ❌ Not available | ❌ Not available | ⚠️ Exception (BLI-853) |
| Complex field update | ⚠️ Limited | ⚠️ Use `--file` | ⚠️ Exception (BLI-854) |
| Conditional update | ❌ Not available | ❌ Not available | ⚠️ Exception (BLI-855) |
| Metadata-only | ❌ Not available | ❌ Not available | ⚠️ Exception (BLI-856) |

## Exception Process

**If you must use direct YAML (CLI gap exists):**

1. **Document the gap**:
   - What operation is missing?
   - Why is it needed?
   - What would the CLI command look like?

2. **Create backlog item**:
   - Use template: `zqk object template backlog_item`
   - Create item documenting the gap
   - Reference existing backlog items if similar:
     - BLI-851: Object Rename Command
     - BLI-852: Object Move Command
     - BLI-853: Reference Management Commands
     - BLI-854: Complex Field Update Support
     - BLI-855: Conditional Update Logic
     - BLI-856: Metadata-Only Update Operations

3. **Perform direct YAML** (if absolutely necessary):
   - Make the change
   - Register hash: `zqk system check <id> --auto-fix --force`

4. **Document in commit**:
   - Reference backlog item in commit message
   - Explain why direct YAML was necessary

## Quick Reference: Available Operations

### ✅ Use MCP Tools (Preferred)

```bash
# Check available tools
tools/list

# Create object
zqk_object_create with:
  id: "ROL-011"
  kind: "role"
  fields: {...}

# Update object
zqk_object_update with:
  id: "ROL-011"
  field: "title"
  value: "New Title"

# Delete object
zqk_object_delete with id="ROL-011"
```

### ✅ Use CLI Commands (Alternative)

```bash
# Create object
zqk object create role --file role.yaml
zqk object create role --field title="Role Title"

# Update object
zqk object update ROL-011 --field title="New Title"
zqk object update ROL-011 --file updates.yaml

# Delete object
zqk object delete ROL-011

# Bulk operations
zqk object bulk create role --file roles.yaml
zqk object bulk update --file updates.yaml

# Template generation
zqk object template role --output template.yaml
```

### ⚠️ Exception: Direct YAML (Only When CLI Gap Exists)

```bash
# 1. Create backlog item for gap
zqk object create backlog_item --file gap-item.yaml

# 2. Perform direct YAML edit
# (Edit file directly)

# 3. Register hash
zqk system check <id> --auto-fix --force

# 4. Document in commit
git commit -m "Fix: <description> (Exception: BLI-XXX)"
```

## Related Documentation

- [AI Agent CLI-First Workflow](./AI_AGENT_CLI_FIRST_WORKFLOW.md)
- [AI Agent YAML Edit Policy](./AI_AGENT_YAML_EDIT_POLICY.md)
- [CLI vs Direct YAML Elicitation](./CLI_VS_DIRECT_YAML_ELICITATION.md)
- [CLI Normative Path v1.0](./cli-normative-path-v1.0.md)

---

**Use this elicitation every time you consider editing YAML. It will help identify gaps and ensure CLI-first workflow.**

