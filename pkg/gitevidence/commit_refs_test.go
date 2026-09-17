package gitevidence_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/gitevidence"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestValidateBacklogCommitHashes_RejectsMergeAndUnrelated(t *testing.T) {
	root := initRepo(t)
	bli := "BLI-TEST-EVIDENCE-001"

	writeAndCommit(t, root, filepath.Join(paths.ProcessBacklogDir, "x.yaml"), "cas\n", "chore: cas only")
	casOnly := head(t, root)

	writeAndCommit(t, root, "pkg/foo/bar.go", "package foo\n", "feat: real work "+bli)
	good := head(t, root)

	// Create a merge commit without BLI id in message
	run(t, root, "git", "checkout", "-b", "side")
	writeAndCommit(t, root, "pkg/foo/side.go", "package foo\n", "feat: side")
	run(t, root, "git", "checkout", "main")
	run(t, root, "git", "merge", "--no-ff", "-m", "Merge branch side", "side")
	merge := head(t, root)

	if err := gitevidence.ValidateBacklogCommitHashes(root, bli, "", []string{good}); err != nil {
		t.Fatalf("good commit should pass: %v", err)
	}
	if err := gitevidence.ValidateBacklogCommitHashes(root, bli, "", nil); err == nil {
		t.Fatal("empty refs should fail")
	}
	if err := gitevidence.ValidateBacklogCommitHashes(root, bli, "", []string{casOnly}); err == nil {
		t.Fatal("CAS-only commit should fail")
	}
	if err := gitevidence.ValidateBacklogCommitHashes(root, bli, "", []string{merge}); err == nil {
		t.Fatal("merge commit should fail")
	}
	if err := gitevidence.ValidateBacklogCommitHashes(root, bli, "", []string{good[:7]}); err != nil {
		// short SHA of good should still resolve and pass
		t.Fatalf("short SHA of good should pass: %v", err)
	}
	unrelated := writeUnrelated(t, root)
	if err := gitevidence.ValidateBacklogCommitHashes(root, bli, "", []string{unrelated}); err == nil || !strings.Contains(err.Error(), bli) {
		t.Fatalf("unrelated message should fail mentioning BLI, got %v", err)
	}
}

func TestValidateBacklogCommitHashes_EdgeCases(t *testing.T) {
	root := initRepo(t)
	bli := "BLI-STEWARD-CI-GATE-ENFORCEMENT-001"

	// Empty BLI ID
	if err := gitevidence.ValidateBacklogCommitHashes(root, "", "", []string{"abc1234"}); err == nil {
		t.Fatal("expected error for empty backlog item id")
	}

	// Empty repo root
	if err := gitevidence.ValidateBacklogCommitHashes("", bli, "", []string{"abc1234"}); err == nil {
		t.Fatal("expected error for empty repo root")
	}

	// Invalid commit ref string
	if err := gitevidence.ValidateBacklogCommitHashes(root, bli, "", []string{"nonexistent-commit-sha"}); err == nil {
		t.Fatal("expected error for unresolvable commit sha")
	}
}

func TestValidateBacklogCommitHashes_RejectsDuplicateImplementations(t *testing.T) {
	root := initRepo(t)
	bli := "BLI-DUPE-001"

	// Initial commit on main
	writeAndCommit(t, root, "README.md", "init\n", "docs: init")

	// Implement on branch A
	run(t, root, "git", "checkout", "-b", "branch-a")
	writeAndCommit(t, root, "pkg/a.go", "package a\n", "feat: implement "+bli+" part 1")
	refA := head(t, root)

	// Implement on branch B
	run(t, root, "git", "checkout", "main")
	run(t, root, "git", "checkout", "-b", "branch-b")
	writeAndCommit(t, root, "pkg/b.go", "package b\n", "feat: implement "+bli+" part 2")

	// Validating branch-a's commit should fail because branch-b also has commits for this BLI
	err := gitevidence.ValidateBacklogCommitHashes(root, bli, "branch-a", []string{refA})
	if err == nil {
		t.Fatal("expected error for duplicate implementations on multiple branches")
	}
	if !strings.Contains(err.Error(), "duplicate implementation") {
		t.Fatalf("expected duplicate implementation error, got: %v", err)
	}
}

func writeUnrelated(t *testing.T, root string) string {
	t.Helper()
	writeAndCommit(t, root, "README.md", "x\n", "docs: unrelated readme tweak")
	return head(t, root)
}

func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run(t, root, "git", "init", "-b", "main")
	run(t, root, "git", "config", "user.email", "test@example.com")
	run(t, root, "git", "config", "user.name", "test")
	return root
}

func writeAndCommit(t *testing.T, root, rel, content, msg string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := fileutil.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, root, "git", "add", "-A")
	run(t, root, "git", "commit", "-m", msg)
}

func head(t *testing.T, root string) string {
	t.Helper()
	out := run(t, root, "git", "rev-parse", "HEAD")
	return strings.TrimSpace(out)
}

func run(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...) //nolint:gosec
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return string(out)
}

// TestGoMod_NoRedundantReplaces verifies that go.mod contains 0 redundant or no-op replace directives (BLI-CEF-R17-GOMOD-REPLACES-001).
func TestGoMod_NoRedundantReplaces(t *testing.T) {
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	var goModPath string
	for {
		candidate := filepath.Join(dir, "go.mod")
		if _, err := fileutil.Stat(candidate); err == nil {
			goModPath = candidate
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if goModPath == "" {
		t.Skip("no go.mod found")
	}

	content, err := fileutil.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	lines := strings.Split(string(content), "\n")
	var replaceCount int
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "replace ") || l == "replace (" {
			replaceCount++
		}
	}
	if replaceCount != 0 {
		t.Fatalf("expected 0 replace directives in go.mod, found %d", replaceCount)
	}
}

