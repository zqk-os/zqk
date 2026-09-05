# Architecture Enforcement Mechanisms

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-01-02  
**Status**: Active  
**Purpose**: Comprehensive guide to enforcing architectural constraints without relying solely on commit hooks

## Overview

While pre-commit hooks provide immediate feedback, architecture enforcement should be multi-layered and integrated throughout the development lifecycle. This document describes enforcement mechanisms beyond commit hooks.

## Enforcement Layers

### 1. Runtime Checks (Code-Level)

**Location**: In code itself

**Implementation**: Architecture compliance checks built into the codebase

```go
// Example: Check for import cycles at package initialization
func init() {
    if hasImportCycle() {
        log.Fatal("Import cycle detected - architecture violation")
    }
}

// Example: Validate storage abstraction at compile time
// Use interfaces, not concrete types
type MyService struct {
    storage storage.ObjectStorageProvider  // ✅ Correct
    // storage *storage.FileObjectStorage  // ❌ Violation
}
```

**Benefits**:
- Immediate feedback during development
- Works in IDE and during builds
- No external tooling required

### 2. Build-Time Validation

**Location**: `go build` and test execution

**Implementation**: Architecture checks integrated into build process

```bash
# Check for import cycles
go build ./...

# Run architecture compliance tests
go test ./pkg/architecture -run TestArchitectureCompliance
```

**Integration**: Add to `Makefile` or build scripts

```makefile
.PHONY: check-architecture
check-architecture:
	@echo "Checking architecture compliance..."
	@./scripts/check-architecture-compliance.sh
	@go test ./pkg/architecture -run TestArchitectureCompliance
```

### 3. CI/CD Pipeline Checks

**Location**: GitHub Actions, GitLab CI, etc.

**Implementation**: Architecture checks in CI pipeline

```yaml
# .github/workflows/architecture-compliance.yml
name: Architecture Compliance

on: [push, pull_request]

jobs:
  architecture-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Check architecture compliance
        run: |
          ./scripts/check-architecture-compliance.sh
          go test ./pkg/architecture -run TestArchitectureCompliance
```

**Benefits**:
- Catches violations before merge
- Provides feedback in PR
- Blocks non-compliant code

### 4. Lifecycle Reminder System

**Location**: Integrated with lifecycle evaluation

**Implementation**: Architecture compliance reminders via lifecycle system

```go
// Architecture compliance reminder
type ArchitectureReminder struct {
    ObjectID    string
    Reason      string  // "pattern_violation", "missing_adr", "import_cycle"
    Severity    string  // "high", "medium", "low"
    Message     string
    Action      string  // Suggested action
    ReasonCode  string  // "architecture"
}
```

**Integration**: Add to lifecycle evaluation

```yaml
# Lifecycle hook for architecture reminders
lifecycle_hooks:
  - name: architecture_compliance_check
    trigger: on_status_change
    action: check_architecture_compliance
    reminder_type: architecture
    reason_code: architecture
```

**Usage**:
```bash
# Get architecture compliance reminders
zqk lifecycle reminders --reason-code architecture

# Via MCP
mcp_tool: get_lifecycle_reminders
arguments:
  reason_code: architecture
```

### 5. MCP Integration

**Location**: MCP tools for AI assistants

**Implementation**: Architecture guidance via MCP

```go
// MCP tool: get_architecture_guidance
func HandleGetArchitectureGuidance(args map[string]any) (any, error) {
    proposedChange := args["proposed_change"].(string)
    
    // Check against patterns
    patterns := discoverRelevantPatterns(proposedChange)
    violations := checkPatternCompliance(proposedChange, patterns)
    
    return map[string]any{
        "relevant_patterns": patterns,
        "violations": violations,
        "suggestions": generateSuggestions(violations),
    }, nil
}
```

**Usage**: AI assistants can query for guidance before implementing

### 6. IDE Integration

**Location**: IDE plugins/extensions

**Implementation**: Real-time architecture checks in IDE

**Options**:
- VS Code extension
- Go language server integration
- Custom lint rules

**Benefits**:
- Immediate feedback while coding
- Pattern suggestions
- Violation highlighting

### 7. Periodic System Checks

**Location**: Scheduled system checks

**Implementation**: Periodic architecture compliance audits

```bash
# Scheduled check (cron, systemd timer, etc.)
0 9 * * * cd /path/t./zqk && ./scripts/check-architecture-compliance.sh --report
```

**Integration**: Lifecycle reminder system can trigger periodic checks

```yaml
# Periodic architecture audit
lifecycle_reminders:
  - name: weekly_architecture_audit
    schedule: weekly
    action: architecture_compliance_audit
    reason_code: architecture
```

### 8. Documentation Integration

**Location**: Architecture documentation

**Implementation**: Architecture patterns discoverable via object API

```bash
# Query for relevant patterns
zqk object list doc_entry --filter group=architecture --filter "title~bridge"

# Query for ADRs
zqk object list decision --filter "title~ADR-"
```

**Benefits**:
- Patterns are first-class objects
- Queryable and discoverable
- Integrated with system

## Reminder System Integration

### Architecture Compliance Reminders

The lifecycle reminder system can provide timely reminders for:

1. **Pattern Violations**: When code violates established patterns
2. **Missing ADRs**: When architectural changes lack decision records
3. **Stale Patterns**: When patterns haven't been reviewed recently
4. **Import Cycles**: When import cycles are detected
5. **Abstraction Violations**: When storage/provider abstractions are violated

### Configuration

```yaml
# .zqk/config.yaml
architecture:
  reminders:
    enabled: true
    check_interval: daily
    reminder_types:
      - pattern_violation
      - missing_adr
      - import_cycle
      - abstraction_violation
    thresholds:
      pattern_violation: high
      missing_adr: medium
      import_cycle: critical
      abstraction_violation: high
```

### Reminder Examples

```bash
# Get architecture reminders
zqk lifecycle reminders --reason-code architecture

# Output:
# ⚠️  Architecture Reminder: Pattern Violation
#    Object: pkg/mcp/metrics_bridge.go
#    Issue: Duplicate bridge pattern detected
#    Suggestion: Use CLI bridge pattern instead
#    Pattern: docs/process/architecture/mcp-cli-bridge-v1.0.md
#    Severity: high
```

## Implementation Priority

### Phase 1: Immediate (Already Implemented)

- ✅ Pre-commit hooks
- ✅ Architecture compliance script
- ✅ Build-time validation (go build)

### Phase 2: Short-Term

- [ ] Lifecycle reminder integration
- [ ] MCP architecture guidance tool
- [ ] CI/CD pipeline checks

### Phase 3: Medium-Term

- [ ] IDE integration
- [ ] Periodic system checks
- [ ] Automated ADR creation prompts

### Phase 4: Long-Term

- [ ] Machine learning pattern detection
- [ ] Automated pattern suggestions
- [ ] Predictive architecture compliance

## Related Documentation

- [Architecture Patterns Library](./ARCHITECTURE_PATTERNS.md)
- [Architecture Review Process](./ARCHITECTURE_REVIEW_PROCESS.md)
- [Architecture Decision Records](./ARCHITECTURE_DECISION_RECORDS.md)
- [Lifecycle Reminders](../../backlog/BLI-092.yaml)

---

*Multi-layered enforcement ensures architecture compliance throughout the development lifecycle, not just at commit time.*

