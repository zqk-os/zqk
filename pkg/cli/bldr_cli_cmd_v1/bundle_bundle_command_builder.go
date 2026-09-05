package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewBundleBundleCommandBuilder creates a new bundle_bundle command
func NewBundleBundleCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("bundle")
	builder.WithShort("Work with scenario and traceability bundles")
	help := clipkg.DynamicHelpBuilder("Work with scenario and traceability bundles")
	help.WithDescriptionLines("Commands for applying and managing scenario bundles (traceability and execution).")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Bundles define requirements, criteria, backlog items, test cases, and fixtures")
	help.WithDescriptionLines("in YAML; the apply subcommand creates those objects in the project using the")
	help.WithDescriptionLines("same storage and validation as the main CLI. See docs/architecture/SCENARIO_BUNDLES_AND_TRACEABILITY.md.")
	help.AddExample("Apply a bundle to the current project", "%s bundle apply -f test-scenarios/persistence-bundle/persistence-bundle.yaml -R .")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
