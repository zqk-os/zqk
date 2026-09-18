package provider

import (
	"encoding/json"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const emptyValue = ""

// metricsSnapshotPersist is JSON-serializable (no error interface fields).
type metricsSnapshotPersist struct {
	Timestamp       time.Time                          `json:"timestamp"`
	Operations      map[string]operationMetricsPersist `json:"operations"`
	TotalOperations int64                              `json:"total_operations"`
	Retries         map[string]retryMetricsPersist     `json:"retries"`
	TotalRetries    int64                              `json:"total_retries"`
	Connections     connectionMetricsPersist           `json:"connections"`
	Transactions    transactionMetricsPersist          `json:"transactions"`
	Pool            poolMetricsPersist                 `json:"pool"`
	Queries         map[string]queryMetricsPersist     `json:"queries"`
	Health          healthMetricsPersist               `json:"health"`
}

type operationMetricsPersist struct {
	Operation     string        `json:"operation"`
	Count         int64         `json:"count"`
	SuccessCount  int64         `json:"success_count"`
	FailureCount  int64         `json:"failure_count"`
	TotalDuration time.Duration `json:"total_duration"`
	MinDuration   time.Duration `json:"min_duration"`
	MaxDuration   time.Duration `json:"max_duration"`
	AvgDuration   time.Duration `json:"avg_duration"`
	LastError     string        `json:"last_error,omitempty"`
	LastErrorTime time.Time     `json:"last_error_time,omitempty"`
	ErrorRate     float64       `json:"error_rate"`
}

type retryMetricsPersist struct {
	Operation         string  `json:"operation"`
	TotalRetries      int64   `json:"total_retries"`
	SuccessfulRetries int64   `json:"successful_retries"`
	FailedRetries     int64   `json:"failed_retries"`
	AvgAttempts       float64 `json:"avg_attempts"`
	MaxAttempts       int     `json:"max_attempts"`
}

type connectionMetricsPersist struct {
	TotalAcquired     int64         `json:"total_acquired"`
	TotalReleased     int64         `json:"total_released"`
	AcquireDuration   time.Duration `json:"acquire_duration"`
	AvgAcquireTime    time.Duration `json:"avg_acquire_time"`
	AcquireErrors     int64         `json:"acquire_errors"`
	ReleaseErrors     int64         `json:"release_errors"`
	ActiveConnections int           `json:"active_connections"`
}

type transactionMetricsPersist struct {
	TotalStarted      int64         `json:"total_started"`
	TotalCommitted    int64         `json:"total_committed"`
	TotalRolledBack   int64         `json:"total_rolled_back"`
	CommitDuration    time.Duration `json:"commit_duration"`
	RollbackDuration  time.Duration `json:"rollback_duration"`
	AvgCommitTime     time.Duration `json:"avg_commit_time"`
	AvgRollbackTime   time.Duration `json:"avg_rollback_time"`
	TransactionErrors int64         `json:"transaction_errors"`
	OpenTransactions  int           `json:"open_transactions"`
}

type poolMetricsPersist struct {
	TotalWaits      int64         `json:"total_waits"`
	TotalWaitTime   time.Duration `json:"total_wait_time"`
	AvgWaitTime     time.Duration `json:"avg_wait_time"`
	MaxWaitTime     time.Duration `json:"max_wait_time"`
	PoolSizeChanges int64         `json:"pool_size_changes"`
	CurrentActive   int           `json:"current_active"`
	CurrentIdle     int           `json:"current_idle"`
	MaxSize         int           `json:"max_size"`
	UtilizationRate float64       `json:"utilization_rate"`
}

type queryMetricsPersist struct {
	Query         string        `json:"query"`
	Count         int64         `json:"count"`
	SuccessCount  int64         `json:"success_count"`
	FailureCount  int64         `json:"failure_count"`
	TotalDuration time.Duration `json:"total_duration"`
	AvgDuration   time.Duration `json:"avg_duration"`
	TotalRows     int64         `json:"total_rows"`
	AvgRows       float64       `json:"avg_rows"`
	ErrorRate     float64       `json:"error_rate"`
}

type healthMetricsPersist struct {
	TotalChecks         int64         `json:"total_checks"`
	HealthyChecks       int64         `json:"healthy_checks"`
	UnhealthyChecks     int64         `json:"unhealthy_checks"`
	AvgCheckTime        time.Duration `json:"avg_check_time"`
	LastCheckTime       time.Time     `json:"last_check_time"`
	LastCheckHealthy    bool          `json:"last_check_healthy"`
	ConsecutiveFailures int64         `json:"consecutive_failures"`
}

func errString(err error) string {
	if err == nil {
		return emptyValue
	}
	return err.Error()
}

func snapshotToPersist(s MetricsSnapshot) metricsSnapshotPersist {
	out := metricsSnapshotPersist{
		Timestamp:       s.Timestamp,
		TotalOperations: s.TotalOperations,
		TotalRetries:    s.TotalRetries,
		Operations:      make(map[string]operationMetricsPersist, len(s.Operations)),
		Retries:         make(map[string]retryMetricsPersist, len(s.Retries)),
		Queries:         make(map[string]queryMetricsPersist, len(s.Queries)),
	}
	for k, v := range s.Operations {
		om := operationMetricsPersist{
			Operation:     v.Operation,
			Count:         v.Count,
			SuccessCount:  v.SuccessCount,
			FailureCount:  v.FailureCount,
			TotalDuration: v.TotalDuration,
			MinDuration:   v.MinDuration,
			MaxDuration:   v.MaxDuration,
			AvgDuration:   v.AvgDuration,
			LastError:     errString(v.LastError),
			LastErrorTime: v.LastErrorTime,
			ErrorRate:     v.ErrorRate,
		}
		out.Operations[k] = om
	}
	for k, v := range s.Retries {
		out.Retries[k] = retryMetricsPersist(v)
	}
	for k, v := range s.Queries {
		out.Queries[k] = queryMetricsPersist(v)
	}
	out.Connections = connectionMetricsPersist{
		TotalAcquired:     s.Connections.TotalAcquired,
		TotalReleased:     s.Connections.TotalReleased,
		AcquireDuration:   s.Connections.AcquireDuration,
		AvgAcquireTime:    s.Connections.AvgAcquireTime,
		AcquireErrors:     s.Connections.AcquireErrors,
		ReleaseErrors:     s.Connections.ReleaseErrors,
		ActiveConnections: s.Connections.ActiveConnections,
	}
	out.Transactions = transactionMetricsPersist{
		TotalStarted:      s.Transactions.TotalStarted,
		TotalCommitted:    s.Transactions.TotalCommitted,
		TotalRolledBack:   s.Transactions.TotalRolledBack,
		CommitDuration:    s.Transactions.CommitDuration,
		RollbackDuration:  s.Transactions.RollbackDuration,
		AvgCommitTime:     s.Transactions.AvgCommitTime,
		AvgRollbackTime:   s.Transactions.AvgRollbackTime,
		TransactionErrors: s.Transactions.TransactionErrors,
		OpenTransactions:  s.Transactions.OpenTransactions,
	}
	out.Pool = poolMetricsPersist{
		TotalWaits:      s.Pool.TotalWaits,
		TotalWaitTime:   s.Pool.TotalWaitTime,
		AvgWaitTime:     s.Pool.AvgWaitTime,
		MaxWaitTime:     s.Pool.MaxWaitTime,
		PoolSizeChanges: s.Pool.PoolSizeChanges,
		CurrentActive:   s.Pool.CurrentActive,
		CurrentIdle:     s.Pool.CurrentIdle,
		MaxSize:         s.Pool.MaxSize,
		UtilizationRate: s.Pool.UtilizationRate,
	}
	out.Health = healthMetricsPersist{
		TotalChecks:         s.Health.TotalChecks,
		HealthyChecks:       s.Health.HealthyChecks,
		UnhealthyChecks:     s.Health.UnhealthyChecks,
		AvgCheckTime:        s.Health.AvgCheckTime,
		LastCheckTime:       s.Health.LastCheckTime,
		LastCheckHealthy:    s.Health.LastCheckHealthy,
		ConsecutiveFailures: s.Health.ConsecutiveFailures,
	}
	return out
}

// EncodeMetricsSnapshotForPersistence JSON-encodes a metrics snapshot for base_metric storage
// (operation last_error as string; suitable for large snapshots up to caller caps).
func EncodeMetricsSnapshotForPersistence(s MetricsSnapshot) ([]byte, error) {
	p := snapshotToPersist(s)
	data, err := json.Marshal(p)
	if err != nil {
		return nil, errfmt.Newf("encode graph provider metrics snapshot").Wrap(err)
	}
	return data, nil
}
