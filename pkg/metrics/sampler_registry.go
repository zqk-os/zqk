package metrics

import (
	"fmt"
	"maps"
	"sync"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage"
)

// maxSamplers caps the number of distinct *Sampler instances (goroutines from flushTickerLoop), not map keys.
// Multiple registry keys may alias the same *Sampler (e.g. shared fallback). When at cap, system metrics
// (empty field name, [MetricTypeSystem]) use getOrCreateFallbackSystemSampler instead of NewSampler.
const maxSamplers = 64

// samplerRegistryWildcardObjectKind matches the ObjectKind stored on the shared fallback sampler config.
const samplerRegistryWildcardObjectKind = "*"

// fallbackSamplerKey is the registry key for the shared fallback when at maxSamplers (see [MetricTypeSystem]).
const fallbackSamplerKey = samplerRegistryWildcardObjectKind + ":" + MetricTypeSystem

// SamplerRegistry manages multiple samplers for different metric types
type SamplerRegistry struct {
	samplers map[string]*Sampler       // Key: "object_kind:field_name:metric_type" or "object_kind:metric_type"
	configs  map[string]*SamplerConfig // Key: same as samplers
	storage  storage.ObjectStorageProvider
	mu       sync.RWMutex
	logger   logging.Logger
	lockLog  concurrency.LockLogger

	// fallbackAliasNotice logs once when registrations must share the fallback sampler due to maxSamplers.
	fallbackAliasNotice sync.Once
}

// NewSamplerRegistry creates a new sampler registry
func NewSamplerRegistry(storageProvider storage.ObjectStorageProvider) *SamplerRegistry {
	prof := string(pkgctx.ProfileSystem)
	return &SamplerRegistry{
		samplers: make(map[string]*Sampler),
		configs:  make(map[string]*SamplerConfig),
		storage:  storageProvider,
		logger:   logging.GetLoggerFromProfile(prof),
		lockLog:  logging.GetLockLoggerFromProfile(prof),
	}
}

// registrySamplerIsSystemKind reports whether fieldName/metricType identify the registry's system sampler slot
// (same convention as [SamplerRegistry.GetOrCreateSampler] with empty field name).
func registrySamplerIsSystemKind(fieldName, metricType string) bool {
	return fieldName == emptyValue && metricType == MetricTypeSystem
}

// bindFallbackSamplerKeyLocked assigns the shared fallback registry key to s and logs once. Caller holds sr.mu (write lock).
func (sr *SamplerRegistry) bindFallbackSamplerKeyLocked(s *Sampler) {
	sr.samplers[fallbackSamplerKey] = s
	sr.fallbackAliasNotice.Do(func() {
		logging.Fluent(sr.logger).Warn("Sampler registry reached max distinct samplers; aliased fallback key to an existing system sampler").
			Int("max_distinct_samplers", maxSamplers).
			Log()
	})
}

// distinctSamplerCountLocked returns the number of unique non-nil *Sampler pointers held by the registry.
// Caller must hold sr.mu (read or write).
func (sr *SamplerRegistry) distinctSamplerCountLocked() int {
	seen := make(map[*Sampler]struct{}, len(sr.samplers))
	for _, s := range sr.samplers {
		if s == nil {
			continue
		}
		seen[s] = struct{}{}
	}
	return len(seen)
}

// samplerRefCountLocked returns how many registry keys reference sampler.
// Caller must hold sr.mu.
func (sr *SamplerRegistry) samplerRefCountLocked(sampler *Sampler) int {
	if sampler == nil {
		return 0
	}
	n := 0
	for _, s := range sr.samplers {
		if s == sampler {
			n++
		}
	}
	return n
}

