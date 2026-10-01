package congruence

import (
	"time"

	"github.com/zqk-os/zqk/pkg/operational"
)

const (
	ReportExtJSON = ".json"
	ReportExtTXT  = ".txt"
)

const (
	OCRKeyTotalDisk         = "total_disk"
	OCRKeyTotalObject       = "total_object"
	OCRKeyAlertCount        = "alert_count"
	OCRKeyDirsWithDisparity = "dirs_with_disparity"
	OCRSeverityWarning      = "warning"
)

const (
	StreamDirAudit        = "audit"
	StreamDirMCPSessions  = "mcp_sessions"
	OCRProcessInternalDir = "_internal"
)

const (
	DefaultMetricsChunkRetentionDays = 14
	MaxMetricsChunkRetentionDays     = 365
	ReportsIndexMaxEntries           = 100
	HighCountThreshold               = 10000
	LegacyAdvisoryThreshold          = 100
	IntegrityTimeout                 = 3 * time.Second
)

// CacheStatus holds object ID cache and reverse reference index status for the report.
type CacheStatus struct {
	ObjectIDCache struct {
		Path        string         `json:"path"`
		Loaded      bool           `json:"loaded"`
		Total       int            `json:"total,omitempty"`
		CountByKind map[string]int `json:"count_by_kind,omitempty"`
	} `json:"object_id_cache"`
	ReverseReferenceIndex struct {
		Path              string `json:"path"`
		Loaded            bool   `json:"loaded"`
		ReferencedIDCount int    `json:"referenced_id_count,omitempty"`
	} `json:"reverse_reference_index"`
}

// ProcessIntegrity holds misplaced files, unmanaged files (not in object ID cache), and trash (unknown dirs, files at root).
type ProcessIntegrity struct {
	MisplacedByKind map[string][]string `json:"misplaced_by_kind,omitempty"` // kind -> paths in wrong dir
	UnmanagedFiles  []string            `json:"unmanaged_files,omitempty"`   // YAML on disk not in cache (not managed by CAS/index)
	UnknownDirs     []string            `json:"unknown_dirs,omitempty"`      // dirs under .zqk/process that don't map to a kind
	FilesAtRoot     []string            `json:"files_at_root,omitempty"`     // files directly under .zqk/process (scattered trash)
}

// ObjectCountReportEntry is one run's metadata for the reports index (discovery and aggregation).
type ObjectCountReportEntry struct {
	Path        string `json:"path"`         // basename of the report JSON file
	GeneratedAt string `json:"generated_at"` // RFC3339
	TotalDisk   int    `json:"total_disk"`
	TotalObject int    `json:"total_object"`
}

// ObjectCountReportsIndex is the durable index of object-count-report runs for dashboards and aggregation.
type ObjectCountReportsIndex struct {
	Latest  *ObjectCountReportEntry  `json:"latest,omitempty"`
	Reports []ObjectCountReportEntry `json:"reports,omitempty"` // newest first, capped at ReportsIndexMaxEntries
}

// ObjectCountDashboardSnapshot is the dashboard-friendly subset of a congruence report (single file for polling/aggregation).
type ObjectCountDashboardSnapshot struct {
	GeneratedAt        string         `json:"generated_at"`
	ProjectRoot        string         `json:"project_root,omitempty"`
	ObjectCountByKind  map[string]int `json:"object_count_by_kind"`
	TotalDisk          int            `json:"total_disk"`
	TotalObject        int            `json:"total_object"`
	TotalInternal      int            `json:"total_internal,omitempty"`
	Alerts             []string       `json:"alerts,omitempty"`
	KindsWithDisparity []string       `json:"kinds_with_disparity,omitempty"`
	// Set when object-count-report runs with --include-filesystem-snapshot.
	FilesystemSnapshotTotalFiles *int64 `json:"filesystem_snapshot_total_files,omitempty"`
	FilesystemSnapshotZqkFiles   *int64 `json:"filesystem_snapshot_zqk_files,omitempty"`
}

// Options configures the operational congruence execution.
type Options struct {
	OutputPath                 string
	EmitEvents                 bool
	IncludeInternal            bool
	DisparityThreshold         int
	NoCache                    bool
	IncludeFilesystemSnapshot  bool
	FilesystemSnapshotScopeStr string
	MetricsChunkRetentionDays  int
	UserProvidedReportFile     bool
}

// Result captures the artifacts and status produced by a congruence run.
type Result struct {
	Report                    *operational.CongruenceReport
	CacheStatus               CacheStatus
	Integrity                 ProcessIntegrity
	FilesystemProjectSnapshot *operational.FilesystemProjectSnapshot
	TextReportPath            string
	JSONReportPath            string
}
