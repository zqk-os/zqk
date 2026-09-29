package do

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/brand"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/paths"
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

	format := cli.GetFormat(cmd)
	if format == cli.FormatJSON || format == cli.FormatYAML || format == cli.FormatJSONL {
		return cli.FormatOutput(cmd, res)
	}

	renderDoSummary(cmd, res)
	return nil
}

func renderDoSummary(cmd *cobra.Command, res *clipkg.AutoExecResult) {
	var buf strings.Builder
	target := res.TargetBLIID
	if target == "" {
		target = res.TargetTaskID
	}
	buf.WriteString(fmt.Sprintf("🎯 ZQK Autonomous Execution: %s\n", target))
	buf.WriteString("=====================================================\n")
	buf.WriteString(fmt.Sprintf("Status: %s (Claimant: %s)\n\n", res.Status, res.Claimant))

	buf.WriteString("Execution Steps:\n")
	for _, s := range res.Steps {
		icon := "✓"
		if s.StepStatus != "ok" && s.StepStatus != "dry_run" {
			icon = "❌"
		}
		buf.WriteString(fmt.Sprintf("  %s [%-12s] %s\n", icon, s.Step, s.Detail))
	}

	if len(res.VerifiedTestIDs) > 0 {
		buf.WriteString(fmt.Sprintf("\n✓ Verified Tests: %s\n", strings.Join(res.VerifiedTestIDs, ", ")))
	}
	if len(res.LatchedCritIDs) > 0 {
		buf.WriteString(fmt.Sprintf("✓ Latched Criteria: %s\n", strings.Join(res.LatchedCritIDs, ", ")))
	}

	exe := brand.ExecutableName()
	buf.WriteString("\n👉 Next Steps for You & Your AI Agent:\n")
	buf.WriteString("  1. Hand off to your agent (Cursor, Claude, Windsurf, Gemini, Cline, Hermes, etc.):\n")
	buf.WriteString(fmt.Sprintf("     Prompt: \"I've claimed %s. Implement the code and satisfy its criteria.\"\n", target))
	buf.WriteString("  2. Verify implementation & latch criteria:\n")
	buf.WriteString(fmt.Sprintf("     $ %s do %s --verify\n", exe, target))
	buf.WriteString("  3. Discover next tasks:\n")
	buf.WriteString(fmt.Sprintf("     $ %s workflow whats-next\n\n", exe))
	buf.WriteString(fmt.Sprintf("💡 Need to connect your agent? Run '%s' or check ZQK_GETTING_STARTED.md\n", paths.CLIUsage("system", "agent-onboard")))

	fmt.Fprint(cmd.OutOrStdout(), buf.String())
}
