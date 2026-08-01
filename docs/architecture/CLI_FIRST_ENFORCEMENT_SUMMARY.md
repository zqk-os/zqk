# CLI-First Enforcement Summary

**Version:** 1.0.0  
**Created:** 2025-12-31  
**Status:** Active  
**Purpose:** Summary of mechanisms to prevent AI agents from manipulating YAML directly

## Problem Statement

**Root Cause**: AI agents prefer to manipulate YAML files directly instead of using CLI/MCP tools, causing:
- Hash registry sync issues (18 Tier 2 violations in recent incident)
- Missing audit trails
- Cache inconsistencies
- Validation bypass
- System integrity violations

**Solution**: Multi-layered prevention and enforcement mechanisms

## Prevention Mechanisms Implemented

### 1. Enhanced Pre-Commit Hook ✅

**Location**: `tools/git-hooks/pre-commit`

**What it does**:
- Detects staged YAML files in object directories
- Runs system check to find missing hash registry entries
- **Blocks commit** if unregistered files detected
- Provides clear error messages with fix instructions

**Impact**: Prevents unregistered files from being committed

### 2. Comprehensive Documentation ✅

**Documents Created**:
1. **CLI vs Direct YAML Elicitation** (`CLI_VS_DIRECT_YAML_ELICITATION.md`)
   - Identifies CLI capabilities vs gaps
   - Documents valid direct YAML use cases
   - Lists backlog items for gaps

2. **AI Agent CLI-First Workflow** (`AI_AGENT_CLI_FIRST_WORKFLOW.md`)
   - Mandatory workflow for AI agents
   - MCP tool usage guide
   - CLI command reference
   - Exception process

3. **AI Agent YAML Edit Policy** (`AI_AGENT_YAML_EDIT_POLICY.md`)
   - Clear decision tree
   - Allowed vs prohibited operations
   - Quick reference guide

4. **MCP/CLI-First Elicitation** (`MCP_CLI_FIRST_ELICITATION.md`)
   - Elicitation prompt for AI agents
   - Decision matrix
   - Exception process

5. **Hash Registry Sync Analysis** (`HASH_REGISTRY_SYNC_ANALYSIS.md`)
   - Root cause analysis
   - Prevention mechanisms
   - Resolution workflow

**Impact**: Clear guidance for AI agents on when/how to use CLI

### 3. Updated AI Agent Onboarding ✅

**Location**: `docs/onboarding/AI_AGENT_ONBOARDING.md`

**Updates**:
- Emphasized MCP tool usage (preferred)
- Clear CLI command examples
- Mandatory workflow requirements
- Exception process documentation

**Impact**: New AI agents understand CLI-first approach from start

### 4. Helper Scripts ✅

**Location**: `scripts/create-object-from-file.sh`

**What it does**:
- Creates objects from YAML files via CLI
- Ensures hash registry is updated
- Verifies hash registry entry after creation

**Impact**: Makes CLI usage easier, reducing temptation for direct YAML

### 5. Backlog Items for Gaps ✅

**Created Backlog Items**:
- **BLI-851**: Object Rename Command (P1)
- **BLI-852**: Object Move Command (P2)
- **BLI-853**: Reference Management Commands (P1)
- **BLI-854**: Complex Field Update Support (P1)
- **BLI-855**: Conditional Update Logic (P2)
- **BLI-856**: Metadata-Only Update Operations (P3)

**Impact**: Identifies and prioritizes CLI functionality gaps

## Known CLI Gaps (Use Exception Process)

These operations currently require direct YAML or workarounds:

1. **Object Renaming** (BLI-851) - P1
2. **Object Moving** (BLI-852) - P2
3. **Reference Add/Remove** (BLI-853) - P1
4. **Complex Field Updates** (BLI-854) - P1
5. **Conditional Updates** (BLI-855) - P2
6. **Metadata-Only Updates** (BLI-856) - P3

## Enforcement Layers

### Layer 1: Pre-Commit Hook
- ✅ Detects unregistered files
- ✅ Blocks commits
- ✅ Provides fix instructions

### Layer 2: System Check
- ✅ Detects hash registry sync issues
- ✅ Reports Tier 2 violations
- ✅ Generates lifecycle reminders

### Layer 3: CI/CD
- ⚠️ Should fail on Tier 2 violations (to be implemented)
- ⚠️ Should provide actionable feedback (to be implemented)

### Layer 4: Documentation
- ✅ Clear policies and workflows
- ✅ Elicitation prompts
- ✅ Quick reference guides

## AI Agent Workflow (Mandatory)

### Before Any YAML Edit

1. **Check file type**:
   - Object file? → Use CLI/MCP
   - System metadata? → Direct YAML OK
   - Documentation? → Use docman-sync

2. **Check available tools**:
   ```bash
   tools/list  # See MCP tools available
   ```

3. **Use MCP tools** (preferred):
   ```bash
   zqk_object_create
   zqk_object_update
   zqk_object_delete
   ```

4. **Use CLI commands** (if MCP unavailable):
   ```bash
   zqk object create <kind> --file <file.yaml>
   zqk object update <id> --field <field>=<value>
   zqk object delete <id>
   ```

5. **Exception process** (if CLI gap):
   - Create backlog item
   - Use direct YAML (if necessary)
   - Register hash: `zqk system check <id> --auto-fix --force`
   - Document in commit

## Success Metrics

### Current State
- ✅ Pre-commit hook implemented
- ✅ Documentation complete
- ✅ Backlog items created
- ✅ Helper scripts available
- ✅ AI agent onboarding updated

### Target State
- **Zero Tier 2 violations** from unregistered files
- **Pre-commit hook** blocks all direct file creations
- **CI/CD** fails on hash registry sync issues
- **100% CLI usage** for object operations (except known gaps)
- **All gaps** have backlog items with priorities

## Next Steps

### Immediate
1. ✅ Enhanced pre-commit hook
2. ✅ Complete documentation
3. ✅ Create backlog items

### Short-Term
4. Enable MCP write operations for AI agents (with proper roles)
5. Implement high-priority backlog items (BLI-851, BLI-853, BLI-854)
6. Add CI/CD validation for Tier 2 violations

### Medium-Term
7. Implement remaining backlog items
8. Add file system watcher for real-time detection
9. Create periodic monitoring jobs

## Related Documentation

- [CLI vs Direct YAML Elicitation](./CLI_VS_DIRECT_YAML_ELICITATION.md)
- [AI Agent CLI-First Workflow](./AI_AGENT_CLI_FIRST_WORKFLOW.md)
- [AI Agent YAML Edit Policy](./AI_AGENT_YAML_EDIT_POLICY.md)
- [MCP/CLI-First Elicitation](./MCP_CLI_FIRST_ELICITATION.md)
- [Hash Registry Sync Analysis](./HASH_REGISTRY_SYNC_ANALYSIS.md)
- [CLI Normative Path v1.0](./cli-normative-path-v1.0.md)

---

**The system now has comprehensive prevention mechanisms to ensure AI agents use CLI/MCP instead of direct YAML manipulation.**

