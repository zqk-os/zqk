package system

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/cmd/zqk/ambient"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/agentonboard"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
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
	gitDir := filepath.Join(cli.ResolveProjectRoot("."), ".git")
	if _, err := fileutil.Stat(gitDir); err == nil {
		prePush := filepath.Join(gitDir, "hooks", "pre-push")
		preCommit := filepath.Join(gitDir, "hooks", "pre-commit")
		hasPush := false
		hasCommit := false
		if info, err := fileutil.Stat(prePush); err == nil && info.Mode()&0111 != 0 {
			hasPush = true
		}
		if info, err := fileutil.Stat(preCommit); err == nil && info.Mode()&0111 != 0 {
			hasCommit = true
		}
		if hasCommit && hasPush {
			buf.WriteString("  • Git verification hooks (pre-commit, pre-push): active\n")
		} else {
			if hasCommit {
				buf.WriteString("  • Git pre-commit hook: active\n")
			} else {
				buf.WriteString("  ⚠️  Git pre-commit hook: NOT configured in .git/hooks/pre-commit\n")
			}
			if hasPush {
				buf.WriteString("  • Git pre-push hook: active\n")
			} else {
				buf.WriteString(fmt.Sprintf("  ⚠️  Git pre-push hook: NOT configured in .git/hooks/pre-push (run '%s')\n", paths.CLIInvocation("system sync-git-hooks")))
			}
		}
	} else {
		buf.WriteString("  • Git hooks: repository not detected (.git missing)\n")
	}
	buf.WriteString("  • Ambient event-driven daemon: running\n")

	// 5. Connect Your AI Agent
	cmdName := paths.CLICommandName
	buf.WriteString("\n🔌 Connect Your AI Agent (Seating Guide):\n")
	buf.WriteString("  • Cursor:             Pre-configured! Reads .agents/AGENTS.md and .cursor/mcp.json.\n")
	buf.WriteString(fmt.Sprintf("  • Claude Desktop:     Run '%s mcp install --client claude-desktop' then restart Claude.\n", cmdName))
	buf.WriteString(fmt.Sprintf("  • Claude Code (CLI):  Run 'claude mcp add %s -- %s mcp serve'.\n", cmdName, cmdName))
	buf.WriteString("  • Windsurf (Cascade): Pre-configured! Reads .windsurfrules and mcp_config.json.\n")
	buf.WriteString("  • Cline / Roo Code:   Pre-configured! Reads .clinerules and cline_mcp_settings.json.\n")
	buf.WriteString("  • Gemini/Antigravity: Native workspace integration via .agents/AGENTS.md.\n")
	buf.WriteString(fmt.Sprintf("  • Hermes / OpenClaw:  Direct CLI ('%s workflow whats-next') or TCP ('%s mcp proxy --tcp 127.0.0.1:7777').\n", cmdName, cmdName))

	buf.WriteString("\n⚡ Recommended Agent Tooling:\n")
	buf.WriteString(fmt.Sprintf("  • Fast In-Process Code Search: Run '%s grep <pattern>' (sub-15ms trigram + AST search; token-budgeted JSON via -f json)\n", cmdName))

	buf.WriteString("\n💬 Prompt your AI agent to begin:\n")
	buf.WriteString(fmt.Sprintf("   \"You are paired with the ZQK Knowledge Kernel. Run '%s do' to claim and execute work.\"\n\n", cmdName))
	buf.WriteString(fmt.Sprintf("Or run directly in terminal: %s do\n", cmdName))
	buf.WriteString(fmt.Sprintf("Launch Visual Web Studio:    %s ui -w\n", cmdName))
	buf.WriteString("\nDetailed Guide: docs/onboarding/AI_AGENT_ONBOARDING.md\n")

	_ = cli.WriteOutput(cmd, []byte(buf.String()))
}

