package community

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestDistributionChannels_FunctionalAcceptance verifies that package-community.sh
// generates cross-platform release archives, sha256 checksums, and a matching Homebrew formula (CRIT-1789615270860117000-06b5c31a).
func TestDistributionChannels_FunctionalAcceptance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-platform package test in short mode")
	}

	root := paths.ResolveProjectRoot(".")
	pkgScript := filepath.Join(root, "scripts", "package-community.sh")
	if !fileutil.Exists(pkgScript) {
		t.Fatalf("package-community.sh not found at %s", pkgScript)
	}

	distDir := t.TempDir()
	cmd := execwrap.Command("bash", pkgScript, "v2.9.4", "--dry")
	cmd.Dir = root
	cmd.Env = append(cmd.Environ(), "ZQK_DIST_DIR="+distDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("package-community.sh --dry failed: %v\nOutput:\n%s", err, string(out))
	}

	// 1. Verify all 4 platform tarballs exist
	for _, p := range SupportedPlatforms {
		archive := FormatArchiveName("2.9.4", p.OS, p.Arch)
		archivePath := filepath.Join(distDir, archive)
		if !fileutil.Exists(archivePath) {
			t.Errorf("expected release archive %s not found", archive)
		}
	}

	// 2. Verify checksum manifest exists and matches
	manifestPath := filepath.Join(distDir, "checksums.txt")
	if !fileutil.Exists(manifestPath) {
		t.Fatalf("checksums.txt not found in %s", distDir)
	}
	if err := VerifyChecksumManifest(distDir); err != nil {
		t.Errorf("VerifyChecksumManifest failed: %v", err)
	}

	// 3. Verify Homebrew formula generated in distDir
	formulaPath := filepath.Join(distDir, "Formula", "zqk.rb")
	if !fileutil.Exists(formulaPath) {
		t.Fatalf("Homebrew formula not found at %s", formulaPath)
	}
	if err := VerifyHomebrewFormula(formulaPath, manifestPath); err != nil {
		t.Errorf("VerifyHomebrewFormula failed: %v", err)
	}
}

// TestDistributionChannels_BoundaryAndErrorHandling verifies corrupted asset rejection,
// checksum mismatch detection, and formula validation error handling (CRIT-1789615270860118000-fbcc198b).
func TestDistributionChannels_BoundaryAndErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Create mock archives
	mockChecksums := make(map[string]string)
	for _, p := range SupportedPlatforms {
		archive := FormatArchiveName("2.9.4", p.OS, p.Arch)
		archivePath := filepath.Join(tmpDir, archive)
		content := []byte("mock binary archive content for " + p.OS + "/" + p.Arch)
		if err := fileutil.WriteFile(archivePath, content, 0644); err != nil {
			t.Fatalf("failed to write mock archive: %v", err)
		}
		hash, err := ComputeFileSHA256(archivePath)
		if err != nil {
			t.Fatalf("failed to compute hash: %v", err)
		}
		mockChecksums[archive] = hash
	}

	// 2. Write valid checksums.txt
	var manifestLines []string
	for archive, hash := range mockChecksums {
		manifestLines = append(manifestLines, hash+"  "+archive)
	}
	manifestPath := filepath.Join(tmpDir, "checksums.txt")
	if err := fileutil.WriteFile(manifestPath, []byte(strings.Join(manifestLines, "\n")), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	// Verify valid state passes
	if err := VerifyChecksumManifest(tmpDir); err != nil {
		t.Fatalf("expected valid manifest to pass, got: %v", err)
	}

	// 3. Test corrupted archive detection
	firstArchive := FormatArchiveName("2.9.4", SupportedPlatforms[0].OS, SupportedPlatforms[0].Arch)
	firstArchivePath := filepath.Join(tmpDir, firstArchive)
	if err := fileutil.WriteFile(firstArchivePath, []byte("tampered content"), 0644); err != nil {
		t.Fatalf("failed to corrupt archive: %v", err)
	}
	if err := VerifyChecksumManifest(tmpDir); err == nil {
		t.Errorf("expected checksum mismatch error for corrupted archive, but got nil")
	}

	// 4. Test missing archive detection
	if err := fileutil.Remove(firstArchivePath); err != nil {
		t.Fatalf("failed to remove archive: %v", err)
	}
	if err := VerifyChecksumManifest(tmpDir); err == nil {
		t.Errorf("expected error for missing archive, but got nil")
	}

	// 5. Test formula mismatch detection
	formulaContent, err := GenerateHomebrewFormula("2.9.4", mockChecksums)
	if err != nil {
		t.Fatalf("GenerateHomebrewFormula failed: %v", err)
	}
	formulaPath := filepath.Join(tmpDir, "zqk.rb")
	if err := fileutil.WriteFile(formulaPath, []byte(formulaContent), 0644); err != nil {
		t.Fatalf("failed to write formula: %v", err)
	}

	// Corrupt formula sha256
	corruptedFormula := strings.Replace(formulaContent, mockChecksums[FormatArchiveName("2.9.4", "darwin", "arm64")], "0000000000000000000000000000000000000000000000000000000000000000", 1)
	corruptedFormulaPath := filepath.Join(tmpDir, "zqk_corrupt.rb")
	if err := fileutil.WriteFile(corruptedFormulaPath, []byte(corruptedFormula), 0644); err != nil {
		t.Fatalf("failed to write corrupted formula: %v", err)
	}
	if err := VerifyHomebrewFormula(corruptedFormulaPath, manifestPath); err == nil {
		t.Errorf("expected error when formula sha256 does not match manifest, got nil")
	}
}

// TestDistributionChannels_IntegrationAndConformance verifies Homebrew formula syntax,
// repo-level formula integrity, and release asset contract conformance (CRIT-1789615270860119000-d8fa7d23).
func TestDistributionChannels_IntegrationAndConformance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")

	// Verify repo-level Formula/zqk.rb exists and has valid class definition
	repoFormula := filepath.Join(root, "Formula", "zqk.rb")
	if !fileutil.Exists(repoFormula) {
		t.Fatalf("Formula/zqk.rb not found at %s", repoFormula)
	}

	bytes, err := fileutil.ReadFile(repoFormula)
	if err != nil {
		t.Fatalf("failed to read Formula/zqk.rb: %v", err)
	}
	content := string(bytes)

	requiredTokens := []string{
		"class Zqk < Formula",
		"homepage \"https://github.com/zqk-os/zqk\"",
		"on_macos do",
		"on_linux do",
		"Hardware::CPU.arm?",
		"bin.install \"zqk\"",
		"system \"#{bin}/zqk\"",
	}

	for _, token := range requiredTokens {
		if !strings.Contains(content, token) {
			t.Errorf("Formula/zqk.rb missing required contract token: %q", token)
		}
	}
}
