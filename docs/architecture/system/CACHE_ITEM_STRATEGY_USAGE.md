# Cache Item Strategy Hook Usage

## Overview

The cache item strategy hook provides a configurable mechanism for special handling of specific objects in the cache validation system. This allows case-by-case configuration without hardcoding object IDs.

**Current Implementation**: Simple and fast (environment variables + programmatic registration)  
**Future Evolution**: May support spec-based configuration when performance tuning requires granular controls  
See [CACHE_ITEM_STRATEGY_EVOLUTION.md](./CACHE_ITEM_STRATEGY_EVOLUTION.md) for the evolution path.

## Architecture

### Components

1. **CacheItemStrategy Interface**: Defines how to handle specific objects
2. **CacheItemStrategyRegistry**: Manages registered strategies
3. **Strategy Implementations**: Pre-built strategies for common use cases
4. **Configuration**: Environment-based configuration for temporary diagnostics

## Usage

### Environment-Based Configuration (Temporary Diagnostics)

The simplest way to configure strategies is via environment variables:

```bash
# Enable diagnostic strategies
export ZQK_CACHE_DIAGNOSTIC_ENABLED=true

# Track specific object IDs
export ZQK_CACHE_DIAGNOSTIC_OBJECTS="REQ-999,CRIT-9091,CRIT-9092"

# Track objects by prefix
export ZQK_CACHE_DIAGNOSTIC_PREFIXES="REQ-,CRIT-"
```

When enabled, the system will:
- Log warnings when tracked objects are missing from cache
- Log debug messages when tracked objects are found in cache
- Track cache load events for tracked objects

### Programmatic Configuration

For more control, you can register strategies programmatically:

```go
import "github.com/lanceman/zqk/cmd/zqk-community/pkg_cmd/system"

// Get the global registry
registry := system.GetGlobalCacheItemStrategyRegistry()

// Register a diagnostic strategy for specific objects
strategy := system.NewDiagnosticCacheItemStrategy(
    "my-diagnostic",
    []string{"REQ-999", "CRIT-9091", "CRIT-9092"},
)
registry.RegisterStrategy(strategy)

// Register a prefix-based strategy
prefixStrategy := system.NewPrefixBasedCacheItemStrategy(
    "req-tracking",
    "REQ-",
    logging.WarnLevel,
)
registry.RegisterStrategy(prefixStrategy)
```

## Strategy Types

### DiagnosticCacheItemStrategy

Tracks specific object IDs for diagnostic purposes. Useful for debugging cache issues with known problematic objects.

```go
strategy := NewDiagnosticCacheItemStrategy("name", []string{"OBJ-001", "OBJ-002"})
```

**Behavior:**
- Logs warnings on cache miss
- Logs debug messages on cache load
- Silent on cache hit (configurable)

### PatternBasedCacheItemStrategy

Tracks objects matching a pattern (prefix, suffix, or custom matcher).

```go
// Prefix-based
strategy := NewPrefixBasedCacheItemStrategy("req-tracking", "REQ-", logging.WarnLevel)

// Suffix-based
strategy := NewSuffixBasedCacheItemStrategy("test-tracking", "-TEST", logging.DebugLevel)
```

### Custom Strategy

Implement the `CacheItemStrategy` interface for custom behavior:

```go
type MyCustomStrategy struct {
    // Your fields
}

func (s *MyCustomStrategy) ShouldTrack(objectID string) bool {
    // Return true if this strategy should track the object
    return strings.Contains(objectID, "SPECIAL")
}

func (s *MyCustomStrategy) OnCacheMiss(objectID string, cacheSize int) (bool, logging.LogLevel, string) {
    // Return: shouldLog, logLevel, message
    return true, logging.WarnLevel, "Special object missing from cache"
}

func (s *MyCustomStrategy) OnCacheHit(objectID string) (bool, logging.LogLevel, string) {
    return false, logging.DebugLevel, "" // Don't log hits
}

func (s *MyCustomStrategy) OnCacheLoad(objectID string, cacheSize int) (bool, logging.LogLevel, string) {
    return true, logging.DebugLevel, "Special object loaded"
}

func (s *MyCustomStrategy) Name() string {
    return "my-custom-strategy"
}
```

## Integration Points

The strategy hook is integrated at:

1. **Cache Load**: When cache is loaded/built, `OnCacheLoad` is called for tracked objects
2. **Cache Get (Miss)**: When an object is not found, `OnCacheMiss` is called
3. **Cache Get (Hit)**: When an object is found, `OnCacheHit` is called (optional)

## Best Practices

1. **Use Environment Variables for Temporary Diagnostics**: 
   - Easy to enable/disable
   - No code changes required
   - Good for debugging specific issues

2. **Use Programmatic Registration for Persistent Strategies**:
   - When you need custom logic
   - When strategies should be part of the codebase
   - For production monitoring

3. **Keep Strategies Focused**:
   - One strategy per concern
   - Clear naming
   - Document the purpose

4. **Use Appropriate Log Levels**:
   - `DebugLevel`: For detailed diagnostics
   - `InfoLevel`: For important but non-critical events
   - `WarnLevel`: For potential issues
   - `ErrorLevel`: For actual errors

## Example: Debugging Cache Issues

```bash
# Enable diagnostics for known problematic objects
export ZQK_CACHE_DIAGNOSTIC_ENABLED=true
export ZQK_CACHE_DIAGNOSTIC_OBJECTS="REQ-999,CRIT-9091,CRIT-9092"

# Run system check
zqk system check

# Check logs for diagnostic messages
# You'll see warnings if objects are missing, debug messages if found
```

## Future Enhancements

Potential future improvements:
- YAML-based configuration file
- Strategy chaining/composition
- Metrics collection per strategy
- Conditional strategies based on cache state
- Integration with coordinator framework for event tracking
