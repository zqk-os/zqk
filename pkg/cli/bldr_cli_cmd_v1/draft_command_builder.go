package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewDraftCommandBuilder creates a new draft command
func NewDraftCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("draft")
	builder.WithShort("Object draft-plane operations (enumerate / sweep / promote)")
	help := clipkg.DynamicHelpBuilder("Object draft-plane operations (enumerate / sweep / promote)")
	help.WithDescriptionLines("Operate on lifecycle-preliminary objects parked under .zqk/object_drafts/")
	help.WithDescriptionLines("(not visible to object list/count). Subcommands: sweep (delete drafts),")
	help.WithDescriptionLines("promote (materialize to CAS) — see CAS create membrane docs.")
	help.AddExample("Dry-run sweep all draft-plane objects", "%s object draft sweep --dry-run --all")
	help.AddExample("Dry-run promote all draft-plane objects", "%s object draft promote --dry-run --all")
	help.AddExample("Sweep draft backlog items older than 7d", "%s object draft sweep --kind backlog_item --older-than 168h --all")
	help.ExcludeFlag("columns")
	help.ExcludeFlag("ignore-scheduler-down")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.NoArgs)
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"columns", "ignore-scheduler-down"})
	cmd := builder.Build()
	return cmd
}
