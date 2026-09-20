// Package tray implements the zqk tray command group — named shortcuts over zqk argv (veneer layer).
package tray

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	traypkg "github.com/zqk-os/zqk/pkg/tray"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation/qa"
	"github.com/zqk-os/zqk/pkg/zqkenv"
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
	cmd.AddCommand(newSignCmd())

	return cmd
}

func resolveRoot(cmd *cobra.Command) (string, error) {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return "", errfmt.Errorf("failed to get context")
	}
	projectRoot := ctx.ProjectRoot
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
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
		Use:   "run <name> [args...]",
		Short: "Run the zqk binary with the tray entry argv and optional additional arguments",
		Args:  cobra.MinimumNArgs(1),
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

	fullArgv := append(append([]string{}, e.Argv...), args[1:]...)

	if !e.IsDefault {
		isPriv, token := traypkg.IsPrivilegedArgv(fullArgv)
		if isPriv {
			if e.Signature == "" {
				return errfmt.Errorf("access denied: tray entry %q contains restricted flag/command (%s) and is not cryptographically signed. Use 'zqk tray sign %s' to authorize, or execute directly.", e.Name, token, e.Name)
			}
			keyPath := filepath.Join(projectRoot, paths.ProjectDataDir, "keystore", "auditor.priv")
			signer, err := qa.NewAuditorSigner(keyPath)
			if err != nil {
				return errfmt.Errorf("access denied: tray entry %q requires signature verification, but failed to load auditor key: %w", e.Name, err)
			}
			if err := traypkg.VerifyEntry(e, signer.PublicKey()); err != nil {
				return errfmt.Errorf("access denied: tray entry %q signature verification failed: %w", e.Name, err)
			}
		}
	}

	if dryRun {
		logging.Fluent(logger).Info("tray dry-run").
			String("entry", e.Name).
			String("binary", bin).
			String("argv", strings.Join(fullArgv, " ")).
			Log()
		return cli.FormatOutput(cmd, map[string]any{
			"dry_run":      true,
			"entry":        e.Name,
			"binary":       bin,
			"argv":         fullArgv,
			"command_line": traypkg.FormatExplainLine(filepath.Base(bin), fullArgv),
			"project_root": projectRoot,
		})
	}

	c := execwrap.CommandContext(runCtx, bin, fullArgv...) //nolint:gosec // argv from tray manifest (schema-validated); same trust as invoking zqk subcommands manually.
	zqkenv.WireExecForIsolatedProject(c, projectRoot)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	env := os.Environ()
	if len(c.Env) > 0 {
		env = c.Env
	}
	c.Env = append(env, zqkenv.ExecSource().Name()+"=tray")
	if err := c.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return errfmt.Errorf("tray run: subprocess exited with code %d", exitErr.ExitCode())
		}
		return errfmt.Newf("tray run").Wrap(err)
	}
	return nil
}

func newSignCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewTraySignCommandBuilder()
	cmd.RunE = runSign
	return cmd
}

func runSign(cmd *cobra.Command, args []string) error {
	projectRoot, err := resolveRoot(cmd)
	if err != nil {
		return err
	}
	userPath := datacell.TrayYAMLPath(projectRoot)
	data, err := fileutil.ReadFile(userPath)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return errfmt.Errorf("user tray manifest %s does not exist", userPath)
		}
		return errfmt.Errorf("read %s: %w", userPath, err)
	}

	var userCfg traypkg.Config
	if err := yaml.Unmarshal(data, &userCfg); err != nil {
		return errfmt.Newf("parse %s", userPath).Wrap(err)
	}

	name := strings.TrimSpace(args[0])
	var targetEntry *traypkg.Entry
	for i := range userCfg.Entries {
		if userCfg.Entries[i].Name == name {
			targetEntry = &userCfg.Entries[i]
			break
		}
	}
	if targetEntry == nil {
		return errfmt.Errorf("tray entry %q not found in %s", name, userPath)
	}

	keyPath, _ := cmd.Flags().GetString("key-path")
	if keyPath == "" {
		keyPath = filepath.Join(projectRoot, paths.ProjectDataDir, "keystore", "auditor.priv")
	} else if !filepath.IsAbs(keyPath) {
		keyPath = filepath.Join(projectRoot, keyPath)
	}

	signer, err := qa.NewAuditorSigner(keyPath)
	if err != nil {
		return errfmt.Errorf("load auditor signer: %w", err)
	}

	accountID := qa.AuditorAccountID
	if secCtx := pkgctx.GetSecurityContext(cmd.Context()); secCtx != nil && secCtx.AccountID != "" {
		accountID = secCtx.AccountID
	}

	if err := traypkg.SignEntry(targetEntry, signer.PrivateKey(), accountID); err != nil {
		return errfmt.Errorf("sign tray entry %q: %w", name, err)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&userCfg); err != nil {
		return errfmt.Errorf("encode %s: %w", userPath, err)
	}
	if err := fileutil.WriteFile(userPath, buf.Bytes(), paths.FilePerm644); err != nil {
		return errfmt.Errorf("write %s: %w", userPath, err)
	}

	return cli.FormatOutput(cmd, map[string]any{
		"status":    "signed",
		"name":      targetEntry.Name,
		"signed_by": targetEntry.SignedBy,
		"signature": targetEntry.Signature,
		"key_path":  keyPath,
	})
}
