package vet

import (
	"bytes"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/git"
)

// isGeneratedCode checks if the file has typical code generator headers.
func isGeneratedCode(src []byte) bool {
	head := src
	if len(head) > 512 {
		head = head[:512]
	}
	return bytes.Contains(head, []byte("Code generated")) && bytes.Contains(head, []byte("DO NOT EDIT"))
}

// collectGitFiles returns all git-tracked files in root.
func collectGitFiles(root string) ([]string, error) {
	g := git.NewFacade(root)
	out, err := g.LSFiles("-z")
	if err == nil {
		var files []string
		for _, p := range strings.Split(string(out), "\x00") {
			if p != "" {
				files = append(files, p)
			}
		}
		if len(files) > 0 {
			return files, nil
		}
	}

	// Fallback to WalkDir if not a git repository
	var files []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".zqk" || name == "vendor" || name == "node_modules" || name == "bin" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		files = append(files, rel)
		return nil
	})
	return files, err
}

// isPathExempt checks if path matches any entry in exemptions.
// Supports exact match, prefix match (with trailing slash or /*), and glob pattern matching.
func isPathExempt(path string, exemptions []string) bool {
	path = filepath.ToSlash(path)
	for _, ex := range exemptions {
		ex = filepath.ToSlash(ex)
		if path == ex {
			return true
		}
		if strings.HasSuffix(ex, "/") && strings.HasPrefix(path, ex) {
			return true
		}
		if strings.HasSuffix(ex, "/*") {
			prefix := strings.TrimSuffix(ex, "/*")
			if strings.HasPrefix(path, prefix+"/") || path == prefix {
				return true
			}
		}
		if matched, _ := filepath.Match(ex, path); matched {
			return true
		}
		if matched, _ := filepath.Match(ex, filepath.Base(path)); matched {
			return true
		}
	}
	return false
}

// collectGoFiles gathers relevant Go files in the repository.
func collectGoFiles(root string, scanDirs []string) ([]string, error) {
	g := git.NewFacade(root)
	out, err := g.LSFiles("-z", "--cached", "--others", "--exclude-standard", "--", "*.go")
	if err == nil {
		var files []string
		for _, p := range strings.Split(string(out), "\x00") {
			if p == "" {
				continue
			}
			if matchesScanDirs(p, scanDirs) {
				files = append(files, p)
			}
		}
		if len(files) > 0 {
			return files, nil
		}
	}

	// Fallback
	var files []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".zqk" || name == "vendor" || name == "node_modules" || name == "bin" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if matchesScanDirs(rel, scanDirs) {
				files = append(files, rel)
			}
		}
		return nil
	})
	return files, err
}

func matchesScanDirs(rel string, scanDirs []string) bool {
	if len(scanDirs) == 0 {
		return !strings.HasPrefix(rel, "vendor/")
	}
	for _, dir := range scanDirs {
		dir = filepath.ToSlash(dir)
		if strings.HasPrefix(rel, dir) {
			return true
		}
	}
	return false
}
