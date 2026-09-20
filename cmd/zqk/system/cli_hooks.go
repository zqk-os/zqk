package system

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/clihooks"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	traypkg "github.com/zqk-os/zqk/pkg/tray"
)

// Subcommand usage fragments (after "cli-hooks ") and hints — align with
// .zqk/cli/specs/system/cli_hooks_command.yaml when changing.
const (
	cliHooksUsageGet            = "get <hook-id>"
	cliHooksUsageEnable         = "enable <hook-id>"
	cliHooksUsageDisable        = "disable <hook-id>"
	cliHooksUsageSetTray        = "set-tray <hook-id> <tray-entry>|--clear"
	cliHooksUsageIsEnabled      = "is-enabled <hook-id>"
	cliHooksUsagePrintTrayEntry = "print-tray-entry <hook-id>"

	cliHooksSubcommandsHint = "list, get, enable, disable, set-tray, is-enabled, print-tray-entry"
)

// requireCliHooksArgs returns nil when len(args) >= want; otherwise a usage error.
func requireCliHooksArgs(args []string, want int, usageRest string) error {
	if len(args) < want {
		return errfmt.Errorf("usage: cli-hooks %s", usageRest)
	}
	return nil
}

// runCliHooks implements zqk system cli-hooks (list, get, enable, disable, set-tray, is-enabled, print-tray-entry).
// Canonical description and examples: .zqk/cli/specs/system/cli_hooks_command.yaml
func runCliHooks(cmd *cobra.Command, ctx *cli.Context, args []string) error {
	projectRoot := ProjectRootOrResolve(ctx.ProjectRoot)
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}
	prof := clihooks.NewProfile(projectRoot)
	if err := prof.Load(); err != nil {
		return err
	}

	if len(args) == 0 {
		return listCliHooks(cmd, prof, projectRoot)
	}

	switch args[0] {
	case "list":
		return listCliHooks(cmd, prof, projectRoot)
	case "get":
		if err := requireCliHooksArgs(args, 2, cliHooksUsageGet); err != nil {
			return err
		}
		return getCliHook(cmd, prof, args[1])
	case "enable":
		if err := requireCliHooksArgs(args, 2, cliHooksUsageEnable); err != nil {
			return err
		}
		return mutateCliHook(cmd, prof, args[1], true, projectRoot)
	case "disable":
		if err := requireCliHooksArgs(args, 2, cliHooksUsageDisable); err != nil {
			return err
		}
		return mutateCliHook(cmd, prof, args[1], false, projectRoot)
	case "set-tray":
		if err := requireCliHooksArgs(args, 3, cliHooksUsageSetTray); err != nil {
			return err
		}
		trayArg := args[2]
		if trayArg == "--clear" {
			trayArg = ""
		}
		return setCliHookTray(cmd, prof, args[1], trayArg, projectRoot)
	case "is-enabled":
		if err := requireCliHooksArgs(args, 2, cliHooksUsageIsEnabled); err != nil {
			return err
		}
		if !prof.Has(args[1]) {
			return errfmt.Errorf("unknown cli hook %q", args[1])
		}
		if !prof.IsEnabled(args[1]) {
			os.Exit(1)
		}
		return nil
	case "print-tray-entry":
		if err := requireCliHooksArgs(args, 2, cliHooksUsagePrintTrayEntry); err != nil {
			return err
		}
		return printTrayEntryCLI(cmd, prof, args[1])
	default:
		return errfmt.Errorf("unknown cli-hooks subcommand %q (try: %s)", args[0], cliHooksSubcommandsHint)
	}
}

func listCliHooks(cmd *cobra.Command, prof *clihooks.Profile, projectRoot string) error {
	hooks := prof.List()
	rows := make([]map[string]any, 0, len(hooks))
	for _, h := range hooks {
		rows = append(rows, map[string]any{
			objects.FieldKeyID:          h.ID,
			objects.FieldKeyDescription: h.Description,
			objects.FieldKeyEnabled:     h.Enabled,
			"tray_entry":                h.TrayEntry,
		})
	}
	return cli.FormatOutput(cmd, map[string]any{
		"hooks":            rows,
		"project_root":     projectRoot,
		"config_path":      datacell.RuntimeOrganismMembraneReadPaths(projectRoot).CLIHookProfilePath(),
		"protocol_version": clihooks.ProtocolVersion,
	})
}

// printTrayEntryCLI writes only the tray entry name via WriteOutput (machine-oriented; for shell $(...)).
// Empty hook or unknown id is an error; empty tray_entry prints a single newline.
func printTrayEntryCLI(cmd *cobra.Command, prof *clihooks.Profile, id string) error {
	h := prof.Get(id)
	if h == nil {
		return errfmt.Errorf("unknown cli hook %q", id)
	}
	line := h.TrayEntry + "\n"
	return cli.WriteOutput(cmd, []byte(line))
}

