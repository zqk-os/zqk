package metrics

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/metricsrecording"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

const (
	aggregationCountMetric = "count"
)

// inferAggregationObjectKind returns the object kind for metric aggregation when the sampler config
// uses the registry wildcard ("*") or empty kind: take kind / target_kind from the batch events.
func inferAggregationObjectKind(cfgKind string, events []map[string]any) string {
	if cfgKind != samplerRegistryWildcardObjectKind && cfgKind != emptyValue {
		return cfgKind
	}
	for _, ev := range events {
		if k, ok := ev[objects.FieldKeyKind].(string); ok && k != emptyValue {
			return k
		}
		if k, ok := ev[objects.FieldKeyTargetKind].(string); ok && k != emptyValue {
			return k
		}
	}
	return cfgKind
}

// SamplerConfig defines sampling and batching configuration for metrics
type SamplerConfig struct {
	// Enabled controls whether sampling is enabled for this metric type
	Enabled bool `yaml:"enabled"`

	// BatchSize is the number of events to collect before creating a metric object
	// Must be between 1 and MaxBatchSize
	BatchSize int `yaml:"batch_size"`

	// MaxBatchSize is the maximum allowed batch size (safety limit)
	MaxBatchSize int `yaml:"max_batch_size"`

	// FlushInterval is the maximum time to wait before flushing a partial batch
	// If a batch is not full after this interval, it will be flushed anyway
	FlushInterval time.Duration `yaml:"flush_interval"`

	// GroupByObjectID if true, batches are per-object (objectID -> batch)
	// If false, batches are global (all objects combined)
	GroupByObjectID bool `yaml:"group_by_object_id"`

	// MetricType is the metric type this sampler handles
	MetricType string `yaml:"metric_type"`

	// ObjectKind is the object kind this sampler handles
	ObjectKind string `yaml:"object_kind"`

	// FieldName is the field name this sampler handles (optional)
	FieldName string `yaml:"field_name"`
}

// DefaultSamplerConfig returns a default sampler configuration
func DefaultSamplerConfig() *SamplerConfig {
	return &SamplerConfig{
		Enabled:         true,
		BatchSize:       50,
		MaxBatchSize:    1000,
		FlushInterval:   5 * time.Minute,
		GroupByObjectID: true,
		MetricType:      "",
		ObjectKind:      "",
		FieldName:       "",
	}
}

// Validate validates the sampler configuration
func (sc *SamplerConfig) Validate() error {
	if sc.BatchSize < 1 {
		return errfmt.Errorf("batch_size must be >= 1")
	}
	if sc.MaxBatchSize < sc.BatchSize {
		return errfmt.Errorf("max_batch_size must be >= batch_size")
	}
	if sc.BatchSize > sc.MaxBatchSize {
		return errfmt.Errorf("batch_size (%d) exceeds max_batch_size (%d)", sc.BatchSize, sc.MaxBatchSize)
	}
	if sc.FlushInterval < 0 {
		return errfmt.Errorf("flush_interval must be >= 0")
	}
	return nil
}

// SamplerBatch represents a batch of events being collected in memory
type SamplerBatch struct {
	ObjectID  string           // Object ID (if GroupByObjectID is true)
	Events    []map[string]any // Collected events
	FirstSeen time.Time        // Timestamp of first event in batch
	LastSeen  time.Time        // Timestamp of last event in batch
	CreatedAt time.Time        // When batch was created
	mu        sync.Mutex
}

// maxSamplerBatchKeys caps the number of distinct batch keys per sampler to prevent
// unbounded memory growth when GroupByObjectID is true (one batch per object ID).
// When over cap, oldest batches (by LastSeen) are flushed before adding new ones.
const maxSamplerBatchKeys = 2000

