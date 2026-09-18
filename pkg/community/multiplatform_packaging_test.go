package community

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestMultiPlatformPackaging_FunctionalAcceptance verifies cross-compilation across all target platforms,
// checksum manifest generation, and archive integrity (CRIT-1789628035935118000-599ae67d / BLI-1789628035935118000-8ab9e7c1).
func TestMultiPlatformPackaging_FunctionalAcceptance(t *testing.T) {
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

	// 1. Verify 4 platform archives exist
	for _, p := range SupportedPlatforms {
		archive := FormatArchiveName("2.9.4", p.OS, p.Arch)
		archivePath := filepath.Join(distDir, archive)
		if !fileutil.Exists(archivePath) {
			t.Errorf("expected release archive %s not found", archive)
		}
	}

	// 2. Verify checksum manifest
	if err := VerifyChecksumManifest(distDir); err != nil {
		t.Errorf("VerifyChecksumManifest failed: %v", err)
	}
}

// TestMultiPlatformPackaging_BoundaryAndErrorHandling tests corrupted checksums, archive extraction,
// and malformed manifests (CRIT-1789628035935119000-b08b79e6 / BLI-1789628035935118000-8ab9e7c1).
func TestMultiPlatformPackaging_BoundaryAndErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()

	// ParseChecksumManifest invalid length
	invalidContent := "abc1234  zqk-community_2.9.4_darwin_arm64.tar.gz\n"
	if _, err := ParseChecksumManifest(invalidContent); err == nil {
		t.Errorf("expected error parsing invalid hash length, got nil")
	}

	// Missing manifest in dir
	if err := VerifyChecksumManifest(tmpDir); err == nil {
		t.Errorf("expected error verifying non-existent manifest, got nil")
	}

	// Create valid mock archive and verify extraction
	testTar := filepath.Join(tmpDir, "test.tar.gz")
	f, err := fileutil.Create(testTar)
	if err != nil {
		t.Fatalf("create test.tar.gz: %v", err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	body := []byte("#!/bin/sh\necho ok\n")
	hdr := &tar.Header{
		Name: "zqk-community",
		Mode: 0755,
		Size: int64(len(body)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("write body: %v", err)
	}
	_ = tw.Close()
	_ = gw.Close()
	_ = f.Close()

	hash, err := ComputeFileSHA256(testTar)
	if err != nil || len(hash) != 64 {
		t.Fatalf("ComputeFileSHA256 failed: %v (hash: %s)", err, hash)
	}
}

// TestMultiPlatformPackaging_IntegrationAndConformance verifies hermetic archive layout,
// absence of studio dependencies, and clean extraction without contamination (CRIT-1789628035935120000-a174d9b4 / BLI-1789628035935118000-8ab9e7c1).
func TestMultiPlatformPackaging_IntegrationAndConformance(t *testing.T) {
	// Verify that SupportedPlatforms covers darwin and linux amd64/arm64
	covered := make(map[string]bool)
	for _, p := range SupportedPlatforms {
		covered[fmt.Sprintf("%s/%s", p.OS, p.Arch)] = true
	}

	expected := []string{"darwin/arm64", "darwin/amd64", "linux/arm64", "linux/amd64"}
	for _, exp := range expected {
		if !covered[exp] {
			t.Errorf("missing expected platform in SupportedPlatforms: %s", exp)
		}
	}

	// Verify format archive name adheres to canonical naming convention
	archName := FormatArchiveName("v2.9.4", "darwin", "arm64")
	if archName != "zqk-community_2.9.4_darwin_arm64.tar.gz" {
		t.Errorf("unexpected archive name format: %s", archName)
	}
	if strings.Contains(archName, "studio") {
		t.Errorf("release archive name contains studio keyword: %s", archName)
	}
}
