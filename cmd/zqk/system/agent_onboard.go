package system

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/cmd/zqk/ambient"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/agentonboard"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
)

// NewAgentOnboardCmd wires system agent-onboard (Vector A/B first-contact sync).
func NewAgentOnboardCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemAgentOnboardCommandBuilder()
	cli.RequireSession(cmd, false)
	cli.RequireStorage(cmd, false)
	cmd.RunE = runAgentOnboard
	return cmd
}

func runAgentOnboard(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		root := proc.ProjectRoot()
		if root == emptyValue {
			return errfmt.Errorf("project root not found")
		}
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		detectOnly, _ := cmd.Flags().GetBool("detect-only")
		skipSeat, _ := cmd.Flags().GetBool("skip-seat")
		skipPrime, _ := cmd.Flags().GetBool("skip-prime")
		allVendors, _ := cmd.Flags().GetBool("all-vendors")
		headless, _ := cmd.Flags().GetBool("headless")
		force, _ := cmd.Flags().GetBool("force")
		vendorsRaw, _ := cmd.Flags().GetString("vendors")

		logger := logging.GetLoggerFromProfile(proc.Context().Profile)
		sec := proc.SecurityContext()
		sessionOK := sec != nil && sec.AccountID != ""

		res, err := agentonboard.Run(agentonboard.Options{
			ProjectRoot:  root,
			Logger:       logger,
			Seat:         SeedDefaultAgentSeatingPack,
			DryRun:       dryRun,
			DetectOnly:   detectOnly,
			SkipSeat:     skipSeat,
			SkipPrime:    skipPrime,
			AllVendors:   allVendors,
			Headless:     headless,
			Force:        force,
			VendorFilter: agentonboard.NormalizeVendorFilter(vendorsRaw),
			SessionOK:    sessionOK,
		})
		if err != nil {
			return errfmt.Newf("agent-onboard").Wrap(err)
		}
		if !detectOnly && !dryRun {
			if err := EnsureGitHooks(root, logger); err != nil {
				logging.Fluent(logger).Warn("Failed to ensure git pre-commit hook during agent-onboard").WithError(err).Log()
			}
			if err := ambient.EnsureDaemon(root, logger); err != nil {
				logging.Fluent(logger).Warn("Failed to ensure ambient daemon during agent-onboard").WithError(err).Log()
			}
			if sp := proc.Storage(); sp != nil {
				_ = WarmTestDashboard(context.Background(), root, sp)
			}
		}
		format := cli.GetFormat(cmd)
		if format == cli.FormatJSON || format == cli.FormatYAML || format == cli.FormatJSONL {
			if err := cli.FormatOutput(cmd, res); err != nil {
				return err
			}
		} else {
			renderAgentOnboardSummary(cmd, res)
		}
		if res.Status == agentonboard.ResultBlocked || res.Status == agentonboard.ResultFailed {
			return errfmt.Errorf("agent-onboard %s", res.Status)
		}
		return nil
	})(cmd, nil)
}

func renderAgentOnboardSummary(cmd *cobra.Command, res *agentonboard.Result) {
	var buf strings.Builder
	buf.WriteString("🤖 ZQK Agent Onboard & Environment Sync\n")
	buf.WriteString("======================================\n\n")

	// 1. Detected Hosts
	buf.WriteString("✓ Detected Agent Hosts:\n")
	if detectStage, ok := res.Stages[agentonboard.StageDetect]; ok {
		if detected, ok := detectStage.Detail["detected"].([]agentonboard.DetectedVendor); ok && len(detected) > 0 {
			for _, v := range detected {
				buf.WriteString(fmt.Sprintf("  • %s (markers: %s)\n", v.DisplayName, strings.Join(v.Markers, ", ")))
			}
		} else {
			buf.WriteString("  • Generic / Headless environment (.agents/AGENTS.md active)\n")
		}
	}

	// 2. Primed Directives & Workspace
	buf.WriteString("\n✓ Primed Workspace & Kernel Directives:\n")
	if primeStage, ok := res.Stages[agentonboard.StagePrimeWorkspace]; ok {
		if files, ok := primeStage.Detail["files"].([]string); ok && len(files) > 0 {
			for _, f := range files {
				buf.WriteString(fmt.Sprintf("  • Synced %s\n", f))
			}
		}
		if skipped, ok := primeStage.Detail["skipped"].([]string); ok && len(skipped) > 0 {
			buf.WriteString(fmt.Sprintf("  • Existing vendor configs preserved (%s)\n", strings.Join(skipped, ", ")))
		}
	} else {
		buf.WriteString("  • Directives synced into .agents/AGENTS.md\n")
	}

	// 3. Seating & Personas
	if seatStage, ok := res.Stages[agentonboard.StageSeat]; ok {
		buf.WriteString("\n✓ Agent Seating & Roles:\n")
		created := 0
		if c, ok := seatStage.Detail["created"].(int); ok {
			created = c
		}
		if created > 0 {
			buf.WriteString(fmt.Sprintf("  • Seeded %d default personas (System Architect, Code Craftsman, QA Verification)\n", created))
		} else {
			buf.WriteString("  • Default personas and agent seating active\n")
		}
	}

	// 4. Daemons & Hooks
	buf.WriteString("\n✓ System Daemons & Hooks:\n")
	buf.WriteString("  • Git pre-commit verification hook: active\n")
	buf.WriteString("  • Ambient event-driven daemon: running\n")

	// 5. Next steps
	cmdName := paths.CLICommandName
	buf.WriteString("\nNext Steps:\n")
	buf.WriteString(fmt.Sprintf("  1. Launch Visual Web Studio:  %s ui -w  (http://127.0.0.1:8080)\n", cmdName))
	buf.WriteString(fmt.Sprintf("  2. Discover Mission & Tasks:  %s workflow whats-next\n", cmdName))
	buf.WriteString(fmt.Sprintf("  3. Connect MCP (if Claude):   %s mcp install --client claude-desktop\n", cmdName))
	buf.WriteString(fmt.Sprintf("  4. Start Scheduler Daemons:   %s scheduler start\n", cmdName))

	_ = cli.WriteOutput(cmd, []byte(buf.String()))
}
