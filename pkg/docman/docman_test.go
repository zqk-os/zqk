package docman

import (
	"github.com/zqk-os/zqk/pkg/paths"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDiscoverer_Discover(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	docsDir := filepath.Join(tmpDir, "docs")
	if err := os.MkdirAll(filepath.Join(docsDir, "sub"), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	doc1 := filepath.Join(docsDir, "guide.md")
	doc2 := filepath.Join(docsDir, "sub", "tutorial.md")
	txt := filepath.Join(docsDir, "notes.txt")

	if err := os.WriteFile(doc1, []byte("# Guide"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(doc2, []byte("# Tutorial"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(txt, []byte("plain text"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	disc := NewDiscoverer(tmpDir)
	files, err := disc.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 discovered markdown files, got %d", len(files))
	}

	foundGuide, foundTutorial := false, false
	for _, f := range files {
		if filepath.Base(f.Path) == "guide.md" {
			foundGuide = true
		}
		if filepath.Base(f.Path) == "tutorial.md" {
			foundTutorial = true
		}
	}

	if !foundGuide || !foundTutorial {
		t.Errorf("failed to discover expected files: guide=%v tutorial=%v", foundGuide, foundTutorial)
	}
}

func TestDiscoverer_NoDocsDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	disc := NewDiscoverer(tmpDir)
	files, err := disc.Discover()
	if err != nil {
		t.Fatalf("Discover on empty root failed: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files when docs dir missing, got %d", len(files))
	}
}

func TestDiscoverer_ExcludeArchiveDirs(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	docsDir := filepath.Join(tmpDir, "docs")
	archiveDir := filepath.Join(docsDir, "archive")
	underscoreArchiveDir := filepath.Join(docsDir, "_archive")
	nestedArchiveDir := filepath.Join(docsDir, "onboarding", "archive")

	for _, d := range []string{archiveDir, underscoreArchiveDir, nestedArchiveDir} {
		if err := os.MkdirAll(d, paths.DirPerm755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
	}

	liveDoc := filepath.Join(docsDir, "live.md")
	archiveDoc := filepath.Join(archiveDir, "old.md")
	underscoreDoc := filepath.Join(underscoreArchiveDir, "ancient.md")
	nestedDoc := filepath.Join(nestedArchiveDir, "summary.md")

	for _, f := range []string{liveDoc, archiveDoc, underscoreDoc, nestedDoc} {
		if err := os.WriteFile(f, []byte("# Note"), paths.FilePerm644); err != nil {
			t.Fatal(err)
		}
	}

	disc := NewDiscoverer(tmpDir)
	files, err := disc.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected exactly 1 discovered file, got %d", len(files))
	}
	if filepath.Base(files[0].Path) != "live.md" {
		t.Fatalf("expected live.md, got %s", files[0].Path)
	}
}

func TestDiscoverer_Subtrees(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create directory structure:
	// docs/architecture/arch.md
	// docs/architecture/_archive/ancient.md
	// docs/best-practices/bp.md
	// docs/onboarding/start.md
	// docs/onboarding/archive/old.md
	// docs/launch/launch.md
	// docs/internal/secret.md
	treeDirs := []string{
		filepath.Join(tmpDir, "docs", "architecture"),
		filepath.Join(tmpDir, "docs", "architecture", "_archive"),
		filepath.Join(tmpDir, "docs", "best-practices"),
		filepath.Join(tmpDir, "docs", "onboarding"),
		filepath.Join(tmpDir, "docs", "onboarding", "archive"),
		filepath.Join(tmpDir, "docs", "launch"),
		filepath.Join(tmpDir, "docs", "internal"),
	}
	for _, d := range treeDirs {
		if err := os.MkdirAll(d, paths.DirPerm755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
	}

	testFiles := map[string]bool{
		filepath.Join(tmpDir, "docs", "architecture", "arch.md"):            true,  // should be included
		filepath.Join(tmpDir, "docs", "architecture", "_archive", "old.md"): false, // archive excluded
		filepath.Join(tmpDir, "docs", "best-practices", "bp.md"):            true,  // should be included
		filepath.Join(tmpDir, "docs", "onboarding", "start.md"):             true,  // should be included
		filepath.Join(tmpDir, "docs", "onboarding", "archive", "old.md"):    false, // archive excluded
		filepath.Join(tmpDir, "docs", "launch", "launch.md"):                false, // not in shipped subtrees
		filepath.Join(tmpDir, "docs", "internal", "secret.md"):              false, // not in shipped subtrees
	}

	for f := range testFiles {
		if err := os.WriteFile(f, []byte("# Test Document"), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write %s: %v", f, err)
		}
	}

	disc := NewDiscovererWithSubtrees(tmpDir, ShippedInitDocSubtrees)
	files, err := disc.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("expected exactly 3 discovered files, got %d: %v", len(files), files)
	}

	for _, f := range files {
		expected, exists := testFiles[f.Path]
		if !exists || !expected {
			t.Errorf("unexpected file discovered: %s", f.RelPath)
		}
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate repo root containing go.mod")
		}
		dir = parent
	}
}

// TestDivioQuadrant_FunctionalAcceptance validates CRIT-1789619391641751000-b4da3bf0:
//  1. All four Divio quadrant directories exist and are non-empty:
//     Tutorials (docs/tutorials), How-To Guides (docs/howto), Reference (docs/manual or docs/reference), Explanation (docs/explanation).
//  2. docs/INDEX.md exists and contains valid links pointing to existing files on disk.
func TestDivioQuadrant_FunctionalAcceptance(t *testing.T) {
	repoRoot := findRepoRoot(t)
	docsDir := filepath.Join(repoRoot, "docs")

	quadrants := map[string][]string{
		"Tutorials":   {"tutorials/README.md"},
		"How-To":      {"howto/README.md", "howto/SCHEDULER_AND_MAINTENANCE.md"},
		"Reference":   {"manual/README.md"},
		"Explanation": {"explanation/README.md"},
	}

	for quadrant, files := range quadrants {
		for _, rel := range files {
			fullPath := filepath.Join(docsDir, rel)
			info, err := os.Stat(fullPath)
			if err != nil {
				t.Fatalf("[%s] required documentation file missing: %s (%v)", quadrant, rel, err)
			}
			if info.Size() == 0 {
				t.Errorf("[%s] documentation file %s is unexpectedly empty", quadrant, rel)
			}
		}
	}

	// Verify docs/INDEX.md links
	indexPath := filepath.Join(docsDir, "INDEX.md")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read docs/INDEX.md: %v", err)
	}

	linkRe := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	matches := linkRe.FindAllStringSubmatch(string(data), -1)
	if len(matches) == 0 {
		t.Fatalf("docs/INDEX.md contains no links")
	}

	validLinkCount := 0
	for _, m := range matches {
		target := m[2]
		// Skip external links or anchors
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "#") {
			continue
		}
		// Strip anchor if present
		cleanTarget := strings.Split(target, "#")[0]
		if cleanTarget == "" {
			continue
		}
		resolved := filepath.Join(docsDir, filepath.Clean(cleanTarget))
		if _, err := os.Stat(resolved); err != nil {
			t.Errorf("broken link in docs/INDEX.md: %s -> %s (resolved: %s)", m[0], target, resolved)
		} else {
			validLinkCount++
		}
	}

	if validLinkCount < 8 {
		t.Errorf("expected at least 8 valid internal documentation links in docs/INDEX.md, got %d", validLinkCount)
	}
}

// TestDivioQuadrant_BoundaryAndErrorHandling validates CRIT-1789619391641752000-2d2fea55:
// Verifies boundary behaviors, empty markdown file handling, missing headings, and non-existent paths.
func TestDivioQuadrant_BoundaryAndErrorHandling(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	parser := NewParser()

	// Case 1: Empty markdown file does not panic and returns non-empty fallback title
	emptyFile := filepath.Join(tmpDir, "empty-guide.md")
	if err := os.WriteFile(emptyFile, []byte(""), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	meta, err := parser.Parse(emptyFile)
	if err != nil {
		t.Fatalf("Parse on empty file failed: %v", err)
	}
	if meta == nil || meta.Title == "" {
		t.Errorf("expected non-empty fallback title on empty file, got %+v", meta)
	}

	// Case 2: Markdown file with only whitespace
	whitespaceFile := filepath.Join(tmpDir, "whitespace.md")
	if err := os.WriteFile(whitespaceFile, []byte("   \n\n  \t\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	meta, err = parser.Parse(whitespaceFile)
	if err != nil {
		t.Fatalf("Parse on whitespace file failed: %v", err)
	}
	if meta.Title != "Whitespace" {
		t.Errorf("expected title 'Whitespace', got %q", meta.Title)
	}

	// Case 3: Non-existent file returns error gracefully
	_, err = parser.Parse(filepath.Join(tmpDir, "does-not-exist.md"))
	if err == nil {
		t.Errorf("expected error when parsing non-existent file, got nil")
	}

	// Case 4: Malformed frontmatter or unusual characters without H1
	weirdFile := filepath.Join(tmpDir, "malformed.md")
	weirdContent := "---\ntitle: unclosed \"quote\n---\nSome text without H1"
	if err := os.WriteFile(weirdFile, []byte(weirdContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	meta, err = parser.Parse(weirdFile)
	if err != nil {
		t.Fatalf("Parse failed on weird markdown: %v", err)
	}
	if meta.Title == "" {
		t.Errorf("expected fallback title from filename, got empty")
	}
}

// TestDivioQuadrant_IntegrationAndConformance validates CRIT-1789619391641753000-e04ee562:
// Verifies integration with live repository docs, zero broken titles, and conformance with POL-DOC-001/002.
func TestDivioQuadrant_IntegrationAndConformance(t *testing.T) {
	repoRoot := findRepoRoot(t)
	disc := NewDiscoverer(repoRoot)
	files, err := disc.Discover()
	if err != nil {
		t.Fatalf("Discover failed on repo docs: %v", err)
	}

	if len(files) < 10 {
		t.Fatalf("expected at least 10 discovered documentation files, got %d", len(files))
	}

	parser := NewParser()
	titlesSeen := make(map[string]string)
	coreQuadrantsSeen := map[string]bool{
		"tutorials":   false,
		"howto":       false,
		"manual":      false,
		"explanation": false,
	}

	for _, file := range files {
		for quad := range coreQuadrantsSeen {
			if strings.Contains(filepath.ToSlash(file.RelPath), "docs/"+quad+"/") {
				coreQuadrantsSeen[quad] = true
			}
		}

		meta, err := parser.Parse(file.Path)
		if err != nil {
			t.Errorf("failed to parse %s: %v", file.RelPath, err)
			continue
		}

		// Check for duplicate titles in the 4 core Divio quadrant paths
		isCore := false
		for quad := range coreQuadrantsSeen {
			if strings.Contains(filepath.ToSlash(file.RelPath), "docs/"+quad+"/") {
				isCore = true
				break
			}
		}
		if isCore && meta.Title != "" {
			if prev, exists := titlesSeen[meta.Title]; exists {
				t.Errorf("POL-DOC-002 Title collision: %q in %s and %s", meta.Title, file.RelPath, prev)
			}
			titlesSeen[meta.Title] = file.RelPath
		}
	}

	for quad, found := range coreQuadrantsSeen {
		if !found {
			t.Errorf("expected to find documents in quadrant %q", quad)
		}
	}
}
