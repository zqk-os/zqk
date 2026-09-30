package storage

import (
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// ownsWriteBehind reports whether this storage instance claimed WAL / write-behind.
func (f *FileObjectStorage) ownsWriteBehind() bool {
	if f == nil {
		return false
	}
	return f.writeBehindWorker != nil || f.writeBuf != nil || f.wal != nil
}

// rejectWriteBehindOnPrivilegedWriterRole is the constructor fail-closed:
// a writer-daemon process must not own write-behind or object.wal.
func rejectWriteBehindOnPrivilegedWriterRole(f *FileObjectStorage) error {
	if !zqkenv.PrivilegedWriterDaemonRole() {
		return nil
	}
	if f.ownsWriteBehind() {
		return errfmt.Errorf("privileged writer daemon must not own write-behind or object.wal")
	}
	return nil
}

// RefuseWriteBehindIfPrivilegedWriterRole is the post-init listen gate for
// `object daemon`. If write-behind was claimed, do not open the socket.
func (f *FileObjectStorage) RefuseWriteBehindIfPrivilegedWriterRole() error {
	return rejectWriteBehindOnPrivilegedWriterRole(f)
}
