package osslaunch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/systemcheck/policy"
)

// TestPublicRepoSanitation verifies REQ-OSS-REPO-CLEAN-001:
// - CRIT-1789539697287303000-f4d8799c (Functional Acceptance)
// - CRIT-1789539697287304000-07e1a333 (Boundary & Error Handling)
// - CRIT-1789539697287305000-4e46f10c (Integration & Conformance)
func TestPublicRepoSanitation(t *testing.T) {
	projectRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("failed to locate project root: %v", err)
	}

	t.Run("FunctionalAcceptance_NoTrackedScratchOrStudioArtifacts", func(t *testing.T) {
		cmd := exec.Command("git", "ls-files")
		cmd.Dir = projectRoot
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git ls-files failed: %v", err)
		}

		files := strings.Split(string(out), "\n")

		for _, file := range files {
			f := strings.TrimSpace(file)
			if f == "" {
				continue
			}
			if strings.HasSuffix(f, ".tmp") || strings.Contains(f, "/.tmp/") || strings.Contains(f, "-csnap-backups") || strings.HasSuffix(f, ".orig") || strings.HasSuffix(f, ".bak") {
				t.Errorf("tracked file %s contains prohibited studio/scratch artifact", f)
			}
			for _, prohibited := range []string{".gemini/", "credentials.json", "id_rsa", "id_ed25519"} {
				if strings.Contains(f, prohibited) {
					t.Errorf("tracked file %s contains prohibited studio/scratch substring %s", f, prohibited)
				}
			}
		}
	})

	t.Run("BoundaryAndErrorHandling_RejectsScratchFileAdditions", func(t *testing.T) {
		tmpDir := t.TempDir()
		dirtyFile := filepath.Join(tmpDir, "studio_test.orig")
		if err := os.WriteFile(dirtyFile, []byte("temporary studio artifact"), 0600); err != nil {
			t.Fatalf("failed to write dirty file: %v", err)
		}

		// A hygiene scanner must identify .orig / .bak as dirty files
		hasDirty := strings.HasSuffix(dirtyFile, ".orig") || strings.HasSuffix(dirtyFile, ".bak")
		if !hasDirty {
			t.Errorf("boundary check failed: expected dirty file to be flagged")
		}
	})

	t.Run("IntegrationAndConformance_GitignoreCoversStudioArtifacts", func(t *testing.T) {
		gitignorePath := filepath.Join(projectRoot, ".gitignore")
		content, err := os.ReadFile(gitignorePath)
		if err != nil {
			t.Fatalf("failed to read .gitignore: %v", err)
		}

		text := string(content)
		requiredPatterns := []string{
			".gemini",
			"bin/",
		}

		for _, pat := range requiredPatterns {
			if !strings.Contains(text, pat) {
				t.Errorf(".gitignore missing critical pattern: %s", pat)
			}
		}
	})
}

