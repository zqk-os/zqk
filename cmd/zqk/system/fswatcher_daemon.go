package system

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/ambient"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// fswatcherDaemonCmd represents the fswatcher-daemon command
var fswatcherDaemonCmd *cobra.Command

// NewFSWatcherDaemonCmd creates the fswatcher-daemon command
func NewFSWatcherDaemonCmd() *cobra.Command {
	builder := clipkg.NewCommandBuilder("fswatcher-daemon")
	builder.WithShort("Start the FSWatcher daemon")

	help := clipkg.DynamicHelpBuilder("Start the FSWatcher daemon")
	help.WithDescriptionLines("Starts a robust filesystem watcher daemon to monitor workspace events and trigger ambient actions.")
	builder.WithHelpBuilder(help)

	cmd := builder.Build()
	cli.BindAsyncProgress(cmd, runFSWatcherDaemon)

	fswatcherDaemonCmd = cmd
	return cmd
}

func runFSWatcherDaemon(cmd *cobra.Command, args []string) error {
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == "" {
		return fmt.Errorf("project root not found")
	}

	_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Starting FSWatcher Daemon...\nMonitoring: %s\nPress Ctrl+C to stop.\n", projectRoot)))

	return ambient.RunWatcherDaemon(cmd.Context(), projectRoot, nil)
}
