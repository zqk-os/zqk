package cas

import (
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
)

// CASMetrics tracks metrics for Content Addressable Storage operations
type CASMetrics struct {
	// Operation counts
	Creates atomic.Int64 // Total number of Create operations
	Reads   atomic.Int64 // Total number of Read operations
	Updates atomic.Int64 // Total number of Update operations
	Deletes atomic.Int64 // Total number of Delete operations

	// Index operation counts
	SetMappings    atomic.Int64 // Total number of SetMapping operations
	RemoveMappings atomic.Int64 // Total number of RemoveMapping operations
	IndexLoads     atomic.Int64 // Total number of index Load operations
	IndexSaves     atomic.Int64 // Total number of index Save operations
	IndexReloads   atomic.Int64 // Total number of index reloads (during SetMapping)

	// Failure counts
	CreateFailures atomic.Int64 // Total number of failed Create operations
	ReadFailures   atomic.Int64 // Total number of failed Read operations
	UpdateFailures atomic.Int64 // Total number of failed Update operations
	DeleteFailures atomic.Int64 // Total number of failed Delete operations
	IndexFailures  atomic.Int64 // Total number of failed index operations

	// Timing metrics (in nanoseconds)
	TotalCreateTime atomic.Int64 // Total time spent on Create operations
	TotalReadTime   atomic.Int64 // Total time spent on Read operations
	TotalUpdateTime atomic.Int64 // Total time spent on Update operations
	TotalDeleteTime atomic.Int64 // Total time spent on Delete operations
	TotalIndexTime  atomic.Int64 // Total time spent on index operations (SetMapping, Save, Load)

	// Max timing metrics (in nanoseconds)
	MaxCreateTime atomic.Int64 // Maximum time for a Create operation
	MaxReadTime   atomic.Int64 // Maximum time for a Read operation
	MaxUpdateTime atomic.Int64 // Maximum time for an Update operation
	MaxDeleteTime atomic.Int64 // Maximum time for a Delete operation
	MaxIndexTime  atomic.Int64 // Maximum time for an index operation

	// Index-specific metrics
	IndexReloadsDuringSetMapping atomic.Int64 // Number of times index was reloaded during SetMapping (indicates concurrent updates)
	IndexMergeOperations         atomic.Int64 // Number of times index merge occurred (loadLocked merges)
	IndexEntriesAdded            atomic.Int64 // Total number of entries added to index
	IndexEntriesRemoved          atomic.Int64 // Total number of entries removed from index
	StaleEntriesRemoved          atomic.Int64 // Stale entries removed during save validation (file not found)
	StaleValidationTimeNs        atomic.Int64 // Total time spent validating file existence before save (nanoseconds)
	IndexSize                    atomic.Int64 // Current index size (number of mappings)

	// Lock contention metrics
	IndexLockContention atomic.Int64 // Number of times index lock was contended (indicates concurrent access)
	IndexLockWaitTime   atomic.Int64 // Total time spent waiting for index lock (nanoseconds)

	// File locking metrics (for index file)
	IndexFileLockAcquisitions atomic.Int64 // Number of times index file lock was acquired
	IndexFileLockFailures     atomic.Int64 // Number of times index file lock acquisition failed
	IndexFileLockWaitTime     atomic.Int64 // Total time spent waiting for index file lock (nanoseconds)

	// Batch processing metrics (for write queue)
	BatchesProcessed         atomic.Int64 // Total number of batches processed
	BatchProcessingFailures  atomic.Int64 // Total number of failed batch processing operations
	TotalBatchProcessingTime atomic.Int64 // Total time spent processing batches (nanoseconds)
	MaxBatchProcessingTime   atomic.Int64 // Maximum time for a batch processing operation
	TotalUpdatesBatched      atomic.Int64 // Total number of updates processed in batches
	AverageBatchSize         atomic.Int64 // Average batch size (calculated from total updates / batches)

	// Orphan cleanup metrics
	OrphanCleanupBatchesProcessed atomic.Int64 // Total number of orphan cleanup batches processed
	OrphanCleanupSuccessCount     atomic.Int64 // Total number of successful orphan file deletions
	OrphanCleanupFailureCount     atomic.Int64 // Total number of failed orphan file deletions
	TotalOrphanCleanupTime        atomic.Int64 // Total time spent on orphan cleanup (nanoseconds)
	MaxOrphanCleanupTime          atomic.Int64 // Maximum time for an orphan cleanup batch

	// New builder-pattern recorder (stored as any to avoid import cycles)
	recorder any // observability.Recorder

	recorderMu sync.Mutex // protects lazy init of recorder (getCASMetricsRecorder)
}

