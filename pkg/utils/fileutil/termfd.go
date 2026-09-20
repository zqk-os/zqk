// Package fileutil centralizes *os.File descriptor int conversion for golang.org/x/term
// and similar APIs (gosec G115 otherwise flags int(fd) at each call site).
package fileutil

// TermFdInt returns f's OS file descriptor as int for term.IsTerminal, term.GetSize, etc.
func TermFdInt(f *File) int {
	return int(f.Fd()) //nolint:gosec // G115: OS FD for terminal APIs
}
