# Project Initialization Guide

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-01-02  
**Status**: Active  
**Purpose**: Guide for initializing new projects with required policies and standards

## Overview

New projects must be initialized with a core set of policies that establish standards, expectations, and best practices. These policies serve as the central index for AI agents and evolve throughout the delivery lifecycle.

## Required Initialization

### Step 1: Create Core Policies

Every new project must have these core policy categories:

```bash
# Architecture policies
zqk object create policy --file policies/architecture/cli-bridge-pattern.yaml
zqk object create policy --file policies/architecture/pattern-discovery.yaml
zqk object create policy --file policies/architecture/storage-abstraction.yaml

# Code quality policies
zqk object create policy --file policies/code-quality/import-cycles.yaml
zqk object create policy --file policies/code-quality/abstraction-layers.yaml

# Documentation policies
zqk object create policy --file policies/documentation/linking-conventions.yaml
zqk object create policy --file policies/documentation/index-requirements.yaml

# Testing policies
zqk object create policy --file policies/testing/tdd-requirements.yaml
zqk object create policy --file policies/testing/coverage-requirements.yaml

# Workflow policies
zqk object create policy --file policies/workflow/git-workflow.yaml
zqk object create policy --file policies/workflow/pr-requirements.yaml
```

### Step 2: Verify Policy Index

After creating policies, verify the index:

```bash
# List all policies
zqk object list policy

# Verify by category
zqk object list policy --filter category=architecture
zqk object list policy --filter category=code_quality
zqk object list policy --filter category=documentation
```

### Step 3: Link to Architecture Patterns

Link policies to architecture patterns:

```bash
# Update policy to reference architecture patterns
zqk object update POL-ARCH-001 --field "related_patterns=[doc_entry:mcp-cli-bridge-v1.0]"
```

## Policy Template

### Standard Policy Template

```yaml
id: POL-XXX-XXX
kind: policy
title: Policy Title
category: architecture | code_quality | documentation | testing | security | workflow
policy_type: standard | requirement | guideline | best_practice | anti_pattern
version: 1.0.0
effective_date: "YYYY-MM-DD"
created_at: "YYYY-MM-DDTHH:MM:SSZ"
created_by: account:system
updated_at: "YYYY-MM-DDTHH:MM:SSZ"
updated_by: account:system
schema_version: 2.0.0
status: active
origin_project: zqk
origin_system: zqk

body: |
  Policy content describing standards, expectations, and best practices.
  Markdown is supported.

examples:
  - "✅ Correct example"
  - "❌ Incorrect example (anti-pattern)"

related_patterns:
  - doc_entry:pattern-name
  - ADR-XXX

enforcement:
  automated: true | false
  reminder_enabled: true | false
  review_required: true | false
  severity: "critical" | "high" | "medium" | "low"

applicability:
  workstreams: []
  object_types: []
  file_patterns:
    - "pkg/**/*.go"
  exclusions: []

goal_refs: []
workstream_refs: []
milestone_refs: []
review_date: "YYYY-MM-DD"
```

## Policy Categories

### Required Categories

1. **architecture**: Architecture patterns and design principles
2. **code_quality**: Code quality standards and best practices
3. **documentation**: Documentation standards and conventions
4. **testing**: Testing requirements and best practices
5. **workflow**: Development workflow and process policies

### Optional Categories

6. **security**: Security policies and requirements
7. **git**: Git workflow and commit policies
8. **ci_cd**: CI/CD pipeline policies

## Policy Evolution

### Updating Policies

Policies evolve throughout the project lifecycle:

```bash
# Update policy version
zqk object update POL-XXX-XXX --field version=1.1.0

# Update policy content
zqk object update POL-XXX-XXX --field "body=Updated policy content..."

# Update effective date
zqk object update POL-XXX-XXX --field effective_date=2025-01-15
```

### Policy Review

Set review dates for policies:

```bash
# Set review date
zqk object update POL-XXX-XXX --field review_date=2025-07-01

# Query policies due for review
zqk object list policy --filter "review_date<=2025-07-01"
```

## Integration with Other Systems

### Architecture Patterns

Policies reference architecture patterns:

```yaml
related_patterns:
  - doc_entry:mcp-cli-bridge-v1.0
  - doc_entry:ARCHITECTURE_PATTERNS
```

### ADRs

Policies can reference ADRs:

```yaml
related_patterns:
  - ADR-001
```

### Lifecycle Reminders

Policies trigger lifecycle reminders:

```bash
# Get policy compliance reminders
zqk lifecycle reminders --reason-code policy
```

## Verification

### Post-Initialization Check

After initialization, verify:

```bash
# Check policy count by category
zqk object list policy --filter category=architecture | wc -l
zqk object list policy --filter category=code_quality | wc -l

# Verify all standards are present
zqk object list policy --filter policy_type=standard

# Check policy index
zqk object list policy --format table
```

## Related Documentation

- [Project Policy System](./PROJECT_POLICY_SYSTEM.md)
- [Policy Object Spec](../_internal/object_specs/policy.yaml)
- [Architecture Patterns Library](./ARCHITECTURE_PATTERNS.md)
- [AI Agent Onboarding](../../onboarding/AI_AGENT_ONBOARDING.md)

---

*Project initialization establishes the foundation for maintaining standards throughout the delivery lifecycle. All new projects must complete this initialization.*

