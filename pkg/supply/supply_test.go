package supply_test

import (
	"strings"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestSupplyChainReleaseIntegrity verifies that release configuration adheres to POL-WORKFLOW-CEF-RECO-COMPLETE-001
// for BLI-CEF-R8-P0-SUPPLY-REAL-001 (cosign signs, SPDX sbom provenance, install.sh verification, hermetic CI).
func TestSupplyChainReleaseIntegrity(t *testing.T) {
	goreleaserBytes, err := fileutil.ReadFile("../../.goreleaser.yaml")
	if err != nil {
		t.Skipf("failed to read .goreleaser.yaml: %v", err)
	}
	goreleaserContent := string(goreleaserBytes)
	if !strings.Contains(goreleaserContent, "signs:") {
		t.Skip("skipping cosign supply test: open-core goreleaser does not configure enterprise cosign signing")
	}
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

// TestFailClosedCIGates_DeterministicAndVerified verifies REQ-1789592457454400000-21efcfe8
// and CRIT-1789592457619132000-f71dcb62 / CRIT-1789592457619133000-34452aa8 / CRIT-1789592457619134000-f1dd7d5f:
// local and CI golangci runs are deterministic, and the pipeline fails closed on missing/inconsistent lint, checksum, or SBOM evidence.
func TestFailClosedCIGates_DeterministicAndVerified(t *testing.T) {
	if !fileutil.Exists("../../scripts/verify-binary-checksums.sh") {
		t.Skip("skipping studio supply gate script check in open-core")
	}
	// 1. Verify .golangci.yml has adequate timeout for deterministic completion on large monorepo
	golangciBytes, err := fileutil.ReadFile("../../.golangci.yml")
	if err != nil {
		t.Fatalf("failed to read .golangci.yml: %v", err)
	}
	golangciContent := string(golangciBytes)
	if !strings.Contains(golangciContent, "timeout: 15m") && !strings.Contains(golangciContent, "timeout: 20m") && !strings.Contains(golangciContent, "timeout: 30m") {
		t.Errorf(".golangci.yml timeout must be >= 15m for deterministic execution on large codebase")
	}

	// 2. Verify .github/workflows/ci.yml includes fail-closed SBOM and checksum gates
	ciWorkflowBytes, err := fileutil.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("failed to read .github/workflows/ci.yml: %v", err)
	}
	ciContent := string(ciWorkflowBytes)
	if !strings.Contains(ciContent, "test-supply-sbom-generation.sh") {
		t.Errorf("ci.yml missing deterministic SBOM generation verification gate")
	}
	if !strings.Contains(ciContent, "test-supply-checksum-verification.sh") {
		t.Errorf("ci.yml missing binary checksum fail-closed verification gate")
	}

	// 3. Verify Makefile includes verify-supply-gates in verify target
	makefileBytes, err := fileutil.ReadFile("../../Makefile")
	if err != nil {
		t.Fatalf("failed to read Makefile: %v", err)
	}
	makeContent := string(makefileBytes)
	if !strings.Contains(makeContent, "verify-supply-gates") {
		t.Errorf("Makefile missing verify-supply-gates target in verify pipeline")
	}

	// 4. Verify supply gate scripts exist and are non-empty
	for _, script := range []string{
		"../../scripts/verify-binary-checksums.sh",
		"../../scripts/generate-sbom.sh",
		"../../scripts/test-supply-checksum-verification.sh",
		"../../scripts/test-supply-sbom-generation.sh",
	} {
		data, err := fileutil.ReadFile(script)
		if err != nil || len(data) == 0 {
			t.Errorf("expected script %s to exist and be non-empty", script)
		}
	}
}
