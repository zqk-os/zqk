package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewWorkflowWhatsNextCommandBuilder creates a new workflow_whats_next command
func NewWorkflowWhatsNextCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("whats-next")
	builder.WithShort("Composite PRI + backlog + CVS + compressed convergence measure")
	help := clipkg.DynamicHelpBuilder("Composite PRI + backlog + CVS + compressed convergence measure")
	help.WithDescriptionLines("Single JSON snapshot for agent session start: current priority plan (when discoverable),")
	help.WithDescriptionLines("backlog counts for that plan, active/paused convergence sessions (summary), and an optional")
	help.WithDescriptionLines("compressed block from the same pipeline as `zqk scheduler convergence measure`.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Prefer this over ad-hoc shell wrappers: one binary, same working directory semantics.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Default priority plan when unset: CLI alpha readiness plan id (still loaded when paused).")
	help.WithDescriptionLines("Default measure session: data-cell CVS if present among active/paused rows, else the first row.")
	help.AddExample("Full composite JSON (default plan + measure)", "%s workflow whats-next --format json")
	help.AddExample("Objects only (no health.jsonl / measure)", "%s workflow whats-next --format json --skip-measure")
	help.AddExample("Explicit plan and CVS for measure", "%s workflow whats-next --format json --priority-plan PLAN-EXAMPLE --session-id CONV-EXAMPLE")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(0))
	builder.AddStringFlag("priority-plan", "", "", "Priority plan id (PLAN-*). When empty, resolves default alpha plan then in_progress / paused fallbacks.")
	builder.AddStringFlag("session-id", "", "", "Convergence session id (CONV-*) for measure compression. When empty, uses data-cell heuristic or first active/paused session.")
	builder.AddBoolFlag("skip-measure", "", false, "Omit scheduler convergence measure (objects plane only; faster).")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
