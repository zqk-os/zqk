package ci

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/localci"
	"github.com/zqk-os/zqk/pkg/paths"

	"github.com/spf13/cobra"

	testcmd "github.com/zqk-os/zqk/cmd/zqk/test"
	pkgci "github.com/zqk-os/zqk/pkg/ci"
	bldr "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewCICmd creates the top-level `ci` command group (Local CI).
func NewCICmd() *cobra.Command {
	cmd := bldr.NewCiCommandBuilder()
	cmd.AddCommand(newCheckoutCmd())
	cmd.AddCommand(newRunCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newDemoteCmd())
	cmd.AddCommand(newInitCmd())
	return cmd
}

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize zero-config GitHub Actions CI/CD workflow and Homebrew packaging",
		Long:  "Scaffolds production-grade GitHub Actions audit workflow (.github/workflows/zqk-audit.yml) and Homebrew formula template (Formula/zqk.rb).",
	}
	cmd.Flags().Bool("dry-run", false, "Preview workflow generation without writing files")
	cmd.Flags().Bool("force", false, "Overwrite existing workflow files")
	cmd.Flags().String("workflow-path", "", "Custom relative path for the workflow file")
	cmd.Flags().String("formula-path", "", "Custom relative path for the formula file")
	cli.BindAsyncProgress(cmd, runCIInit)
	return cmd
}

func runCIInit(cmd *cobra.Command, args []string) error {
	studio, err := resolveStudioRoot()
	if err != nil {
		return err
	}
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	force, _ := cmd.Flags().GetBool("force")
	workflowPath, _ := cmd.Flags().GetString("workflow-path")
	formulaPath, _ := cmd.Flags().GetString("formula-path")

	res, err := pkgci.InitCI(cmd.Context(), pkgci.InitOptions{
		RepoRoot:     studio,
		WorkflowPath: workflowPath,
		FormulaPath:  formulaPath,
		DryRun:       dryRun,
		Force:        force,
	})
	if err != nil {
		return errfmt.Errorf("ci init failed: %w", err)
	}

	if res.DryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "ci init (dry-run): would scaffold %s and %s\n", res.WorkflowPath, res.FormulaPath)
		return nil
	}

	for _, f := range res.WrittenFiles {
		fmt.Fprintf(cmd.OutOrStdout(), "ci init: created %s\n", f)
	}
	return nil
}

func newDemoteCmd() *cobra.Command {
	cmd := bldr.NewCiDemoteCommandBuilder()
	cli.BindAsyncProgress(cmd, runDemote)
	return cmd
}

func newCheckoutCmd() *cobra.Command {
	cmd := bldr.NewCiCheckoutCommandBuilder()
	cli.BindAsyncProgress(cmd, runCheckout)
	return cmd
}

func newRunCmd() *cobra.Command {
	cmd := bldr.NewCiRunCommandBuilder()
	cli.BindAsyncProgress(cmd, runCIRun)
	return cmd
}

func newStatusCmd() *cobra.Command {
	// Do not use NewStatusCommandBuilder — that DNA is scheduler status (name collision).
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show Local CI SOURCE_SHA and workdir",
	}
	cli.BindAsyncProgress(cmd, runStatus)
	return cmd
}

func runCheckout(cmd *cobra.Command, args []string) error {
	return runLocalCICheckout(cmd)
}

func runCIRun(cmd *cobra.Command, args []string) error {
	checkoutOnly, _ := cmd.Flags().GetBool("checkout-only")
	if err := runLocalCICheckout(cmd); err != nil {
		return err
	}
	if checkoutOnly {
		return nil
	}
	return runTestCasesAfterCheckout(cmd)
}

func resolveStudioRoot() (string, error) {
	studio := cli.ResolveProjectRoot(".")
	if studio == "" {
		return "", errfmt.Errorf("project root not found")
	}
	return studio, nil
}

func runLocalCICheckout(cmd *cobra.Command) error {
	studio, err := resolveStudioRoot()
	if err != nil {
		return err
	}
	sha, _ := cmd.Flags().GetString("sha")
	allowDirty, _ := cmd.Flags().GetBool("allow-dirty")
	noArchive, _ := cmd.Flags().GetBool("no-archive")

	res, err := localci.Checkout(cmd.Context(), localci.Options{
		RepoRoot:   studio,
		SHA:        sha,
		SHAPinned:  strings.TrimSpace(sha) != "",
		AllowDirty: allowDirty,
		Archive:    !noArchive,
	})
	if err != nil {
		return errfmt.Errorf("local-ci checkout failed: %w", err)
	}
	if res.Skipped {
		fmt.Fprintf(cmd.OutOrStdout(), "local-ci checkout skipped: %s already checked out at %s\n", res.SHA, res.Workdir)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "local-ci checkout: %s checked out at %s\n", res.SHA, res.Workdir)
	}
	return nil
}

func resolveCIWorkdir() (base string, workdir string, err error) {
	studio, err := resolveStudioRoot()
	if err != nil {
		return "", "", err
	}
	base = localci.BaseDir(studio, "")
	workdir = filepath.Join(base, paths.LocalCIWorkdirName)
	return base, workdir, nil
}

func runTestCasesAfterCheckout(cmd *cobra.Command) error {
	_, workdir, err := resolveCIWorkdir()
	if err != nil {
		return err
	}
	if _, err := fileutil.Stat(filepath.Join(workdir, "go.mod")); err != nil {
		return errfmt.Errorf("local-ci workdir missing go.mod at %s (run checkout first)", workdir)
	}
	run := testcmd.NewRunCmd()
	run.SetContext(cmd.Context())
	run.SetOut(cmd.OutOrStdout())
	run.SetErr(cmd.ErrOrStderr())
	run.SetArgs([]string{"--all"})
	return run.Execute()
}

func runStatus(cmd *cobra.Command, args []string) error {
	base, workdir, err := resolveCIWorkdir()
	if err != nil {
		return err
	}
	shaPath := filepath.Join(base, paths.LocalCISourceSHAFile)
	var b strings.Builder
	fmt.Fprintf(&b, "Local CI status\n")
	if raw, err := fileutil.ReadFile(shaPath); err == nil {
		fmt.Fprintf(&b, "  SOURCE_SHA=%s\n", strings.TrimSpace(string(raw)))
	} else {
		fmt.Fprintf(&b, "%s", paths.RewriteCanonicalCLIInvocations("  SOURCE_SHA=(not set — run: zqk ci checkout)\n"))
	}
	if st, err := fileutil.Stat(filepath.Join(workdir, "go.mod")); err == nil && !st.IsDir() {
		fmt.Fprintf(&b, "  workdir=%s\n", workdir)
	} else {
		fmt.Fprintf(&b, "  workdir=(missing)\n")
	}
	return cli.WriteOutput(cmd, []byte(b.String()))
}

func runDemote(cmd *cobra.Command, args []string) error {
	studio, err := resolveStudioRoot()
	if err != nil {
		return err
	}
	if err := localci.Demote(cmd.Context(), localci.Options{
		RepoRoot: studio,
	}); err != nil {
		return errfmt.Errorf("local-ci demote failed: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "local-ci demote: worktrees and pointer files removed")
	return nil
}
