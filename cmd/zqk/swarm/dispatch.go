package swarm

import (
	"fmt"

	"github.com/spf13/cobra"

	pkgci "github.com/zqk-os/zqk/pkg/ci"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewDispatchCmd creates the one-click autonomous worker swarm dispatch command.
func NewDispatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dispatch",
		Short: "One-click autonomous worker swarm dispatch with token budgeting and AST verification",
		Long:  "Dispatches an autonomous swarm execution runner with bounded token budgeting, concurrency limits, and live progress reporting.",
		RunE:  runDispatch,
	}

	cmd.Flags().Int("token-budget", pkgci.DefaultTokenBudget, "Total token budget allocated across workers")
	cmd.Flags().Int("max-parallel", pkgci.DefaultMaxParallel, "Maximum concurrent worker subagents")
	cmd.Flags().String("entrypoint", "auto-pilot", "Entrypoint instruction or execution phase")
	cmd.Flags().Bool("dry-run", false, "Validate swarm configuration and token allocations without dispatching")

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
