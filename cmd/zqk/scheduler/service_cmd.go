package scheduler

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/cmd/zqk/ambient"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/scheduler/hostservice"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// NewServiceCmd manages per-root host OS scheduler units (launchd/systemd).
// DNA: .zqk/cli/specs/scheduler/service_command.yaml and scheduler/service/*.yaml
func NewServiceCmd() *cobra.Command {
	parent := bldr_cli_cmd_v1.NewServiceCommandBuilder()
	installCmd := bldr_cli_cmd_v1.NewSchedulerServiceInstallCommandBuilder()
	installCmd.RunE = runSchedulerServiceInstall
	listCmd := bldr_cli_cmd_v1.NewSchedulerServiceListCommandBuilder()
	listCmd.RunE = runSchedulerServiceList
	startCmd := bldr_cli_cmd_v1.NewSchedulerServiceStartCommandBuilder()
	startCmd.RunE = runSchedulerServiceStart
	stopCmd := bldr_cli_cmd_v1.NewSchedulerServiceStopCommandBuilder()
	stopCmd.RunE = runSchedulerServiceStop
	restartCmd := bldr_cli_cmd_v1.NewSchedulerServiceRestartCommandBuilder()
	restartCmd.RunE = runSchedulerServiceRestart
	uninstallCmd := bldr_cli_cmd_v1.NewSchedulerServiceUninstallCommandBuilder()
	uninstallCmd.RunE = runSchedulerServiceUninstall
	gcCmd := bldr_cli_cmd_v1.NewSchedulerServiceGcCommandBuilder()
	gcCmd.RunE = runSchedulerServiceGC
	rebindCmd := bldr_cli_cmd_v1.NewSchedulerServiceRebindCommandBuilder()
	rebindCmd.RunE = runSchedulerServiceRebind
	statusCmd := bldr_cli_cmd_v1.NewSchedulerServiceStatusCommandBuilder()
	statusCmd.RunE = runSchedulerServiceStatus
	parent.AddCommand(installCmd, listCmd, startCmd, stopCmd, restartCmd, uninstallCmd, gcCmd, rebindCmd, statusCmd)
	return parent
}

func requireSchedulerServiceControl(cmd *cobra.Command) error {
	// Privilege: host unit install/enable/start/stop require the system account or admin.
	if sec := pkgctx.GetSecurityContext(cmd.Context()); sec != nil {
		if sec.AccountID == pkgctx.SystemAccountID || slices.Contains(sec.Roles, "admin") {
			return nil
		}
	}
	if zqkenv.APIKey().Get() == pkgctx.SystemAccountID {
		return nil
	}
	if zqkenv.TestBypassAuth().Get() != "" || zqkenv.TestRoot().Get() != "" {
		return nil
	}
	return errfmt.Errorf("scheduler service control requires %s (set %s=%s)", pkgctx.SystemAccountID, zqkenv.APIKey(), pkgctx.SystemAccountID)
}

func parseServiceRoot(cmd *cobra.Command) (string, error) {
	var flags clipkg.FlagBag
	root := flags.String(cmd, "root")
	if err := flags.Err(); err != nil {
		return "", err
	}
	return serviceRootOrCwd(root), nil
}

func resolveControlledServiceEntry(cmd *cobra.Command) (hostservice.Entry, error) {
	if err := requireSchedulerServiceControl(cmd); err != nil {
		return hostservice.Entry{}, err
	}
	target, err := parseServiceRoot(cmd)
	if err != nil {
		return hostservice.Entry{}, err
	}
	return hostservice.ResolveEntry(target)
}

func executeSchedulerServiceTransition(cmd *cobra.Command, action string, desiredState string, op func(adapter hostservice.PlatformAdapter, entry hostservice.Entry) error) error {
	e, err := resolveControlledServiceEntry(cmd)
	if err != nil {
		return err
	}
	if err := op(hostservice.NewAdapter(), e); err != nil {
		return err
	}
	if _, err := hostservice.SetEntryDesiredState(e.RootID, desiredState); err != nil {
		return err
	}
	return cli.WriteOutput(cmd, []byte(fmt.Sprintf("%s %s\n", action, e.UnitLabel)))
}

