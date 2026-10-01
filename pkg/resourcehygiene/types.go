package resourcehygiene

import (
	"time"
)

// ChildFolderUsage reports files and bytes for an immediate subdirectory.
type ChildFolderUsage struct {
	Files int64 `json:"files"`
	Bytes int64 `json:"bytes"`
}

// IOResourceTelemetry aggregates I/O state, file descriptor metrics, and volume breakdown.
type IOResourceTelemetry struct {
	CapturedAt           string                      `json:"captured_at"`
	OpenFileDescriptors  int                         `json:"open_file_descriptors"`
	MaxFileDescriptors   int                         `json:"max_file_descriptors"`
	TotalZqkFiles        int64                       `json:"total_zqk_files"`
	TotalZqkBytes        int64                       `json:"total_zqk_bytes"`
	VolumeTotalBytes     uint64                      `json:"volume_total_bytes"`
	VolumeAvailableBytes uint64                      `json:"volume_available_bytes"`
	ZqkChildren          map[string]ChildFolderUsage `json:"zqk_children,omitempty"`
	StaleLocksCount       int                         `json:"stale_locks_count"`
	OrphanedTempCount     int                         `json:"orphaned_temp_count"`
	OrphanedProcessCount  int                         `json:"orphaned_process_count"`
	StaleLockPaths        []string                    `json:"stale_lock_paths,omitempty"`
	OrphanedTempPaths     []string                    `json:"orphaned_temp_paths,omitempty"`
	OrphanedProcesses     []string                    `json:"orphaned_processes,omitempty"`
}

// HygieneOptions configures automated cleanup execution.
type HygieneOptions struct {
	ReapLocks        bool          `json:"reap_locks"`
	ReapTemp         bool          `json:"reap_temp"`
	ReapProcesses    bool          `json:"reap_processes"`
	EnforceRetention bool          `json:"enforce_retention"`
	DryRun           bool          `json:"dry_run"`
	LockThreshold    time.Duration `json:"lock_threshold"`
	TempThreshold    time.Duration `json:"temp_threshold"`
	LogMaxAge        time.Duration `json:"log_max_age"`
	LogMaxSize       int64         `json:"log_max_size"`
}

// DefaultHygieneOptions returns sensible defaults for automated resource hygiene.
func DefaultHygieneOptions() HygieneOptions {
	return HygieneOptions{
		ReapLocks:        true,
		ReapTemp:         true,
		ReapProcesses:    true,
		EnforceRetention: true,
		DryRun:           false,
		LockThreshold:    15 * time.Minute,
		TempThreshold:    30 * time.Minute,
		LogMaxAge:        48 * time.Hour,
		LogMaxSize:       10 * 1024 * 1024, // 10MB
	}
}

// HygieneExecutionReport summarizes the results of a hygiene sweep.
type HygieneExecutionReport struct {
	Timestamp       string   `json:"timestamp"`
	DryRun          bool     `json:"dry_run"`
	LocksReaped     int      `json:"locks_reaped"`
	TempReaped      int      `json:"temp_reaped"`
	ProcessesReaped int      `json:"processes_reaped"`
	LogsPruned      int      `json:"logs_pruned"`
	BytesReclaimed  int64    `json:"bytes_reclaimed"`
	ReapedPaths     []string `json:"reaped_paths,omitempty"`
	Errors          []string `json:"errors,omitempty"`
}
