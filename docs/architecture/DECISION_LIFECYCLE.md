# Decision Lifecycle: System Awareness and Recurring Considerations

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-01-02  
**Status**: Active  
**Purpose**: Documentation of decision lifecycle with system awareness and recurring review mechanisms, including ADR-specific considerations

## Overview

The decision lifecycle establishes appropriate levels of system awareness and recurring considerations to ensure decisions (including Architecture Decision Records) remain current, relevant, and properly tracked throughout the project delivery lifecycle.

## Lifecycle Statuses

### Status Flow

```
draft → under_review → active → [approved | rejected | archived]
```

### Status Descriptions

1. **draft**: Decision being developed (initial status)
2. **under_review**: Decision submitted for review
3. **active**: Decision is active and in effect
4. **approved**: Decision formally approved (terminal status)
5. **rejected**: Decision rejected (terminal status)
6. **archived**: Decision archived (terminal status)

### ADR Status Mapping

For Architecture Decision Records (ADRs), the lifecycle statuses map to ADR conventions:

- **draft** → ADR being written
- **under_review** → ADR proposed for review
- **active** → ADR accepted and in effect
- **approved** → ADR formally approved (equivalent to "accepted")
- **rejected** → ADR rejected
- **archived** → ADR deprecated or superseded

**Note**: ADRs can also use `decision_refs` to link to superseding ADRs when deprecated or superseded, while keeping status as `active` or `approved`.

## System Awareness Mechanisms

### 1. Revisit Date Tracking

**Field**: `revisit`

**System Awareness**:
- Decisions with `revisit` field are monitored by the lifecycle reminder system
- Reminders triggered when revisit date approaches (configurable threshold)
- High-priority reminders when revisit date passes

**Configuration**:
```yaml
# .zqk/config.yaml
decision:
  reminders:
    revisit_threshold_days: 30  # Remind 30 days before revisit date
    overdue_severity: high      # Severity when revisit overdue
```

**Lifecycle Integration**:
- Revisit reminders use `reason_code: decision`
- Reminders include decision ID, revisit date, and suggested action
- Reminders can trigger decision status transition to `under_review` for reassessment

**Example**:
```yaml
id: ADR-001
kind: decision
title: Architecture Decision: CLI Bridge Pattern
status: active
revisit: "2026-01-02T00:00:00Z"  # System will remind when approaching
```

### 2. Decision Dependency Tracking

**Field**: `decision_refs`

**System Awareness**:
- Decisions can reference other decisions via `decision_refs`
- System tracks decision dependencies
- When a decision is deprecated or superseded, dependent decisions are flagged for review

**Lifecycle Integration**:
- Dependent decisions can be queried: `zqk object related ADR-001 --via decision_refs`
- System can generate reminders when referenced decisions change status

### 3. Impact Tracking

**Field**: `impact`, `impact_level`

**System Awareness**:
- `impact_level` (primary, secondary, observed) determines reminder priority
- Primary impact decisions trigger higher-priority reminders
- Impact changes tracked for decision evolution

**Lifecycle Integration**:
- Impact level influences reminder severity
- Primary impact decisions reviewed more frequently

## Recurring Considerations

### 1. Periodic Decision Revisit

**Mechanism**: `revisit` field + lifecycle reminder system

**Process**:
1. Decision created with `revisit` date (e.g., 1 year from creation)
2. Lifecycle reminder system monitors `revisit` field
3. Reminder triggered when revisit date approaches
4. Decision owner reviews and reassesses decision
5. `revisit` date updated for next review cycle, or decision status updated

**Configuration**:
```yaml
# Default revisit intervals by decision type
decision:
  revisit_intervals:
    architecture: 365 days      # 1 year for ADRs
    governance: 180 days        # 6 months for governance decisions
    process: 180 days           # 6 months for process decisions
    default: 365 days           # 1 year default
```

### 2. Decision Evolution Tracking

**Mechanism**: Status transitions + `decision_refs` field

**Process**:
1. Decision updated (rationale, impact, context, etc.)
2. Status may transition to `under_review` for reassessment
3. If decision is superseded, new decision created and linked via `decision_refs`
4. Original decision status updated or kept as `active` with reference to new decision

**ADR Supersession Example**:
```yaml
# Original ADR
id: ADR-001
status: active
decision_refs:
  - ADR-002  # Superseding ADR

# New ADR
id: ADR-002
status: active
decision_refs:
  - ADR-001  # References original ADR
```

### 3. Decision Staleness Detection

**Mechanism**: Lifecycle reminder system + `updated_at` field

**Process**:
1. System monitors `updated_at` field
2. Decisions not updated within threshold flagged
3. Reminder generated with `reason_code: decision`
4. Decision owner reviews and updates or confirms still valid

**Configuration**:
```yaml
decision:
  staleness_thresholds:
    architecture: 180 days      # 6 months for ADRs
    governance: 90 days         # 3 months for governance decisions
    process: 90 days            # 3 months for process decisions
```

### 4. Decision Dependency Review

**Mechanism**: `decision_refs` field + lifecycle evaluation

**Process**:
1. When a decision changes status (e.g., deprecated, superseded)
2. System identifies dependent decisions via `decision_refs`
3. Reminders generated for dependent decisions to review impact
4. Dependent decisions may need status updates or reassessment

