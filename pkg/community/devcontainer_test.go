package community_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type devcontainerConfig struct {
	Name              string         `json:"name"`
	DockerFile        string         `json:"dockerFile"`
	ForwardPorts      []int          `json:"forwardPorts"`
	RemoteUser        string         `json:"remoteUser"`
	PostCreateCommand string         `json:"postCreateCommand"`
	Features          map[string]any `json:"features"`
	Customizations    map[string]any `json:"customizations"`
}

// TestDevcontainer_FunctionalAcceptance satisfies CRIT-1789711553454840000-dac1492b.
// Verifies standardized .devcontainer/devcontainer.json, Dockerfile, and setup scripts
// supporting both local VS Code Remote Containers and GitHub Codespaces.
func TestDevcontainer_FunctionalAcceptance(t *testing.T) {
	repoRoot := findRepoRootForSDK(t)
	devcontainerDir := filepath.Join(repoRoot, ".devcontainer")

	jsonPath := filepath.Join(devcontainerDir, "devcontainer.json")
	dockerfilePath := filepath.Join(devcontainerDir, "Dockerfile")
	scriptPath := filepath.Join(devcontainerDir, "post-create.sh")

	for _, p := range []string{jsonPath, dockerfilePath, scriptPath} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected devcontainer artifact missing at: %s", p)
		}
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("failed to read devcontainer.json: %v", err)
	}

	var cfg devcontainerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse devcontainer.json: %v", err)
	}

	if cfg.Name == "" {
		t.Error("expected non-empty devcontainer name")
	}
	if cfg.DockerFile != "Dockerfile" {
		t.Errorf("expected dockerFile 'Dockerfile', got %q", cfg.DockerFile)
	}
	if cfg.RemoteUser != "vscode" {
		t.Errorf("expected remoteUser 'vscode', got %q", cfg.RemoteUser)
	}
	if cfg.PostCreateCommand == "" {
		t.Error("expected non-empty postCreateCommand")
	}

	// Verify forwarded port 8443
	foundPort := false
	for _, port := range cfg.ForwardPorts {
		if port == 8443 {
			foundPort = true
			break
		}
	}
	if !foundPort {
		t.Errorf("expected port 8443 in forwardPorts, got: %v", cfg.ForwardPorts)
	}

	// Verify Go toolchain feature
	if _, ok := cfg.Features["ghcr.io/devcontainers/features/go:1"]; !ok {
		t.Error("expected Go feature 'ghcr.io/devcontainers/features/go:1' in devcontainer.json")
	}

	// Verify VS Code extensions
	vscodeCustom, ok := cfg.Customizations["vscode"].(map[string]any)
	if !ok {
		t.Fatal("expected 'vscode' customization in devcontainer.json")
	}
	extsRaw, ok := vscodeCustom["extensions"].([]any)
	if !ok {
		t.Fatal("expected 'extensions' list in vscode customization")
	}
	extSet := make(map[string]bool)
	for _, e := range extsRaw {
		if s, ok := e.(string); ok {
			extSet[s] = true
		}
	}
	for _, requiredExt := range []string{"golang.go", "redhat.vscode-yaml"} {
		if !extSet[requiredExt] {
			t.Errorf("missing required VS Code extension: %s", requiredExt)
		}
	}

	// Verify Codespaces customizations
	codespacesCustom, ok := cfg.Customizations["codespaces"].(map[string]any)
	if !ok {
		t.Fatal("expected 'codespaces' customization in devcontainer.json")
	}
	if _, ok := codespacesCustom["openFiles"].([]any); !ok {
		t.Error("expected 'openFiles' list in codespaces customization")
	}
}

