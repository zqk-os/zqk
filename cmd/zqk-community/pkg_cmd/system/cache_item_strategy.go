package system

import (
	"sync"
	"sync/atomic"

	"github.com/lanceman/zqk/pkg/logging"
)

// CacheItemStrategy defines how to handle specific objects in the cache
// This allows case-by-case configuration for special handling of objects
// without hardcoding object IDs in the validation logic
//
// Evolution Path:
// - Current: Simple programmatic registration + environment variables (fast iteration)
// - Future: May evolve to spec-based configuration (cache_item_strategy objects) for:
//   - Versioned strategy definitions
//   - Lifecycle management
//   - Multi-team configuration
//   - Performance tuning with granular controls
//   - Integration with coordinator framework for metrics/alerting
//
// The interface is designed to support both approaches - strategies can be registered
// programmatically (current) or loaded from specs (future) without breaking changes.
type CacheItemStrategy interface {
	// ShouldTrack returns true if this strategy should track the given object ID
	ShouldTrack(objectID string) bool

	// OnCacheMiss is called when an object expected to be in cache is not found
	// Returns true if the miss should be logged/warned, false to silently ignore
	OnCacheMiss(objectID string, cacheSize int) (shouldLog bool, logLevel logging.LogLevel, message string)

	// OnCacheHit is called when an object is found in cache (optional, for diagnostics)
	OnCacheHit(objectID string) (shouldLog bool, logLevel logging.LogLevel, message string)

	// OnCacheLoad is called when cache is loaded/built (optional, for diagnostics)
	OnCacheLoad(objectID string, cacheSize int) (shouldLog bool, logLevel logging.LogLevel, message string)

	// Name returns the strategy name for identification
	Name() string
}

// CacheItemStrategyRegistry manages cache item strategies
type CacheItemStrategyRegistry struct {
	strategies atomic.Value // []CacheItemStrategy
	mu         sync.Mutex
}

var (
	globalCacheItemStrategyRegistry *CacheItemStrategyRegistry
	registryOnce                    sync.Once
)

// GetGlobalCacheItemStrategyRegistry returns the global strategy registry
func GetGlobalCacheItemStrategyRegistry() *CacheItemStrategyRegistry {
	registryOnce.Do(func() {
		globalCacheItemStrategyRegistry = NewCacheItemStrategyRegistry()
	})
	return globalCacheItemStrategyRegistry
}

// NewCacheItemStrategyRegistry creates a new strategy registry
func NewCacheItemStrategyRegistry() *CacheItemStrategyRegistry {
	r := &CacheItemStrategyRegistry{}
	r.strategies.Store(make([]CacheItemStrategy, 0))
	return r
}

// RegisterStrategy registers a cache item strategy
func (r *CacheItemStrategyRegistry) RegisterStrategy(strategy CacheItemStrategy) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current := r.strategies.Load().([]CacheItemStrategy)
	newStrategies := make([]CacheItemStrategy, len(current)+1)
	copy(newStrategies, current)
	newStrategies[len(current)] = strategy
	r.strategies.Store(newStrategies)
}

// GetStrategiesForObject returns all strategies that should track the given object ID
func (r *CacheItemStrategyRegistry) GetStrategiesForObject(objectID string) []CacheItemStrategy {
	var matching []CacheItemStrategy
	current := r.strategies.Load().([]CacheItemStrategy)
	for _, strategy := range current {
		if strategy.ShouldTrack(objectID) {
			matching = append(matching, strategy)
		}
	}
	return matching
}

// HandleCacheMiss handles a cache miss for an object using registered strategies
func (r *CacheItemStrategyRegistry) HandleCacheMiss(objectID string, cacheSize int, logger logging.Logger) {
	strategies := r.GetStrategiesForObject(objectID)
	if len(strategies) == 0 {
		return // No special handling for this object
	}

	for _, strategy := range strategies {
		shouldLog, level, message := strategy.OnCacheMiss(objectID, cacheSize)
		if shouldLog {
			switch level {
			case logging.DebugLevel:
				logging.Fluent(logger).Debug(message).ObjectID(objectID).Strategy(strategy.Name()).Log()
			case logging.InfoLevel:
				logging.Fluent(logger).Info(message).ObjectID(objectID).Strategy(strategy.Name()).Log()
			case logging.WarnLevel:
				logging.Fluent(logger).Warn(message).ObjectID(objectID).Strategy(strategy.Name()).Log()
			case logging.ErrorLevel:
				logging.Fluent(logger).Error(message, nil).ObjectID(objectID).Strategy(strategy.Name()).Log()
			}
		}
	}
}

// HandleCacheHit handles a cache hit for an object using registered strategies (optional diagnostics)
func (r *CacheItemStrategyRegistry) HandleCacheHit(objectID string, logger logging.Logger) {
	strategies := r.GetStrategiesForObject(objectID)
	if len(strategies) == 0 {
		return // No special handling for this object
	}

	for _, strategy := range strategies {
		shouldLog, level, message := strategy.OnCacheHit(objectID)
		if shouldLog {
			switch level {
			case logging.DebugLevel:
				logging.Fluent(logger).Debug(message).ObjectID(objectID).Strategy(strategy.Name()).Log()
			case logging.InfoLevel:
				logging.Fluent(logger).Info(message).ObjectID(objectID).Strategy(strategy.Name()).Log()
			}
		}
	}
}

// HandleCacheLoad handles cache load for an object using registered strategies (optional diagnostics)
func (r *CacheItemStrategyRegistry) HandleCacheLoad(objectID string, cacheSize int, logger logging.Logger) {
	strategies := r.GetStrategiesForObject(objectID)
	if len(strategies) == 0 {
		return // No special handling for this object
	}

	for _, strategy := range strategies {
		shouldLog, level, message := strategy.OnCacheLoad(objectID, cacheSize)
		if shouldLog {
			switch level {
			case logging.DebugLevel:
				logging.Fluent(logger).Debug(message).ObjectID(objectID).Strategy(strategy.Name()).CacheSize(cacheSize).Log()
			case logging.InfoLevel:
				logging.Fluent(logger).Info(message).ObjectID(objectID).Strategy(strategy.Name()).CacheSize(cacheSize).Log()
			}
		}
	}
}
