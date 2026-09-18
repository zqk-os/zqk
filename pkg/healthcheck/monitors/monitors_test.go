package monitors

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSchedulerEventsMonitor_Run(t *testing.T) {
	t.Parallel()

	m := NewSchedulerEventsMonitor()
	ctx := context.Background()

	t.Run("no_summary_file_returns_degraded", func(t *testing.T) {
		tmpDir := t.TempDir()
		res, err := m.Run(ctx, tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Status != "degraded" {
			t.Fatalf("expected status degraded, got %s", res.Status)
		}
	})

	t.Run("empty_jobs_returns_ok", func(t *testing.T) {
		tmpDir := t.TempDir()
		summaryDir := filepath.Dir(scheduler.SummaryPath(tmpDir))
		_ = fileutil.EnsureDir(summaryDir)

		summary := scheduler.SchedulerMetricsSummary{
			JobStats: map[string]scheduler.JobExecutionStats{},
		}
		data, _ := json.Marshal(summary)
		_ = fileutil.WriteStandardFile(scheduler.SummaryPath(tmpDir), data)

		res, err := m.Run(ctx, tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Status != "ok" {
			t.Fatalf("expected status ok, got %s", res.Status)
		}
	})

	t.Run("slow_job_returns_degraded", func(t *testing.T) {
		tmpDir := t.TempDir()
		summaryDir := filepath.Dir(scheduler.SummaryPath(tmpDir))
		_ = fileutil.EnsureDir(summaryDir)

		summary := scheduler.SchedulerMetricsSummary{
			JobStats: map[string]scheduler.JobExecutionStats{
				"slow_job": {
					Completed:        1,
					Failed:           0,
					TotalDurationSec: float64(scheduler.SlowJobThresholdSec + 5),
				},
			},
		}
		data, _ := json.Marshal(summary)
		_ = fileutil.WriteStandardFile(scheduler.SummaryPath(tmpDir), data)

		res, err := m.Run(ctx, tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Status != "degraded" {
			t.Fatalf("expected status degraded, got %s", res.Status)
		}
	})

	t.Run("failed_job_returns_fail", func(t *testing.T) {
		tmpDir := t.TempDir()
		summaryDir := filepath.Dir(scheduler.SummaryPath(tmpDir))
		_ = fileutil.EnsureDir(summaryDir)

		summary := scheduler.SchedulerMetricsSummary{
			JobStats: map[string]scheduler.JobExecutionStats{
				"failing_job": {
					Completed:        5,
					Failed:           1,
					TotalDurationSec: 2.0,
				},
			},
		}
		data, _ := json.Marshal(summary)
		_ = fileutil.WriteStandardFile(scheduler.SummaryPath(tmpDir), data)

		res, err := m.Run(ctx, tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Status != "fail" {
			t.Fatalf("expected status fail, got %s", res.Status)
		}
	})
}

func TestObjectVolumeMonitor_Run(t *testing.T) {
	t.Parallel()

	m := &objectVolumeMonitor{}
	ctx := context.Background()

	t.Run("empty_project_root_returns_ok", func(t *testing.T) {
		res, err := m.Run(ctx, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Status != "ok" {
			t.Fatalf("expected status ok, got %s", res.Status)
		}
	})

	t.Run("missing_metrics_dir_returns_ok", func(t *testing.T) {
		tmpDir := t.TempDir()
		res, err := m.Run(ctx, tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Status != "ok" {
			t.Fatalf("expected status ok, got %s", res.Status)
		}
	})
}

func TestStreamVolumeMonitor_Run(t *testing.T) {
	t.Parallel()

	m := &streamVolumeMonitor{}
	ctx := context.Background()

	t.Run("empty_project_root_returns_ok", func(t *testing.T) {
		res, err := m.Run(ctx, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Status != "ok" {
			t.Fatalf("expected status ok, got %s", res.Status)
		}
	})

	t.Run("missing_metrics_dir_returns_ok", func(t *testing.T) {
		tmpDir := t.TempDir()
		res, err := m.Run(ctx, tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Status != "ok" {
			t.Fatalf("expected status ok, got %s", res.Status)
		}
	})
}

func TestWalBacklogMonitor_Run(t *testing.T) {
	t.Parallel()

	m := &walBacklogMonitor{}
	ctx := context.Background()

	t.Run("empty_project_root_returns_ok", func(t *testing.T) {
		res, err := m.Run(ctx, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Status != "ok" {
			t.Fatalf("expected status ok, got %s", res.Status)
		}
	})

	t.Run("missing_wal_returns_ok", func(t *testing.T) {
		tmpDir := t.TempDir()
		res, err := m.Run(ctx, tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Status != "ok" {
			t.Fatalf("expected status ok, got %s", res.Status)
		}
	})
}

func TestAutonomyInboxMonitor_Run(t *testing.T) {
	t.Parallel()

	m := &autonomyInboxMonitor{}
	ctx := context.Background()

	t.Run("empty_project_root_returns_ok", func(t *testing.T) {
		res, err := m.Run(ctx, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Status != "ok" {
			t.Fatalf("expected status ok, got %s", res.Status)
		}
	})

	t.Run("empty_inbox_returns_ok_live", func(t *testing.T) {
		tmpDir := t.TempDir()
		res, err := m.Run(ctx, tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Status != "ok" {
			t.Fatalf("expected status ok, got %s", res.Status)
		}
		if syn, ok := res.Details["synthetic"].(bool); !ok || syn {
			t.Fatalf("expected synthetic=false, got %v", res.Details["synthetic"])
		}
		if avail, ok := res.Details["available"].(bool); !ok || !avail {
			t.Fatalf("expected available=true, got %v", res.Details["available"])
		}
	})

	t.Run("inbox_with_items_returns_count", func(t *testing.T) {
		tmpDir := t.TempDir()
		inboxDir := filepath.Join(tmpDir, ".zqk", "inbox")
		_ = fileutil.EnsureDir(inboxDir)
		_ = fileutil.WriteStandardFile(filepath.Join(inboxDir, "msg1.json"), []byte("{}"))
		_ = fileutil.WriteStandardFile(filepath.Join(inboxDir, "msg2.json"), []byte("{}"))

		res, err := m.Run(ctx, tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Status != "ok" {
			t.Fatalf("expected status ok, got %s", res.Status)
		}
		if count, ok := res.Details["pending_count"].(int); !ok || count != 2 {
			t.Fatalf("expected pending_count=2, got %v", res.Details["pending_count"])
		}
	})
}
