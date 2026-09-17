package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObjectRenameCommandBuilder creates a new object_rename command
func NewObjectRenameCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("rename")
	builder.WithShort("Rename an object (change its ID)")
	help := clipkg.DynamicHelpBuilder("Rename an object (change its ID)")
	help.WithDescriptionLines("Rename an object by changing its ID. Same kind; preserves history and updates hash registry.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Optionally updates all objects that reference the old ID to use the new ID (--update-references).")
	help.AddExample("Rename an object", "%s object rename BLI-001 BLI-XYZ")
	help.AddExample("Rename and update references", "%s object rename BLI-001 BLI-XYZ --update-references")
	help.AddExample("Dry-run to see what would happen", "%s object rename BLI-001 BLI-XYZ --dry-run")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(2))
	builder.AddBoolFlag("update-references", "", false, "Update all objects that reference the old ID to use the new ID")
	builder.AddBoolFlag("dry-run", "", false, "Show what would be renamed without making changes")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
