package community

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestContainerDistribution_FunctionalAcceptance verifies that Dockerfile.community
// and deploy/helm/zqk-community chart conform to open-core container distribution specifications
// (CRIT-1789709342532029000-afe2f34a).
func TestContainerDistribution_FunctionalAcceptance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")

	// 1. Verify Dockerfile.community
	dockerfilePath := filepath.Join(root, "Dockerfile.community")
	if !fileutil.Exists(dockerfilePath) {
		t.Skipf("Dockerfile.community not found at %s (studio-only artifact)", dockerfilePath)
	}

	dockerfileBytes, err := fileutil.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("failed to read Dockerfile.community: %v", err)
	}
	dockerfileContent := string(dockerfileBytes)

	if !strings.Contains(dockerfileContent, "ENTRYPOINT") {
		t.Errorf("expected Dockerfile.community to define an ENTRYPOINT")
	}
	if !strings.Contains(dockerfileContent, "USER 10001") && !strings.Contains(dockerfileContent, "USER zqk") {
		t.Errorf("expected Dockerfile.community to run as non-root user (10001 or zqk)")
	}
	if !strings.Contains(dockerfileContent, "HEALTHCHECK") {
		t.Errorf("expected Dockerfile.community to define a HEALTHCHECK")
	}

	// 2. Verify Helm chart structure
	helmDir := filepath.Join(root, "deploy", "helm", "zqk-community")
	chartYamlPath := filepath.Join(helmDir, "Chart.yaml")
	valuesYamlPath := filepath.Join(helmDir, "values.yaml")
	templatesDir := filepath.Join(helmDir, "templates")

	if !fileutil.Exists(chartYamlPath) {
		t.Skipf("Chart.yaml not found at %s (studio-only artifact)", chartYamlPath)
	}
	if !fileutil.Exists(valuesYamlPath) {
		t.Fatalf("expected values.yaml at %s", valuesYamlPath)
	}
	if !fileutil.Exists(templatesDir) {
		t.Fatalf("expected templates directory at %s", templatesDir)
	}

	chartBytes, err := fileutil.ReadFile(chartYamlPath)
	if err != nil {
		t.Fatalf("failed to read Chart.yaml: %v", err)
	}
	chartContent := string(chartBytes)
	if !strings.Contains(chartContent, "name: zqk-community") {
		t.Errorf("expected Chart.yaml to specify 'name: zqk-community'")
	}
	if !strings.Contains(chartContent, "apiVersion: v2") {
		t.Errorf("expected Chart.yaml to specify 'apiVersion: v2'")
	}

	// 3. Package Helm chart into temporary directory
	packageScript := filepath.Join(root, "scripts", "package-helm-chart.sh")
	if !fileutil.Exists(packageScript) {
		t.Skipf("package-helm-chart.sh not found at %s (studio-only artifact)", packageScript)
	}

	tmpDir := t.TempDir()
	cmd := testkit.ManagedCommand(t, t.Context(), "bash", packageScript, "v1.5.0", tmpDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("package-helm-chart.sh failed: %v, output: %s", err, string(out))
	}

	expectedTgz := filepath.Join(tmpDir, "zqk-community-1.5.0.tgz")
	if !fileutil.Exists(expectedTgz) {
		t.Fatalf("expected packaged helm chart at %s", expectedTgz)
	}
}

