# Cache Item Strategy Evolution Path

**Last Verified:** 2026-08-31


## Current State (Simple & Fast)

**Goal**: Fast iteration for stability, reliability, and efficiency improvements

**Implementation**:
- Environment variable configuration (`ZQK_CACHE_DIAGNOSTIC_*`)
- Programmatic registration via `CacheItemStrategyRegistry`
- Simple strategy implementations (diagnostic, pattern-based)
- Runtime-only (not persisted)

**Use Cases**:
- Temporary diagnostics for debugging cache issues
- Quick enable/disable of monitoring
- Development and testing

## Future Evolution (When Needed)

**Trigger**: When performance tuning requires more granular controls

**Potential Enhancements**:

### 1. Spec-Based Configuration
- Create `cache_item_strategy` object kind
- Spec file: `.zqk/specs/objects/cache_item_strategy.yaml`
- Spec builder: `pkg/specbuilder/bldr_v2/cache_item_strategy_builder.go`
- Stored as objects in the system (like `bucketing_strategy`)
- Managed via CLI: `zqk object create cache_item_strategy ...`

**Benefits**:
- Versioned strategy definitions
- Lifecycle management
- Multi-team configuration
- Validation and inheritance

### 2. Performance Tuning Controls
When we need fine-grained performance controls:

```yaml
# Example future spec structure
kind: cache_item_strategy
id: CACHE-STRAT-001
strategy_type: performance_tuning
object_pattern: "REQ-*"
cache_behavior:
  priority: high
  eviction_policy: never
  preload: true
  metrics_collection: detailed
performance_targets:
  max_lookup_time_ms: 10
  cache_hit_ratio_target: 0.95
  memory_limit_mb: 100
```

### 3. Granular Controls
- Per-object-type cache policies
- Cache size limits per strategy
- Eviction policies
- Preloading strategies
- Metrics collection levels
- Alerting thresholds

### 4. Integration Points
- Coordinator framework for metrics/events
- Scheduler for cache pre-warming
- Metrics system for performance tracking
- Alerting for cache health

## Migration Path

The current interface is designed to support both approaches:

1. **Current**: Strategies registered programmatically
2. **Future**: Strategies loaded from specs and registered automatically
3. **Hybrid**: Both approaches can coexist

**Example Future Loader**:
```go
// Future: Load strategies from object storage
func LoadCacheItemStrategiesFromStorage(ctx context.Context, projectRoot string) error {
    storageFactory, _ := storage.NewStorageFactory(ctx, projectRoot)
    storageProvider := storageFactory.GetStorage()
    
    filter := storage.ListFilter{Kind: "cache_item_strategy"}
    results, _ := storageProvider.List(ctx, secCtx, storageCtx, filter)
    
    registry := GetGlobalCacheItemStrategyRegistry()
    for _, strategyObj := range results.Objects {
        strategy := buildStrategyFromObject(strategyObj)
        registry.RegisterStrategy(strategy)
    }
    
    return nil
}
```

## Decision Criteria

**Keep Simple When**:
- Fast iteration needed
- Temporary diagnostics
- Simple use cases
- Development/testing

**Evolve to Specs When**:
- Performance tuning requires granular controls
- Multiple teams need to configure strategies
- Strategies need versioning/lifecycle
- Integration with other systems needed
- Production monitoring/alerting required

## Design Principles

1. **Interface Stability**: The `CacheItemStrategy` interface won't change
2. **Backward Compatible**: Existing strategies continue to work
3. **Progressive Enhancement**: Add spec support without breaking current code
4. **Performance First**: Simple approach is fast; only add complexity when needed

## Current Status

✅ **Simple implementation complete**
- Environment variable configuration
- Programmatic registration
- Basic strategy types
- Integration with cache operations

⏳ **Future work (when needed)**
- Spec-based configuration
- Performance tuning controls
- Advanced metrics/alerting
- Coordinator integration
