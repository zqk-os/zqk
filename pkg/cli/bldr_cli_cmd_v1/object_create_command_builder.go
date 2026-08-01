package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObjectCreateCommandBuilder creates a new object_create command
func NewObjectCreateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCRUDCommandBuilder("create", "create <kind>")
	builder.WithDataInputFlags()
	builder.WithDryRunFlag()
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	builder.AddBoolFlag("keep-file", "", false, "Keep the source file after creation (default: files are removed automatically)")
	builder.AddBoolFlag("relaxed", "", true, "Relax integrity constraints during creation (allow references to objects that will be created later in batch operations) [Default: true]")
	builder.AddBoolFlag("force", "", false, "Force overwrite existing object (update instead of fail if object already exists)")
	builder.AddBoolFlag("interactive", "i", false, "Launch interactive wizard to create object")
	builder.WithShort("Create a new object")
	help := clipkg.DynamicHelpBuilder("Create a new object")
	help.WithDescriptionLines("Create a new object of the specified kind.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("The object data can be provided via:")
	help.WithDescriptionLines("  - --file: Path to a YAML file containing the object")
	help.WithDescriptionLines("  - --data: Inline YAML data")
	help.WithDescriptionLines("  - stdin: YAML data piped from another command")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Note: Source files are automatically removed after successful creation.")
	help.WithDescriptionLines("Use --keep-file to preserve the source file.")
	help.AddExample("Create from file (file auto-removed)", "%s create backlog_item --file item.yaml")
	help.AddExample("Create from file and keep it", "%s create backlog_item --file item.yaml --keep-file")
	help.AddExample("Create from inline data", "%s create backlog_item --data 'title: \"New Item\"'")
	help.AddExample("Create from stdin", "cat item.yaml | %s create backlog_item")
	help.AddExample("Generate template, edit, and create", "%s object template requirement --output tmp-req.yaml && $EDITOR tmp-req.yaml && %s object create requirement --file tmp-req.yaml")
	help.AddExample("See available fields", "%s object backlog_item fields")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	return builder.Build()
}
