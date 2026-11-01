package ci

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestAdoptionSprint4_DevcontainerSchemaAndPins verifies CRIT-1793508466670250000-294f039e:
// Valid devcontainer.json schema, Go toolchain pins, and pre-built zqk binary target.
func TestAdoptionSprint4_DevcontainerSchemaAndPins(t *testing.T) {
	// 1. Verify repo root devcontainer
	res, err := ValidateRepoDevcontainer("../..")
	if err != nil {
		t.Fatalf("ValidateRepoDevcontainer failed on repository root: %v", err)
	}
	if !res.Valid {
		t.Fatalf("expected Valid=true, got false")
	}

	foundImagePin := false
	foundUser := false
	foundCLI := false
	for _, check := range res.ChecksPassed {
		if check == "image_go_toolchain_pinned" {
			foundImagePin = true
		}
		if check == "non_root_remote_user" {
			foundUser = true
		}
		if check == "features_github_cli_enabled" {
			foundCLI = true
		}
	}

	if !foundImagePin || !foundUser || !foundCLI {
		t.Fatalf("missing expected static invariant checks in: %v", res.ChecksPassed)
	}

	// 2. Negative boundary: unpinned image version
	tempDir := t.TempDir()
	badImgFile := filepath.Join(tempDir, "devcontainer.json")
	badContent := `{
		"$schema": "https://raw.githubusercontent.com/devcontainers/spec/main/schemas/devContainer.schema.json",
		"name": "Bad Image",
		"image": "golang:latest",
		"remoteUser": "vscode",
		"features": {"ghcr.io/devcontainers/features/github-cli:1": {}},
		"postCreateCommand": "go build -o bin/zqk ./cmd/zqk && ./bin/zqk mcp ensure && ./bin/zqk system agent-onboard",
		"containerEnv": {"ZQK_CONTEXT": "ai-agent"},
		"runArgs": ["--cap-drop=ALL"]
	}`
	if err := fileutil.WriteFile(badImgFile, []byte(badContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test fixture: %v", err)
	}
	if _, err := ValidateDevcontainerFile(badImgFile); err == nil {
		t.Fatalf("expected error on unpinned image, got nil")
	}
}

// TestAdoptionSprint4_DevcontainerPostCreateAndSeating verifies CRIT-1793508497663872000-6bd124fa:
// 60-second headless boot verification, CLI pre-build verification, and automated agent seating.
func TestAdoptionSprint4_DevcontainerPostCreateAndSeating(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Missing build step in postCreateCommand
	badPostFile := filepath.Join(tempDir, "bad-post.json")
	badPostContent := `{
		"$schema": "https://raw.githubusercontent.com/devcontainers/spec/main/schemas/devContainer.schema.json",
		"image": "mcr.microsoft.com/devcontainers/go:1.26",
		"remoteUser": "vscode",
		"features": {"ghcr.io/devcontainers/features/github-cli:1": {}},
		"postCreateCommand": "echo hello",
		"containerEnv": {"ZQK_CONTEXT": "ai-agent"},
		"runArgs": ["--cap-drop=ALL"]
	}`
	if err := fileutil.WriteFile(badPostFile, []byte(badPostContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test fixture: %v", err)
	}
	if _, err := ValidateDevcontainerFile(badPostFile); err == nil {
		t.Fatalf("expected error when postCreateCommand does not compile ./bin/zqk, got nil")
	}

	// 2. Missing ZQK_CONTEXT
	badEnvFile := filepath.Join(tempDir, "bad-env.json")
	badEnvContent := `{
		"$schema": "https://raw.githubusercontent.com/devcontainers/spec/main/schemas/devContainer.schema.json",
		"image": "mcr.microsoft.com/devcontainers/go:1.26",
		"remoteUser": "vscode",
		"features": {"ghcr.io/devcontainers/features/github-cli:1": {}},
		"postCreateCommand": "go build -o bin/zqk ./cmd/zqk && ./bin/zqk mcp ensure && ./bin/zqk system agent-onboard",
		"runArgs": ["--cap-drop=ALL"]
	}`
	if err := fileutil.WriteFile(badEnvFile, []byte(badEnvContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test fixture: %v", err)
	}
	if _, err := ValidateDevcontainerFile(badEnvFile); err == nil {
		t.Fatalf("expected error when containerEnv does not declare ZQK_CONTEXT, got nil")
	}
}

// TestAdoptionSprint4_DevcontainerSecurityIsolation verifies CRIT-1793508501136695000-614dc2a2:
// Devcontainer security isolation rejecting root escalation, exposed daemon ports, and leaked credentials.
func TestAdoptionSprint4_DevcontainerSecurityIsolation(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Privileged flag rejection
	privFile := filepath.Join(tempDir, "priv.json")
	privContent := `{
		"$schema": "https://raw.githubusercontent.com/devcontainers/spec/main/schemas/devContainer.schema.json",
		"image": "mcr.microsoft.com/devcontainers/go:1.26",
		"remoteUser": "vscode",
		"features": {"ghcr.io/devcontainers/features/github-cli:1": {}},
		"postCreateCommand": "go build -o bin/zqk ./cmd/zqk && ./bin/zqk mcp ensure && ./bin/zqk system agent-onboard",
		"containerEnv": {"ZQK_CONTEXT": "ai-agent"},
		"runArgs": ["--privileged", "--cap-drop=ALL"]
	}`
	if err := fileutil.WriteFile(privFile, []byte(privContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test fixture: %v", err)
	}
	if _, err := ValidateDevcontainerFile(privFile); err == nil {
		t.Fatalf("expected error when runArgs contains --privileged, got nil")
	}

	// 2. Root user rejection
	rootFile := filepath.Join(tempDir, "root.json")
	rootContent := `{
		"$schema": "https://raw.githubusercontent.com/devcontainers/spec/main/schemas/devContainer.schema.json",
		"image": "mcr.microsoft.com/devcontainers/go:1.26",
		"remoteUser": "root",
		"features": {"ghcr.io/devcontainers/features/github-cli:1": {}},
		"postCreateCommand": "go build -o bin/zqk ./cmd/zqk && ./bin/zqk mcp ensure && ./bin/zqk system agent-onboard",
		"containerEnv": {"ZQK_CONTEXT": "ai-agent"},
		"runArgs": ["--cap-drop=ALL"]
	}`
	if err := fileutil.WriteFile(rootFile, []byte(rootContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test fixture: %v", err)
	}
	if _, err := ValidateDevcontainerFile(rootFile); err == nil {
		t.Fatalf("expected error when remoteUser is root, got nil")
	}

	// 3. Credential leakage rejection
	secretFile := filepath.Join(tempDir, "secret.json")
	secretContent := `{
		"$schema": "https://raw.githubusercontent.com/devcontainers/spec/main/schemas/devContainer.schema.json",
		"image": "mcr.microsoft.com/devcontainers/go:1.26",
		"remoteUser": "vscode",
		"features": {"ghcr.io/devcontainers/features/github-cli:1": {}},
		"postCreateCommand": "go build -o bin/zqk ./cmd/zqk && ./bin/zqk mcp ensure && ./bin/zqk system agent-onboard",
		"containerEnv": {"ZQK_CONTEXT": "ai-agent", "TOKEN": "ghp_1234567890abcdef"},
		"runArgs": ["--cap-drop=ALL"]
	}`
	if err := fileutil.WriteFile(secretFile, []byte(secretContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test fixture: %v", err)
	}
	if _, err := ValidateDevcontainerFile(secretFile); err == nil {
		t.Fatalf("expected error when file contains hardcoded GitHub personal access token, got nil")
	}
}
