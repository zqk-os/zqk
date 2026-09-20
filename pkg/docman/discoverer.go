package docman

import (
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/appledouble"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ShippedInitDocSubtrees defines the default documentation subtrees that system init
// registers into the kernel doc_entry graph.
var ShippedInitDocSubtrees = []string{
	"docs/architecture",
	"docs/best-practices",
	"docs/onboarding",
}

// MarkdownFile represents a discovered markdown file
type MarkdownFile struct {
	Path    string
	RelPath string // Relative path from project root
}

// Discoverer scans for markdown files in the documentation directory
type Discoverer struct {
	rootPath    string
	excludeDirs []string
	subtrees    []string
}

// NewDiscoverer creates a new documentation discoverer
// Uses scanner configuration instead of hardcoded exclude lists
func NewDiscoverer(rootPath string) *Discoverer {
	// Import scanner config (same package structure)
	// For now, we'll duplicate the logic to avoid circular dependency
	// In the future, could move scanner_config to a shared package
	excludeDirs := []string{
		".git",
		"node_modules",
		"__pycache__",
		paths.ProjectDataDir,
		"archive",
		"_archive",
	}

	return &Discoverer{
		rootPath:    rootPath,
		excludeDirs: excludeDirs,
	}
}

// NewDiscovererWithSubtrees creates a discoverer that restricts scanning to the specified subtrees.
// Subtrees are relative paths from projectRoot (e.g. "docs/architecture").
func NewDiscovererWithSubtrees(rootPath string, subtrees []string) *Discoverer {
	d := NewDiscoverer(rootPath)
	d.subtrees = subtrees
	return d
}

// isExcludedPath checks if any path segment matches an excluded directory
func (d *Discoverer) isExcludedPath(relPath string) bool {
	relSlash := filepath.ToSlash(relPath)
	parts := strings.Split(relSlash, "/")
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		for _, ex := range d.excludeDirs {
			if strings.EqualFold(part, ex) {
				return true
			}
		}
	}
	return false
}

// Discover finds all markdown files in the docs directory or specified subtrees
func (d *Discoverer) Discover() ([]*MarkdownFile, error) {
	var files []*MarkdownFile

	scanDirs := make([]string, 0)
	if len(d.subtrees) > 0 {
		for _, sub := range d.subtrees {
			targetDir := filepath.Join(d.rootPath, filepath.FromSlash(sub))
			if _, err := fileutil.Stat(targetDir); err == nil {
				scanDirs = append(scanDirs, targetDir)
			}
		}
	} else {
		docsDir := paths.ResolvePath(d.rootPath, "prefix:docs")
		if _, err := fileutil.Stat(docsDir); err == nil {
			scanDirs = append(scanDirs, docsDir)
		}
	}

	for _, dir := range scanDirs {
		err := filepath.Walk(dir, func(path string, info fileutil.FileInfo, err error) error {
			if err != nil {
				return err
			}

			relPath, err := filepath.Rel(d.rootPath, path)
			if err != nil {
				return err
			}

			// Skip directories matching exclude patterns or having archive path segments
			if info.IsDir() {
				if d.isExcludedPath(relPath) {
					return filepath.SkipDir
				}
				return nil
			}

			if d.isExcludedPath(relPath) {
				return nil
			}

			if appledouble.SkipPathInTreeWalk(path) {
				return nil
			}

			// Check if file is markdown
			if !strings.HasSuffix(strings.ToLower(path), ".md") {
				return nil
			}

			files = append(files, &MarkdownFile{
				Path:    path,
				RelPath: filepath.ToSlash(relPath),
			})

			return nil
		})

		if err != nil {
			return nil, errfmt.Newf("failed to scan documentation directory").Wrap(err)
		}
	}

	return files, nil
}
