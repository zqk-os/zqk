# Policy Lifecycle: System Awareness and Recurring Considerations

**Version**: 1.0.0  
**Created**: 2025-01-02  
**Status**: Active  
**Purpose**: Documentation of policy lifecycle with system awareness and recurring review mechanisms

## Overview

The policy lifecycle establishes appropriate levels of system awareness and recurring considerations to ensure policies remain current, effective, and properly enforced throughout the project delivery lifecycle.

## Lifecycle Statuses

### Status Flow

```
draft → under_review → active → [deprecated | superseded | archived]
```

### Status Descriptions

1. **draft**: Policy being developed (initial status)
2. **under_review**: Policy submitted for review and approval
3. **active**: Policy is active and enforced
4. **deprecated**: Policy no longer applicable but kept for reference
5. **superseded**: Policy replaced by a new policy
6. **archived**: Policy archived (terminal status)

## System Awareness Mechanisms

### 1. Review Date Tracking

**Field**: `review_date`

**System Awareness**:
- Policies with `review_date` are monitored by the lifecycle reminder system
- Reminders triggered when review date approaches (configurable threshold)
- High-priority reminders when review date passes

**Configuration**:
```yaml
# .zqk/config.yaml
policy:
  reminders:
    review_threshold_days: 30  # Remind 30 days before review date
    overdue_severity: high      # Severity when review overdue
```

**Lifecycle Integration**:
- Review reminders use `reason_code: policy`
- Reminders include policy ID, review date, and suggested action
- Reminders can trigger policy status transition to `under_review`

### 2. Version Change Detection

**Field**: `version`

**System Awareness**:
- Version changes detected during object updates
- Automatic transition to `under_review` when version changes
- Ensures policy updates go through review process

**Lifecycle Transition**:
```yaml
- auto: true
  description: Policy version changed - trigger review
  from: active
  system: true
  condition: version_changed
  to: under_review
```

### 3. Effective Date Tracking

**Field**: `effective_date`

**System Awareness**:
- Policies in `under_review` automatically activate when effective date reached
- Ensures policies become active at the right time
- Supports phased policy rollouts

**Lifecycle Transition**:
```yaml
- auto: true
  description: Policy effective date reached - activate if in review
  from: under_review
  system: true
  condition: effective_date_reached
  to: active
```

### 4. Enforcement Configuration Monitoring

**Field**: `enforcement`

**System Awareness**:
- `enforcement.automated`: Triggers automated compliance checks
- `enforcement.reminder_enabled`: Enables lifecycle reminders
- `enforcement.severity`: Determines reminder priority
- `enforcement.review_required`: Flags policies requiring human review

**Integration Points**:
- Pre-commit hooks check `enforcement.automated` policies
- Lifecycle reminder system checks `enforcement.reminder_enabled` policies
- CI/CD pipeline respects `enforcement.review_required` flag

## Recurring Considerations

### 1. Periodic Policy Review

**Mechanism**: `review_date` field + lifecycle reminder system

**Process**:
1. Policy created with initial `review_date` (e.g., 6 months from creation)
2. Lifecycle reminder system monitors `review_date`
3. Reminder triggered when review date approaches
4. Policy owner reviews and updates policy
5. `review_date` updated for next review cycle

**Configuration**:
```yaml
# Default review intervals by policy type
policy:
  review_intervals:
    standard: 180 days      # 6 months
    requirement: 180 days    # 6 months
    guideline: 365 days      # 1 year
    best_practice: 365 days  # 1 year
    anti_pattern: 180 days  # 6 months
```

### 2. Policy Evolution Tracking

**Mechanism**: `version` field + lifecycle transitions

**Process**:
1. Policy updated (body, enforcement, applicability, etc.)
2. `version` field incremented (SemVer)
3. Automatic transition to `under_review`
4. Review and approval process
5. Transition to `active` with new version

**Version History**:
- Version changes tracked in object lifecycle
- `effective_date` updated when new version becomes active
- Previous versions remain accessible for reference

### 3. Policy Staleness Detection

**Mechanism**: Lifecycle reminder system + `updated_at` field

**Process**:
1. System monitors `updated_at` field
2. Policies not updated within threshold flagged
3. Reminder generated with `reason_code: policy`
4. Policy owner reviews and updates or confirms still valid

