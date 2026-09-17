package search

import (
	"bytes"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// DefaultIgnoreDirs contains directory names excluded from search by default.
var DefaultIgnoreDirs = map[string]bool{
	".git":           true,
	".zqk":           true,
	".zqk-state":     true,
	".gemini":        true,
	"vendor":         true,
	"node_modules":   true,
	"bin":            true,
	"dist":           true,
	"dist-community": true,
	"build":          true,
	"target":         true,
	".vscode":        true,
	".idea":          true,
	"tmp":            true,
	".cache":         true,
	".cursor":        true,
	".claude":        true,
	".agent":         true,
	".agents":        true,
}

// DefaultIgnoreExtensions contains binary and noise extensions excluded by default when no explicit extension filter is passed.
var DefaultIgnoreExtensions = map[string]bool{
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".bin": true,
	".test": true, ".out": true, ".o": true, ".a": true,
	".tar": true, ".gz": true, ".tgz": true, ".zip": true, ".bz2": true, ".xz": true, ".7z": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true, ".webp": true, ".bmp": true,
	".pdf": true, ".woff": true, ".woff2": true, ".ttf": true, ".eot": true,
	".csnap": true, ".db": true, ".sqlite": true,
	".log": true, ".tmp": true, ".bak": true, ".swp": true,
}

// IsBinary detects whether content contains null bytes in the first 512 bytes,
// indicating binary content (same heuristic used by git and file).
func IsBinary(data []byte) bool {
	checkLen := len(data)
	if checkLen > 512 {
		checkLen = 512
	}
	return bytes.IndexByte(data[:checkLen], 0) != -1
}

// IsBinaryFile inspects the first 512 bytes of a file to check for binary content.
func IsBinaryFile(path string) bool {
	f, err := fileutil.Open(path)
	if err != nil {
		return true // Treat unreadable files as skipped/binary
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return true
	}
	return IsBinary(buf[:n])
}

// MatchExtension checks if a path matches the desired file extensions.
func MatchExtension(path string, exts []string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if len(exts) == 0 {
		return !DefaultIgnoreExtensions[ext]
	}
	for _, target := range exts {
		target = strings.ToLower(target)
		if !strings.HasPrefix(target, ".") {
			target = "." + target
		}
		if ext == target {
			return true
		}
	}
	return false
}

// ShouldSkipDir returns true if a directory should be skipped during walk.
func ShouldSkipDir(name string, includeHidden bool, customExcludes []string) bool {
	if !includeHidden && strings.HasPrefix(name, ".") && name != "." && name != ".." {
		return true
	}
	if DefaultIgnoreDirs[name] {
		return true
	}
	for _, exclude := range customExcludes {
		if strings.EqualFold(name, exclude) {
			return true
		}
	}
	return false
}

// CollectFiles gathers candidate file paths under root that pass extension and directory filters.
func CollectFiles(root string, opts SearchOptions) ([]string, error) {
	root = filepath.Clean(root)
	info, err := fileutil.Stat(root)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		if !MatchExtension(root, opts.FileExtensions) {
			return nil, nil
		}
		if IsBinaryFile(root) {
			return nil, nil
		}
		return []string{root}, nil
	}

	var files []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip unreadable paths without terminating walk
		}

		if d.IsDir() {
			if path == root {
				return nil
			}
			if ShouldSkipDir(d.Name(), opts.IncludeHidden, opts.ExcludePatterns) {
				return filepath.SkipDir
			}
			return nil
		}

		if !d.Type().IsRegular() {
			return nil
		}

		name := d.Name()
		// Skip known root/intermediate compiled binaries with no extension
		if name == "zqk" || name == "zqk-mcp" || name == "zqk_local" || name == "split-pri-214-by-theme" || name == "seed_brand_assets_to_glossary" {
			return nil
		}

		if !MatchExtension(path, opts.FileExtensions) {
			return nil
		}

		info, err := d.Info()
		if err == nil {
			// Skip oversized files (>2MB) to prevent memory exhaustion and I/O stalls
			if info.Size() > 2*1024*1024 {
				return nil
			}
			// Skip executable files with no extension (compiled Mach-O/ELF binaries)
			if filepath.Ext(name) == "" && info.Mode()&0111 != 0 {
				return nil
			}
		}

		files = append(files, path)
		return nil
	})

	if err != nil {
		return nil, err
	}
	return files, nil
}
