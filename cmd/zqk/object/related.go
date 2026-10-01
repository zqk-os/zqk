package object

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// NewRelatedCmd creates a new related command
// Generated from command spec - DO NOT EDIT MANUALLY (use spec file as source of truth)
func NewRelatedCmd() *cobra.Command {
	// Use generated builder from spec
	cmd := bldr_cli_cmd_v1.NewObjectRelatedCommandBuilder()
	// Add RunE implementation
	cli.BindAsyncProgress(cmd, runRelated)
	return cmd
}

func runRelated(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		id := args[0]

		var err error
		_ = err

		// Get flags
		depth, err := cmd.Flags().GetInt("depth")
		if err != nil {
			depth = 1 // Default depth
		}
		if depth < 1 {
			depth = 1
		}

		//nolint:errcheck // Flag get - error indicates flag not set, default used
		relationshipType, _ := cmd.Flags().GetString("relationship")

		logging.FluentEvent(proc.Logger()).Debug("Finding related objects").
			ObjectID(id).
			Int("depth", depth).
			String("relationship", relationshipType).
			Log()

		// Get related objects
		related, err := proc.Storage().GetRelated(
			proc.OperationContext(),
			proc.SecurityContext(),
			id,
			relationshipType,
			depth,
		)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to find related objects", err).
				ObjectID(id).
				Log()
			return cli.Guard(cmd).Err(err).Wrapf("failed to find related objects: %w").Return()
		}

		logging.FluentEvent(proc.Logger()).Debug("Found related objects").
			ObjectID(id).
			Int("count", len(related)).
			Log()

		// Output based on format
		format := proc.Format()
		switch format {
		case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
			// Use FormatOutput for structured data (JSON/YAML)
			return cli.FormatOutput(cmd, related)
		case cli.FormatTable:
			// Use custom table formatting for better readability
			return outputTableArray(cmd, related)
		default:
			// Default to FormatOutput (will use YAML)
			return cli.FormatOutput(cmd, related)
		}
	})(cmd, args)
}

func outputTableArray(cmd *cobra.Command, objs []map[string]any) error {
	var buf strings.Builder

	if len(objs) == 0 {
		buf.WriteString("No related objects found.\n")
		return cli.WriteOutput(cmd, []byte(buf.String()))
	}

	// Print header
	fmt.Fprintf(&buf, "%-15s %-20s %-60s\n", "ID", "Kind", "Title")
	buf.WriteString(strings.Repeat("-", 95) + "\n")

	// Print rows
	for _, obj := range objs {
		id, _ := obj[objects.FieldKeyID].(string)
		kind, _ := obj[objects.FieldKeyKind].(string)
		title, _ := obj[objects.FieldKeyTitle].(string)

		// Truncate long titles
		if len(title) > 57 {
			title = title[:54] + "..."
		}

		fmt.Fprintf(&buf, "%-15s %-20s %-60s\n", id, kind, title)
	}

	fmt.Fprintf(&buf, "\nTotal: %d object(s)\n", len(objs))
	return cli.WriteOutput(cmd, []byte(buf.String()))
}
