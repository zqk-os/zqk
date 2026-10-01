package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewNewObjectSpecKindCommandBuilder creates a new new_object_spec_kind command
func NewNewObjectSpecKindCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("object-spec")
	builder.WithShort("Write a draft object kind spec YAML (object_specs shape, with inherited defaults)")
	help := clipkg.DynamicHelpBuilder("Write a draft object kind spec YAML (object_specs shape, with inherited defaults)")
	help.WithDescriptionLines("Materializes a starter file for .zqk/specs/objects/<ontology>.yaml.")
	help.WithDescriptionLines("Includes effective inherited top-level keys (e.g. storage_profile, traits) from the extends chain,")
	help.WithDescriptionLines("with comments explaining when to omit vs override. Next: edit fields, then generate-spec-index / validate.")
	help.AddExample("Draft kind spec extending base_object (stdout)", "%s new object-spec my_kind -o -")
	help.AddExample("Extend extensible_object instead", "%s new object-spec my_component --extends extensible_object -o -")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(1))
	builder.AddStringFlag("extends", "", "base_object", "Parent kind name (YAML stem under object_specs/, e.g. base_object, extensible_object)")
	builder.AddStringFlag("output", "o", "", "Output file path, or '-' for stdout (default: .zqk/drafts/<ontology>-spec-<timestamp>.yaml)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
