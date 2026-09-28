package storage

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/errfmt"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	draftSweepErrPrefix = "object draft plane: delete %s"
)

// DeleteObjectDraftFile removes a draft-plane file and cleans up empty shard dirs.
func DeleteObjectDraftFile(projectRoot, kind, id string) error {
	if projectRoot == emptyValue || kind == emptyValue || id == emptyValue {
		return nil
	}
	path := ObjectDraftPlanePath(projectRoot, kind, id)
	if err := fileutil.Remove(path); err != nil && !fileutil.IsNotExist(err) {
		return errfmt.Newf(draftSweepErrPrefix, id).Wrap(err)
	}
	shardDir := filepath.Dir(path)
	if rmErr := fileutil.Remove(shardDir); rmErr != nil {
		// Shard dir might not be empty; cleanup is best effort
	}
	parentDir := filepath.Dir(shardDir)
	if rmParentErr := fileutil.Remove(parentDir); rmParentErr != nil {
		// Parent kind dir might not be empty; cleanup is best effort
	}
	return nil
}
