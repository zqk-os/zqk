package kernelcas

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestMutatorPackagesReferenceAllowlistedKinds is a lightweight coverage gate:
// storage + system check/state-restore sources must mention at least one kernel.cas_* kind
// (or the kernelcas package import path) so mutators stay on the pipeline allowlist.
// TRACK: REDACTED
func TestMutatorPackagesReferenceAllowlistedKinds(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	paths := []string{
		filepath.Join(repoRoot, "pkg", "storage", "object_storage_file_delete.go"),
		filepath.Join(repoRoot, "pkg", "storage", "object_storage_file_create.go"),
		filepath.Join(repoRoot, "pkg", "storage", "object_storage_file_update.go"),
		filepath.Join(repoRoot, "pkg", "storage", "object_storage_file_transaction.go"),
		filepath.Join(repoRoot, "pkg", "storage", "object_storage_graph_crud.go"),
		filepath.Join(repoRoot, "cmd", "zqk", "system", "state_restore.go"),
		filepath.Join(repoRoot, "cmd", "zqk", "system", "check_orphaned_files.go"),
		filepath.Join(repoRoot, "cmd", "zqk", "system", "kernel_integrity.go"),
	}
	allow := AllKinds()
	for _, p := range paths {
		b, err := fileutil.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		s := string(b)
		if !strings.Contains(s, "kernelcas") && !strings.Contains(s, "kernel.cas_") &&
			!strings.Contains(s, "denyCoreKernelHardDelete") {
			t.Errorf("%s: expected kernelcas / kernel.cas_* / denyCoreKernelHardDelete reference", p)
			continue
		}
		foundKind := false
		for _, k := range allow {
			if strings.Contains(s, k) || strings.Contains(s, "RunErase") || strings.Contains(s, "RunCreate") ||
				strings.Contains(s, "RunUpdate") || strings.Contains(s, "RunTransition") ||
				strings.Contains(s, "RunRestoreMerge") || strings.Contains(s, "RunReconcileIndex") ||
				strings.Contains(s, "AllKinds") || strings.Contains(s, "denyCoreKernelHardDelete") {
				foundKind = true
				break
			}
		}
		if !foundKind {
			t.Errorf("%s: no allowlisted kernelcas Run*/AllKinds reference", p)
		}
	}
}

func TestAllKindsClosedSet(t *testing.T) {
	got := AllKinds()
	if len(got) != 7 {
		t.Fatalf("AllKinds len=%d want 7", len(got))
	}
	seen := map[string]bool{}
	for _, k := range got {
		if !strings.HasPrefix(k, "kernel.") {
			t.Errorf("kind %q missing kernel. prefix", k)
		}
		if seen[k] {
			t.Errorf("duplicate kind %q", k)
		}
		seen[k] = true
	}
}
