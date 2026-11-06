package storage

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
)

// SyncCASIndexMapping flushes pending writes and ensures the CAS index mapping for objectID is on disk (GH-1005).
func SyncCASIndexMapping(projectRoot, kind, objectID string) error {
	if projectRoot == "" {
		return nil
	}
	if err := FlushListingIndexForProjectRoot(projectRoot, kind); err != nil {
		return err
	}
	kindDir := filepath.Join(projectRoot, paths.ProcessDir, objects.GetDirectoryFromKind(kind))
	return filecas.SyncKindIndex(kindDir, kind, objectID)
}
