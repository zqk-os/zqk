//go:build !darwin

package filecas

import (
	"github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
)

// casPublishSyncFileOS fsyncs the temp file before CAS hardlink/rename so a
// crash cannot leave a published name pointing at buffered-only bytes.
func CasPublishSyncFileOS(f *fileutil.File) error {
	if f == nil {
		return errfmt.Errorf("failed to sync temp file")
	}
	if err := f.Sync(); err != nil {
		return errfmt.Newf("failed to sync temp file").Wrap(err)
	}
	return nil
}

// casPublishSyncDirOS fsyncs the parent directory so the published directory
// entry is durable (POSIX crash consistency for Linux/Windows release targets).
func CasPublishSyncDirOS(dirPath string) error {
	dirFD, err := fileutil.Open(dirPath)
	if err != nil {
		return errfmt.Newf("failed to sync cas directory").Wrap(err)
	}
	defer func() {
		if cerr := dirFD.Close(); cerr != nil {
			logging.LogSwallowedError(cerr)
		}
	}()
	if err := dirFD.Sync(); err != nil {
		return errfmt.Newf("failed to sync cas directory").Wrap(err)
	}
	return nil
}

// DrainDarwinSyncQueue is a no-op on non-darwin platforms.
func DrainDarwinSyncQueue(_ ...any) error {
	return nil
}
