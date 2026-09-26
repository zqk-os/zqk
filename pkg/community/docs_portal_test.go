package community

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestDocsPortal_FunctionalAcceptance verifies that scripts/generate-docs-portal.sh
// generates static HTML portal pages and offline lunr-compatible search indexes
// (CRIT-1789708521365161000-969fc6a6).
func TestDocsPortal_FunctionalAcceptance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	scriptPath := filepath.Join(root, "scripts", "open-core", "docs-portal", "generate-docs-portal.sh")
	if !fileutil.Exists(scriptPath) {
		t.Fatalf("expected generate-docs-portal.sh script at %s", scriptPath)
	}

	pyScriptPath := filepath.Join(root, "scripts", "open-core", "docs-portal", "generate_docs_portal.py")
	if !fileutil.Exists(pyScriptPath) {
		t.Fatalf("expected generate_docs_portal.py script at %s", pyScriptPath)
	}

	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "dist-docs-test")

	cmd := testkit.ManagedCommand(t, t.Context(), "bash", scriptPath, outDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generate-docs-portal.sh failed: %v, output: %s", err, string(out))
	}

	// Verify index.html generated
	indexPath := filepath.Join(outDir, "index.html")
	if !fileutil.Exists(indexPath) {
		t.Fatalf("expected index.html at %s", indexPath)
	}

	indexContentBytes, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}
	indexContent := string(indexContentBytes)
	if !strings.Contains(indexContent, "ZQK Community Docs") {
		t.Errorf("expected header 'ZQK Community Docs' in index.html")
	}

	// Verify style.css
	stylePath := filepath.Join(outDir, "assets", "style.css")
	if !fileutil.Exists(stylePath) {
		t.Fatalf("expected style.css at %s", stylePath)
	}

	// Verify search-index.js
	searchPath := filepath.Join(outDir, "search", "search-index.js")
	if !fileutil.Exists(searchPath) {
		t.Fatalf("expected search-index.js at %s", searchPath)
	}
	searchBytes, err := os.ReadFile(searchPath)
	if err != nil {
		t.Fatalf("failed to read search-index.js: %v", err)
	}
	if !strings.Contains(string(searchBytes), "docsIndex") {
		t.Errorf("expected docsIndex array in search-index.js")
	}
}

// TestDocsPortal_BoundaryAndErrorHandling verifies archive generation, verification mode,
// and error handling for missing or invalid inputs (CRIT-1789708521365162000-933653ab).
func TestDocsPortal_BoundaryAndErrorHandling(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	scriptPath := filepath.Join(root, "scripts", "open-core", "docs-portal", "generate-docs-portal.sh")

	// Verification mode failure on nonexistent tarball
	cmd := testkit.ManagedCommand(t, t.Context(), "bash", scriptPath, "--verify", "/nonexistent/path/docs.tar.gz")
	if err := cmd.Run(); err == nil {
		t.Errorf("expected failure when verifying nonexistent tarball")
	}

	// Build into temp directory and verify created tarball
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "dist-docs-verify")
	tarballPath := outDir + ".tar.gz"

	buildCmd := testkit.ManagedCommand(t, t.Context(), "bash", scriptPath, outDir)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v, out: %s", err, string(out))
	}

	if !fileutil.Exists(tarballPath) {
		t.Fatalf("expected tarball at %s", tarballPath)
	}

	// Verify tarball passes verification check
	verifyCmd := testkit.ManagedCommand(t, t.Context(), "bash", scriptPath, "--verify", tarballPath)
	out, err := verifyCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tarball verification failed: %v, out: %s", err, string(out))
	}
	if !strings.Contains(string(out), "valid structure") {
		t.Errorf("expected 'valid structure' in verification output, got: %s", string(out))
	}
}

// TestDocsPortal_IntegrationAndConformance verifies that release workflows and packaging
// can invoke documentation portal generation (CRIT-1789708521365163000-559def00).
func TestDocsPortal_IntegrationAndConformance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	scriptPath := filepath.Join(root, "scripts", "open-core", "docs-portal", "generate-docs-portal.sh")

	fi, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("failed to stat generate-docs-portal.sh: %v", err)
	}
	if fi.Mode()&0111 == 0 {
		t.Errorf("expected generate-docs-portal.sh to be executable")
	}

	pyScript := filepath.Join(root, "scripts", "open-core", "docs-portal", "generate_docs_portal.py")
	contentBytes, err := os.ReadFile(pyScript)
	if err != nil {
		t.Fatalf("failed to read generate_docs_portal.py: %v", err)
	}
	content := string(contentBytes)
	if !strings.Contains(content, "def build_portal") || !strings.Contains(content, "def verify_tarball") {
		t.Errorf("expected build_portal and verify_tarball in generate_docs_portal.py")
	}
}