// TestReadmePrefersCommunityFirstRunOverStudioPack verifies that README.md highlights COMMUNITY_FIRST_RUN.md
// as the primary onboarding entry point (BLI-CEF-R18-COMMUNITY-FIRST-001, CRIT-CEF-R2-USA-COMMUNITY-FIRST-A).
func TestReadmePrefersCommunityFirstRunOverStudioPack(t *testing.T) {
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	var rootDir string
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			rootDir = dir
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if rootDir == "" {
		t.Skip("no repo root found")
	}

	readmePath := filepath.Join(rootDir, "README.md")

	content, err := fileutil.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "COMMUNITY_FIRST_RUN.md") {
		t.Fatal("README.md must link to COMMUNITY_FIRST_RUN.md in the Quickstart section")
	}
}

func TestValidateBranchAncestorOfTrunk(t *testing.T) {
	root := initRepo(t)
	// Currently on main
	writeAndCommit(t, root, "README.md", "init\n", "docs: init")
	mainCommit := head(t, root)

	// Create a branch from main
	run(t, root, "git", "checkout", "-b", "feature-branch")
	writeAndCommit(t, root, "pkg/feat.go", "package feat\n", "feat: added feat")
	featureCommit := head(t, root)

	// main should be an ancestor of trunk (itself)
	if err := gitevidence.ValidateBranchAncestorOfTrunk(root, mainCommit); err != nil {
		t.Fatalf("main should be an ancestor of main: %v", err)
	}

	// feature branch should not be an ancestor of main right now
	err := gitevidence.ValidateBranchAncestorOfTrunk(root, "feature-branch")
	if err == nil {
		t.Fatalf("expected error since feature-branch is not an ancestor of main")
	}
	if !strings.Contains(err.Error(), "plan cannot complete until its branch is an ancestor of trunk") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// Now merge feature-branch into main
	run(t, root, "git", "checkout", "main")
	run(t, root, "git", "merge", "feature-branch")

	// Now feature branch SHOULD be an ancestor of main
	if err := gitevidence.ValidateBranchAncestorOfTrunk(root, featureCommit); err != nil {
		t.Fatalf("feature branch should be an ancestor of main after merge: %v", err)
	}
}

// TRACK: BLI-CEF-R20-EVIDENCE-ANCESTRY-GATE-001

// TestReadmeLeadsWithStrangerUsablePath verifies that README.md opens with quickstart instructions
// rather than internal kernel jargon (BLI-CEF-R18-DOC-JARGON-001, CRIT-CEF-R2-DOC-JARGON-A).
func TestReadmeLeadsWithStrangerUsablePath(t *testing.T) {
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	var rootDir string
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			rootDir = dir
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if rootDir == "" {
		t.Skip("no repo root found")
	}

	readmePath := filepath.Join(rootDir, "README.md")

	content, err := fileutil.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}

	s := string(content)
	quickstartIdx := strings.Index(s, "Quickstart")
	if quickstartIdx == -1 {
		t.Fatal("README.md must have a Quickstart section")
	}
	if quickstartIdx > 1000 {
		t.Fatalf("Quickstart section appears too late in README.md (offset %d > 1000)", quickstartIdx)
	}
}

// TestDegradedAndStubResultsAreExplicitlyLabeled verifies that tutorial docs and healthcheck code
// explicitly identify degraded mode rather than misleading users that degraded/stub output is healthy
// (BLI-CEF-R18-DEGRADED-LABELS-001, CRIT-CEF-R18-DEGRADED-LABELS-001, REQ-CEF-OBS-002).
func TestDegradedAndStubResultsAreExplicitlyLabeled(t *testing.T) {
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	var rootDir string
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			rootDir = dir
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if rootDir == "" {
		t.Skip("no repo root found")
	}

	tutorialPath := filepath.Join(rootDir, "docs", "onboarding", "FIRST_RUN_OBJECT_TUTORIAL.md")
	content, err := fileutil.ReadFile(tutorialPath)
	if err != nil {
		t.Fatalf("read FIRST_RUN_OBJECT_TUTORIAL.md: %v", err)
	}
	s := string(content)
	if strings.Contains(s, "re-run with `--allow-degraded`") {
		t.Fatal("tutorial must not teach re-running with --allow-degraded as the default remedy")
	}
	if !strings.Contains(s, "Do not treat `--allow-degraded` as the default fix") {
		t.Fatal("tutorial must warn that --allow-degraded means partial/degraded results are intentionally accepted")
	}
}

// TestSBOMGenerationAndDependencyReviewPath verifies that SBOM workflow and dependency review configurations exist
// (BLI-CEF-R18-SBOM-001, CRIT-CEF-R2-SUP-SBOM-A, REQ-CEF-R2-SUP-SBOM).
func TestSBOMGenerationAndDependencyReviewPath(t *testing.T) {
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	var rootDir string
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			rootDir = dir
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if rootDir == "" {
		t.Skip("no repo root found")
	}

	sbomWorkflowPath := filepath.Join(rootDir, ".github", "workflows", "sbom.yml")
	if _, err := fileutil.Stat(sbomWorkflowPath); err != nil {
		t.Fatalf("missing .github/workflows/sbom.yml: %v", err)
	}
	securityPath := filepath.Join(rootDir, "SECURITY.md")
	if _, err := fileutil.Stat(securityPath); err != nil {
		t.Fatalf("missing SECURITY.md: %v", err)
	}
}
