package audit

import (
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/kindnames"
)

const MonthLayout = "2006-01"
const SummaryYAMLSuffix = ".yaml"

// KindDir is the stream_current overlay directory for audit_event.
// Audit events are stream-backed; they are not CAS objects under .zqk/process/audit.
func KindDir(projectRoot string) string {
	return datacell.StreamCurrentKindDir(projectRoot, kindnames.AuditEvent)
}

// MonthlyDir is the ID-sequence bucket under the stream overlay (YYYY-MM).
func MonthlyDir(projectRoot string, now time.Time) string {
	return filepath.Join(KindDir(projectRoot), now.UTC().Format(MonthLayout))
}

// SummaryFilePath is dir/AUD-n.yaml for a flush persist.
func SummaryFilePath(dir, id string) string {
	return filepath.Join(dir, id+SummaryYAMLSuffix)
}
