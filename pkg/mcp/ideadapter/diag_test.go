package ideadapter

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestSetDiagLogRoot_andDiagf(t *testing.T) {
	root := t.TempDir()
	SetDiagLogRoot(root)
	t.Cleanup(func() { SetDiagLogRoot("") })

	diagf("hello %s", "adapter")

	want := filepath.Join(paths.MCPDirPath(root), paths.MCPLogsDir, ideAdapterDiagFile)
	b, err := fileutil.ReadFile(want)
	if err != nil {
		t.Fatalf("read diag log: %v", err)
	}
	if !strings.Contains(string(b), "hello adapter") {
		t.Fatalf("diag log missing message: %q", b)
	}
}

func TestSetDiagLogRoot_emptyDisables(t *testing.T) {
	root := t.TempDir()
	SetDiagLogRoot(root)
	SetDiagLogRoot("")
	diagf("should-not-write")
	want := filepath.Join(paths.MCPDirPath(root), paths.MCPLogsDir, ideAdapterDiagFile)
	if _, err := fileutil.Stat(want); !fileutil.IsNotExist(err) {
		t.Fatalf("expected no diag file when root cleared, err=%v", err)
	}
}
