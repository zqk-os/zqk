# MCP Roles and Permissions

**Last Verified:** 2026-08-31


**Status**: Draft  
**Date**: 2025-01-01  
**Purpose**: Define the role hierarchy and permission structure for MCP command access control

## Role Hierarchy

### System Roles (Highest Authority)

#### `admin`
- **Description**: Full system administrator with unrestricted access
- **Permissions**: `read:*`, `write:*`, `delete:*`
- **Use Cases**: System operations, configuration, emergency access
- **Default Context**: System operations (`ACC-*` admin/system account; not `account:*`)
- **Access**: All commands, all objects, all fields

#### `founder`
- **Description**: Project founder with executive authority
- **Permissions**: `read:*`, `write:*`, `delete:*` (same as admin, but distinct role)
- **Use Cases**: Project ownership, strategic decisions
- **Access**: All commands, all objects, all fields

### Human Roles

#### `executive`
- **Description**: Executive-level decision maker
- **Influence Level**: executive
- **Permissions**: 
  - `read:*`
  - `read:confidential`
  - `write:strategic_plan`
  - `write:roadmap`
  - `write:goal`
  - `write:decision`
- **Use Cases**: Strategic planning, high-level decisions, confidential data access
- **Access**: Read all, write strategic objects, access confidential fields

#### `owner`
- **Description**: Project/component owner
- **Influence Level**: owner
- **Permissions**:
  - `read:*`
  - `write:backlog_item`
  - `write:requirement`
  - `write:goal`
  - `write:workstream`
  - `write:milestone`
- **Use Cases**: Project management, backlog management, requirements definition
- **Access**: Read all, write project management objects

#### `developer`
- **Description**: Software developer
- **Permissions**:
  - `read:*`
  - `write:code`
  - `write:test_case`
  - `write:backlog_item` (status updates)
  - `read:code`
- **Use Cases**: Code development, testing, status updates
- **Access**: Read all, write code and tests, limited backlog updates

#### `viewer`
- **Description**: Read-only access for viewing
- **Permissions**: `read:*`
- **Use Cases**: Stakeholders, external reviewers, read-only access
- **Access**: Read all public objects, no write access

### Collective Roles

#### `collective`
- **Description**: Team or group with shared authority
- **Influence Level**: collective
- **Permissions**: Defined per collective (varies by team)
- **Use Cases**: Team-based access, shared ownership
- **Access**: Team-specific permissions

### Automation Roles

#### `observer_agent`
- **Description**: Observer agent that ingests code and builds GraphRAG knowledge kernel
- **Influence Level**: automation
- **Permissions**:
  - `read:*`
  - `read:code`
  - `read:ast`
  - `write:graph_node`
  - `write:graph_edge`
  - `read:graph_node`
  - `read:graph_edge`
- **Use Cases**: Code analysis, graph population, AST parsing, entity extraction
- **Restrictions**: Cannot write code, cannot modify backlog, read-only on zqk objects
- **Access**: Read code/objects, write graph nodes/edges only

#### `test_agent`
- **Description**: Test agent that automatically generates tests
- **Influence Level**: automation
- **Permissions**:
  - `read:*`
  - `write:test_case`
  - `write:code` (test code only)
  - `read:graph_node`
  - `read:graph_edge`
- **Use Cases**: Test generation, test execution, code coverage
- **Prerequisites**: Observer agent ready, graph population complete
- **Access**: Read all, write test cases and test code

#### `coder_agent`
- **Description**: Coder agent that writes and modifies code
- **Influence Level**: automation
- **Permissions**:
  - `read:*`
  - `write:code`
  - `write:backlog_item`
  - `write:requirement`
  - `read:graph_node`
  - `read:graph_edge`
- **Use Cases**: Code implementation, refactoring, feature development, bug fixes
- **Prerequisites**: Observer agent ready, test agent ready, governor approval
- **Access**: Read all, write code and project objects (with approval)

## Permission Patterns

