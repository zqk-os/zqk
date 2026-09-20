// Package diskusage reports directory sizes with pure Go (no du/find shell).
//
// Intended as a cross-platform companion to housekeeping (see clean-project-sprawl.sh):
// surface where generated data and logs grow without relying on OS-specific utilities.
//
// TRACK: docs/architecture/FILESYSTEM_DATA_LAYOUT.md — keep ≤100 top-level entries per data dir.
package diskusage

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// Options control a Scan.
type Options struct {
	// MaxDepth is relative to Root (root itself is depth 0). Directories deeper than
	// MaxDepth are still walked for size aggregation but are not listed. Default 2.
	MaxDepth int
	// MinBytes filters listed entries (0 = no filter).
	MinBytes int64
	// Limit caps how many entries are returned after sort (0 = unlimited).
	Limit int
	// IncludeFiles lists regular files at depth <= MaxDepth in addition to directories.
	IncludeFiles bool
}

// Entry is one reported path with cumulative byte size.
type Entry struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	SizeHuman string `json:"size_human"`
	Depth     int    `json:"depth"`
	IsDir     bool   `json:"is_dir"`
}

// Result is the structured scan payload.
type Result struct {
	Root           string  `json:"root"`
	MaxDepth       int     `json:"max_depth"`
	MinBytes       int64   `json:"min_bytes"`
	MinHuman       string  `json:"min_human,omitempty"`
	Entries        []Entry `json:"entries"`
	SkippedErrors  int     `json:"skipped_errors"`
	DirsVisited    int     `json:"dirs_visited"`
	FilesVisited   int     `json:"files_visited"`
	TotalBytesRoot int64   `json:"total_bytes_root"`
}

// Scan walks root and returns directory (and optional file) sizes up to MaxDepth,
// sorted descending by size. Symlinks are not followed.
func Scan(root string, opts Options) (*Result, error) {
	root = filepath.Clean(root)
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	root = abs

	if opts.MaxDepth < 0 {
		opts.MaxDepth = 0
	}

	dirSizes := map[string]int64{}
	fileEntries := map[string]int64{} // path -> size, only if IncludeFiles
	var skipped, dirsVisited, filesVisited int

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			skipped++
			// Skip unreadable dirs; continue siblings.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		// Do not follow symlinks (treat as leaf; do not descend into symlink dirs).
		if d.Type()&fs.ModeSymlink != 0 {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			dirsVisited++
			if _, ok := dirSizes[path]; !ok {
				dirSizes[path] = 0
			}
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			skipped++
			return nil
		}
		filesVisited++
		sz := info.Size()
		if opts.IncludeFiles {
			depth := pathDepth(root, path)
			if depth >= 0 && depth <= opts.MaxDepth {
				fileEntries[path] = sz
			}
		}
		// Attribute file size to every ancestor directory up to root.
		for p := filepath.Dir(path); ; p = filepath.Dir(p) {
			dirSizes[p] += sz
			if p == root || filepath.Dir(p) == p {
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	entries := make([]Entry, 0, len(dirSizes)+len(fileEntries))
	for path, sz := range dirSizes {
		depth := pathDepth(root, path)
		if depth < 0 || depth > opts.MaxDepth {
			continue
		}
		if sz < opts.MinBytes {
			continue
		}
		entries = append(entries, Entry{
			Path:      displayPath(root, path),
			SizeBytes: sz,
			SizeHuman: FormatBytes(sz),
			Depth:     depth,
			IsDir:     true,
		})
	}
	for path, sz := range fileEntries {
		if sz < opts.MinBytes {
			continue
		}
		entries = append(entries, Entry{
			Path:      displayPath(root, path),
			SizeBytes: sz,
			SizeHuman: FormatBytes(sz),
			Depth:     pathDepth(root, path),
			IsDir:     false,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].SizeBytes != entries[j].SizeBytes {
			return entries[i].SizeBytes > entries[j].SizeBytes
		}
		return entries[i].Path < entries[j].Path
	})
	if opts.Limit > 0 && len(entries) > opts.Limit {
		entries = entries[:opts.Limit]
	}

	res := &Result{
		Root:           root,
		MaxDepth:       opts.MaxDepth,
		MinBytes:       opts.MinBytes,
		Entries:        entries,
		SkippedErrors:  skipped,
		DirsVisited:    dirsVisited,
		FilesVisited:   filesVisited,
		TotalBytesRoot: dirSizes[root],
	}
	if opts.MinBytes > 0 {
		res.MinHuman = FormatBytes(opts.MinBytes)
	}
	return res, nil
}

func pathDepth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return -1
	}
	if rel == "." {
		return 0
	}
	if strings.HasPrefix(rel, "..") {
		return -1
	}
	n := 0
	for _, p := range strings.Split(rel, string(filepath.Separator)) {
		if p != "" && p != "." {
			n++
		}
	}
	return n
}

func displayPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return "."
	}
	return filepath.ToSlash(rel)
}