// TestContainerDistribution_BoundaryAndErrorHandling verifies error handling
// on invalid chart archives, missing arguments, and corrupt archives
// (CRIT-1789709342532030000-97b190d2).
func TestContainerDistribution_BoundaryAndErrorHandling(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	packageScript := filepath.Join(root, "scripts", "package-helm-chart.sh")
	if !fileutil.Exists(packageScript) {
		t.Skipf("package-helm-chart.sh not found at %s (studio-only artifact)", packageScript)
	}

	// 1. Missing verify target argument
	cmdNoArg := testkit.ManagedCommand(t, t.Context(), "bash", packageScript, "--verify")
	if err := cmdNoArg.Run(); err == nil {
		t.Errorf("expected --verify without argument to exit with error")
	}

	// 2. Nonexistent archive
	cmdNonexistent := testkit.ManagedCommand(t, t.Context(), "bash", packageScript, "--verify", "/nonexistent/chart.tgz")
	if err := cmdNonexistent.Run(); err == nil {
		t.Errorf("expected --verify on nonexistent file to exit with error")
	}

	// 3. Corrupt archive
	tmpDir := t.TempDir()
	corruptTgz := filepath.Join(tmpDir, "corrupt.tgz")
	if err := fileutil.WriteFile(corruptTgz, []byte("invalid gzip stream content"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write corrupt archive: %v", err)
	}
	cmdCorrupt := testkit.ManagedCommand(t, t.Context(), "bash", packageScript, "--verify", corruptTgz)
	if err := cmdCorrupt.Run(); err == nil {
		t.Errorf("expected --verify on corrupt archive to exit with error")
	}

	// 4. Archive missing Chart.yaml
	emptyTgz := filepath.Join(tmpDir, "empty.tgz")
	f, err := os.Create(emptyTgz)
	if err != nil {
		t.Fatalf("failed to create empty tarball: %v", err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	tw.Close()
	gw.Close()
	f.Close()

	cmdMissingChart := testkit.ManagedCommand(t, t.Context(), "bash", packageScript, "--verify", emptyTgz)
	if err := cmdMissingChart.Run(); err == nil {
		t.Errorf("expected --verify on archive missing Chart.yaml to fail")
	}
}

// TestContainerDistribution_IntegrationAndConformance verifies release packaging
// and workflow integration for container registries and Helm charts
// (CRIT-1789709342532031000-56f851f9).
func TestContainerDistribution_IntegrationAndConformance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")

	// 1. Verify package-community.sh integrates Helm chart packaging
	packageScript := filepath.Join(root, "scripts", "package-community.sh")
	if !fileutil.Exists(packageScript) {
		t.Skipf("package-community.sh not found at %s (studio-only artifact)", packageScript)
	}
	packageBytes, err := fileutil.ReadFile(packageScript)
	if err != nil {
		t.Fatalf("failed to read package-community.sh: %v", err)
	}
	packageContent := string(packageBytes)

	if !strings.Contains(packageContent, "package-helm-chart.sh") {
		t.Errorf("expected package-community.sh to invoke package-helm-chart.sh")
	}
	if !strings.Contains(packageContent, "--verify") {
		t.Errorf("expected package-community.sh to verify helm chart archive")
	}
	if !strings.Contains(packageContent, "*.tgz") {
		t.Errorf("expected package-community.sh to include *.tgz in checksums and release upload")
	}

	// 2. Verify release-community.yml workflow includes GHCR and Helm OCI publishing
	workflowPath := filepath.Join(root, ".github", "workflows", "release-community.yml")
	if !fileutil.Exists(workflowPath) {
		t.Skipf("release-community.yml not found at %s (studio-only artifact)", workflowPath)
	}
	workflowBytes, err := fileutil.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("failed to read release-community.yml: %v", err)
	}
	workflowContent := string(workflowBytes)

	if !strings.Contains(workflowContent, "ghcr.io") {
		t.Errorf("expected release-community.yml to configure ghcr.io registry")
	}
	if !strings.Contains(workflowContent, "package-helm-chart.sh") {
		t.Errorf("expected release-community.yml to execute package-helm-chart.sh")
	}
	if !strings.Contains(workflowContent, "helm push") {
		t.Errorf("expected release-community.yml to push helm chart to OCI registry")
	}

	// 3. Verify Helm chart tarball internal entries
	tmpDir := t.TempDir()
	genScript := filepath.Join(root, "scripts", "package-helm-chart.sh")
	if out, err := testkit.ManagedCommand(t, t.Context(), "bash", genScript, "v2.0.0", tmpDir).CombinedOutput(); err != nil {
		t.Fatalf("failed to package helm chart: %v, out: %s", err, string(out))
	}

	chartPath := filepath.Join(tmpDir, "zqk-community-2.0.0.tgz")
	cf, err := os.Open(chartPath)
	if err != nil {
		t.Fatalf("failed to open packaged chart: %v", err)
	}
	defer cf.Close()

	gr, err := gzip.NewReader(cf)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	hasChart := false
	hasValues := false
	hasTemplates := false

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("failed reading tar: %v", err)
		}
		name := filepath.ToSlash(hdr.Name)
		if strings.HasSuffix(name, "Chart.yaml") {
			hasChart = true
		}
		if strings.HasSuffix(name, "values.yaml") {
			hasValues = true
		}
		if strings.Contains(name, "templates/") {
			hasTemplates = true
		}
	}

	if !hasChart || !hasValues || !hasTemplates {
		t.Errorf("packaged chart missing required entries: Chart=%v, Values=%v, Templates=%v", hasChart, hasValues, hasTemplates)
	}
}