// Sampler manages in-memory batching of events before creating metric objects
type Sampler struct {
	config      *SamplerConfig
	batches     map[string]*SamplerBatch // Key: objectID (if GroupByObjectID) or "global"
	storage     storage.ObjectStorageProvider
	aggregator  MetricAggregator
	flushTicker *time.Ticker
	mu          sync.RWMutex
	logger      logging.Logger
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewSampler creates a new sampler with the given configuration
func NewSampler(config *SamplerConfig, storageProvider storage.ObjectStorageProvider, aggregator MetricAggregator) (*Sampler, error) {
	if err := config.Validate(); err != nil {
		return nil, errfmt.Newf("invalid sampler config").Wrap(err)
	}

	// Use system context for internal cancellation (long-lived component)
	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())

	s := &Sampler{
		config:     config,
		batches:    make(map[string]*SamplerBatch),
		storage:    storageProvider,
		aggregator: aggregator,
		logger:     logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
		ctx:        ctx,
		cancel:     cancel,
	}

	// Start flush ticker if flush interval is set
	if config.FlushInterval > 0 {
		s.flushTicker = time.NewTicker(config.FlushInterval)
		goroutinelabels.NewGoroutine("metrics_sampler_flush_ticker", "periodically flushing metric samples").
			WithCleanup(func() {
				if s.flushTicker != nil {
					s.flushTicker.Stop()
				}
			}).
			StartWithContext(s.ctx, func(ctx context.Context) error {
				s.flushTickerLoop()
				return nil
			})
	}

	return s, nil
}

// Sample adds an event to the sampler batch
// Returns true if the batch was flushed (full or timeout)
func (s *Sampler) Sample(event map[string]any) (bool, error) {
	kind, _ := event[objects.FieldKeyKind].(string)
	if kind == emptyValue {
		kind, _ = event[objects.FieldKeyTargetKind].(string)
	}
	if kind == emptyValue {
		kind = s.config.ObjectKind
	}
	if !metricsrecording.EnabledForKind(kind) {
		return false, nil
	}
	if !s.config.Enabled {
		// Sampling disabled - create metric immediately
		return s.createMetricImmediately(event)
	}

	// Determine batch key
	batchKey := s.getBatchKey(event)

	var batch *SamplerBatch
	var exists bool
	var evictCandidates []struct {
		key       string
		createdAt time.Time
	}
	var batchCount int
	_ = concurrency.RunInLockWithLogger(
		&s.mu, LockNameSamplerGetOrCreateBatch, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			batch, ok = s.batches[batchKey]
			exists = ok
			if !exists {
				batch = &SamplerBatch{
					ObjectID:  batchKey,
					Events:    make([]map[string]any, 0, s.config.BatchSize),
					CreatedAt: time.Now().UTC(),
				}
				s.batches[batchKey] = batch
			}
			batchCount = len(s.batches)
			if batchCount > maxSamplerBatchKeys {
				evictCandidates = make([]struct {
					key       string
					createdAt time.Time
				}, 0, batchCount-1)
				for k, b := range s.batches {
					if k == batchKey {
						continue
					}
					evictCandidates = append(evictCandidates, struct {
						key       string
						createdAt time.Time
					}{key: k, createdAt: b.CreatedAt})
				}
			}
			return nil
		},
	)

	// Evict oldest batches (by CreatedAt) when over cap so s.batches stays bounded (avoids memory leak).
	if len(evictCandidates) > 0 {
		n := batchCount - maxSamplerBatchKeys
		if n > 0 {
			sort.Slice(evictCandidates, func(i, j int) bool {
				return evictCandidates[i].createdAt.Before(evictCandidates[j].createdAt)
			})
			for i := 0; i < n && i < len(evictCandidates); i++ {
				_ = s.flushBatch(evictCandidates[i].key) // best effort
			}
		}
	}

	// Add event to batch
	now := time.Now().UTC()
	var eventCount int
	_ = concurrency.RunInLockWithLogger(
		&batch.mu, LockNameSamplerBatchAddEvent, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if len(batch.Events) == 0 {
				batch.FirstSeen = now
			}
			batch.LastSeen = now
			batch.Events = append(batch.Events, event)
			eventCount = len(batch.Events)
			return nil
		},
	)

	// Check if batch is full
	if eventCount >= s.config.BatchSize {
		// Flush batch
		if err := s.flushBatch(batchKey); err != nil {
			return false, errfmt.Newf("failed to flush batch").Wrap(err)
		}
		return true, nil
	}

	return false, nil
}

