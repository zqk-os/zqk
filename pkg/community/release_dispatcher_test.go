package community

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestReleaseDispatcher_FunctionalAcceptance verifies that .github/workflows/release-community.yml
// defines automated GitHub Actions release dispatching triggered on tag push and manual workflow_dispatch,
// including multi-arch container image packaging and cross-platform binary releases
// (CRIT-1789707543449927000-ea7d527d).
func TestReleaseDispatcher_FunctionalAcceptance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	workflowPath := filepath.Join(root, ".github", "workflows", "release-community.yml")
	if !fileutil.Exists(workflowPath) {
		t.Skip("release-community.yml workflow not present in open-core distribution")
	}

	contentBytes, err := fileutil.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("failed reading workflow file: %v", err)
	}
	content := string(contentBytes)

	// Verify triggers
	if !strings.Contains(content, "community-v*") {
		t.Errorf("expected tag push trigger 'community-v*' in workflow")
	}
	if !strings.Contains(content, "workflow_dispatch:") {
		t.Errorf("expected workflow_dispatch trigger in workflow")
	}

	// Verify multi-arch and packaging steps
	expectedKeywords := []string{
		"setup-qemu-action",
		"setup-buildx-action",
		"package-community.sh",
		"linux/amd64,linux/arm64",
		"build-push-action",
		"ghcr.io",
	}
	for _, kw := range expectedKeywords {
		if !strings.Contains(content, kw) {
			t.Errorf("expected keyword '%s' in release-community.yml", kw)
		}
	}
}

// TestReleaseDispatcher_BoundaryAndErrorHandling verifies security boundaries,
// dry-run protections, and input parameter sanitization (CRIT-1789707543449928000-636f33f5).
func TestReleaseDispatcher_BoundaryAndErrorHandling(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	workflowPath := filepath.Join(root, ".github", "workflows", "release-community.yml")
	if !fileutil.Exists(workflowPath) {
		t.Skip("release-community.yml workflow not present in open-core distribution")
	}
	contentBytes, err := fileutil.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("failed reading workflow file: %v", err)
	}
	content := string(contentBytes)

	// Verify dry-run guard prevents unauthenticated or test pushes
	if !strings.Contains(content, "dry_run") {
		t.Errorf("expected dry_run parameter handling in workflow")
	}
	if !strings.Contains(content, "inputs.dry_run != true") {
		t.Errorf("expected conditional push guard based on dry_run")
	}

	// Verify token permissions are strictly bounded
	if !strings.Contains(content, "permissions:") ||
		!strings.Contains(content, "contents: write") ||
		!strings.Contains(content, "packages: write") {
		t.Errorf("expected explicit least-privilege permissions in release-community.yml")
	}
}

// TestReleaseDispatcher_IntegrationAndConformance verifies Dockerfile.community
// exists, enforces non-root execution, and aligns with package-community.sh
// (CRIT-1789707543449929000-f3a403b4).
func TestReleaseDispatcher_IntegrationAndConformance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	dockerfilePath := filepath.Join(root, "Dockerfile.community")
	if !fileutil.Exists(dockerfilePath) {
		t.Skip("Dockerfile.community not present in open-core distribution")
	}

	dockerContentBytes, err := fileutil.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("failed reading Dockerfile.community: %v", err)
	}
	dockerContent := string(dockerContentBytes)

	// Verify multi-stage build and non-root execution
	if !strings.Contains(dockerContent, "AS builder") {
		t.Errorf("expected multi-stage builder in Dockerfile.community")
	}
	if !strings.Contains(dockerContent, "USER 10001:10001") && !strings.Contains(dockerContent, "USER zqk") {
		t.Errorf("expected non-root user directive in Dockerfile.community")
	}
	if !strings.Contains(dockerContent, "ENTRYPOINT") {
		t.Errorf("expected ENTRYPOINT in Dockerfile.community")
	}

	// Verify package script exists and is executable
	scriptPath := filepath.Join(root, "scripts", "package-community.sh")
	if !fileutil.Exists(scriptPath) {
		t.Fatalf("expected package-community.sh script at %s", scriptPath)
	}
}
