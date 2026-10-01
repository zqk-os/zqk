package asynccheck

import (
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
)

const (
	ProgressStageLoading         = "loading"
	ProgressEventTypeCompleted   = "completed"
	ProgressEventTypeTimeout     = "timeout"
	ProgressEventTypeStuck       = "stuck"
	ProgressEventTypeSummary     = "progress_summary"
	ProgressEventTypeProgress    = "progress"

	SeverityLow      = "low"
	SeverityMedium   = "medium"
	SeverityHigh     = "high"
	SeverityCritical = "critical"

	OperationTypeAsyncRouter     = "async_router"
	OperationTypeAsyncValidation = "async_validation"
	SourceBackgroundWorker       = "background_worker"

	DefaultMaxWorkers = 16
)

// ProgressStats holds current snapshot metrics for async check execution.
type ProgressStats struct {
	TotalTasks      int           `json:"total_tasks"`
	CompletedTasks  int           `json:"completed_tasks"`
	FailedTasks     int           `json:"failed_tasks"`
	QueueSize       int           `json:"queue_size"`
	PercentComplete float64       `json:"percent_complete"`
	Phase           string        `json:"phase"`
	ActiveWorkers   int           `json:"active_workers"`
	ActiveGoroutines int          `json:"active_goroutines"`
	Elapsed         time.Duration `json:"elapsed"`
	RatePerSecond   float64       `json:"rate_per_second"`
}

// EngineOptions configures the decoupled async system check runner.
type EngineOptions struct {
	ProjectRoot     string
	Profile         string
	OperationID     string
	TargetKind      string
	TargetIDs       []string
	MaxWorkers      int
	Timeout         time.Duration
	StorageProvider storage.ObjectStorageProvider
	AsyncValidator  *validation.AsyncValidator
	Metrics         *validation.ValidationMetrics
	Logger          logging.Logger
}
