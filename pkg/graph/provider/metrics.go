package provider

import (
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"

	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
)

// MetricsCollector is the interface for collecting and reporting metrics
// Implementations can send metrics to Prometheus, StatsD, custom backends, etc.
//
// DEPRECATED: New code should use the builder-pattern metrics API from pkg/observability.
// This interface is maintained for backward compatibility only.
// New code should use observability.Recorder with the builder pattern directly.
type MetricsCollector interface {
	// Operation metrics
	RecordOperation(operation string, duration time.Duration, err error)
	RecordRetry(operation string, attempt int, err error)

	// Connection metrics
	RecordConnectionAcquired(duration time.Duration)
	RecordConnectionReleased()
	RecordConnectionError(err error)

	// Transaction metrics
	RecordTransactionStarted()
	RecordTransactionCommitted(duration time.Duration)
	RecordTransactionRolledBack(duration time.Duration, reason string)
	RecordTransactionError(err error)

	// Pool metrics
	RecordPoolWait(duration time.Duration)
	RecordPoolSizeChange(active, idle, maxSize int)

	// Query metrics
	RecordQuery(operation string, duration time.Duration, rowsAffected int, err error)

	// Batch metrics
	RecordBatch(batchSize int, duration time.Duration, chunkCount int, fallbackSingle bool, err error)

	// Health metrics
	RecordHealthCheck(duration time.Duration, healthy bool)

	// Get metrics snapshot
	GetMetrics() MetricsSnapshot
}

// BatchMetrics tracks graph bulk insertion batching behavior
type BatchMetrics struct {
	TotalBatches        int64
	TotalItems          int64
	TotalChunks         int64
	FallbackSingleCount int64
	TotalDuration       time.Duration
	AvgBatchSize        float64
	AvgDuration         time.Duration
	FailureCount        int64
}

// MetricsSnapshot provides a point-in-time view of all metrics
type MetricsSnapshot struct {
	Timestamp time.Time

	// Operation metrics
	Operations      map[string]OperationMetrics
	TotalOperations int64

	// Retry metrics
	Retries      map[string]RetryMetrics
	TotalRetries int64

	// Connection metrics
	Connections ConnectionMetrics

	// Transaction metrics
	Transactions TransactionMetrics

	// Pool metrics
	Pool PoolMetrics

	// Query metrics
	Queries map[string]QueryMetrics

	// Batch metrics
	Batch BatchMetrics

	// Health metrics
	Health HealthMetrics
}

// OperationMetrics tracks metrics for a specific operation
type OperationMetrics struct {
	Operation     string
	Count         int64
	SuccessCount  int64
	FailureCount  int64
	TotalDuration time.Duration
	MinDuration   time.Duration
	MaxDuration   time.Duration
	AvgDuration   time.Duration
	LastError     error
	LastErrorTime time.Time
	ErrorRate     float64 // Percentage
}

// RetryMetrics tracks retry behavior
type RetryMetrics struct {
	Operation         string
	TotalRetries      int64
	SuccessfulRetries int64
	FailedRetries     int64
	AvgAttempts       float64
	MaxAttempts       int
}

// ConnectionMetrics tracks connection-level metrics
type ConnectionMetrics struct {
	TotalAcquired     int64
	TotalReleased     int64
	AcquireDuration   time.Duration
	AvgAcquireTime    time.Duration
	AcquireErrors     int64
	ReleaseErrors     int64
	ActiveConnections int
}

// TransactionMetrics tracks transaction-level metrics
type TransactionMetrics struct {
	TotalStarted      int64
	TotalCommitted    int64
	TotalRolledBack   int64
	CommitDuration    time.Duration
	RollbackDuration  time.Duration
	AvgCommitTime     time.Duration
	AvgRollbackTime   time.Duration
	TransactionErrors int64
	OpenTransactions  int
}

// PoolMetrics tracks pool-level metrics
type PoolMetrics struct {
	TotalWaits      int64
	TotalWaitTime   time.Duration
	AvgWaitTime     time.Duration
	MaxWaitTime     time.Duration
	PoolSizeChanges int64
	CurrentActive   int
	CurrentIdle     int
	MaxSize         int
	UtilizationRate float64 // Percentage
}

