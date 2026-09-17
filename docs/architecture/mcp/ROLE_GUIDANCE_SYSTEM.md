# Role Guidance System

**Last Verified:** 2026-08-31


## Overview

The role guidance system externalizes role-specific responsibilities, duties, and recommendations from hardcoded text into system role objects. This enables:

- **Traceability**: Responsibilities linked to criteria objects (CRIT-####)
- **Acknowledgment Tracking**: Mark responsibilities that require agent acknowledgment
- **Review Lifecycle**: Schedule regular review and feedback exchange
- **Formalization**: Structured responsibilities instead of hardcoded strings

## Architecture

### Components

1. **`RoleGuidanceGenerator`** (`role_guidance.go`): Loads role objects and generates formatted guidance
2. **`RoleLoader`** Interface: Abstracts role object loading from storage
3. **`CriteriaLoader`** Interface: Abstracts criteria object loading for traceability
4. **`RoleAwarePromptGenerator`**: Updated to use role guidance system

### Role Object Structure

Role objects should include a `responsibilities` field with the following structure:

```yaml
kind: role
role_id: developer
description: Software developer responsible for code development
influence_level: collective
permissions:
  - read:*
  - write:code
  - write:test_case

# NEW: Structured responsibilities field
responsibilities:
  - description: "Following workflow policies strictly (branch, PR, commit)"
    category: "workflow"
    criteria_refs:
      - "CRIT-1234"  # Reference to criteria object
      - "CRIT-5678"
    requires_ack: true
    review_frequency: "monthly"
  
  - description: "Understanding lifecycles before state changes"
    category: "workflow"
    criteria_refs:
      - "CRIT-9012"
    requires_ack: false
    review_frequency: "quarterly"
  
  - description: "Writing code that adheres to code quality policies"
    category: "code_quality"
    criteria_refs:
      - "CRIT-3456"
    requires_ack: true
    review_frequency: "monthly"

# NEW: Access upgrade instructions (for roles with limited access)
access_upgrade_steps:
  - "Contact your administrator to update your role/permissions"
  - "They can add roles like \"developer\" or \"admin\" to your security context"
  - "They can enable write_operations in the server config"
access_upgrade_prompt: "role_based_access"  # Optional: prompt name for more details

# NEW: Role-specific welcome messages
welcome_message: "Welcome! As a developer, you can help build and improve the system..."
welcome_quick_start: |
  You can use:
  - prompts/get with name='getting_started' to see a comprehensive guide
  - prompts/get with name='execution_context' for current execution context
  - resources/list to see available documentation
  - tools/list to see all available tools based on your role
```

### Criteria Object Structure

Criteria objects (CRIT-####) should be created for each responsibility to enable traceability:

```yaml
kind: criteria
id: CRIT-1234
title: Developer follows workflow policies
category: acceptance
description: |
  Developers must follow all workflow policies including:
  - Branch naming conventions
  - PR requirements
  - Commit message standards
validation_method: automated_test
```

## Usage

### In MCP Server

The system automatically attempts to load role objects when generating role-specific guidance. If role objects are found, they are used; otherwise, it falls back to hardcoded guidance.

```go
// Role guidance is automatically loaded in handlePromptsGet
// Storage adapter should be created outside mcp package to avoid import cycles
```

### Creating Storage Adapter

To enable role object loading, create a storage adapter outside the `mcp` package:

```go
// In cmd/zqk/mcp or similar location where storage is available
type StorageAdapter struct {
    provider storage.ObjectStorageProvider
}

func (a *StorageAdapter) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter interface{}) (interface{}, error) {
    // Convert filter map to storage.ListFilter
    // Call a.provider.List()
    // Convert QueryResult to map format
}
```

### Fallback Behavior

If role objects are not available or storage is not accessible:
- System falls back to hardcoded role guidance
- Maintains backward compatibility
- No breaking changes

## Benefits

1. **Traceability**: Every responsibility can be traced to criteria objects
2. **Acknowledgment**: Track which responsibilities agents have acknowledged
3. **Review Lifecycle**: Schedule regular reviews for continuous improvement
4. **Maintainability**: Update role guidance by modifying role objects, not code
5. **Consistency**: Same role objects used by MCP and CLI

## Future Enhancements

- Acknowledgment tracking system
- Review lifecycle automation
- Feedback collection and analysis
- Role guidance versioning
- Multi-language support
