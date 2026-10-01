package snapshot

import (
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/storage"
)

// ReadAndExpandSnapshot reads and expands a compressed snapshot file.
func ReadAndExpandSnapshot(csnapFile string) ([]map[string]any, error) {
	cs, err := storage.ReadCompressedSnapshot(csnapFile)
	if err != nil {
		return nil, errfmt.Newf("failed to read compressed snapshot").Wrap(err)
	}

	expanded, err := cs.Expand()
	if err != nil {
		return nil, errfmt.Newf("failed to expand snapshot").Wrap(err)
	}

	return expanded, nil
}

// CalculateVerifyCount calculates how many objects to verify.
func CalculateVerifyCount(totalObjects, maxObjects int) int {
	if maxObjects > 0 && maxObjects < totalObjects {
		return maxObjects
	}
	return totalObjects
}
