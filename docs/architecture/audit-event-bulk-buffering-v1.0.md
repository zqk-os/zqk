# Audit Event Bulk Operation Buffering v1.0

**Last Verified:** 2026-08-31


**Date:** 2025-12-25  
**Status:** Design  
**Related:** BLI-633, audit-event-aggregation-v1.0.md

## Problem Statement

During bulk operations (e.g., `zqk check all --force`), the system creates individual audit events for each operation. This leads to:
- **Storage bloat**: 100 hash mismatch fixes = 100 individual audit event files
- **Performance overhead**: 100 file writes during bulk operation
- **Query complexity**: Hard to see "how many times was BLI-626 fixed?" without aggregating
- **Noise**: Individual events for repeated operations on the same object

## Solution: Buffering and Occurrence Tracking

Buffer audit events during bulk operations and update existing events with occurrence counts and timestamps, similar to `status_history` append-only pattern.

## Design Principles

1. **Append-Only Updates**: Audit events are immutable except for occurrence tracking fields
2. **Key-Based Lookup**: Events are keyed by `target_id + event_type` (or `event_type` if no target_id)
3. **Bulk Operation Awareness**: Only buffer during bulk operations (detected by context)
4. **Backward Compatible**: Existing events work without occurrence fields

## Audit Event Schema Updates

Add occurrence tracking fields to `audit_event` spec:

```yaml
occurrence_count:
  type: integer
  semantic_type: quantity
  traits: [readable, writable]
  validation:
    required: false
    min: 1
  checklist:
    purpose: Number of times this event occurred (for bulk operations).
    system_usage: ["bulk operation tracking", "frequency analysis"]
    criticality: association
    cardinality: zero_or_one
    lifecycle: mutable (incremented during bulk operations)
    authority: automation
    validation: Must be >= 1 if present.
    dependencies: only present when event occurred multiple times
    default: "null (assumed to be 1 if not present)"
    observability: yes
    security: non-sensitive
    automation_hooks: used to track repeated operations in bulk contexts.
    field_profile_code: AUE-017

occurrence_timestamps:
  type: list
  semantic_type: statement
  traits: [readable, writable]
  validation:
    required: false
    items:
      type: string
      pattern: "^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}Z$"
  checklist:
    purpose: List of timestamps when this event occurred (for bulk operations).
    system_usage: ["temporal analysis", "bulk operation tracking"]
    criticality: association
    cardinality: zero_or_one
    lifecycle: mutable (append-only, new timestamps added)
    authority: automation
    validation: List of ISO 8601 timestamps.
    dependencies: only present when occurrence_count > 1
    default: "null (assumed to be [created_at] if not present)"
    observability: yes
    security: non-sensitive
    automation_hooks: used to track when repeated operations occurred.
    field_profile_code: AUE-018
```

**Lifecycle Exception**: These fields are mutable (append-only), which is an exception to the "immutable" rule for audit events. This is acceptable because:
- Only occurrence data is updated, not the core event data
- Similar to `status_history` append-only pattern
- Maintains audit trail integrity (all occurrences are tracked)

## Implementation Strategy

### 1. Audit Event Buffer

```go
type AuditEventBuffer struct {
    mu          sync.RWMutex
    buffer      map[string]*BufferedEvent // key -> event
    projectRoot string
    ctx         *context.Context
}

type BufferedEvent struct {
    Key           string // "target_id:event_type" or "event_type"
    EventType     string
    TargetID      string
    TargetKind    string
    TargetPath    string
    Operation     string
    Severity      string
    Metadata      map[string]interface{}
    Occurrences   []time.Time // Timestamps of occurrences
    FirstOccurrence time.Time
    LastOccurrence  time.Time
}

// Key generation
func (b *AuditEventBuffer) generateKey(targetID, eventType string) string {
    if targetID != "" {
        return fmt.Sprintf("%s:%s", targetID, eventType)
    }
    return eventType
}
```

### 2. Buffer Operations

