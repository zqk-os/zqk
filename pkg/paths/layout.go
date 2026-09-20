package paths

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// EnsureDir wraps [os.MkdirAll] with a stable error shape. Use with [DirPerm755], [DirPerm700], etc.
// For several directories under one root, prefer [LayoutUnder] to avoid repetition.
func EnsureDir(path string, mode fs.FileMode) error {
	if err := os.MkdirAll(path, mode); err != nil {
		return errfmt.Errorf("mkdir %s: %w", path, err)
	}
	return nil
}

// Layout creates project-relative directories under a single root via a fluent chain.
// rel arguments are typically package constants (e.g. [ProjectDataDir], [ProcessInternalObjectSpecsDir]).
//
// Pipeline: safe to call from a [github.com/zqk-os/zqk/pkg/pipeline] step—capture root in the closure
// and return [Layout.Err] from the step func (no pipeline import required here).
//
// Example:
//
//	if err := paths.LayoutUnder(absRoot).
//	    Dir(ProjectDataDir, DirPerm755).
//	    Dir(ProcessDir, DirPerm755).
//	    Dir(ProcessInternalObjectSpecsDir, DirPerm755).
//	    Err(); err != nil {
//	    return err
//	}
type Layout struct {
	root string
	err  error
}

// LayoutUnder starts a directory layout under root (absolute project or temp root).
func LayoutUnder(root string) *Layout {
	return &Layout{root: root}
}

// Dir creates filepath.Join(root, rel) with mode. Stops applying further steps after the first error.
func (l *Layout) Dir(rel string, mode fs.FileMode) *Layout {
	if l.err != nil {
		return l
	}
	l.err = EnsureDir(filepath.Join(l.root, rel), mode)
	return l
}

// Err returns the first error from the chain, or nil if all dirs were created.
func (l *Layout) Err() error {
	return l.err
}

// EnsureProcessAndObjectSpecsLayout creates [ProcessDir], [ProcessInternalObjectSpecsDir],
// [ProcessInternalLifecyclesDir], and [ProcessInternalTraitsDir] under root.
// Typical for storage (and similar) tests before [NewFileObjectStorageForTest] / [NewStorageFactory].
// Tests that Create objects must still copy lifecycle YAML into the lifecycles dir.
func EnsureProcessAndObjectSpecsLayout(root string) error {
	return LayoutUnder(root).
		Dir(ProcessDir, DirPerm755).
		Dir(ProcessInternalObjectSpecsDir, DirPerm755).
		Dir(ProcessInternalLifecyclesDir, DirPerm755).
		Dir(ProcessInternalTraitsDir, DirPerm755).
		Err()
}
