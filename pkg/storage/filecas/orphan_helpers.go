package filecas

import (
	"fmt"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func RemoveRetiredCASHashFileOnIDChange(filePath string) error {
	if filePath == emptyValue {
		return nil
	}
	if err := fileutil.RemoveFileIfExists(filePath); err != nil {
		return err
	}
	return nil
}

func RemoveOrphanCASHashFileSync(filePath string) error {
	tmpPath := filePath + ".tmp"
	if err := fileutil.RenameFile(filePath, tmpPath); err != nil {
		if fileutil.IsNotExist(err) {
			if rmErr := fileutil.RemoveFileIfExists(tmpPath); rmErr != nil {
				logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstStreamFailedToRemoveTmpOrphanFileStrValN, tmpPath, rmErr), nil).Log()
			}
			return nil
		}
		if removeErr := fileutil.RemoveFileIfExists(filePath); removeErr != nil {
			return errfmt.Errorf(ConstStreamFailedToRemoveOrphanedFileStrErr, filePath, removeErr)
		}
		return nil
	}
	if err := fileutil.RemoveFileIfExists(tmpPath); err != nil {
		return errfmt.Errorf(ConstStreamFailedToRemoveOrphanedTmpFileStrErr, tmpPath, err)
	}
	dir := filepath.Dir(filePath)
	if syncErr := fileutil.SyncDir(dir); syncErr != nil {
		logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstStreamFailedToSyncDirStrAfterOrphanRemovalValN, dir, syncErr), nil).Log()
	}
	return nil
}
