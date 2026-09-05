# Accounts, Roles, and Permission Hierarchy v1.0

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2026-01-01  
**Status:** Active  
**Purpose:** Reference document showing accounts, roles, and permission hierarchy in the system

**Generated:** 2026-01-01  
**Total Accounts:** 12  
**Total Roles:** 10

## Permission Hierarchy (Highest to Lowest)

### Tier 1: Full System Access (Executive Level)
**Influence Level:** `executive`

#### 1. **admin** (ROL-002)
- **Permissions:** `read:*`, `write:*`, `delete:*`
- **Description:** System administrator with unrestricted access
- **Accounts:**
  - `account:founder` (also has `founder` role)
  - `account:lanceettl` (also has `developer` role)

#### 2. **founder** (ROL-003)
- **Permissions:** `read:*`, `write:*`, `delete:*`
- **Description:** Project founder with executive authority (same as admin)
- **Accounts:**
  - `account:founder` (also has `admin` role)

---

### Tier 2: Strategic Decision Making (Executive Level)
**Influence Level:** `executive`

#### 3. **executive** (ROL-004)
- **Permissions:**
  - `read:*`
  - `read:confidential`
  - `write:strategic_plan`
  - `write:roadmap`
  - `write:goal`
  - `write:decision`
- **Description:** Executive-level decision maker with strategic planning access
- **Accounts:**
  - `account:executive`

---

### Tier 3: Project Ownership (Owner Level)
**Influence Level:** `owner`

#### 4. **owner** (ROL-005)
- **Permissions:**
  - `read:*`
  - `write:backlog_item`
  - `write:requirement`
  - `write:goal`
  - `write:workstream`
  - `write:milestone`
- **Description:** Project/component owner with authority over project management
- **Accounts:**
  - `account:owner`
  - `account:senior_dev` (also has `developer` role)
  - `account:team_alpha` (also has `collective` role)

---

### Tier 4: Development Access (Observer Level)
**Influence Level:** `observer`

#### 5. **developer** (ROL-006)
- **Permissions:** `read:*`, `write:*`
- **Description:** Software developer with full read and write access (temporary broad permission set)
- **Accounts:**
  - `account:cursor-vscode`
  - `account:developer`
  - `account:lanceettl` (also has `admin` role)
  - `account:senior_dev` (also has `owner` role)

---

### Tier 5: Read-Only Access (Observer Level)
**Influence Level:** `observer`

#### 6. **viewer** (ROL-007)
- **Permissions:** `read:*`
- **Description:** Read-only access for stakeholders and external reviewers
- **Accounts:**
  - `account:viewer`
  - **Note:** This is the default role for MCP connections (AI agents)

---

### Tier 6: Team/Collective (Collective Level)
**Influence Level:** `collective`

#### 7. **collective** (ROL-008)
- **Permissions:** `read:*`
- **Description:** Team or group with shared authority (permissions vary by team)
- **Accounts:**
  - `account:team_alpha` (also has `owner` role)

---

### Tier 7: Automation Agents (Automation Level)
**Influence Level:** `automation`

#### 8. **coder_agent** (ROL-010)
- **Permissions:**
  - `read:*`
  - `write:code`
  - `write:backlog_item`
  - `write:requirement`
  - `read:graph_node`
  - `read:graph_edge`
- **Description:** Coder agent that writes and modifies code. Requires observer agent and test agent to be ready, and governor approval for operations.
- **Accounts:**
  - `account:coder_agent`

#### 9. **observer_agent** (ROL-001)
- **Permissions:**
  - `read:*`
  - `read:code`
  - `read:ast`
  - `write:graph_node`
  - `write:graph_edge`
  - `read:graph_node`
  - `read:graph_edge`
- **Description:** Observer agent that ingests code and builds GraphRAG knowledge kernel
- **Accounts:**
  - `account:observer_agent`

#### 10. **test_agent** (ROL-009)
- **Permissions:**
  - `read:*`
  - `write:test_case`
  - `write:code`
  - `read:graph_node`
  - `read:graph_edge`
- **Description:** Test agent that automatically generates tests for code
- **Accounts:**
  - `account:test_agent`

---

## Complete Account Listing

### Accounts with Multiple Roles

