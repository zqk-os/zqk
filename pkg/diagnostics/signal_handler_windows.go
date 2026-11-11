//go:build windows

package diagnostics

import "context"

// SetupSignalHandler is a no-op on Windows (SIGUSR1 unavailable).
func SetupSignalHandler(ctx context.Context, projectRoot, prefix string) {}
