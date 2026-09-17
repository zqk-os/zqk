//go:build !production
// +build !production

package storage

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// moduleRootFromGoEnv returns the directory containing go.mod for the current module (`go env GOMOD`).
// Duplicates pkg/testing.ModuleRootFromGoEnv so this file does not import pkg/testing (import-cycle hygiene).
func moduleRootFromGoEnv(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	modPath := strings.TrimSpace(string(out))
	if modPath == "" || modPath == "/dev/null" {
		t.Skip("no module root (GOMOD empty or not in module context)")
	}
	return filepath.Dir(modPath)
}

// moduleRootFromGoEnvTB is like [moduleRootFromGoEnv] but fails the benchmark on missing GOMOD (no Skip).
func moduleRootFromGoEnvTB(tb testing.TB) string {
	tb.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		tb.Fatalf("go env GOMOD: %v", err)
	}
	modPath := strings.TrimSpace(string(out))
	if modPath == "" || modPath == "/dev/null" {
		tb.Fatal("no module root (GOMOD empty or not in module context)")
	}
	return filepath.Dir(modPath)
}

// projectRootForSpecCopy returns the module root when it contains .zqk/specs/objects.
// Uses go env GOMOD instead of walking from os.Getwd so tests do not depend on cwd.
func projectRootForSpecCopy(t *testing.T) string {
	t.Helper()
	root := moduleRootFromGoEnv(t)
	specs := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
	if st, err := fileutil.Stat(specs); err != nil || !st.IsDir() {
		return ""
	}
	return root
}

// ProjectRootForSpecCopyForTest exposes projectRootForSpecCopy for storage_test package tests.
func ProjectRootForSpecCopyForTest(t *testing.T) string {
	return projectRootForSpecCopy(t)
}
