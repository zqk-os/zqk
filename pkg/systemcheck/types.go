package systemcheck

import "time"

// CheckResult represents the result of checking an object.
type CheckResult struct {
	ObjectID   string   `json:"object_id"`
	ObjectKind string   `json:"object_kind"`
	FilePath   string   `json:"file_path"`
	Status     string   `json:"status,omitempty"`
	Issues     []Issue  `json:"issues"`
	AutoFixed  []string `json:"auto_fixed,omitempty"`
}

// Issue represents a validation issue found during checking.
type Issue struct {
	Tier        int    `json:"tier"`     // 1: blocking, 2: warning, 3: informational, 4: recommendation
	Category    string `json:"category"` // registration, lifecycle, policy, integrity, reference
	Message     string `json:"message"`
	AutoFixable bool   `json:"auto_fixable"`
	FixCommand  string `json:"fix_command,omitempty"` // Command to fix this issue (generated at error time)
	// IssueClass is the fitness plane class (process_failure, data_completeness, …).
	// Omitted when empty; populated for surface-filtered projections.
	IssueClass string `json:"issue_class,omitempty"`
}

// CheckSnapshot represents a snapshot of check results for analysis.
type CheckSnapshot struct {
	Metadata CheckSnapshotMetadata `json:"metadata"`
	Results  []CheckResult         `json:"results"`
}

// CheckSnapshotMetadata contains metadata about the snapshot.
type CheckSnapshotMetadata struct {
	Timestamp     time.Time `json:"timestamp"`
	ProjectRoot   string    `json:"project_root"`
	TotalObjects  int       `json:"total_objects"`
	TotalIssues   int       `json:"total_issues"`
	BlockingCount int       `json:"blocking_count"`
	WarningCount  int       `json:"warning_count"`
	InfoCount     int       `json:"info_count"`
	RecCount      int       `json:"rec_count"`
	Command       string    `json:"command"` // Original command that generated this snapshot
}

// ResolutionResult represents the result of resolving a violation.
type ResolutionResult struct {
	Resolved     bool     `json:"resolved"`
	Resolution   string   `json:"resolution,omitempty"`  // How it was resolved
	Suggestions  []string `json:"suggestions,omitempty"` // Suggested fixes if not resolved
	Confidence   float64  `json:"confidence,omitempty"`  // Confidence level (0.0-1.0)
	AutoLinkable bool     `json:"auto_linkable"`         // Whether it can be auto-linked
}

// HashMismatchInfo tracks hash mismatch information for batch processing.
type HashMismatchInfo struct {
	ObjectID     string `json:"object_id"`
	Kind         string `json:"kind"`
	FilePath     string `json:"file_path"`
	OriginalHash string `json:"original_hash"`
}

// CheckResultDiff represents a modified check result between snapshot baselines.
type CheckResultDiff struct {
	ObjectID string      `json:"object_id"`
	Baseline CheckResult `json:"baseline"`
	Expanded CheckResult `json:"expanded"`
}
