package scheduler

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestResolveTestBundleHealthLinesForTick_MissingSkip(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	h := &ConvergenceSessionTickHandler{projectRoot: root, logger: nil}
	job := &ScheduledJob{
		ID: "SCH-tick",
		EnvironmentVariables: map[string]string{
			EnvKeyHealthFileMissing: healthFileMissingModeSkip,
		},
	}
	lines, stop, err := h.resolveTestBundleHealthLinesForTick(context.Background(), job, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !stop {
		t.Fatal("expected stop")
	}
	if lines != nil {
		t.Fatal("expected nil lines")
	}
}

func TestResolveTestBundleHealthLinesForTick_MissingFailDefault(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	h := &ConvergenceSessionTickHandler{projectRoot: root}
	job := &ScheduledJob{ID: "SCH-tick", EnvironmentVariables: map[string]string{}}
	_, _, err := h.resolveTestBundleHealthLinesForTick(context.Background(), job, 100)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveTestBundleHealthLinesForTick_MissingSkipBudget(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	h := &ConvergenceSessionTickHandler{projectRoot: root}
	job := &ScheduledJob{
		ID: "SCH-budget",
		EnvironmentVariables: map[string]string{
			EnvKeyHealthFileMissing:           healthFileMissingModeSkip,
			EnvKeyHealthFileMissingSkipBudget: "2",
		},
	}
	for i := 0; i < 2; i++ {
		_, stop, err := h.resolveTestBundleHealthLinesForTick(context.Background(), job, 100)
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		if !stop {
			t.Fatalf("iter %d: expected stop", i)
		}
	}
	_, _, err := h.resolveTestBundleHealthLinesForTick(context.Background(), job, 100)
	if err == nil {
		t.Fatal("expected budget error")
	}
}

func TestResolveTestBundleHealthLinesForTick_ResetsStreakWhenHealthExists(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	logDir := JobLogsTestBundlesDir(root)
	if err := fileutil.EnsureDir(logDir); err != nil {
		t.Fatal(err)
	}
	healthPath := filepath.Join(logDir, "health.jsonl")
	if err := fileutil.WriteSecureFile(healthPath, []byte(`{"ts":"2026-01-01T00:00:00Z"}`+"\n")); err != nil {
		t.Fatal(err)
	}
	h := &ConvergenceSessionTickHandler{projectRoot: root}
	job := &ScheduledJob{ID: "SCH-reset", EnvironmentVariables: map[string]string{EnvKeyHealthFileMissing: healthFileMissingModeSkip}}
	_, _, err := h.resolveTestBundleHealthLinesForTick(context.Background(), job, 100)
	if err != nil {
		t.Fatal(err)
	}
	_ = fileutil.Remove(healthPath)
	for i := 0; i < 3; i++ {
		_, stop, err := h.resolveTestBundleHealthLinesForTick(context.Background(), job, 100)
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		if !stop {
			t.Fatalf("iter %d: expected skip", i)
		}
	}
}
