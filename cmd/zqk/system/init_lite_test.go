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

func TestInitLite_WithMCPFlag(t *testing.T) {
	root := t.TempDir()
	srcFile := filepath.Join(root, "service.py")
	if err := fileutil.WriteFile(srcFile, []byte("class BillingService:\n    def charge(self): pass\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed writing sample python file: %v", err)
	}

	t.Setenv("ZQK_TEST_ROOT", root)
	cmd := NewInitCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--lite", "--with-mcp"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("zqk init --lite --with-mcp failed: %v", err)
	}
	if fileutil.Exists(filepath.Join(root, paths.ProcessDir)) {
		t.Errorf("expected --lite --with-mcp NOT to seed %s", paths.ProcessDir)
	}
	if !strings.Contains(buf.String(), "Multi-host MCP server configuration wired") {
		t.Errorf("expected MCP wiring confirmation in output, got:\n%s", buf.String())
	}
}

func TestInitLite_WithHooksFlag(t *testing.T) {
	root := t.TempDir()
	srcFile := filepath.Join(root, "worker.rs")
	if err := fileutil.WriteFile(srcFile, []byte("pub struct Worker;\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed writing sample rust file: %v", err)
	}

	t.Setenv("ZQK_TEST_ROOT", root)
	cmd := NewInitCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--lite", "--with-hooks"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("zqk init --lite --with-hooks failed: %v", err)
	}
	if fileutil.Exists(filepath.Join(root, paths.ProcessDir)) {
		t.Errorf("expected --lite --with-hooks NOT to seed %s", paths.ProcessDir)
	}

	hooksPath := filepath.Join(root, ".agents", "hooks.json")
	data, err := fileutil.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("expected %s to exist: %v", hooksPath, err)
	}
	for _, want := range []string{"zgrep-compliance-enforcer", "run_command", "agent hook pre-command"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("expected hooks.json to contain %q, got:\n%s", want, string(data))
		}
	}

	// Non-destructive default: custom hook content is preserved without --force.
	custom := []byte(`{"custom": true}`)
	if err := fileutil.WriteFile(hooksPath, custom, paths.FilePerm644); err != nil {
		t.Fatalf("failed writing custom hooks.json: %v", err)
	}
	cmd2 := NewInitCmd()
	var buf2 bytes.Buffer
	cmd2.SetOut(&buf2)
	cmd2.SetErr(&buf2)
	cmd2.SetArgs([]string{"--lite", "--with-hooks"})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("second zqk init --lite --with-hooks failed: %v", err)
	}
	got, err := fileutil.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("failed reading hooks.json: %v", err)
	}
	if string(got) != string(custom) {
		t.Errorf("expected existing hooks.json to be preserved without --force, got:\n%s", string(got))
	}
	if !strings.Contains(buf2.String(), "Kept existing hook file") {
		t.Errorf("expected kept-existing message, got:\n%s", buf2.String())
	}

	// With --force, hooks.json is overwritten cleanly.
	cmd3 := NewInitCmd()
	var buf3 bytes.Buffer
	cmd3.SetOut(&buf3)
	cmd3.SetErr(&buf3)
	cmd3.SetArgs([]string{"--lite", "--with-hooks", "--force"})
	if err := cmd3.Execute(); err != nil {
		t.Fatalf("forced zqk init --lite --with-hooks failed: %v", err)
	}
	got3, err := fileutil.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("failed reading hooks.json after --force: %v", err)
	}
	if !strings.Contains(string(got3), "zgrep-compliance-enforcer") {
		t.Errorf("expected --force to restore zgrep-compliance-enforcer hook, got:\n%s", string(got3))
	}
}
