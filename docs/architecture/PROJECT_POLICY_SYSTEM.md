# Project Policy System

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-01-02  
**Status**: Active  
**Purpose**: Central policy system for establishing and maintaining project standards throughout the delivery lifecycle

## Overview

The Project Policy System provides a generalized `policy` object that serves as the central index for project standards, expectations, and best practices. Policies are **required at project initialization** and **evolve throughout the delivery lifecycle**, providing AI agents with a quick reference to understand expectations before implementing.

## Policy Object

Policies use the `policy` object spec with the following key characteristics:

### Required at Project Start

Every new project must have a core set of policies established:

```bash
# Project initialization should create default policies
zqk object create policy --file policies/architecture-patterns.yaml
zqk object create policy --file policies/code-quality.yaml
zqk object create policy --file policies/documentation.yaml
```

### Central Index

Policies serve as the central index for all project standards:

```bash
# Query all policies
zqk object list policy

# Query by category
zqk object list policy --filter category=architecture
zqk object list policy --filter category=code_quality

# Query by policy type
zqk object list policy --filter policy_type=standard
zqk object list policy --filter policy_type=requirement
```

## Policy Categories

### Standard Categories

1. **architecture**: Architecture patterns and design principles
2. **code_quality**: Code quality standards and best practices
3. **documentation**: Documentation standards and conventions
4. **testing**: Testing requirements and best practices
5. **security**: Security policies and requirements
6. **workflow**: Development workflow and process policies
7. **git**: Git workflow and commit policies
8. **ci_cd**: CI/CD pipeline policies

**Universal spine (all projects):** **`POL-WORKFLOW-VDS`** — Verifiable Decomposition Spine. Rigid stages and independently verifiable chunks; project preferences (lint, test runner, style) live only in customization. See [VERIFIABLE_DECOMPOSITION_SPINE.md](./VERIFIABLE_DECOMPOSITION_SPINE.md).

## Policy Types

### 1. Standard (Mandatory)

**Enforcement**: Hard requirement, blocks non-compliant code

**Example**: "All MCP exposure must use CLI bridge pattern"

```yaml
id: POL-001
kind: policy
title: Architecture Pattern: CLI Bridge for MCP Exposure
category: architecture
policy_type: standard
body: |
  All functionality exposed via MCP must use the CLI bridge pattern.
  Do not create separate MCP bridges when CLI commands would suffice.
  
  **Pattern**: Create CLI commands, not separate MCP bridges.
  **Enforcement**: Pre-commit hooks and CI/CD checks.
```

### 2. Requirement (Must Follow)

**Enforcement**: Required, but may have exceptions with approval

**Example**: "All architectural changes require ADR"

```yaml
id: POL-002
kind: policy
title: Architecture Decision Records Required
category: architecture
policy_type: requirement
body: |
  All significant architectural changes must have an associated ADR.
  ADRs document the decision, rationale, and impact.
  
  **Exception**: Minor changes may be exempted with approval.
```

### 3. Guideline (Should Follow)

**Enforcement**: Recommended, reminders provided

**Example**: "Follow single context principle"

```yaml
id: POL-003
kind: policy
title: Single Context Principle
category: architecture
policy_type: guideline
body: |
  Functions should use a single merged context, deriving variations as needed.
  Avoid functions with multiple context parameters.
```

### 4. Best Practice (Recommended)

**Enforcement**: Informational, suggestions provided

**Example**: "Use storage provider abstraction"

```yaml
id: POL-004
kind: policy
title: Storage Provider Abstraction
category: architecture
policy_type: best_practice
body: |
  Code should depend on storage.ObjectStorageProvider interface,
  not concrete implementations like FileObjectStorage.
```

### 5. Anti-Pattern (What to Avoid)

**Enforcement**: Warnings when detected

**Example**: "Avoid duplicate architecture patterns"

```yaml
id: POL-005
kind: policy
title: Anti-Pattern: Duplicate Architecture
category: architecture
policy_type: anti_pattern
body: |
  Do not create duplicate architecture patterns when existing patterns
  would work. Always check for existing patterns before implementing.
```

## Policy Lifecycle

### 1. Initialization

Policies are created at project start:

```bash
# Create core policies during project initialization
zqk object create policy --file policies/architecture-patterns.yaml
zqk object create policy --file policies/code-quality.yaml
```

### 2. Evolution

Policies evolve throughout the project lifecycle:

```bash
# Update policy as standards evolve
zqk object update POL-001 --field version=1.1.0
zqk object update POL-001 --field "body=Updated policy content..."
zqk object update POL-001 --field effective_date=2025-01-15
```

### 3. Review

Policies are reviewed periodically:

```bash
# Set review date
zqk object update POL-001 --field review_date=2025-07-01

# Query policies due for review
zqk object list policy --filter "review_date<=2025-07-01"
```

## Policy Discovery for AI Agents

### Pre-Implementation Query

Before implementing, AI agents should query relevant policies:

