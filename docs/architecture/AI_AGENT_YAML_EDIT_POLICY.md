# AI Agent YAML Edit Policy

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2025-12-31  
**Status:** Active  
**Purpose:** Clear policy on when AI agents can edit YAML directly vs when they must use CLI/MCP

## Core Policy

**Direct YAML manipulation is PROHIBITED for object files. Use CLI/MCP tools for ALL object operations.**

## Decision Tree

### Step 1: Identify File Type

```
Is the file in docs/process/{kind}/? (e.g., roles/, accounts/, backlog/)
├─ YES → This is an OBJECT FILE
│   └─ ❌ PROHIBITED: Use CLI/MCP tool instead
│
└─ NO → Continue to Step 2
```

### Step 2: Identify System Metadata

```
Is the file system metadata?
├─ Spec files (docs/process/_internal/object_specs/)
├─ Lifecycle files (docs/process/_internal/lifecycles/)
├─ Config files (.zqk/*.yaml)
└─ Test fixtures (test data, not production)
│
├─ YES → ✅ ALLOWED: Direct YAML is OK
│
└─ NO → Continue to Step 3
```

### Step 3: Identify Documentation

```
Is the file documentation? (Markdown files)
├─ YES → ⚠️ Use docman-sync: zqk automation docman-sync
│
└─ NO → ❌ PROHIBITED: Use CLI/MCP
```

## Allowed Direct YAML Operations

### ✅ System Metadata Files

**These files are managed outside the CLI:**

1. **Object Specs** (`docs/process/_internal/object_specs/`)
   - System metadata defining object structure
   - Not user objects
   - Direct YAML is OK

2. **Lifecycle Definitions** (`docs/process/_internal/lifecycles/`)
   - System metadata defining state transitions
   - Not user objects
   - Direct YAML is OK

3. **Configuration Files** (`.zqk/config.yaml`, `.zqk/mcp/config.yaml`)
   - System configuration
   - Not user objects
   - Direct YAML is OK

4. **Test Fixtures** (Test data files)
   - Test data, not production objects
   - Direct YAML is OK

### ⚠️ Documentation Files (Use docman-sync)

**Markdown files should be registered:**

- Use `zqk automation docman-sync` to register
- Direct YAML is OK for content, but register via CLI

## Prohibited Direct YAML Operations

### ❌ Object Files (MUST use CLI/MCP)

**ALL files in object directories MUST use CLI/MCP:**

- `docs/process/roles/` - Use `zqk object create/update/delete role`
- `docs/process/accounts/` - Use `zqk object create/update/delete account`
- `docs/process/backlog/` - Use `zqk object create/update/delete backlog_item`
- `docs/process/criteria/` - Use `zqk object create/update/delete criteria`
- `docs/process/requirements/` - Use `zqk object create/update/delete requirement`
- `docs/process/tests/` - Use `zqk object create/update/delete test_case`
- `docs/process/policies/` - Use `zqk object create/update/delete policy`
- `docs/process/decisions/` - Use `zqk object create/update/delete decision`
- `docs/process/questions/` - Use `zqk object create/update/delete question`
- `docs/process/missions/` - Use `zqk object create/update/delete mission`
- `docs/process/visions/` - Use `zqk object create/update/delete vision`
- `docs/process/roadmaps/` - Use `zqk object create/update/delete roadmap`
- `docs/process/strategic_plans/` - Use `zqk object create/update/delete strategic_plan`
- `docs/process/goals/` - Use `zqk object create/update/delete goal`
- `docs/process/milestones/` - Use `zqk object create/update/delete milestone`
- `docs/process/workstreams/` - Use `zqk object create/update/delete workstream`
- `docs/process/backlog_items/` - Use `zqk object create/update/delete backlog_item`
- `docs/process/components/` - Use `zqk object create/update/delete component`
- `docs/process/audit/` - Use audit event creation (automated)
- `docs/process/change_journal/` - Use change journal creation (automated)

## Exception Process

**If CLI doesn't support an operation:**

1. **Identify the gap**: Document what CLI operation is missing
2. **Create backlog item**: Create BLI-XXX for the gap
3. **Use direct YAML** (if absolutely necessary):
   - Perform the operation
   - Register hash: `zqk system check <id> --auto-fix --force`
4. **Document exception**: Reference backlog item in commit message

**Known Gaps** (use exception process):
- Object renaming (BLI-851)
- Object moving (BLI-852)
- Reference add/remove (BLI-853)
- Complex field updates (BLI-854)
- Conditional updates (BLI-855)
- Metadata-only updates (BLI-856)

## Enforcement

### Pre-Commit Hook

- ✅ Detects unregistered object files
- ✅ Blocks commits with missing hash registry entries
- ✅ Provides fix instructions

### System Check

- ✅ Detects hash registry sync issues
- ✅ Reports Tier 2 violations
- ✅ Generates lifecycle reminders

## Quick Reference

### Creating Objects

```bash
# ✅ CORRECT: Use MCP tool
zqk_object_create with id="ROL-011", kind="role", fields={...}

# ✅ ALSO CORRECT: Use CLI
zqk object create role --file role.yaml

# ❌ INCORRECT: Direct file creation
# Writing YAML file directly
```

### Updating Objects

```bash
# ✅ CORRECT: Use MCP tool
zqk_object_update with id="ROL-011", field="title", value="New Title"

# ✅ ALSO CORRECT: Use CLI
zqk object update ROL-011 --field title="New Title"

# ❌ INCORRECT: Direct file edit
# Editing YAML file directly
```

### Deleting Objects

```bash
# ✅ CORRECT: Use MCP tool
zqk_object_delete with id="ROL-011"

# ✅ ALSO CORRECT: Use CLI
zqk object delete ROL-011

# ❌ INCORRECT: Direct file deletion
# rm docs/process/roles/ROL-011.yaml
```

## Related Documentation

- [AI Agent CLI-First Workflow](./AI_AGENT_CLI_FIRST_WORKFLOW.md)
- [CLI vs Direct YAML Elicitation](./CLI_VS_DIRECT_YAML_ELICITATION.md)
- [CLI Normative Path v1.0](./cli-normative-path-v1.0.md)
- [Hash Registry Sync Analysis](./HASH_REGISTRY_SYNC_ANALYSIS.md)

---

**Remember**: When in doubt, use CLI/MCP. Direct YAML is the exception, not the rule.

