package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewBundleApplyCommandBuilder creates a new bundle_apply command
func NewBundleApplyCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("apply")
	builder.WithShort("Apply a scenario bundle to the current project (objects only)")
	help := clipkg.DynamicHelpBuilder("Apply a scenario bundle to the current project (objects only)")
	help.WithDescriptionLines("Apply a scenario bundle to the current project in objects-only mode.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Creates objects (goals, requirements, criteria, backlog_items, test_cases) and")
	help.WithDescriptionLines("fixtures (e.g. scheduler_jobs) from the bundle YAML using the same storage and")
	help.WithDescriptionLines("validation layer as the main CLI. Create order is derived from the spec index")
	help.WithDescriptionLines("so that dependencies (e.g. goal before requirement) are respected.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Project root is required: use --project-root or set ZQK_PROJECT_ROOT / ZQK_TEST_ROOT.")
	help.WithDescriptionLines("A scenario summary is written to .zqk/scenarios/<bundle-name>/scenario-summary.json")
	help.WithDescriptionLines("with hint_to_id mappings and created IDs.")
	help.AddExample("Apply persistence traceability bundle", "%s bundle apply -b test-scenarios/persistence-bundle/persistence-bundle.yaml -R .")
	help.AddExample("Apply with project root from environment", "%s bundle apply --file my-bundle.yaml")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(0))
	builder.AddStringFlag("file", "b", "", "Path to scenario bundle file (YAML)")
	builder.AddStringFlag("project-root", "R", "", "Explicit project root to apply the bundle to (overrides ZQK_PROJECT_ROOT / ZQK_TEST_ROOT)")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
