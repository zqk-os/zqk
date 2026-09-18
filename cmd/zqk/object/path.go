package object

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

// NewPathCmd creates a new path command
// Generated from command spec - DO NOT EDIT MANUALLY (use spec file as source of truth)
func NewPathCmd() *cobra.Command {
	// Use generated builder from spec
	cmd := bldr_cli_cmd_v1.NewObjectPathCommandBuilder()
	// Add RunE implementation
	cli.BindAsyncProgress(cmd, runPath)
	return cmd
}

func runPath(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		fromID := args[0]
		toID := args[1]

		var err error
		_ = err

		logging.FluentEvent(proc.Logger()).Debug("Finding path between objects").
			String("from", fromID).
			String("to", toID).
			Log()

		// Get path
		path, err := proc.Storage().GetPath(
			proc.OperationContext(),
			proc.SecurityContext(),
			fromID,
			toID,
		)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to find path", err).
				String("from", fromID).
				String("to", toID).
				Log()
			return cli.Guard(cmd).Err(err).Wrapf("failed to find path: %w").Return()
		}

		logging.FluentEvent(proc.Logger()).Debug("Path found").
			String("from", fromID).
			String("to", toID).
			Int("length", len(path)).
			Log()

		if len(path) == 0 {
			// Use WriteOutput for simple message
			msg := fmt.Sprintf("No path found between %s and %s\n", fromID, toID)
			return cli.WriteOutput(cmd, []byte(msg))
		}

		// Output based on format
		format := proc.Format()
		switch format {
		case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
			// Use FormatOutput for structured data (JSON/YAML)
			return cli.FormatOutput(cmd, path)
		case cli.FormatTable:
			// Use custom table formatting for better readability
			return outputPathTable(cmd, path)
		default:
			// Default to FormatOutput (will use YAML)
			return cli.FormatOutput(cmd, path)
		}
	})(cmd, args)
}

func outputPathTable(cmd *cobra.Command, path []map[string]any) error {
	var buf strings.Builder

	if len(path) == 0 {
		buf.WriteString("No path found.\n")
		return cli.WriteOutput(cmd, []byte(buf.String()))
	}

	fmt.Fprintf(&buf, "Path (length: %d):\n\n", len(path))
	fmt.Fprintf(&buf, "%-5s %-15s %-20s %-60s\n", "Step", "ID", "Kind", "Title")
	buf.WriteString(strings.Repeat("-", 100) + "\n")

	for i, obj := range path {
		id, _ := obj[objects.FieldKeyID].(string)
		kind, _ := obj[objects.FieldKeyKind].(string)
		title, _ := obj[objects.FieldKeyTitle].(string)

		// Truncate long titles
		if len(title) > 57 {
			title = title[:54] + "..."
		}

		fmt.Fprintf(&buf, "%-5d %-15s %-20s %-60s\n", i+1, id, kind, title)
	}

	return cli.WriteOutput(cmd, []byte(buf.String()))
}

// outputJSONArray and outputYAMLArray are no longer needed - FormatOutput handles these