// TestContainerPackaging_StaticFloorInvariant verifies that container definitions
// specify non-root execution (USER 10001), multi-stage builds, and healthcheck configurations
// (CRIT-1794382593524814000-58594100).
func TestContainerPackaging_StaticFloorInvariant(t *testing.T) {
	root := paths.ResolveProjectRoot(".")

	// Invariant 1: Dockerfile.community multi-stage and hardening
	communityPath := filepath.Join(root, "Dockerfile.community")
	communityBytes, err := fileutil.ReadFile(communityPath)
	if err != nil {
		t.Fatalf("failed to read Dockerfile.community: %v", err)
	}
	communityStr := string(communityBytes)
	if !strings.Contains(communityStr, "FROM golang:1.26-alpine AS builder") || !strings.Contains(communityStr, "AS runner") {
		t.Errorf("expected Dockerfile.community multi-stage scaffolding")
	}
	if !strings.Contains(communityStr, "USER 10001:10001") && !strings.Contains(communityStr, "USER 10001") {
		t.Errorf("expected Dockerfile.community non-root USER 10001")
	}
	if !strings.Contains(communityStr, "HEALTHCHECK") {
		t.Errorf("expected Dockerfile.community HEALTHCHECK")
	}

	// Invariant 2: Dockerfile.standalone multi-stage and local inference topology
	standalonePath := filepath.Join(root, "Dockerfile.standalone")
	standaloneBytes, err := fileutil.ReadFile(standalonePath)
	if err != nil {
		t.Fatalf("failed to read Dockerfile.standalone: %v", err)
	}
	standaloneStr := string(standaloneBytes)
	if !strings.Contains(standaloneStr, "AS zqk-builder") || !strings.Contains(standaloneStr, "AS llama-builder") || !strings.Contains(standaloneStr, "AS runner") {
		t.Errorf("expected Dockerfile.standalone multi-stage scaffolding")
	}
	if !strings.Contains(standaloneStr, "USER 10001") {
		t.Errorf("expected Dockerfile.standalone non-root USER 10001")
	}
	if !strings.Contains(standaloneStr, "HEALTHCHECK") {
		t.Errorf("expected Dockerfile.standalone HEALTHCHECK")
	}
	if !strings.Contains(standaloneStr, "EXPOSE 8080 8443") {
		t.Errorf("expected Dockerfile.standalone EXPOSE 8080 8443")
	}

	// Invariant 3: GitHub Actions Container Publishing Workflow includes both images
	wfPath := filepath.Join(root, ".github", "workflows", "publish-containers.yml")
	wfBytes, err := fileutil.ReadFile(wfPath)
	if err != nil {
		t.Fatalf("failed to read publish-containers.yml: %v", err)
	}
	wfStr := string(wfBytes)
	if !strings.Contains(wfStr, "build-and-push-community:") || !strings.Contains(wfStr, "build-and-push-standalone:") {
		t.Errorf("expected publishing workflow to build both community and standalone images")
	}
}

