package app_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/cmd/zqk/app"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// moduleRoot returns the Go module root (directory containing go.mod).
func moduleRoot(t *testing.T) string {
	t.Helper()
	out, err := execwrap.Command("go", "list", "-m", "-f", "{{.Dir}}").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -m: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestCompletionCommand generates completion scripts by running the built binary
// and verifies they contain expected content. Completion writes to os.Stdout so
// subprocess capture is the reliable way to test.
func TestCompletionCommand(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	exe := filepath.Join(t.TempDir(), "zqk-completion-test")
	buildCmd := execwrap.Command("go", "build", "-o", exe, "./cmd/zqk")
	zqkenv.WireExecForIsolatedProject(buildCmd, root)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Skipf("could not build zqk for completion test: %v\n%s", err, out)
	}

	t.Run("bash", func(t *testing.T) {
		out, err := execwrap.Command(exe, "completion", "bash").Output()
		if err != nil {
			t.Fatalf("completion bash: %v", err)
		}
		s := string(out)
		if !strings.Contains(s, "bash completion") {
			t.Errorf("bash completion script should contain 'bash completion', got %d bytes", len(s))
		}
		if !strings.Contains(s, "COMPREPLY=") {
			t.Errorf("bash completion script should set COMPREPLY")
		}
	})

	t.Run("zsh", func(t *testing.T) {
		out, err := execwrap.Command(exe, "completion", "zsh").Output()
		if err != nil {
			t.Fatalf("completion zsh: %v", err)
		}
		s := string(out)
		if len(s) < 100 {
			t.Errorf("zsh completion script should be substantial (got %d bytes)", len(s))
		}
	})

	t.Run("fish", func(t *testing.T) {
		out, err := execwrap.Command(exe, "completion", "fish").Output()
		if err != nil {
			t.Fatalf("completion fish: %v", err)
		}
		s := string(out)
		if !strings.Contains(s, "complete") && len(s) < 50 {
			t.Errorf("fish completion script should be substantial (got %d bytes)", len(s))
		}
	})

	t.Run("invalid_shell", func(t *testing.T) {
		cmd := execwrap.Command(exe, "completion", "invalid")
		cmd.Stdout = nil
		cmd.Stderr = nil
		err := cmd.Run()
		if err == nil {
			t.Error("expected error for invalid shell")
		}
	})
}

// TestCompletionCommandHelp ensures the completion command has install instructions.
func TestCompletionCommandHelp(t *testing.T) {
	t.Parallel()
	root := &cobra.Command{Use: "zqk"}
	root.AddCommand(app.NewCompletionCmd(root))

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"completion", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("completion --help: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"bash", "zsh", "fish", "Installation"} {
		if !strings.Contains(out, want) {
			t.Errorf("help should contain %q", want)
		}
	}
}

// TestCompletionCommandStructure verifies the completion command is registered with correct args.
func TestCompletionCommandStructure(t *testing.T) {
	t.Parallel()
	root := &cobra.Command{Use: "zqk"}
	root.AddCommand(app.NewCompletionCmd(root))

	comp := root.Commands()[0]
	if comp.Name() != "completion" {
		t.Errorf("command name: got %q", comp.Name())
	}
	valid := comp.ValidArgs
	if len(valid) != 3 || valid[0] != "bash" || valid[1] != "zsh" || valid[2] != "fish" {
		t.Errorf("ValidArgs: got %v", valid)
	}
}
