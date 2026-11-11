//go:build windows

package diagnostics

// StartCPUProfileOnSIGUSR2 is a no-op on Windows (SIGUSR2 unavailable).
func StartCPUProfileOnSIGUSR2(logsDir string) {}
