package object

import (
	"fmt"
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/pkg/objects"
)

// NewObjectRootFieldsCmd creates a top-level "object fields" command for parity and --list-kinds.
// Kind-specific fields remain under "object <kind> fields".
func NewObjectRootFieldsCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"List object kinds or explore fields (use object <kind> fields for a specific kind)",
		"Without --list-kinds, use 'object <kind> fields' to list fields for a specific kind.",
		"With --list-kinds, output all object kinds (table, json, or yaml).",
		"",
		fmt.Sprintf("Examples: %s object backlog_item fields", paths.CLICommandName),
	).
		AddExample("List all kinds", fmt.Sprintf("%s object fields --list-kinds", paths.CLICommandName)).
		AddExample("List kinds as JSON", fmt.Sprintf("%s object fields --list-kinds --format json", paths.CLICommandName)).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewObjectFieldsCommandBuilder(), &cobra.Command{
		Use:  "fields [flags]",
		Args: cobra.NoArgs,
		RunE: runObjectRootFields,
	})
	helpBuilder.ApplyToCommand(cmd)

	clipkg.AddFieldsFlags(cmd, true)

	return cmd
}

func runObjectRootFields(cmd *cobra.Command, args []string) error {
	opts := clipkg.ParseFieldsFlags(cmd)
	if !opts.ListKinds {
		return cli.Guard(cmd).Requiref(false, "kind is required (usage: %s object <kind> fields); use --list-kinds to list all kinds", paths.CLICommandName).Return()
	}

	// Prefer spec index when available (fast, immutable snapshot); fall back to dynamic
	// FieldRegistry when the index has not been generated yet.
	var kinds []string
	projectRoot := cli.ResolveProjectRoot(".")
	if idx := objects.TryLoadSpecIndexForProjectRoot(projectRoot); idx != nil {
		kinds = objects.GetAllKindsFromIndex(idx)
	} else {
		registry := objects.GetGlobalFieldRegistry()
		if err := registry.LoadFields(); err != nil {
			logging.FluentEvent(logging.GetLoggerFromContext(cmd.Context())).Error("Failed to load fields", err).Log()
			return cli.Guard(cmd).Err(err).Wrapf("failed to load fields: %w").Return()
		}
		var err error
		kinds, err = registry.GetAllKinds()
		if err != nil {
			logging.FluentEvent(logging.GetLoggerFromContext(cmd.Context())).Error("Failed to get kinds", err).Log()
			return cli.Guard(cmd).Err(err).Wrapf("failed to get kinds: %w").Return()
		}
	}

	switch opts.Format {
	case objectFormatJSON, objectFormatYAML:
		data := map[string]any{"kinds": kinds}
		if err := cli.FormatOutput(cmd, data); err != nil {
			return cli.Guard(cmd).Err(err).Return()
		}
		return nil
	default:
		return outputKindsTable(cmd, kinds)
	}
}

func outputKindsTable(cmd *cobra.Command, kinds []string) error {
	var b strings.Builder
	b.WriteString("Kinds:\n")
	b.WriteString(strings.Repeat("-", 40) + "\n")
	for _, k := range kinds {
		b.WriteString("  " + k + "\n")
	}
	return cli.WriteOutput(cmd, []byte(b.String()))
}