// RecordOrphanCleanupBatch records metrics for an orphan cleanup batch operation
func (m *CASMetrics) RecordOrphanCleanupBatch(duration time.Duration, batchSize, successCount, failureCount int) {
	if !metricsrecording.Enabled() {
		return
	}
	m.OrphanCleanupBatchesProcessed.Add(1)
	m.OrphanCleanupSuccessCount.Add(int64(successCount))
	m.OrphanCleanupFailureCount.Add(int64(failureCount))

	durationNs := int64(duration)
	m.TotalOrphanCleanupTime.Add(durationNs)

	// Update max cleanup time
	for {
		current := m.MaxOrphanCleanupTime.Load()
		if durationNs <= current {
			break
		}
		if m.MaxOrphanCleanupTime.CompareAndSwap(current, durationNs) {
			break
		}
	}
}

var globalCASMetrics = &CASMetrics{}

// GetCASMetrics returns the global CAS metrics
func GetCASMetrics() *CASMetrics {
	return globalCASMetrics
}

// ResetCASMetrics resets all metrics (useful for testing)
func ResetCASMetrics() {
	globalCASMetrics = &CASMetrics{}
}

// ObjectStorageMetrics is a type alias for [CASMetrics]. Legacy name "CAS" refers to content-addressed
// file layout in [FileObjectStorage], not stream segments or data cells. Prefer [ObjectStorageMetrics]
// identifiers in new code (see docs/architecture/STORAGE_PUBLIC_API_NEUTRAL_ALIASES.md).
type ObjectStorageMetrics = CASMetrics

// GetObjectStorageMetrics returns the global file-object storage metrics (same singleton as [GetCASMetrics]).
func GetObjectStorageMetrics() *ObjectStorageMetrics {
	return GetCASMetrics()
}

// ResetObjectStorageMetrics resets metrics for tests (same as [ResetCASMetrics]).
func ResetObjectStorageMetrics() {
	ResetCASMetrics()
}

// RecordCreate records a Create operation
func (m *CASMetrics) RecordCreate(duration time.Duration, err error) {
	if !metricsrecording.Enabled() {
		return
	}
	m.Creates.Add(1)
	if err != nil {
		m.CreateFailures.Add(1)
	} else {
		durationNs := int64(duration)
		m.TotalCreateTime.Add(durationNs)

		// Update max create time
		for {
			current := m.MaxCreateTime.Load()
			if durationNs <= current {
				break
			}
			if m.MaxCreateTime.CompareAndSwap(current, durationNs) {
				break
			}
		}
	}

	// Record using new builder-pattern API
	recorder := m.getCASMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildCASOperationMetric("put", duration, err)
		if err := recorder.Record(ConstStreamContentAddressedPut, builder); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstStreamFailedToRecordContentAddressedPutMetricValN, err).Log()
		}
	}
}

// RecordRead records a Read operation
func (m *CASMetrics) RecordRead(duration time.Duration, err error) {
	if !metricsrecording.Enabled() {
		return
	}
	m.Reads.Add(1)
	if err != nil {
		m.ReadFailures.Add(1)
	} else {
		durationNs := int64(duration)
		m.TotalReadTime.Add(durationNs)

		// Update max read time
		for {
			current := m.MaxReadTime.Load()
			if durationNs <= current {
				break
			}
			if m.MaxReadTime.CompareAndSwap(current, durationNs) {
				break
			}
		}
	}

	// Record using new builder-pattern API
	recorder := m.getCASMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildCASOperationMetric("read", duration, err)
		if err := recorder.Record(ConstStreamContentAddressedRead, builder); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstStreamFailedToRecordContentAddressedReadMetricValN, err).Log()
		}
	}
}

// RecordUpdate records an Update operation
func (m *CASMetrics) RecordUpdate(duration time.Duration, err error) {
	if !metricsrecording.Enabled() {
		return
	}
	m.Updates.Add(1)
	if err != nil {
		m.UpdateFailures.Add(1)
		return
	}

	durationNs := int64(duration)
	m.TotalUpdateTime.Add(durationNs)

	// Update max update time
	for {
		current := m.MaxUpdateTime.Load()
		if durationNs <= current {
			break
		}
		if m.MaxUpdateTime.CompareAndSwap(current, durationNs) {
			break
		}
	}
}

