package vet

import (
	"bytes"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"
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
	cmd := execwrap.Command("git", "-C", root, "ls-files", "-z")
	out, err := cmd.Output()
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

// collectGoFiles gathers relevant Go files in the repository (under pkg/, cmd/, internal/, scripts/).
func collectGoFiles(root string) ([]string, error) {
	cmd := execwrap.Command("git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", "*.go")
	out, err := cmd.Output()
	if err == nil {
		var files []string
		for _, p := range strings.Split(string(out), "\x00") {
			if p == "" {
				continue
			}
			if strings.HasPrefix(p, "pkg/") || strings.HasPrefix(p, "cmd/") || strings.HasPrefix(p, "internal/") || strings.HasPrefix(p, "scripts/") {
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
			if strings.HasPrefix(rel, "pkg/") || strings.HasPrefix(rel, "cmd/") || strings.HasPrefix(rel, "internal/") || strings.HasPrefix(rel, "scripts/") {
				files = append(files, rel)
			}
		}
		return nil
	})
	return files, err
}
