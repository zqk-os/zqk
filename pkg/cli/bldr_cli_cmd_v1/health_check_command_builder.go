package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewHealthCheckCommandBuilder creates a new health_check command
func NewHealthCheckCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("health-check")
	builder.WithShort("Check scheduler daemon health (external health check)")
	help := clipkg.DynamicHelpBuilder("Check scheduler daemon health (external health check)")
	help.WithDescriptionLines("Check if the scheduler daemon is running and healthy.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("This command runs externally (not inside the scheduler daemon) and can be used")
	help.WithDescriptionLines("by monitoring systems, cron jobs, or other external tools to detect if the")
	help.WithDescriptionLines("scheduler daemon is down.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Exit codes:")
	help.WithDescriptionLines("  0 - Scheduler is healthy (running and responsive)")
	help.WithDescriptionLines("  1 - Scheduler is unhealthy (down but should be running, or keep-alive is stale)")
	help.WithDescriptionLines("  2 - Error checking scheduler status")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("This command checks:")
	help.WithDescriptionLines("  - PID file exists and process is running")
	help.WithDescriptionLines("  - Keep-alive file is fresh (updated within last 2 minutes)")
	help.WithDescriptionLines("  - If enabled timer jobs exist, daemon must be running")
	help.AddExample("Check scheduler health", "%s scheduler health-check")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
