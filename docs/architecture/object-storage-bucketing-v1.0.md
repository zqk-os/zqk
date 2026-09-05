# Object Storage Bucketing Strategy v1.0

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Status**: Design  
**Date**: 2025-12-25  
**Related**: BLI-626, audit_event spec

## Problem Statement

Some system objects generate many instances over time (e.g., `audit_event`, `change_journal_entry`). Storing all instances in a single flat directory becomes unwieldy:

- **Performance**: File system performance degrades with thousands of files in one directory
- **Usability**: Difficult to browse, search, and manage large directories
- **Maintenance**: Hard to archive, backup, or clean up old data
- **Git**: Large flat directories cause git performance issues

## Bucketing Strategy

### When to Bucket

Objects should be bucketed when they meet **any** of these criteria:

1. **High Volume**: Expected to generate >100 instances per month
2. **Time-Series**: Primarily queried by time range
3. **Append-Only**: Immutable or rarely modified after creation
4. **Audit/Log Nature**: Used for compliance, security, or debugging

**Examples**: `audit_event`, `change_journal_entry`, `integrity_manifest`

### Bucketing Patterns

#### Pattern 1: Chronological (Primary)

**Structure**: `{kind}/{YYYY-MM-DD}/`

```
docs/process/audit/
├── 2025-12-25/
│   ├── AUD-001.yaml
│   ├── AUD-002.yaml
│   └── AUD-003.yaml
├── 2025-12-26/
│   ├── AUD-004.yaml
│   └── AUD-005.yaml
└── 2025-12-27/
    └── AUD-006.yaml
```

**Use When**:
- Objects are primarily queried by date/time
- Natural archival boundaries (daily/monthly)
- Time-based reporting and analysis

**Pros**:
- Natural time-based queries
- Easy to archive old data (move entire date directories)
- Clear temporal boundaries
- Git-friendly (changes isolated to date directories)

**Cons**:
- Need date to locate file (but `created_at` is always available)
- Uneven distribution (some days may have many events)

#### Pattern 2: Categorical (Secondary)

**Structure**: `{kind}/{category}/`

```
docs/process/audit/
├── hash_mismatch_fix/
│   ├── AUD-001.yaml
│   ├── AUD-002.yaml
│   └── AUD-003.yaml
├── integrity_recovery/
│   ├── AUD-004.yaml
│   └── AUD-005.yaml
└── security_alert/
    └── AUD-006.yaml
```

**Use When**:
- Objects are primarily queried by category/type
- Categories have distinct access patterns
- Different retention policies per category

**Pros**:
- Easy to find related events
- Good for filtering and reporting
- Can apply category-specific policies

**Cons**:
- Uneven distribution (some categories may be huge)
- Need to know category to locate file

#### Pattern 3: Hybrid (Recommended for Audit Events)

**Structure**: `{kind}/{YYYY-MM}/{event_type}/` or `{kind}/{YYYY-MM-DD}/{event_type}/`

```
docs/process/audit/
├── 2025-12/
│   ├── hash_mismatch_fix/
│   │   ├── AUD-001.yaml
│   │   └── AUD-002.yaml
│   ├── integrity_recovery/
│   │   └── AUD-003.yaml
│   └── security_alert/
│       └── AUD-004.yaml
└── 2026-01/
    └── hash_mismatch_fix/
        └── AUD-005.yaml
```

**Use When**:
- High volume with both time and category queries
- Need both temporal and categorical organization
- Want to balance distribution

**Pros**:
- Best of both worlds (time + category)
- Even distribution (date prevents category explosion)
- Easy to query by date range or category
- Natural archival boundaries

**Cons**:
- More complex path structure
- Need both date and category to locate file

#### Pattern 4: ID-Based Sharding (For Very High Volume)

**Structure**: `{kind}/{id_prefix}/`

```
docs/process/audit/
├── 00/
│   ├── AUD-0001.yaml
│   └── AUD-0002.yaml
├── 01/
│   └── AUD-0100.yaml
└── FF/
    └── AUD-FF00.yaml
```