// RecordDelete records a Delete operation
func (m *CASMetrics) RecordDelete(duration time.Duration, err error) {
	if !metricsrecording.Enabled() {
		return
	}
	m.Deletes.Add(1)
	if err != nil {
		m.DeleteFailures.Add(1)
	} else {
		durationNs := int64(duration)
		m.TotalDeleteTime.Add(durationNs)

		// Update max delete time
		for {
			current := m.MaxDeleteTime.Load()
			if durationNs <= current {
				break
			}
			if m.MaxDeleteTime.CompareAndSwap(current, durationNs) {
				break
			}
		}
	}

	// Record using new builder-pattern API
	recorder := m.getCASMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildCASOperationMetric(OpDelete, duration, err)
		if err := recorder.Record(ConstStreamContentAddressedDelete, builder); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstStreamFailedToRecordContentAddressedDeleteMetricValN, err).Log()
		}
	}
}

// RecordSetMapping records a SetMapping operation
func (m *CASMetrics) RecordSetMapping(duration time.Duration, err error, reloaded bool) {
	if !metricsrecording.Enabled() {
		return
	}
	m.SetMappings.Add(1)
	if err != nil {
		m.IndexFailures.Add(1)
	} else {
		if reloaded {
			m.IndexReloadsDuringSetMapping.Add(1)
		}

		durationNs := int64(duration)
		m.TotalIndexTime.Add(durationNs)
		m.IndexEntriesAdded.Add(1)

		// Update max index time
		for {
			current := m.MaxIndexTime.Load()
			if durationNs <= current {
				break
			}
			if m.MaxIndexTime.CompareAndSwap(current, durationNs) {
				break
			}
		}
	}

	// Record using new builder-pattern API
	recorder := m.getCASMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildListingIndexOperationMetric("set_mapping", duration, err, map[string]any{
			"reloaded": reloaded,
		})
		if err := recorder.Record(ConstStreamListingIndexSetMapping, builder); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstStreamFailedToRecordListingIndexSetMappingMetricValN, err).Log()
		}
	}
}

// RecordRemoveMapping records a RemoveMapping operation
func (m *CASMetrics) RecordRemoveMapping(duration time.Duration, err error) {
	if !metricsrecording.Enabled() {
		return
	}
	m.RemoveMappings.Add(1)
	if err != nil {
		m.IndexFailures.Add(1)
	} else {
		durationNs := int64(duration)
		m.TotalIndexTime.Add(durationNs)
		m.IndexEntriesRemoved.Add(1)

		// Update max index time
		for {
			current := m.MaxIndexTime.Load()
			if durationNs <= current {
				break
			}
			if m.MaxIndexTime.CompareAndSwap(current, durationNs) {
				break
			}
		}
	}

	// Record using new builder-pattern API
	recorder := m.getCASMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildListingIndexOperationMetric(ConstStreamRemoveMapping, duration, err, nil)
		if err := recorder.Record(ConstStreamListingIndexRemoveMapping, builder); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstStreamFailedToRecordListingIndexRemoveMappingMetricValN, err).Log()
		}
	}
}

// RecordIndexLoad records an index Load operation
func (m *CASMetrics) RecordIndexLoad(duration time.Duration, err error, entriesLoaded int) {
	if !metricsrecording.Enabled() {
		return
	}
	m.IndexLoads.Add(1)
	if err != nil {
		m.IndexFailures.Add(1)
	} else {
		durationNs := int64(duration)
		m.TotalIndexTime.Add(durationNs)

		// Update max index time
		for {
			current := m.MaxIndexTime.Load()
			if durationNs <= current {
				break
			}
			if m.MaxIndexTime.CompareAndSwap(current, durationNs) {
				break
			}
		}
	}

	// Record using new builder-pattern API
	recorder := m.getCASMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildListingIndexOperationMetric("load", duration, err, map[string]any{
			ConstStreamEntriesLoaded: entriesLoaded,
		})
		if err := recorder.Record(ConstStreamListingIndexLoad, builder); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstStreamFailedToRecordListingIndexLoadMetricValN, err).Log()
		}
	}
}

// RecordIndexSave records an index Save operation
func (m *CASMetrics) RecordIndexSave(duration time.Duration, err error, entriesSaved int) {
	if !metricsrecording.Enabled() {
		return
	}
	m.IndexSaves.Add(1)
	if err != nil {
		m.IndexFailures.Add(1)
	} else {
		durationNs := int64(duration)
		m.TotalIndexTime.Add(durationNs)

		// Update max index time
		for {
			current := m.MaxIndexTime.Load()
			if durationNs <= current {
				break
			}
			if m.MaxIndexTime.CompareAndSwap(current, durationNs) {
				break
			}
		}

		// Update index size
		m.IndexSize.Store(int64(entriesSaved))
	}

	// Record using new builder-pattern API
	recorder := m.getCASMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildListingIndexOperationMetric("save", duration, err, map[string]any{
			"entries_saved": entriesSaved,
		})
		if err := recorder.Record(ConstStreamListingIndexSave, builder); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstStreamFailedToRecordListingIndexSaveMetricValN, err).Log()
		}
	}
}

