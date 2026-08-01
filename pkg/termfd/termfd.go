// Package termfd centralizes *os.File descriptor int conversion for golang.org/x/term
// and similar APIs (gosec G115 otherwise flags int(fd) at each call site).
package termfd

import "os"

// Int returns f's OS file descriptor as int for term.IsTerminal, term.GetSize, etc.
func Int(f *os.File) int {
	return int(f.Fd()) //nolint:gosec // G115: OS FD for terminal APIs
}
