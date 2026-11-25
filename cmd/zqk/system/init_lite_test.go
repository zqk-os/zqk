package system

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestInitLite_ZeroBloatFootprintAndQuickstart(t *testing.T) {
	root := t.TempDir()
	// Seed a tiny source file so trigram index build exercises indexing.
	srcFile := filepath.Join(root, "app.ts")
	if err := fileutil.WriteFile(srcFile, []byte("export class PilotEngine { run() {} }\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed writing sample source file: %v", err)
	}

	t.Setenv("ZQK_TEST_ROOT", root)
	cmd := NewInitCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--lite"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("zqk init --lite failed: %v", err)
	}

	// Verify minimal .zqk/{cache,logs,state} directories exist.
	for _, rel := range []string{
		filepath.Join(paths.ProjectDataDir, paths.CacheDir),
		filepath.Join(paths.ProjectDataDir, paths.LogsDir),
		filepath.Join(paths.ProjectDataDir, paths.StateDir),
	} {
		full := filepath.Join(root, rel)
		if !fileutil.Exists(full) {
			t.Errorf("expected lite directory %s to exist", full)
		}
	}

	// Verify zero-bloat guarantee: .zqk/process MUST NOT be seeded.
	processDir := filepath.Join(root, paths.ProcessDir)
	if fileutil.Exists(processDir) {
		t.Errorf("expected zero-bloat lite init NOT to create %s", processDir)
	}

	// Verify quickstart output mentions ROI scorecard and expand-to-kernel bridge.
	out := buf.String()
	for _, want := range []string{
		"Initialized ZQK Lite (zgrep-first) workspace",
		"zqk grep --ast --kind struct",
		"zqk grep --roi",
		"zqk init --merge",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected lite init output to contain %q, got:\n%s", want, out)
		}
	}
}