// RecordIndexReload records an index reload (during SetMapping)
func (m *CASMetrics) RecordIndexReload() {
	if !metricsrecording.Enabled() {
		return
	}
	m.IndexReloads.Add(1)
}

// RecordIndexMerge records an index merge operation (loadLocked merge)
func (m *CASMetrics) RecordIndexMerge(entriesMerged int) {
	if !metricsrecording.Enabled() {
		return
	}
	m.IndexMergeOperations.Add(1)
}

// RecordIndexLockContention records index lock contention
func (m *CASMetrics) RecordIndexLockContention(waitTime time.Duration) {
	if !metricsrecording.Enabled() {
		return
	}
	m.IndexLockContention.Add(1)
	m.IndexLockWaitTime.Add(int64(waitTime))
}

// RecordIndexFileLock records index file lock acquisition
func (m *CASMetrics) RecordIndexFileLock(acquired bool, waitTime time.Duration) {
	if !metricsrecording.Enabled() {
		return
	}
	if acquired {
		m.IndexFileLockAcquisitions.Add(1)
		m.IndexFileLockWaitTime.Add(int64(waitTime))
	} else {
		m.IndexFileLockFailures.Add(1)
	}
}

// CASMetricsSnapshot provides a point-in-time view of CAS metrics
type CASMetricsSnapshot struct {
	Timestamp time.Time

	// Operation counts
	Creates        int64
	Reads          int64
	Updates        int64
	Deletes        int64
	SetMappings    int64
	RemoveMappings int64
	IndexLoads     int64
	IndexSaves     int64
	IndexReloads   int64

	// Failure counts
	CreateFailures int64
	ReadFailures   int64
	UpdateFailures int64
	DeleteFailures int64
	IndexFailures  int64

	// Average durations
	AvgCreateTime time.Duration
	AvgReadTime   time.Duration
	AvgUpdateTime time.Duration
	AvgDeleteTime time.Duration
	AvgIndexTime  time.Duration

	// Max durations
	MaxCreateTime time.Duration
	MaxReadTime   time.Duration
	MaxUpdateTime time.Duration
	MaxDeleteTime time.Duration
	MaxIndexTime  time.Duration

	// Index metrics
	IndexReloadsDuringSetMapping int64
	IndexMergeOperations         int64
	IndexEntriesAdded            int64
	IndexEntriesRemoved          int64
	IndexSize                    int64

	// Lock metrics
	IndexLockContention       int64
	IndexLockWaitTime         time.Duration
	IndexFileLockAcquisitions int64
	IndexFileLockFailures     int64
	IndexFileLockWaitTime     time.Duration
}

// GetTotalOperations returns the total number of operations for title generation
func (s CASMetricsSnapshot) GetTotalOperations() int64 {
	return s.Creates + s.Reads + s.Updates + s.Deletes
}

