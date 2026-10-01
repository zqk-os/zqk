package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSubmitCommandBuilder creates a new submit command
func NewSubmitCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("submit")
	builder.WithShort("Submit a command as a background job to the scheduler")
	help := clipkg.DynamicHelpBuilder("Submit a command as a background job to the scheduler")
	help.WithDescriptionLines("Submit a command to run as a background job in the scheduler.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("This creates a one-time scheduler job that executes immediately. The job runs")
	help.WithDescriptionLines("in the background and you can check its status using 'scheduler activity'")
	help.WithDescriptionLines("or 'scheduler history'.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("The command is executed using run_wrapper, which provides:")
	help.WithDescriptionLines("  - Progress tracking (stdout/stderr capture)")
	help.WithDescriptionLines("  - Timeout management")
	help.WithDescriptionLines("  - Retry logic")
	help.WithDescriptionLines("  - Execution metrics")
	help.AddExample("Submit a simple command", "%s scheduler submit \"echo hello world\"")
	help.AddExample("Submit with timeout and working directory", "%s scheduler submit \"make build\" --max-runtime 600 --workdir /path/to/project")
	help.AddExample("Submit with environment variables", "%s scheduler submit \"npm test\" --env \"NODE_ENV=test\" --env \"CI=true\"")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.MinimumNArgs(1))
	builder.AddIntFlag("max-runtime", "", 3600, "Maximum runtime in seconds (0 = no timeout, default: 3600)")
	builder.AddStringFlag("workdir", "", "", "Working directory for command execution")
	builder.AddStringArrayFlag("env", "", "Environment variables (format: KEY=VALUE, can be specified multiple times)")
	builder.AddIntFlag("retry", "", 0, "Number of retry attempts on failure (default: 0)")
	builder.AddIntFlag("retry-delay", "", 5, "Delay between retries in seconds (default: 5)")
	builder.AddStringFlag("title", "", "", "Job title (default: auto-generated from command)")
	builder.AddStringFlag("callback-completion", "", "", "Command to run on job completion (receives JSON payload via stdin)")
	builder.AddStringFlag("callback-failure", "", "", "Command to run on job failure (receives JSON payload via stdin)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
