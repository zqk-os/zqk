package community

import (
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
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}
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

	indexContentBytes, err := fileutil.ReadFile(indexPath)
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
	searchBytes, err := fileutil.ReadFile(searchPath)
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
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}
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

	fi, err := fileutil.Stat(scriptPath)
	if err != nil {
		t.Fatalf("failed to stat generate-docs-portal.sh: %v", err)
	}
	if fi.Mode()&0111 == 0 {
		t.Errorf("expected generate-docs-portal.sh to be executable")
	}

	pyScript := filepath.Join(root, "scripts", "open-core", "docs-portal", "generate_docs_portal.py")
	contentBytes, err := fileutil.ReadFile(pyScript)
	if err != nil {
		t.Fatalf("failed to read generate_docs_portal.py: %v", err)
	}
	content := string(contentBytes)
	if !strings.Contains(content, "def build_portal") || !strings.Contains(content, "def verify_tarball") {
		t.Errorf("expected build_portal and verify_tarball in generate_docs_portal.py")
	}
}

// TestDocsPortal_DeployDryRunAndCoherency verifies that deploy-docs-portal.sh
// compiles clean HTML pages with CNAME and .nojekyll markers (CRIT-DOC-DEPLOY-001, CRIT-WEB-COHERENCY-001).
func TestDocsPortal_DeployDryRunAndCoherency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}
	root := paths.ResolveProjectRoot(".")
	deployScript := filepath.Join(root, "scripts", "open-core", "docs-portal", "deploy-docs-portal.sh")

	fi, err := fileutil.Stat(deployScript)
	if err != nil {
		t.Fatalf("failed to stat deploy-docs-portal.sh: %v", err)
	}
	if fi.Mode()&0111 == 0 {
		t.Errorf("expected deploy-docs-portal.sh to be executable")
	}

	cmd := testkit.ManagedCommand(t, t.Context(), "bash", deployScript, "--dry-run")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("deploy-docs-portal.sh --dry-run failed: %v, output: %s", err, string(out))
	}
	outputStr := string(out)
	if !strings.Contains(outputStr, "Compiled") || !strings.Contains(outputStr, "HTML documentation pages") {
		t.Errorf("expected compiled html documentation pages in output, got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "Portal verified at /tmp/zqk-docs-build") {
		t.Errorf("expected portal verification in output, got: %s", outputStr)
	}

	cnamePath := filepath.Join("/tmp", "zqk-docs-build", "CNAME")
	cnameBytes, err := fileutil.ReadFile(cnamePath)
	if err != nil {
		t.Fatalf("failed to read CNAME marker: %v", err)
	}
	if !strings.Contains(string(cnameBytes), "docs.zqk.dev") {
		t.Errorf("expected docs.zqk.dev in CNAME, got: %s", string(cnameBytes))
	}

	nojekyllPath := filepath.Join("/tmp", "zqk-docs-build", ".nojekyll")
	if !fileutil.Exists(nojekyllPath) {
		t.Errorf("expected .nojekyll marker at %s", nojekyllPath)
	}
}

// TestDocsPortal_CIInvariantAndLinkRotSentinel verifies that the documentation portal
// dry-run build succeeds and internal link validation passes with zero broken links
// (CRIT-1794319905719999000-fdfb2782).
func TestDocsPortal_CIInvariantAndLinkRotSentinel(t *testing.T) {
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
		t.Fatalf("check-doc-links.sh failed: %v, output:\n%s", err, string(out))
	}
	outputStr := string(out)
	if !strings.Contains(outputStr, "Validation passed: All internal links resolve correctly.") {
		t.Errorf("expected validation success message, got: %s", outputStr)
	}
}

// TestDocsPortal_BrokenLinkDetection_NegativeBoundary verifies that check-doc-links.sh
// fails closed and detects deliberate broken markdown links (CRIT-1794319905720000000-adbe923f).
func TestDocsPortal_BrokenLinkDetection_NegativeBoundary(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	checkScript := filepath.Join(root, "scripts", "open-core", "check-doc-links.sh")

	// Create a temporary dummy doc file under docs/ with a broken link
	testDocPath := filepath.Join(root, "docs", "TMP_SENTINEL_NEGATIVE_TEST.md")
	defer func() {
		_ = fileutil.Remove(testDocPath)
	}()

	brokenContent := "# Temporary Sentinel Test\n\n[Broken Link](./nonexistent_ghost_file_xyz_123.md)\n"
	if err := fileutil.WriteDurableFile(testDocPath, []byte(brokenContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	cmd := testkit.ManagedCommand(t, t.Context(), "bash", checkScript)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected check-doc-links.sh to fail on broken link, but exited 0. Output:\n%s", string(out))
	}
	outputStr := string(out)
	if !strings.Contains(outputStr, "Broken link found in ./docs/TMP_SENTINEL_NEGATIVE_TEST.md") {
		t.Errorf("expected broken link identification in output, got: %s", outputStr)
	}
}

// TestDocsPortal_WorkflowAndArchitectureConformance verifies that CI workflow configuration
// and generator scripts mandate documentation portal verification on PR gates
// (CRIT-1794319905720001000-2a8b4721).
func TestDocsPortal_WorkflowAndArchitectureConformance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	ciPath := filepath.Join(root, ".github", "workflows", "ci.yml")
	ciBytes, err := fileutil.ReadFile(ciPath)
	if err != nil {
		t.Fatalf("failed to read .github/workflows/ci.yml: %v", err)
	}
	ciContent := string(ciBytes)
	if !strings.Contains(ciContent, "deploy-docs-portal.sh --dry-run") {
		t.Errorf("expected deploy-docs-portal.sh --dry-run in ci.yml")
	}
	if !strings.Contains(ciContent, "check-doc-links.sh") {
		t.Errorf("expected check-doc-links.sh in ci.yml")
	}

	deployScript := filepath.Join(root, "scripts", "open-core", "docs-portal", "deploy-docs-portal.sh")
	deployFi, err := fileutil.Stat(deployScript)
	if err != nil {
		t.Fatalf("failed to stat deploy-docs-portal.sh: %v", err)
	}
	if deployFi.Mode()&0111 == 0 {
		t.Errorf("expected deploy-docs-portal.sh to have executable permissions")
	}

	checkScript := filepath.Join(root, "scripts", "open-core", "check-doc-links.sh")
	checkFi, err := fileutil.Stat(checkScript)
	if err != nil {
		t.Fatalf("failed to stat check-doc-links.sh: %v", err)
	}
	if checkFi.Mode()&0111 == 0 {
		t.Errorf("expected check-doc-links.sh to have executable permissions")
	}
}
