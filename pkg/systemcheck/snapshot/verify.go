package snapshot

import (
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/storage"
)

// ReadAndExpandSnapshot reads and expands a compressed snapshot file.
func ReadAndExpandSnapshot(csnapFile string) ([]map[string]any, error) {
	_, expanded, err := ReadAndExpandSnapshotWithHeader(csnapFile)
	return expanded, err
}

// ReadAndExpandSnapshotWithHeader reads and expands a compressed snapshot file, returning the parsed snapshot and expanded maps.
func ReadAndExpandSnapshotWithHeader(csnapFile string) (*storage.CompressedSnapshot, []map[string]any, error) {
	cs, err := storage.ReadCompressedSnapshot(csnapFile)
	if err != nil {
		return nil, nil, errfmt.Newf("failed to read compressed snapshot").Wrap(err)
	}

	expanded, err := cs.Expand()
	if err != nil {
		return nil, nil, errfmt.Newf("failed to expand snapshot").Wrap(err)
	}

	return cs, expanded, nil
}

// CalculateVerifyCount calculates how many objects to verify.
func CalculateVerifyCount(totalObjects, maxObjects int) int {
	if maxObjects > 0 && maxObjects < totalObjects {
		return maxObjects
	}
	return totalObjects
}
