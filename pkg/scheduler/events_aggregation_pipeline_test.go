package scheduler

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestRunAggregationViaPipeline_EmptyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	eventsPath := filepath.Join(dir, "diagnostics.jsonl")
	summaryPath := filepath.Join(dir, "scheduler-metrics-summary.json")
	// Empty events file (create so INGEST sees it)
	if err := fileutil.WriteSecureFile(eventsPath, []byte("")); err != nil {
		t.Fatal(err)
	}
	logger := logging.GetLoggerFromProfile("test")
	ctx := context.Background()

	summary, linesRead, err := RunAggregationViaPipeline(ctx, dir, eventsPath, summaryPath, logger)
	if err != nil {
		t.Fatalf("RunAggregationViaPipeline: %v", err)
	}
	if linesRead != 0 {
		t.Errorf("expected linesRead 0, got %d", linesRead)
	}
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if summary.JobStats == nil {
		t.Error("expected JobStats non-nil")
	}
	if _, err := fileutil.Stat(summaryPath); err != nil {
		t.Errorf("summary file not written: %v", err)
	}
}

func TestRunAggregationViaPipeline_NoEventsFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	eventsPath := filepath.Join(dir, "diagnostics.jsonl")
	summaryPath := filepath.Join(dir, "scheduler-metrics-summary.json")
	// Do not create events file (missing is allowed, returns empty summary)
	logger := logging.GetLoggerFromProfile("test")
	ctx := context.Background()

	summary, linesRead, err := RunAggregationViaPipeline(ctx, dir, eventsPath, summaryPath, logger)
	if err != nil {
		t.Fatalf("RunAggregationViaPipeline: %v", err)
	}
	if linesRead != 0 {
		t.Errorf("expected linesRead 0 when file missing, got %d", linesRead)
	}
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if _, err := fileutil.Stat(summaryPath); err != nil {
		t.Errorf("summary file not written: %v", err)
	}
}