// RegisterSampler registers a sampler configuration.
// If a sampler already exists for this key, it will be replaced. Distinct *Sampler instances are capped at
// maxSamplers; at capacity, only system metrics with an empty field name alias to the shared fallback sampler.
func (sr *SamplerRegistry) RegisterSampler(config *SamplerConfig) error {
	if err := config.Validate(); err != nil {
		return errfmt.Newf("invalid sampler config").Wrap(err)
	}

	key := sr.getSamplerKey(config.ObjectKind, config.FieldName, config.MetricType)

	var oldSampler *Sampler
	var hadKey bool
	_ = concurrency.RunInLockWithLogger(
		&sr.mu, LockNameSamplerRegistryRegisterCheck, sr.lockLog,
		func() error {
			var ok bool
			oldSampler, ok = sr.samplers[key]
			hadKey = ok
			return nil
		},
	)

	if hadKey {
		var toStop *Sampler
		_ = concurrency.RunInLockWithLogger(
			&sr.mu, LockNameSamplerRegistryRegisterStore, sr.lockLog,
			func() error {
				delete(sr.samplers, key)
				delete(sr.configs, key)
				if oldSampler != nil && sr.samplerRefCountLocked(oldSampler) == 0 {
					toStop = oldSampler
				}
				return nil
			},
		)
		if toStop != nil {
			if err := toStop.Stop(); err != nil {
				logging.Fluent(sr.logger).Warn("Failed to stop replaced sampler").
					String("key", key).
					WithError(err).
					Log()
			}
		}
	}

	var atCap bool
	_ = concurrency.RunInLockWithLogger(
		&sr.mu, LockNameSamplerRegistryRegisterCheck, sr.lockLog,
		func() error {
			atCap = sr.distinctSamplerCountLocked() >= maxSamplers
			return nil
		},
	)

	var sampler *Sampler
	var err error
	switch {
	case atCap && registrySamplerIsSystemKind(config.FieldName, config.MetricType):
		sampler, err = sr.getOrCreateFallbackSystemSampler()
		if err != nil {
			return err
		}
	case atCap:
		return errfmt.Errorf("sampler registry at capacity (%d), cannot register sampler for key %s", maxSamplers, key)
	default:
		aggregator := sr.getAggregatorForMetricType(config.MetricType)
		sampler, err = NewSampler(config, sr.storage, aggregator)
		if err != nil {
			return errfmt.Newf("failed to create sampler").Wrap(err)
		}
	}

	_ = concurrency.RunInLockWithLogger(
		&sr.mu, LockNameSamplerRegistryRegisterStore, sr.lockLog,
		func() error {
			sr.samplers[key] = sampler
			sr.configs[key] = config
			return nil
		},
	)

	logging.Fluent(sr.logger).Info("Registered sampler").
		String("key", key).
		String("enabled", fmt.Sprintf("%v", config.Enabled)).
		BatchSize(config.BatchSize).
		Log()

	return nil
}

// GetSampler returns the sampler for the given key, or nil if not found
func (sr *SamplerRegistry) GetSampler(objectKind, fieldName, metricType string) *Sampler {
	key := sr.getSamplerKey(objectKind, fieldName, metricType)

	var sampler *Sampler
	_ = concurrency.RunInRLockWithLogger(
		&sr.mu, LockNameSamplerRegistryGet, sr.lockLog,
		func() error {
			sampler = sr.samplers[key]
			return nil
		},
	)

	return sampler
}

// GetOrCreateSampler gets an existing sampler or creates a new one with default config.
// When at maxSamplers, system metrics (empty field, [MetricTypeSystem]) use a shared fallback sampler
// to avoid unbounded flushTickerLoop goroutines (scheduler dump finding).
func (sr *SamplerRegistry) GetOrCreateSampler(objectKind, fieldName, metricType string) (*Sampler, error) {
	key := sr.getSamplerKey(objectKind, fieldName, metricType)

	var sampler *Sampler
	var exists bool
	var atCap bool
	_ = concurrency.RunInLockWithLogger(
		&sr.mu, LockNameSamplerRegistryGetOrCreateCheck, sr.lockLog,
		func() error {
			var ok bool
			sampler, ok = sr.samplers[key]
			exists = ok
			atCap = sr.distinctSamplerCountLocked() >= maxSamplers
			return nil
		},
	)

	if exists {
		return sampler, nil
	}

	// At cap: use shared fallback for system metrics (avoids one sampler per object kind).
	if atCap && registrySamplerIsSystemKind(fieldName, metricType) {
		return sr.getOrCreateFallbackSystemSampler()
	}

	// Create default config
	config := DefaultSamplerConfig()
	config.ObjectKind = objectKind
	config.FieldName = fieldName
	config.MetricType = metricType

	// Get or create aggregator
	aggregator := sr.getAggregatorForMetricType(metricType)

	// Create sampler (outside lock to avoid holding lock during I/O)
	newSampler, err := NewSampler(config, sr.storage, aggregator)
	if err != nil {
		return nil, errfmt.Newf("failed to create sampler").Wrap(err)
	}

	var useFallback bool
	_ = concurrency.RunInLockWithLogger(
		&sr.mu, LockNameSamplerRegistryGetOrCreateStore, sr.lockLog,
		func() error {
			if existing, ok := sr.samplers[key]; ok {
				sampler = existing
				return nil
			}
			if sr.distinctSamplerCountLocked() >= maxSamplers && registrySamplerIsSystemKind(fieldName, metricType) {
				useFallback = true
				return nil
			}
			sr.samplers[key] = newSampler
			sr.configs[key] = config
			sampler = newSampler
			return nil
		},
	)

	if sampler != nil {
		return sampler, nil
	}
	if useFallback {
		_ = newSampler.Stop() // release goroutine since we didn't register it
		return sr.getOrCreateFallbackSystemSampler()
	}
	return nil, errfmt.Errorf("sampler registry at capacity (%d), cannot create new sampler", maxSamplers)
}

