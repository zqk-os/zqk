package testjobgen

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testscan"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// writeGuardScript creates a stand-in guard at root/scripts and returns its path.
func writeGuardScript(t *testing.T, root string) string {
	t.Helper()
	dir := filepath.Join(root, paths.ScriptsDir)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	guard := filepath.Join(dir, "check-test-contamination.sh")
	// Invoked as `sh <path>`, so the executable bit is not required.
	if err := fileutil.WriteFile(guard, []byte("#!/bin/sh\nexec \"$@\"\n"), paths.FilePerm644); err != nil {
		t.Fatalf("write %s: %v", guard, err)
	}
	return guard
}

func TestContaminationGuardPath(t *testing.T) {
	t.Run("returns the guard when present", func(t *testing.T) {
		root := t.TempDir()
		want := writeGuardScript(t, root)
		if got := contaminationGuardPath(root); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	// A tree without the script must still run its bundles rather than emit a
	// command that fails on a missing file.
	t.Run("returns empty when absent", func(t *testing.T) {
		if got := contaminationGuardPath(t.TempDir()); got != emptyValue {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("returns empty for an empty project root", func(t *testing.T) {
		if got := contaminationGuardPath(emptyValue); got != emptyValue {
			t.Errorf("got %q, want empty", got)
		}
	})

	// A directory of that name is not runnable.
	t.Run("returns empty when the path is a directory", func(t *testing.T) {
		root := t.TempDir()
		asDir := filepath.Join(root, paths.ScriptsDir, "check-test-contamination.sh")
		if err := fileutil.MkdirAll(asDir, paths.DirPerm755); err != nil {
			t.Fatalf("mkdir %s: %v", asDir, err)
		}
		if got := contaminationGuardPath(root); got != emptyValue {
			t.Errorf("got %q, want empty", got)
		}
	})
}

// TestBuildTestArgsWrapsWithContaminationGuard pins that bundles actually run under
// the guard. Without this the schema plane is unobserved during test runs, which is
// how fixture specs replaced base_object, auditable, backlog_item and requirement.
// TRACK: PRI-STABILIZE-FAILCLOSED-READS-001
func TestBuildTestArgsWrapsWithContaminationGuard(t *testing.T) {
	bundle := &testscan.TestBundle{
		ID:                "bundle-guard",
		PackagePath:       "cmd/zqk/object",
		IsParallel:        false,
		EstimatedDuration: 30 * time.Second,
		Tests:             []*testscan.TestFunction{{Name: "TestBulkUpdateCmd", Package: "object"}},
	}

	var generator JobGenerator

	guarded := t.TempDir()
	guard := writeGuardScript(t, guarded)
	args := generator.buildTestArgs(bundle, guarded, filepath.Join(guarded, "b.log"))
	if len(args) < 2 || args[0] != "-c" {
		t.Fatalf("expected shell -c invocation, got %v", args)
	}
	cmd := args[1]
	wantPrefix := "sh " + guard + " go test"
	if !strings.HasPrefix(cmd, wantPrefix) {
		t.Errorf("command must run under the guard.\n got: %s\nwant prefix: %s", cmd, wantPrefix)
	}
	// The guard must not swallow the redirect that produces the bundle log.
	if !strings.Contains(cmd, "b.log 2>&1") {
		t.Errorf("expected output redirection to survive wrapping, got: %s", cmd)
	}

	bare := t.TempDir()
	bareArgs := generator.buildTestArgs(bundle, bare, filepath.Join(bare, "b.log"))
	if len(bareArgs) < 2 {
		t.Fatalf("expected shell invocation, got %v", bareArgs)
	}
	if !strings.HasPrefix(bareArgs[1], "go test") {
		t.Errorf("without the guard the command must start with go test, got: %s", bareArgs[1])
	}
}
