// Package tray implements the zqk tray command group — named shortcuts over zqk argv (veneer layer).
package tray

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/execwrap"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	traypkg "github.com/lanceman/zqk/pkg/tray"
	"github.com/spf13/cobra"
)

const emptyValue = ""

// NewTrayCmd returns the tray command group (list, show, explain, run).
func NewTrayCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Tray — configurable named shortcuts to zqk subcommands",
		"Loads embedded defaults plus optional .zqk/tray.yaml. Each entry is a name, description, and argv slice.",
		"",
		"Use list/show to discover entries; explain prints the resolved command line; run executes the same binary with those args.",
	).
		AddExample("List entries", "%s tray list").
		AddExample("Show one entry", "%s tray show scheduler-status").
		AddExample("Explain resolved argv", "%s tray explain scheduler-status").
		AddExample("Run (dry run)", "%s tray run scheduler-status --dry-run")

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewTrayCommandBuilder(), &cobra.Command{
		Use: "tray",
	})
	helpBuilder.ApplyToCommand(cmd)

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newShowCmd())
	cmd.AddCommand(newExplainCmd())
	cmd.AddCommand(newRunCmd())

	return cmd
}

func resolveRoot(cmd *cobra.Command) (string, error) {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return "", errfmt.Errorf("failed to get context")
	}
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue {
		projectRoot = ctx.ProjectRoot
	}
	if projectRoot == emptyValue {
		return "", errfmt.Errorf("project root not found (run from a zqk project or set project root)")
	}
	return projectRoot, nil
}

func newListCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewTrayListCommandBuilder(), &cobra.Command{
		Use:   "list",
		Short: "List tray entry names and descriptions",
		RunE:  runList,
	})
	cli.AddCommonFlags(cmd)
	return cmd
}

func runList(cmd *cobra.Command, _ []string) error {
	projectRoot, err := resolveRoot(cmd)
	if err != nil {
		return err
	}
	entries, err := traypkg.Load(projectRoot)
	if err != nil {
		return err
	}
	rows := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, map[string]any{
			objects.FieldKeyName:        e.Name,
			objects.FieldKeyDescription: e.Description,
		})
	}
	return cli.FormatOutput(cmd, map[string]any{
		objects.FieldKeyEntries: rows,
		"meta": map[string]any{
			"project_root": projectRoot,
			"count":        len(rows),
		},
	})
}

func newShowCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewTrayShowCommandBuilder(), &cobra.Command{
		Use:   "show <name>",
		Short: "Show one tray entry (name, description, argv)",
		Args:  cobra.ExactArgs(1),
		RunE:  runShow,
	})
	cli.AddCommonFlags(cmd)
	return cmd
}

func runShow(cmd *cobra.Command, args []string) error {
	projectRoot, err := resolveRoot(cmd)
	if err != nil {
		return err
	}
	entries, err := traypkg.Load(projectRoot)
	if err != nil {
		return err
	}
	e := traypkg.Find(entries, args[0])
	if e == nil {
		return errfmt.Errorf("unknown tray entry %q (see: zqk tray list)", args[0])
	}
	return cli.FormatOutput(cmd, map[string]any{
		objects.FieldKeyName:        e.Name,
		objects.FieldKeyDescription: e.Description,
		"argv":                      e.Argv,
		"project_root":              projectRoot,
	})
}

func newExplainCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewTrayExplainCommandBuilder(), &cobra.Command{
		Use:   "explain <name>",
		Short: "Print the shell-style command line for a tray entry",
		Args:  cobra.ExactArgs(1),
		RunE:  runExplain,
	})
	cli.AddCommonFlags(cmd)
	return cmd
}

func runExplain(cmd *cobra.Command, args []string) error {
	projectRoot, err := resolveRoot(cmd)
	if err != nil {
		return err
	}
	entries, err := traypkg.Load(projectRoot)
	if err != nil {
		return err
	}
	e := traypkg.Find(entries, args[0])
	if e == nil {
		return errfmt.Errorf("unknown tray entry %q (see: zqk tray list)", args[0])
	}
	bin := filepath.Base(os.Args[0])
	line := traypkg.FormatExplainLine(bin, e.Argv)
	return cli.FormatOutput(cmd, map[string]any{
		objects.FieldKeyName:        e.Name,
		objects.FieldKeyDescription: e.Description,
		"argv":                      e.Argv,
		"command_line":              line,
		"binary":                    bin,
	})
}

func newRunCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewTrayRunCommandBuilder(), &cobra.Command{
		Use:   "run <name>",
		Short: "Run the zqk binary with the tray entry argv",
		Args:  cobra.ExactArgs(1),
		RunE:  runRun,
	})
	cmd.Flags().Bool(cli.FlagDryRun, false, "Print argv and exit without executing")
	cli.AddCommonFlags(cmd)
	return cmd
}

func runRun(cmd *cobra.Command, args []string) error {
	projectRoot, err := resolveRoot(cmd)
	if err != nil {
		return err
	}
	entries, err := traypkg.Load(projectRoot)
	if err != nil {
		return err
	}
	e := traypkg.Find(entries, args[0])
	if e == nil {
		return errfmt.Errorf("unknown tray entry %q (see: zqk tray list)", args[0])
	}
	dryRun, _ := cmd.Flags().GetBool(cli.FlagDryRun)
	bin := os.Args[0]
	runCtx := cli.CommandContextOr(cmd, context.Background()) // Background: request-or-shutdown derived
	ictx := cli.GetContext(cmd)
	profile := ""
	if ictx != nil {
		profile = ictx.Profile
	}
	logger := logging.GetLoggerFromProfile(profile)

	if dryRun {
		logging.Fluent(logger).Info("tray dry-run").
			String("entry", e.Name).
			String("binary", bin).
			String("argv", strings.Join(e.Argv, " ")).
			Log()
		return cli.FormatOutput(cmd, map[string]any{
			"dry_run":      true,
			"entry":        e.Name,
			"binary":       bin,
			"argv":         e.Argv,
			"command_line": traypkg.FormatExplainLine(filepath.Base(bin), e.Argv),
			"project_root": projectRoot,
		})
	}

	c := execwrap.CommandContext(runCtx, bin, e.Argv...) //nolint:gosec // argv from tray manifest (schema-validated); same trust as invoking zqk subcommands manually.
	zqkenv.WireExecForIsolatedProject(c, projectRoot)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	c.Env = os.Environ()
	if err := c.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return errfmt.Errorf("tray run: subprocess exited with code %d", exitErr.ExitCode())
		}
		return errfmt.Newf("tray run").Wrap(err)
	}
	return nil
}