// getOrCreateFallbackSystemSampler returns the shared sampler for system metrics when at maxSamplers.
func (sr *SamplerRegistry) getOrCreateFallbackSystemSampler() (*Sampler, error) {
	var sampler *Sampler
	_ = concurrency.RunInLockWithLogger(
		&sr.mu, LockNameSamplerRegistryFallbackCheck, sr.lockLog,
		func() error {
			if s, ok := sr.samplers[fallbackSamplerKey]; ok {
				sampler = s
			}
			return nil
		},
	)
	if sampler != nil {
		return sampler, nil
	}

	var shouldCreate bool
	var aliasErr error
	_ = concurrency.RunInLockWithLogger(
		&sr.mu, LockNameSamplerRegistryFallbackStore, sr.lockLog,
		func() error {
			if s, ok := sr.samplers[fallbackSamplerKey]; ok {
				sampler = s
				return nil
			}
			if sr.distinctSamplerCountLocked() < maxSamplers {
				shouldCreate = true
				return nil
			}
			for k, s := range sr.samplers {
				if k == fallbackSamplerKey {
					continue
				}
				cfg := sr.configs[k]
				if cfg != nil && registrySamplerIsSystemKind(cfg.FieldName, cfg.MetricType) {
					sr.bindFallbackSamplerKeyLocked(s)
					sampler = s
					return nil
				}
			}
			aliasErr = errfmt.Errorf("sampler registry at capacity (%d): cannot create or alias fallback system sampler", maxSamplers)
			return nil
		},
	)
	if sampler != nil {
		return sampler, nil
	}
	if aliasErr != nil {
		return nil, aliasErr
	}
	if !shouldCreate {
		return nil, errfmt.Errorf("internal: fallback system sampler resolution failed")
	}

	config := DefaultSamplerConfig()
	config.ObjectKind = samplerRegistryWildcardObjectKind
	config.FieldName = emptyValue
	config.MetricType = MetricTypeSystem
	aggregator := sr.getAggregatorForMetricType(MetricTypeSystem)
	newSampler, err := NewSampler(config, sr.storage, aggregator)
	if err != nil {
		return nil, errfmt.Newf("failed to create fallback system sampler").Wrap(err)
	}

	_ = concurrency.RunInLockWithLogger(
		&sr.mu, LockNameSamplerRegistryFallbackStore, sr.lockLog,
		func() error {
			if existing, ok := sr.samplers[fallbackSamplerKey]; ok {
				sampler = existing
				return nil
			}
			sr.samplers[fallbackSamplerKey] = newSampler
			sr.configs[fallbackSamplerKey] = config
			sampler = newSampler
			return nil
		},
	)

	if sampler != nil && sampler != newSampler {
		_ = newSampler.Stop()
	}
	return sampler, nil
}

// GetOrCreatePipelineSampleSampler returns the shared system sampler used for high-frequency
// MetricPipeline.Sample events. One sampler avoids spawning a flushTickerLoop goroutine per distinct
// object_kind (scheduler memory blow-up when many kinds emit sampled events).
func (sr *SamplerRegistry) GetOrCreatePipelineSampleSampler() (*Sampler, error) {
	return sr.getOrCreateFallbackSystemSampler()
}

// UnregisterSampler removes a sampler key and stops the underlying sampler only when no keys reference it.
func (sr *SamplerRegistry) UnregisterSampler(objectKind, fieldName, metricType string) error {
	key := sr.getSamplerKey(objectKind, fieldName, metricType)

	var sampler *Sampler
	var exists bool
	var refsLeft int
	_ = concurrency.RunInLockWithLogger(
		&sr.mu, LockNameSamplerRegistryUnregisterCopy, sr.lockLog,
		func() error {
			var ok bool
			sampler, ok = sr.samplers[key]
			exists = ok
			if exists {
				delete(sr.samplers, key)
				delete(sr.configs, key)
				if sampler != nil {
					refsLeft = sr.samplerRefCountLocked(sampler)
				}
			}
			return nil
		},
	)

	if exists && refsLeft == 0 && sampler != nil {
		if err := sampler.Stop(); err != nil {
			logging.Fluent(sr.logger).Warn("Failed to stop sampler").
				String("key", key).
				WithError(err).
				Log()
		}
	}

	return nil
}

