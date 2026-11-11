package community

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestLinkSentinel_StaticFloorInvariant verifies that all documentation under docs/
// contains zero leaked developer workstation paths or raw file:// schemes, and that
// the root install.sh is a standalone regular file matching scripts/install.sh byte-for-byte.
// Satisfies CRIT-1794355003653283000-cc6a6502.
func TestLinkSentinel_StaticFloorInvariant(t *testing.T) {
	root := paths.ResolveProjectRoot(".")

	// 1. Verify root install.sh is a regular executable file (not a symlink)
	rootInstallPath := filepath.Join(root, "install.sh")
	scriptsInstallPath := filepath.Join(root, "scripts", "install.sh")

	fi, err := os.Lstat(rootInstallPath)
	if err != nil {
		t.Fatalf("failed to stat root install.sh: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("install.sh at repo root must be a regular file, not a symlink (breaks raw.githubusercontent.com curls)")
	}
	if fi.Mode().Perm()&0111 == 0 {
		t.Errorf("install.sh must be executable, got perm: %v", fi.Mode().Perm())
	}

	rootContent, err := fileutil.ReadFile(rootInstallPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", rootInstallPath, err)
	}
	scriptsContent, err := fileutil.ReadFile(scriptsInstallPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", scriptsInstallPath, err)
	}
	if !bytes.Equal(rootContent, scriptsContent) {
		t.Errorf("root install.sh content does not match scripts/install.sh")
	}

	// 2. Scan all markdown files in docs/ for forbidden link targets and leaked usernames
	docsDir := filepath.Join(root, "docs")
	linkRe := regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)
	var leakedLinks []string
	var leakedWorkstations []string

	err = filepath.WalkDir(docsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		content, readErr := fileutil.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		s := string(content)

		// Check for specific developer username leakage anywhere in doc
		if strings.Contains(s, "/Users/lanceettl") || strings.Contains(s, "/home/lanceettl") {
			leakedWorkstations = append(leakedWorkstations, path)
		}

		// Check all extracted link targets
		matches := linkRe.FindAllStringSubmatch(s, -1)
		for _, m := range matches {
			if len(m) < 2 {
				continue
			}
			target := strings.TrimSpace(m[1])
			if strings.HasPrefix(target, "file://") || strings.HasPrefix(target, "/Users/") || strings.HasPrefix(target, "/home/") {
				leakedLinks = append(leakedLinks, path+": "+target)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk failed: %v", err)
	}

	if len(leakedWorkstations) > 0 {
		t.Errorf("found %d documentation files with leaked developer workstation paths: %v", len(leakedWorkstations), leakedWorkstations)
	}
	if len(leakedLinks) > 0 {
		t.Errorf("found %d illegitimate link targets (file:// or workstation root): %v", len(leakedLinks), leakedLinks)
	}
}

// TestLinkSentinel_OperationalDynamicProof verifies that scripts/open-core/check-doc-links.sh
// passes cleanly across the entire repository with zero broken or illegitimate links.
// Satisfies CRIT-1794355003653284000-c9f8ed9c.
func TestLinkSentinel_OperationalDynamicProof(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}
	root := paths.ResolveProjectRoot(".")
	checkScript := filepath.Join(root, "scripts", "open-core", "check-doc-links.sh")
	if !fileutil.Exists(checkScript) {
		t.Fatalf("expected check-doc-links.sh at %s", checkScript)
	}

	cmd := testkit.ManagedCommand(t, t.Context(), "bash", checkScript)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("check-doc-links.sh failed (exit %v): %s", err, string(out))
	}
	if !strings.Contains(string(out), "Validation passed") {
		t.Errorf("expected 'Validation passed' in output, got: %s", string(out))
	}
}

// TestLinkSentinel_NegativeAdversarialBoundary verifies that the link sentinel
// detects and fails-closed when encountering illegitimate workstation paths or file:// schemes.
// Satisfies CRIT-1794355003653285000-c0fc2e27.
func TestLinkSentinel_NegativeAdversarialBoundary(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	checkScript := filepath.Join(root, "scripts", "open-core", "check-doc-links.sh")

	tmpDir := t.TempDir()
	badDoc := filepath.Join(tmpDir, "ADVERSARIAL_TEST.md")
	badContent := "# Adversarial Document\n\n[Leaked File](file:///Users/victim/secrets.yaml)\n[Home Directory](/Users/developer/code)\n"
	if err := fileutil.WriteFile(badDoc, []byte(badContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Run check-doc-links.sh with custom directory scope
	cmd := testkit.ManagedCommand(t, t.Context(), "bash", checkScript)
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected check-doc-links.sh to fail on adversarial workstation paths, but it passed: %s", string(out))
	}
	outStr := string(out)
	if !strings.Contains(outStr, "Illegitimate workstation path or file:// URI found") {
		t.Errorf("expected failure message 'Illegitimate workstation path or file:// URI found', got: %s", outStr)
	}
}
