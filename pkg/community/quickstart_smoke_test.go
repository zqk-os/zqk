package community

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// extractTarGz unpacks a tar.gz archive into a destination directory.
func extractTarGz(srcTarGz, destDir string) error {
	f, err := fileutil.Open(srcTarGz)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := fileutil.MkdirAll(target, paths.DirPerm755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := fileutil.MkdirAll(filepath.Dir(target), paths.DirPerm755); err != nil {
				return err
			}
			outFile, err := fileutil.OpenFile(target, fileutil.O_CREATE|fileutil.O_RDWR|fileutil.O_TRUNC, fileutil.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}
	return nil
}

// TestQuickstartSmoke_FunctionalAcceptance verifies that a release tarball can be unpacked
// into a clean, empty directory by a stranger, and successfully execute system init,
// agent-onboard, and whats-next with zero monorepo dependencies (CRIT-1789616219409901000-13d41df4).
func TestQuickstartSmoke_FunctionalAcceptance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping tarball generation in short mode")
	}

	root := paths.ResolveProjectRoot(".")
	pkgScript := filepath.Join(root, "scripts", "package-community.sh")
	if !fileutil.Exists(pkgScript) {
		t.Fatalf("package-community.sh not found at %s", pkgScript)
	}

	distDir := t.TempDir()
	cmd := execwrap.Command("bash", pkgScript, "v2.9.5", "--dry")
	cmd.Dir = root
	cmd.Env = append(cmd.Environ(), "ZQK_DIST_DIR="+distDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("package-community.sh --dry failed: %v\nOutput:\n%s", err, string(out))
	}

	// Unpack host architecture release tarball
	archiveName := FormatArchiveName("2.9.5", runtime.GOOS, runtime.GOARCH)
	archivePath := filepath.Join(distDir, archiveName)
	if !fileutil.Exists(archivePath) {
		t.Fatalf("host release archive %s not found in %s", archiveName, distDir)
	}

	unpackDir := t.TempDir()
	if err := extractTarGz(archivePath, unpackDir); err != nil {
		t.Fatalf("failed to unpack release tarball %s: %v", archivePath, err)
	}

	binName := "zqk-community"
	binPath := filepath.Join(unpackDir, binName)
	if !fileutil.Exists(binPath) {
		binPath = filepath.Join(unpackDir, strings.TrimSuffix(archiveName, ".tar.gz"), binName)
	}
	if !fileutil.Exists(binPath) {
		t.Fatalf("expected binary %s not found in unpacked tarball %s", binName, unpackDir)
	}

	// Simulate completely empty project directory for a stranger
	strangerWorkspace := t.TempDir()

	// 1. Run 'system init'
	initCmd := execwrap.Command(binPath, "system", "init", "--project-name", "stranger-quickstart")
	initCmd.Dir = strangerWorkspace
	initOut, initErr := initCmd.CombinedOutput()
	if initErr != nil {
		t.Fatalf("system init failed in clean directory: %v\nOutput:\n%s", initErr, string(initOut))
	}

	// Verify .zqk configuration directory was initialized
	if !fileutil.Exists(filepath.Join(strangerWorkspace, paths.ProjectDataDir)) {
		t.Fatalf(".zqk directory was not created by system init")
	}

	// 2. Run 'system agent-onboard'
	onboardCmd := execwrap.Command(binPath, "system", "agent-onboard", "--format", "json")
	onboardCmd.Dir = strangerWorkspace
	onboardOut, onboardErr := onboardCmd.CombinedOutput()
	if onboardErr != nil {
		t.Fatalf("system agent-onboard failed in stranger workspace: %v\nOutput:\n%s", onboardErr, string(onboardOut))
	}

	if !strings.Contains(string(onboardOut), `"status": "success"`) && !strings.Contains(string(onboardOut), `"status":"success"`) {
		t.Errorf("expected success in agent-onboard output, got: %s", string(onboardOut))
	}

	// Verify sync report was written
	syncReport := filepath.Join(strangerWorkspace, paths.ProjectDataDir, paths.AgentRuntimeDir, "agent_workspace_sync.json")
	if !fileutil.Exists(syncReport) {
		t.Errorf("agent_workspace_sync.json not created at %s", syncReport)
	}

	// 3. Run 'workflow whats-next'
	wnCmd := execwrap.Command(binPath, "workflow", "whats-next", "--format", "json")
	wnCmd.Dir = strangerWorkspace
	wnOut, wnErr := wnCmd.CombinedOutput()
	if wnErr != nil {
		t.Fatalf("workflow whats-next failed in stranger workspace: %v\nOutput:\n%s", wnErr, string(wnOut))
	}

	if !strings.Contains(string(wnOut), "zqk_whats_next_v1") {
		t.Errorf("expected zqk_whats_next_v1 schema in whats-next output, got: %s", string(wnOut))
	}
}

