package ci

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/spf13/cobra"

	schcmd "github.com/zqk-os/zqk/cmd/zqk/scheduler"
	"github.com/zqk-os/zqk/internal/cli"
	bldr "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewCICmd creates the top-level `ci` command group (Local CI).
// Kernel: PRI-1785699924616992000-8000284f
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
	cmd := bldr.NewCheckoutCommandBuilder()
	cli.BindAsyncProgress(cmd, runCheckout)
	return cmd
}

func newRunCmd() *cobra.Command {
	cmd := bldr.NewRunCommandBuilder()
	cli.BindAsyncProgress(cmd, runCIRun)
	return cmd
}

func newStatusCmd() *cobra.Command {
	// Do not use NewStatusCommandBuilder — that DNA is scheduler status (name collision).
	// TRACK: BLI-1785699946601766000-26fbe6a5 — prefer ci_status_* builder once codegen nests like mcp_svc_*.
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show Local CI SOURCE_SHA and test-bundle health summary",
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
	return runScanTestsAfterCheckout(cmd)
}

func runLocalCICheckout(cmd *cobra.Command) error {
	studio := cli.ResolveProjectRoot(".")
	if studio == "" {
		return errfmt.Errorf("project root not found")
	}
	sha, _ := cmd.Flags().GetString("sha")

	// Checkout SHA worktree gate: skip if already checked out
	base := filepath.Join(studio, ".zqk", "local-ci")
	shaPath := filepath.Join(base, "SOURCE_SHA")
	if raw, err := fileutil.ReadFile(shaPath); err == nil {
		currentSHA := strings.TrimSpace(string(raw))
		if sha != "" {
			c := execwrap.Command("git", "rev-parse", "--verify", sha+"^{commit}")
			c.Dir = studio
			if resolved, err := c.Output(); err == nil {
				if strings.TrimSpace(string(resolved)) == currentSHA {
					fmt.Fprintf(cmd.OutOrStdout(), "local-ci checkout skipped: %s already checked out at %s\n", currentSHA, filepath.Join(base, "workdir"))
					return nil
				}
			}
		}
	}

	script := filepath.Join(studio, "scripts", "local-ci-checkout.sh")
	if _, err := fileutil.Stat(script); err != nil {
		return errfmt.Errorf("local-ci checkout script missing: %s", script)
	}
	allowDirty, _ := cmd.Flags().GetBool("allow-dirty")
	noArchive, _ := cmd.Flags().GetBool("no-archive")

	argv := []string{script}
	if strings.TrimSpace(sha) != "" {
		argv = append(argv, "--sha", sha)
	}
	if allowDirty {
		argv = append(argv, "--allow-dirty")
	}
	if noArchive {
		argv = append(argv, "--no-archive")
	}
	c := execwrap.Command(argv[0], argv[1:]...)
	c.Dir = studio
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	if err := c.Run(); err != nil {
		return errfmt.Errorf("local-ci checkout failed: %w", err)
	}
	return nil
}

func runScanTestsAfterCheckout(cmd *cobra.Command) error {
	studio := cli.ResolveProjectRoot(".")
	if studio == "" {
		return errfmt.Errorf("project root not found")
	}
	workdir := filepath.Join(studio, ".zqk", "local-ci", "workdir")
	if _, err := fileutil.Stat(filepath.Join(workdir, "go.mod")); err != nil {
		return errfmt.Errorf("local-ci workdir missing go.mod at %s (run checkout first)", workdir)
	}
	pkg, _ := cmd.Flags().GetString("package")
	all, _ := cmd.Flags().GetBool("all")
	if strings.TrimSpace(pkg) == "" && !all {
		all = true
	}
	scan := schcmd.NewScanTestsCmd()
	// Inherit parent context for FormatOutput / project root
	scan.SetContext(cmd.Context())
	scan.SetOut(cmd.OutOrStdout())
	scan.SetErr(cmd.ErrOrStderr())
	// Explicit source-root: CI-honest go test cwd (studio keeps SCH-run + health logs).
	var argv []string
	argv = append(argv, "--source-root", workdir)
	if strings.TrimSpace(pkg) != "" {
		argv = append(argv, "--package", pkg)
	}
	if all {
		argv = append(argv, "--all")
	}
	scan.SetArgs(argv)
	return scan.Execute()
}

func runStatus(cmd *cobra.Command, args []string) error {
	studio := cli.ResolveProjectRoot(".")
	if studio == "" {
		return errfmt.Errorf("project root not found")
	}
	base := filepath.Join(studio, ".zqk", "local-ci")
	shaPath := filepath.Join(base, "SOURCE_SHA")
	workdir := filepath.Join(base, "workdir")
	var b strings.Builder
	fmt.Fprintf(&b, "Local CI status (PRI-1785699924616992000-8000284f)\n")
	if raw, err := fileutil.ReadFile(shaPath); err == nil {
		fmt.Fprintf(&b, "  SOURCE_SHA=%s\n", strings.TrimSpace(string(raw)))
	} else {
		fmt.Fprintf(&b, "  SOURCE_SHA=(not set — run: zqk ci checkout)\n")
	}
	if st, err := fileutil.Stat(filepath.Join(workdir, "go.mod")); err == nil && !st.IsDir() {
		fmt.Fprintf(&b, "  workdir=%s\n", workdir)
	} else {
		fmt.Fprintf(&b, "  workdir=(missing)\n")
	}
	if err := cli.WriteOutput(cmd, []byte(b.String())); err != nil {
		return err
	}
	// Best-effort health summary (studio logs)
	health := execwrap.Command(os.Args[0], "scheduler", "test-failures", "health")
	health.Dir = studio
	health.Stdout = cmd.OutOrStdout()
	health.Stderr = cmd.ErrOrStderr()
	_ = health.Run()
	return nil
}

func runDemote(cmd *cobra.Command, args []string) error {
	studio := cli.ResolveProjectRoot(".")
	if studio == "" {
		return errfmt.Errorf("project root not found")
	}
	script := filepath.Join(studio, "scripts", "local-ci-demote.sh")
	if _, err := fileutil.Stat(script); err != nil {
		return errfmt.Errorf("local-ci demote script missing: %s", script)
	}
	c := execwrap.Command(script)
	c.Dir = studio
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	if err := c.Run(); err != nil {
		return errfmt.Errorf("local-ci demote failed: %w", err)
	}
	return nil
}