```bash
# Query architecture policies
zqk object list policy --filter category=architecture

# Query code quality policies
zqk object list policy --filter category=code_quality

# Query all standards (mandatory)
zqk object list policy --filter policy_type=standard
```

### MCP Integration

Policies are available via MCP for AI assistants:

```json
{
  "name": "get_policies",
  "arguments": {
    "category": "architecture",
    "policy_type": "standard"
  }
}
```

### Lifecycle Reminders

Policies trigger reminders when violated:

```bash
# Get policy compliance reminders
zqk lifecycle reminders --reason-code policy
```

## Policy Enforcement

### Enforcement Configuration

Each policy can configure its enforcement:

```yaml
enforcement:
  automated: true              # Automated checks enabled
  reminder_enabled: true       # Lifecycle reminders enabled
  review_required: false       # Requires human review
  severity: "high"            # Reminder severity
```

### Enforcement Mechanisms

1. **Automated Checks**: Pre-commit hooks, CI/CD
2. **Lifecycle Reminders**: Proactive reminders via lifecycle system
3. **Review Requirements**: Human review for certain policies
4. **Severity Levels**: Determines reminder priority

## Policy Index

### Central Reference

The policy index serves as the single source of truth:

```bash
# Get policy index
zqk object list policy --format table

# Get policy by category
zqk object list policy --filter category=architecture --format yaml

# Search policies
zqk object list policy --filter "title~bridge"
```

### Integration with Other Systems

Policies integrate with:

- **Architecture Patterns**: Policies reference patterns via `related_patterns`
- **ADRs**: Policies can reference ADRs
- **Rules**: Policies complement governance rules
- **Lifecycle System**: Policies trigger reminders
- **MCP**: Policies available via MCP tools

## Example Policies

### Architecture Policy

```yaml
id: POL-ARCH-001
kind: policy
title: CLI Bridge Pattern for MCP Exposure
category: architecture
policy_type: standard
version: 1.0.0
effective_date: "2025-01-02"
body: |
  All functionality exposed via MCP must use the CLI bridge pattern.
  
  **Pattern**: Create CLI commands, not separate MCP bridges.
  **Rationale**: Single source of truth, no code duplication, automatic privilege filtering.
  
  **Anti-Pattern**: Creating separate bridge packages when CLI commands would suffice.
examples:
  - "✅ Create `zqk reports pcs` command - automatically exposed via CLI bridge"
  - "❌ Create `pkg/mcp/bridge/metrics_bridge.go` - violates pattern"
related_patterns:
  - doc_entry:mcp-cli-bridge-v1.0
  - ADR-001
enforcement:
  automated: true
  reminder_enabled: true
  review_required: false
  severity: "high"
applicability:
  workstreams: []
  object_types: []
  file_patterns:
    - "pkg/mcp/**/*.go"
  exclusions: []
```

### Code Quality Policy

```yaml
id: POL-CODE-001
kind: policy
title: Import Cycle Prevention
category: code_quality
policy_type: requirement
version: 1.0.0
effective_date: "2025-01-02"
body: |
  Code must not create import cycles between packages.
  
  **Enforcement**: Build must succeed (`go build ./...`).
  **Resolution**: Use interfaces, bridge patterns, or separate packages.
examples:
  - "✅ Use `storage.ObjectStorageProvider` interface"
  - "❌ Direct import of `pkg/storage/object_storage_file.go`"
enforcement:
  automated: true
  reminder_enabled: true
  review_required: false
  severity: "critical"
```

## Project Initialization

### Required Policies

New projects should initialize with core policies:

1. **Architecture Policies**: Pattern requirements, ADR requirements
2. **Code Quality Policies**: Import cycles, abstraction layers
3. **Documentation Policies**: Documentation standards, linking conventions
4. **Testing Policies**: TDD requirements, test coverage
5. **Workflow Policies**: Git workflow, PR requirements

### Initialization Script

```bash
#!/bin/sh
# Initialize project policies

# Architecture policies
zqk object create policy --file policies/architecture/cli-bridge-pattern.yaml
zqk object create policy --file policies/architecture/storage-abstraction.yaml
zqk object create policy --file policies/architecture/context-principle.yaml

# Code quality policies
zqk object create policy --file policies/code-quality/import-cycles.yaml
zqk object create policy --file policies/code-quality/abstraction-layers.yaml

# Documentation policies
zqk object create policy --file policies/documentation/linking-conventions.yaml
zqk object create policy --file policies/documentation/index-requirements.yaml
```

## Related Documentation

- [Policy Object Spec](../../_internal/object_specs/policy.yaml)
- [Architecture Patterns Library](./ARCHITECTURE_PATTERNS.md)
- [Architecture Governance Framework](./ARCHITECTURE_GOVERNANCE.md)
- [Rule Object Spec](../../_internal/object_specs/rule.yaml)

---

*Policies provide the central index for project standards, ensuring AI agents understand expectations before implementing.*