// GetSnapshot returns a snapshot of current metrics
func (m *CASMetrics) GetSnapshot() CASMetricsSnapshot {
	creates := m.Creates.Load()
	reads := m.Reads.Load()
	updates := m.Updates.Load()
	deletes := m.Deletes.Load()

	var avgCreateTime time.Duration
	if creates > 0 {
		avgCreateTime = time.Duration(m.TotalCreateTime.Load() / creates)
	}

	var avgReadTime time.Duration
	if reads > 0 {
		avgReadTime = time.Duration(m.TotalReadTime.Load() / reads)
	}

	var avgUpdateTime time.Duration
	if updates > 0 {
		avgUpdateTime = time.Duration(m.TotalUpdateTime.Load() / updates)
	}

	var avgDeleteTime time.Duration
	if deletes > 0 {
		avgDeleteTime = time.Duration(m.TotalDeleteTime.Load() / deletes)
	}

	totalIndexOps := m.SetMappings.Load() + m.RemoveMappings.Load() + m.IndexLoads.Load() + m.IndexSaves.Load()
	var avgIndexTime time.Duration
	if totalIndexOps > 0 {
		avgIndexTime = time.Duration(m.TotalIndexTime.Load() / totalIndexOps)
	}

	return CASMetricsSnapshot{
		Timestamp: time.Now(),

		Creates:        creates,
		Reads:          reads,
		Updates:        updates,
		Deletes:        deletes,
		SetMappings:    m.SetMappings.Load(),
		RemoveMappings: m.RemoveMappings.Load(),
		IndexLoads:     m.IndexLoads.Load(),
		IndexSaves:     m.IndexSaves.Load(),
		IndexReloads:   m.IndexReloads.Load(),

		CreateFailures: m.CreateFailures.Load(),
		ReadFailures:   m.ReadFailures.Load(),
		UpdateFailures: m.UpdateFailures.Load(),
		DeleteFailures: m.DeleteFailures.Load(),
		IndexFailures:  m.IndexFailures.Load(),

		AvgCreateTime: avgCreateTime,
		AvgReadTime:   avgReadTime,
		AvgUpdateTime: avgUpdateTime,
		AvgDeleteTime: avgDeleteTime,
		AvgIndexTime:  avgIndexTime,

		MaxCreateTime: time.Duration(m.MaxCreateTime.Load()),
		MaxReadTime:   time.Duration(m.MaxReadTime.Load()),
		MaxUpdateTime: time.Duration(m.MaxUpdateTime.Load()),
		MaxDeleteTime: time.Duration(m.MaxDeleteTime.Load()),
		MaxIndexTime:  time.Duration(m.MaxIndexTime.Load()),

		IndexReloadsDuringSetMapping: m.IndexReloadsDuringSetMapping.Load(),
		IndexMergeOperations:         m.IndexMergeOperations.Load(),
		IndexEntriesAdded:            m.IndexEntriesAdded.Load(),
		IndexEntriesRemoved:          m.IndexEntriesRemoved.Load(),
		IndexSize:                    m.IndexSize.Load(),

		IndexLockContention:       m.IndexLockContention.Load(),
		IndexLockWaitTime:         time.Duration(m.IndexLockWaitTime.Load()),
		IndexFileLockAcquisitions: m.IndexFileLockAcquisitions.Load(),
		IndexFileLockFailures:     m.IndexFileLockFailures.Load(),
		IndexFileLockWaitTime:     time.Duration(m.IndexFileLockWaitTime.Load()),
	}
}

// ToMap converts CASMetricsSnapshot to a map representation for structured logging and API export.
func (s CASMetricsSnapshot) ToMap() map[string]any {
	return map[string]any{
		"timestamp":                        s.Timestamp.Format(time.RFC3339),
		"creates":                          s.Creates,
		"reads":                            s.Reads,
		"updates":                          s.Updates,
		"deletes":                          s.Deletes,
		"set_mappings":                     s.SetMappings,
		"remove_mappings":                  s.RemoveMappings,
		"index_loads":                      s.IndexLoads,
		"index_saves":                      s.IndexSaves,
		"index_reloads":                    s.IndexReloads,
		"create_failures":                  s.CreateFailures,
		"read_failures":                    s.ReadFailures,
		"update_failures":                  s.UpdateFailures,
		"delete_failures":                  s.DeleteFailures,
		"index_failures":                   s.IndexFailures,
		"avg_create_time_ns":               s.AvgCreateTime.Nanoseconds(),
		"avg_read_time_ns":                 s.AvgReadTime.Nanoseconds(),
		"avg_update_time_ns":               s.AvgUpdateTime.Nanoseconds(),
		"avg_delete_time_ns":               s.AvgDeleteTime.Nanoseconds(),
		"avg_index_time_ns":                s.AvgIndexTime.Nanoseconds(),
		"max_create_time_ns":               s.MaxCreateTime.Nanoseconds(),
		"max_read_time_ns":                 s.MaxReadTime.Nanoseconds(),
		"max_update_time_ns":               s.MaxUpdateTime.Nanoseconds(),
		"max_delete_time_ns":               s.MaxDeleteTime.Nanoseconds(),
		"max_index_time_ns":                s.MaxIndexTime.Nanoseconds(),
		"index_reloads_during_set_mapping": s.IndexReloadsDuringSetMapping,
		"index_merge_operations":           s.IndexMergeOperations,
		"index_entries_added":              s.IndexEntriesAdded,
		"index_entries_removed":            s.IndexEntriesRemoved,
		"index_size":                       s.IndexSize,
		"index_lock_contention":            s.IndexLockContention,
		"index_lock_wait_time_ns":          s.IndexLockWaitTime.Nanoseconds(),
		"index_file_lock_acquisitions":     s.IndexFileLockAcquisitions,
		"index_file_lock_failures":         s.IndexFileLockFailures,
		"index_file_lock_wait_time_ns":     s.IndexFileLockWaitTime.Nanoseconds(),
		"total_operations":                 s.GetTotalOperations(),
	}
}
