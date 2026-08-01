# Architecture Review Process

**Version**: 1.0.0  
**Created**: 2025-01-02  
**Status**: Active  
**Purpose**: Process for reviewing and approving architecture changes

## Overview

This process ensures that all architecture changes follow established patterns and maintain system integrity. AI agents must follow this process before implementing significant changes.

## Review Triggers

### Mandatory Review Required For:

1. **New Package Creation**: Creating new packages in `pkg/` or `internal/`
2. **New Bridge Patterns**: Creating any bridge or adapter pattern
3. **Import Cycle Introduction**: Any change that creates circular dependencies
4. **Storage Abstraction Violations**: Direct dependencies on concrete storage implementations
5. **Context Pattern Violations**: Functions requiring multiple context objects
6. **MCP Tool Registration**: Any manual MCP tool registration (should use CLI bridge)

### Optional Review For:

1. **New CLI Commands**: Adding commands to existing command groups
2. **New Architecture Documents**: Creating new architecture documentation
3. **Pattern Extensions**: Extending existing patterns

## Review Process

### Step 1: Pattern Discovery

Before implementing, discover existing patterns:

```bash
# Query architecture patterns
zqk object list doc_entry --filter group=architecture

# Review Architecture Patterns Library
cat docs/architecture/ARCHITECTURE_PATTERNS.md

# Check for similar implementations
grep -r "similar pattern" pkg/ internal/
```

### Step 2: Pattern Matching

Match your proposed implementation to existing patterns:

- ✅ **Pattern Exists**: Use the existing pattern
- ⚠️ **Pattern Extension**: Extend existing pattern, document extension
- ❌ **New Pattern**: Requires review before implementation

### Step 3: Pre-Implementation Checklist

Complete the [Pre-Implementation Checklist](./ARCHITECTURE_PATTERNS.md#pre-implementation-checklist):

- [ ] **Completed [OHTV Decision Framework](./DATA_DRIVEN_DECISION_FRAMEWORK.md) cycle**
  - [ ] OBSERVED: Collected data about current state
  - [ ] HYPOTHESIZED: Formed data-driven hypothesis
  - [ ] TESTED: Designed verifiable test
  - [ ] VERIFIED: Will verify outcome after implementation
- [ ] Checked existing architecture patterns
- [ ] Reviewed related architecture documents
- [ ] **Reviewed [Lessons Learned](./LESSONS_LEARNED.md) for relevant warnings**
- [ ] Confirmed no duplicate patterns exist
- [ ] Verified no import cycles will be created
- [ ] Ensured CLI commands will be used (if MCP exposure needed)
- [ ] Confirmed storage provider abstraction is used
- [ ] Verified single context principle is followed
- [ ] Checked that CLI will be used for object operations
- [ ] **Verified core CLI commands are working before proceeding**

### Step 4: Implementation

If checklist passes, proceed with implementation following the established pattern.

### Step 5: Post-Implementation Review

After implementation, verify:

1. **No Import Cycles**: `go build ./...` succeeds
2. **Pattern Compliance**: Implementation follows established pattern
3. **Documentation Updated**: Architecture patterns documented if new pattern created
4. **Tests Pass**: All tests pass, including architecture compliance tests

## Architecture Decision Records (ADRs)

When creating new patterns, create an ADR:

### ADR Template

```yaml
id: ADR-XXX
title: Pattern Name
status: proposed | accepted | rejected | deprecated
context: |
  Problem statement and context
decision: |
  Decision and rationale
consequences: |
  Positive and negative consequences
related: |
  Related ADRs, backlog items, or architecture documents
```

### ADR Location

ADRs should be stored as `doc_entry` objects:

```bash
zqk object create doc_entry --file adr-template.yaml
```

## Automated Checks

### Pre-Commit Hooks

The following checks run automatically:

1. **Import Cycle Detection**: `go build ./...` must succeed
2. **Pattern Compliance**: Architecture pattern compliance checks
3. **Documentation**: Architecture documents must be linked via `doc_entry`

### Manual Checks

Before committing, run:

```bash
# Check for import cycles
go build ./...

# Check architecture compliance
zqk system check --fast

# Verify documentation
zqk docman-sync --check-only
```

## Violation Handling

### Minor Violations

- **Pattern Deviation**: Update to follow pattern, document lesson learned
- **Missing Documentation**: Add documentation, update patterns library

### Major Violations

- **Import Cycles**: Refactor to remove cycles
- **Duplicate Architecture**: Remove duplicate, use established pattern
- **Storage Abstraction Violation**: Refactor to use interfaces

## Pattern Evolution

Patterns can evolve, but changes must:

1. **Maintain Backward Compatibility**: Existing code continues to work
2. **Update Documentation**: Architecture patterns library updated
3. **Update Examples**: Code examples reflect new pattern
4. **Notify Stakeholders**: Architecture changes documented in ADR

## Lessons Learned Review

**Critical**: Before implementing changes, review [Lessons Learned](./LESSONS_LEARNED.md) to avoid repeating past mistakes.

### Review Frequency
- **Weekly** during active development
- **Monthly** during maintenance
- **Before major releases** (alpha, beta, production)

### Review Checklist
- [ ] Are we skipping tests without fix plans?
- [ ] Are we editing persisted files directly?
- [ ] Are we verifying changes end-to-end?
- [ ] Are core CLI commands working?
- [ ] Is logging/observability functioning?
- [ ] Are tests reflecting real system behavior?

## Related Documentation

- [Architecture Patterns Library](./ARCHITECTURE_PATTERNS.md)
- [Lessons Learned](./LESSONS_LEARNED.md) - **Review before major changes**
- [MCP CLI Bridge Architecture v1.0](./mcp-cli-bridge-v1.0.md)
- [AI Agent Onboarding](../../onboarding/AI_AGENT_ONBOARDING.md)

---

*This process ensures architectural consistency and prevents drift. All AI agents must follow this process.*

