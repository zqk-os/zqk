package community

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestSeatedCommunityKernel(t *testing.T) {
	t.Parallel()
	empty := t.TempDir()
	if seatedCommunityKernel(empty) {
		t.Fatalf("empty dir must not look seated")
	}
	kernel := t.TempDir()
	if err := fileutil.MkdirAll(filepath.Join(kernel, ".zqk", "process"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !seatedCommunityKernel(kernel) {
		t.Fatalf("dir with .zqk/process must look seated")
	}
	envOnly := t.TempDir()
	if err := fileutil.WriteFile(filepath.Join(envOnly, ".env"), []byte("ZQK_PROJECT_ROOT=/tmp/example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !seatedCommunityKernel(envOnly) {
		t.Fatalf("dir with .env must look seated")
	}
}
