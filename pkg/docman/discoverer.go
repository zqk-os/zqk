package docman

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/appledouble"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
)

// MarkdownFile represents a discovered markdown file
type MarkdownFile struct {
	Path    string
	RelPath string // Relative path from project root
}

// Discoverer scans for markdown files in the documentation directory
type Discoverer struct {
	rootPath    string
	excludeDirs []string
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
	}

	// Try to load from config if available
	// Note: This would require importing scanner package or moving config to shared location
	// For now, use defaults but structure allows for future enhancement

	return &Discoverer{
		rootPath:    rootPath,
		excludeDirs: excludeDirs,
	}
}

// Discover finds all markdown files in the docs directory
func (d *Discoverer) Discover() ([]*MarkdownFile, error) {
	var files []*MarkdownFile

	docsDir := paths.ResolvePath(d.rootPath, "prefix:docs")
	if _, err := os.Stat(docsDir); os.IsNotExist(err) {
		return files, nil // No docs directory, return empty
	}

	err := filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			baseName := filepath.Base(path)
			for _, excludeDir := range d.excludeDirs {
				if baseName == excludeDir {
					return filepath.SkipDir
				}
			}
			return nil
		}

		if appledouble.SkipPathInTreeWalk(path) {
			return nil
		}

		// Check if file is markdown
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}

		// Calculate relative path from project root
		relPath, err := filepath.Rel(d.rootPath, path)
		if err != nil {
			return nil
		}

		files = append(files, &MarkdownFile{
			Path:    path,
			RelPath: relPath,
		})

		return nil
	})

	if err != nil {
		return nil, errfmt.Newf("failed to scan docs directory").Wrap(err)
	}

	return files, nil
}
