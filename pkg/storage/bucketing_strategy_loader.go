package storage

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/loader"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	"github.com/zqk-os/zqk/pkg/validation"
)

// BucketStrategyLoader loads bucketing strategy configurations from storage.
// Supports multiple backends (file, graph, RDF, etc.) via BucketStrategyStorageProvider.
// Initialize uses the component loader pattern (pkg/loader) for wait-for-completion and timeouts.
type BucketStrategyLoader struct {
	storageProvider       BucketStrategyStorageProvider
	specLoader            *objects.SpecLoader
	validator             *validation.GoValidator
	cache                 map[string]map[string]any // strategyID -> strategy
	kindIndex             map[string][]string       // kind -> []strategyID
	schemaVersionCache    map[string]string         // kind -> schema version (avoids LoadSpecWithInheritance on hot path)
	mu                    sync.RWMutex
	initialized           bool
	runner                *loader.Runner
	runnerOnce            sync.Once
	strategiesLoadedTotal atomic.Int64
	cacheHitsTotal        atomic.Int64
}

// GetLoaderStats returns lifetime counters for strategies loaded and cache hits.
func (l *BucketStrategyLoader) GetLoaderStats() (strategiesLoaded, cacheHits int64) {
	return l.strategiesLoadedTotal.Load(), l.cacheHitsTotal.Load()
}

// NewBucketStrategyLoader creates a new strategy loader
func NewBucketStrategyLoader(ctx context.Context, projectRoot string) (*BucketStrategyLoader, error) {
	// Create storage factory to detect backend
	factory, err := NewBucketStrategyStorageFactory(ctx, projectRoot)
	if err != nil {
		return nil, errfmt.Newf(ErrMsgCreateStorageFactory).Wrap(err)
	}

	storageProvider := factory.GetStorageProvider()

	return &BucketStrategyLoader{
		storageProvider:    storageProvider,
		specLoader:         objects.GetGlobalSpecLoader(),
		validator:          validation.NewGoValidator(),
		cache:              make(map[string]map[string]any),
		kindIndex:          make(map[string][]string),
		schemaVersionCache: make(map[string]string),
		initialized:        false,
	}, nil
}

// NewBucketStrategyLoaderWithProvider creates a loader with a specific storage provider
// Useful for testing or when you want to explicitly control the backend
func NewBucketStrategyLoaderWithProvider(provider BucketStrategyStorageProvider) *BucketStrategyLoader {
	return &BucketStrategyLoader{
		storageProvider:    provider,
		specLoader:         objects.GetGlobalSpecLoader(),
		validator:          validation.NewGoValidator(),
		cache:              make(map[string]map[string]any),
		kindIndex:          make(map[string][]string),
		schemaVersionCache: make(map[string]string),
		initialized:        false,
	}
}

// getRunner returns the shared loader.Runner for this loader (lazily created).
func (l *BucketStrategyLoader) getRunner() *loader.Runner {
	l.runnerOnce.Do(func() {
		l.runner = loader.NewRunner(RunnerNameBucketStrategyLoader, func(ctx context.Context) error {
			return l.doInitialize(ctx)
		})
	})
	return l.runner
}

// Initialize loads all strategies from storage and builds indexes via the component loader pattern.
func (l *BucketStrategyLoader) Initialize(ctx context.Context) error {
	return l.getRunner().Load(ctx)
}

