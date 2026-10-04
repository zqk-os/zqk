package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/cmd/zqk/ambient"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/daemon/overseer"
	"github.com/zqk-os/zqk/pkg/daemon/singleton"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/scheduler/hostservice"
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
	cmd.AddCommand(newServiceCmd())
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

func getDaemonClient(cmd *cobra.Command) (string, *overseer.IPCClient) {
	projectRoot := resolveProjectRoot(cmd)
	sockPath := overseer.SocketPath(projectRoot)
	return projectRoot, overseer.NewIPCClient(sockPath)
}

func sendOverseerRequest(ctx context.Context, client *overseer.IPCClient, req overseer.IPCRequest) (*overseer.IPCResponse, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resp, err := client.Send(reqCtx, req)
	if err != nil {
		return nil, errfmt.Newf("send %s command", req.Action).Wrap(err)
	}
	if !resp.Success {
		return nil, errfmt.Errorf("%s", resp.Error)
	}
	return resp, nil
}

func sendOverseerAction(ctx context.Context, client *overseer.IPCClient, action, target string) (*overseer.IPCResponse, error) {
	return sendOverseerRequest(ctx, client, overseer.IPCRequest{
		Action: action,
		Target: target,
	})
}

func loadDaemonRegistry(projectRoot string) (*overseer.Registry, error) {
	regPath := overseer.DefaultRegistryPath(projectRoot)
	reg := overseer.NewRegistry(regPath)
	if err := reg.Load(); err != nil {
		return nil, errfmt.Newf("load daemon registry").Wrap(err)
	}
	return reg, nil
}

func formatDaemonSuccessOutput(cmd *cobra.Command, name string, daemons any) error {
	return cli.FormatOutput(cmd, map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusSuccess,
		"daemon":               name,
		"daemons":              daemons,
	})
}

func handleDaemonStateToggle(cmd *cobra.Command, name string, desired overseer.DesiredState) error {
	projectRoot, client := getDaemonClient(cmd)
	stateStr := string(desired)
	if client.IsRunning() {
		if _, err := sendOverseerAction(cmd.Context(), client, stateStr, name); err != nil {
			return err
		}
		return cli.FormatOutput(cmd, map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusSuccess,
			"daemon":               name,
			"desired":              stateStr,
		})
	}

	if err := setDesiredStateOffline(projectRoot, name, desired); err != nil {
		return err
	}
	return cli.FormatOutput(cmd, map[string]any{
		"daemon":  name,
		"desired": stateStr,
	})
}

func setDesiredStateOffline(projectRoot, name string, state overseer.DesiredState) error {
	reg, err := loadDaemonRegistry(projectRoot)
	if err != nil {
		return err
	}
	return reg.SetDesiredState(name, state)
}

func newStatusCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonStatusCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		projectRoot, client := getDaemonClient(cmd)

		var target string
		if len(args) > 0 {
			target = args[0]
		}

		if client.IsRunning() {
			resp, err := sendOverseerAction(cmd.Context(), client, "status", target)
			if err != nil {
				return err
			}
			return cli.FormatOutput(cmd, map[string]any{
				"overseer_running": true,
				"daemons":          resp.Daemons,
			})
		}

		// Overseer not running; fallback to reading persistent registry
		reg, err := loadDaemonRegistry(projectRoot)
		if err != nil {
			return err
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
		projectRoot, client := getDaemonClient(cmd)

		if !client.IsRunning() {
			if err := setDesiredStateOffline(projectRoot, name, overseer.DesiredStateEnabled); err != nil {
				return err
			}
			return cli.FormatOutput(cmd, map[string]any{
				"daemon":  name,
				"desired": "enabled",
				"note":    "Overseer is not running; daemon will start when overseer runs.",
			})
		}

		resp, err := sendOverseerAction(cmd.Context(), client, "start", name)
		if err != nil {
			return err
		}
		return formatDaemonSuccessOutput(cmd, name, resp.Daemons)
	}
	return cmd
}

func newStopCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonStopCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		name := args[0]
		projectRoot, client := getDaemonClient(cmd)

		if !client.IsRunning() {
			if err := setDesiredStateOffline(projectRoot, name, overseer.DesiredStateDisabled); err != nil {
				return err
			}
			return cli.FormatOutput(cmd, map[string]any{
				"daemon":  name,
				"desired": "disabled",
			})
		}

		resp, err := sendOverseerAction(cmd.Context(), client, "stop", name)
		if err != nil {
			return err
		}
		return formatDaemonSuccessOutput(cmd, name, resp.Daemons)
	}
	return cmd
}

func newRestartCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonRestartCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		name := args[0]
		_, client := getDaemonClient(cmd)

		if !client.IsRunning() {
			return errfmt.Errorf("cannot restart %q: overseer is not running", name)
		}

		resp, err := sendOverseerAction(cmd.Context(), client, "restart", name)
		if err != nil {
			return err
		}
		return formatDaemonSuccessOutput(cmd, name, resp.Daemons)
	}
	return cmd
}

func newEnableCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonEnableCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return handleDaemonStateToggle(cmd, args[0], overseer.DesiredStateEnabled)
	}
	cli.AddCommonFlags(cmd)
	return cmd
}

func newDisableCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonDisableCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return handleDaemonStateToggle(cmd, args[0], overseer.DesiredStateDisabled)
	}
	cli.AddCommonFlags(cmd)
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

		projectRoot, client := getDaemonClient(cmd)

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
				"daemon":               name,
				objects.FieldKeyStatus: objects.ObjectStatusAdded,
				"desired":              string(desiredState),
				"note":                 "Overseer is not running; daemon configuration saved to registry.",
			})
		}

		resp, err := sendOverseerRequest(cmd.Context(), client, overseer.IPCRequest{
			Action: "add",
			Spec:   spec,
		})
		if err != nil {
			return err
		}
		return formatDaemonSuccessOutput(cmd, name, resp.Daemons)
	}
	return cmd
}

func newRemoveCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonRemoveCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		name := args[0]
		projectRoot, client := getDaemonClient(cmd)

		if !client.IsRunning() {
			reg, err := loadDaemonRegistry(projectRoot)
			if err != nil {
				return err
			}
			if err := reg.Delete(name); err != nil {
				return err
			}
			return cli.FormatOutput(cmd, map[string]any{
				"daemon":               name,
				objects.FieldKeyStatus: objects.ObjectStatusRemoved,
			})
		}

		if _, err := sendOverseerAction(cmd.Context(), client, "remove", name); err != nil {
			return err
		}
		return cli.FormatOutput(cmd, map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusSuccess,
			"daemon":               name,
		})
	}
	return cmd
}

func newRunCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonRunCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		projectRoot := resolveProjectRoot(cmd)
		logger := logging.GetLoggerFromProfile("human")

		releaseLock, err := singleton.Guard(projectRoot, "overseer")
		if err != nil {
			var alreadyRunning *singleton.ErrDaemonAlreadyRunning
			if errors.As(err, &alreadyRunning) {
				_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Overseer daemon is already running for %s (PID: %d). Existing instance retained.\n", projectRoot, alreadyRunning.PID)))
				return nil
			}
			return err
		}
		defer releaseLock()

		pgMgr, err := overseer.NewProcessGroupManager(true)
		if err != nil {
			return errfmt.Newf("initialize process group manager").Wrap(err)
		}

		if err := pgMgr.EnableSubreaper(); err != nil {
			logger.Warn(fmt.Sprintf("child subreaper not supported on this host: %v", err))
		}

		reg, err := loadDaemonRegistry(projectRoot)
		if err != nil {
			return err
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

func newServiceCmd() *cobra.Command {
	parent := bldr_cli_cmd_v1.NewDaemonServiceCommandBuilder()
	parent.AddCommand(newServiceInstallCmd())
	parent.AddCommand(newServiceUninstallCmd())
	parent.AddCommand(newServiceStatusCmd())
	parent.AddCommand(newServiceCleanupLegacyCmd())
	return parent
}

func newServiceInstallCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonServiceInstallCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		root, _ := cmd.Flags().GetString("root")
		if root == "" {
			root = resolveProjectRoot(cmd)
		}
		bin, _ := cmd.Flags().GetString("binary")

		st, err := overseer.InstallOverseerLaunchAgent(root, bin)
		if err != nil {
			return err
		}
		return cli.FormatOutput(cmd, map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusInstalled,
			"label":                st.Label,
			"plist_path":           st.PlistPath,
			"cleaned_legacy_units": st.CleanedUnits,
			"running":              st.Running,
			"pid":                  st.PID,
			"note":                 "Solitary overseer LaunchAgent active. All daemons run under its process group.",
		})
	}
	return cmd
}

func newServiceUninstallCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonServiceUninstallCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		projectRoot := resolveProjectRoot(cmd)

		// 1. Stop all project daemons (ambient, scheduler host unit) so nothing is left behind
		if projectRoot != "" {
			_ = ambient.StopDaemon(projectRoot)
			_ = hostservice.UninstallRoot(projectRoot)
		}

		// 2. Stop overseer process group if active
		if _, client := getDaemonClient(cmd); client != nil {
			ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Second)
			_, _ = sendOverseerAction(ctx, client, "shutdown", "")
			cancel()
		}

		// 3. Uninstall overseer LaunchAgent
		if err := overseer.UninstallOverseerLaunchAgent(); err != nil {
			return err
		}

		// 4. Clean up any legacy LaunchAgents / units
		cleaned, _ := overseer.CleanLegacyLaunchAgents()

		return cli.FormatOutput(cmd, map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusUninstalled,
			"label":                overseer.OverseerLaunchAgentLabel,
			"project_root":         projectRoot,
			"cleaned_legacy_units": cleaned,
		})
	}
	return cmd
}

func newServiceStatusCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonServiceStatusCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		st, err := overseer.StatusOverseerLaunchAgent()
		if err != nil {
			return err
		}
		return cli.FormatOutput(cmd, map[string]any{
			"label":      st.Label,
			"installed":  st.Installed,
			"loaded":     st.Loaded,
			"running":    st.Running,
			"pid":        st.PID,
			"plist_path": st.PlistPath,
		})
	}
	return cmd
}

func newServiceCleanupLegacyCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDaemonServiceCleanupLegacyCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		cleaned, err := overseer.CleanLegacyLaunchAgents()
		if err != nil {
			return err
		}
		return cli.FormatOutput(cmd, map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusCleaned,
			"cleaned_legacy_units": cleaned,
			"count":                len(cleaned),
		})
	}
	return cmd
}
