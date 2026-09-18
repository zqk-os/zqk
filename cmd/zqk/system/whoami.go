package system

import (
	"strings"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/spf13/cobra"
)

// NewWhoamiCmd creates a new whoami command
func NewWhoamiCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Display the current user account information",
		"Display the current user account information.",
		"",
		"This command shows:",
		"  - Account ID",
		"  - Roles",
		"  - Permissions",
		"  - Account details (if available)",
		"",
		"The account is the AuthMiddleware-resolved ACC (credentials / ZQK_API_KEY / session),",
		"not a hardcoded system account. Lane is admin | planner | doer.",
	).
		AddExample("Display current account", "%s system whoami").
		AddExample("Display in JSON format", "%s system whoami --format json").
		AddExample("Display in YAML format", "%s system whoami --format yaml").
		ExcludeCommonFlags()

	whoamiCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemWhoamiCommandBuilder(), &cobra.Command{
		Use: "whoami",
	})
	// Spec short until generate-command-builders refreshes the empty-Use slop builder.
	// TRACK: BLI-1787804771598596000-27599a81
	whoamiCmd.Short = "Show the authenticated account, lane, roles, and permissions"
	cli.BindAsyncProgress(whoamiCmd, func(cmd *cobra.Command, args []string) error {
		return runWhoami(cmd)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(whoamiCmd)

	// Add common flags
	cli.AddCommonFlags(whoamiCmd)

	return whoamiCmd
}

func runWhoami(cmd *cobra.Command) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}

	// Get logger
	profile := profileOrDefault(ctx.Profile, systemProfileHuman)
	logger := logging.GetLoggerFromProfile(profile)

	// Get project root
	projectRoot, err := getProjectRootForWhoami(ctx)
	if err != nil {
		logging.Fluent(logger).Error("Project root validation failed", err).Log()
		return err
	}

	accountInfo := loadWhoamiFromSecurityContext(cmd, projectRoot)
	if accountInfo == nil {
		return errfmt.Errorf("unauthorized: no security context (run from a seated CLI; see POL-AGENT-ACCOUNT-LOGIN-001)")
	}

	// Build output data
	outputData := buildWhoamiOutputData(accountInfo)

	// Output based on format
	format := cli.GetFormat(cmd)
	switch format {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, outputData)
	case cli.FormatTable:
		return outputWhoamiTable(cmd, outputData)
	default:
		return outputWhoamiTable(cmd, outputData)
	}
}

func outputWhoamiTable(cmd *cobra.Command, data map[string]any) error {
	var buf strings.Builder

	buf.WriteString("Current User Account\n")
	buf.WriteString("====================\n\n")

	buf.WriteString(formatWhoamiTableAccount(data))
	buf.WriteString(formatWhoamiTableLane(data))
	buf.WriteString(formatWhoamiTableName(data))
	buf.WriteString(formatWhoamiTableUserInfo(data))
	buf.WriteString(formatWhoamiTablePersona(data))
	buf.WriteString(formatWhoamiTableRoles(data))
	buf.WriteString(formatWhoamiTablePermissions(data))
	buf.WriteString(formatWhoamiTableContext(data))
	buf.WriteString("\n")

	return cli.WriteOutput(cmd, []byte(buf.String()))
}
