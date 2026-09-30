package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewObjectCreateCommandBuilder creates a new object_create command
func NewObjectCreateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCRUDCommandBuilder("create", "create <kind>")
	builder.WithDataInputFlags()
	builder.WithDryRunFlag()
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	builder.AddBoolFlag("keep-file", "", false, "Keep temporary scratch file after creation (files matching tmp-*.yaml are removed by default)")
	builder.AddBoolFlag("relaxed", "", false, "Relax integrity constraints during creation (allow references to objects that will be created later in batch operations) [Default: false]")
	builder.AddBoolFlag("force", "", false, "Force overwrite existing object (update instead of fail if object already exists)")
	builder.AddBoolFlag("interactive", "i", false, "Launch interactive wizard to create object")
	builder.AddBoolFlag("promote", "", false, "Promote object immediately to first shovel-ready lifecycle status (bypasses draft plane)")
	builder.AddBoolFlag("override", "", false, "Break-glass override to allow manual status assignment on create")
	builder.AddStringFlag("reason-code", "", "", "Reason code required when using --override")
	builder.WithShort("Create a new object")
	help := clipkg.DynamicHelpBuilder("Create a new object")
	help.WithDescriptionLines("Create a new object of the specified kind.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("The object data can be provided via:")
	help.WithDescriptionLines("  - --file: Path to a YAML file containing the object")
	help.WithDescriptionLines("  - --data: Inline YAML data")
	help.WithDescriptionLines("  - stdin: YAML data piped from another command")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Note: Temporary scratch files (matching tmp-*.yaml) are cleaned up automatically after creation.")
	help.WithDescriptionLines("User source files are left untouched. Use --keep-file to preserve scratch files.")
	help.AddExample("Create from file", "%s create backlog_item --file item.yaml")
	help.AddExample("Create from scratch template and keep it", "%s create backlog_item --file tmp-item.yaml --keep-file")
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
