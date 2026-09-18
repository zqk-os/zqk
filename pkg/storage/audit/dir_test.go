package audit

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
)

func TestMonthlyDir(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	got := MonthlyDir("/proj", now)
	want := filepath.Join("/proj", paths.ProcessAuditDir, "2026-09")
	if got != want {
		t.Fatalf("%q want %q", got, want)
	}
}

func TestSummaryFilePath(t *testing.T) {
	t.Parallel()
	auditDir := filepath.Join("/proj", paths.ProcessAuditDir, "2026-09")
	got := SummaryFilePath(auditDir, "AUD-1")
	want := filepath.Join(auditDir, "AUD-1.yaml")
	if got != want {
		t.Fatalf("%q want %q", got, want)
	}
}
