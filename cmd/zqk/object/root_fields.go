package object

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/authcred"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewObjectRootFieldsCmd creates a top-level "object fields" command for parity and --list-kinds.
// Kind-specific fields remain under "object <kind> fields".
func NewObjectRootFieldsCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"List object kinds or explore fields (use object <kind> fields for a specific kind)",
		"Without --list-kinds, use 'object <kind> fields' to list fields for a specific kind.",
		"With --list-kinds, output object kinds for the current seat (planner/doer membrane).",
		"Use --all-kinds for the full registered catalog (admin break-glass).",
		"",
		fmt.Sprintf("Examples: %s object backlog_item fields", paths.CLICommandName),
	).
		AddExample("List seat-scoped kinds", fmt.Sprintf("%s object fields --list-kinds", paths.CLICommandName)).
		AddExample("List kinds as JSON", fmt.Sprintf("%s object fields --list-kinds --format json", paths.CLICommandName)).
		AddExample("Full catalog", fmt.Sprintf("%s object fields --list-kinds --all-kinds", paths.CLICommandName)).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewObjectFieldsCommandBuilder(), &cobra.Command{
		Use:  "fields [flags]",
		Args: cobra.NoArgs,
		RunE: runObjectRootFields,
	})
	helpBuilder.ApplyToCommand(cmd)

	clipkg.AddFieldsFlags(cmd, true)
	addAllKindsFlag(cmd)

	return cmd
}

func runObjectRootFields(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		opts, err := clipkg.ParseFieldsFlags(cmd)
		if err != nil {
			return err
		}
		if !opts.ListKinds {
			return cli.Guard(cmd).Requiref(false, "kind is required (usage: %s object <kind> fields); use --list-kinds to list all kinds", paths.CLICommandName).Return()
		}

		var kinds []string
		projectRoot := proc.ProjectRoot()
		if projectRoot == "" {
			projectRoot = cli.ResolveProjectRoot(".")
		}
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

		kinds, lane, allKinds := applyDiscoveryMembrane(cmd, proc, kinds)

		switch opts.Format {
		case objectFormatJSON, objectFormatYAML:
			data := map[string]any{
				"kinds":          kinds,
				"discovery_lane": string(lane),
				"all_kinds":      allKinds,
				"membrane":       "persona_rbac_discovery",
			}
			if err := cli.FormatOutput(cmd, data); err != nil {
				return cli.Guard(cmd).Err(err).Return()
			}
			return nil
		default:
			return outputKindsTable(cmd, kinds, lane, allKinds)
		}
	})(cmd, args)
}

func outputKindsTable(cmd *cobra.Command, kinds []string, lane authcred.DiscoveryLane, allKinds bool) error {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Kinds (discovery_lane=%s all_kinds=%v):\n", lane, allKinds))
	b.WriteString(strings.Repeat("-", 40) + "\n")
	for _, k := range kinds {
		b.WriteString("  " + k + "\n")
	}
	if !allKinds && lane != authcred.DiscoveryLaneFull {
		b.WriteString("\nUse --all-kinds for the full registered catalog.\n")
	}
	return cli.WriteOutput(cmd, []byte(b.String()))
}
