package scheduler

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
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
	uninstallCmd := bldr_cli_cmd_v1.NewSchedulerServiceUninstallCommandBuilder()
	uninstallCmd.RunE = runSchedulerServiceUninstall
	gcCmd := bldr_cli_cmd_v1.NewSchedulerServiceGcCommandBuilder()
	gcCmd.RunE = runSchedulerServiceGC
	rebindCmd := bldr_cli_cmd_v1.NewSchedulerServiceRebindCommandBuilder()
	rebindCmd.RunE = runSchedulerServiceRebind
	statusCmd := bldr_cli_cmd_v1.NewSchedulerServiceStatusCommandBuilder()
	statusCmd.RunE = runSchedulerServiceStatus
	parent.AddCommand(installCmd, listCmd, startCmd, stopCmd, uninstallCmd, gcCmd, rebindCmd, statusCmd)
	return parent
}

func requireSchedulerServiceControl(cmd *cobra.Command) error {
	// Privilege: host unit install/enable/start/stop require the system account.
	if sec := pkgctx.GetSecurityContext(cmd.Context()); sec != nil && sec.AccountID == pkgctx.SystemAccountID {
		return nil
	}
	if zqkenv.APIKey().Get() == pkgctx.SystemAccountID {
		return nil
	}
	return errfmt.Errorf("scheduler service control requires %s (set %s=%s)", pkgctx.SystemAccountID, zqkenv.APIKey(), pkgctx.SystemAccountID)
}

func runSchedulerServiceInstall(cmd *cobra.Command, _ []string) error {
	if err := requireSchedulerServiceControl(cmd); err != nil {
		return err
	}
	var flags clipkg.FlagBag
	root := flags.String(cmd, "root")
	if err := flags.Err(); err != nil {
		return err
	}
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(cli.ResolveProjectRoot(root))
	if err != nil {
		return errfmt.Newf("resolve project root").Wrap(err)
	}
	if abs == "" {
		abs, _ = filepath.Abs(root)
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
	if err := requireSchedulerServiceControl(cmd); err != nil {
		return err
	}
	var flags clipkg.FlagBag
	root := flags.String(cmd, "root")
	if err := flags.Err(); err != nil {
		return err
	}
	e, err := hostservice.ResolveEntry(serviceRootOrCwd(root))
	if err != nil {
		return err
	}
	if err := hostservice.NewAdapter().Start(e); err != nil {
		return err
	}
	if _, err := hostservice.SetEntryDesiredState(e.RootID, hostservice.DesiredStateEnabled); err != nil {
		return err
	}
	return cli.WriteOutput(cmd, []byte(fmt.Sprintf("started %s\n", e.UnitLabel)))
}

func runSchedulerServiceStop(cmd *cobra.Command, _ []string) error {
	if err := requireSchedulerServiceControl(cmd); err != nil {
		return err
	}
	var flags clipkg.FlagBag
	root := flags.String(cmd, "root")
	if err := flags.Err(); err != nil {
		return err
	}
	e, err := hostservice.ResolveEntry(serviceRootOrCwd(root))
	if err != nil {
		return err
	}
	if err := hostservice.NewAdapter().Stop(e); err != nil {
		return err
	}
	if _, err := hostservice.SetEntryDesiredState(e.RootID, hostservice.DesiredStateDisabled); err != nil {
		return err
	}
	return cli.WriteOutput(cmd, []byte(fmt.Sprintf("stopped %s\n", e.UnitLabel)))
}

func runSchedulerServiceUninstall(cmd *cobra.Command, _ []string) error {
	if err := requireSchedulerServiceControl(cmd); err != nil {
		return err
	}
	var flags clipkg.FlagBag
	root := flags.String(cmd, "root")
	if err := flags.Err(); err != nil {
		return err
	}
	target := serviceRootOrCwd(root)
	if err := hostservice.UninstallRoot(target); err != nil {
		return err
	}
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
	var flags clipkg.FlagBag
	root := flags.String(cmd, "root")
	if err := flags.Err(); err != nil {
		return err
	}
	e, err := hostservice.ResolveEntry(serviceRootOrCwd(root))
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
