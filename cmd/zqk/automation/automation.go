package automation

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/spf13/cobra"
)

const emptyValue = ""

// NewAutomationCmd creates a new automation command group
func NewAutomationCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Automation and integration operations (hooks, CI/CD, and scripts)",
		"Automation and integration operations for use in git hooks, CI/CD pipelines, and scripts.",
		"",
		"This command group provides operations designed for automation contexts:",
		"  Common operations:",
		"    - lint-bypass-audit: Create audit events for lint bypasses",
		"    - docman-sync: Sync documentation registration (discover and register markdown files)",
		"  Specialized operations: (future: ci-validate, hook-setup, etc.)",
	).
		AddExample("Create audit event for lint bypass", "%s automation lint-bypass-audit --git-user \"John Doe\" --git-email \"john@example.com\" --commit-message \"fix: update code\" --files \"pkg/storage/file.go\"").
		AddExample("Sync documentation registration", "%s automation docman-sync").
		AddExample("Sync only if markdown files are staged", "%s automation docman-sync --git-aware")

	automationCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewAutomationCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use: "automation",
	})

	helpBuilder.ApplyToCommand(automationCmd)

	// Common operations
	automationCmd.AddCommand(NewLintBypassAuditCmd())
	automationCmd.AddCommand(NewDocmanSyncCmd())

	// Specialized operations (future)
	// automationCmd.AddCommand(NewCIValidateCmd())
	// automationCmd.AddCommand(NewHookSetupCmd())

	return automationCmd
}
