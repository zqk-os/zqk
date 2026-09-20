//go:build windows

package operational

// fillVolumeStats is a no-op on Windows; subtree walks still run. A future version could use
// GetDiskFreeSpaceEx or similar to populate Volume.
func fillVolumeStats(snap *FilesystemProjectSnapshot, path string) {
	_ = snap
	_ = path
}
