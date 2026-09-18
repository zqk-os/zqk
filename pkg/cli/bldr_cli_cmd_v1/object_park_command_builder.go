package bldr_cli_cmd_v1

import (
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObjectParkCommandBuilder creates object park (lateral lifecycle exits: deferred/roadmap/archived).
// TRACK: promote/demote DNA parity — park follows sibling promote/demote builder shape until full CLI DNA land.
func NewObjectParkCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("park")
	builder.WithShort("Park one or more objects (deferred, roadmap, or archived)")
	help := clipkg.DynamicHelpBuilder("Park one or more objects along an intentional lifecycle exit")
	help.WithDescriptionLines(
		"Moves objects to deferred, roadmap, or archived when a lifecycle edge exists",
		"(including '*' → archived). Archive is the antithesis of the draft plane: the",
		"CRIT↔BLI↔REQ cluster (and PRI children when parking a plan) shockwaves together.",
		"If any hop is blocked, the whole tree stays on the prior membrane.",
		"Use this instead of promote/demote/--override for acknowledged-but-not-now or",
		"permanent history exits. Parents (goal/milestone/mission/vision) stay as lineage.",
	)
	help.AddExample("Defer an exploring idea", "%s object park BLI-… --to deferred")
	help.AddExample("Horizon-park validated work", "%s object park BLI-… --to roadmap")
	help.AddExample("Archive cluster (history) from any status", "%s object park CRIT-… --to archived")
	help.AddExample("Park many (comma-separated seeds; one unioned tree)", "%s object park BLI-001,BLI-002 --to deferred")
	help.WithDescriptionLines("IDs may be positional args, comma-separated (ID1,ID2), and/or --ids.")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	cmd := builder.Build()
	cmd.Use = "park <id>[,id...] [id...]"
	cmd.Flags().String("to", "", "Park target: deferred | roadmap | archived (required)")
	cmd.Flags().String("ids", "", "Comma-separated list of object IDs (same as positional multi-ID / comma lists)")
	_ = cmd.MarkFlagRequired("to")
	cmd.Args = cobra.MinimumNArgs(0)
	return cmd
}
