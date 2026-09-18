package system

import (
	"strings"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/when"
	"github.com/spf13/cobra"
)

// NewStatusCmd creates a new status command
func NewStatusCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Show system status and health summary",
		"Show system status and health summary.",
		"",
		"This command displays:",
		"  - Project information",
		"  - Current priority plan",
		"  - System health (from system check)",
		"  - Recent activity",
		"  - Storage backend information",
	).
		AddExample("Basic status", "%s system status").
		AddExample("Verbose status with detailed information", "%s system status --verbose").
		ExcludeCommonFlags()

	statusCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemStatusCommandBuilder(), &cobra.Command{
		Use: "status",
	})
	cli.BindAsyncProgress(statusCmd, func(cmd *cobra.Command, args []string) error {
		verbose, _ := cmd.Flags().GetBool("verbose")
		return runStatus(cmd, verbose)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(statusCmd)

	// Add common flags (format, output, verbose, etc.) for MCP compatibility
	cli.AddCommonFlags(statusCmd)

	return statusCmd
}

func runStatus(cmd *cobra.Command, verbose bool) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return cli.Guard(cmd).Err(errfmt.Errorf("failed to get context")).Return()
	}

	// Get logger for error logging
	profile := profileOrDefault(ctx.Profile, systemProfileHuman)
	logger := logging.GetLoggerFromProfile(profile)

	// Check for CLI reminder FIRST (interrupt notification)
	projectRoot := ProjectRootOrResolve(ctx.ProjectRoot)
	if !when.IsEmpty(projectRoot) {
		checkCLIReminder(cmd, projectRoot)
	}

	// Build status data
	statusData, err := buildStatusData(cmd, ctx, verbose)
	if err != nil {
		logging.Fluent(logger).Error("Failed to build status data", err).Log()
		return cli.Guard(cmd).Err(err).Return()
	}

	// Add storage information for verbose mode
	if verbose {
		storageInfo := buildStorageInfo(cmd, ctx)
		if storageInfo != nil {
			statusData[objects.FieldKeyStorage] = storageInfo
		}
	}

	// Output based on format
	format := cli.GetFormat(cmd)
	switch format {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		if err := cli.FormatOutput(cmd, statusData); err != nil {
			logging.Fluent(logger).Error("status format output", err).Log()
			return cli.Guard(cmd).Err(err).Return()
		}
		return nil
	default:
		return outputStatusTable(cmd, statusData)
	}
}

func extractProjectName(configContent string) string {
	// Simple extraction - look for "name:" line under "project:"
	lines := strings.Split(configContent, "\n")
	inProject := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "project:") {
			inProject = true
			continue
		}
		if inProject {
			if strings.HasPrefix(trimmed, "name:") {
				// Extract value after "name:"
				parts := strings.SplitN(trimmed, ":", 2)
				if len(parts) == 2 {
					name := strings.TrimSpace(parts[1])
					// Remove quotes if present
					name = strings.Trim(name, "\"'")
					return name
				}
			}
			// If we hit another top-level key, we're done with project section
			if trimmed != emptyValue && !strings.HasPrefix(trimmed, " ") && !strings.HasPrefix(trimmed, "\t") && !strings.HasPrefix(trimmed, "#") {
				break
			}
		}
	}

	return ""
}

func outputStatusTable(cmd *cobra.Command, statusData map[string]any) error {
	var buf strings.Builder

	buf.WriteString(formatStatusTableHeader())
	buf.WriteString(formatStatusTableProject(statusData))
	buf.WriteString(formatStatusTableStatus(statusData))
	buf.WriteString(formatStatusTablePlan(statusData))
	buf.WriteString(formatStatusTableCapReview(statusData))
	buf.WriteString(formatStatusTableStorage(statusData))
	buf.WriteString(formatStatusTableActivity(statusData))
	buf.WriteString("\n")

	return cli.WriteOutput(cmd, []byte(buf.String()))
}
