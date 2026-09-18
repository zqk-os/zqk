package bldr_cli_cmd_v1

import (
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewTriggerCommandBuilder creates a new trigger command
func NewTriggerCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("trigger")
	builder.WithShort("Manually trigger a scheduler job")
	help := clipkg.DynamicHelpBuilder("Manually trigger a scheduler job")
	help.WithDescriptionLines("Manually trigger execution of a scheduler job by ID.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("This allows you to run a job immediately, regardless of its trigger_type.")
	help.WithDescriptionLines("The job must be enabled and support manual triggering.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Use --pre-commit for pre-commit category jobs (lint, policy, integrity). When set,")
	help.WithDescriptionLines("the job's callback_on_completion runs after the job finishes (writes pre-commit")
	help.WithDescriptionLines("category and runs aggregate) so the hook sees results. Timer runs do not run callbacks.")
	help.AddExample("Trigger a job immediately", "%s scheduler trigger SCH-001")
	help.AddExample("Trigger pre-commit lint job so callback runs and hook sees result", "%s scheduler trigger SCH-pre-commit-lint --pre-commit")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(1))
	builder.AddBoolFlag("pre-commit", "", false, "Run with pre-commit origin so the job's callback_on_completion runs (writes pre-commit category and aggregate)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
