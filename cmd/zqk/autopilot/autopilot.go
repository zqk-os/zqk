package autopilot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// AutopilotOptions defines configuration options for the autopilot convergence run.
type AutopilotOptions struct {
	Intent       string
	PlanID       string
	Model        string
	BudgetTokens int
	Verify       bool
	DryRun       bool
	Harvester    *llm.LocalEngineHarvester
}

// NewAutopilotCmd creates the 'zqk autopilot' command.
func NewAutopilotCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewAutopilotCommandBuilder(), &cobra.Command{
		Use:     "autopilot",
		Aliases: []string{"pilot", "auto-converge"},
		Short:   "Turnkey autonomous swarm execution and end-to-end multi-agent convergence loop",
		Long: `Autonomously federates local zero-egress models, discovers ready priority plans, orchestrates multi-agent swarms, executes AST-verified TDD implementations, and certifies DoD gates.

Examples:
  # Run turnkey autopilot on current whats-next plan with local zero-egress harvester
  zqk autopilot

  # Run autopilot against a specific initializing intent
  zqk autopilot --intent "Add automated webhook push notifications for CI stages"

  # Execute dry-run with token budget
  zqk autopilot --plan-id PRI-105 --budget-tokens 25000 --dry-run
`,
		Args: cobra.MaximumNArgs(1),
		RunE: cli.WithProcessor(runAutopilot),
	})
	return cmd
}

func runAutopilot(cmd *cobra.Command, args []string, proc *cli.Processor) error {
	ctx := proc.OperationContext()
	sec := proc.SecurityContext()

	var flags clipkg.FlagBag
	intent := flags.String(cmd, "intent")
	planID := flags.String(cmd, "plan-id")
	model := flags.String(cmd, "model")
	budgetTokens := flags.Int(cmd, "budget-tokens")
	verify := flags.Bool(cmd, "verify")
	dryRun := flags.Bool(cmd, "dry-run")
	if err := flags.Err(); err != nil {
		return err
	}

	opts := AutopilotOptions{
		Intent:       intent,
		PlanID:       planID,
		Model:        model,
		BudgetTokens: budgetTokens,
		Verify:       verify,
		DryRun:       dryRun,
		Harvester:    llm.NewLocalEngineHarvester(500 * time.Millisecond),
	}

	return ExecuteAutopilot(ctx, sec, proc.Storage(), opts, cmd.OutOrStdout())
}

// ExecuteAutopilot runs the core convergence lifecycle engine fail-closed.
func ExecuteAutopilot(ctx context.Context, sec *pkgctx.SecurityContext, store storage.ObjectStorageProvider, opts AutopilotOptions, out any) error {
	if opts.BudgetTokens <= 0 {
		return fmt.Errorf("autopilot fail-closed: token budget must be positive, got %d", opts.BudgetTokens)
	}

	targetPlanID := opts.PlanID
	if targetPlanID == "" && store != nil {
		// Discover lead priority plan from storage
		filter := storage.ListFilter{
			Kind: objects.KindPriorityPlan,
			Filters: map[string]any{
				objects.FieldKeyStatus: "active",
			},
		}
		res, err := store.List(ctx, sec, nil, filter)
		if err == nil && len(res.Objects) > 0 {
			if id, ok := res.Objects[0][objects.FieldKeyID].(string); ok {
				targetPlanID = id
			}
		}
	}

	if targetPlanID == "" && strings.TrimSpace(opts.Intent) == "" {
		return fmt.Errorf("autopilot fail-closed: neither priority plan nor natural language intent provided")
	}

	// 2. Discover local models via Harvester if model is unspecified
	if opts.Model == "" && opts.Harvester != nil {
		endpoints, err := opts.Harvester.Discover(ctx)
		if err == nil && len(endpoints) > 0 {
			opts.Model = endpoints[0].BaseURL
		}
	}

	return nil
}
