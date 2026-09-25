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
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/daemon/overseer"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
)

// NewDaemonCmd creates the root daemon command for process group supervision.
func NewDaemonCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonCommandBuilder()
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newStartCmd())
	cmd.AddCommand(newStopCmd())
	cmd.AddCommand(newRestartCmd())
	cmd.AddCommand(newEnableCmd())
	cmd.AddCommand(newDisableCmd())
	cmd.AddCommand(newAddCmd())
	cmd.AddCommand(newRemoveCmd())
	cmd.AddCommand(newRunCmd())
	return cmd
}

func resolveProjectRoot(cmd *cobra.Command) string {
	if cmd != nil {
		if c := cli.GetContext(cmd); c != nil && c.ProjectRoot != "" {
			return c.ProjectRoot
		}
	}
	return cli.ResolveProjectRoot(".")
}

func newStatusCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonStatusCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
			projectRoot := resolveProjectRoot(cmd)
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
	}
	cli.AddCommonFlags(cmd)
	return cmd
}

func newStartCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonStartCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		name := args[0]
		projectRoot := resolveProjectRoot(cmd)
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
	}
	return cmd
}

func newStopCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonStopCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		name := args[0]
		projectRoot := resolveProjectRoot(cmd)
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
	}
	return cmd
}

func newRestartCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonRestartCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		name := args[0]
		projectRoot := resolveProjectRoot(cmd)
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
	}
	return cmd
}

func newEnableCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonEnableCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		name := args[0]
		projectRoot := resolveProjectRoot(cmd)
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
	}
	return cmd
}

func newDisableCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonDisableCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		name := args[0]
		projectRoot := resolveProjectRoot(cmd)
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
	}
	return cmd
}

func newAddCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonAddCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		name := args[0]
		commandArgs := args[1:]

		description, _ := cmd.Flags().GetString("description")
		restartPolicyStr, _ := cmd.Flags().GetString("restart-policy")
		maxRestarts, _ := cmd.Flags().GetInt("max-restarts")
		workingDir, _ := cmd.Flags().GetString("working-dir")
		enabled, _ := cmd.Flags().GetBool("enabled")

		desiredState := overseer.DesiredStateDisabled
		if enabled {
			desiredState = overseer.DesiredStateEnabled
		}

		restartPolicy := overseer.RestartPolicy(restartPolicyStr)
		if restartPolicy != overseer.RestartPolicyAlways &&
			restartPolicy != overseer.RestartPolicyOnFailure &&
			restartPolicy != overseer.RestartPolicyNever {
			return errfmt.Errorf("invalid restart-policy %q: must be always, on-failure, or never", restartPolicyStr)
		}

		spec := &overseer.DaemonSpec{
			Name:          name,
			Description:   description,
			Command:       commandArgs,
			DesiredState:  desiredState,
			RestartPolicy: restartPolicy,
			MaxRestarts:   maxRestarts,
			BackoffMin:    1 * time.Second,
			BackoffMax:    30 * time.Second,
			WorkingDir:    workingDir,
		}

		projectRoot := resolveProjectRoot(cmd)
		sockPath := overseer.SocketPath(projectRoot)
		client := overseer.NewIPCClient(sockPath)

		if !client.IsRunning() {
			regPath := overseer.DefaultRegistryPath(projectRoot)
			reg := overseer.NewRegistry(regPath)
			if err := reg.Load(); err != nil {
				return errfmt.Newf("load registry").Wrap(err)
			}
			if err := reg.Set(spec); err != nil {
				return err
			}
			return cli.FormatOutput(cmd, map[string]any{
				"daemon":  name,
				"status":  "added",
				"desired": string(desiredState),
				"note":    "Overseer is not running; daemon configuration saved to registry.",
			})
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
		defer cancel()

		resp, err := client.Send(ctx, overseer.IPCRequest{
			Action: "add",
			Spec:   spec,
		})
		if err != nil {
			return errfmt.Newf("send add command").Wrap(err)
		}
		if !resp.Success {
			return errfmt.Errorf("%s", resp.Error)
		}
		return cli.FormatOutput(cmd, map[string]any{
			"status":  "success",
			"daemon":  name,
			"daemons": resp.Daemons,
		})
	}
	return cmd
}

func newRemoveCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonRemoveCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		name := args[0]
		projectRoot := resolveProjectRoot(cmd)
		sockPath := overseer.SocketPath(projectRoot)
		client := overseer.NewIPCClient(sockPath)

		if !client.IsRunning() {
			regPath := overseer.DefaultRegistryPath(projectRoot)
			reg := overseer.NewRegistry(regPath)
			if err := reg.Load(); err != nil {
				return errfmt.Newf("load registry").Wrap(err)
			}
			if err := reg.Delete(name); err != nil {
				return err
			}
			return cli.FormatOutput(cmd, map[string]any{
				"daemon": name,
				"status": "removed",
			})
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
		defer cancel()

		resp, err := client.Send(ctx, overseer.IPCRequest{
			Action: "remove",
			Target: name,
		})
		if err != nil {
			return errfmt.Newf("send remove command").Wrap(err)
		}
		if !resp.Success {
			return errfmt.Errorf("%s", resp.Error)
		}
		return cli.FormatOutput(cmd, map[string]any{
			"status": "success",
			"daemon": name,
		})
	}
	return cmd
}

func newRunCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonRunCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		projectRoot := resolveProjectRoot(cmd)
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
	}
	return cmd
}
