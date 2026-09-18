package community

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TargetPlatform defines an operating system and architecture pair.
type TargetPlatform struct {
	OS   string
	Arch string
}

// SupportedPlatforms lists the standard 4 cross-compilation targets.
var SupportedPlatforms = []TargetPlatform{
	{OS: "darwin", Arch: "amd64"},
	{OS: "darwin", Arch: "arm64"},
	{OS: "linux", Arch: "amd64"},
	{OS: "linux", Arch: "arm64"},
}

// FormatArchiveName returns the canonical tarball name for a platform and version.
func FormatArchiveName(version, goos, goarch string) string {
	verNum := strings.TrimPrefix(version, "v")
	return fmt.Sprintf("zqk_%s_%s_%s.tar.gz", verNum, goos, goarch)
}

// ParseChecksumManifest parses a standard sha256 checksums.txt file content.
// Returns a map from archive filename to hex-encoded SHA256 string.
func ParseChecksumManifest(content string) (map[string]string, error) {
	result := make(map[string]string)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		hash := strings.ToLower(fields[0])
		filename := filepath.Base(fields[1])
		if len(hash) != 64 {
			return nil, fmt.Errorf("invalid sha256 hash length (%d) for %s: %s", len(hash), filename, hash)
		}
		result[filename] = hash
	}
	return result, nil
}

// GenerateHomebrewFormula renders an automated, valid Ruby Homebrew formula.
func GenerateHomebrewFormula(version string, checksums map[string]string) (string, error) {
	verNum := strings.TrimPrefix(version, "v")

	darwinAmd64 := checksums[FormatArchiveName(verNum, "darwin", "amd64")]
	darwinArm64 := checksums[FormatArchiveName(verNum, "darwin", "arm64")]
	linuxAmd64 := checksums[FormatArchiveName(verNum, "linux", "amd64")]
	linuxArm64 := checksums[FormatArchiveName(verNum, "linux", "arm64")]

	if darwinAmd64 == "" || darwinArm64 == "" || linuxAmd64 == "" || linuxArm64 == "" {
		return "", fmt.Errorf("incomplete checksums map: missing platform sha256 entries")
	}

	tmpl := fmt.Sprintf(`class Zqk < Formula
  desc "Kernel and orchestration CLI for AI-human hybrid software engineering"
  homepage "https://github.com/zqk-os/zqk"
  version "%s"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk_#{version}_darwin_arm64.tar.gz"
      sha256 "%s"
    else
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk_#{version}_darwin_amd64.tar.gz"
      sha256 "%s"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk_#{version}_linux_arm64.tar.gz"
      sha256 "%s"
    else
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk_#{version}_linux_amd64.tar.gz"
      sha256 "%s"
    end
  end

  def install
    bin.install "zqk"
  end

  test do
    system "#{bin}/zqk", "--help"
  end
end
`, verNum, darwinArm64, darwinAmd64, linuxArm64, linuxAmd64)

	return tmpl, nil
}

// ComputeFileSHA256 returns the lowercase hex sha256 of the given file path.
func ComputeFileSHA256(filePath string) (string, error) {
	f, err := fileutil.OpenRead(filePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// VerifyChecksumManifest verifies all archives listed in checksums.txt exist in distDir and match hashes.
func VerifyChecksumManifest(distDir string) error {
	manifestPath := filepath.Join(distDir, "checksums.txt")
	content, err := fileutil.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read checksum manifest: %w", err)
	}

	checksums, err := ParseChecksumManifest(string(content))
	if err != nil {
		return fmt.Errorf("failed to parse checksum manifest: %w", err)
	}

	if len(checksums) < len(SupportedPlatforms) {
		return fmt.Errorf("insufficient checksum entries: got %d, expected at least %d", len(checksums), len(SupportedPlatforms))
	}

	for filename, expectedHash := range checksums {
		archivePath := filepath.Join(distDir, filename)
		actualHash, err := ComputeFileSHA256(archivePath)
		if err != nil {
			return fmt.Errorf("failed to hash archive %s: %w", filename, err)
		}
		if !strings.EqualFold(actualHash, expectedHash) {
			return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", filename, expectedHash, actualHash)
		}
	}

	return nil
}

// VerifyHomebrewFormula verifies that formula at formulaPath matches the checksums in manifestPath.
func VerifyHomebrewFormula(formulaPath, manifestPath string) error {
	manifestBytes, err := fileutil.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read checksum manifest: %w", err)
	}
	checksums, err := ParseChecksumManifest(string(manifestBytes))
	if err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}

	formulaBytes, err := fileutil.ReadFile(formulaPath)
	if err != nil {
		return fmt.Errorf("failed to read formula: %w", err)
	}
	formulaContent := string(formulaBytes)

	// Verify all platform hashes appear in formula
	for _, p := range SupportedPlatforms {
		archive := FormatArchiveName("", p.OS, p.Arch)
		// Find matching archive in checksums
		var foundHash string
		for name, h := range checksums {
			if strings.Contains(name, p.OS) && strings.Contains(name, p.Arch) {
				foundHash = h
				break
			}
		}
		if foundHash == "" {
			return fmt.Errorf("no checksum entry found for platform %s/%s (%s)", p.OS, p.Arch, archive)
		}
		if !strings.Contains(formulaContent, foundHash) {
			return fmt.Errorf("formula %s does not contain checksum %s for %s/%s", formulaPath, foundHash, p.OS, p.Arch)
		}
	}

	if !strings.Contains(formulaContent, "bin.install \"zqk\"") {
		return fmt.Errorf("formula missing binary installation for zqk")
	}

	return nil
}

var releaseTagRegex = regexp.MustCompile(`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

// ValidateReleaseTag asserts that a given release tag strictly conforms to standard SemVer with mandatory leading 'v'.
func ValidateReleaseTag(tag string) error {
	if tag == "" {
		return errors.New("release tag cannot be empty")
	}
	if !strings.HasPrefix(tag, "v") {
		return fmt.Errorf("release tag %q must start with 'v' prefix", tag)
	}
	if !releaseTagRegex.MatchString(tag) {
		return fmt.Errorf("release tag %q is not a valid semantic version tag (expected format e.g. v2.9.4-rc1)", tag)
	}
	return nil
}
