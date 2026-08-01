# System Check Monitoring and Awareness

**Version**: 1.0.0  
**Created**: 2025-01-02  
**Status**: Active  
**Purpose**: Proactive monitoring and awareness of system check violations to prevent system degradation

## Overview

System check violations must be proactively monitored and project resources must be made aware when violations occur or accumulate. This document describes the monitoring mechanisms, lifecycle reminder integration, and notification channels.

## Policy Reference

- **POL-CODE-004**: System Check Violations - Prevention and Resolution
- **POL-CODE-005**: Proactive System Check Monitoring and Awareness

## Monitoring Mechanisms

### 1. Automated System Check Execution

**CI/CD Integration**:
```yaml
# .github/workflows/system-check.yml — see repo for full file (branch filters, permissions).
# Companion: .github/workflows/go-quality.yml (go mod verify, build ./..., vet ./..., golangci-lint).
name: System Integrity Check
on: [push, pull_request]
jobs:
  system-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26.0'
      - name: Build zqk
        run: go build -o zqk ./cmd/zqk
      - name: Run system integrity check
        run: ./scripts/check-system-integrity.sh --fail-on-tier1 --fail-on-tier2
```

**Pre-Commit Hooks**:
```bash
# tools/git-hooks/pre-commit (implemented)
# Checks for Tier 1 violations before commit
# Full check with all tiers runs in CI/CD
./scripts/check-system-integrity.sh --fail-on-tier1 --quiet
```

**Script Usage**:
```bash
# Check system integrity (exits with error if Tier 1 violations found)
./scripts/check-system-integrity.sh --fail-on-tier1

# Also fail on Tier 2 threshold violations
./scripts/check-system-integrity.sh --fail-on-tier1 --fail-on-tier2

# JSON output for CI/CD parsing
./scripts/check-system-integrity.sh --fail-on-tier1 --json

# Quiet mode (minimal output)
./scripts/check-system-integrity.sh --fail-on-tier1 --quiet
```

**Periodic Checks**:
- Daily automated checks via scheduled jobs
- Weekly violation trend reports
- Monthly compliance reviews

### 2. Violation Thresholds

**Tier 1 (Blocking)**:
- **Threshold**: Zero tolerance
- **Action**: Immediate blocking, high-priority reminder
- **Delivery**: CLI, CI/CD, lifecycle reminders

**Tier 2 (Warnings)**:
- **Threshold**: 5 violations
- **Action**: High-priority reminder, review required
- **Delivery**: Lifecycle reminders, periodic reports

**Tier 3 (Informational)**:
- **Threshold**: 50 violations
- **Action**: Medium-priority reminder, review recommended
- **Delivery**: Lifecycle reminders, periodic reports

**Accumulation Rate**:
- **Threshold**: >10 new violations per day
- **Action**: Alert to project maintainers
- **Delivery**: High-priority lifecycle reminder

### 3. Lifecycle Reminder Integration

**Reason Code**: `system_check`

**Reminder Types**:
1. **Tier 1 Violations**: Critical reminders for blocking issues
2. **Tier 2 Accumulation**: Warnings when threshold exceeded
3. **Tier 3 Accumulation**: Informational when threshold exceeded
4. **Rapid Accumulation**: Alerts for rapid violation growth

**Reminder Generation**:
```go
// System check violation reminder
type SystemCheckReminder struct {
    ReasonCode    string   // "system_check"
    Severity      string   // "critical", "high", "medium", "low"
    Tier1Count    int      // Number of Tier 1 violations
    Tier2Count    int      // Number of Tier 2 violations
    Tier3Count    int      // Number of Tier 3 violations
    AccumulationRate float64 // Violations per day
    Message       string   // Human-readable message
    SuggestedAction string // Recommended action
    DetailsPath   string   // Path to detailed report
}
```

**Reminder Triggers**:
- System check execution detects violations
- Violation thresholds exceeded
- Rapid accumulation detected
- Periodic monitoring runs

### 4. Notification Channels

**CLI**:
```bash
# Check for system check violation reminders
zqk lifecycle reminders --reason-code system_check

# High-priority violations
zqk lifecycle reminders --reason-code system_check --severity high

# View detailed violation report
zqk system check --format table
```

**MCP Integration**:
```json
{
  "name": "get_lifecycle_reminders",
  "arguments": {
    "reason_code": "system_check",
    "severity": "high"
  }
}
```

