package swarm

import (
	"path/filepath"
	"sort"
	"strings"
)

// MutationWriteRelPaths returns repo-relative paths written by mutation
// evidence tools (write_code / write_file), sorted and de-duplicated.
// Paths that escape root are dropped rather than trusted. Toolchain
// adapters (pkg/adapters/golang) filter this list; the kernel does not.
func MutationWriteRelPaths(history []ToolCallRecord, root string) []string {
	seen := map[string]struct{}{}
	for _, record := range history {
		if !IsMutationEvidenceTool(record.Name) {
			continue
		}
		p := strings.TrimSpace(toolPathArg(record.Arguments))
		if p == "" {
			continue
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		rel, err := filepath.Rel(root, p)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		seen[filepath.ToSlash(rel)] = struct{}{}
	}
	files := make([]string, 0, len(seen))
	for file := range seen {
		files = append(files, file)
	}
	sort.Strings(files)
	return files
}

// HistoryHasMutationWrite reports whether any write_code / write_file call
// is in history. Language adapters decide what those writes mean.
func HistoryHasMutationWrite(history []ToolCallRecord) bool {
	for _, record := range history {
		if IsMutationEvidenceTool(record.Name) {
			return true
		}
	}
	return false
}