```go
// Add event to buffer (or update existing)
func (b *AuditEventBuffer) Add(eventType, targetID, targetKind, targetPath, operation, severity string, metadata map[string]interface{}) {
    key := b.generateKey(targetID, eventType)
    
    b.mu.Lock()
    defer b.mu.Unlock()
    
    if existing, ok := b.buffer[key]; ok {
        // Update existing: increment count, append timestamp
        existing.Occurrences = append(existing.Occurrences, time.Now().UTC())
        existing.LastOccurrence = time.Now().UTC()
        // Merge metadata (latest wins for conflicting keys)
        for k, v := range metadata {
            existing.Metadata[k] = v
        }
    } else {
        // Create new buffered event
        now := time.Now().UTC()
        b.buffer[key] = &BufferedEvent{
            Key:            key,
            EventType:      eventType,
            TargetID:       targetID,
            TargetKind:     targetKind,
            TargetPath:     targetPath,
            Operation:      operation,
            Severity:       severity,
            Metadata:       metadata,
            Occurrences:    []time.Time{now},
            FirstOccurrence: now,
            LastOccurrence:  now,
        }
    }
}

// Flush buffer: create/update audit events
func (b *AuditEventBuffer) Flush() error {
    b.mu.Lock()
    defer b.mu.Unlock()
    
    for key, buffered := range b.buffer {
        // Check if audit event already exists
        existingEvent, err := b.findExistingAuditEvent(buffered.TargetID, buffered.EventType)
        if err == nil && existingEvent != nil {
            // Update existing event
            err = b.updateAuditEvent(existingEvent, buffered)
        } else {
            // Create new event
            err = b.createAuditEvent(buffered)
        }
        if err != nil {
            // Log but continue
            fmt.Fprintf(os.Stderr, "Warning: Failed to flush audit event %s: %v\n", key, err)
        }
    }
    
    // Clear buffer after flush
    b.buffer = make(map[string]*BufferedEvent)
    return nil
}
```

### 3. Finding Existing Events

```go
// Find existing audit event by target_id and event_type
func (b *AuditEventBuffer) findExistingAuditEvent(targetID, eventType string) (map[string]interface{}, error) {
    if targetID == "" {
        // No target_id means we can't find existing event (would need event_type only lookup)
        return nil, fmt.Errorf("no target_id provided")
    }
    
    // Search in current month's audit directory
    now := time.Now().UTC()
    month := now.Format("2006-01")
    auditDir := filepath.Join(b.projectRoot, "docs", "process", "audit", month)
    
    entries, err := os.ReadDir(auditDir)
    if err != nil {
        return nil, err
    }
    
    // Scan audit events in this month (could be optimized with index)
    for _, entry := range entries {
        if !strings.HasSuffix(entry.Name(), ".yaml") {
            continue
        }
        
        auditPath := filepath.Join(auditDir, entry.Name())
        data, err := os.ReadFile(auditPath)
        if err != nil {
            continue
        }
        
        var event map[string]interface{}
        if err := yaml.Unmarshal(data, &event); err != nil {
            continue
        }
        
        // Check if this event matches
        if event["target_id"] == targetID && event["event_type"] == eventType {
            return event, nil
        }
    }
    
    return nil, fmt.Errorf("event not found")
}
```

### 4. Updating Existing Events

```go
// Update existing audit event with occurrence data
func (b *AuditEventBuffer) updateAuditEvent(existing map[string]interface{}, buffered *BufferedEvent) error {
    // Get current occurrence count
    currentCount := 1
    if count, ok := existing["occurrence_count"].(int); ok {
        currentCount = count
    }
    
    // Get existing timestamps
    timestamps := []string{}
    if ts, ok := existing["occurrence_timestamps"].([]interface{}); ok {
        for _, t := range ts {
            if str, ok := t.(string); ok {
                timestamps = append(timestamps, str)
            }
        }
    } else {
        // If no timestamps, use created_at as first occurrence
        if created, ok := existing["created_at"].(string); ok {
            timestamps = append(timestamps, created)
        }
    }
    
    // Append new timestamps
    for _, t := range buffered.Occurrences {
        timestamps = append(timestamps, t.Format("2006-01-02T15:04:05Z"))
    }
    
    // Update event
    existing["occurrence_count"] = currentCount + len(buffered.Occurrences)
    existing["occurrence_timestamps"] = timestamps
    existing["updated_at"] = time.Now().UTC().Format("2006-01-02T15:04:05Z")
    existing["updated_by"] = b.ctx.Profile
    
    // Write updated event
    auditPath := filepath.Join(
        filepath.Dir(filepath.Join(b.projectRoot, "docs", "process", "audit", time.Now().UTC().Format("2006-01"))),
        fmt.Sprintf("%s.yaml", existing["id"]),
    )
    
    data, err := yaml.Marshal(existing)
    if err != nil {
        return err
    }
    
    return os.WriteFile(auditPath, data, 0644)
}
```

### 5. Integration with Check Command

