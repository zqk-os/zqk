//go:build windows

package diagnostics

import "context"

func RegisterSignalHandlers(ctx context.Context) {}

func SetupSignalHandler(ctx context.Context, args ...string) {}
