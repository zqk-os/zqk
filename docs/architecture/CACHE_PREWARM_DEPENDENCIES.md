# Cache Pre-Warming Dependency Chain

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2026-01-08  
**Status**: ✅ Implemented  
**Purpose**: Define dependency-aware cache pre-warming with tiered processing

## Dependency Analysis

### Tier 1: Base Caches (No Dependencies)
- **SpecLoader**: No dependencies (loads specs directly from filesystem)
  - Must be pre-warmed FIRST
  - All other caches depend on this

### Tier 2: Derived Caches (Depend on Tier 1)
- **LifecycleLoader**: Independent (loads lifecycle YAML files, no dependency on SpecLoader)
  - Can be parallelized with other Tier 2 caches
- **FieldRegistry**: Depends on SpecLoader (calls `specLoader.LoadSpecWithInheritance()`)
  - Must be pre-warmed AFTER Tier 1
  - Can be parallelized with other Tier 2 caches
- **SystemFieldsRegistry**: Depends on SpecLoader (calls `specLoader.LoadSpecWithInheritance()`)
  - Must be pre-warmed AFTER Tier 1
  - Can be parallelized with other Tier 2 caches

### Tier 3: Composite Caches (Depend on Tier 2)
- **Hash Registries**: May depend on specs/lifecycles (loaded per kind)
- **Object ID Cache**: Independent (builds from file system)

## Pre-Warming Sequence

```
Tier 1 (Sequential - Base):
  └─ SpecLoader (pre-warm all specs)

Tier 2 (Parallel - Derived, after Tier 1 completes):
  ├─ LifecycleLoader (parallel)
  ├─ FieldRegistry (parallel)
  └─ SystemFieldsRegistry (parallel)

Tier 3 (Parallel - Composite, after Tier 2 completes):
  ├─ Hash Registries (parallel)
  └─ Object ID Cache (parallel)
```

## Implementation Pattern

```go
// Tier 1: Base (sequential)
if err := prewarmSpecCache(ctx); err != nil {
    // Log but continue
}

// Tier 2: Derived (parallel, after Tier 1)
var wg sync.WaitGroup
wg.Add(3)

go func() {
    defer wg.Done()
    if err := prewarmLifecycleCache(ctx); err != nil {
        // Log but continue
    }
}()

go func() {
    defer wg.Done()
    if err := prewarmFieldRegistry(ctx); err != nil {
        // Log but continue
    }
}()

go func() {
    defer wg.Done()
    if err := prewarmSystemFieldsRegistry(ctx); err != nil {
        // Log but continue
    }
}()

wg.Wait() // Wait for all Tier 2 caches

// Tier 3: Composite (parallel, after Tier 2)
// ...
```

## Key Principles

1. **Dependency Awareness**: Never prime a cache before its dependencies
2. **Tiered Processing**: Tiers define ordinality - process sequentially across tiers
3. **Parallelization**: Same-tier caches can be parallelized (independent operations)
4. **Sequential Ordering**: Different-tier caches must be sequential (dependencies matter)
5. **Lifecycle Pattern**: Follow the dependency lifecycle - base → derived → composite

## Error Handling

- Pre-warming failures in Tier 1: Log and abort (other tiers depend on this)
- Pre-warming failures in Tier 2+: Log and continue (best-effort, fallback to on-demand)
- Parallel operations: Failures in one don't block others (same tier)
