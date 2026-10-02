package system

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewAuditReportCmd creates a command to generate audit reports from metrics
func NewAuditReportCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Generate audit report from command metrics",
		"Generate comprehensive audit reports analyzing command execution metrics.",
		"",
		"This command analyzes command metrics to identify:",
		"  - Commands with high failure rates",
		"  - Frequent timeouts",
		"  - Slow commands",
		"  - Improvement opportunities",
		"  - Churn indicators (user confusion patterns)",
		"",
		"The report helps identify areas for improvement and clarifications to reduce churn.",
	).
		AddExample("Generate audit report", "%s system audit-report").
		AddExample("Save report to file", "%s system audit-report --output report.md").
		AddExample("Generate report with specific focus", "%s system audit-report --focus failures").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemAuditReportCommandBuilder(), &cobra.Command{
		Use:  "audit-report",
		Args: cobra.NoArgs,
		RunE: runAuditReport,
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().String("focus", "all", "Focus on a report section: failures, timeouts, slow, churn, or all")

	cli.AddCommonFlags(cmd)
	return cmd
}

func runAuditReport(cmd *cobra.Command, args []string) error {
	projectRoot := ProjectRootOrResolve("")
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	store, err := openCommandMetricsStore(projectRoot)
	if err != nil {
		return err
	}

	// Create analyzer
	analyzer := clipkg.NewMetricsAnalyzer(store)

	// Perform analysis
	analysis, err := analyzer.Analyze()
	if err != nil {
		return errfmt.Newf("failed to analyze metrics").Wrap(err)
	}

	// Honor the --focus flag: it is accepted by the CLI contract, so the
	// report must actually be filtered by it.
	focus, err := cmd.Flags().GetString("focus")
	if err != nil {
		return errfmt.Newf("failed to read focus flag").Wrap(err)
	}
	analysis = ApplyFocusFilter(analysis, focus)

	// Get format flag
	format := cli.GetFormat(cmd)
	formatStr := string(format)

	// Generate report based on format
	var output []byte
	switch formatStr {
	case "json":
		jsonData, jsonErr := analyzer.GenerateReportJSON(analysis)
		if jsonErr != nil {
			return errfmt.Newf("failed to generate JSON report").Wrap(jsonErr)
		}
		output = jsonData
	case "yaml":
		yamlData, yamlErr := analyzer.GenerateReportYAML(analysis)
		if yamlErr != nil {
			return errfmt.Newf("failed to generate YAML report").Wrap(yamlErr)
		}
		output = yamlData
	case "table":
		// For table format, use markdown report (which is table-friendly)
		report := analyzer.GenerateReport(analysis)
		output = []byte(report)
	default:
		// Default to table (markdown report format)
		report := analyzer.GenerateReport(analysis)
		output = []byte(report)
	}

	// Output (use format flag from common flags, or default to stdout)
	outputPath := cli.GetOutputPath(cmd)
	if outputPath != emptyValue {
		// Ensure directory exists
		dir := filepath.Dir(outputPath)
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			return errfmt.Newf("failed to create output directory").Wrap(err)
		}

		if err := fileutil.WriteFile(outputPath, output, paths.FilePerm644); err != nil { //nolint:gosec // Report files - 0644 keeps reports readable by other users on the same host
			return errfmt.Newf("failed to write report").Wrap(err)
		}

		// Only print message for non-JSON formats (JSON is typically piped/redirected)
		if formatStr != "json" {
			msg := fmt.Sprintf("Audit report written to: %s\n", outputPath)
			if outErr := cli.WriteOutput(cmd, []byte(msg)); outErr != nil {
				return errfmt.Newf("failed to write output message").Wrap(outErr)
			}
		}
	} else {
		if outErr := cli.WriteOutput(cmd, output); outErr != nil {
			return errfmt.Newf("failed to write report output").Wrap(outErr)
		}
	}

	return nil
}
