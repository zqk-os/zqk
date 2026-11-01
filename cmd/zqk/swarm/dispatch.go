package swarm

import (
	"fmt"

	"github.com/spf13/cobra"

	pkgci "github.com/zqk-os/zqk/pkg/ci"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewDispatchCmd creates the one-click autonomous worker swarm dispatch command.
func NewDispatchCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSwarmDispatchCommandBuilder()
	cmd.RunE = runDispatch
	return cmd
}

func runDispatch(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	tokenBudget, _ := cmd.Flags().GetInt("token-budget")
	maxParallel, _ := cmd.Flags().GetInt("max-parallel")
	entrypoint, _ := cmd.Flags().GetString("entrypoint")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	studioRoot := paths.ResolveProjectRoot(".")
	if studioRoot == "" {
		return errfmt.Errorf("swarm dispatch failed: project root not found")
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "🚀 [SWARM DISPATCH] Initializing autonomous swarm runner...\n")
	fmt.Fprintf(out, "   Token Budget: %d | Max Parallel: %d | Entrypoint: %s\n", tokenBudget, maxParallel, entrypoint)

	res, err := pkgci.DispatchSwarmRunner(ctx, pkgci.SwarmRunnerOptions{
		RepoRoot:    studioRoot,
		TokenBudget: tokenBudget,
		MaxParallel: maxParallel,
		Entrypoint:  entrypoint,
		DryRun:      dryRun,
	})
	if err != nil {
		return errfmt.Errorf("swarm dispatch failed: %w", err)
	}

	if res.DryRun {
		fmt.Fprintf(out, "✓ [SWARM DISPATCH] Dry-run validated successfully (Allocated: %d tokens/worker, AST: %v)\n",
			res.AllocatedBudgetPerWorker, res.ASTVerified)
		return nil
	}

	fmt.Fprintf(out, "✓ [SWARM DISPATCH] Swarm runner active. Evidence: %s\n", res.ExecutionEvidence)
	return nil
}