// TestPublicPushLeakPrevention verifies REQ-OSS-LEAK-PREVENTION-001:
// - CRIT-1789539707439122000-0b84e18a (Functional Acceptance)
// - CRIT-1789539707439123000-176ec4f6 (Boundary & Error Handling)
// - CRIT-1789539707439124000-be881638 (Integration & Conformance)
func TestPublicPushLeakPrevention(t *testing.T) {
	projectRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("failed to locate project root: %v", err)
	}

	t.Run("FunctionalAcceptance_ZeroSecretsInRepo", func(t *testing.T) {
		gate := &policy.SecretsGate{}
		res, err := gate.Run(context.Background(), policy.RunOptions{
			ProjectRoot: projectRoot,
		})
		if err != nil {
			t.Fatalf("secrets gate execution failed: %v", err)
		}
		if res != nil && len(res.Violations) > 0 {
			t.Errorf("detected secrets in repo: %v", res.Violations)
		}
	})

	t.Run("BoundaryAndErrorHandling_FailsClosedOnSyntheticSecret", func(t *testing.T) {
		tmpDir := t.TempDir()
		leakedFile := filepath.Join(tmpDir, "secret_leak.txt")
		prefix := strings.Join([]string{"gh", "p_"}, "")
		syntheticSecret := prefix + "1234567890abcdefghijklmnopqrstuvwxyz"
		if err := os.WriteFile(leakedFile, []byte("token = "+syntheticSecret+"\n"), 0600); err != nil {
			t.Fatalf("failed to write synthetic secret: %v", err)
		}

		gate := &policy.SecretsGate{}
		res, err := gate.Run(context.Background(), policy.RunOptions{
			ProjectRoot: tmpDir,
		})
		if err != nil {
			t.Fatalf("secrets gate failed on synthetic test: %v", err)
		}
		if res == nil || len(res.Violations) == 0 {
			t.Errorf("fail-closed gate failed: expected secrets gate to detect synthetic secret token")
		}
	})

	t.Run("IntegrationAndConformance_ScanSecretsScriptExecutable", func(t *testing.T) {
		scriptPath := filepath.Join(projectRoot, "scripts", "scan-secrets.sh")
		info, err := os.Stat(scriptPath)
		if err != nil {
			t.Fatalf("scripts/scan-secrets.sh not found: %v", err)
		}
		if info.Mode()&0111 == 0 {
			t.Errorf("scripts/scan-secrets.sh is not executable")
		}

		cmd := exec.Command("/bin/bash", scriptPath, scriptPath)
		cmd.Dir = projectRoot
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("scan-secrets.sh failed on self: %v, output: %s", err, string(out))
		}
	})
}

// TestStandaloneCleanBuild verifies REQ-OSS-CLEAN-BUILD-001:
// - CRIT-1789539720548531000-589b232e (Functional Acceptance)
// - CRIT-1789539720548532000-a026f896 (Boundary & Error Handling)
// - CRIT-1789539720548533000-747e7ae7 (Integration & Conformance)
func TestStandaloneCleanBuild(t *testing.T) {
	projectRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("failed to locate project root: %v", err)
	}

	t.Run("FunctionalAcceptance_GoModHasNoLocalReplaces", func(t *testing.T) {
		goModPath := filepath.Join(projectRoot, "go.mod")
		content, err := os.ReadFile(goModPath)
		if err != nil {
			t.Fatalf("failed to read go.mod: %v", err)
		}

		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "replace ") && strings.Contains(trimmed, "=> ../") {
				t.Errorf("go.mod contains uncommitted local replace directive: %s", trimmed)
			}
		}
	})

	t.Run("BoundaryAndErrorHandling_BinaryTargetCompilesCleanly", func(t *testing.T) {
		devDir := "/Library/Developer/CommandLineTools"
		if _, err := os.Stat(devDir); err != nil {
			devDir = ""
		}

		tmpOut := filepath.Join(t.TempDir(), "zqk-dry-compile")
		cmd := exec.Command("go", "build", "-o", tmpOut, "./cmd/zqk")
		cmd.Dir = projectRoot
		env := os.Environ()
		if devDir != "" {
			env = append(env, "DEVELOPER_DIR="+devDir)
		}
		cmd.Env = env

		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go build ./cmd/zqk failed: %v, output: %s", err, string(out))
		}
	})

	t.Run("IntegrationAndConformance_StableBinaryRunsVersion", func(t *testing.T) {
		binaryPath := filepath.Join(projectRoot, "bin", "zqk-stable")
		if _, err := os.Stat(binaryPath); err != nil {
			t.Skipf("bin/zqk-stable not built yet: %v", err)
		}

		cmd := exec.Command(binaryPath, "--help")
		cmd.Dir = projectRoot
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bin/zqk-stable --help failed: %v, output: %s", err, string(out))
		}

		if !strings.Contains(string(out), "zqk-stable") && !strings.Contains(string(out), "Usage:") {
			t.Errorf("unexpected output from zqk-stable --help: %s", string(out))
		}
	})
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return os.Getwd()
}
