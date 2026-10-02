package system

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/ambient"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
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

	ctx := cmd.Context()

	// Initialize the ambient event hub
	hub := ambient.NewEventHub()

	// Initialize heuristic processors and writers to listen on the hub
	_ = ambient.NewArtifactWriter(hub)
	_ = ambient.NewCoachHeuristics(hub)

	// Initialize ambient ingest service to map events back to discrete system objects
	secCtx := pkgctx.NewSecurityContext(pkgctx.SystemAccountID, []string{"system"}, []string{"*"})
	ingestService := ambient.NewAmbientIngestService(projectRoot, secCtx)
	ingestService.BindToHub(hub)

	// Create and start the FSWatcher
	watcher, err := ambient.NewFSWatcher(projectRoot, hub)
	if err != nil {
		return fmt.Errorf("failed to create fswatcher: %w", err)
	}

	// Bind cache invalidation to filesystem events
	ambient.BindProcessYAMLInvalidator(hub, nil)

	if err := watcher.Start(ctx); err != nil {
		return fmt.Errorf("failed to start fswatcher: %w", err)
	}
	defer func() { _ = watcher.Stop() }()

	// Wait for termination
	<-ctx.Done()

	return nil
}
