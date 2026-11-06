package newcmd

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
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
	return storage.SyncCASIndexMapping(projectRoot, kind, id)
}
