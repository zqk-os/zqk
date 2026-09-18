package testenvroot

import "github.com/zqk-os/zqk/pkg/utils/fileutil"

// WriteFile delegates to fileutil.WriteFile (includes the repo-write guard).
func WriteFile(name string, data []byte, perm fileutil.FileMode) error {
	return fileutil.WriteFile(name, data, perm)
}

// MkdirAll delegates to fileutil.MkdirAll (includes the repo-write guard).
func MkdirAll(path string, perm fileutil.FileMode) error {
	return fileutil.MkdirAll(path, perm)
}
