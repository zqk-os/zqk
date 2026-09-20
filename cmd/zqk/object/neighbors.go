package object

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
)

// NewNeighborsCmd creates a new neighbors command
// Generated from command spec - DO NOT EDIT MANUALLY (use spec file as source of truth)
func NewNeighborsCmd() *cobra.Command {
	// Use generated builder from spec
	cmd := bldr_cli_cmd_v1.NewObjectNeighborsCommandBuilder()
	// Add RunE implementation
	cli.BindAsyncProgress(cmd, runNeighbors)
	return cmd
}

func runNeighbors(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		id := args[0]

		var err error
		_ = err

		// Get flags
		direction, err := cmd.Flags().GetString("direction")
		if err != nil {
			direction = "both" // Default direction
		}
		if direction == emptyValue {
			direction = "both"
		}

		// Validate direction
		if direction != "outgoing" && direction != "incoming" && direction != "both" {
			return cli.Guard(cmd).Err(errfmt.Errorf("invalid direction: %s (must be: outgoing, incoming, or both)", direction)).Return()
		}

		logging.FluentEvent(proc.Logger()).Debug("Finding neighbors").
			ObjectID(id).
			String("direction", direction).
			Log()

		// Get neighbors
		neighbors, err := proc.Storage().GetNeighbors(
			proc.OperationContext(),
			proc.SecurityContext(),
			id,
			direction,
		)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to find neighbors", err).
				ObjectID(id).
				Log()
			return cli.Guard(cmd).Err(err).Wrapf("failed to find neighbors: %w").Return()
		}

		logging.FluentEvent(proc.Logger()).Debug("Found neighbors").
			ObjectID(id).
			Int("count", len(neighbors)).
			Log()

		// Output based on format
		format := proc.Format()
		switch format {
		case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
			// Use FormatOutput for structured data (JSON/YAML)
			return cli.FormatOutput(cmd, neighbors)
		case cli.FormatTable:
			// Use custom table formatting for better readability
			return outputTableArray(cmd, neighbors)
		default:
			// Default to FormatOutput (will use YAML)
			return cli.FormatOutput(cmd, neighbors)
		}
	})(cmd, args)
}
