package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/pkg/logging"
)

func TestCleanupConfigHandler_Execute_emptyProjectRoot(t *testing.T) {
	h := NewCleanupConfigHandler("", logging.GetLoggerFromProfile("test"))
	err := h.Execute(context.Background(), &ScheduledJob{ID: "SCH-cleanup", JobType: JobTypeCleanup})
	if err != nil {
		t.Errorf("Execute with empty project root: %v", err)
	}
}

func TestCleanupConfigHandler_Execute_missingConfig(t *testing.T) {
	dir := t.TempDir()
	h := NewCleanupConfigHandler(dir, logging.GetLoggerFromProfile("test"))
	err := h.Execute(context.Background(), &ScheduledJob{ID: "SCH-cleanup", JobType: JobTypeCleanup})
	if err == nil {
		t.Fatal("expected error when config missing")
	}
	if !strings.Contains(err.Error(), "cleanup config not found") && !strings.Contains(err.Error(), "no such file") {
		t.Errorf("error should mention missing config: %v", err)
	}
}

func TestCleanupConfigHandler_Execute_emptySteps(t *testing.T) {
	dir := t.TempDir()
	cleanupDir := filepath.Join(dir, paths.ProjectDataDir, "cleanup")
	if err := os.MkdirAll(cleanupDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cleanupDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("steps: []"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	h := NewCleanupConfigHandler(dir, logging.GetLoggerFromProfile("test"))
	err := h.Execute(context.Background(), &ScheduledJob{ID: "SCH-cleanup", JobType: JobTypeCleanup})
	if err != nil {
		t.Errorf("Execute with empty steps: %v", err)
	}
}

func TestCleanupConfigHandler_Execute_truncateFiles(t *testing.T) {
	dir := t.TempDir()
	cleanupDir := filepath.Join(dir, paths.ProjectDataDir, "cleanup")
	if err := os.MkdirAll(cleanupDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "test.log")
	lines := make([]string, 20)
	for i := 0; i < 20; i++ {
		lines[i] = "line"
	}
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cleanupDir, "config.yaml")
	cfgData := []byte("steps:\n  - type: truncate_files\n    path: test.log\n    keep_last_lines: 10\n")
	if err := os.WriteFile(cfgPath, cfgData, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	h := NewCleanupConfigHandler(dir, logging.GetLoggerFromProfile("test"))
	err := h.Execute(context.Background(), &ScheduledJob{ID: "SCH-cleanup", JobType: JobTypeCleanup})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	count := len(strings.Split(strings.TrimSuffix(string(data), "\n"), "\n"))
	if count != 10 {
		t.Errorf("after truncate: got %d lines, want 10", count)
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"7d", 7 * 24 * time.Hour},
		{"1d", 24 * time.Hour},
		{"0d", 0},
	}
	for _, tt := range tests {
		got, err := parseDuration(tt.in)
		if err != nil {
			t.Errorf("parseDuration(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseDuration(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
