package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObjectBulkGetCommandBuilder creates a new object_bulk_get command
func NewObjectBulkGetCommandBuilder() *cobra.Command {
	builder := clipkg.NewCRUDCommandBuilder("read", "get")
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	builder.AddStringFlag("ids", "", "", "Comma-separated list of object IDs")
	builder.AddStringFlag("file", "", "", "Path to YAML file containing array of IDs")
	builder.AddStringFlag("view", "", "", "Optional view name to scope output fields per object.")
	builder.AddStringArrayFlag("fields", "", "Top-level keys per successful object in output (repeat or comma-separated); same hybrid projection as object get.")
	builder.AddStringFlag("link-hydration", "", "", "Reference overlay per object: none, lazy, default, or eager (same semantics as object get; default is none/raw).")
	builder.WithShort("Get multiple objects by ID")
	help := clipkg.DynamicHelpBuilder("Get multiple objects by ID")
	help.WithDescriptionLines("Get multiple objects by their IDs.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("You can provide IDs either:")
	help.WithDescriptionLines("  - Via --ids flag (comma-separated): --ids BLI-001,BLI-002,BLI-003")
	help.WithDescriptionLines("  - Via --file flag (YAML array of IDs): --file ids.yaml")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Full objects are loaded and overlays applied per object; optional --fields keeps only those")
	help.WithDescriptionLines("top-level keys in each successful result (same hybrid projection as object get / list).")
	help.AddExample("Get multiple objects by ID", "%s object bulk get --ids BLI-001,BLI-002,BLI-003")
	help.AddExample("Get from file", "%s object bulk get --file ids.yaml")
	help.AddExample("Output as JSON", "%s object bulk get --ids BLI-001,BLI-002 --format json")
	help.AddExample("JSON with selected keys only", "%s object bulk get --ids PROMPT-001,PROMPT-002 --fields id,kind,prompt_body --format json")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	return builder.Build()
}
