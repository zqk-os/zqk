# Resource URI Scheme Configuration

## Overview

Resource URI scheme rules determine which URI scheme (e.g., `file://`, `docs://`, `internal://`) is used for resources based on their location. This allows categorization and organization of resources by their location in the project structure.

## Configuration

URI scheme rules can be configured in `.zqk/mcp/config.yaml`:

```yaml
mcp_server:
  resources:
    uri_scheme_rules:
      - pattern: "docs/architecture/_internal/**"
        scheme: "internal://"
        category: "internal"
        description: "Internal documentation (lifecycles, specs, etc.)"
      
      - pattern: "docs/architecture/architecture/**"
        scheme: "docs://"
        category: "architecture"
        description: "Architecture documentation"
      
      - pattern: "docs/architecture/workstreams/**"
        scheme: "docs://"
        category: "workstreams"
        description: "Workstream documentation"
      
      - pattern: "docs/architecture/policies/**"
        scheme: "docs://"
        category: "policy"
        description: "Policy documentation"
      
      - pattern: "docs/onboarding/**"
        scheme: "docs://"
        category: "onboarding"
        description: "Onboarding documentation"
      
      - pattern: "docs/**"
        scheme: "file://"
        category: "documentation"
        description: "General documentation"
```

## Default Rules

If no rules are configured, the system uses default rules:

- `docs/architecture/_internal/**` → `internal://` (category: "internal")
- `docs/architecture/architecture/**` → `docs://` (category: "architecture")
- `docs/architecture/workstreams/**` → `docs://` (category: "workstreams")
- `docs/architecture/policies/**` → `docs://` (category: "policy")
- `docs/onboarding/**` → `docs://` (category: "onboarding")
- `docs/**` → `file://` (category: "documentation")
- Default fallback → `file://` (category: "documentation")

## Pattern Matching

Patterns support glob-style matching:

- `**/pattern` - Match anywhere in path
- `pattern/**` - Match prefix
- `pattern*` - Match prefix with wildcard
- Exact match - Direct path match

## Benefits

1. **Categorization**: Resources are automatically categorized by location
2. **Organization**: Different URI schemes help organize resources
3. **Customization**: Users can configure their own schemes and categories
4. **Flexibility**: Rules can be customized per project or environment

## Usage

Rules are automatically applied when:
- Resources are discovered via `DiscoverAdditionalResources`
- Resources are registered via `RegisterCriticalResources`
- Server configuration is loaded via `SetConfig`

The resolver checks rules in order and uses the first matching rule. If no rule matches, it falls back to `file://` scheme.
