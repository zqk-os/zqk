package fileutil

import (
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

var (
	// ErrUnsafeFlagPath is returned when a path or path segment begins with a hyphen ('-').
	ErrUnsafeFlagPath = errfmt.Errorf("unsafe path: flag-like segment prohibited")
)

// ValidateSafePath verifies that no path segment begins with a leading '-' or '--'.
// Path segments like '--help', '-rf', '--all', or '-p' indicate accidental option injection
// or unparsed flags being treated as filesystem targets, which poses significant risk of
// unintended directory creation and destructive shell argument injection.
func ValidateSafePath(path string) error {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil
	}
	clean := filepath.Clean(trimmed)
	if strings.HasPrefix(clean, "-") {
		return errfmt.Errorf("%w: path %q begins with flag-like prefix", ErrUnsafeFlagPath, path)
	}

	parts := strings.Split(clean, string(filepath.Separator))
	for _, part := range parts {
		if part == "." || part == ".." || part == "" {
			continue
		}
		if strings.HasPrefix(part, "-") {
			return errfmt.Errorf("%w: path %q contains flag-like segment %q", ErrUnsafeFlagPath, path, part)
		}
	}
	return nil
}