**Lifecycle Reminders**:
- Proactive notifications for accumulated violations
- Threshold-based alerts
- Periodic summary reports

**CI/CD**:
- Pre-commit hooks block Tier 1 violations
- Pull request checks validate all tiers
- Build failures on Tier 1 violations

**Periodic Reports**:
- Daily automated checks
- Weekly violation trend reports
- Monthly compliance reviews

## Implementation

### 1. Lifecycle Reminder Generation

**Location**: `cmd/zqk/system/check_impl.go` ✅ **IMPLEMENTED**

**Implementation**: The `generateSystemCheckReminders()` function is called during `outputTable()` to generate reminders based on violation counts.

**Process**:
1. Run system check
2. Analyze violation counts by tier (separated into public and internal)
3. Check thresholds per POL-CODE-005:
   - Tier 1: Always generate critical reminder (zero tolerance)
   - Tier 2: Generate high-priority reminder when ≥5 violations
   - Tier 3: Generate medium-priority reminder when ≥50 violations
4. Display reminders in system check output
5. Ready for integration with lifecycle reminder system

**Current Behavior**:
- Reminders are displayed in the system check output
- Includes reason code (`system_check`), severity, violation counts, and suggested actions
- Provides query commands for future lifecycle reminder system integration

**Example Output**:
```
=== System Check Reminders (POL-CODE-005) ===
⚠️  Lifecycle reminders generated for system check violations:

❌ [critical] 1 Tier 1 blocking violation(s) detected
   Reason Code: system_check
   Tier 1: 1
   Action: Run 'zqk system check --tier 1' to view and resolve
   Query: zqk lifecycle reminders --reason-code system_check --severity critical
```

### 2. Integration Points

**System Check Command**:
- Generate reminders after check execution
- Store reminders in lifecycle system
- Return reminder summary in output

**Lifecycle Evaluation**:
- Include system check reminders in evaluation
- Trigger reminders on object status changes
- Periodic reminder generation

**Pre-Commit Hooks**:
- Check for Tier 1 violations
- Block commit if violations detected
- Generate immediate reminder

**CI/CD Pipeline**:
- Run system check in pipeline
- Generate reminders for violations
- Block merge on Tier 1 violations

### 3. Reminder Delivery

**CLI**:
```bash
# View system check reminders
zqk lifecycle reminders --reason-code system_check

# Filter by severity
zqk lifecycle reminders --reason-code system_check --severity critical

# View with details
zqk lifecycle reminders --reason-code system_check --verbose
```

**MCP**:
```json
{
  "name": "get_lifecycle_reminders",
  "arguments": {
    "reason_code": "system_check",
    "severity": "high",
    "include_low_priority": false
  }
}
```

**Periodic Reports**:
- Weekly summary of violation trends
- Monthly compliance review
- Quarterly policy review

## Configuration

### Reminder Thresholds

```yaml
# .zqk/config.yaml
system_check:
  monitoring:
    enabled: true
    check_interval: daily
    thresholds:
      tier1: 0          # Zero tolerance
      tier2: 5         # High-priority reminder
      tier3: 50        # Medium-priority reminder
      accumulation_rate: 10  # Violations per day
    reminder_severity:
      tier1: critical
      tier2: high
      tier3: medium
      accumulation: high
```

### Notification Channels

```yaml
# .zqk/config.yaml
system_check:
  notifications:
    cli: true
    mcp: true
    lifecycle_reminders: true
    ci_cd: true
    periodic_reports: true
```

## Benefits

1. **Proactive**: Violations detected before they accumulate
2. **Awareness**: Project resources notified of violations
3. **Prevention**: Thresholds prevent system degradation
4. **Actionable**: Clear guidance on resolution
5. **Integrated**: Works with existing lifecycle reminder system

## Related Documentation

- [POL-CODE-004](../policies/POL-CODE-004.yaml): System Check Violations - Prevention and Resolution
- [POL-CODE-005](../policies/POL-CODE-005.yaml): Proactive System Check Monitoring and Awareness
- [Architecture Lifecycle Reminders](./ARCHITECTURE_LIFECYCLE_REMINDERS.md)
- [Lifecycle Reminder System](../../backlog/BLI-092.yaml)

---

*System check monitoring ensures violations are detected early and project resources are made aware before the system reaches a degraded state.*