// doInitialize performs the actual load and cache update; called by loader.Runner.
func (l *BucketStrategyLoader) doInitialize(ctx context.Context) error {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	StorageLog(logger).Debug(LogEventStorageBucketingLoaderInitializingDebug).
		String("backend", l.storageProvider.GetBackendType()).
		Log()

	strategies, err := l.storageProvider.LoadAllStrategies(ctx)
	if err != nil {
		return errfmt.Newf(ErrMsgLoadStrategies).Wrap(err)
	}

	kindVersionIndex := make(map[string]string)
	cacheUpdates := make(map[string]map[string]any)
	kindIndexUpdates := make(map[string][]string)

	for _, strategy := range strategies {
		validationErrors := validation.ValidateBucketingStrategy(strategy)
		if len(validationErrors) > 0 {
			strategyID, ok := strategy[objects.FieldKeyID].(string)
			if !ok {
				strategyID = DefaultUnknownID
			}
			// Log first few error messages so users can fix the strategy (e.g. add strategy_type, field, format)
			detailMsgs := make([]string, 0, len(validationErrors))
			for i, e := range validationErrors {
				if i >= 3 {
					detailMsgs = append(detailMsgs, fmt.Sprintf(FmtAndMore, len(validationErrors)-3))
					break
				}
				detailMsgs = append(detailMsgs, e.Field+": "+e.Message)
			}
			StorageLog(logger).Warn(LogEventStorageBucketingLoaderValidationFailedSkip).
				String("strategy_id", strategyID).
				Int("error_count", len(validationErrors)).
				String("details", fmt.Sprintf("%v", detailMsgs)).
				Log()
			continue
		}

		strategyID, ok := strategy[objects.FieldKeyID].(string)
		if !ok {
			StorageLog(logger).Warn(LogEventStorageBucketingLoaderMissingIDSkip).Log()
			continue
		}

		cacheUpdates[strategyID] = strategy

		appliesTo, ok := strategy[objects.FieldKeyAppliesTo].([]any)
		if ok {
			enabled, ok := strategy[objects.FieldKeyEnabled].(bool)
			if ok && enabled {
				specLoader := objects.GetGlobalSpecLoader()
				strategyType, ok := strategy[objects.FieldKeyStrategyType].(string)
				if !ok {
					strategyType = ""
				}
				field, ok := strategy[objects.FieldKeyField].(string)
				if !ok {
					field = ""
				}
				hasConflicts := false

				for _, kindVal := range appliesTo {
					kind, ok := kindVal.(string)
					if !ok {
						continue
					}
					schemaVersion := l.getSchemaVersionForKind(ctx, specLoader, kind)
					key := fmt.Sprintf("%s:%s:%s:%s", kind, schemaVersion, strategyType, field)

					if existingStrategyID, exists := kindVersionIndex[key]; exists {
						StorageLog(logger).Warn(LogEventStorageBucketingLoaderConflictSkipDuplicate).
							String("strategy_id", strategyID).
							Kind(kind).
							String(FieldKeySchemaVersion, schemaVersion).
							String("strategy_type", strategyType).
							String("field", field).
							String(FieldKeyExistingStrategyID, existingStrategyID).
							String("conflict_key", key).
							Log()
						hasConflicts = true
						continue
					}

					kindVersionIndex[key] = strategyID
					kindIndexUpdates[kind] = append(kindIndexUpdates[kind], strategyID)
				}

				if hasConflicts {
					StorageLog(logger).Warn(LogEventStorageBucketingLoaderPartialIndexConflict).
						String("strategy_id", strategyID).
						Log()
				}
			}
		}
	}

	return concurrency.RunInLockWithLogger(
		&l.mu, locknames.LockNameBucketStrategyLoaderUpdateCache, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			for strategyID, strategy := range cacheUpdates {
				l.cache[strategyID] = strategy
			}
			for kind, strategyIDs := range kindIndexUpdates {
				l.kindIndex[kind] = append(l.kindIndex[kind], strategyIDs...)
			}
			l.initialized = true
			StorageLog(logger).Debug(LogEventStorageBucketingLoaderInitializedDebug).
				Int(FieldKeyStrategyCount, len(l.cache)).
				String("backend", l.storageProvider.GetBackendType()).
				Log()
			return nil
		},
	)
}

// GetStrategy retrieves a strategy by ID
func (l *BucketStrategyLoader) GetStrategy(ctx context.Context, strategyID string) (map[string]any, error) {
	var strategy map[string]any
	var found bool
	err := concurrency.RunInRLockWithLogger(
		&l.mu, locknames.LockNameBucketStrategyLoaderGetCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if cached, ok := l.cache[strategyID]; ok {
				strategy = cached
				found = true
			}
			return nil
		},
	)
	if err != nil {
		return nil, errfmt.Newf(ErrMsgTimeoutCheckCache).Wrap(err)
	}
	if found {
		l.cacheHitsTotal.Add(1)
		return strategy, nil
	}

	// Not in cache - load from storage (NO LOCK HELD)
	strategy, err = l.storageProvider.LoadStrategy(ctx, strategyID)
	if err != nil {
		return nil, err
	}
	l.strategiesLoadedTotal.Add(1)

	// Cache it
	if err := concurrency.RunInLockWithLogger(
		&l.mu, locknames.LockNameBucketStrategyLoaderCache, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			l.cache[strategyID] = strategy
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockGetStrategy, err).Log()
	}

	return strategy, nil
}

