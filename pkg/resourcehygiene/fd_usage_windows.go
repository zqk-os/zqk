//go:build windows

package resourcehygiene

// GetProcessFDUsage returns stub values on Windows where POSIX file descriptors are not exposed.
func GetProcessFDUsage() (int, int, error) {
	return -1, -1, nil
}
