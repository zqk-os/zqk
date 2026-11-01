package packaging

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// FormulaConfig specifies release artifacts for Homebrew formula generation.
type FormulaConfig struct {
	Version      string
	DarwinArmSHA string
	DarwinAmdSHA string
	LinuxArmSHA  string
	LinuxAmdSHA  string
}

// GenerateHomebrewFormula renders a canonical Homebrew ruby formula for zqk.
func GenerateHomebrewFormula(cfg FormulaConfig) (string, error) {
	if strings.TrimSpace(cfg.Version) == "" {
		return "", errfmt.Errorf("formula version cannot be empty")
	}

	tmpl := fmt.Sprintf(`class Zqk < Formula
  desc "Knowledge Kernel & Distributed Autonomous Swarm Orchestration Engine"
  homepage "https://github.com/zqk-os/zqk"
  version "%s"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk-community_#{version}_darwin_arm64.tar.gz"
      sha256 "%s"
    else
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk-community_#{version}_darwin_amd64.tar.gz"
      sha256 "%s"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk-community_#{version}_linux_arm64.tar.gz"
      sha256 "%s"
    else
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk-community_#{version}_linux_amd64.tar.gz"
      sha256 "%s"
    end
  end

  def install
    if File.exist?("zqk")
      bin.install "zqk"
    else
      bin.install "zqk-community" => "zqk"
    end
  end

  test do
    system "#{bin}/zqk", "--help"
  end
end
`, cfg.Version, cfg.DarwinArmSHA, cfg.DarwinAmdSHA, cfg.LinuxArmSHA, cfg.LinuxAmdSHA)

	return tmpl, nil
}

// ComputeSHA256 computes the hex-encoded SHA-256 checksum of a file.
func ComputeSHA256(filePath string) (sha string, err error) {
	f, err := fileutil.Open(filePath)
	if err != nil {
		return "", errfmt.Errorf("failed to open file for checksum computation %s: %w", filePath, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", errfmt.Errorf("failed to compute checksum for %s: %w", filePath, err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// VerifyChecksum verifies that a file matches the expected hex-encoded SHA-256 digest.
func VerifyChecksum(filePath, expectedSHA string) error {
	actual, err := ComputeSHA256(filePath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(actual, strings.TrimSpace(expectedSHA)) {
		return errfmt.Errorf("checksum mismatch for %s: expected %s, got %s", filePath, expectedSHA, actual)
	}
	return nil
}
