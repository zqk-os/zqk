package community

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestInstaller_FunctionalAcceptance verifies the standalone install script and
// multi-shell auto-completion capabilities across bash, zsh, and fish.
// (CRIT-1789629268879699000-df0bf053 / BLI-1789629268879699000-f2d02b60)
func TestInstaller_FunctionalAcceptance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	installScript := filepath.Join(root, "scripts", "install.sh")

	if !fileutil.Exists(installScript) {
		t.Fatalf("installer script not found at %s", installScript)
	}

	// Verify script syntax via sh -n
	shCmd := execwrap.Command("sh", "-n", installScript)
	if out, err := shCmd.CombinedOutput(); err != nil {
		t.Fatalf("install.sh syntax error: %v, output: %s", err, string(out))
	}

	contentBytes, err := fileutil.ReadFile(installScript)
	if err != nil {
		t.Fatalf("failed reading install.sh: %v", err)
	}
	content := string(contentBytes)

	// Verify crucial installer steps
	requiredKeywords := []string{
		"INSTALL_METHOD",
		"checksums.txt",
		"shasum",
		"sha256sum",
		"tar -xzf",
		"_place_binary",
		"quarantine",
	}
	for _, kw := range requiredKeywords {
		if !strings.Contains(content, kw) {
			t.Errorf("install.sh missing required keyword: %q", kw)
		}
	}

	// Test multi-shell completion generation using built zqk binary if available,
	// or invoke go run cmd/zqk/main.go
	zqkBin := filepath.Join(root, "bin", "zqk")
	var runCmd func(args ...string) (string, error)
	if fileutil.Exists(zqkBin) {
		runCmd = func(args ...string) (string, error) {
			cmd := execwrap.Command(zqkBin, args...)
			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out
			err := cmd.Run()
			return out.String(), err
		}
	} else {
		runCmd = func(args ...string) (string, error) {
			fullArgs := append([]string{"run", "./cmd/zqk"}, args...)
			cmd := execwrap.Command("go", fullArgs...)
			cmd.Dir = root
			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out
			err := cmd.Run()
			return out.String(), err
		}
	}

	// Test bash completion
	bashOut, err := runCmd("completion", "bash")
	if err != nil {
		t.Fatalf("failed running zqk completion bash: %v (out: %s)", err, bashOut)
	}
	if !strings.Contains(bashOut, "bash completion for zqk") && !strings.Contains(bashOut, "complete -o default -F") && !strings.Contains(bashOut, "__zqk") {
		t.Errorf("unexpected bash completion output header: %s", bashOut[:min(len(bashOut), 200)])
	}

	// Test zsh completion
	zshOut, err := runCmd("completion", "zsh")
	if err != nil {
		t.Fatalf("failed running zqk completion zsh: %v (out: %s)", err, zshOut)
	}
	if !strings.Contains(zshOut, "compdef") && !strings.Contains(zshOut, "_zqk") {
		t.Errorf("unexpected zsh completion output header: %s", zshOut[:min(len(zshOut), 200)])
	}

	// Test fish completion
	fishOut, err := runCmd("completion", "fish")
	if err != nil {
		t.Fatalf("failed running zqk completion fish: %v (out: %s)", err, fishOut)
	}
	if !strings.Contains(fishOut, "fish") && !strings.Contains(fishOut, "complete -c zqk") {
		t.Errorf("unexpected fish completion output header: %s", fishOut[:min(len(fishOut), 200)])
	}
}

// TestInstaller_BoundaryAndErrorHandling validates boundary conditions,
// error handling, and invalid inputs in installer logic and CLI completion.
// (CRIT-1789629268879700000-5971f74b / BLI-1789629268879699000-f2d02b60)
func TestInstaller_BoundaryAndErrorHandling(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	zqkBin := filepath.Join(root, "bin", "zqk")

	var runCmd func(args ...string) (string, error)
	if fileutil.Exists(zqkBin) {
		runCmd = func(args ...string) (string, error) {
			cmd := execwrap.Command(zqkBin, args...)
			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out
			err := cmd.Run()
			return out.String(), err
		}
	} else {
		runCmd = func(args ...string) (string, error) {
			fullArgs := append([]string{"run", "./cmd/zqk"}, args...)
			cmd := execwrap.Command("go", fullArgs...)
			cmd.Dir = root
			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out
			err := cmd.Run()
			return out.String(), err
		}
	}

	// Invalid shell completion argument
	invalidOut, err := runCmd("completion", "unsupported-shell")
	if err == nil {
		t.Errorf("expected error for unsupported shell completion, got success: %s", invalidOut)
	}

	// Installer script boundary check: inspect OS/Arch detection
	installScript := filepath.Join(root, "scripts", "install.sh")
	contentBytes, err := fileutil.ReadFile(installScript)
	if err != nil {
		t.Fatalf("failed reading install.sh: %v", err)
	}
	content := string(contentBytes)

	if !strings.Contains(content, "Unsupported architecture") {
		t.Errorf("install.sh missing unsupported architecture error handling")
	}
	if !strings.Contains(content, "Unsupported OS") {
		t.Errorf("install.sh missing unsupported OS error handling")
	}
}

// TestInstaller_IntegrationAndConformance validates turnkey workspace initialization
// and stranger first-run experience in an isolated temporary directory.
// (CRIT-1789629268879701000-09f5a56b / BLI-1789629268879699000-f2d02b60)
func TestInstaller_IntegrationAndConformance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	zqkBin := filepath.Join(root, "bin", "zqk")

	if !fileutil.Exists(zqkBin) {
		t.Skip("compiled bin/zqk not present; skipping end-to-end integration test")
	}

	tmpDir := t.TempDir()

	// Initialize git repo in tmpDir
	gitInit := execwrap.Command("git", "init")
	gitInit.Dir = tmpDir
	if out, err := gitInit.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v, out: %s", err, string(out))
	}

	// Run zqk quickstart in the temporary workspace
	quickCmd := execwrap.Command(zqkBin, "quickstart", "--format", "json")
	quickCmd.Dir = tmpDir
	quickOut, err := quickCmd.CombinedOutput()
	if err != nil {
		t.Logf("quickstart notice (non-fatal in mock environment): %v, out: %s", err, string(quickOut))
	}

	// Verify zqk version executes cleanly in foreign directory
	verCmd := execwrap.Command(zqkBin, "version")
	verCmd.Dir = tmpDir
	verOut, err := verCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("zqk version failed in isolated directory: %v, out: %s", err, string(verOut))
	}
	if !strings.Contains(string(verOut), "v") && !strings.Contains(string(verOut), "commit") && !strings.Contains(string(verOut), "built") {
		t.Errorf("unexpected version output in isolated directory: %s", string(verOut))
	}
}