// QueryMetrics tracks query-level metrics
type QueryMetrics struct {
	Query         string
	Count         int64
	SuccessCount  int64
	FailureCount  int64
	TotalDuration time.Duration
	AvgDuration   time.Duration
	TotalRows     int64
	AvgRows       float64
	ErrorRate     float64
}

// HealthMetrics tracks health check metrics
type HealthMetrics struct {
	TotalChecks         int64
	HealthyChecks       int64
	UnhealthyChecks     int64
	AvgCheckTime        time.Duration
	LastCheckTime       time.Time
	LastCheckHealthy    bool
	ConsecutiveFailures int64
}

// DefaultMetricsCollector is a thread-safe in-memory metrics collector
// It can be used standalone or as a base for other implementations
// Supports configurable sampling and async recording for non-blocking operation
type DefaultMetricsCollector struct {
	mu sync.RWMutex

	config MetricsConfig

	operations map[string]*OperationMetrics
	retries    map[string]*RetryMetrics
	queries    map[string]*QueryMetrics

	connections  ConnectionMetrics
	transactions TransactionMetrics
	pool         PoolMetrics
	batch        BatchMetrics
	health       HealthMetrics

	// Async recording
	recordChan chan func()
	stopChan   chan struct{}
	running    int32 // Atomic flag

	startTime time.Time
}

// NewDefaultMetricsCollector creates a new default metrics collector
func NewDefaultMetricsCollector() *DefaultMetricsCollector {
	metricsConfig := DefaultMetricsConfig()
	return NewMetricsCollectorWithConfig(&metricsConfig)
}

// NewMetricsCollectorWithConfig creates a new metrics collector with custom configuration
func NewMetricsCollectorWithConfig(config *MetricsConfig) *DefaultMetricsCollector {
	collector := &DefaultMetricsCollector{
		config:     *config,
		operations: make(map[string]*OperationMetrics),
		retries:    make(map[string]*RetryMetrics),
		queries:    make(map[string]*QueryMetrics),
		startTime:  time.Now(),
	}

	// Start async recording if enabled
	if config.Enabled && config.AsyncRecording {
		collector.recordChan = make(chan func(), config.BufferSize)
		collector.stopChan = make(chan struct{})
		atomic.StoreInt32(&collector.running, 1)
		goroutinelabels.NewGoroutine("graph_provider_metrics_async_recorder", "recording graph provider metrics asynchronously").
			StartSimple(collector.asyncRecorder)
	}

	return collector
}

// asyncRecorder processes metrics recording in the background
func (m *DefaultMetricsCollector) asyncRecorder() {
	for {
		select {
		case <-m.stopChan:
			return
		case recordFn := <-m.recordChan:
			// Execute the recording function
			recordFn()
		}
	}
}

// recordAsync queues a recording operation for async execution
func (m *DefaultMetricsCollector) recordAsync(fn func()) {
	if !m.config.Enabled {
		return
	}

	if m.config.AsyncRecording && atomic.LoadInt32(&m.running) == 1 {
		select {
		case m.recordChan <- fn:
			// Successfully queued
		default:
			// Buffer full, drop metric (non-blocking)
			// In production, you might want to log this or use a larger buffer
		}
	} else {
		// Synchronous recording
		fn()
	}
}

// shouldSample determines if this operation should be sampled
func (m *DefaultMetricsCollector) shouldSample() bool {
	if !m.config.Enabled {
		return false
	}
	if m.config.SampleRate >= 1.0 {
		return true
	}
	// Simple sampling: use time-based sampling for thread-safety
	// In production, you might use a more sophisticated sampling algorithm
	return time.Now().UnixNano()%100 < int64(m.config.SampleRate*100)
}

// Stop stops async recording (call before shutdown)
func (m *DefaultMetricsCollector) Stop() {
	if atomic.LoadInt32(&m.running) == 1 {
		atomic.StoreInt32(&m.running, 0)
		close(m.stopChan)
		// Drain remaining records
		for {
			select {
			case recordFn := <-m.recordChan:
				recordFn()
			default:
				return
			}
		}
	}
}

