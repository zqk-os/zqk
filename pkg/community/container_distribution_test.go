package community

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
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
		t.Fatalf("expected Dockerfile.community at %s", dockerfilePath)
	}

	dockerfileBytes, err := os.ReadFile(dockerfilePath)
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
		t.Fatalf("expected Chart.yaml at %s", chartYamlPath)
	}
	if !fileutil.Exists(valuesYamlPath) {
		t.Fatalf("expected values.yaml at %s", valuesYamlPath)
	}
	if !fileutil.Exists(templatesDir) {
		t.Fatalf("expected templates directory at %s", templatesDir)
	}

	chartBytes, err := os.ReadFile(chartYamlPath)
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
		t.Fatalf("expected package-helm-chart.sh at %s", packageScript)
	}

	tmpDir := t.TempDir()
	cmd := exec.Command("bash", packageScript, "v1.5.0", tmpDir)
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

	// 1. Missing verify target argument
	cmdNoArg := exec.Command("bash", packageScript, "--verify")
	if err := cmdNoArg.Run(); err == nil {
		t.Errorf("expected --verify without argument to exit with error")
	}

	// 2. Nonexistent archive
	cmdNonexistent := exec.Command("bash", packageScript, "--verify", "/nonexistent/chart.tgz")
	if err := cmdNonexistent.Run(); err == nil {
		t.Errorf("expected --verify on nonexistent file to exit with error")
	}

	// 3. Corrupt archive
	tmpDir := t.TempDir()
	corruptTgz := filepath.Join(tmpDir, "corrupt.tgz")
	if err := os.WriteFile(corruptTgz, []byte("invalid gzip stream content"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write corrupt archive: %v", err)
	}
	cmdCorrupt := exec.Command("bash", packageScript, "--verify", corruptTgz)
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

	cmdMissingChart := exec.Command("bash", packageScript, "--verify", emptyTgz)
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
	packageBytes, err := os.ReadFile(packageScript)
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
	workflowBytes, err := os.ReadFile(workflowPath)
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
	if out, err := exec.Command("bash", genScript, "v2.0.0", tmpDir).CombinedOutput(); err != nil {
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