## ADR-Specific Considerations

### ADR Lifecycle Workflow

1. **Draft**: ADR being written
   ```bash
   zqk object create decision --file adr-draft.yaml
   # Status: draft
   ```

2. **Under Review**: ADR proposed for review
   ```bash
   zqk object update ADR-001 --field status=under_review
   ```

3. **Active**: ADR accepted and in effect
   ```bash
   zqk object update ADR-001 --field status=active
   zqk object update ADR-001 --field revisit=2026-01-02T00:00:00Z
   ```

4. **Approved**: ADR formally approved
   ```bash
   zqk object update ADR-001 --field status=approved
   ```

5. **Deprecated/Superseded**: ADR no longer applicable
   ```bash
   # Option 1: Update status and link to new ADR
   zqk object update ADR-001 --field "decision_refs=[ADR-002]"
   # Keep status as active or approved with reference
   
   # Option 2: Archive
   zqk object update ADR-001 --field status=archived
   ```

### ADR Revisit Process

ADRs should be revisited periodically:

```bash
# Set revisit date when ADR is accepted
zqk object update ADR-001 --field revisit=2026-01-02T00:00:00Z

# Query ADRs due for revisit
zqk object list decision --filter "title~ADR-" --filter "revisit<=2025-12-31"

# Get revisit reminders
zqk lifecycle reminders --reason-code decision
```

## Lifecycle Reminder Integration

### Reminder Types

1. **Revisit Due**: `revisit` date approaching
   - Reason code: `decision`
   - Severity: `medium`
   - Message: "Decision {{id}} revisit due on {{revisit}}"

2. **Revisit Overdue**: `revisit` date passed
   - Reason code: `decision`
   - Severity: `high`
   - Message: "Decision {{id}} revisit overdue (due {{revisit}})"

3. **Stale Decision**: Decision not updated recently
   - Reason code: `decision`
   - Severity: `low`
   - Message: "Decision {{id}} hasn't been updated in {{days}} days"

4. **Dependency Change**: Referenced decision changed
   - Reason code: `decision`
   - Severity: `medium`
   - Message: "Decision {{id}} references decision {{ref_id}} which has changed status"

### Reminder Queries

```bash
# Get all decision reminders
zqk lifecycle reminders --reason-code decision

# Get overdue decision revisits
zqk lifecycle reminders --reason-code decision --severity high

# Get ADRs due for revisit
zqk object list decision --filter "title~ADR-" --filter "revisit<=2025-12-31"
```

## Decision Lifecycle Workflow

### Initial Creation

1. Decision created in `draft` status
2. Required fields populated (context, rationale, impact)
3. Decision submitted for review (`under_review`)
4. Decision approved and activated (`active` or `approved`)

### Ongoing Maintenance

1. **Periodic Revisit**:
   - Revisit date approaches → reminder triggered
   - Decision reviewed and reassessed
   - `revisit` date updated for next cycle, or decision status updated

2. **Decision Updates**:
   - Decision content updated
   - Status may transition to `under_review` for reassessment
   - Revisit date updated if needed

3. **Decision Supersession**:
   - New decision created
   - Old decision `decision_refs` updated
   - Old decision status updated or kept with reference

4. **Decision Deprecation**:
   - Decision no longer applicable
   - Status updated or archived
   - Impact field updated to note deprecation

## System Integration Points

### 1. Lifecycle Evaluation

Decisions are evaluated during lifecycle evaluation:

```bash
# Evaluate decisions
zqk lifecycle evaluate decision

# Evaluate specific decision
zqk lifecycle evaluate ADR-001
```

### 2. Lifecycle Reminders

Decision reminders integrated with lifecycle reminder system:

```bash
# Get decision reminders
zqk lifecycle reminders --reason-code decision
```

### 3. MCP Integration

Decisions available via MCP:

```json
{
  "name": "get_decisions",
  "arguments": {
    "title": "ADR-",
    "status": "active"
  }
}
```

### 4. Decision Dependency Graph

Decisions can be queried for dependencies:

```bash
# Find decisions that reference this decision
zqk object related ADR-001 --via decision_refs

# Find decisions this decision references
zqk object get ADR-001 --format json | jq '.decision_refs'
```

## Configuration

### Decision Reminder Configuration

```yaml
# .zqk/config.yaml
decision:
  reminders:
    enabled: true
    check_interval: daily
    revisit_threshold_days: 30
    staleness_thresholds:
      architecture: 180 days
      governance: 90 days
      process: 90 days
      default: 180 days
    severity_levels:
      revisit_due: medium
      revisit_overdue: high
      stale: low
      dependency_change: medium
```

## Related Documentation

- [Decision Object Spec](../_internal/object_specs/decision.yaml)
- [Decision Lifecycle](../_internal/lifecycles/decision_lifecycle.yaml)
- [Architecture Decision Records](./ARCHITECTURE_DECISION_RECORDS.md)
- [Policy Lifecycle](./POLICY_LIFECYCLE.md)

---

*The decision lifecycle ensures decisions (including ADRs) remain current, relevant, and properly tracked through system awareness and recurring review mechanisms.*