func getCliHook(cmd *cobra.Command, prof *clihooks.Profile, id string) error {
	h := prof.Get(id)
	if h == nil {
		return errfmt.Errorf("unknown cli hook %q", id)
	}
	return cli.FormatOutput(cmd, map[string]any{
		objects.FieldKeyID:          h.ID,
		objects.FieldKeyDescription: h.Description,
		objects.FieldKeyEnabled:     h.Enabled,
		"tray_entry":                h.TrayEntry,
		"protocol_version":          clihooks.ProtocolVersion,
	})
}

func mutateCliHook(cmd *cobra.Command, prof *clihooks.Profile, id string, enabled bool, projectRoot string) error {
	if err := requireCliHookWritePermission(); err != nil {
		return err
	}
	if err := prof.SetEnabled(id, enabled); err != nil {
		return err
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	logging.Fluent(logger).Info("cli hook updated").
		String("hook_id", id).
		String("enabled", fmt.Sprintf("%v", enabled)).
		ProjectRoot(projectRoot).
		Log()
	return cli.FormatOutput(cmd, map[string]any{
		objects.FieldKeyID:      id,
		objects.FieldKeyEnabled: enabled,
		"ok":                    true,
	})
}

func setCliHookTray(cmd *cobra.Command, prof *clihooks.Profile, id, trayEntry, projectRoot string) error {
	if err := requireCliHookWritePermission(); err != nil {
		return err
	}
	if trayEntry != "" {
		entries, err := traypkg.Load(projectRoot)
		if err != nil {
			return err
		}
		if traypkg.Find(entries, trayEntry) == nil {
			return errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("unknown tray entry %q (see: zqk tray list)", trayEntry)))
		}
	}
	if err := prof.SetTrayEntry(id, trayEntry); err != nil {
		return err
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	logging.Fluent(logger).Info("cli hook tray entry updated").
		String("hook_id", id).
		String("tray_entry", trayEntry).
		ProjectRoot(projectRoot).
		Log()
	return cli.FormatOutput(cmd, map[string]any{
		objects.FieldKeyID: id,
		"tray_entry":       trayEntry,
		"ok":               true,
	})
}

func requireCliHookWritePermission() error {
	secCtx := getSecurityContextForFeatureFlags()
	if !hasPermission(secCtx, "write:system") && !hasPermission(secCtx, "write:config") {
		return errfmt.Errorf("insufficient permissions: cli-hooks changes require write:system or write:config. Current account: %s",
			secCtx.AccountID)
	}
	return nil
}

// NewCliHooksCmd creates the cli-hooks subcommand (built-in hook profile + optional tray binding).
func NewCliHooksCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"CLI hook profile (built-in hooks + optional tray entry)",
		"Manage built-in automation hooks the same way as feature flags: persisted JSON under .zqk/config/cli_hook_profile.json.",
		"", paths.RewriteCanonicalCLIInvocations("Hooks are stable IDs (e.g. post_commit_scan_tests) that git hooks or scripts can query. Optional tray_entry points at a zqk tray name for delegated commands."),
	).
		AddExample("List hooks", "%s system cli-hooks list").
		AddExample("Disable post-commit test scheduling", "%s system cli-hooks disable post_commit_scan_tests").
		AddExample("Shell: exit 1 if hook disabled", "%s system cli-hooks is-enabled post_commit_scan_tests").
		AddExample("Machine: tray name for shell capture", "%s system cli-hooks print-tray-entry post_commit_scan_tests").
		AddExample("Bind a tray entry", "%s system cli-hooks set-tray post_commit_scan_tests scheduler-activity")

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemCliHooksCommandBuilder(), &cobra.Command{
		Use:  "cli-hooks [command] [args]",
		Args: cobra.MinimumNArgs(0),
	})
	// Machine-oriented subcommands run synchronously (no async progress / coordinator heartbeat):
	// git hooks may invoke these on every commit; keep startup minimal.
	cmd.RunE = func(c *cobra.Command, args []string) error {
		initCtx := &pkgctx.CliInitializationContext{
			ProjectRoot: ProjectRootOrResolve(""),
		}
		ctx, err := cli.GetContextFromCommand(c, initCtx)
		if err != nil {
			return err
		}
		if len(args) >= 1 && (args[0] == "is-enabled" || args[0] == "print-tray-entry") {
			return runCliHooks(c, ctx, args)
		}
		return cli.RunWithAsyncProgress(c, args, cli.OperationTypeFromCommand(c), func(cmd *cobra.Command, args []string) error {
			initAsync := &pkgctx.CliInitializationContext{
				ProjectRoot: ProjectRootOrResolve(""),
			}
			cliCtx, err := cli.GetContextFromCommand(cmd, initAsync)
			if err != nil {
				return err
			}
			return runCliHooks(cmd, cliCtx, args)
		})
	}
	helpBuilder.ApplyToCommand(cmd)
	cli.AddCommonFlags(cmd)
	return cmd
}
