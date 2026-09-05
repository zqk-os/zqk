package supply_test

import (
	"strings"
	"testing"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestSupplyChainReleaseIntegrity verifies that release configuration adheres to POL-WORKFLOW-CEF-RECO-COMPLETE-001
// for BLI-CEF-R8-P0-SUPPLY-REAL-001 (cosign signs, SPDX sbom provenance, install.sh verification, hermetic CI).
func TestSupplyChainReleaseIntegrity(t *testing.T) {
	// 1. Verify .goreleaser.yaml includes cosign signs and sboms (K:F-L-SUPPLY-RELEASE-002)
	goreleaserBytes, err := fileutil.ReadFile("../../.goreleaser.yaml")
	if err != nil {
		t.Fatalf("failed to read .goreleaser.yaml: %v", err)
	}
	goreleaserContent := string(goreleaserBytes)
	if !strings.Contains(goreleaserContent, "signs:") || !strings.Contains(goreleaserContent, "cosign") {
		t.Errorf(".goreleaser.yaml missing cosign signs block")
	}
	if !strings.Contains(goreleaserContent, "sboms:") || !strings.Contains(goreleaserContent, "spdx.json") {
		t.Errorf(".goreleaser.yaml missing SPDX sboms block")
	}

	// 2. Verify install.sh authenticates checksums.txt signature (K:F-L-SUPPLY-RELEASE-003)
	installBytes, err := fileutil.ReadFile("../../install.sh")
	if err != nil {
		t.Fatalf("failed to read install.sh: %v", err)
	}
	installContent := string(installBytes)
	if !strings.Contains(installContent, "checksums.txt.sig") || !strings.Contains(installContent, "cosign verify-blob") {
		t.Errorf("install.sh missing cosign signature verification for checksums.txt")
	}

	// 3. Verify .github/workflows/release.yml enforces hermetic CI release builds with cosign (L:F-SUP-01)
	releaseWorkflowBytes, err := fileutil.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatalf("failed to read .github/workflows/release.yml: %v", err)
	}
	releaseWorkflowContent := string(releaseWorkflowBytes)
	if !strings.Contains(releaseWorkflowContent, "sigstore/cosign-installer") {
		t.Errorf("release.yml missing sigstore/cosign-installer step")
	}
	if !strings.Contains(releaseWorkflowContent, "release --clean") {
		t.Errorf("release.yml missing goreleaser release --clean flag")
	}
}
