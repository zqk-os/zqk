package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewWorkflowNextCommandBuilder creates a new workflow_next command
func NewWorkflowNextCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("next")
	builder.WithShort("Return deterministic next workflow action")
	help := clipkg.DynamicHelpBuilder("Return deterministic next workflow action")
	help.WithDescriptionLines("Return one deterministic next-step recommendation using explicit precedence:")
	help.WithDescriptionLines("  1) critical unacknowledged policy interrupts")
	help.WithDescriptionLines("  2) active convergence sessions")
	help.WithDescriptionLines("  3) current priority-plan backlog routing")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("For the backlog branch, loads the in-progress priority plan and selected backlog item")
	help.WithDescriptionLines("from storage and includes plan title, truncated descriptions, tier, goal_refs, and optional")
	help.WithDescriptionLines("convergence_session_ref / convergence_session_profile in JSON/YAML; table output adds a")
	help.WithDescriptionLines("work-context block and steering tip.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Also reports open_questions_count (question objects with status=open) as a workspace signal.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Session context is keyed by ZQK_SESSION_ID and falls back to .zqk/state/session.")
	help.AddExample("Get next workflow recommendation", "%s workflow next")
	help.AddExample("Get JSON recommendation payload", "%s workflow next --format json")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(0))
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
