# Role Prompt Templates System

## Overview

The role prompt templates system allows all MCP prompts to be externalized and customized per role. Similar to object lifecycles, roles can define their own requirements, duties, expectations, and prompt content.

## Architecture

### Components

1. **`RolePromptTemplate`**: Represents a prompt template with role-specific customization
2. **`RolePromptRenderer`**: Renders prompts with role context substitutions
3. **`RoleGuidance.PromptTemplates`**: Map of prompt templates for a role

### Standard Prompt Suite

The following prompts are part of the standard suite that can be customized per role:

- `welcome` - Welcome message
- `getting_started` - Getting started guide
- `query_help` - Query syntax help
- `common_tasks` - Common task examples
- `object_lifecycle` - Object lifecycle guide
- `filter_syntax` - Filter syntax guide
- `role_based_access` - Role-based access explanation
- `create_object_template` - Object creation guide
- `execution_context` - Execution context
- `big_picture` - Project big picture
- `my_role` / `current_role` - Current role information

## Role Object Structure

Role objects can include a `prompt_templates` field with role-specific prompt content:

```yaml
kind: role
role_id: developer
description: Software developer responsible for code development

# Standard prompt suite with role-specific customization
prompt_templates:
  - name: "getting_started"
    description: "Getting started guide for developers"
    content: |
      # Getting Started with {{system_name}} MCP Server
      
      I'm the {{mcp_server_name}}, your interface to the knowledge kernel. 
      As a {{role_title}}, you have the following responsibilities:
      
      {{responsibilities}}
      
      ## Your Permissions
      
      - Read: {{read_permissions}}
      - Write: {{write_permissions}}
      
      ## Available Capabilities
      
      {{privileges}}
      
      ## Restrictions
      
      {{restrictions}}
      
      ## Quick Start
      
      1. Check system health: {{tool:system_status}}
      2. Review your role: prompts/get with name="my_role"
      3. Start working on backlog items
    variables:
      custom_message: "Developers should focus on code quality"
  
  - name: "query_help"
    description: "Query help customized for developers"
    content: |
      # Query Help for {{role_title}}
      
      As a {{role_title}}, you can query objects using:
      - {{tool:object_list}} for listing objects
      - {{tool:object_get}} for getting specific objects
      
      {{custom_message}}
```

## Template Variables

The following variables are automatically substituted in prompt templates:

### Role Context Variables
- `{{role}}` - Role ID (e.g., "developer")
- `{{role_title}}` - Capitalized role name (e.g., "Developer")
- `{{description}}` - Role description
- `{{influence_level}}` - Influence level (e.g., "collective")

### Permission Variables
- `{{read_permissions}}` - List of read permissions
- `{{write_permissions}}` - List of write permissions (or "none (read-only access)")

### Responsibility Variables
- `{{responsibilities}}` - Formatted list of responsibilities

### Privilege/Restriction Variables
- `{{privileges}}` - What the role can do
- `{{restrictions}}` - What the role cannot do

### System Variables
- `{{system_name}}` - System name (default: "ZQK")
- `{{mcp_server_name}}` - MCP server name (default: "ZQK MCP Server")

### Tool Variables
- `{{tool:tool_name}}` - Tool display name (e.g., `{{tool:system_status}}` becomes "zqk_system_status")

### Custom Variables
- Any variables defined in the `variables` field of the prompt template

## Fallback Behavior

If a prompt template is not found in role objects:
1. System checks for role-specific prompt template
2. Falls back to hardcoded default prompts
3. Maintains backward compatibility

## Usage Example

### Developer Role with Custom Prompts

```yaml
kind: role
role_id: developer
prompt_templates:
  - name: "getting_started"
    content: |
      # Developer Getting Started
      
      As a developer, you should:
      1. Always check system health first
      2. Follow workflow policies
      3. Write tests for your code
      
      Your responsibilities:
      {{responsibilities}}
```

### Viewer Role with Limited Access Prompts

```yaml
kind: role
role_id: viewer
prompt_templates:
  - name: "getting_started"
    content: |
      # Viewer Getting Started
      
      As a viewer, you have read-only access.
      
      You can:
      - Read all objects
      - Query and explore the system
      - Access documentation
      
      You cannot:
      - Create, update, or delete objects
      - Execute write operations
      
      {{restrictions}}
```

## Benefits

1. **Role-Specific Customization**: Each role can have tailored prompts
2. **Consistency**: Standard prompt suite ensures all roles have access to help
3. **Maintainability**: Update prompts by modifying role objects, not code
4. **Flexibility**: Custom variables allow fine-grained customization
5. **Traceability**: Prompts can reference criteria objects for requirements

## Future Enhancements

- Prompt versioning
- Prompt inheritance (roles can extend base prompts)
- Multi-language support
- Prompt analytics and usage tracking
- Dynamic prompt generation based on system state
