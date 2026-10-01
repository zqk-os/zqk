# SpecBuilder Enum Consolidation and AST Pruning Architecture

## Overview

The SpecBuilder code generation pipeline historically produced 220+ discrete micro-packages under `pkg/specbuilder/bldr_enum_v1/`. Each micro-package represented an isolated enum definition or single status type (e.g., `pkg/specbuilder/bldr_enum_v1/shared_agent_instructions`, `pkg/specbuilder/bldr_enum_v1/policies`, `pkg/specbuilder/bldr_enum_v1/roles`).

This extreme fragmentation generated significant architectural debt (`TDE-CEF-F-ARCH-005`):
1. **Go AST and Package Graph Bloat**: 220 distinct packages inflated compiler symbol tables, package metadata parsing, and build graph traversal times.
2. **Dual Sources of Truth**: Duplicate type constants and status strings proliferated between `pkg/objects` and the individual enum packages.
3. **Orphaned Duplicate Directories**: Over 80 unreferenced plural directories existed alongside singular packages without active callers or consumers.

This document details the consolidated domain enum architecture, the directory pruning model, and the continuous deprecation path.

---

## Architecture Design

### 1. Domain Grouping Architecture

Rather than generating an isolated package for every entity or schema kind, enums are organized into cohesive domain-level packages:

- **`pkg/specbuilder/bldr_enum_v1/domain_kernel`**:
  Houses core kernel lifecycle, state, priority, and session enums:
  - `PlaneDraft`, `PlanePromoted`
  - `PriorityTierP0` .. `PriorityTierP3`
  - `AccountStatusActive`, `AccountStatusSuspended`
  - `QASuccessStatusSuccess`, `QASuccessStatusFailed`
  - `SessionStatusActive`, `SessionStatusArchived`, `SessionStatusCompleted`, etc.

- **`pkg/specbuilder/bldr_enum_v1/domain_platform`**:
  Houses platform operational, governance, and audit enums:
  - `AuditStatusActive`, `AuditStatusArchived`
  - `PolicyStatusActive`, `PolicyStatusInactive`
  - `RuleStatusImplemented`, `RuleStatusDraft`
  - `SchedulerJobStatusActive`, `SchedulerJobStatusPending`

Each domain package exports strongly-typed enums backed by string types and validated against canonical strings in `pkg/objects`.

### 2. Static Floor and Directory Pruning Invariant

To guarantee package bloat cannot regress, [`pkg/testkit/enum_consolidation_test.go`](../../pkg/testkit/enum_consolidation_test.go) defines:

```go
// TestEnumConsolidation_DirectoryCount_StaticFloor verifies CRIT-SPECBUILDER-P32-001.
// Prunes orphaned micro-package directories and enforces a strict ceiling on bldr_enum_v1.
func TestEnumConsolidation_DirectoryCount_StaticFloor(t *testing.T) {
    ...
    // Must be strictly <= 140 (reduced from 218 by eliminating orphaned micro-packages)
    assert.LessOrEqual(t, dirCount, 140, "bldr_enum_v1 package count must not exceed 140 (was 218, current: %d)", dirCount)
}
```

The 83 completely unreferenced orphaned directories were eliminated:
- `pkg/specbuilder/bldr_enum_v1/agent_architectures`
- `pkg/specbuilder/bldr_enum_v1/components`
- `pkg/specbuilder/bldr_enum_v1/certificates`
- `pkg/specbuilder/bldr_enum_v1/organizations`
- `pkg/specbuilder/bldr_enum_v1/personas`
- `pkg/specbuilder/bldr_enum_v1/policies`
- `pkg/specbuilder/bldr_enum_v1/roles`
- `pkg/specbuilder/bldr_enum_v1/rules`
- `pkg/specbuilder/bldr_enum_v1/tests`
- `pkg/specbuilder/bldr_enum_v1/workflows`
- and 73 additional orphaned directories.

Total directory count was reduced from 220 to 137, satisfying the `<= 140` static floor.

### 3. Fail-Closed Validation and Negative Boundary Testing

All domain enums implement fail-closed boundary checking. Any unmapped or unrecognized status string coerced to a domain enum type is rejected by validation assertions, preventing silent data corruption across boundaries:

```go
func TestEnumConsolidation_InvalidEnumRejection_NegativeBoundary(t *testing.T) {
    ...
    for _, invalid := range invalidStatuses {
        coerced := domain_kernel.ZqkSessionStatus(invalid)
        assert.False(t, validSessionStatuses[coerced], "invalid status %q must be rejected by domain enum validator", invalid)
    }
}
```

---

## Deprecation Path

1. **Phase 32 (Complete)**:
   - Pruned 83 unreferenced micro-packages.
   - Enforced `< 140` directory static floor via unit test invariant.
   - Verified clean compilation across all `pkg/specbuilder/...` packages.
2. **Phase 33-35 (Migration)**:
   - Progressively migrate active consumers of legacy `bldr_enum_v1/<entity>` to `domain_kernel` and `domain_platform`.
   - Deprecate remaining micro-packages with Go deprecation comments (`// Deprecated: Use domain_kernel or domain_platform instead.`).
3. **Phase 36 (Total Retirement)**:
   - Remove legacy micro-packages once all consumers point to consolidated domain packages.
   - Tighten static ceiling invariant from `<= 140` to `<= 5`.
