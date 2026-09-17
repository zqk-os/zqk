package docman

import (
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
	if err := os.MkdirAll(filepath.Join(docsDir, "sub"), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	doc1 := filepath.Join(docsDir, "guide.md")
	doc2 := filepath.Join(docsDir, "sub", "tutorial.md")
	txt := filepath.Join(docsDir, "notes.txt")

	if err := os.WriteFile(doc1, []byte("# Guide"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(doc2, []byte("# Tutorial"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(txt, []byte("plain text"), 0644); err != nil {
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

// TestDivioQuadrant_FunctionalAcceptance validates CRIT-REDACTED:
//  1. All four Divio quadrant directories exist and are non-empty:
//     Tutorials (docs/tutorials), How-To Guides (docs/howto), Reference (docs/manual or docs/reference), Explanation (docs/explanation).
//  2. docs/INDEX.md exists and contains valid links pointing to existing files on disk.
func TestDivioQuadrant_FunctionalAcceptance(t *testing.T) {
	repoRoot := findRepoRoot(t)
	docsDir := filepath.Join(repoRoot, "docs")

	quadrants := map[string][]string{
		"Tutorials":   {"tutorials/quickstart.md", "tutorials/first-agent-session.md", "tutorials/object-lifecycle.md"},
		"How-To":      {"howto/create-objects.md", "howto/workflow-vds.md", "howto/agent-admin-membrane.md", "howto/process-cas-commits.md"},
		"Reference":   {"manual/CLI_REFERENCE.md", "reference/README.md"},
		"Explanation": {"explanation/kernel-vs-ide.md", "explanation/vds-state-machine.md", "explanation/agent-membrane.md"},
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

// TestDivioQuadrant_BoundaryAndErrorHandling validates CRIT-REDACTED:
// Verifies boundary behaviors, empty markdown file handling, missing headings, and non-existent paths.
func TestDivioQuadrant_BoundaryAndErrorHandling(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	parser := NewParser()

	// Case 1: Empty markdown file does not panic and returns non-empty fallback title
	emptyFile := filepath.Join(tmpDir, "empty-guide.md")
	if err := os.WriteFile(emptyFile, []byte(""), 0644); err != nil {
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
	if err := os.WriteFile(whitespaceFile, []byte("   \n\n  \t\n"), 0644); err != nil {
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
	if err := os.WriteFile(weirdFile, []byte(weirdContent), 0644); err != nil {
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

// TestDivioQuadrant_IntegrationAndConformance validates CRIT-REDACTED:
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
