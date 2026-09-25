//go:build linux

package overseer

import "golang.org/x/sys/unix"

func setSubreaper() error {
	return unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0)
}

func isSubreaperSupported() bool {
	return true
}
