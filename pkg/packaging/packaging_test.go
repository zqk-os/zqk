package packaging

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAdoptionSprint3_Packaging_GenerateFormula(t *testing.T) {
	cfg := FormulaConfig{
		Version:      "1.24.0",
		DarwinArmSHA: "0589ee71ef9a17e4a3adc3e10611386f87157273700a51034dd24e75cd2cf236",
		DarwinAmdSHA: "648689d3e85e3e0e4eb359e8d5daa5f99dd3984e4d794ab069ab7c2bea5bae6f",
		LinuxArmSHA:  "bb26f20a9862df6ea817d98ac6229c930e4dc0177f2012e78df140f2ae0b0082",
		LinuxAmdSHA:  "850912f19c500e27ba3ea03fc2a967dfd77db84bd31750e4774ac72fdb487bea",
	}

	formula, err := GenerateHomebrewFormula(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(formula, `version "1.24.0"`) {
		t.Errorf("formula missing expected version")
	}
	if !strings.Contains(formula, cfg.DarwinArmSHA) {
		t.Errorf("formula missing darwin arm sha")
	}
	if !strings.Contains(formula, cfg.LinuxArmSHA) {
		t.Errorf("formula missing linux arm sha")
	}

	// Empty version boundary
	_, err = GenerateHomebrewFormula(FormulaConfig{Version: ""})
	if err == nil {
		t.Errorf("expected error for empty version, got nil")
	}
}

func TestAdoptionSprint3_Packaging_ChecksumVerification(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test-binary.tar.gz")
	content := []byte("zqk distribution archive content")

	if err := fileutil.WriteFile(filePath, content, paths.FilePerm644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	sha, err := ComputeSHA256(filePath)
	if err != nil {
		t.Fatalf("failed to compute sha: %v", err)
	}

	if sha == "" {
		t.Fatalf("sha should not be empty")
	}

	// Successful verification
	if err := VerifyChecksum(filePath, sha); err != nil {
		t.Fatalf("expected valid checksum verification, got: %v", err)
	}

	// Mismatched checksum boundary
	if err := VerifyChecksum(filePath, "0000000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Fatalf("expected checksum mismatch error, got nil")
	}
}
