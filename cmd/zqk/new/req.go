package newcmd

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
)

// FlushAndRefreshRequirementCASIndex flushes pending CAS index updates and ensures
// the requirement index on disk (.requirement.index) is synchronized for the given
// requirement ID prior to downstream pipeline resolution (GH-1005).
func FlushAndRefreshRequirementCASIndex(projectRoot, id string) error {
	return FlushAndRefreshCASIndex(projectRoot, objects.KindRequirement, id)
}

// FlushAndRefreshCASIndex flushes any pending CAS listing index writes for the kind,
// and ensures the on-disk index file (e.g. .requirement.index, .test_case.index) contains
// the mapping for the given ID (GH-1005).
func FlushAndRefreshCASIndex(projectRoot, kind, id string) error {
	if projectRoot == "" {
		return nil
	}
	// 1. Flush pending writes from the listing index queue to disk
	_ = storage.FlushListingIndexForProjectRoot(projectRoot, kind)

	// 2. Ensure the in-memory/on-disk index mapping is populated and persisted
	kindDir := filepath.Join(projectRoot, paths.ProcessDir, objects.GetDirectoryFromKind(kind))
	cas := filecas.NewContentAddressableStorage(kindDir, kind)
	if cas != nil {
		_ = cas.EnsureIndexPopulated()
		if id != "" {
			if _, err := cas.GetHashForID(id); err == nil {
				_ = cas.GetIndex().Save()
			}
		}
	}
	return nil
}
