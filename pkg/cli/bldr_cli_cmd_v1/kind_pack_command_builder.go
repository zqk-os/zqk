package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewKindPackCommandBuilder creates a new kind-pack command
func NewKindPackCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("kind-pack")
	builder.WithShort("Record a verified spec pack as typed object kinds")
	help := clipkg.DynamicHelpBuilder("Record a verified spec pack as typed object kinds")
	help.WithDescriptionLines("Record a verified spec pack as typed object kinds so its kinds load as objects.")
	help.AddExample("Install a verified spec pack", "zqk kind-pack install packs/work")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	cmd := builder.Build()
	return cmd
}
