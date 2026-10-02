package system

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/drifthotspots"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/when"
)

// NewAnalyzeDriftHotspotsCmd scans Go sources for drift-prone literals (kind strings, system field keys, schema version).
// Command structure and flags: .zqk/cli/specs/system/analyze_drift_hotspots_command.yaml (builder: bldr_cli_cmd_v1.NewSystemAnalyzeDriftHotspotsCommandBuilder).
func NewAnalyzeDriftHotspotsCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemAnalyzeDriftHotspotsCommandBuilder(), &cobra.Command{Use: "analyze-drift-hotspots"})
	cli.BindAsyncProgress(cmd, runAnalyzeDriftHotspots)
	return cmd
}

func runAnalyzeDriftHotspots(cmd *cobra.Command, _ []string) error {
	ctx, logger, err := resolveContextAndLogger(cmd, systemProfileHuman)
	if err != nil {
		return cli.Guard(cmd).Err(err).Return()
	}

	root, _ := cmd.Flags().GetString("root")
	if when.IsEmpty(root) {
		root = ProjectRootOrResolve(ctx.ProjectRoot)
	}
	if when.IsEmpty(root) {
		err := errfmt.Errorf("could not resolve project root; set --root or run from repo")
		logging.Fluent(logger).Error("analyze-drift-hotspots failed", err).Log()
		return cli.Guard(cmd).Err(err).Return()
	}

	specsDir, _ := cmd.Flags().GetString("specs-dir")
	if when.IsEmpty(specsDir) {
		specsDir = filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
	}

	includeTests, _ := cmd.Flags().GetBool("include-tests")
	minRisk, _ := cmd.Flags().GetString("min-risk")
	extraKinds, _ := cmd.Flags().GetString("extra-kinds")

	extraSet := map[string]struct{}{}
	for p := range strings.SplitSeq(extraKinds, ",") {
		p = strings.TrimSpace(p)
		if !when.IsEmpty(p) {
			extraSet[p] = struct{}{}
		}
	}

	includeBarrels, _ := cmd.Flags().GetBool("include-constant-barrels")

	opts := drifthotspots.Options{
		RootDir:                root,
		SpecsDir:               specsDir,
		IncludeTests:           includeTests,
		MinRisk:                minRisk,
		ExtraKindSet:           extraSet,
		IncludeConstantBarrels: includeBarrels,
	}

	findings, err := drifthotspots.Analyze(opts)
	if err != nil {
		logging.Fluent(logger).Error("analyze-drift-hotspots failed", err).Log()
		return cli.Guard(cmd).Err(err).Return()
	}

	report := drifthotspots.BuildReport(opts, findings)

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, report)
	default:
		return writeDriftHotspotsText(cmd, report)
	}
}

func writeDriftHotspotsText(cmd *cobra.Command, report drifthotspots.Report) error {
	summaryOnly, _ := cmd.Flags().GetBool("summary-only")
	details, _ := cmd.Flags().GetBool("details")
	topN, _ := cmd.Flags().GetInt("top")
	if topN <= 0 {
		topN = 25
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Drift hotspot scan\n")
	fmt.Fprintf(&b, "  root: %s\n", report.RootDir)
	fmt.Fprintf(&b, "  specs: %s\n", report.SpecsDir)
	fmt.Fprintf(&b, "  include_tests: %v\n", report.IncludeTests)
	fmt.Fprintf(&b, "  include_constant_barrels: %v\n", report.IncludeConstantBarrels)
	fmt.Fprintf(&b, "  min_risk_floor: %s\n", report.MinRiskFloor)
	fmt.Fprintf(&b, "  findings: %d (files with hits: %d)\n\n", len(report.Findings), len(report.FileSummaries))

	if len(report.CountByRisk) > 0 {
		fmt.Fprintf(&b, "By risk: ")
		first := true
		for _, k := range []string{drifthotspots.RiskCritical, drifthotspots.RiskHigh, drifthotspots.RiskMedium, drifthotspots.RiskLow} {
			if n := report.CountByRisk[k]; n > 0 {
				if !first {
					b.WriteString(", ")
				}
				first = false
				fmt.Fprintf(&b, "%s=%d", k, n)
			}
		}
		b.WriteString("\n")
	}
	if len(report.CountByCategory) > 0 {
		fmt.Fprintf(&b, "By category: ")
		first := true
		for _, k := range []string{
			drifthotspots.CategoryKindLiteralCompare,
			drifthotspots.CategoryKindLiteralAssign,
			drifthotspots.CategorySystemFieldKey,
			drifthotspots.CategorySchemaVersionLiteral,
		} {
			if n := report.CountByCategory[k]; n > 0 {
				if !first {
					b.WriteString(", ")
				}
				first = false
				fmt.Fprintf(&b, "%s=%d", k, n)
			}
		}
		b.WriteString("\n\n")
	}

	if len(report.FileSummaries) > 0 {
		limit := topN
		if limit > len(report.FileSummaries) {
			limit = len(report.FileSummaries)
		}
		fmt.Fprintf(&b, "Top files by finding count (showing %d of %d):\n", limit, len(report.FileSummaries))
		for i := 0; i < limit; i++ {
			fs := report.FileSummaries[i]
			fmt.Fprintf(&b, "  %4d  %s\n", fs.Count, fs.File)
		}
		if !summaryOnly {
			b.WriteString("\nTip: use --summary-only for totals only; add --details for every line.\n")
		}
		b.WriteByte('\n')
	}

	if summaryOnly {
		return cli.WriteOutput(cmd, []byte(b.String()))
	}
	if !details {
		fmt.Fprintf(&b, "Omitted %d line-level findings. Re-run with --details to list each hit.\n", len(report.Findings))
		return cli.WriteOutput(cmd, []byte(b.String()))
	}

	for _, row := range report.Findings {
		fmt.Fprintf(&b, "%s:%d:%d [%s] %s %q\n  %s\n",
			row.File, row.Line, row.Column, row.Risk, row.Category, row.Literal, row.Hint)
	}
	return cli.WriteOutput(cmd, []byte(b.String()))
}
