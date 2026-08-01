package objects

import (
	"sync"
	"sync/atomic"
	"time"
)

// KindMappingsMetrics tracks metrics for kind mappings operations
type KindMappingsMetrics struct {
	// Config loading metrics
	ConfigLoads        int64 // Total number of config loads
	ConfigLoadFailures int64 // Total number of failed config loads
	ConfigLoadDuration int64 // Total time spent loading configs (nanoseconds)

	// Backend operations
	BackendSwitches     int64 // Total number of backend type switches
	BackendConfigMerges int64 // Total number of backend config merges

	// Mapping lookups
	DirectoryLookups  int64 // Total number of GetDirectoryFromKind calls
	KindLookups       int64 // Total number of GetKindFromDirectory calls
	CacheHits         int64 // Number of cache hits
	CacheMisses       int64 // Number of cache misses
	InferenceRuleHits int64 // Number of times inference rules were used

	// Initialization metrics
	Initializations        int64 // Total number of Initialize() calls
	InitializationDuration int64 // Total time spent initializing (nanoseconds)
	MaxInitializationTime  int64 // Maximum initialization time (nanoseconds)

	// Discovery metrics
	DirectoriesScanned int64 // Total number of directories scanned
	SpecsScanned       int64 // Total number of spec files scanned
	MappingsDiscovered int64 // Total number of mappings discovered

	// Error metrics
	DiscoveryErrors int64 // Total number of discovery errors
	LookupErrors    int64 // Total number of lookup errors

	mu sync.RWMutex //nolint:unused // Reserved for future thread-safety
}

var globalKindMappingsMetrics = &KindMappingsMetrics{}

// GetKindMappingsMetrics returns the global kind mappings metrics
func GetKindMappingsMetrics() *KindMappingsMetrics {
	return globalKindMappingsMetrics
}

// ResetKindMappingsMetrics resets all metrics (useful for testing)
func ResetKindMappingsMetrics() {
	globalKindMappingsMetrics = &KindMappingsMetrics{}
}

// RecordConfigLoad records a config load operation
func (m *KindMappingsMetrics) RecordConfigLoad(duration time.Duration, err error) {
	if err != nil {
		atomic.AddInt64(&m.ConfigLoadFailures, 1)
	} else {
		atomic.AddInt64(&m.ConfigLoads, 1)
		atomic.AddInt64(&m.ConfigLoadDuration, int64(duration))
	}
}

// RecordBackendSwitch records a backend type switch
func (m *KindMappingsMetrics) RecordBackendSwitch() {
	atomic.AddInt64(&m.BackendSwitches, 1)
}

// RecordBackendConfigMerge records a backend config merge operation
func (m *KindMappingsMetrics) RecordBackendConfigMerge() {
	atomic.AddInt64(&m.BackendConfigMerges, 1)
}

// RecordDirectoryLookup records a GetDirectoryFromKind call
func (m *KindMappingsMetrics) RecordDirectoryLookup(cacheHit, usedInference bool) {
	atomic.AddInt64(&m.DirectoryLookups, 1)
	if cacheHit {
		atomic.AddInt64(&m.CacheHits, 1)
	} else {
		atomic.AddInt64(&m.CacheMisses, 1)
	}
	if usedInference {
		atomic.AddInt64(&m.InferenceRuleHits, 1)
	}
}

// RecordKindLookup records a GetKindFromDirectory call
func (m *KindMappingsMetrics) RecordKindLookup(cacheHit, usedInference bool) {
	atomic.AddInt64(&m.KindLookups, 1)
	if cacheHit {
		atomic.AddInt64(&m.CacheHits, 1)
	} else {
		atomic.AddInt64(&m.CacheMisses, 1)
	}
	if usedInference {
		atomic.AddInt64(&m.InferenceRuleHits, 1)
	}
}

// RecordInitialization records an initialization operation
func (m *KindMappingsMetrics) RecordInitialization(duration time.Duration, err error) {
	atomic.AddInt64(&m.Initializations, 1)
	if err != nil {
		atomic.AddInt64(&m.DiscoveryErrors, 1)
	} else {
		atomic.AddInt64(&m.InitializationDuration, int64(duration))

		// Update max initialization time
		for {
			current := atomic.LoadInt64(&m.MaxInitializationTime)
			if int64(duration) <= current {
				break
			}
			if atomic.CompareAndSwapInt64(&m.MaxInitializationTime, current, int64(duration)) {
				break
			}
		}
	}
}

// RecordDirectoryScan records scanning a directory
func (m *KindMappingsMetrics) RecordDirectoryScan() {
	atomic.AddInt64(&m.DirectoriesScanned, 1)
}

// RecordSpecScan records scanning a spec file
func (m *KindMappingsMetrics) RecordSpecScan() {
	atomic.AddInt64(&m.SpecsScanned, 1)
}