// UpdateConfig updates the metrics configuration
func (m *DefaultMetricsCollector) UpdateConfig(config *MetricsConfig) {
	var oldAsync bool
	_ = concurrency.RunInLockWithLogger(
		&m.mu, LockNameMetricsCollectorUpdateConfig, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			oldAsync = m.config.AsyncRecording
			m.config = *config
			return nil
		},
	)

	// Restart async recorder if needed
	if config.Enabled && config.AsyncRecording && !oldAsync {
		if m.recordChan == nil {
			m.recordChan = make(chan func(), config.BufferSize)
		}
		if m.stopChan == nil {
			m.stopChan = make(chan struct{})
		}
		atomic.StoreInt32(&m.running, 1)
		goroutinelabels.NewGoroutine("graph_provider_metrics_async_recorder", "recording graph provider metrics asynchronously").
			StartSimple(m.asyncRecorder)
	} else if !config.AsyncRecording && oldAsync {
		// Stop async recorder
		m.Stop()
	}
}

func (m *DefaultMetricsCollector) RecordOperation(operation string, duration time.Duration, err error) {
	if !m.shouldSample() {
		return
	}

	m.recordAsync(func() {
		_ = concurrency.RunInLockWithLogger(
			&m.mu, LockNameMetricsCollectorRecordOperation, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				op, exists := m.operations[operation]
				if !exists {
					op = &OperationMetrics{
						Operation:   operation,
						MinDuration: duration,
						MaxDuration: duration,
					}
					m.operations[operation] = op
				}

				op.Count++
				op.TotalDuration += duration

				if duration < op.MinDuration || op.MinDuration == 0 {
					op.MinDuration = duration
				}
				if duration > op.MaxDuration {
					op.MaxDuration = duration
				}

				if err != nil {
					op.FailureCount++
					op.LastError = err
					op.LastErrorTime = time.Now()
				} else {
					op.SuccessCount++
				}

				op.AvgDuration = op.TotalDuration / time.Duration(op.Count)
				if op.Count > 0 {
					op.ErrorRate = float64(op.FailureCount) / float64(op.Count) * 100
				}
				return nil
			},
		)
	})
}

func (m *DefaultMetricsCollector) RecordRetry(operation string, attempt int, err error) {
	if !m.shouldSample() {
		return
	}

	m.recordAsync(func() {
		_ = concurrency.RunInLockWithLogger(
			&m.mu, LockNameMetricsCollectorRecordRetry, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				retry, exists := m.retries[operation]
				if !exists {
					retry = &RetryMetrics{Operation: operation}
					m.retries[operation] = retry
				}

				retry.TotalRetries++
				if attempt > retry.MaxAttempts {
					retry.MaxAttempts = attempt
				}

				if err == nil {
					retry.SuccessfulRetries++
				} else {
					retry.FailedRetries++
				}
				return nil
			},
		)
	})
}

func (m *DefaultMetricsCollector) RecordConnectionAcquired(duration time.Duration) {
	if !m.shouldSample() {
		return
	}

	m.recordAsync(func() {
		_ = concurrency.RunInLockWithLogger(
			&m.mu, LockNameMetricsCollectorRecordConnAcquired, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				m.connections.TotalAcquired++
				m.connections.AcquireDuration += duration
				m.connections.AvgAcquireTime = m.connections.AcquireDuration / time.Duration(m.connections.TotalAcquired)
				m.connections.ActiveConnections++
				return nil
			},
		)
	})
}

func (m *DefaultMetricsCollector) RecordConnectionReleased() {
	if !m.shouldSample() {
		return
	}

	m.recordAsync(func() {
		_ = concurrency.RunInLockWithLogger(
			&m.mu, LockNameMetricsCollectorRecordConnReleased, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				m.connections.TotalReleased++
				if m.connections.ActiveConnections > 0 {
					m.connections.ActiveConnections--
				}
				return nil
			},
		)
	})
}

func (m *DefaultMetricsCollector) RecordConnectionError(err error) {
	// Always record errors (not sampled)
	if !m.config.Enabled {
		return
	}

	m.recordAsync(func() {
		_ = concurrency.RunInLockWithLogger(
			&m.mu, LockNameMetricsCollectorRecordConnError, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				if err != nil {
					m.connections.AcquireErrors++
				}
				return nil
			},
		)
	})
}