```go
// In check_impl.go

// Global buffer for bulk operations
var auditEventBuffer *AuditEventBuffer
var bufferOnce sync.Once

func getAuditEventBuffer(ctx *context.Context) *AuditEventBuffer {
    bufferOnce.Do(func() {
        projectRoot := ctx.ProjectRoot
        if projectRoot == "" {
            projectRoot = context.FindProjectRoot(".")
        }
        auditEventBuffer = &AuditEventBuffer{
            buffer:      make(map[string]*BufferedEvent),
            projectRoot: projectRoot,
            ctx:         ctx,
        }
    })
    return auditEventBuffer
}

// Modified createHashMismatchFixAuditEvent
func createHashMismatchFixAuditEvent(ctx *context.Context, obj *parser.ParsedObject, filePath, kind, originalHash string) error {
    // Check if we're in a bulk operation context
    // (could detect by checking if checkAll is running, or add a flag)
    isBulkOperation := detectBulkOperation(ctx)
    
    if isBulkOperation {
        // Add to buffer instead of creating immediately
        buffer := getAuditEventBuffer(ctx)
        buffer.Add(
            "hash_mismatch_fix",
            obj.ID,
            kind,
            filePath,
            fmt.Sprintf("Regenerated integrity hash for %s", obj.ID),
            "high",
            map[string]interface{}{
                "command": "zqk check --force",
                "original_hash": originalHash,
            },
        )
        return nil
    }
    
    // Non-bulk: create immediately (existing behavior)
    // ... existing implementation ...
}

// At end of checkAll, flush buffer
func checkAll(ctx *context.Context, cmd *cobra.Command) error {
    // ... existing check logic ...
    
    // Flush audit event buffer at end
    if auditEventBuffer != nil {
        if err := auditEventBuffer.Flush(); err != nil {
            logger.Warn("Failed to flush audit event buffer", logging.Error(err))
        }
    }
    
    return nil
}
```

## Benefits

1. **Reduced Storage**: 1 event file instead of N for repeated operations
2. **Better Performance**: Fewer file writes during bulk operations
3. **Improved Queryability**: `occurrence_count` shows frequency at a glance
4. **Temporal Tracking**: `occurrence_timestamps` shows when operations occurred
5. **Backward Compatible**: Existing events work without occurrence fields

## Edge Cases

1. **No target_id**: Can't find existing events, always create new
2. **Cross-month operations**: Only search current month (could extend to recent months)
3. **Concurrent operations**: Buffer is thread-safe, but flush should be atomic
4. **Process crash**: Buffer is lost (could persist to temp file)

## Queue + BulkCreate (reduce I/O storm)

A simpler, complementary approach **without** merging by key: **queue** object_creation / object_update / object_deletion audit events during bulk operations and **flush as one BulkCreate** at the end. We still create N audit events (one per operation), but we avoid N synchronous Creates (N file writes, N hash registry updates) by batching.

### Mechanism

1. **Context flag**: `WithDeferAuditEvents(ctx)` marks the context as "bulk operation". When set, `createCreateAuditEvent` / `createUpdateAuditEvent` / `createDeleteAuditEvent` **enqueue** (options + projectRoot + secCtx + fileStorage) instead of calling `CreateAuditEventWithBuilder` immediately.
2. **Pending queue**: Thread-safe slice (or map keyed by projectRoot) of pending items. Each item holds enough to build one audit event instance (options, projectRoot, secCtx, fileStorage).
3. **Flush**: `FlushPendingAuditEvents(ctx, projectRoot, storageProvider, secCtx)` drains the queue for that project, builds one audit event instance per item (using existing builder + ID generator), then `BulkCreate(ctx, secCtx, instances)` with `WithBulkCreateDeferFlush(ctx)`, then `FlushKind("audit_event")`. One transaction, one CAS flush.
4. **Call sites**: Bulk paths set the flag and flush at the end. Example: `BulkDeleteOptimized` wraps ctx with `WithDeferAuditEvents` at start; after the delete loop (fast or slow path), calls `FlushPendingAuditEvents`.

### Benefits

- **No change to single-operation path**: Normal Create/Update/Delete still create one audit event immediately.
- **Bulk path gets batching**: N deletes → N queue appends (cheap) → one BulkCreate at end → one CAS flush. Reduces I/O and avoids "bogging the system down".
- **We aggregate later anyway**: Downstream aggregation (e.g. audit_aggregation_metric) still sees N events; batching only changes how they are persisted.

### Edge cases

- **Recursion**: When we BulkCreate audit_event, each Create skips creating a nested audit event (kind == "audit_event" guard). So no loop.
- **Ordering**: Audit events are independent; batch order is fine.
- **ID generation**: Flush builds instances one by one, each with a unique ID from the existing generator (same as today).

## Future Enhancements

1. **Persistent Buffer**: Write buffer to temp file for crash recovery
2. **Cross-Month Search**: Search last N months for existing events
3. **Index File**: Maintain index of audit events by key for faster lookup
4. **Configurable Threshold**: Only buffer if operation count > threshold

