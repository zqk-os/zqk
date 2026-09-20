package metrics

import (
	"context"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// MetricPipeline collects and aggregates metrics from objects/fields with metric traits
type MetricPipeline struct {
	storageProvider storage.ObjectStorageProvider
	traitRegistry   *objects.TraitRegistry
	specLoader      *objects.SpecLoader
	aggregators     map[string]MetricAggregator // metric type -> aggregator
	samplerRegistry *SamplerRegistry            // Optional sampler registry for batching

	subscribers   []func(map[string]any)
	subscribersMu sync.RWMutex
}

// AddSubscriber adds a callback that will be invoked for every sampled event.
// This allows other components in the same process to react to telemetry.
func (mp *MetricPipeline) AddSubscriber(callback func(map[string]any)) {
	mp.subscribersMu.Lock()
	defer mp.subscribersMu.Unlock()
	mp.subscribers = append(mp.subscribers, callback)
}

// MetricAggregator defines how to aggregate metric data
type MetricAggregator interface {
	// Aggregate collects and aggregates metric data from objects
	Aggregate(ctx context.Context, config *AggregationConfig) (*AggregationResult, error)

	// GetMetricType returns the metric type this aggregator handles
	GetMetricType() string
}

// AggregationConfig defines what to aggregate
type AggregationConfig struct {
	ObjectKind   string         // Object kind to aggregate (e.g., "backlog_item")
	FieldName    string         // Field name to aggregate (optional, if empty aggregates all metric fields)
	MetricType   string         // Metric type (scalar_metric, list_metric, etc.)
	WindowStart  time.Time      // Aggregation window start
	WindowEnd    time.Time      // Aggregation window end
	Filters      map[string]any // Additional filters for objects
	GroupBy      []string       // Fields to group by (optional)
	Aggregations []string       // Aggregation functions (sum, avg, min, max, count, frequency, distribution)
}

// AggregationResult contains aggregated metric data
type AggregationResult struct {
	MetricID        string                        // ID of created base_metric object
	ObjectKind      string                        // Object kind aggregated
	FieldName       string                        // Field name aggregated
	MetricType      string                        // Metric type
	WindowStart     time.Time                     // Aggregation window start
	WindowEnd       time.Time                     // Aggregation window end
	Aggregations    map[string]any                // Aggregated values (e.g., {"sum": 100, "avg": 10})
	Groups          map[string]*AggregationResult // Grouped results (if GroupBy specified)
	ObjectCount     int                           // Number of objects aggregated
	CollectionCount int                           // Number of metric collections
}

// NewMetricPipeline creates a new metrics pipeline
// TraitRegistry is initialized lazily to avoid blocking on filesystem I/O during initialization
func NewMetricPipeline(storageProvider storage.ObjectStorageProvider) *MetricPipeline {
	return &MetricPipeline{
		storageProvider: storageProvider,
		traitRegistry:   nil, // Initialize lazily on first use
		specLoader:      objects.GetGlobalSpecLoader(),
		aggregators:     make(map[string]MetricAggregator),
		samplerRegistry: NewSamplerRegistry(storageProvider),
	}
}

// getTraitRegistry returns the trait registry, initializing it lazily if needed
func (mp *MetricPipeline) getTraitRegistry() *objects.TraitRegistry {
	if mp.traitRegistry == nil {
		// Initialize on first use (lazy initialization)
		// This avoids blocking during pipeline creation
		mp.traitRegistry = objects.NewTraitRegistry()
	}
	return mp.traitRegistry
}

// InitializeSamplerConfig loads and applies sampler configuration from config file
// This should be called after creating the pipeline to load sampler profiles
func (mp *MetricPipeline) InitializeSamplerConfig(projectRoot string) error {
	if mp.samplerRegistry == nil {
		mp.samplerRegistry = NewSamplerRegistry(mp.storageProvider)
	}

	// Try to find and load sampler config
	configPath, err := FindSamplerConfigFile(projectRoot)
	if err != nil {
		// Config file not found - use defaults (sampler registry will create samplers on-demand)
		return nil // Not an error - defaults are acceptable
	}

	configFile, err := LoadSamplerConfig(configPath)
	if err != nil {
		return errfmt.Newf("failed to load sampler config").Wrap(err)
	}

	if err := ApplySamplerConfig(mp.samplerRegistry, configFile, projectRoot); err != nil {
		return errfmt.Newf("failed to apply sampler config").Wrap(err)
	}

	return nil
}

// SetSamplerRegistry sets the sampler registry (useful for testing or custom configuration)
func (mp *MetricPipeline) SetSamplerRegistry(registry *SamplerRegistry) {
	mp.samplerRegistry = registry
}

// GetSamplerRegistry returns the sampler registry
func (mp *MetricPipeline) GetSamplerRegistry() *SamplerRegistry {
	return mp.samplerRegistry
}

// RegisterAggregator registers a metric aggregator
func (mp *MetricPipeline) RegisterAggregator(aggregator MetricAggregator) {
	mp.aggregators[aggregator.GetMetricType()] = aggregator
}

// CollectMetrics collects metrics from objects with metric-enabled traits
// This is the main entry point for the metrics pipeline
// If a sampler is configured for this metric type, it will batch events in memory
func (mp *MetricPipeline) CollectMetrics(ctx context.Context, config *AggregationConfig) (*AggregationResult, error) {
	// Check if sampling is enabled for this metric type
	if mp.samplerRegistry != nil {
		sampler := mp.samplerRegistry.GetSampler(config.ObjectKind, config.FieldName, config.MetricType)
		if sampler != nil {
			// Sampling is enabled - events should be sent via Sample() method
			// This method is for direct aggregation (non-sampled)
			// Return an error indicating sampling should be used
			return nil, errfmt.Errorf("sampling is enabled for %s.%s - use Sample() method instead", config.ObjectKind, config.FieldName)
		}
	}

	// Find aggregator for metric type
	aggregator, ok := mp.aggregators[config.MetricType]
	if !ok {
		// Use default aggregator based on metric type
		aggregator = mp.getDefaultAggregator(config.MetricType)
		if aggregator == nil {
			return nil, errfmt.Errorf("no aggregator found for metric type: %s", config.MetricType)
		}
	}

	// Execute aggregation on the caller goroutine so parallel CollectMetrics callers
	// cannot multiply unbounded metrics_aggregator goroutines. Enforce the same 30s
	// ceiling as the previous async+select implementation.
	base := ctx
	if base == nil {
		base = context.Background()
	}
	aggCtx, cancel := context.WithTimeout(base, 30*time.Second)
	defer cancel()

	return aggregator.Aggregate(aggCtx, config)
}

// Sample adds an event to the sampler batch (for high-frequency events)
// Returns true if a metric was created (batch was flushed)
func (mp *MetricPipeline) Sample(event map[string]any) (bool, error) {
	if mp.samplerRegistry == nil {
		return false, errfmt.Errorf("sampler registry not initialized")
	}

	// Determine object kind and metric type from event
	objectKind, _ := event[objects.FieldKeyKind].(string)
	if objectKind == emptyValue {
		objectKind, _ = event[objects.FieldKeyTargetKind].(string)
	}
	// Invoke subscribers
	mp.subscribersMu.RLock()
	subs := make([]func(map[string]any), len(mp.subscribers))
	copy(subs, mp.subscribers)
	mp.subscribersMu.RUnlock()

	for _, sub := range subs {
		sub(event)
	}

	if !metricsrecording.EnabledForKind(objectKind) {
		return false, nil
	}

	// Use the shared pipeline sampler — not GetOrCreateSampler(per kind), which creates one ticker
	// goroutine per distinct object_kind.
	sampler, err := mp.samplerRegistry.GetOrCreatePipelineSampleSampler()
	if err != nil {
		return false, errfmt.Newf("failed to get sampler").Wrap(err)
	}

	return sampler.Sample(event)
}

// DiscoverMetricFields discovers fields with metric traits in a spec
func (mp *MetricPipeline) DiscoverMetricFields(ontology string) ([]MetricField, error) {
	spec, err := mp.specLoader.LoadSpecWithInheritance(ontology + ".yaml")
	if err != nil {
		return nil, errfmt.Newf("failed to load spec").Wrap(err)
	}

	var metricFields []MetricField

	// Check object-level traits
	objectTraits := spec.ResolvedTraits
	expandedTraits, _ := mp.traitRegistry.ExpandTraits(objectTraits) //nolint:errcheck // Use original traits if expansion fails
	hasMetricEnabled := false
	for _, trait := range expandedTraits {
		if trait == "base_metric_enabled_group" {
			hasMetricEnabled = true
			break
		}
	}

	// Check field-level traits
	if spec.ResolvedFields != nil {
		for fieldName, fieldDef := range spec.ResolvedFields {
			fieldMap, ok := fieldDef.(map[string]any)
			if !ok {
				continue
			}

			// Get field traits
			fieldTraits := objects.ExtractFieldTraits(fieldMap)
			expandedFieldTraits, _ := mp.getTraitRegistry().ExpandTraits(fieldTraits) //nolint:errcheck // Use original traits if expansion fails

			// Check if field has metric traits
			metricType := mp.detectMetricType(expandedFieldTraits)
			hasReadable := false
			for _, trait := range expandedFieldTraits {
				if trait == "readable" {
					hasReadable = true
					break
				}
			}
			if metricType != emptyValue || (hasMetricEnabled && hasReadable) {
				metricFields = append(metricFields, MetricField{
					FieldName:  fieldName,
					MetricType: metricType,
					FieldDef:   fieldMap,
					ObjectKind: ontology,
				})
			}
		}
	}

	return metricFields, nil
}

// MetricField represents a field that can be collected as a metric
type MetricField struct {
	FieldName  string
	MetricType string // scalar_metric, list_metric, ordered_list_metric, status_history_metric
	FieldDef   map[string]any
	ObjectKind string
}

// detectMetricType detects the metric type from field traits
func (mp *MetricPipeline) detectMetricType(traits []string) string {
	// Check for specific metric types (most specific first)
	for _, trait := range traits {
		switch trait {
		case MetricTypeStatusHistory:
			return MetricTypeStatusHistory
		case MetricTypeOrderedList:
			return MetricTypeOrderedList
		case MetricTypeList:
			return MetricTypeList
		case MetricTypeScalar:
			return MetricTypeScalar
		}
	}
	return ""
}

// getDefaultAggregator returns a default aggregator for a metric type
func (mp *MetricPipeline) getDefaultAggregator(metricType string) MetricAggregator {
	switch metricType {
	case MetricTypeScalar:
		return NewScalarMetricAggregator(mp.storageProvider)
	case MetricTypeList:
		return NewListMetricAggregator(mp.storageProvider)
	case MetricTypeOrderedList:
		return NewOrderedListMetricAggregator(mp.storageProvider)
	case MetricTypeStatusHistory:
		return NewStatusHistoryMetricAggregator(mp.storageProvider)
	default:
		return nil
	}
}

// HasReadableTrait checks if a field has the readable trait
func HasReadableTrait(fieldDef map[string]any) bool {
	traits := objects.ExtractFieldTraits(fieldDef)
	for _, trait := range traits {
		if trait == "readable" {
			return true
		}
	}
	return false
}