// distinctNonNilSamplersLocked returns each unique non-nil *Sampler once. Caller must hold sr.mu (read or write).
func (sr *SamplerRegistry) distinctNonNilSamplersLocked() []*Sampler {
	seen := make(map[*Sampler]struct{}, len(sr.samplers))
	out := make([]*Sampler, 0, len(sr.samplers))
	for _, s := range sr.samplers {
		if s == nil {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// FlushAll flushes all samplers
func (sr *SamplerRegistry) FlushAll() error {
	var samplers []*Sampler
	_ = concurrency.RunInRLockWithLogger(
		&sr.mu, LockNameSamplerRegistryFlushAllCopy, sr.lockLog,
		func() error {
			samplers = sr.distinctNonNilSamplersLocked()
			return nil
		},
	)

	var errors []error
	for _, sampler := range samplers {
		if err := sampler.FlushAll(); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return errfmt.Errorf("failed to flush some samplers: %v", errors)
	}

	return nil
}

// StopAll stops all samplers and flushes pending batches
func (sr *SamplerRegistry) StopAll() error {
	var samplersCopy map[string]*Sampler
	_ = concurrency.RunInLockWithLogger(
		&sr.mu, LockNameSamplerRegistryStopAllCopy, sr.lockLog,
		func() error {
			samplersCopy = make(map[string]*Sampler, len(sr.samplers))
			maps.Copy(samplersCopy, sr.samplers)
			// Clear all samplers
			sr.samplers = make(map[string]*Sampler)
			sr.configs = make(map[string]*SamplerConfig)
			return nil
		},
	)

	seen := make(map[*Sampler]struct{}, len(samplersCopy))
	var errors []error
	for key, sampler := range samplersCopy {
		if sampler == nil {
			continue
		}
		if _, dup := seen[sampler]; dup {
			continue
		}
		seen[sampler] = struct{}{}
		if err := sampler.Stop(); err != nil {
			errors = append(errors, errfmt.Errorf("sampler %s: %w", key, err))
		}
	}

	if len(errors) > 0 {
		return errfmt.Errorf("failed to stop some samplers: %v", errors)
	}

	return nil
}

// GetStats returns statistics about all samplers
func (sr *SamplerRegistry) GetStats() map[string]any {
	var stats map[string]any
	_ = concurrency.RunInRLockWithLogger(
		&sr.mu, LockNameSamplerRegistryGetStats, sr.lockLog,
		func() error {
			stats = make(map[string]any)
			stats["sampler_count"] = len(sr.samplers)
			stats["distinct_sampler_count"] = sr.distinctSamplerCountLocked()
			stats["samplers"] = make(map[string]any)

			totalPending := 0
			seenPending := make(map[*Sampler]struct{})
			for key, sampler := range sr.samplers {
				pendingN := 0
				batchN := 0
				if sampler != nil {
					pendingN = sampler.GetTotalPendingEvents()
					batchN = sampler.GetBatchCount()
				}
				stats["samplers"].(map[string]any)[key] = map[string]any{
					"batch_count":    batchN,
					"pending_events": pendingN,
					"config":         sr.configs[key],
				}
				if sampler == nil {
					continue
				}
				if _, ok := seenPending[sampler]; ok {
					continue
				}
				seenPending[sampler] = struct{}{}
				totalPending += pendingN
			}
			stats["total_pending_events"] = totalPending
			return nil
		},
	)
	return stats
}

// getSamplerKey generates a unique key for a sampler
func (sr *SamplerRegistry) getSamplerKey(objectKind, fieldName, metricType string) string {
	if fieldName != emptyValue {
		return fmt.Sprintf("%s:%s:%s", objectKind, fieldName, metricType)
	}
	return fmt.Sprintf("%s:%s", objectKind, metricType)
}

// getAggregatorForMetricType gets or creates an aggregator for a metric type
func (sr *SamplerRegistry) getAggregatorForMetricType(metricType string) MetricAggregator {
	// This is a simplified version - in production, you might want to use
	// a factory pattern or registry for aggregators
	switch metricType {
	case MetricTypeScalar:
		return NewScalarMetricAggregator(sr.storage)
	case MetricTypeList:
		return NewListMetricAggregator(sr.storage)
	case MetricTypeOrderedList:
		return NewOrderedListMetricAggregator(sr.storage)
	case MetricTypeStatusHistory:
		return NewStatusHistoryMetricAggregator(sr.storage)
	default:
		// Default to a generic aggregator (could be a no-op or simple counter)
		return NewScalarMetricAggregator(sr.storage)
	}
}
