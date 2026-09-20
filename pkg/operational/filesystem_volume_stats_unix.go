//go:build !windows

package operational

import (
	"golang.org/x/sys/unix"
)

// fillVolumeStats sets snap.Volume using statfs(2). This is the same class of information as
// df(1) for the mount containing path, without spawning a subprocess or parsing text.
func fillVolumeStats(snap *FilesystemProjectSnapshot, path string) {
	if snap == nil || path == emptyValue {
		return
	}
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return
	}
	bs := uint64(st.Bsize)
	v := &FilesystemVolumeStats{
		Source:          "statfs",
		BlockSizeBytes:  bs,
		BlocksTotal:     st.Blocks,
		BlocksFree:      st.Bfree,
		BlocksAvailable: st.Bavail,
	}
	v.TotalBytes = st.Blocks * bs
	v.FreeBytes = st.Bfree * bs
	v.AvailableBytes = st.Bavail * bs
	snap.Volume = v
}
