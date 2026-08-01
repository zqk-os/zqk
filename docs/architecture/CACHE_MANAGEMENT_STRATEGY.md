# Cache Management Strategy

**Version**: 1.0.0  
**Created**: 2026-01-08  
**Status**: Analysis & Recommendations  
**Purpose**: Define hybrid cache management strategy following GC-like incremental/strategic refresh patterns

## Current State

### Fast Operations (On-Demand / Incremental)
- **SpecLoader.mtime_check**: `os.Stat()` to check file modification time
  - Cost: Very low (~microseconds)
  - Pattern: On-demand (lazy evaluation)
  - Blocking: No (fast path)
  - Background needed: ❌ No

- **SpecLoader.LoadSpecWithInheritance**: Parse YAML, resolve inheritance chain
  - Cost: Medium (milliseconds per spec, ~5-50ms)
  - Pattern: On-demand (with mtime check first - avoids unnecessary work)
  - Blocking: Yes (blocks caller until complete)
  - Background needed: ❌ No (mtime check prevents unnecessary loads)

### Medium Operations (On-Demand with Pre-Warming Opportunity)
- **SystemFieldsRegistry.loadSystemFields**: Load base_object + auditable specs, derive system fields
  - Cost: Medium (depends on spec complexity, ~10-100ms)
  - Pattern: On-demand (first access only)
  - Blocking: Yes (blocks first caller)
  - Background needed: ⚠️ Could benefit from pre-warming
  - Current: Not pre-warmed

### Expensive Operations (Require Pre-Warming)
- **FieldRegistry.LoadFields**: Load ALL specs, extract fields for ALL kinds
  - Cost: High (hundreds of specs, ~100ms-1s+)
  - Pattern: On-demand (first access)
  - Blocking: Yes (blocks first caller significantly)
  - Background needed: ✅ Yes - should be pre-warmed
  - Current: Not pre-warmed (only SpecLoader and LifecycleLoader are pre-warmed)

## Garbage Collection Pattern

The cache management strategy follows a GC-like pattern:

1. **Incremental Updates** (Fast Path):
   - Mtime checks (fast, non-blocking)
   - Single spec loads (medium, but mtime check prevents unnecessary work)
   - Event-driven invalidation (remove specific entries)

2. **Strategic Pre-Warming** (Background Jobs):
   - Expensive operations (FieldRegistry, SystemFieldsRegistry) pre-warmed on startup
   - Periodic refresh for expensive caches (timer-based scheduler jobs)
   - Scheduled during low-activity periods

3. **Full Refresh** (On-Demand Fallback):
   - Only when cache is invalidated or corrupted
   - Blocks caller (acceptable as fallback)
   - Triggered by manual invalidation or cache corruption

4. **Parallelization**:
   - Spec loading can be parallelized (independent operations)
   - Field extraction can be parallelized (per-kind operations)
   - Cache pre-warming uses scheduler jobs (managed, non-blocking)

## Current Implementation

### Scheduler Jobs (Not Hardcoded Loops)

**SCH-001** (Cache Pre-Warming):
- **Trigger**: Timer (every 2 hours)
- **Job Type**: `cache_prewarm`
- **What it pre-warms**:
  - ✅ SpecLoader (all object specs)
  - ✅ LifecycleLoader (all lifecycle definitions)
  - ❌ FieldRegistry (NOT pre-warmed)
  - ❌ SystemFieldsRegistry (NOT pre-warmed)

**SCH-007** (Initial Cache Pre-Warming):
- **Trigger**: Immediate (on scheduler start)
- **Job Type**: `cache_prewarm`
- **What it pre-warms**: Same as SCH-001

### On-Demand Cache Operations

- **SpecLoader**: Mtime check → Load if changed (incremental)
- **FieldRegistry**: Load all fields on first access (expensive, blocking)
- **SystemFieldsRegistry**: Load system fields on first access (medium, blocking)

## Gaps and Recommendations

### Gap 1: FieldRegistry Not Pre-Warmed

**Problem**: `FieldRegistry.LoadFields()` is expensive (loads ALL specs) but not pre-warmed.

**Impact**: First caller to `GetFieldsForKind()` blocks for ~100ms-1s+ while all specs are loaded.

**Recommendation**: Add FieldRegistry pre-warming to `CachePrewarmHandler`:

```go
// In pkg/scheduler/handlers.go
func (h *CachePrewarmHandler) prewarmFieldRegistry(_ context.Context) error {
    fieldRegistry := objects.GetGlobalFieldRegistry()
    // Trigger load by calling GetAllKinds() or GetFieldsForKind() for each known kind
    // This will load all specs and extract fields in the background
    if _, err := fieldRegistry.GetAllKinds(); err != nil {
        h.logger.Warn("Failed to pre-warm field registry", logging.Error(err))
        return err
    }
    return nil
}
```

### Gap 2: SystemFieldsRegistry Not Pre-Warmed

