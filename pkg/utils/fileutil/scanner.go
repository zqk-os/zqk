package fileutil

import "path/filepath"

// FileMatch represents a successfully found file during a scan.
type FileMatch struct {
	Identifier string // The user-defined identifier for this file type
	Path       string // The absolute path where the file was found
}

// FindFiles scans multiple directories for specific filenames.
// It avoids scanning the same directory twice and prevents runaway traversal
// by skipping large known directories and enforcing a max depth.
// - searchDirs: A list of base directories to scan.
// - knownFiles: A map of target filename (e.g., "mcp.json") to a descriptive identifier.
func FindFiles(searchDirs []string, knownFiles map[string]string) []FileMatch {
	var matches []FileMatch
	scannedDirs := make(map[string]bool)

	skipDirs := map[string]bool{
		".git": true, "node_modules": true, "vendor": true,
		"Caches": true, "Cache": true, "Containers": true,
		"tmp": true, "temp": true, ".npm": true, ".cache": true,
	}

	for _, dir := range searchDirs {
		cleanDir := filepath.Clean(dir)

		if scannedDirs[cleanDir] {
			continue
		}
		if stat, err := Stat(cleanDir); err != nil || !stat.IsDir() {
			continue
		}
		scannedDirs[cleanDir] = true

		baseDepth := len(filepath.SplitList(cleanDir))

		_ = filepath.WalkDir(cleanDir, func(path string, d DirEntry, err error) error {
			if err != nil {
				return nil
			}

			if d.IsDir() {
				// Skip huge directories
				if skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				// Enforce depth limit (e.g., max 5 levels deep)
				currentDepth := len(filepath.SplitList(path))
				if currentDepth-baseDepth > 5 {
					return filepath.SkipDir
				}
				return nil
			}

			if identifier, ok := knownFiles[d.Name()]; ok {
				matches = append(matches, FileMatch{
					Identifier: identifier,
					Path:       path,
				})
			}
			return nil
		})
	}

	return matches
}