// getBatchKey returns the key for the batch this event belongs to
func (s *Sampler) getBatchKey(event map[string]any) string {
	if s.config.GroupByObjectID {
		// Use object ID as batch key
		if objectID, ok := event["object_id"].(string); ok && objectID != emptyValue {
			return objectID
		}
		if targetID, ok := event[objects.FieldKeyTargetID].(string); ok && targetID != emptyValue {
			return targetID
		}
		// Fallback to kind if object ID not available
		if kind, ok := event[objects.FieldKeyKind].(string); ok {
			return kind
		}
	}
	// Global batch
	return "global"
}

// flushBatch flushes a batch and creates a metric object
func (s *Sampler) flushBatch(batchKey string) error {
	var batch *SamplerBatch
	var exists bool
	_ = concurrency.RunInLockWithLogger(
		&s.mu, LockNameSamplerFlushBatchGet, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			batch, ok = s.batches[batchKey]
			exists = ok
			if exists {
				delete(s.batches, batchKey)
			}
			return nil
		},
	)
	if !exists {
		return nil // Batch already flushed or doesn't exist
	}

	var events []map[string]any
	var firstSeen, lastSeen time.Time
	_ = concurrency.RunInLockWithLogger(
		&batch.mu, LockNameSamplerFlushBatchCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			events = batch.Events
			firstSeen = batch.FirstSeen
			lastSeen = batch.LastSeen
			return nil
		},
	)

	if len(events) == 0 {
		return nil // Empty batch, nothing to do
	}

	// Create aggregation config
	objectKind := inferAggregationObjectKind(s.config.ObjectKind, events)
	config := &AggregationConfig{
		ObjectKind:   objectKind,
		FieldName:    s.config.FieldName,
		MetricType:   s.config.MetricType,
		WindowStart:  firstSeen,
		WindowEnd:    lastSeen,
		Filters:      make(map[string]any),
		Aggregations: []string{aggregationCountMetric}, // Default aggregation
	}

	// Aggregate events
	result, err := s.aggregateEvents(events, config)
	if err != nil {
		return errfmt.Newf("failed to aggregate events").Wrap(err)
	}

	// Create metric object
	_, err = s.createMetricObject(s.ctx, config, result, len(events))
	if err != nil {
		return errfmt.Newf("failed to create metric object").Wrap(err)
	}

	logging.Fluent(s.logger).Debug("Flushed sampler batch").
		String("batch_key", batchKey).
		Int("event_count", len(events)).
		String("window_start", firstSeen.Format(time.RFC3339)).
		String("window_end", lastSeen.Format(time.RFC3339)).
		Log()

	return nil
}

// aggregateEvents aggregates a batch of events
//
//nolint:unparam // Always returns nil error - function is designed to always succeed
func (s *Sampler) aggregateEvents(events []map[string]any, _ *AggregationConfig) (map[string]any, error) {
	// For now, use the aggregator's Aggregate method
	// In the future, we could optimize this for in-memory aggregation
	// For audit events, we might want to count by event_type, severity, etc.

	aggregations := make(map[string]any)
	aggregations[aggregationCountMetric] = len(events)

	// Count by event type if available
	eventTypeCounts := make(map[string]int)
	for _, event := range events {
		if eventType, ok := event[objects.FieldKeyEventType].(string); ok {
			eventTypeCounts[eventType]++
		}
	}
	if len(eventTypeCounts) > 0 {
		aggregations[objects.FieldKeyEventTypeCounts] = eventTypeCounts
	}

	// Count by severity if available
	severityCounts := make(map[string]int)
	for _, event := range events {
		if severity, ok := event[objects.FieldKeySeverity].(string); ok {
			severityCounts[severity]++
		}
	}
	if len(severityCounts) > 0 {
		aggregations["severity_counts"] = severityCounts
	}

	return aggregations, nil
}

