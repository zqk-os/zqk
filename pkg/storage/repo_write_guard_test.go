package storage_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestFailClosedGate_TestsMayNotWriteRepoProcessState verifies that tests cannot mutate
// repository-root process state (BLI-CEF-R13-NO-REPO-WRITE-GATE-001).
func TestFailClosedGate_TestsMayNotWriteRepoProcessState(t *testing.T) {
	home, err := fileutil.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	// Must not live under $TMPDIR or contain "-test-", or IsTestOrTempProjectRoot
	// treats it as an isolated test root and the gate no-ops.
	probe := filepath.Join(home, "Library", "Caches", "zqk-r13-repo-write-guard-probe")
	if err := fileutil.EnsureDir(filepath.Join(probe, "docs", "process")); err != nil {
		t.Fatalf("EnsureDir probe: %v", err)
	}
	t.Cleanup(func() { _ = fileutil.RemoveAll(probe) })

	sp, err := storage.NewFileObjectStorage(probe)
	if err != nil {
		t.Fatalf("NewFileObjectStorage probe: %v", err)
	}
	err = sp.CheckTestRepoWriteGuard()
	if err == nil {
		t.Fatal("expected Create-equivalent guard to fail closed on a non-temp repo root")
	}
	if !strings.Contains(err.Error(), "fail-closed gate: tests may not write to repository root process state") {
		t.Fatalf("expected fail-closed gate error message, got: %v", err)
	}

	tempRoot := t.TempDir()
	spTemp, err := storage.NewFileObjectStorageForTest(tempRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}
	if guardErr := spTemp.CheckTestRepoWriteGuard(); guardErr != nil {
		t.Fatalf("CheckTestRepoWriteGuard should pass for temp directory, got: %v", guardErr)
	}
}
