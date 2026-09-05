package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewRollbackReconstructCommandBuilder creates a new rollback_reconstruct command
func NewRollbackReconstructCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("reconstruct")
	builder.WithShort("Reconstruct object states at a timestamp (beyond-threshold path)")
	help := clipkg.DynamicHelpBuilder("Reconstruct object states at a timestamp (beyond-threshold path)")
	help.WithDescriptionLines("Recompute the status-relevant graph from scope, then reconstruct each object's state")
	help.WithDescriptionLines("at the given timestamp using the change journal and apply. Use when the rollback")
	help.WithDescriptionLines("point was pruned (beyond retention).")
	help.AddExample("Reconstruct at a timestamp for a priority plan", "%s rollback reconstruct --scope-id priority_plan:PRI-1:complete --timestamp 2026-01-02T12:00:00Z")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("scope-type", "", "lifecycle", "Scope type (e.g. lifecycle)")
	builder.AddStringFlag("scope-id", "", "", "Scope ID (e.g. priority_plan:PRI-1:complete)")
	builder.AddStringFlag("timestamp", "", "", "Target timestamp (RFC3339)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