func runSchedulerServiceInstall(cmd *cobra.Command, _ []string) error {
	if err := requireSchedulerServiceControl(cmd); err != nil {
		return err
	}
	abs, err := parseServiceRoot(cmd)
	if err != nil {
		return err
	}
	e, err := hostservice.InstallRoot(abs)
	if err != nil {
		return err
	}
	profile := string(pkgctx.ProfileSystem)
	if ctx := cli.GetContext(cmd); ctx != nil && ctx.Profile != "" {
		profile = ctx.Profile
	}
	logging.Fluent(logging.GetLoggerFromProfile(profile)).Info("scheduler host service installed").
		String("root_id", e.RootID).
		String("abs_root", e.AbsRoot).
		String("unit_label", e.UnitLabel).
		Log()
	return cli.WriteOutput(cmd, []byte(fmt.Sprintf("installed root_id=%s unit=%s binary=%s\n", e.RootID, e.UnitLabel, e.BinaryRef)))
}

func runSchedulerServiceList(cmd *cobra.Command, _ []string) error {
	var flags clipkg.FlagBag
	orphansOnly := flags.Bool(cmd, "orphans")
	if err := flags.Err(); err != nil {
		return err
	}
	var entries []hostservice.Entry
	var err error
	if orphansOnly {
		entries, err = hostservice.Orphans()
	} else {
		reg, loadErr := hostservice.LoadRegistry()
		err = loadErr
		if reg != nil {
			entries = reg.Entries
		}
	}
	if err != nil {
		return err
	}
	if entries == nil {
		entries = []hostservice.Entry{}
	}
	b, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return errfmt.Newf("marshal service list").Wrap(err)
	}
	return cli.WriteOutput(cmd, append(b, '\n'))
}

func runSchedulerServiceStart(cmd *cobra.Command, _ []string) error {
	return executeSchedulerServiceTransition(cmd, "started", hostservice.DesiredStateEnabled, func(a hostservice.PlatformAdapter, e hostservice.Entry) error {
		return a.Start(e)
	})
}

func runSchedulerServiceStop(cmd *cobra.Command, _ []string) error {
	return executeSchedulerServiceTransition(cmd, "stopped", hostservice.DesiredStateDisabled, func(a hostservice.PlatformAdapter, e hostservice.Entry) error {
		return a.Stop(e)
	})
}

func runSchedulerServiceRestart(cmd *cobra.Command, _ []string) error {
	return executeSchedulerServiceTransition(cmd, "restarted", hostservice.DesiredStateEnabled, func(a hostservice.PlatformAdapter, e hostservice.Entry) error {
		_ = a.Stop(e)
		return a.Start(e)
	})
}

func runSchedulerServiceUninstall(cmd *cobra.Command, _ []string) error {
	if err := requireSchedulerServiceControl(cmd); err != nil {
		return err
	}
	target, err := parseServiceRoot(cmd)
	if err != nil {
		return err
	}
	if err := hostservice.UninstallRoot(target); err != nil {
		return err
	}
	_ = ambient.StopDaemon(target)
	return cli.WriteOutput(cmd, []byte(fmt.Sprintf("uninstalled %s\n", target)))
}

func runSchedulerServiceGC(cmd *cobra.Command, _ []string) error {
	if err := requireSchedulerServiceControl(cmd); err != nil {
		return err
	}
	removed, err := hostservice.GC()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(removed, "", "  ")
	if err != nil {
		return errfmt.Newf("marshal gc result").Wrap(err)
	}
	return cli.WriteOutput(cmd, append(b, '\n'))
}

func runSchedulerServiceRebind(cmd *cobra.Command, _ []string) error {
	if err := requireSchedulerServiceControl(cmd); err != nil {
		return err
	}
	var flags clipkg.FlagBag
	rootID := flags.String(cmd, "root-id")
	newPath := flags.String(cmd, "new-path")
	if err := flags.Err(); err != nil {
		return err
	}
	if rootID == "" || newPath == "" {
		return errfmt.Errorf("--root-id and --new-path are required")
	}
	e, err := hostservice.Rebind(rootID, newPath)
	if err != nil {
		return err
	}
	return cli.WriteOutput(cmd, []byte(fmt.Sprintf("rebound root_id=%s abs_root=%s\n", e.RootID, e.AbsRoot)))
}

func runSchedulerServiceStatus(cmd *cobra.Command, _ []string) error {
	target, err := parseServiceRoot(cmd)
	if err != nil {
		return err
	}
	e, err := hostservice.ResolveEntry(target)
	if err != nil {
		return err
	}
	st, err := hostservice.NewAdapter().Status(e)
	if err != nil {
		return err
	}
	return cli.WriteOutput(cmd, []byte(fmt.Sprintf("root_id=%s desired=%s status=%s\n", e.RootID, e.DesiredState, st)))
}

func serviceRootOrCwd(root string) string {
	if root == "" {
		root = "."
	}
	resolved := cli.ResolveProjectRoot(root)
	if resolved == "" {
		abs, _ := filepath.Abs(root)
		return abs
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return resolved
	}
	return abs
}
