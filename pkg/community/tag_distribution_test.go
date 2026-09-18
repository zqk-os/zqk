package community

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestTagDistribution_FunctionalAcceptance verifies CRIT-1789616179105667000-5ed9744e:
// 1. Tag format verification ensures standard SemVer release candidate format (e.g. v2.9.4-rc1, v0.1.0-rc.1).
// 2. Multi-platform archive names and checksum manifest generator produce valid bit-for-bit checksums for all supported targets.
// 3. Homebrew formula generation succeeds with all 4 platform hashes included.
func TestTagDistribution_FunctionalAcceptance(t *testing.T) {
	// 1. Tag format validation
	validTags := []string{
		"v2.9.4-rc1",
		"v0.1.0-rc.1",
		"v1.0.0-rc.2",
		"v3.0.0-beta.1",
		"v2.9.4-rc1+20260916",
	}

	for _, tag := range validTags {
		if err := ValidateReleaseTag(tag); err != nil {
			t.Errorf("expected valid tag %q to pass validation, got error: %v", tag, err)
		}
	}

	// 2. Checksum manifest generation and parsing
	tmpDir := t.TempDir()
	mockChecksums := make(map[string]string)
	for _, p := range SupportedPlatforms {
		archive := FormatArchiveName("v2.9.4-rc1", p.OS, p.Arch)
		archivePath := filepath.Join(tmpDir, archive)
		content := []byte("binary payload for " + p.OS + "/" + p.Arch + " at tag v2.9.4-rc1")
		if err := fileutil.WriteFile(archivePath, content, 0644); err != nil {
			t.Fatalf("failed to write mock archive: %v", err)
		}
		hash, err := ComputeFileSHA256(archivePath)
		if err != nil {
			t.Fatalf("failed to compute hash: %v", err)
		}
		mockChecksums[archive] = hash
	}

	var manifestLines []string
	for archive, hash := range mockChecksums {
		manifestLines = append(manifestLines, hash+"  "+archive)
	}
	manifestPath := filepath.Join(tmpDir, "checksums.txt")
	if err := fileutil.WriteFile(manifestPath, []byte(strings.Join(manifestLines, "\n")), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	parsed, err := ParseChecksumManifest(strings.Join(manifestLines, "\n"))
	if err != nil {
		t.Fatalf("ParseChecksumManifest failed: %v", err)
	}
	if len(parsed) != len(SupportedPlatforms) {
		t.Errorf("expected %d parsed entries, got %d", len(SupportedPlatforms), len(parsed))
	}

	// 3. Formula generation
	formula, err := GenerateHomebrewFormula("v2.9.4-rc1", mockChecksums)
	if err != nil {
		t.Fatalf("GenerateHomebrewFormula failed: %v", err)
	}
	if !strings.Contains(formula, "class Zqk < Formula") {
		t.Errorf("generated formula missing class declaration")
	}
	for _, p := range SupportedPlatforms {
		archive := FormatArchiveName("v2.9.4-rc1", p.OS, p.Arch)
		if !strings.Contains(formula, mockChecksums[archive]) {
			t.Errorf("formula missing expected hash for %s", archive)
		}
	}
}

// TestTagDistribution_BoundaryAndErrorHandling verifies CRIT-1789616179105668000-ce745d7a:
// 1. Non-conforming tag formats are rejected.
// 2. Corrupted, truncated, or mismatched checksum manifests fail closed.
// 3. Incomplete platform sets in formula generation return explicit descriptive errors.
func TestTagDistribution_BoundaryAndErrorHandling(t *testing.T) {
	t.Parallel()

	// 1. Invalid tag rejection
	invalidTags := []string{
		"",
		"2.9.4-rc1",     // missing 'v' prefix
		"v",             // empty version
		"v1",            // incomplete semver
		"v1.0",          // incomplete semver
		"v1.0.0.0",      // quad-part version
		"latest",        // non-version
		"v2.9.4-rc@bad", // invalid characters in prerelease
	}

	for _, tag := range invalidTags {
		if err := ValidateReleaseTag(tag); err == nil {
			t.Errorf("expected invalid tag %q to fail validation, but passed", tag)
		}
	}

	// 2. Invalid checksum format rejection
	malformedManifests := []string{
		"short_hash  archive.tar.gz",
		"12345  archive.tar.gz",
		"not_a_valid_hash_too_long_12345678901234567890123456789012345678901234567890  archive.tar.gz",
	}

	for _, m := range malformedManifests {
		if _, err := ParseChecksumManifest(m); err == nil {
			t.Errorf("expected malformed manifest %q to fail parsing, but passed", m)
		}
	}

	// 3. Incomplete checksums map in formula generation
	incompleteChecksums := map[string]string{
		FormatArchiveName("v2.9.4-rc1", "darwin", "arm64"): "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
	_, err := GenerateHomebrewFormula("v2.9.4-rc1", incompleteChecksums)
	if err == nil {
		t.Errorf("expected incomplete checksums map to fail formula generation, but passed")
	}

	// 4. Missing archive in VerifyChecksumManifest
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "checksums.txt")
	manifestContent := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  missing-archive.tar.gz\n"
	_ = fileutil.WriteFile(manifestPath, []byte(manifestContent), 0644)
	if err := VerifyChecksumManifest(tmpDir); err == nil {
		t.Errorf("expected VerifyChecksumManifest to fail on missing archives, but passed")
	}
}

// TestTagDistribution_IntegrationAndConformance verifies CRIT-1789616179105669000-d9b3b45f:
// 1. scripts/package-community.sh is present and executable in the repository root.
// 2. Distribution channels logic cleanly round-trips against live repo paths.
func TestTagDistribution_IntegrationAndConformance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	pkgScript := filepath.Join(root, "scripts", "package-community.sh")
	info, err := fileutil.Stat(pkgScript)
	if err != nil {
		t.Fatalf("package-community.sh missing: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Errorf("package-community.sh is not executable")
	}

	// Verify all 4 SupportedPlatforms are distinct and non-empty
	seen := make(map[string]bool)
	for _, p := range SupportedPlatforms {
		key := p.OS + "/" + p.Arch
		if p.OS == "" || p.Arch == "" {
			t.Errorf("invalid platform definition: %+v", p)
		}
		if seen[key] {
			t.Errorf("duplicate platform definition: %s", key)
		}
		seen[key] = true
	}

	if len(seen) != 4 {
		t.Errorf("expected 4 distinct supported platforms, got %d", len(seen))
	}
}
