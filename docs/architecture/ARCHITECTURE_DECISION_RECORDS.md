# Architecture Decision Records (ADRs)

**Version**: 1.0.0  
**Created**: 2025-01-02  
**Status**: Active  
**Purpose**: Guide for creating and managing Architecture Decision Records using the decision object spec

## Overview

Architecture Decision Records (ADRs) document important architectural decisions and their rationale. In zqk, ADRs are implemented as `decision` objects with specific metadata to distinguish them from other types of decisions.

## ADR as Decision Object

ADRs use the existing `decision` object spec with additional conventions:

### Required Fields

- **`kind`**: `decision` (standard)
- **`title`**: Descriptive title starting with "ADR-" or "Architecture Decision:"
- **`context`**: Problem statement and context
- **`rationale`**: Decision and rationale
- **`impact`**: Consequences (positive and negative)
- **`status`**: `proposed` | `accepted` | `rejected` | `deprecated` | `superseded`

### ADR-Specific Conventions

1. **ID Format**: Use `ADR-XXX` format (e.g., `ADR-001`)
2. **Category Tagging**: Include `category: architecture` in metadata
3. **Related References**: Link to related backlog items, goals, or other ADRs
4. **Revisit Date**: Set `revisit` field for decisions that should be reassessed

### Example ADR

```yaml
id: ADR-001
kind: decision
title: Architecture Decision: CLI Bridge Pattern for MCP Exposure
status: accepted
context: |
  Need to expose functionality via MCP without duplicating code or creating
  import cycles. Multiple approaches considered: separate MCP bridges,
  manual tool registration, or automatic CLI command discovery.
rationale: |
  Decision: Use CLI bridge pattern that automatically discovers CLI commands
  and exposes them as MCP tools.
  
  Rationale:
  - Single source of truth (CLI commands)
  - No code duplication
  - Automatic privilege filtering
  - Context-driven bootstrap
  - No import cycles
impact: |
  Positive:
  - All functionality automatically available via MCP
  - Consistent architecture pattern
  - Reduced maintenance burden
  
  Negative:
  - CLI commands must be well-designed for MCP exposure
  - Some edge cases may require special handling
revisit: "2026-01-02T00:00:00Z"
goal_refs:
  - GOAL-XXXX
milestone_refs:
  - MIL-XXX
decision_refs:
  - DEC-012  # Related decision about MCP strategy
schema_version: 2.0.0
```

## ADR Lifecycle

ADRs use the standard decision lifecycle with the following status flow:

### Status Mapping

- **draft** → ADR being written
- **under_review** → ADR proposed for review
- **active** → ADR accepted and in effect
- **approved** → ADR formally approved (equivalent to "accepted")
- **rejected** → ADR rejected
- **archived** → ADR deprecated or superseded

### 1. Draft

When an architectural decision is being considered:

```bash
zqk object create decision --file adr-draft.yaml
```

Status: `draft`

### 2. Under Review

Submit ADR for review:

```bash
zqk object update ADR-001 --field status=under_review
```

Status: `under_review`

### 3. Active/Approved

After review and approval:

```bash
# Option 1: Active (in effect)
zqk object update ADR-001 --field status=active
zqk object update ADR-001 --field revisit=2026-01-02T00:00:00Z

# Option 2: Approved (formally approved)
zqk object update ADR-001 --field status=approved
```

Status: `active` or `approved`

### 4. Deprecated/Superseded

When a decision is no longer applicable or replaced:

```bash
# Option 1: Link to superseding ADR (keep status active/approved)
zqk object update ADR-001 --field "decision_refs=[ADR-002]"
zqk object update ADR-001 --field "impact=This ADR is superseded by ADR-002. See ADR-002 for replacement."

# Option 2: Archive
zqk object update ADR-001 --field status=archived
```

Status: `active`/`approved` (with reference) or `archived`

**Note**: The lifecycle reminder system monitors the `revisit` field for periodic reassessment. See [Decision Lifecycle](./DECISION_LIFECYCLE.md) for details.

## Querying ADRs

```bash
# List all ADRs
zqk object list decision --filter "title~ADR-"

# Find accepted ADRs
zqk object list decision --filter "title~ADR-" --filter status=accepted

# Find ADRs due for revisit
zqk object list decision --filter "title~ADR-" --filter "revisit<=2025-12-31"
```

## Integration with Architecture Patterns

ADRs should reference architecture patterns:

```yaml
decision_refs:
  - ADR-001  # CLI Bridge Pattern
  - ADR-002  # Context-Driven Bootstrap
```

Patterns should reference ADRs:

```markdown
## Pattern 1: CLI Bridge for MCP Exposure

**ADR**: [ADR-001](../decisions/ADR-001.yaml)
**Status**: Accepted
```

## Related Documentation

- [Decision Object Spec](../../_internal/object_specs/decision.yaml)
- [Decision Lifecycle](./DECISION_LIFECYCLE.md) - System awareness and recurring considerations
- [Architecture Patterns Library](./ARCHITECTURE_PATTERNS.md)
- [Architecture Review Process](./ARCHITECTURE_REVIEW_PROCESS.md)

---

*ADRs provide traceability and context for architectural decisions. All significant architecture changes should have an associated ADR.*

