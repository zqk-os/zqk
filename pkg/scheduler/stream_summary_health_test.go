package scheduler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestWriteTestBundleHealthStreamSummary(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p := TestBundlesHealthFilePath(root)
	if err := fileutil.EnsureDir(filepath.Dir(p)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(p, []byte("{\"x\":1}\n{\"x\":2}\n")); err != nil {
		t.Fatal(err)
	}
	sum, err := WriteTestBundleHealthStreamSummary(root)
	if err != nil {
		t.Fatalf("WriteTestBundleHealthStreamSummary: %v", err)
	}
	if sum.LineCount != 2 {
		t.Fatalf("LineCount: got %d want 2", sum.LineCount)
	}
	if sum.PathAlias != StreamSummaryTestBundleHealthAlias {
		t.Fatalf("PathAlias: %s", sum.PathAlias)
	}
	dest := filepath.Join(root, paths.ProjectDataDir, "stream_summary", "test_bundle_health.json")
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("expected written summary: %v", err)
	}
}
