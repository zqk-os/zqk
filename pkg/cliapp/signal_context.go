package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

// CommandSignalContext returns a context that cancels on SIGINT or SIGTERM, inheriting from cmd.Context().
func CommandSignalContext(cmd *cobra.Command) (context.Context, context.CancelFunc) {
	parent := cmd.Context()
	if parent == nil {
		parent = context.Background()
	}
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}
