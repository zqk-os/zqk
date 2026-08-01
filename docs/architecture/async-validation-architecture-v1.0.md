# Async Validation Architecture v1.0

**Version:** 1.0.0  
**Created:** 2026-01-01  
**Status:** Active  
**Purpose:** Complete architecture documentation for async validation system

## Overview

The async validation system transforms system check from a synchronous, blocking operation into an eventually consistent, priority-based validation system with caching and background processing.

## Core Concerns

### 1. Eventually Consistent Validation
- Validation results are cached and updated asynchronously
- System converges to correct state over time
- Users see cached results immediately, fresh results as they complete

### 2. Priority-Based Execution
- Critical validations (Tier 1) run first
- Lower priority validations (Tier 2-4) run after
- Objects with previous violations are prioritized

### 3. Cached State Management
- Validation results cached with file checksums
- Cache invalidated when files change
- Stale cache entries automatically refreshed

### 4. Background Processing
- Validations run in worker pool
- Non-blocking user experience
- Progress reporting via channels

### 5. Context Object Pattern
- All validation state grouped in ValidationContext
- Follows established context patterns
- Easy to test and extend

## Architecture Components

### ValidationContext
Groups all validation-related state and configuration:
- Project configuration (project root)
- Execution configuration (workers, timeout, retries)
- Scope configuration (tiers, kinds, categories)
- Cache configuration (cache-only, refresh)
- Priority configuration (mode, custom priorities)
- Progress reporting (enabled, interval)
- Execution state (running, paused, canceled)

### ValidationStateCache
Manages cached validation results:
- Stores validation states with timestamps
- Tracks file checksums for change detection
- Supports invalidation by ID, kind, or pattern
- Persists to `.zqk/validation_cache.json`

### PriorityQueue
Manages validation task queue:
- Heap-based priority queue
- Prioritizes by Tier (1 = highest, 4 = lowest)
- Thread-safe for concurrent access
- Handles retry logic

### AsyncValidator
Orchestrates async validation:
- Manages worker pool
- Enqueues tasks by priority
- Provides progress reporting
- Handles file checksum validation

### Check Integration
Bridges check command with async validator:
- Uses ValidationContext for configuration
- Provides `--cache-only` mode
- Shows incremental progress
- Uses cached state when available

## Data Flow

```
1. User invokes: zqk system check
   ↓
2. Create ValidationContext from command flags
   ↓
3. Load ValidationStateCache
   ↓
4. For each object:
   - Check cache for fresh validation state
   - If cache hit and file unchanged → use cached result
   - If cache miss or file changed → enqueue validation task
   ↓
5. Start AsyncValidator workers (if not running)
   ↓
6. Workers process tasks by priority:
   - Tier 1 → Tier 2 → Tier 3 → Tier 4
   - Update cache with results
   - Send progress updates
   ↓
7. Display results from cache + live updates
```

## Priority System

### Default Priority
- **Priority 1**: Objects with Tier 1 (blocking) issues
- **Priority 2**: Objects with Tier 2 (warning) issues or critical kinds
- **Priority 3**: All other objects
- **Priority 4**: Low-priority objects (if configured)

### Custom Priority
- Can override per object ID or kind
- Supports custom priority modes

## Cache Strategy

### Cache Location
- `.zqk/validation_cache.json`

### Cache Invalidation
- File checksum mismatch (file changed)
- Cache entry older than max age
- Manual invalidation by ID/kind/pattern
- Object updates via CLI

### Cache Freshness
- Default max age: 1 hour
- Configurable via ValidationContext
- Stale entries automatically revalidated

## Progress Reporting

### Progress Updates
- Total tasks queued
- Completed tasks
- Current tier being processed
- Current object being validated
- Status (queued, validating, completed, error)

### Update Frequency
- Default: 500ms
- Configurable via ValidationContext

## Error Handling

### Retry Logic
- Default: 3 retries
- Configurable via ValidationContext
- Exponential backoff (future enhancement)

### Failure Handling
- Failed validations logged
- Retried up to max retries
- Final failures reported to user

## Testing Strategy

### Unit Tests
- ValidationStateCache operations
- PriorityQueue ordering
- AsyncValidator worker pool
- ValidationContext configuration

### Integration Tests
- End-to-end validation flow
- Cache persistence
- Progress reporting
- Error handling

### Test Isolation
- Uses `ZQK_TEST_ROOT` for isolated test data
- No project data pollution
- Automatic cleanup

## Performance Considerations

### Scalability
- Worker pool handles large object counts
- Priority queue ensures critical validations first
- Cache reduces redundant work

### Resource Usage
- Configurable worker count
- Configurable cache size (future)
- Timeout prevents runaway validations

## Security Considerations

### Access Control
- Validation uses SecurityContext
- Respects permissions
- Audit events for validation operations

### Data Integrity
- File checksums prevent stale cache hits
- Cache validation on load
- Atomic cache writes

## Related Documentation

- [Async Validation System](./async-validation-system-v1.0.md)
- [Async Validation Testing](./async-validation-testing-v1.0.md)
- [ADR: Validation Context Pattern](../decisions/ADR-ASYNC-VALIDATION-CONTEXT-v1.0.md)
- [Context Architecture](./cli-context/architecture-v1.0.md)