func (m *DefaultMetricsCollector) RecordTransactionStarted() {
	// Always record transactions (not sampled)
	if !m.config.Enabled {
		return
	}

	m.recordAsync(func() {
		_ = concurrency.RunInLockWithLogger(
			&m.mu, LockNameMetricsCollectorRecordTxStarted, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				m.transactions.TotalStarted++
				m.transactions.OpenTransactions++
				return nil
			},
		)
	})
}

func (m *DefaultMetricsCollector) RecordTransactionCommitted(duration time.Duration) {
	// Always record transactions (not sampled)
	if !m.config.Enabled {
		return
	}

	m.recordAsync(func() {
		_ = concurrency.RunInLockWithLogger(
			&m.mu, LockNameMetricsCollectorRecordTxCommitted, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				m.transactions.TotalCommitted++
				m.transactions.CommitDuration += duration
				m.transactions.AvgCommitTime = m.transactions.CommitDuration / time.Duration(m.transactions.TotalCommitted)
				if m.transactions.OpenTransactions > 0 {
					m.transactions.OpenTransactions--
				}
				return nil
			},
		)
	})
}

func (m *DefaultMetricsCollector) RecordTransactionRolledBack(duration time.Duration, reason string) {
	// Always record transactions (not sampled)
	if !m.config.Enabled {
		return
	}

	m.recordAsync(func() {
		_ = concurrency.RunInLockWithLogger(
			&m.mu, LockNameMetricsCollectorRecordTxRolledBack, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				m.transactions.TotalRolledBack++
				m.transactions.RollbackDuration += duration
				m.transactions.AvgRollbackTime = m.transactions.RollbackDuration / time.Duration(m.transactions.TotalRolledBack)
				if m.transactions.OpenTransactions > 0 {
					m.transactions.OpenTransactions--
				}
				return nil
			},
		)
	})
}

func (m *DefaultMetricsCollector) RecordTransactionError(err error) {
	// Always record errors (not sampled)
	if !m.config.Enabled {
		return
	}

	m.recordAsync(func() {
		_ = concurrency.RunInLockWithLogger(
			&m.mu, LockNameMetricsCollectorRecordTxError, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				m.transactions.TransactionErrors++
				return nil
			},
		)
	})
}

func (m *DefaultMetricsCollector) RecordPoolWait(duration time.Duration) {
	if !m.shouldSample() {
		return
	}

	m.recordAsync(func() {
		_ = concurrency.RunInLockWithLogger(
			&m.mu, LockNameMetricsCollectorRecordPoolWait, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				m.pool.TotalWaits++
				m.pool.TotalWaitTime += duration
				m.pool.AvgWaitTime = m.pool.TotalWaitTime / time.Duration(m.pool.TotalWaits)
				if duration > m.pool.MaxWaitTime {
					m.pool.MaxWaitTime = duration
				}
				return nil
			},
		)
	})
}

func (m *DefaultMetricsCollector) RecordPoolSizeChange(active, idle, maxSize int) {
	// Always record pool changes (not sampled)
	if !m.config.Enabled {
		return
	}

	m.recordAsync(func() {
		_ = concurrency.RunInLockWithLogger(
			&m.mu, LockNameMetricsCollectorRecordPoolSize, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				m.pool.CurrentActive = active
				m.pool.CurrentIdle = idle
				m.pool.MaxSize = maxSize
				m.pool.PoolSizeChanges++
				if maxSize > 0 {
					m.pool.UtilizationRate = float64(active) / float64(maxSize) * 100
				}
				return nil
			},
		)
	})
}

func (m *DefaultMetricsCollector) RecordQuery(operation string, duration time.Duration, rowsAffected int, err error) {
	if !m.shouldSample() {
		return
	}

	m.recordAsync(func() {
		_ = concurrency.RunInLockWithLogger(
			&m.mu, LockNameMetricsCollectorRecordQuery, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				query, exists := m.queries[operation]
				if !exists {
					query = &QueryMetrics{Query: operation}
					m.queries[operation] = query
				}

				query.Count++
				query.TotalDuration += duration
				query.AvgDuration = query.TotalDuration / time.Duration(query.Count)
				query.TotalRows += int64(rowsAffected)
				query.AvgRows = float64(query.TotalRows) / float64(query.Count)

				if err != nil {
					query.FailureCount++
				} else {
					query.SuccessCount++
				}

				if query.Count > 0 {
					query.ErrorRate = float64(query.FailureCount) / float64(query.Count) * 100
				}
				return nil
			},
		)
	})
}

