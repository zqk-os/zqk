package crud

import (
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

// PathWithID holds a file path and its object ID for discovery.
type PathWithID struct {
	Path string
	ID   string
}

func cleanBaseName(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext != "" {
		return strings.TrimSuffix(base, ext)
	}
	return base
}

func isCASBase(base string) bool {
	return len(base) == 64 && IsHex(base)
}

func resolveNonCASID(base, kind string) string {
	// Account: account-username -> account:username
	accountDir := objects.GetDirectoryFromKind(objects.KindAccount)
	if accountDir != "" && objects.GetDirectoryFromKind(kind) == accountDir && strings.HasPrefix(base, "account-") {
		return "account:" + strings.TrimPrefix(base, "account-")
	}
	if base != "" {
		return base
	}
	return ""
}

// GetObjectIDAndKindFromPath extracts object ID and (for hash-based files) kind from a file.
// For hash-based files returns (id, fileKind); for others returns (id, "") so callers can skip kind filtering.
func GetObjectIDAndKindFromPath(path, kind string) (id, fileKind string) {
	base := cleanBaseName(path)
	if isCASBase(base) {
		return objects.ReadIDAndKindFromYAMLFile(path)
	}
	return resolveNonCASID(base, kind), ""
}

// GetObjectIDFromPath extracts object ID from a file path (filename or file content for hash-based files).
func GetObjectIDFromPath(path, kind string) string {
	base := cleanBaseName(path)
	if isCASBase(base) {
		return objects.ReadIDFromYAMLFile(path)
	}
	return resolveNonCASID(base, kind)
}

func IsHex(s string) bool {
	for _, c := range s {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}

// IsHashBasedFilename reports whether the filename stem is 64-char hex (content-addressed name).
func IsHashBasedFilename(filename string) bool {
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	return len(stem) == 64 && IsHex(stem)
}
