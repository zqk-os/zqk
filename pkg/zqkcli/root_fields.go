package internal

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"

	"github.com/zqk-os/zqk/pkg/objects"
)

// NewInternalRootFieldsCmd creates a top-level "internal fields" command for parity with object.
// Kind-specific fields remain under "internal <kind> fields".
func NewInternalRootFieldsCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"List internal object kinds or explore fields (use internal <kind> fields for a specific kind)",
		"Without --list-kinds, use 'internal <kind> fields' to list fields for a specific internal kind.",
		"With --list-kinds, output all internal object kinds (table, json, or yaml).",
		"",
		fmt.Sprintf("Examples: %s internal object_spec fields", paths.CLICommandName),
	).
		AddExample("List all kinds", fmt.Sprintf("%s internal fields --list-kinds", paths.CLICommandName)).
		AddExample("List kinds as JSON", fmt.Sprintf("%s internal fields --list-kinds --format json", paths.CLICommandName)).
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use:  "fields [flags]",
		Args: cobra.NoArgs,
		RunE: runInternalRootFields,
	}
	helpBuilder.ApplyToCommand(cmd)

	clipkg.AddFieldsFlags(cmd, true)

	return cmd
}

func runInternalRootFields(cmd *cobra.Command, args []string) error {
	opts, err := clipkg.ParseFieldsFlags(cmd)
	if err != nil {
		return err
	}
	if !opts.ListKinds {
		return errfmt.Errorf("kind is required (usage: %s internal <kind> fields); use --list-kinds to list all kinds", paths.CLICommandName)
	}

	registry := objects.GetGlobalFieldRegistry()
	if err := registry.LoadFields(); err != nil {
		logging.FluentEvent(logging.GetLoggerFromContext(cmd.Context())).Error("Failed to load fields", err).Log()
		return errfmt.Newf("failed to load fields").Wrap(err)
	}

	kinds, err := registry.GetAllKinds()
	if err != nil {
		logging.FluentEvent(logging.GetLoggerFromContext(cmd.Context())).Error("Failed to get kinds", err).Log()
		return errfmt.Newf("failed to get kinds").Wrap(err)
	}

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, map[string]any{"kinds": kinds})
	default:
		return outputInternalKindsTable(cmd, kinds)
	}
}

func outputInternalKindsTable(cmd *cobra.Command, kinds []string) error {
	var b strings.Builder
	b.WriteString("Kinds:\n")
	b.WriteString(strings.Repeat("-", 40) + "\n")
	for _, k := range kinds {
		b.WriteString("  " + k + "\n")
	}
	return cli.WriteOutput(cmd, []byte(b.String()))
}