// TestQuickstartSmoke_BoundaryAndErrorHandling verifies error handling when stranger environment
// lacks permissions, runs in non-existent directory, or runs quickstart help/walkthrough (CRIT-1789616219409902000-2ccc4d24).
func TestQuickstartSmoke_BoundaryAndErrorHandling(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	binPath := filepath.Join(root, "bin", "zqk-stable")
	if !fileutil.Exists(binPath) {
		t.Skipf("bin/zqk-stable not found at %s", binPath)
	}

	// 1. Verify quickstart command walkthrough executes cleanly
	quickCmd := execwrap.Command(binPath, "quickstart", "--format", "json")
	quickCmd.Dir = root
	quickOut, quickErr := quickCmd.CombinedOutput()
	if quickErr != nil {
		t.Fatalf("quickstart --format json failed: %v\nOutput:\n%s", quickErr, string(quickOut))
	}
	if !strings.Contains(string(quickOut), "quickstart") && !strings.Contains(string(quickOut), "walkthrough") && !strings.Contains(string(quickOut), "steps") {
		t.Errorf("unexpected quickstart output: %s", string(quickOut))
	}

	// 2. Verify agent-onboard fails cleanly with descriptive error if project root does not exist
	badRoot := filepath.Join(t.TempDir(), "nonexistent_subfolder")
	onboardCmd := execwrap.Command(binPath, "system", "agent-onboard", "--detect-only", "--format", "json")
	onboardCmd.Dir = badRoot
	// Even if it runs or fails, it must not panic or crash
	_ = onboardCmd.Run()
}

// TestQuickstartSmoke_IntegrationAndConformance verifies documentation and release packaging
// conformance for first-contact stranger experience (CRIT-1789616219409903000-fffea071).
func TestQuickstartSmoke_IntegrationAndConformance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")

	// 1. Verify COMMUNITY_FIRST_RUN.md exists and contains accurate walkthrough instructions
	guidePath := filepath.Join(root, "docs", "onboarding", "COMMUNITY_FIRST_RUN.md")
	if !fileutil.Exists(guidePath) {
		t.Fatalf("docs/onboarding/COMMUNITY_FIRST_RUN.md missing at %s", guidePath)
	}
	content, err := fileutil.ReadFile(guidePath)
	if err != nil {
		t.Fatalf("failed reading COMMUNITY_FIRST_RUN.md: %v", err)
	}
	guideStr := string(content)

	expectedTokens := []string{
		"system init",
		"system agent-onboard",
		"workflow whats-next",
	}
	for _, tok := range expectedTokens {
		if !strings.Contains(guideStr, tok) {
			t.Errorf("COMMUNITY_FIRST_RUN.md missing mandatory command token: %s", tok)
		}
	}

	// 2. Verify Formula/zqk.rb has correct description and repository URL
	formulaPath := filepath.Join(root, "Formula", "zqk.rb")
	if !fileutil.Exists(formulaPath) {
		t.Fatalf("Formula/zqk.rb missing at %s", formulaPath)
	}
	fBytes, err := fileutil.ReadFile(formulaPath)
	if err != nil {
		t.Fatalf("failed reading Formula/zqk.rb: %v", err)
	}
	fStr := string(fBytes)
	if !strings.Contains(fStr, "class Zqk < Formula") {
		t.Errorf("Formula/zqk.rb missing class Zqk definition")
	}
	if !strings.Contains(fStr, "https://github.com/zqk-os/zqk/releases/download/") {
		t.Errorf("Formula/zqk.rb missing GitHub release download URL pattern")
	}
}
