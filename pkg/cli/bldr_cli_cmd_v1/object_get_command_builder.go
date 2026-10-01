package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewObjectGetCommandBuilder creates a new object_get command
func NewObjectGetCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("get")
	builder.WithShort("Get an object by ID")
	help := clipkg.DynamicHelpBuilder("Get an object by ID")
	help.WithDescriptionLines("Get an object by its ID.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("The object kind is inferred from the ID format (e.g., BLI-001 -> backlog_item).")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Default get returns raw CAS/spec fields (no resolved_* embeds). Optional --link-hydration")
	help.WithDescriptionLines("adds inline embeds; optional --fields keeps only those top-level keys (same hybrid projection as list).")
	help.AddExample("Get a backlog item", "%s get BLI-626")
	help.AddExample("Get with JSON output", "%s get BLI-626 --format json")
	help.AddExample("Get with YAML output", "%s get BLI-626 --format yaml")
	help.AddExample("Milestone completion report slice", "%s get MIL-001 --view milestone-completion-report")
	help.AddExample("JSON with selected keys only (projection)", "%s get PROMPT-001 --fields prompt_body --format json")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(1))
	builder.AddStringFlag("view", "", "", "Optional view name to scope output fields (default: full object with reference-resolver overlay).")
	builder.AddStringArrayFlag("fields", "", "Top-level keys to include in output (repeat or comma-separated); same hybrid projection as list. Omit for full object after overlays. Put -h before --fields if you need help so '-h' is not parsed as a field name.")
	builder.AddStringFlag("link-hydration", "", "", "Reference overlay: none/omit/raw (default) = no embeds; lazy = one hop; default = depth 2; eager = deeper.")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
