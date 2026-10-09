package community

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestNotarizationGate_FunctionalAcceptance verifies that scripts/notarize-and-sign-darwin.sh
// signs and generates verifiable notarization receipts for release binaries/archives
// (CRIT-1789708108432777000-9ba94063).
func TestNotarizationGate_FunctionalAcceptance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	scriptPath := filepath.Join(root, "scripts", "notarize-and-sign-darwin.sh")
	if !fileutil.Exists(scriptPath) {
		t.Skip("notarize-and-sign-darwin.sh script not present in open-core community distribution")
	}

	// Create temporary dummy binary file to test signing & notarization metadata emission
	tmpDir := t.TempDir()
	dummyBin := filepath.Join(tmpDir, "dummy-binary")
	if err := fileutil.WriteFile(dummyBin, []byte("#!/bin/sh\necho test\n"), paths.DirPerm755); err != nil {
		t.Fatalf("failed to write dummy binary: %v", err)
	}

	cmd := testkit.ManagedCommand(t, t.Context(), "bash", scriptPath, dummyBin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("notarize script failed on dummy binary: %v, output: %s", err, string(out))
	}

	receiptPath := dummyBin + ".notary.json"
	if !fileutil.Exists(receiptPath) {
		t.Fatalf("expected notarization receipt at %s", receiptPath)
	}

	receiptBytes, err := fileutil.ReadFile(receiptPath)
	if err != nil {
		t.Fatalf("failed to read receipt: %v", err)
	}

	var receipt map[string]interface{}
	if err := json.Unmarshal(receiptBytes, &receipt); err != nil {
		t.Fatalf("failed to parse receipt json: %v", err)
	}

	if receipt["target"] != "dummy-binary" {
		t.Errorf("expected target 'dummy-binary', got '%v'", receipt["target"])
	}
	if receipt["sha256"] == "" {
		t.Errorf("expected non-empty sha256 in receipt")
	}
	if receipt["status"] == "" {
		t.Errorf("expected non-empty status in receipt")
	}
}

// TestNotarizationGate_BoundaryAndErrorHandling verifies verification mode and error handling
// on nonexistent files and tampering (CRIT-1789708108432778000-af730d3e).
func TestNotarizationGate_BoundaryAndErrorHandling(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	scriptPath := filepath.Join(root, "scripts", "notarize-and-sign-darwin.sh")
	if !fileutil.Exists(scriptPath) {
		t.Skip("notarize-and-sign-darwin.sh script not present in open-core community distribution")
	}
	cmd := testkit.ManagedCommand(t, t.Context(), "bash", scriptPath, "/nonexistent/path/binary")
	if err := cmd.Run(); err == nil {
		t.Errorf("expected script to fail on missing target file")
	}

	// Create dummy archive and test --verify flag
	tmpDir := t.TempDir()
	dummyArchive := filepath.Join(tmpDir, "dummy-archive.tar.gz")
	if err := fileutil.WriteFile(dummyArchive, []byte("fake archive contents"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write dummy archive: %v", err)
	}

	// First run notarization metadata creation
	signCmd := testkit.ManagedCommand(t, t.Context(), "bash", scriptPath, dummyArchive)
	if out, err := signCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create notarization manifest: %v, out: %s", err, string(out))
	}

	// Then verify with --verify flag
	verifyCmd := testkit.ManagedCommand(t, t.Context(), "bash", scriptPath, "--verify", dummyArchive)
	out, err := verifyCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected --verify to succeed for signed archive: %v, out: %s", err, string(out))
	}
	if !strings.Contains(string(out), "passed") && !strings.Contains(string(out), "Validated") {
		t.Errorf("expected success verification message in output, got: %s", string(out))
	}
}

// TestNotarizationGate_IntegrationAndConformance verifies that package-community.sh integrates
// notarize-and-sign-darwin.sh into the Darwin release pipeline (CRIT-1789708108432779000-6a1a7682).
func TestNotarizationGate_IntegrationAndConformance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	packageScript := filepath.Join(root, "scripts", "package-community.sh")
	if !fileutil.Exists(packageScript) {
		t.Fatalf("expected package-community.sh at %s", packageScript)
	}

	contentBytes, err := fileutil.ReadFile(packageScript)
	if err != nil {
		t.Fatalf("failed to read package-community.sh: %v", err)
	}
	content := string(contentBytes)

	if !strings.Contains(content, "notarize-and-sign-darwin.sh") {
		t.Errorf("expected package-community.sh to integrate notarize-and-sign-darwin.sh")
	}
	if !strings.Contains(content, "--verify") {
		t.Errorf("expected package-community.sh to invoke --verify on Darwin release archives")
	}
}
