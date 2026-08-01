package operational

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
)

// FilesystemSnapshotScope limits which parts of the project tree the snapshot walks.
// Narrow scopes reduce wall time for scheduled jobs that only care about landfill under .zqk/ or docs/.
type FilesystemSnapshotScope string

const (
	// FSSnapshotScopeFull walks every top-level directory under the project root (plus root files).
	FSSnapshotScopeFull FilesystemSnapshotScope = "full"
	// FSSnapshotScopeZqk walks only under .zqk/ (fan-out parallel across immediate children when present).
	FSSnapshotScopeZqk FilesystemSnapshotScope = "zqk"
	// FSSnapshotScopeDocs walks only under docs/.
	FSSnapshotScopeDocs FilesystemSnapshotScope = "docs"
	// FSSnapshotScopeZqkAndDocs walks .zqk/ and docs/ with the same fan-out rules as the narrow scopes.
	FSSnapshotScopeZqkAndDocs FilesystemSnapshotScope = "zqk-and-docs"
)

// ParseFilesystemSnapshotScope normalizes CLI input.
func ParseFilesystemSnapshotScope(s string) (FilesystemSnapshotScope, error) {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "", "full":
		return FSSnapshotScopeFull, nil
	case "zqk":
		return FSSnapshotScopeZqk, nil
	case "docs":
		return FSSnapshotScopeDocs, nil
	case "zqk-and-docs", "zqk_docs", "zqk+docs":
		return FSSnapshotScopeZqkAndDocs, nil
	default:
		return "", errfmt.Errorf("unknown filesystem snapshot scope %q (full, zqk, docs, zqk-and-docs)", s)
	}
}

// planParallelWalkRoots returns absolute directory paths each of which gets its own WalkDir goroutine.
// includeRootFiles is true only for full scope (count go.mod etc. at repo root).
func planParallelWalkRoots(projectRootAbs string, scope FilesystemSnapshotScope) (walkRoots []string, includeRootFiles bool, err error) {
	switch scope {
	case FSSnapshotScopeFull:
		return planWalkRootsFull(projectRootAbs)
	case FSSnapshotScopeZqk:
		roots, err := planWalkRootsFanout(filepath.Join(projectRootAbs, paths.ProjectDataDir))
		return roots, false, err
	case FSSnapshotScopeDocs:
		roots, err := planWalkRootsFanout(filepath.Join(projectRootAbs, paths.DocsDir))
		return roots, false, err
	case FSSnapshotScopeZqkAndDocs:
		var roots []string
		r1, e1 := planWalkRootsFanout(filepath.Join(projectRootAbs, paths.ProjectDataDir))
		if e1 != nil {
			return nil, false, e1
		}
		roots = append(roots, r1...)
		r2, e2 := planWalkRootsFanout(filepath.Join(projectRootAbs, paths.DocsDir))
		if e2 != nil {
			return nil, false, e2
		}
		roots = append(roots, r2...)
		return roots, false, nil
	default:
		return nil, false, errfmt.Errorf("invalid scope %q", scope)
	}
}

func planWalkRootsFull(projectRootAbs string) (walkRoots []string, includeRootFiles bool, err error) {
	entries, err := os.ReadDir(projectRootAbs)
	if err != nil {
		return nil, false, errfmt.Newf("read project root").Wrap(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if _, skip := DefaultFilesystemSnapshotSkipDirs[name]; skip {
			continue
		}
		p := filepath.Join(projectRootAbs, name)
		if li, err := os.Lstat(p); err != nil || li.Mode()&os.ModeSymlink != 0 {
			continue
		}
		walkRoots = append(walkRoots, p)
	}
	return walkRoots, true, nil
}

// planWalkRootsFanout returns one path per immediate child directory under base, or base itself if there are no child dirs.
func planWalkRootsFanout(base string) (walkRoots []string, err error) {
	fi, err := os.Stat(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !fi.IsDir() {
		return nil, nil
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, skip := DefaultFilesystemSnapshotSkipDirs[e.Name()]; skip {
			continue
		}
		p := filepath.Join(base, e.Name())
		if li, err := os.Lstat(p); err != nil {
			continue
		} else if li.Mode()&os.ModeSymlink != 0 {
			continue
		}
		walkRoots = append(walkRoots, p)
	}
	if len(walkRoots) == 0 {
		return []string{base}, nil
	}
	return walkRoots, nil
}
