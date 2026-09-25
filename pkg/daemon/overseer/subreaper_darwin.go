//go:build darwin

package overseer

func setSubreaper() error {
	// Darwin/XNU kernel does not support PR_SET_CHILD_SUBREAPER.
	// Process isolation and group signaling are managed via POSIX process groups (pgid).
	return nil
}

func isSubreaperSupported() bool {
	return false
}