func (m *DefaultMetricsCollector) RecordBatch(batchSize int, duration time.Duration, chunkCount int, fallbackSingle bool, err error) {
	if !m.config.Enabled {
		return
	}

	m.recordAsync(func() {
		_ = concurrency.RunInLockWithLogger(
			&m.mu, LockNameMetricsCollectorRecordQuery, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				m.batch.TotalBatches++
				m.batch.TotalItems += int64(batchSize)
				m.batch.TotalChunks += int64(chunkCount)
				m.batch.TotalDuration += duration
				if fallbackSingle {
					m.batch.FallbackSingleCount++
				}
				if err != nil {
					m.batch.FailureCount++
				}
				if m.batch.TotalBatches > 0 {
					m.batch.AvgBatchSize = float64(m.batch.TotalItems) / float64(m.batch.TotalBatches)
					m.batch.AvgDuration = m.batch.TotalDuration / time.Duration(m.batch.TotalBatches)
				}
				return nil
			},
		)
	})
}

func (m *DefaultMetricsCollector) RecordHealthCheck(duration time.Duration, healthy bool) {
	// Always record health checks (not sampled)
	if !m.config.Enabled {
		return
	}

	m.recordAsync(func() {
		_ = concurrency.RunInLockWithLogger(
			&m.mu, LockNameMetricsCollectorRecordHealth, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				m.health.TotalChecks++
				m.health.AvgCheckTime = (m.health.AvgCheckTime*time.Duration(m.health.TotalChecks-1) + duration) / time.Duration(m.health.TotalChecks)
				m.health.LastCheckTime = time.Now()
				m.health.LastCheckHealthy = healthy

				if healthy {
					m.health.HealthyChecks++
					m.health.ConsecutiveFailures = 0
				} else {
					m.health.UnhealthyChecks++
					m.health.ConsecutiveFailures++
				}
				return nil
			},
		)
	})
}

func (m *DefaultMetricsCollector) GetMetrics() MetricsSnapshot {
	var snapshot MetricsSnapshot
	_ = concurrency.RunInRLockWithLogger(
		&m.mu, LockNameMetricsCollectorGetMetrics, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			operations := make(map[string]OperationMetrics)
			for k, v := range m.operations {
				operations[k] = *v
			}

			retries := make(map[string]RetryMetrics)
			for k, v := range m.retries {
				retries[k] = *v
			}

			queries := make(map[string]QueryMetrics)
			for k, v := range m.queries {
				queries[k] = *v
			}

			var totalOps int64
			for _, op := range m.operations {
				totalOps += op.Count
			}

			var totalRetries int64
			for _, retry := range m.retries {
				totalRetries += retry.TotalRetries
			}

			snapshot = MetricsSnapshot{
				Timestamp:       time.Now(),
				Operations:      operations,
				TotalOperations: totalOps,
				Retries:         retries,
				TotalRetries:    totalRetries,
				Connections:     m.connections,
				Transactions:    m.transactions,
				Pool:            m.pool,
				Queries:         queries,
				Batch:           m.batch,
				Health:          m.health,
			}
			return nil
		},
	)
	return snapshot
}

// Reset clears all metrics (useful for testing or periodic resets)
func (m *DefaultMetricsCollector) Reset() {
	_ = concurrency.RunInLockWithLogger(
		&m.mu, LockNameMetricsCollectorReset, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			m.operations = make(map[string]*OperationMetrics)
			m.retries = make(map[string]*RetryMetrics)
			m.queries = make(map[string]*QueryMetrics)
			m.connections = ConnectionMetrics{}
			m.transactions = TransactionMetrics{}
			m.pool = PoolMetrics{}
			m.health = HealthMetrics{}
			m.startTime = time.Now()
			return nil
		},
	)
}
