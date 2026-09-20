package search

import (
	"bytes"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// DefaultIgnoreDirs contains directory names excluded from search by default.
var DefaultIgnoreDirs = map[string]bool{
	".git":           true,
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
}

func init() {
	DefaultIgnoreDirs[paths.DefaultProjectStateDir] = true
}

// ZqkIgnoredSubdirs contains subdirectories under .zqk that should always be ignored (runtime caches/logs/builds).
var ZqkIgnoredSubdirs = map[string]bool{
	"cache":                 true,
	"state":                 true,
	"logs":                  true,
	"wal":                   true,
	"lock":                  true,
	"autofix":               true,
	objects.FieldKeyMetrics: true,
	"cleanup":               true,
	"test-bundles":          true,
	"keystore":              true,
	"credentials":           true,
	"streams":               true,
	"pre-commit":            true,
	"system-health":         true,
}

// DefaultIgnoreExtensions contains binary and noise extensions excluded by default when no explicit extension filter is passed.
var DefaultIgnoreExtensions = map[string]bool{
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".bin": true,
	".test": true, ".out": true, ".o": true, ".a": true,
	".tar": true, ".gz": true, ".tgz": true, ".zip": true, ".bz2": true, ".xz": true, ".7z": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true, ".webp": true, ".bmp": true,
	".pdf": true, ".woff": true, ".woff2": true, ".ttf": true, ".eot": true,
	".csnap": true, ".db": true, ".sqlite": true,
	".hashes": true, ".index": true,
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
	if name == ".git" {
		return true
	}
	// Always skip heavy build / vendor dirs
	if name == "vendor" || name == "node_modules" || name == "dist" || name == "dist-community" || name == "bin" || name == "target" || name == paths.DefaultProjectStateDir {
		return true
	}
	if ZqkIgnoredSubdirs[name] {
		return true
	}
	if !includeHidden {
		if strings.HasPrefix(name, ".") && name != "." && name != ".." {
			// Whitelist .agent, .agents, and .zqk for core project knowledge
			if name != ".agent" && name != ".agents" && name != paths.ProjectDataDir {
				return true
			}
		}
		if DefaultIgnoreDirs[name] {
			return true
		}
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
		// Skip internal index and cache dump files
		if strings.HasSuffix(name, "_cache.json") || strings.HasSuffix(name, ".cache.json") || strings.HasSuffix(name, ".idx") {
			return nil
		}
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
