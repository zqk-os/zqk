package daemon

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/daemon/overseer"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
)

// NewDaemonCmd creates the root daemon command for process group supervision.
func NewDaemonCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Manage background daemons under unified process group supervision",
		"Unified Process Group Overseer for all ZQK background daemons (scheduler, ambient, fswatcher, steward, mcp).",
		"",
		"Ensures all daemons run under a dedicated POSIX process group with subreaper supervision,",
		"exponential backoff crash recovery, and unified desired vs actual state reconciliation.",
	).
		AddExample("Check daemon status matrix", "%s daemon status").
		AddExample("Start a daemon", "%s daemon start scheduler").
		AddExample("Stop a daemon", "%s daemon stop ambient").
		AddExample("Restart a daemon", "%s daemon restart scheduler").
		AddExample("Enable auto-start for a daemon", "%s daemon enable fswatcher").
		AddExample("Disable a daemon", "%s daemon disable mcp").
		AddExample("Run the overseer foreground supervisor", "%s daemon run").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage background daemons under unified process group supervision",
	}

	helpBuilder.ApplyToCommand(cmd)

	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newStartCmd())
	cmd.AddCommand(newStopCmd())
	cmd.AddCommand(newRestartCmd())
	cmd.AddCommand(newEnableCmd())
	cmd.AddCommand(newDisableCmd())
	cmd.AddCommand(newRunCmd())

	cli.AddCommonFlags(cmd)
	return cmd
}

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status [daemon]",
		Short: "Show status matrix of all managed daemons or a specific daemon",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot := cli.ResolveProjectRoot(".")
			sockPath := overseer.SocketPath(projectRoot)
			client := overseer.NewIPCClient(sockPath)

			var target string
			if len(args) > 0 {
				target = args[0]
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Second)
			defer cancel()

			if client.IsRunning() {
				resp, err := client.Send(ctx, overseer.IPCRequest{
					Action: "status",
					Target: target,
				})
				if err != nil {
					return errfmt.Newf("query overseer status").Wrap(err)
				}
				if !resp.Success {
					return errfmt.Errorf("%s", resp.Error)
				}
				return cli.FormatOutput(cmd, map[string]any{
					"overseer_running": true,
					"daemons":          resp.Daemons,
				})
			}

			// Overseer not running; fallback to reading persistent registry
			regPath := overseer.DefaultRegistryPath(projectRoot)
			reg := overseer.NewRegistry(regPath)
			if err := reg.Load(); err != nil {
				return errfmt.Newf("load daemon registry").Wrap(err)
			}

			specs := reg.List()
			var statuses []overseer.DaemonStatus
			for _, sp := range specs {
				if target != "" && sp.Name != target {
					continue
				}
				statuses = append(statuses, overseer.DaemonStatus{
					Name:         sp.Name,
					DesiredState: sp.DesiredState,
					ActualState:  overseer.ActualStateStopped,
				})
			}

			if target != "" && len(statuses) == 0 {
				return errfmt.Errorf("daemon %q not found in registry", target)
			}

			return cli.FormatOutput(cmd, map[string]any{
				"overseer_running": false,
				"daemons":          statuses,
			})
		},
	}
	cli.AddCommonFlags(cmd)
	return cmd
}

func newStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start <daemon>",
		Short: "Start a managed daemon and mark it as enabled",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			projectRoot := cli.ResolveProjectRoot(".")
			sockPath := overseer.SocketPath(projectRoot)
			client := overseer.NewIPCClient(sockPath)

			if !client.IsRunning() {
				// Update registry directly
				regPath := overseer.DefaultRegistryPath(projectRoot)
				reg := overseer.NewRegistry(regPath)
				if err := reg.Load(); err != nil {
					return errfmt.Newf("load registry").Wrap(err)
				}
				if err := reg.SetDesiredState(name, overseer.DesiredStateEnabled); err != nil {
					return err
				}
				return cli.FormatOutput(cmd, map[string]any{
					"daemon":  name,
					"desired": "enabled",
					"note":    "Overseer is not running; daemon will start when overseer runs.",
				})
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()

			resp, err := client.Send(ctx, overseer.IPCRequest{
				Action: "start",
				Target: name,
			})
			if err != nil {
				return errfmt.Newf("send start command").Wrap(err)
			}
			if !resp.Success {
				return errfmt.Errorf("%s", resp.Error)
			}
			return cli.FormatOutput(cmd, map[string]any{
				"status":  "success",
				"daemon":  name,
				"daemons": resp.Daemons,
			})
		},
	}
	cli.AddCommonFlags(cmd)
	return cmd
}

func newStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <daemon>",
		Short: "Stop a managed daemon and mark it as disabled",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			projectRoot := cli.ResolveProjectRoot(".")
			sockPath := overseer.SocketPath(projectRoot)
			client := overseer.NewIPCClient(sockPath)

			if !client.IsRunning() {
				regPath := overseer.DefaultRegistryPath(projectRoot)
				reg := overseer.NewRegistry(regPath)
				if err := reg.Load(); err != nil {
					return errfmt.Newf("load registry").Wrap(err)
				}
				if err := reg.SetDesiredState(name, overseer.DesiredStateDisabled); err != nil {
					return err
				}
				return cli.FormatOutput(cmd, map[string]any{
					"daemon":  name,
					"desired": "disabled",
				})
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()

			resp, err := client.Send(ctx, overseer.IPCRequest{
				Action: "stop",
				Target: name,
			})
			if err != nil {
				return errfmt.Newf("send stop command").Wrap(err)
			}
			if !resp.Success {
				return errfmt.Errorf("%s", resp.Error)
			}
			return cli.FormatOutput(cmd, map[string]any{
				"status":  "success",
				"daemon":  name,
				"daemons": resp.Daemons,
			})
		},
	}
	cli.AddCommonFlags(cmd)
	return cmd
}

func newRestartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restart <daemon>",
		Short: "Restart a managed daemon",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			projectRoot := cli.ResolveProjectRoot(".")
			sockPath := overseer.SocketPath(projectRoot)
			client := overseer.NewIPCClient(sockPath)

			if !client.IsRunning() {
				return errfmt.Errorf("cannot restart %q: overseer is not running", name)
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()

			resp, err := client.Send(ctx, overseer.IPCRequest{
				Action: "restart",
				Target: name,
			})
			if err != nil {
				return errfmt.Newf("send restart command").Wrap(err)
			}
			if !resp.Success {
				return errfmt.Errorf("%s", resp.Error)
			}
			return cli.FormatOutput(cmd, map[string]any{
				"status":  "success",
				"daemon":  name,
				"daemons": resp.Daemons,
			})
		},
	}
	cli.AddCommonFlags(cmd)
	return cmd
}

func newEnableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable <daemon>",
		Short: "Enable a daemon to start automatically under supervision",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			projectRoot := cli.ResolveProjectRoot(".")
			sockPath := overseer.SocketPath(projectRoot)
			client := overseer.NewIPCClient(sockPath)

			if client.IsRunning() {
				ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
				defer cancel()
				resp, err := client.Send(ctx, overseer.IPCRequest{Action: "enable", Target: name})
				if err != nil {
					return errfmt.Newf("send enable command").Wrap(err)
				}
				if !resp.Success {
					return errfmt.Errorf("%s", resp.Error)
				}
				return cli.FormatOutput(cmd, map[string]any{
					"status":  "success",
					"daemon":  name,
					"desired": "enabled",
				})
			}

			regPath := overseer.DefaultRegistryPath(projectRoot)
			reg := overseer.NewRegistry(regPath)
			if err := reg.Load(); err != nil {
				return errfmt.Newf("load registry").Wrap(err)
			}
			if err := reg.SetDesiredState(name, overseer.DesiredStateEnabled); err != nil {
				return err
			}
			return cli.FormatOutput(cmd, map[string]any{
				"daemon":  name,
				"desired": "enabled",
			})
		},
	}
	cli.AddCommonFlags(cmd)
	return cmd
}

func newDisableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable <daemon>",
		Short: "Disable a daemon under supervision",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			projectRoot := cli.ResolveProjectRoot(".")
			sockPath := overseer.SocketPath(projectRoot)
			client := overseer.NewIPCClient(sockPath)

			if client.IsRunning() {
				ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
				defer cancel()
				resp, err := client.Send(ctx, overseer.IPCRequest{Action: "disable", Target: name})
				if err != nil {
					return errfmt.Newf("send disable command").Wrap(err)
				}
				if !resp.Success {
					return errfmt.Errorf("%s", resp.Error)
				}
				return cli.FormatOutput(cmd, map[string]any{
					"status":  "success",
					"daemon":  name,
					"desired": "disabled",
				})
			}

			regPath := overseer.DefaultRegistryPath(projectRoot)
			reg := overseer.NewRegistry(regPath)
			if err := reg.Load(); err != nil {
				return errfmt.Newf("load registry").Wrap(err)
			}
			if err := reg.SetDesiredState(name, overseer.DesiredStateDisabled); err != nil {
				return err
			}
			return cli.FormatOutput(cmd, map[string]any{
				"daemon":  name,
				"desired": "disabled",
			})
		},
	}
	cli.AddCommonFlags(cmd)
	return cmd
}

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the overseer supervisor in the foreground as process group leader",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot := cli.ResolveProjectRoot(".")
			logger := logging.GetLoggerFromProfile("human")

			pgMgr, err := overseer.NewProcessGroupManager(true)
			if err != nil {
				return errfmt.Newf("initialize process group manager").Wrap(err)
			}

			if err := pgMgr.EnableSubreaper(); err != nil {
				logger.Warn(fmt.Sprintf("child subreaper not supported on this host: %v", err))
			}

			regPath := overseer.DefaultRegistryPath(projectRoot)
			reg := overseer.NewRegistry(regPath)
			if err := reg.Load(); err != nil {
				return errfmt.Newf("load daemon registry").Wrap(err)
			}

			sup := overseer.NewSupervisor(projectRoot, reg, pgMgr)

			sockPath := overseer.SocketPath(projectRoot)
			ipcServer := overseer.NewIPCServer(sockPath, sup)
			if err := ipcServer.Start(); err != nil {
				return errfmt.Newf("start IPC server").Wrap(err)
			}
			defer func() { _ = ipcServer.Stop() }()

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

			if err := sup.Start(ctx, 100*time.Millisecond); err != nil {
				return errfmt.Newf("start supervisor").Wrap(err)
			}

			logger.Info(fmt.Sprintf("Overseer running (PGID: %d, Socket: %s)", pgMgr.PGID(), sockPath))

			sig := <-sigCh
			logger.Info(fmt.Sprintf("Received signal %v, shutting down overseer and process group...", sig))

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()

			return sup.Stop(shutdownCtx)
		},
	}
	cli.AddCommonFlags(cmd)
	return cmd
}
