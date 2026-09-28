package do

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// NewDoCmd creates the 'zqk do' / 'zqk auto-exec' command.
func NewDoCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewDoCommandBuilder(), &cobra.Command{
		Use:     "do [task_or_bli_id]",
		Aliases: []string{"auto-exec", "auto-do"},
		Short:   "Execute autonomous single-command workflow loop for a backlog item or task",
		Long: `Autonomously discovers, claims, context-prepares, verifies, and latches a backlog item or task in a single invocation.

Examples:
  # Auto-discover the next planned backlog item and execute end-to-end
  zqk do

  # Execute a designated backlog item
  zqk do BLI-ERGONOMICS-AUTO-EXEC-001

  # Dry run without mutating storage
  zqk do --dry-run
`,
		Args: cobra.MaximumNArgs(1),
		RunE: cli.WithProcessor(runDo),
	})
	return cmd
}

func runDo(cmd *cobra.Command, args []string, proc *cli.Processor) error {
	ctx := proc.OperationContext()
	sec := proc.SecurityContext()

	var flags clipkg.FlagBag
	dryRun := flags.Bool(cmd, "dry-run")
	runVerify := flags.Bool(cmd, "verify")
	claimant := flags.String(cmd, "by")
	personaRef := flags.String(cmd, "persona-ref")
	if err := flags.Err(); err != nil {
		return err
	}

	targetID := ""
	if len(args) > 0 {
		targetID = strings.TrimSpace(args[0])
	}
	who := strings.TrimSpace(claimant)
	if who == "" {
		if v := strings.TrimSpace(zqkenv.AgentID().Get()); v != "" {
			who = v
		} else if sec != nil && sec.AccountID != "" {
			who = sec.AccountID
		} else {
			who = "agent:auto-exec"
		}
	}

	pipeline := clipkg.NewAutoExecPipeline(proc.Storage())
	opts := clipkg.AutoExecOptions{
		TargetID:   targetID,
		Claimant:   who,
		PersonaRef: personaRef,
		DryRun:     dryRun,
		RunVerify:  runVerify,
	}

	res, err := pipeline.Execute(ctx, sec, opts)
	if err != nil {
		_ = cli.FormatOutput(cmd, res)
		return err
	}

	return cli.FormatOutput(cmd, res)
}