// TestContainerPackaging_OperationalProof verifies turnkey standalone supervisor execution
// with command forwarding and simulated local inference healthcheck
// (CRIT-1794382599001279000-ffa42c37).
func TestContainerPackaging_OperationalProof(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	entrypointScript := filepath.Join(root, "scripts", "open-core", "entrypoint-standalone.sh")

	tmpDir := t.TempDir()
	mockBin := filepath.Join(tmpDir, "mock-zqk.sh")
	mockScript := "#!/bin/sh\necho \"MOCK_ZQK_INVOKED: $@\"\nexit 0\n"
	if err := fileutil.WriteFile(mockBin, []byte(mockScript), paths.FilePerm755); err != nil {
		t.Fatalf("failed to write mock binary: %v", err)
	}

	cmd := testkit.ManagedCommand(t, t.Context(), "bash", entrypointScript, "do", "--timeout", "5m")
	cmd.Env = append(cmd.Environ(),
		"ZQK_BIN="+mockBin,
		"MODEL_PATH=",
		"ZQK_UID_OVERRIDE=10001",
		"ZQK_CONTAINER_MEM_MB=8192",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("entrypoint script execution failed: %v, out: %s", err, string(out))
	}
	outStr := string(out)

	if !strings.Contains(outStr, "[STANDALONE] No local GGUF model detected") {
		t.Errorf("expected fallback notice in output, got: %s", outStr)
	}
	if !strings.Contains(outStr, "MOCK_ZQK_INVOKED: do --timeout 5m") {
		t.Errorf("expected argument forwarding to ZQK_BIN, got: %s", outStr)
	}
}

// TestContainerPackaging_NegativeAdversarialBoundary verifies that container execution
// fails closed on root UID 0 execution or sub-6GB memory allocations
// (CRIT-1794382604694864000-68135698).
func TestContainerPackaging_NegativeAdversarialBoundary(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	entrypointScript := filepath.Join(root, "scripts", "open-core", "entrypoint-standalone.sh")

	tmpDir := t.TempDir()
	mockBin := filepath.Join(tmpDir, "mock-zqk.sh")
	mockScript := "#!/bin/sh\necho \"SHOULD_NOT_EXECUTE\"\nexit 0\n"
	if err := fileutil.WriteFile(mockBin, []byte(mockScript), paths.FilePerm755); err != nil {
		t.Fatalf("failed to write mock binary: %v", err)
	}

	// Boundary 1: Root execution rejection (UID 0)
	cmdRoot := testkit.ManagedCommand(t, t.Context(), "bash", entrypointScript, "do")
	cmdRoot.Env = append(cmdRoot.Environ(),
		"ZQK_BIN="+mockBin,
		"MODEL_PATH=",
		"ZQK_UID_OVERRIDE=0",
		"ZQK_CONTAINER_MEM_MB=8192",
	)
	outRoot, errRoot := cmdRoot.CombinedOutput()
	if errRoot == nil {
		t.Fatalf("expected root execution to fail, but succeeded with output: %s", string(outRoot))
	}
	if !strings.Contains(string(outRoot), "Container execution as root (UID 0) is prohibited") {
		t.Errorf("expected root execution failure message, got: %s", string(outRoot))
	}

	// Boundary 2: Sub-6GB memory allocation rejection (< 6144 MB)
	cmdSubMem := testkit.ManagedCommand(t, t.Context(), "bash", entrypointScript, "do")
	cmdSubMem.Env = append(cmdSubMem.Environ(),
		"ZQK_BIN="+mockBin,
		"MODEL_PATH=",
		"ZQK_UID_OVERRIDE=10001",
		"ZQK_CONTAINER_MEM_MB=4096",
	)
	outSubMem, errSubMem := cmdSubMem.CombinedOutput()
	if errSubMem == nil {
		t.Fatalf("expected sub-6GB RAM allocation to fail, but succeeded with output: %s", string(outSubMem))
	}
	if !strings.Contains(string(outSubMem), "Insufficient host RAM allocated") || !strings.Contains(string(outSubMem), "minimum of 6144MB") {
		t.Errorf("expected memory boundary failure message, got: %s", string(outSubMem))
	}
}