**Use When**:
- Extremely high volume (>10,000 instances)
- Even distribution needed
- ID-based lookups are primary access pattern

**Pros**:
- Even distribution
- Fast ID-based lookups
- Scales to millions of objects

**Cons**:
- Not human-readable
- Hard to browse
- Doesn't support time-based queries well

## Recommended Strategy for Audit Events

### Primary: Chronological by Month

**Structure**: `docs/process/audit/{YYYY-MM}/`

```
docs/process/audit/
├── 2025-12/
│   ├── AUD-001.yaml
│   ├── AUD-002.yaml
│   └── AUD-003.yaml
└── 2026-01/
    ├── AUD-004.yaml
    └── AUD-005.yaml
```

**Rationale**:
1. **Time-Series Nature**: Audit events are primarily queried by time range
2. **Monthly Granularity**: Balances directory count vs. file count per directory
3. **Natural Archival**: Easy to archive/compress old months
4. **Git-Friendly**: Monthly boundaries align with git commit patterns
5. **Query Patterns**: Most queries are "events in last N days/months"

### Optional: Category Sub-Buckets (If Needed)

If a single month exceeds ~1000 events, add category sub-buckets:

**Structure**: `docs/process/audit/{YYYY-MM}/{event_type}/`

```
docs/process/audit/
├── 2025-12/
│   ├── hash_mismatch_fix/
│   │   ├── AUD-001.yaml
│   │   └── AUD-002.yaml
│   └── integrity_recovery/
│       └── AUD-003.yaml
```

**Threshold**: Add category sub-buckets when a month exceeds 1000 events

## Implementation

### Path Generation

```go
func GetAuditEventPath(projectRoot string, event *AuditEvent) string {
    // Extract date from created_at
    created, _ := time.Parse("2006-01-02T15:04:05Z", event.CreatedAt)
    month := created.Format("2006-01")
    
    baseDir := filepath.Join(projectRoot, "docs", "process", "audit", month)
    
    // Check if month directory has >1000 files
    if countFiles(baseDir) > 1000 {
        // Use category sub-bucket
        return filepath.Join(baseDir, event.EventType, fmt.Sprintf("%s.yaml", event.ID))
    }
    
    // Use flat monthly structure
    return filepath.Join(baseDir, fmt.Sprintf("%s.yaml", event.ID))
}
```

### Query Support

The bucketing strategy should be transparent to queries:

```go
// Query by date range (automatically scans relevant month directories)
func QueryAuditEvents(startDate, endDate time.Time) []AuditEvent {
    months := getMonthsInRange(startDate, endDate)
    var events []AuditEvent
    
    for _, month := range months {
        monthDir := filepath.Join(projectRoot, "docs", "process", "audit", month)
        events = append(events, scanMonthDirectory(monthDir)...)
    }
    
    return events
}

// Query by event type (scans all months, filters by type)
func QueryAuditEventsByType(eventType string) []AuditEvent {
    // Scan all month directories
    // Filter by event_type field
}
```

## Migration Strategy

For existing flat structures:

1. **New Objects**: Use bucketed structure immediately
2. **Existing Objects**: Migrate on-demand or in batches
3. **Backward Compatibility**: Support both flat and bucketed paths during transition

## Configuration

Bucketing strategy can be configured per object kind:

```yaml
# .zqk/config.yaml
storage:
  bucketing:
    audit_event:
      strategy: "chronological_monthly"
      category_threshold: 1000  # Add category sub-buckets if >1000 files/month
    change_journal_entry:
      strategy: "chronological_daily"
```

## Benefits

1. **Performance**: Faster file system operations with smaller directories
2. **Usability**: Easier to browse and manage
3. **Archival**: Simple to archive old data (compress/remove month directories)
4. **Git**: Better git performance with smaller change sets
5. **Scalability**: Supports growth to thousands of events per month
6. **Query Efficiency**: Time-based queries only scan relevant directories