### Read Permissions
- `read:*` - Read all objects
- `read:backlog_item` - Read backlog items
- `read:goal` - Read goals
- `read:code` - Read code files
- `read:ast` - Read AST (abstract syntax tree)
- `read:graph_node` - Read graph nodes
- `read:graph_edge` - Read graph edges
- `read:confidential` - Read confidential fields

### Write Permissions
- `write:*` - Write all objects
- `write:backlog_item` - Write backlog items
- `write:goal` - Write goals
- `write:code` - Write code files
- `write:test_case` - Write test cases
- `write:graph_node` - Write graph nodes
- `write:graph_edge` - Write graph edges
- `write:requirement` - Write requirements
- `write:strategic_plan` - Write strategic plans
- `write:roadmap` - Write roadmaps
- `write:workstream` - Write workstreams
- `write:milestone` - Write milestones

### Delete Permissions
- `delete:*` - Delete all objects
- `delete:backlog_item` - Delete backlog items
- `delete:code` - Delete code files

## Role-Based Command Access

### Commands by Role

#### Admin/Founder (Full Access)
- All commands available
- No restrictions

#### Executive
- **Read**: All read commands
- **Write**: Strategic planning commands (goal, roadmap, strategic_plan, decision)
- **Blocked**: Code modification, system operations

#### Owner
- **Read**: All read commands
- **Write**: Project management commands (backlog_item, requirement, goal, workstream, milestone)
- **Blocked**: Code modification, system operations, strategic planning

#### Developer
- **Read**: All read commands
- **Write**: Code and test commands (code, test_case), limited backlog updates
- **Blocked**: Strategic planning, system operations, delete operations

#### Viewer
- **Read**: All read commands
- **Write**: None
- **Blocked**: All write/delete commands

#### Observer Agent
- **Read**: All read commands
- **Write**: Graph operations only (graph_node, graph_edge)
- **Blocked**: All other write operations

#### Test Agent
- **Read**: All read commands
- **Write**: Test case creation, test code
- **Blocked**: Production code, strategic planning, system operations

#### Coder Agent
- **Read**: All read commands
- **Write**: Code, backlog items, requirements (with approval)
- **Blocked**: Strategic planning, system operations, delete operations

## Default Role Assignment

### When No Role Specified
- **Anonymous/Unknown**: `viewer` role with `read:*` permissions
- **System Operations**: `admin` role with full permissions
- **Account with No Roles**: Defaults to `viewer` with `read:*`

### Role Inference from Permissions
- If permissions include `*`: Assign `admin` role
- If permissions include `write:*`: Assign `developer` or `owner` role
- If permissions are read-only: Assign `viewer` role

## Implementation Notes

### Command Annotations
Commands should be annotated with required roles/permissions:

```go
cmd.Annotations = map[string]string{
    "mcp.roles": "admin,executive",              // Requires admin or executive role
    "mcp.permissions": "read:*,write:goal",     // Requires read:* and write:goal
}
```

### Security Context Creation
Security contexts are created from:
1. Client capabilities in `initialize` request
2. Account definitions (from account YAML files)
3. Default fallback (viewer with read-only)

### Permission Checking
1. **Role Check**: User must have at least one required role (or `admin` bypasses all)
2. **Permission Check**: User must have at least one required permission
3. **Config Check**: Command must be in `exposed_commands` and not in `blocked_commands`
4. **Write Operation Check**: Write commands require `write_operations` config and permissions

## Future Considerations

### Privilege Escalation
- Observer agent can escalate to `observer_agent_advanced` after 1000+ successful graph operations
- Escalation grants additional permissions (e.g., `write:backlog_item` status only)

### Role Combinations
- Users can have multiple roles (e.g., `admin` + `developer`)
- Permissions are unioned (user has all permissions from all roles)
- Admin role always bypasses restrictions

### Custom Roles
- Roles can be defined in `docs/process/roles/ROL-*.yaml`
- Each role specifies `role_id`, `influence_level`, and `permissions`
- Roles are loaded dynamically from role definitions

