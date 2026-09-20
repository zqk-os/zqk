package scheduler

import (
	"path/filepath"
	"testing"

	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestReadTestBundleHealthTailForAgentPrompt_MissingFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	lines, err := readTestBundleHealthTailForAgentPrompt(root, 50)
	if err != nil {
		t.Fatalf("missing health.jsonl: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected empty lines, got %d", len(lines))
	}
}

func TestReadTestBundleHealthTailForAgentPrompt_WithFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p := schedpkg.TestBundlesHealthFilePath(root)
	if err := fileutil.EnsureDir(filepath.Dir(p)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(p, []byte(`{"timestamp":"2026-04-06T00:00:00Z","bundle_command_fingerprint":"fp1","test_outcome":"pass"}`+"\n")); err != nil {
		t.Fatal(err)
	}
	lines, err := readTestBundleHealthTailForAgentPrompt(root, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
}
