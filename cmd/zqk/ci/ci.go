package ci

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/localci"
	"github.com/zqk-os/zqk/pkg/paths"

	"github.com/spf13/cobra"

	testcmd "github.com/zqk-os/zqk/cmd/zqk/test"
	"github.com/zqk-os/zqk/pkg/cliapp"
	bldr "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
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
	return cmd
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

func runLocalCICheckout(cmd *cobra.Command) error {
	studio := cli.ResolveProjectRoot(".")
	if studio == "" {
		return errfmt.Errorf("project root not found")
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

func runTestCasesAfterCheckout(cmd *cobra.Command) error {
	studio := cli.ResolveProjectRoot(".")
	if studio == "" {
		return errfmt.Errorf("project root not found")
	}
	workdir := filepath.Join(studio, paths.ProjectDataDir, "local-ci", "workdir")
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
	studio := cli.ResolveProjectRoot(".")
	if studio == "" {
		return errfmt.Errorf("project root not found")
	}
	base := filepath.Join(studio, paths.ProjectDataDir, "local-ci")
	shaPath := filepath.Join(base, "SOURCE_SHA")
	workdir := filepath.Join(base, "workdir")
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
	studio := cli.ResolveProjectRoot(".")
	if studio == "" {
		return errfmt.Errorf("project root not found")
	}
	if err := localci.Demote(cmd.Context(), localci.Options{
		RepoRoot: studio,
	}); err != nil {
		return errfmt.Errorf("local-ci demote failed: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "local-ci demote: worktrees and pointer files removed")
	return nil
}