// GetStrategiesForKind returns all enabled strategies that apply to a specific object kind
func (l *BucketStrategyLoader) GetStrategiesForKind(ctx context.Context, kind string) ([]map[string]any, error) {
	// Ensure initialized
	if !l.initialized {
		if err := l.Initialize(WithBucketStrategyRegistryInitialization(ctx)); err != nil {
			return nil, err
		}
	}

	var strategyIDs []string
	var ok bool
	err := concurrency.RunInRLockWithLogger(
		&l.mu, locknames.LockNameBucketStrategyLoaderGetForKind, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			strategyIDs, ok = l.kindIndex[kind]
			return nil
		},
	)
	if err != nil {
		return nil, errfmt.Newf(ErrMsgTimeoutGetStrategies).Wrap(err)
	}
	if !ok {
		return []map[string]any{}, nil // No strategies for this kind
	}

	// Load strategies from cache
	var strategies []map[string]any
	if err := concurrency.RunInRLockWithLogger(
		&l.mu, locknames.LockNameBucketStrategyLoaderLoadFromCache, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			strategies = make([]map[string]any, 0, len(strategyIDs))
			for _, strategyID := range strategyIDs {
				if strategy, ok := l.cache[strategyID]; ok {
					strategies = append(strategies, strategy)
				}
			}
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockGetStrategiesKind, err).Log()
	}

	return strategies, nil
}

// GetAllStrategies returns all loaded strategies
func (l *BucketStrategyLoader) GetAllStrategies(ctx context.Context) ([]map[string]any, error) {
	// Ensure initialized
	if !l.initialized {
		if err := l.Initialize(ctx); err != nil {
			return nil, err
		}
	}

	var strategies []map[string]any
	if err := concurrency.RunInRLockWithLogger(
		&l.mu, locknames.LockNameBucketStrategyLoaderGetAll, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			strategies = make([]map[string]any, 0, len(l.cache))
			for _, strategy := range l.cache {
				strategies = append(strategies, strategy)
			}
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockGetAllStrategies, err).Log()
	}
	return strategies, nil
}

// Reload clears the cache and reinitializes from storage
func (l *BucketStrategyLoader) Reload(ctx context.Context) error {
	if err := concurrency.RunInLockWithLogger(
		&l.mu, locknames.LockNameBucketStrategyLoaderReload, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			l.cache = make(map[string]map[string]any)
			l.kindIndex = make(map[string][]string)
			l.initialized = false
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockReload, err).Log()
	}
	l.getRunner().ResetLoaded()
	return l.getRunner().Load(ctx)
}

// GetBackendType returns the type of storage backend in use
func (l *BucketStrategyLoader) GetBackendType() string {
	return l.storageProvider.GetBackendType()
}

// getSchemaVersionForKind retrieves the schema version for an object kind
// Returns DefaultSchemaVersion as default if spec cannot be loaded.
// Results are cached to avoid LoadSpecWithInheritance on every call (OBJECT_OPERATIONS_PERFORMANCE.md).
func (l *BucketStrategyLoader) getSchemaVersionForKind(_ context.Context, specLoader *objects.SpecLoader, kind string) string {
	if specLoader == nil {
		return objects.DefaultSchemaVersion // Default schema version
	}

	l.mu.RLock()
	cached, ok := l.schemaVersionCache[kind]
	l.mu.RUnlock()
	if ok {
		return cached
	}

	// Try to load spec for this kind
	// Spec files are named after the kind (e.g., "audit_event.yaml")
	specFile := kind + ExtYaml
	spec, err := specLoader.LoadSpecWithInheritance(specFile)
	version := objects.DefaultSchemaVersion
	if err == nil && spec.SchemaVersion != emptyValue {
		version = spec.SchemaVersion
	}

	l.mu.Lock()
	l.schemaVersionCache[kind] = version
	l.mu.Unlock()
	return version
}

// Global loader instance
var (
	globalStrategyLoader *BucketStrategyLoader
	strategyLoaderOnce   sync.Once
)

// GetGlobalStrategyLoader returns the global strategy loader instance
func GetGlobalStrategyLoader(ctx context.Context, projectRoot string) (*BucketStrategyLoader, error) {
	var err error
	strategyLoaderOnce.Do(func() {
		globalStrategyLoader, err = NewBucketStrategyLoader(ctx, projectRoot)
	})
	return globalStrategyLoader, err
}