// TestDevcontainer_BoundaryAndErrorHandling satisfies CRIT-1789711553454841000-b9667206.
// Validates error handling for invalid/missing configurations, missing scripts, and bad schemas.
func TestDevcontainer_BoundaryAndErrorHandling(t *testing.T) {
	repoRoot := findRepoRootForSDK(t)
	verifier := filepath.Join(repoRoot, "scripts", "verify-devcontainer.sh")
	tmpDir := t.TempDir()

	// Sub-test 1: Missing devcontainer directory
	t.Run("MissingDirectory", func(t *testing.T) {
		cmd := exec.Command("bash", verifier, filepath.Join(tmpDir, "non_existent"))
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected error for missing directory, but command succeeded:\n%s", string(out))
		}
	})

	// Sub-test 2: Malformed JSON syntax
	t.Run("MalformedJSON", func(t *testing.T) {
		badDir := filepath.Join(tmpDir, "bad_json")
		if err := os.MkdirAll(badDir, 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(badDir, "devcontainer.json"), []byte("{not_json: true}\n"), 0o644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
		if err := os.WriteFile(filepath.Join(badDir, "Dockerfile"), []byte("FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04\nUSER vscode\n"), 0o644); err != nil {
			t.Fatalf("failed to write Dockerfile: %v", err)
		}
		if err := os.WriteFile(filepath.Join(badDir, "post-create.sh"), []byte("#!/bin/bash\n"), 0o755); err != nil {
			t.Fatalf("failed to write post-create.sh: %v", err)
		}

		cmd := exec.Command("bash", verifier, badDir)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected error for malformed JSON, but command succeeded:\n%s", string(out))
		}
	})

	// Sub-test 3: Missing required port 8443
	t.Run("MissingPort8443", func(t *testing.T) {
		noPortDir := filepath.Join(tmpDir, "no_port")
		if err := os.MkdirAll(noPortDir, 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		invalidJSON := `{
  "name": "Bad Port",
  "dockerFile": "Dockerfile",
  "forwardPorts": [3000],
  "remoteUser": "vscode",
  "postCreateCommand": "bash post-create.sh",
  "customizations": {
    "vscode": {
      "extensions": ["golang.go", "redhat.vscode-yaml"]
    }
  }
}`
		if err := os.WriteFile(filepath.Join(noPortDir, "devcontainer.json"), []byte(invalidJSON), 0o644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
		if err := os.WriteFile(filepath.Join(noPortDir, "Dockerfile"), []byte("FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04\nUSER vscode\n"), 0o644); err != nil {
			t.Fatalf("failed to write Dockerfile: %v", err)
		}
		if err := os.WriteFile(filepath.Join(noPortDir, "post-create.sh"), []byte("#!/bin/bash\n"), 0o755); err != nil {
			t.Fatalf("failed to write post-create.sh: %v", err)
		}

		cmd := exec.Command("bash", verifier, noPortDir)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected error for missing port 8443, but command succeeded:\n%s", string(out))
		}
	})
}

// TestDevcontainer_IntegrationAndConformance satisfies CRIT-1789711553454842000-a22edab1.
// Validates end-to-end conformance via scripts/verify-devcontainer.sh and shell script syntax.
func TestDevcontainer_IntegrationAndConformance(t *testing.T) {
	repoRoot := findRepoRootForSDK(t)
	verifier := filepath.Join(repoRoot, "scripts", "verify-devcontainer.sh")
	devcontainerDir := filepath.Join(repoRoot, ".devcontainer")

	cmd := exec.Command("bash", verifier, devcontainerDir)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("verify-devcontainer.sh failed: %v\nOutput:\n%s", err, string(out))
	}

	// Verify post-create.sh syntax with bash -n
	scriptPath := filepath.Join(devcontainerDir, "post-create.sh")
	checkCmd := exec.Command("bash", "-n", scriptPath)
	if checkOut, err := checkCmd.CombinedOutput(); err != nil {
		t.Fatalf("post-create.sh failed syntax check: %v\nOutput:\n%s", err, string(checkOut))
	}

	// Verify Dockerfile has multi-arch compatible instructions
	dockerfilePath := filepath.Join(devcontainerDir, "Dockerfile")
	dockerBytes, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("failed reading Dockerfile: %v", err)
	}
	dockerContent := string(dockerBytes)
	for _, expectedDirective := range []string{"FROM mcr.microsoft.com/devcontainers/base:", "USER vscode", "ENV ZQK_ROOT=/workspaces/zqk"} {
		if !strings.Contains(dockerContent, expectedDirective) {
			t.Errorf("Dockerfile missing expected directive: %q", expectedDirective)
		}
	}
}
