package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObjectMoveCommandBuilder creates a new object_move command
func NewObjectMoveCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("move")
	builder.WithShort("Move an object to a different directory/kind")
	help := clipkg.DynamicHelpBuilder("Move an object to a different directory/kind")
	help.WithDescriptionLines("Move an object to a different directory/kind while preserving history.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("This command allows reorganizing objects while maintaining system integrity:")
	help.WithDescriptionLines("  - Preserves created_at, created_by, and other history fields")
	help.WithDescriptionLines("  - Updates hash registry atomically")
	help.WithDescriptionLines("  - Creates audit event for move operation")
	help.WithDescriptionLines("  - Optionally updates references pointing to the moved object")
	help.AddExample("Move a backlog item to goal kind", "%s object move BLI-001 --kind goal")
	help.AddExample("Move and update references", "%s object move BLI-001 --kind goal --update-references")
	help.AddExample("Dry-run to see what would happen", "%s object move BLI-001 --kind goal --dry-run")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(1))
	builder.AddStringFlag("kind", "", "", "Target kind to move the object to")
	builder.AddBoolFlag("update-references", "", false, "Update references pointing to the moved object")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
