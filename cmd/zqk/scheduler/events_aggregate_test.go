package scheduler

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

func TestAggregateEventsFromFile_MissingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	summary, linesRead, err := schedpkg.AggregateEventsFromFile(path)
	if err != nil {
		t.Fatalf("AggregateEventsFromFile(missing) err = %v, want nil", err)
	}
	if linesRead != 0 {
		t.Errorf("linesRead = %d, want 0", linesRead)
	}
	if summary == nil || summary.JobStats == nil {
		t.Error("summary or JobStats should be non-nil (empty result)")
	}
	if len(summary.JobStats) != 0 {
		t.Errorf("JobStats length = %d, want 0", len(summary.JobStats))
	}
}

func TestAggregateEventsFromFile_EmptyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	if err := fileutil.WriteFile(path, []byte(""), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	summary, linesRead, err := schedpkg.AggregateEventsFromFile(path)
	if err != nil {
		t.Fatalf("AggregateEventsFromFile(empty) err = %v, want nil", err)
	}
	if linesRead != 0 {
		t.Errorf("linesRead = %d, want 0", linesRead)
	}
	if len(summary.JobStats) != 0 {
		t.Errorf("JobStats length = %d, want 0", len(summary.JobStats))
	}
}

func TestAggregateEventsFromFile_ValidJSONL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	// JSONL: one completed, one failed, one non-job event (ignored)
	content := `{"event_type":"scheduler_job_completed","job_id":"SCH-001","duration_seconds":1.5,"timestamp":"2026-02-21T12:00:00Z"}
{"event_type":"scheduler_job_failed","job_id":"SCH-002","timestamp":"2026-02-21T12:01:00Z"}
{"event_type":"scheduler_job_completed","job_id":"SCH-001","duration_seconds":2.0,"timestamp":"2026-02-21T12:02:00Z"}
{"event_type":"scheduler_health_monitoring","metric_id":"SHM-1"}
`
	if err := fileutil.WriteFile(path, []byte(content), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	summary, linesRead, err := schedpkg.AggregateEventsFromFile(path)
	if err != nil {
		t.Fatalf("AggregateEventsFromFile err = %v", err)
	}
	if linesRead != 4 {
		t.Errorf("linesRead = %d, want 4", linesRead)
	}

	// SCH-001: 2 completed, 0 failed, total duration 3.5
	st1, ok := summary.JobStats["SCH-001"]
	if !ok {
		t.Fatal("SCH-001 not in JobStats")
	}
	if st1.Completed != 2 || st1.Failed != 0 {
		t.Errorf("SCH-001: completed=%d failed=%d, want 2, 0", st1.Completed, st1.Failed)
	}
	if st1.TotalDurationSec != 3.5 {
		t.Errorf("SCH-001 TotalDurationSec = %v, want 3.5", st1.TotalDurationSec)
	}
	if st1.LastCompletedISO != "2026-02-21T12:02:00Z" {
		t.Errorf("SCH-001 LastCompletedISO = %q, want 2026-02-21T12:02:00Z", st1.LastCompletedISO)
	}

	// SCH-002: 0 completed, 1 failed
	st2, ok := summary.JobStats["SCH-002"]
	if !ok {
		t.Fatal("SCH-002 not in JobStats")
	}
	if st2.Completed != 0 || st2.Failed != 1 {
		t.Errorf("SCH-002: completed=%d failed=%d, want 0, 1", st2.Completed, st2.Failed)
	}
	if st2.LastFailedISO != "2026-02-21T12:01:00Z" {
		t.Errorf("SCH-002 LastFailedISO = %q", st2.LastFailedISO)
	}
}

func TestAggregateEventsFromFile_InvalidLineSkipped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	content := `{"event_type":"scheduler_job_completed","job_id":"SCH-001","duration_seconds":1.0}
not json
{"event_type":"scheduler_job_completed","job_id":"SCH-002","duration_seconds":2.0}
`
	if err := fileutil.WriteFile(path, []byte(content), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	summary, linesRead, err := schedpkg.AggregateEventsFromFile(path)
	if err != nil {
		t.Fatalf("AggregateEventsFromFile err = %v", err)
	}
	if linesRead != 3 {
		t.Errorf("linesRead = %d, want 3", linesRead)
	}
	if len(summary.JobStats) != 2 {
		t.Errorf("JobStats length = %d, want 2 (invalid line skipped)", len(summary.JobStats))
	}
}