**Configuration**:
```yaml
policy:
  staleness_thresholds:
    standard: 90 days       # 3 months
    requirement: 90 days    # 3 months
    guideline: 180 days     # 6 months
    best_practice: 180 days # 6 months
```

### 4. Policy Supersession Tracking

**Mechanism**: `related_patterns` field + lifecycle status

**Process**:
1. New policy created that supersedes existing policy
2. Existing policy `related_patterns` updated to reference new policy
3. Existing policy status changed to `superseded`
4. System tracks policy lineage via `related_patterns`

**Lifecycle Transition**:
```yaml
- auto: false
  description: Policy superseded by new policy
  from: active
  manual: true
  preconditions:
    - related_patterns contains reference to superseding policy
  to: superseded
```

## Lifecycle Reminder Integration

### Reminder Types

1. **Review Due**: `review_date` approaching
   - Reason code: `policy`
   - Severity: `medium`
   - Message: "Policy {{id}} review due on {{review_date}}"

2. **Review Overdue**: `review_date` passed
   - Reason code: `policy`
   - Severity: `high`
   - Message: "Policy {{id}} review overdue (due {{review_date}})"

3. **Stale Policy**: Policy not updated recently
   - Reason code: `policy`
   - Severity: `low`
   - Message: "Policy {{id}} hasn't been updated in {{days}} days"

4. **Version Change**: Policy version updated
   - Reason code: `policy`
   - Severity: `medium`
   - Message: "Policy {{id}} version changed to {{version}} - review required"

### Reminder Queries

```bash
# Get all policy reminders
zqk lifecycle reminders --reason-code policy

# Get overdue policy reviews
zqk lifecycle reminders --reason-code policy --severity high

# Get policies due for review
zqk object list policy --filter "review_date<=2025-07-01"
```

## Policy Lifecycle Workflow

### Initial Creation

1. Policy created in `draft` status
2. Required fields populated (category, policy_type, body)
3. Policy submitted for review (`under_review`)
4. Policy approved and activated (`active`)

### Ongoing Maintenance

1. **Periodic Review**:
   - Review date approaches → reminder triggered
   - Policy reviewed and updated if needed
   - `review_date` updated for next cycle

2. **Policy Updates**:
   - Policy content updated
   - `version` incremented
   - Automatic transition to `under_review`
   - Review and reactivation

3. **Policy Deprecation**:
   - Policy no longer applicable
   - Status changed to `deprecated`
   - Policy kept for reference

4. **Policy Supersession**:
   - New policy created
   - Old policy `related_patterns` updated
   - Old policy status changed to `superseded`

## System Integration Points

### 1. Lifecycle Evaluation

Policies are evaluated during lifecycle evaluation:

```bash
# Evaluate policies
zqk lifecycle evaluate policy

# Evaluate specific policy
zqk lifecycle evaluate POL-ARCH-001
```

### 2. Lifecycle Reminders

Policy reminders integrated with lifecycle reminder system:

```bash
# Get policy reminders
zqk lifecycle reminders --reason-code policy
```

### 3. MCP Integration

Policies available via MCP:

```json
{
  "name": "get_policies",
  "arguments": {
    "category": "architecture",
    "policy_type": "standard"
  }
}
```

### 4. Automated Enforcement

Policies with `enforcement.automated: true` trigger automated checks:

- Pre-commit hooks
- CI/CD pipeline
- Build-time validation

## Configuration

### Policy Reminder Configuration

```yaml
# .zqk/config.yaml
policy:
  reminders:
    enabled: true
    check_interval: daily
    review_threshold_days: 30
    staleness_thresholds:
      standard: 90 days
      requirement: 90 days
      guideline: 180 days
      best_practice: 180 days
      anti_pattern: 90 days
    severity_levels:
      review_due: medium
      review_overdue: high
      stale: low
      version_change: medium
```

## Related Documentation

- [Policy Object Spec](../_internal/object_specs/policy.yaml)
- [Policy Lifecycle](../_internal/lifecycles/policy_lifecycle.yaml)
- [Project Policy System](./PROJECT_POLICY_SYSTEM.md)
- [Lifecycle Reminders](./ARCHITECTURE_LIFECYCLE_REMINDERS.md)

---

*The policy lifecycle ensures policies remain current, effective, and properly enforced through system awareness and recurring review mechanisms.*

