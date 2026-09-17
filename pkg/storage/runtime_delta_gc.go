package storage

import (
	"context"
	"path/filepath"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

type RuntimeDeltaGCResult struct {
	Kind         string
	FilesRemoved int
	Errors       int
}

// GCRuntimeDeltaCurrentForKind removes runtime-delta overlay files for IDs that no longer exist.
func GCRuntimeDeltaCurrentForKind(projectRoot, kind string) RuntimeDeltaGCResult {
	res := RuntimeDeltaGCResult{Kind: kind}
	dir := runtimeDeltaCurrentDir(projectRoot, kind)
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return res
		}
		res.Errors++
		return res
	}
	st, err := NewFileObjectStorage(projectRoot)
	if err != nil {
		res.Errors++
		return res
	}
	sys := pkgctx.NewSystemSecurityContext()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		id := trimYAMLExt(e.Name())
		if id == emptyValue {
			continue
		}
		exists, exErr := st.Exists(context.Background(), sys, id) // Background: request-or-shutdown derived
		if exErr == nil && exists {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if removeErr := fileutil.Remove(p); removeErr != nil && !fileutil.IsNotExist(removeErr) {
			res.Errors++
			continue
		}
		res.FilesRemoved++
	}
	return res
}

func trimYAMLExt(name string) string {
	switch {
	case strings.HasSuffix(name, ".yaml"):
		return strings.TrimSuffix(name, ".yaml")
	case strings.HasSuffix(name, ".yml"):
		return strings.TrimSuffix(name, ".yml")
	default:
		return ""
	}
}
