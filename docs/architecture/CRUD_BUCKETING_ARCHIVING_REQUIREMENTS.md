# CRUD Operations Must Consider Bucketing and Archiving Strategies

**Last Verified:** 2026-08-31


**Status**: Design Requirement  
**Date**: 2026-01-08  
**Priority**: High

## Core Principle

**All CRUD operations (Create, Read, Update, Delete, List) must always consider bucketing and archiving strategies, and the directory (bucket) configuration must be reconstructible from the spec alone (no hardcoded logic in Go code).**

## Core kernel objects: no silent hard-delete

**Incident (2026-08-03):** dozens of `workstream` / `criteria` / `question` CAS files disappeared from the worktree without an archive bundle or lineage record. Recovery was `git restore` + `sync-cas-index`.

**Policy:**
- Hard-delete of **core kernel kinds** (workstream, criteria, goals, milestones, requirements, priority plans, policies, vision/mission/strategic plans, questions, org/division, domain registry, glossary, accounts) is **fail-closed**.
- Break-glass only via `zqk object delete … --reason-code "…" (≥30 chars)` → `WithAllowCoreObjectDelete`.
- Proper retirement is **archive** (lifecycle) then **aggregate + compress** with **lineage preserved** for audit (see archive_strategy compression + change_journal / audit aggregation). Do not equate “archived status” with “erase the blob.”

**TRACK:** `BLI-1785723654802038000-b14064bc` (guard); program **`PRI-1785784837719634000-c9473ae7`** / [`KERNEL_MUTATION_PIPELINE.md`](./KERNEL_MUTATION_PIPELINE.md) — logical erase must enter `kernel.cas_object_erase` (pkg/pipeline); critical kinds include backlog_item, test_case, convergence_session. Delete-worthiness (archived + no live inbound refs) and linger-before-Exists-false: [`CAS_MUTATION_SHOCKWAVE.md`](./CAS_MUTATION_SHOCKWAVE.md).

## Current State Analysis

### Bucketing Strategy

✅ **Spec-Based Configuration**: `bucketing_strategy.yaml` spec defines bucketing strategies with fields:
- `strategy_type`: chronological, state, size, composite
- `strategy_name`: monthly, daily, status_based, etc.
- `field`: Field to extract bucket key from (e.g., "created_at")
- `format`: Time format for chronological strategies
- `applies_to`: List of object kinds this strategy applies to
- `archive_strategy`: Optional archive configuration

✅ **Loader Infrastructure**: `BucketStrategyLoader` loads strategies from storage (file-based or graph-based)

✅ **Registry System**: `DefaultBucketStrategyRegistry` provides strategy lookup by kind

❌ **CRUD Integration Gap**: CRUD operations still use hardcoded logic:
- `usesBucketedStorage()` has hardcoded fallback list: `audit_event`, `change_journal_entry`, `integrity_manifest`
- `getObjectFilePath()` uses hardcoded date pattern matching (`^\d{4}-\d{2}(-\d{2})?$`)
- `prepareObjectPath()` may not use strategy loader for all cases

### Archiving Strategy

✅ **Spec-Based Configuration**: `archive_strategy` field in `bucketing_strategy.yaml`:
- `enabled`: Whether archival is enabled
- `archive_after`: Duration before archiving (e.g., "720h" = 30 days)
- `archive_tier`: warm, cold, iced
- `tier_progression`: Multi-tier archive progression
- `compression`: Whether to compress archived objects
- `encryption`: Whether to encrypt archived objects

❌ **CRUD Integration Gap**: No CRUD operations currently consider archiving strategies:
- Create operations don't check if objects should be archived immediately
- Read operations don't check archive tiers
- Update operations don't trigger archive migrations
- Delete operations don't handle archive cleanup
- List operations don't search archive tiers

## Requirements

### 1. Bucketing Strategy Integration

**All CRUD operations must**:
1. Load bucketing strategy from spec (via `BucketStrategyLoader`)
2. Use strategy to determine bucket directory
3. Never use hardcoded fallbacks or assumptions
4. Support multiple strategies per kind (composite strategies)

**Implementation Pattern**:
```go
// Get strategy for kind
strategy, err := f.getBucketStrategyRegistry().GetStrategyForKind(ctx, kind)
if err != nil {
    return fmt.Errorf("failed to get bucketing strategy: %w", err)
}

// Use strategy to get bucket directory
bucketKey := strategy.GetBucketKey(obj, filePath)
bucketDir := strategy.GetBucketDirectory(baseDir, bucketKey)
```

### 2. Archiving Strategy Integration

**All CRUD operations must**:
1. Load archiving strategy from bucketing strategy spec
2. Consider archive tiers when reading/listing
3. Trigger archive migration based on `archive_after` duration
4. Handle archive location paths from `tier_progression`
5. Apply compression/encryption based on strategy

**Implementation Pattern**:
```go
// Get archiving strategy from bucketing strategy
archiveStrategy := bucketingStrategy.GetArchiveStrategy()
if archiveStrategy.Enabled {
    // Check if object should be archived
    if shouldArchive(obj, archiveStrategy.ArchiveAfter) {
        archiveTier := archiveStrategy.ArchiveTier
        archiveLocation := getArchiveLocation(archiveTier, archiveStrategy)
        // Move to archive location
    }
}
```

### 3. Spec-Only Configuration

**No hardcoded logic**:
- ❌ Remove hardcoded bucketed kinds list
- ❌ Remove hardcoded date patterns
- ❌ Remove hardcoded bucket directory calculations
- ✅ Load all configuration from `bucketing_strategy` objects
- ✅ Reconstruct directory structure from strategy spec fields

**Reconstruction Requirements**:
- Given a `bucketing_strategy` spec, the system must be able to:
  1. Determine if a kind uses bucketing
  2. Calculate bucket directory for any object
  3. List all possible bucket directories
  4. Determine archive locations and tiers

## Implementation Checklist

### Phase 1: Remove Hardcoded Logic
- [ ] Remove hardcoded bucketed kinds list from `usesBucketedStorage()`
- [ ] Remove hardcoded date patterns from `getObjectFilePath()`
- [ ] Update `prepareObjectPath()` to use strategy loader
- [ ] Ensure all path calculations use strategy methods

### Phase 2: Integrate Bucketing Strategy
- [ ] Update `Create()` to use strategy for bucket directory
- [ ] Update `Read()` to use strategy for path resolution
- [ ] Update `Update()` to use strategy for path resolution
- [ ] Update `Delete()` to use strategy for path resolution
- [ ] Update `List()` to use strategy for directory scanning

### Phase 3: Integrate Archiving Strategy
- [ ] Implement archive location resolution from strategy
- [ ] Update `Create()` to check archive requirements
- [ ] Update `Read()` to search archive tiers
- [ ] Update `List()` to include archive tiers
- [ ] Implement archive migration job/task

### Phase 4: Validation and Testing
- [ ] Add tests for spec-only bucketing configuration
- [ ] Add tests for archiving strategy integration
- [ ] Verify directory structure can be reconstructed from spec
- [ ] Performance testing for strategy lookup overhead

## Related Documentation

- `bucketing_strategy.yaml` - Spec definition
- `BucketStrategyLoader` - Strategy loading infrastructure
- `DefaultBucketStrategyRegistry` - Strategy lookup
- `BUCKET_SIZE_MANAGEMENT.md` - Bucket size constraints
- `bucketing_strategy_archive_extension.md` - Archive strategy design
