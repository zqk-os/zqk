package reports

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/objects"
)

// NewBlockersCmd creates a command to get Dependencies & Blockers
func NewBlockersCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Get Dependencies & Blockers (D&B)",
		"Calculate and display Dependencies & Blockers (D&B), identifying potential",
		"dependencies and blockers that may impact project delivery.",
		"",
		"The D&B analysis is enhanced with Git commit data when available:",
		"  - Code-level dependencies from file co-changes",
		"  - Potential blockers from stale files or irregular patterns",
		"  - Commit relationship analysis",
		"",
		"Dependencies are categorized by:",
		"  - Type: code, work_item, external",
		"  - Severity: low, medium, high, critical",
		"",
		"Blockers are identified from:",
		"  - Risk/blocker objects in the system",
		"  - Code patterns indicating blocked work",
		"  - Commit frequency anomalies",
	).
		AddExample("Get dependencies and blockers", "%s reports blockers").
		AddExample("Get as JSON", "%s reports blockers --format json").
		AddExample("Get with commit data explicitly enabled", "%s reports blockers --include-commit-data").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewReportsBlockersCommandBuilder(), &cobra.Command{
		Use:  "blockers",
		Args: cobra.NoArgs,
		Annotations: map[string]string{
			"mcp.permissions": "read:metrics",
		},
	})

	helpBuilder.ApplyToCommand(cmd)
	cli.BindAsyncProgress(cmd, runBlockers)
	cmd.Flags().Bool("include-commit-data", false, "Explicitly include Git commit data (auto-detected by default)")
	cli.AddCommonFlags(cmd)

	return cmd
}

func runBlockers(cmd *cobra.Command, args []string) error {
	projectMetrics, err := calculateReportMetrics(cmd, "D&B")
	if err != nil {
		return err
	}

	// Output based on format (structured payloads via shared FormatOutput)
	result := map[string]any{
		objects.FieldKeySummary:      projectMetrics.DB.Summary,
		objects.FieldKeyDependencies: projectMetrics.DB.Dependencies,
		objects.FieldKeyBlockers:     projectMetrics.DB.Blockers,
		"action_items":               projectMetrics.DB.ActionItems,
	}
	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, result)
	default:
		return outputBlockersTable(cmd, projectMetrics.DB)
	}
}

func outputBlockersTable(cmd *cobra.Command, db *metrics.DependenciesBlockers) error {
	var buf strings.Builder

	buf.WriteString("\nDependencies & Blockers (D&B)\n")
	buf.WriteString("============================\n\n")

	// Summary
	if db.Summary != emptyValue {
		fmt.Fprintf(&buf, "Summary: %s\n\n", db.Summary)
	}

	// Dependencies
	if len(db.Dependencies) > 0 {
		fmt.Fprintf(&buf, "Dependencies (%d):\n", len(db.Dependencies))
		buf.WriteString("-------------------\n")
		for i, dep := range db.Dependencies {
			fmt.Fprintf(&buf, "%d. [%s] %s - %s\n", i+1, dep.Severity, dep.Type, dep.Description)
		}
		buf.WriteString("\n")
	} else {
		buf.WriteString("Dependencies: None identified\n\n")
	}

	// Blockers
	if len(db.Blockers) > 0 {
		fmt.Fprintf(&buf, "Blockers (%d):\n", len(db.Blockers))
		buf.WriteString("-------------\n")
		for i, blocker := range db.Blockers {
			fmt.Fprintf(&buf, "%d. [%s] %s - %s\n", i+1, blocker.Severity, blocker.Type, blocker.Description)
		}
		buf.WriteString("\n")
	} else {
		buf.WriteString("Blockers: None identified\n\n")
	}

	// Action Items
	if len(db.ActionItems) > 0 {
		fmt.Fprintf(&buf, "Action Items (%d):\n", len(db.ActionItems))
		buf.WriteString("----------------\n")
		for i, action := range db.ActionItems {
			fmt.Fprintf(&buf, "%d. [%s] %s\n", i+1, action.Priority, action.Description)
		}
		buf.WriteString("\n")
	}

	return cli.WriteOutput(cmd, []byte(buf.String()))
}
