package audit

import (
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
)

const MonthLayout = "2006-01"
const SummaryYAMLSuffix = ".yaml"

// MonthlyDir is the bucket directory for audit_event files (.zqk/process/audit/YYYY-MM).
func MonthlyDir(projectRoot string, now time.Time) string {
	return filepath.Join(projectRoot, paths.ProcessAuditDir, now.UTC().Format(MonthLayout))
}

// SummaryFilePath is dir/AUD-n.yaml for a flush persist.
func SummaryFilePath(dir, id string) string {
	return filepath.Join(dir, id+SummaryYAMLSuffix)
}
