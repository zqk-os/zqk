// Package safepath builds filesystem paths confined under a root directory to mitigate
// directory traversal when joining untrusted or external segments (gosec G703).
// TRACK: BLI-CEF-R15-PATH-TRAVERSAL-001 / REQ-CEF-R2-SEC-PATH-TRAVERSAL
package safepath

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	emptyValue         = ""
	errEmptyRoot       = "safepath: empty root"
	errPathEscapesRoot = "safepath: path escapes root"
	errAbsRootFmt      = "safepath: abs root: %w"
	errRelPathFmt      = "safepath: rel: %w"
	parentDirMarker    = ".."
)

// JoinUnderRoot is filepath.Join(root, elems...) followed by a containment check:
// the result must be lexically under root (after Clean). Rejects empty root or any
// ".." segment that would escape root.
func JoinUnderRoot(root string, elems ...string) (string, error) {
	if root == emptyValue {
		return emptyValue, errors.New(errEmptyRoot)
	}
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) {
		abs, err := filepath.Abs(root)
		if err != nil {
			return emptyValue, errfmt.Errorf(errAbsRootFmt, err)
		}
		root = abs
	}
	parts := append([]string{root}, elems...)
	p := filepath.Join(parts...)
	p = filepath.Clean(p)
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return emptyValue, errfmt.Errorf(errRelPathFmt, err)
	}
	if rel == parentDirMarker || strings.HasPrefix(rel, parentDirMarker+string(fileutil.PathSeparator)) {
		return emptyValue, errors.New(errPathEscapesRoot)
	}
	return p, nil
}
