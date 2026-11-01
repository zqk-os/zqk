package detector

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestDetectorSubsystem_ExtendedCoverage(t *testing.T) {
	tempDir := t.TempDir()

	// 1. findGoMod from current directory
	gmPath := findGoMod()
	if gmPath == "" {
		t.Log("findGoMod returned empty, which is possible in some test environments")
	} else {
		if !filepath.IsAbs(gmPath) {
			t.Errorf("expected absolute path from findGoMod, got %s", gmPath)
		}
	}

	// 2. LoadBinaryManifest failure modes
	nonExistentManifest := filepath.Join(tempDir, "missing-manifest.yaml")
	if _, err := LoadBinaryManifest(nonExistentManifest); err == nil {
		t.Error("expected error loading non-existent manifest")
	}

	invalidYamlManifest := filepath.Join(tempDir, "invalid-manifest.yaml")
	if err := fileutil.WriteFile(invalidYamlManifest, []byte(":: invalid yaml ::"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write invalid yaml: %v", err)
	}
	if _, err := LoadBinaryManifest(invalidYamlManifest); err == nil {
		t.Error("expected error loading invalid yaml manifest")
	}

	// 3. SaveBinaryManifest failure mode: place a file where directory should be
	blockerFile := filepath.Join(tempDir, "blocker-file")
	if err := fileutil.WriteFile(blockerFile, []byte("file"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	invalidSavePath := filepath.Join(blockerFile, "sub", "manifest.yaml")
	if err := SaveBinaryManifest(&BinaryManifest{Version: "1.0.0"}, invalidSavePath); err == nil {
		t.Error("expected error saving manifest into file path")
	}

	// 4. UpdateManifestAfterInstall failure modes
	fakeBin := filepath.Join(tempDir, "fake-bin")
	if err := fileutil.WriteFile(fakeBin, []byte("#!/bin/sh\necho test\n"), paths.FilePerm755); err != nil {
		t.Fatalf("failed to write fake bin: %v", err)
	}
	// Manifest path in directory blocked by file
	if err := UpdateManifestAfterInstall(fakeBin, "1.0.0", invalidSavePath); err == nil {
		t.Error("expected error updating manifest in invalid save path")
	}

	// 5. Signature verification branches
	d := NewBinaryDetector()

	// LookPath path for GetPath and IsAvailable
	shDetector := &BinaryDetector{
		binaryName:          "sh",
		requireVerification: false,
	}
	if !shDetector.IsAvailable() {
		t.Error("expected sh to be available without verification")
	}
	if p, err := shDetector.GetPath(); err != nil || p == "" {
		t.Errorf("expected GetPath to find sh, got %q, err: %v", p, err)
	}

	// Empty output for GetVersion to trigger could not parse version
	emptyVerScript := filepath.Join(tempDir, "empty-ver")
	if err := fileutil.WriteFile(emptyVerScript, []byte("#!/bin/sh\necho ''\n"), paths.FilePerm755); err != nil {
		t.Fatal(err)
	}
	if _, err := d.GetVersion(emptyVerScript); err == nil {
		t.Error("expected error for empty version output")
	}

	// On macOS, test verifyMacOSSignature on a system binary
	if _, err := exec.LookPath("codesign"); err == nil {
		shPath, _ := exec.LookPath("sh")
		if shPath != "" {
			_ = d.verifyMacOSSignature(shPath)
			trustedShDetector := &BinaryDetector{
				trustedSigners: []string{"Software Signing", "Apple"},
			}
			_ = trustedShDetector.verifyMacOSSignature(shPath)
		}
	}

	// Linux signature with existing .sig file but invalid gpg output
	fakeSig := fakeBin + ".sig"
	if err := fileutil.WriteFile(fakeSig, []byte("dummy signature content"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write fake sig: %v", err)
	}
	// verifyLinuxSignature when .sig exists
	errSig := d.verifyLinuxSignature(fakeBin)
	if errSig == nil {
		t.Error("expected error verifying dummy signature")
	}

	// Non-existent binary for signatures
	missingBin := filepath.Join(tempDir, "missing-bin")
	_ = d.verifyMacOSSignature(missingBin)
	_ = d.verifyWindowsSignature(missingBin)

	// verifyTrustedGPGKey with invalid fingerprint strings
	_ = d.verifyTrustedGPGKey("random output with no key")
	_ = d.verifyTrustedGPGKey("Primary key fingerprint:")

	// GetBinaryHash with missing file
	if _, err := GetBinaryHash(missingBin); err == nil {
		t.Error("expected error for missing binary hash")
	}

	// VerifyBinaryIntegrity with missing file
	if err := VerifyBinaryIntegrity(missingBin, "somehash"); err == nil {
		t.Error("expected error verifying integrity of missing binary")
	}

	// 6. Manifest parsing & module version
	validManifestPath := filepath.Join(tempDir, "valid-manifest.yaml")
	manifest := &BinaryManifest{
		Version:   "1.0.0",
		Algorithm: "sha256",
		Compatibility: &CompatibilityConstraints{
			CLIVersion: ">=1.0.0",
			Modules: map[string]string{
				"github.com/foo/bar": ">=1.0.0",
			},
			Backends: map[string]string{
				"badger": ">=1.0.0",
			},
		},
		Entries: []BinaryManifestEntry{
			{
				Binary:   "zqk-migrate",
				Hash:     "dummy-hash",
				Version:  "1.0.0",
				Platform: "darwin/arm64",
			},
		},
	}
	if err := SaveBinaryManifest(manifest, validManifestPath); err != nil {
		t.Fatalf("failed to save valid manifest: %v", err)
	}
	loadedManifest, err := LoadBinaryManifest(validManifestPath)
	if err != nil {
		t.Fatalf("failed to load manifest: %v", err)
	}
	if loadedManifest.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", loadedManifest.Version)
	}

	// parseModuleVersionFromGoModContent variations
	contentWithComment := []byte("// comment\nmodule test\nrequire github.com/test/mod v1.2.3 // indirect\n")
	v, err := parseModuleVersionFromGoModContent(contentWithComment, "github.com/test/mod")
	if err != nil || v != "1.2.3" {
		t.Errorf("expected 1.2.3, got %q, err: %v", v, err)
	}

	contentNotFound := []byte("module test\nrequire github.com/other/mod v1.0.0\n")
	_, err = parseModuleVersionFromGoModContent(contentNotFound, "github.com/notfound/mod")
	if err == nil {
		t.Error("expected error for missing module in content")
	}

	// Test findGoMod fallback by creating a temp directory structure
	subDir := filepath.Join(tempDir, "a", "b", "c")
	if err := fileutil.MkdirAll(subDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	tempGoMod := filepath.Join(tempDir, "a", "go.mod")
	if err := fileutil.WriteFile(tempGoMod, []byte("module subtest\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	_ = os.Chdir(subDir)
	foundGm := findGoMod()
	_ = os.Chdir(cwd)
	if foundGm == "" {
		t.Log("findGoMod in subDir did not find parent go.mod")
	}
}
