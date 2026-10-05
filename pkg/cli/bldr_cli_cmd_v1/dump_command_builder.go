package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewDumpCommandBuilder creates a new dump command
func NewDumpCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("dump")
	builder.WithShort("Capture process dump from the running scheduler daemon")
	help := clipkg.DynamicHelpBuilder("Capture process dump from the running scheduler daemon")
	help.WithDescriptionLines("Sends SIGUSR1 to the scheduler daemon to capture goroutine dump, heap profile,")
	help.WithDescriptionLines("and thread info to .zqk/scheduler/diagnostics/. Use this when threads or goroutines")
	help.WithDescriptionLines(paths.RewriteCanonicalCLIInvocations("are escalating. The daemon must be running (zqk scheduler start). Not supported"))
	help.WithDescriptionLines("on Windows (SIGUSR1 not available).")
	help.AddExample("Capture process dump from running daemon", "%s scheduler dump")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