// createMetricObject persists a sampled batch as a base_metric (same goroutine as flush).
//
//nolint:unparam // Return value is intentionally unused - function creates metric object but doesn't use the ID
func (s *Sampler) createMetricObject(
	ctx context.Context,
	config *AggregationConfig,
	aggregations map[string]any,
	eventCount int,
) (_ string, err error) {
	if !metricsrecording.EnabledForKind(config.ObjectKind) {
		return "", nil
	}
	title := fmt.Sprintf("Sampled metric aggregation: %s from %s to %s (%d events)",
		config.ObjectKind,
		config.WindowStart.Format("2006-01-02 15:04:05"),
		config.WindowEnd.Format("2006-01-02 15:04:05"),
		eventCount)

	objConfig := &MetricObjectConfig{
		Source:  MetricSourcePipelineSampler,
		Sampled: true,
	}

	factory := NewMetricFactory(s.storage)

	additionalFields := make(map[string]any, len(aggregations)+1)
	additionalFields[objects.FieldKeyBatchSize] = s.config.BatchSize
	maps.Copy(additionalFields, aggregations)

	// Create on this goroutine. Async+wait doubled Gs (coordinator_metrics_router
	// parked on select, metric_factory_async_config parked on stream registry lock).
	return factory.CreateMetricFromConfig(ctx, config, title, eventCount, objConfig, additionalFields)
}

// createMetricImmediately creates a metric object immediately (sampling disabled)
func (s *Sampler) createMetricImmediately(event map[string]any) (bool, error) {
	objectKind := inferAggregationObjectKind(s.config.ObjectKind, []map[string]any{event})
	if !metricsrecording.EnabledForKind(objectKind) {
		return false, nil
	}
	config := &AggregationConfig{
		ObjectKind:   objectKind,
		FieldName:    s.config.FieldName,
		MetricType:   s.config.MetricType,
		WindowStart:  time.Now().UTC(),
		WindowEnd:    time.Now().UTC(),
		Filters:      make(map[string]any),
		Aggregations: []string{aggregationCountMetric},
	}

	aggregations, err := s.aggregateEvents([]map[string]any{event}, config)
	if err != nil {
		return false, err
	}

	_, err = s.createMetricObject(s.ctx, config, aggregations, 1)
	return err == nil, err
}

// flushTickerLoop periodically flushes partial batches
func (s *Sampler) flushTickerLoop() {
	// This method is now called from StartWithContext, so ctx parameter is available
	// But we keep the method signature for backward compatibility
	// The actual context is passed via closure in StartWithContext
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.flushTicker.C:
			_ = s.FlushAll() //nolint:errcheck // Periodic flush - errors are non-critical
		}
	}
}

// FlushAll flushes all pending batches
func (s *Sampler) FlushAll() error {
	var batchKeys []string
	_ = concurrency.RunInLockWithLogger(
		&s.mu, LockNameSamplerFlushAllCopyKeys, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			batchKeys = make([]string, 0, len(s.batches))
			for key := range s.batches {
				batchKeys = append(batchKeys, key)
			}
			return nil
		},
	)

	var errors []error
	for _, key := range batchKeys {
		if err := s.flushBatch(key); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return errfmt.Errorf("failed to flush some batches: %v", errors)
	}

	return nil
}

// Stop stops the sampler and flushes all pending batches
func (s *Sampler) Stop() error {
	if s.flushTicker != nil {
		s.flushTicker.Stop()
	}
	s.cancel()
	return s.FlushAll()
}

// GetBatchCount returns the number of active batches
func (s *Sampler) GetBatchCount() int {
	var count int
	_ = concurrency.RunInRLockWithLogger(
		&s.mu, LockNameSamplerGetBatchCount, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			count = len(s.batches)
			return nil
		},
	)
	return count
}

// GetTotalPendingEvents returns the total number of events in all batches
func (s *Sampler) GetTotalPendingEvents() int {
	var batchesCopy map[string]*SamplerBatch
	_ = concurrency.RunInRLockWithLogger(
		&s.mu, LockNameSamplerGetTotalEventsCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			batchesCopy = make(map[string]*SamplerBatch, len(s.batches))
			maps.Copy(batchesCopy, s.batches)
			return nil
		},
	)

	total := 0
	for _, batch := range batchesCopy {
		var eventCount int
		_ = concurrency.RunInLockWithLogger(
			&batch.mu, LockNameSamplerBatchGetEventCount, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				eventCount = len(batch.Events)
				return nil
			},
		)
		total += eventCount
	}
	return total
}
