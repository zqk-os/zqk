package scheduler

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentprompt"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestGatherHealthSignals verifies that gatherHealthSignals reads failing test count
// from health.jsonl and returns a populated KernelHealthSignals struct.
func TestGatherHealthSignals(t *testing.T) {
	tempDir := t.TempDir()

	// Write a mock health.jsonl with some failing test entries
	healthDir := filepath.Join(tempDir, ".zqk", "scheduler")
	if err := fileutil.EnsureDir(healthDir); err != nil {
		t.Fatalf("failed to create health dir: %v", err)
	}
	healthLog := filepath.Join(healthDir, "health.jsonl")
	content := `{"level":"error","event":"test_failure","package":"pkg/foo","timestamp":"2026-07-03T10:00:00Z"}
{"level":"error","event":"test_failure","package":"pkg/bar","timestamp":"2026-07-03T10:01:00Z"}
{"level":"info","event":"test_pass","package":"pkg/baz","timestamp":"2026-07-03T10:02:00Z"}
`
	if err := fileutil.WriteStandardFile(healthLog, []byte(content)); err != nil {
		t.Fatalf("failed to write health.jsonl: %v", err)
	}

	signals := gatherHealthSignals(tempDir)

	if signals.FailingTestCount != 2 {
		t.Errorf("expected 2 failing tests, got %d", signals.FailingTestCount)
	}
}

// TestGatherHealthSignals_EmptyLog verifies that missing health.jsonl returns zero signals.
func TestGatherHealthSignals_EmptyLog(t *testing.T) {
	tempDir := t.TempDir()

	signals := gatherHealthSignals(tempDir)

	if signals.FailingTestCount != 0 {
		t.Errorf("expected 0 failing tests for empty log, got %d", signals.FailingTestCount)
	}
	if len(signals.DriftIndicators) != 0 {
		t.Errorf("expected no drift indicators for empty log, got %v", signals.DriftIndicators)
	}
}

// Compile-time check: gatherHealthSignals must return agentprompt.KernelHealthSignals
var _ agentprompt.KernelHealthSignals = agentprompt.KernelHealthSignals{}