1. **account:founder**
   - Roles: `admin`, `founder`
   - Highest tier: Full system access

2. **account:lanceettl**
   - Roles: `admin`, `developer`
   - Highest tier: Full system access

3. **account:senior_dev**
   - Roles: `developer`, `owner`
   - Highest tier: Project ownership + development access

4. **account:team_alpha**
   - Roles: `collective`, `owner`
   - Highest tier: Project ownership

### Single-Role Accounts

5. **account:coder_agent** → `coder_agent`
6. **account:cursor-vscode** → `developer`
7. **account:developer** → `developer`
8. **account:executive** → `executive`
9. **account:observer_agent** → `observer_agent`
10. **account:owner** → `owner`
11. **account:test_agent** → `test_agent`
12. **account:viewer** → `viewer`

---

## Permission Patterns

### Full Access Patterns
- **`read:*`, `write:*`, `delete:*`**: `admin`, `founder`
- **`read:*`, `write:*`**: `developer`

### Strategic/Planning Patterns
- **Strategic planning**: `executive` (strategic_plan, roadmap, goal, decision)
- **Project management**: `owner` (backlog_item, requirement, goal, workstream, milestone)

### Code/Development Patterns
- **Code writing**: `coder_agent`, `test_agent`, `developer`
- **Graph operations**: `observer_agent`, `coder_agent`, `test_agent` (graph_node, graph_edge)

### Read-Only Patterns
- **Full read access**: All roles have `read:*`
- **Read-only only**: `viewer`, `collective`

---

## Influence Level Hierarchy

1. **executive** → `admin`, `founder`, `executive`
2. **owner** → `owner`
3. **observer** → `developer`, `viewer`
4. **collective** → `collective`
5. **automation** → `coder_agent`, `observer_agent`, `test_agent`

---

## Key Observations

1. **Admin/Founder**: Identical permissions, both have full system access
2. **Developer**: Currently has broad `write:*` access (temporary - will be refined)
3. **MCP Default**: MCP connections default to `viewer` role (read-only)
4. **Agent Roles**: Three specialized agent roles with specific code/graph permissions
5. **Multi-Role Accounts**: Several accounts have multiple roles for flexible access
6. **Permission Granularity**: Most roles use wildcard permissions (`read:*`, `write:*`), with some roles having specific permissions (e.g., `write:code`, `write:backlog_item`)

---

## Permission Comparison Matrix

| Role | read:* | write:* | delete:* | Special Permissions |
|------|--------|----------|----------|---------------------|
| admin | ✅ | ✅ | ✅ | None (full access) |
| founder | ✅ | ✅ | ✅ | None (full access) |
| executive | ✅ | ❌ | ❌ | strategic_plan, roadmap, goal, decision, confidential |
| owner | ✅ | ❌ | ❌ | backlog_item, requirement, goal, workstream, milestone |
| developer | ✅ | ✅ | ❌ | None (broad write access) |
| viewer | ✅ | ❌ | ❌ | None (read-only) |
| collective | ✅ | ❌ | ❌ | None (read-only, team-based) |
| coder_agent | ✅ | ❌ | ❌ | code, backlog_item, requirement, graph_node, graph_edge |
| observer_agent | ✅ | ❌ | ❌ | code, ast, graph_node, graph_edge |
| test_agent | ✅ | ❌ | ❌ | test_case, code, graph_node, graph_edge |

---

## Recommendations

1. **Developer Role Refinement**: The `developer` role currently has broad `write:*` access. Consider refining to specific permissions like other roles.

2. **MCP Default Role**: MCP connections correctly default to `viewer` (read-only), which is appropriate for AI agents.

3. **Role Assignment**: Consider documenting which accounts should use which roles for MCP connections.

## Related Documentation

- [Multi-Agent Permission Issue Analysis](./multi-agent-permission-issue-analysis-v1.0.md)
- [Observer Agent Privileges Update](./observer-agent-privileges-update-v1.0.md)
- [MCP Role Elicitation](../MCP_ROLE_ELICITATION.md)
- [MCP Roles and Permissions](../MCP_ROLES_AND_PERMISSIONS.md)

4. **Permission Granularity**: Most roles use wildcard permissions. Consider if more granular permissions would improve security.

