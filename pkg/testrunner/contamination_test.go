package testrunner_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/testrunner"
)

func TestContaminationSnapshotAndDiff(t *testing.T) {
	tempDir := t.TempDir()
	planeDir := filepath.Join(tempDir, ".zqk", "specs")
	if err := os.MkdirAll(planeDir, 0o755); err != nil {
		t.Fatalf("failed to create plane dir: %v", err)
	}

	f1 := filepath.Join(planeDir, "spec1.yaml")
	if err := os.WriteFile(f1, []byte("content: 1"), 0o644); err != nil {
		t.Fatalf("write f1: %v", err)
	}

	snap1, err := testrunner.SnapshotPlane(tempDir, ".zqk/specs")
	if err != nil {
		t.Fatalf("snapshot1: %v", err)
	}
	if len(snap1) != 1 {
		t.Errorf("expected 1 file in snap1, got %d", len(snap1))
	}

	// 1. Unchanged check
	diff0 := testrunner.DiffSnapshots(snap1, snap1)
	if diff0.HasContamination() {
		t.Errorf("expected no contamination on identical snapshot, got %v", diff0)
	}

	// 2. Mutate file
	if err := os.WriteFile(f1, []byte("content: 2"), 0o644); err != nil {
		t.Fatalf("mutate f1: %v", err)
	}
	// Add file
	f2 := filepath.Join(planeDir, "spec2.yaml")
	if err := os.WriteFile(f2, []byte("content: added"), 0o644); err != nil {
		t.Fatalf("write f2: %v", err)
	}

	snap2, err := testrunner.SnapshotPlane(tempDir, ".zqk/specs")
	if err != nil {
		t.Fatalf("snapshot2: %v", err)
	}

	diff1 := testrunner.DiffSnapshots(snap1, snap2)
	if !diff1.HasContamination() {
		t.Fatalf("expected contamination, got none")
	}
	if len(diff1.Modified) != 1 || diff1.Modified[0] != "spec1.yaml" {
		t.Errorf("expected spec1.yaml modified, got %v", diff1.Modified)
	}
	if len(diff1.Added) != 1 || diff1.Added[0] != "spec2.yaml" {
		t.Errorf("expected spec2.yaml added, got %v", diff1.Added)
	}
}

func TestRunWithContaminationCheck(t *testing.T) {
	tempDir := t.TempDir()
	planeDir := filepath.Join(tempDir, ".zqk", "specs")
	if err := os.MkdirAll(planeDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f1 := filepath.Join(planeDir, "spec1.yaml")
	if err := os.WriteFile(f1, []byte("name: initial"), 0o644); err != nil {
		t.Fatalf("write f1: %v", err)
	}

	opts := testrunner.ContaminationCheckOptions{
		ProjectRoot: tempDir,
		PlanePath:   ".zqk/specs",
	}

	// Non-mutating command (e.g. echo)
	exitCode, diff, err := testrunner.RunWithContaminationCheck(context.Background(), opts, []string{"echo", "clean"}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exitCode 0, got %d", exitCode)
	}
	if diff.HasContamination() {
		t.Errorf("expected no contamination, got %v", diff.AllChanged())
	}
}
