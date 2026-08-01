# Architecture Governance Framework

**Version**: 1.0.0  
**Created**: 2025-01-02  
**Status**: Active  
**Purpose**: Complete framework for ensuring AI agents follow established architecture patterns

## Overview

This document defines the complete lifecycle and policy framework to ensure AI agents reliably understand and follow established architecture patterns, code quality standards, and best practices.

## Problem Statement

Without proper governance, AI agents may:
- Create duplicate architectures when patterns exist
- Introduce import cycles
- Violate abstraction layers
- Create brittle, disconnected systems
- Ignore established patterns

## Solution Framework

### 1. Pattern Discovery (Pre-Implementation)

**Requirement**: Before implementing any new functionality, AI agents MUST discover existing patterns.

**Process**:
```bash
# Step 1: Query architecture patterns
zqk object list doc_entry --filter group=architecture

# Step 2: Review Architecture Patterns Library
cat docs/architecture/ARCHITECTURE_PATTERNS.md

# Step 3: Check for similar implementations
grep -r "similar pattern" pkg/ internal/
```

**Documentation**:
- [Architecture Patterns Library](./ARCHITECTURE_PATTERNS.md)
- [Architecture Review Process](./ARCHITECTURE_REVIEW_PROCESS.md)

### 2. Pre-Implementation Checklist

**Requirement**: Complete checklist before implementing.

**Checklist**:
- [ ] Checked existing architecture patterns
- [ ] Reviewed related architecture documents
- [ ] Confirmed no duplicate patterns exist
- [ ] Verified no import cycles will be created
- [ ] Ensured CLI commands will be used (if MCP exposure needed)
- [ ] Confirmed storage provider abstraction is used
- [ ] Verified single context principle is followed
- [ ] Checked that CLI will be used for object operations

### 3. Automated Compliance Checks

**Requirement**: Pre-commit hooks automatically check for violations.

**Checks**:
1. **Import Cycle Detection**: `go build ./...` must succeed
2. **Storage Abstraction**: No direct imports of concrete storage implementations
3. **Bridge Pattern Duplication**: Warns on new bridges when CLI bridge exists
4. **Manual MCP Registration**: Warns on manual tool registration
5. **Documentation**: Warns on new packages without README

**Implementation**:
- Script: `scripts/check-architecture-compliance.sh`
- Hook: `tools/git-hooks/pre-commit` (automatically runs checks)

### 4. Architecture Review Process

**Requirement**: Significant changes require review.

**Triggers**:
- New package creation
- New bridge patterns
- Import cycle introduction
- Storage abstraction violations
- Context pattern violations
- Manual MCP tool registration

**Process**: See [Architecture Review Process](./ARCHITECTURE_REVIEW_PROCESS.md)

### 5. Documentation Requirements

**Requirement**: All architecture decisions must be documented.

**Documentation Types**:
1. **Architecture Patterns**: Documented in [Architecture Patterns Library](./ARCHITECTURE_PATTERNS.md)
2. **Architecture Decisions**: Recorded as ADRs (Architecture Decision Records)
3. **Package Documentation**: Each new package requires README.md
4. **Pattern Extensions**: Extensions to patterns must be documented

### 6. Onboarding Integration

**Requirement**: All new AI agents must understand patterns before starting work.

**Integration Points**:
1. **Onboarding Document**: [AI Agent Onboarding](../../onboarding/AI_AGENT_ONBOARDING.md) includes pattern discovery section
2. **First Task**: New agents must review Architecture Patterns Library
3. **Pattern Queries**: Agents must query for patterns before implementing

## Established Patterns

### Pattern 1: CLI Bridge for MCP Exposure

**Rule**: All MCP exposure happens via CLI commands, not separate bridges.

**Example**: Metrics tools (`zqk reports pcs`) are exposed via CLI bridge, not a separate metrics bridge.

**Anti-Pattern**: Creating `pkg/mcp/bridge/metrics_bridge.go` when CLI commands would suffice.

### Pattern 2: Context-Driven Bootstrap

**Rule**: Systems bootstrap from context, not manual registration.

**Example**: CLI bridge automatically discovers commands and registers them as MCP tools.

**Anti-Pattern**: Manual `RegisterMetricsTools()` when CLI bridge auto-discovers.

### Pattern 3: Single Context Principle

**Rule**: Use single merged context, derive variations as needed.

**Example**: `ctx.WithFormat("json")` instead of passing separate format context.

**Anti-Pattern**: Functions with signatures like `func(fmtCtx, storageCtx, secCtx)`.

### Pattern 4: CLI as Normative Path

**Rule**: Always use CLI for object operations.

**Example**: `zqk object update BLI-655` instead of editing YAML directly.

**Anti-Pattern**: Direct file edits requiring `--force` flags.

### Pattern 5: Storage Provider Abstraction

**Rule**: Use `ObjectStorageProvider` interface, not concrete implementations.

**Example**: Code depends on `storage.ObjectStorageProvider`, not `FileObjectStorage`.

**Anti-Pattern**: Direct imports of `pkg/storage/object_storage_file.go`.

## Enforcement Mechanisms

### 1. Pre-Commit Hooks

**Location**: `tools/git-hooks/pre-commit`

**Checks**:
- Import cycle detection
- Architecture compliance
- Linting

**Result**: Commit blocked if violations detected.

### 2. CI/CD Validation

**Location**: GitHub Actions workflows

**Checks**:
- Build validation
- Test execution
- Architecture compliance

**Result**: PR blocked if violations detected.

### 3. Code Review

**Process**: Human review of PRs

**Focus**:
- Architecture pattern compliance
- Documentation completeness
- Test coverage

### 4. Automated Scripts

**Scripts**:
- `scripts/check-architecture-compliance.sh`: Architecture compliance checker
- `scripts/generate-architecture-readme-index.sh`: Documentation index generator

## Pattern Evolution

Patterns can evolve, but changes must:

1. **Maintain Backward Compatibility**: Existing code continues to work
2. **Update Documentation**: Architecture patterns library updated
3. **Update Examples**: Code examples reflect new pattern
4. **Notify Stakeholders**: Architecture changes documented in ADR

## Metrics & Monitoring

### Compliance Metrics

Track:
- Number of architecture violations caught by pre-commit hooks
- Number of patterns discovered vs. new patterns created
- Time to pattern discovery (should be < 5 minutes)

### Pattern Usage

Track:
- Which patterns are most commonly used
- Which patterns are most commonly violated
- Pattern discovery queries

## Related Documentation

- [Architecture Patterns Library](./ARCHITECTURE_PATTERNS.md)
- [Architecture Review Process](./ARCHITECTURE_REVIEW_PROCESS.md)
- [MCP CLI Bridge Architecture v1.0](./mcp-cli-bridge-v1.0.md)
- [AI Agent Onboarding](../../onboarding/AI_AGENT_ONBOARDING.md)

---

*This framework ensures architectural consistency and prevents drift. All AI agents must follow this framework.*