**Problem**: `SystemFieldsRegistry.loadSystemFields()` blocks first caller for ~10-100ms.

**Impact**: First caller to `IsSystemGeneratedField()` blocks while system fields are derived.

**Recommendation**: Add SystemFieldsRegistry pre-warming to `CachePrewarmHandler`:

```go
// In pkg/scheduler/handlers.go
func (h *CachePrewarmHandler) prewarmSystemFieldsRegistry(_ context.Context) error {
    systemFieldsRegistry := objects.GetGlobalSystemFieldsRegistry()
    // Trigger load by calling GetSystemGeneratedFields()
    if _, err := systemFieldsRegistry.GetSystemGeneratedFields(); err != nil {
        h.logger.Warn("Failed to pre-warm system fields registry", logging.Error(err))
        return err
    }
    return nil
}
```

### Gap 3: Event-Driven Incremental Refresh

**Current**: Spec changes require manual cache invalidation or waiting for periodic refresh.

**Recommendation**: Integrate with event-driven cache invalidation:
- When a spec file changes, invalidate only affected caches (not full refresh)
- Use `SpecLoader.InvalidateSpecByFile()` (already exists)
- Use `SystemFieldsRegistry.Reload()` if base_object or auditable changes
- Use `FieldRegistry.Reload()` if any spec changes (more expensive, but targeted)

**Implementation**: Hook into file system events or storage operations that modify spec files.

## Recommended Strategy

### Tier 1: Fast Path (On-Demand, Incremental)
- **Mtime checks**: Always on-demand (fast, no background needed)
- **Single spec loads**: On-demand with mtime check (prevents unnecessary work)

### Tier 2: Strategic Pre-Warming (Scheduler Jobs)
- **FieldRegistry**: Pre-warm on startup (SCH-007) and periodic refresh (SCH-001)
- **SystemFieldsRegistry**: Pre-warm on startup (SCH-007) and periodic refresh (SCH-001)
- **SpecLoader**: Already pre-warmed ✅
- **LifecycleLoader**: Already pre-warmed ✅

### Tier 3: Event-Driven Incremental Refresh
- **Spec changes**: Invalidate only affected caches (not full refresh)
- **Base object changes**: Reload SystemFieldsRegistry and FieldRegistry (targeted)
- **Individual spec changes**: Invalidate only that spec in SpecLoader (already supported)

### Tier 4: Full Refresh (On-Demand Fallback)
- **Cache corruption**: Full reload (blocking, acceptable as fallback)
- **Manual invalidation**: Full reload (blocking, user-initiated)
- **Scheduler job failure**: On-demand fallback (user doesn't wait, but first access blocks)

## Parallelization Opportunities

1. **Spec Loading**: Parallelize loading of independent specs (per-kind operations)
2. **Field Extraction**: Parallelize field extraction per kind (independent operations)
3. **Cache Pre-Warming**: Scheduler jobs already run in background (non-blocking)

## Performance Impact

### Current (Without FieldRegistry/SystemFieldsRegistry Pre-Warming)
- First access to `FieldRegistry.GetFieldsForKind()`: **~100ms-1s+ blocking**
- First access to `SystemFieldsRegistry.IsSystemGeneratedField()`: **~10-100ms blocking**

### With Pre-Warming (Recommended)
- First access to `FieldRegistry.GetFieldsForKind()`: **~0ms (cache hit)**
- First access to `SystemFieldsRegistry.IsSystemGeneratedField()`: **~0ms (cache hit)**
- Pre-warming cost: **Background (non-blocking, scheduled)**

## Conclusion

**Answer to Question**: The cache manager uses **scheduler jobs** (not hardcoded loops) for expensive operations, and **on-demand incremental updates** (mtime checks) for fast operations. This follows a GC-like pattern:

- ✅ **Incremental**: Fast operations (mtime checks) are on-demand
- ✅ **Strategic**: Expensive operations are pre-warmed via scheduler jobs
- ✅ **Parallelizable**: Spec loading can be parallelized
- ✅ **Dependency-Aware**: Tiered pre-warming with proper dependency ordering
- ✅ **Implemented**: FieldRegistry and SystemFieldsRegistry are now pre-warmed

**Implementation Status**:
1. ✅ Added FieldRegistry pre-warming to `CachePrewarmHandler` (Tier 2, parallel with LifecycleLoader and SystemFieldsRegistry)
2. ✅ Added SystemFieldsRegistry pre-warming to `CachePrewarmHandler` (Tier 2, parallel with LifecycleLoader and FieldRegistry)
3. ✅ Implemented tiered processing: Tier 1 (SpecLoader) → Tier 2 (parallel) → Tier 3 (parallel)
4. 🔄 Consider event-driven incremental refresh for spec changes (future enhancement)

See `CACHE_PREWARM_DEPENDENCIES.md` for detailed dependency analysis and tiered processing strategy.
