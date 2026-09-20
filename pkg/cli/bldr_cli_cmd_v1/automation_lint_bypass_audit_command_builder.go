package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewAutomationLintBypassAuditCommandBuilder creates a new automation_lint_bypass_audit command
func NewAutomationLintBypassAuditCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for lint-bypass-audit")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
