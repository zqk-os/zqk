package snapshot

import (
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/systemcheck"
)

// CheckDiff represents the difference between two check outputs.
type CheckDiff struct {
	BaselineCount int                           `json:"baseline_count"`
	ExpandedCount int                           `json:"expanded_count"`
	Added         []systemcheck.CheckResult     `json:"added"`
	Removed       []systemcheck.CheckResult     `json:"removed"`
	Modified      []systemcheck.CheckResultDiff `json:"modified"`
}

// SnapshotSaveContext groups state for saving check snapshots.
type SnapshotSaveContext struct {
	Results      []systemcheck.CheckResult
	SnapshotPath string
	ProjectRoot  string
	Command      string
	Logger       logging.Logger
}

// VerificationStats tracks verification statistics for compressed snapshots.
type VerificationStats struct {
	Verified   int
	Mismatches int
	NotFound   int
}