// RecordMappingDiscovered records discovering a new mapping
func (m *KindMappingsMetrics) RecordMappingDiscovered() {
	atomic.AddInt64(&m.MappingsDiscovered, 1)
}

// RecordLookupError records a lookup error
func (m *KindMappingsMetrics) RecordLookupError() {
	atomic.AddInt64(&m.LookupErrors, 1)
}

// GetSnapshot returns a snapshot of current metrics
func (m *KindMappingsMetrics) GetSnapshot() KindMappingsMetricsSnapshot {
	return KindMappingsMetricsSnapshot{
		ConfigLoads:            atomic.LoadInt64(&m.ConfigLoads),
		ConfigLoadFailures:     atomic.LoadInt64(&m.ConfigLoadFailures),
		ConfigLoadDuration:     time.Duration(atomic.LoadInt64(&m.ConfigLoadDuration)),
		BackendSwitches:        atomic.LoadInt64(&m.BackendSwitches),
		BackendConfigMerges:    atomic.LoadInt64(&m.BackendConfigMerges),
		DirectoryLookups:       atomic.LoadInt64(&m.DirectoryLookups),
		KindLookups:            atomic.LoadInt64(&m.KindLookups),
		CacheHits:              atomic.LoadInt64(&m.CacheHits),
		CacheMisses:            atomic.LoadInt64(&m.CacheMisses),
		InferenceRuleHits:      atomic.LoadInt64(&m.InferenceRuleHits),
		Initializations:        atomic.LoadInt64(&m.Initializations),
		InitializationDuration: time.Duration(atomic.LoadInt64(&m.InitializationDuration)),
		MaxInitializationTime:  time.Duration(atomic.LoadInt64(&m.MaxInitializationTime)),
		DirectoriesScanned:     atomic.LoadInt64(&m.DirectoriesScanned),
		SpecsScanned:           atomic.LoadInt64(&m.SpecsScanned),
		MappingsDiscovered:     atomic.LoadInt64(&m.MappingsDiscovered),
		DiscoveryErrors:        atomic.LoadInt64(&m.DiscoveryErrors),
		LookupErrors:           atomic.LoadInt64(&m.LookupErrors),
	}
}

// KindMappingsMetricsSnapshot provides a point-in-time view of metrics
type KindMappingsMetricsSnapshot struct {
	ConfigLoads            int64
	ConfigLoadFailures     int64
	ConfigLoadDuration     time.Duration
	BackendSwitches        int64
	BackendConfigMerges    int64
	DirectoryLookups       int64
	KindLookups            int64
	CacheHits              int64
	CacheMisses            int64
	InferenceRuleHits      int64
	Initializations        int64
	InitializationDuration time.Duration
	MaxInitializationTime  time.Duration
	DirectoriesScanned     int64
	SpecsScanned           int64
	MappingsDiscovered     int64
	DiscoveryErrors        int64
	LookupErrors           int64
}

// CacheHitRate returns the cache hit rate as a percentage
//
//nolint:gocritic // Value receiver by design - getter method, no mutation
func (s KindMappingsMetricsSnapshot) CacheHitRate() float64 {
	total := s.CacheHits + s.CacheMisses
	if total == 0 {
		return 0
	}
	return float64(s.CacheHits) / float64(total) * 100
}

// AverageConfigLoadTime returns the average config load time
//
//nolint:gocritic // Value receiver by design - getter method, no mutation
func (s KindMappingsMetricsSnapshot) AverageConfigLoadTime() time.Duration {
	if s.ConfigLoads == 0 {
		return 0
	}
	return s.ConfigLoadDuration / time.Duration(s.ConfigLoads)
}

// AverageInitializationTime returns the average initialization time
//
//nolint:gocritic // Value receiver by design - getter method, no mutation
func (s KindMappingsMetricsSnapshot) AverageInitializationTime() time.Duration {
	if s.Initializations == 0 {
		return 0
	}
	return s.InitializationDuration / time.Duration(s.Initializations)
}

// ConfigLoadSuccessRate returns the config load success rate as a percentage
//
//nolint:gocritic // Value receiver by design - getter method, no mutation
func (s KindMappingsMetricsSnapshot) ConfigLoadSuccessRate() float64 {
	total := s.ConfigLoads + s.ConfigLoadFailures
	if total == 0 {
		return 0
	}
	return float64(s.ConfigLoads) / float64(total) * 100
}

// InferenceRuleUsageRate returns the inference rule usage rate as a percentage
//
//nolint:gocritic // Value receiver by design - getter method, no mutation
func (s KindMappingsMetricsSnapshot) InferenceRuleUsageRate() float64 {
	total := s.DirectoryLookups + s.KindLookups
	if total == 0 {
		return 0
	}
	return float64(s.InferenceRuleHits) / float64(total) * 100
}
