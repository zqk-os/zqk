package reports

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewPCSCmd creates a command to get Project Confidence Score
func NewPCSCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Get Project Confidence Score (PCS)",
		"Calculate and display the Project Confidence Score (PCS), a metric that",
		"indicates overall project health and confidence level (0-100).",
		"",
		"The PCS is enhanced with Git commit data when available:",
		"  - Commit frequency and patterns",
		"  - Recent activity levels",
		"  - Code change momentum",
		"",
		"Higher scores indicate:",
		"  - More completed work items",
		"  - Healthy commit patterns",
		"  - Positive development momentum",
		"",
		"Lower scores may indicate:",
		"  - Low completion rates",
		"  - Irregular commit patterns",
		"  - Declining activity",
	).
		AddExample("Get PCS score", "%s reports pcs").
		AddExample("Get PCS as JSON", "%s reports pcs --format json").
		AddExample("Get PCS with commit data explicitly enabled", "%s reports pcs --include-commit-data").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewReportsPcsCommandBuilder(), &cobra.Command{
		Use:  "pcs",
		Args: cobra.NoArgs,
		Annotations: map[string]string{
			"mcp.permissions": "read:metrics",
		},
	})

	helpBuilder.ApplyToCommand(cmd)
	cli.BindAsyncProgress(cmd, runPCS)
	cmd.Flags().Bool("include-commit-data", false, "Explicitly include Git commit data (auto-detected by default)")
	cli.AddCommonFlags(cmd)

	return cmd
}

func runPCS(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}

	projectRoot := ctx.ProjectRoot
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
		if projectRoot == emptyValue {
			return errfmt.Errorf("project root not found")
		}
	}

	// Get storage provider
	storageFactory, err := storage.NewStorageFactory(cmd.Context(), projectRoot)
	if err != nil {
		return errfmt.Newf("failed to initialize storage").Wrap(err)
	}
	storageProvider := storageFactory.GetStorage()

	// Create security context
	secCtx := pkgctx.NewSystemSecurityContext()

	// Get include commit data flag
	//nolint:errcheck // Flag get - error indicates flag not set, default used
	includeCommitData, _ := cmd.Flags().GetBool("include-commit-data")

	// Calculate metrics
	projectMetrics, err := metrics.CalculateProjectMetrics(
		pkgctx.NewSystemContext(),
		storageProvider,
		secCtx,
		"default", // projectID - could be enhanced to accept as flag
		includeCommitData,
	)
	if err != nil {
		return errfmt.Newf("failed to calculate PCS").Wrap(err)
	}

	// Output based on format
	format := cli.GetFormat(cmd)
	switch format {
	case cli.FormatJSON:
		return outputPCSJSON(cmd, projectMetrics.PCS)
	case cli.FormatYAML:
		return outputPCSYAML(cmd, projectMetrics.PCS)
	default:
		return outputPCSTable(cmd, projectMetrics.PCS)
	}
}

func outputPCSTable(cmd *cobra.Command, pcs float64) error {
	var buf strings.Builder

	buf.WriteString("\nProject Confidence Score (PCS)\n")
	buf.WriteString("=============================\n\n")
	fmt.Fprintf(&buf, "Score: %.2f / 100.00\n\n", pcs)

	// Provide interpretation
	switch {
	case pcs >= 80:
		buf.WriteString("Status: Excellent - High confidence in project delivery\n")
	case pcs >= 60:
		buf.WriteString("Status: Good - Project on track with minor concerns\n")
	case pcs >= 40:
		buf.WriteString("Status: Fair - Some concerns, monitor closely\n")
	case pcs >= 20:
		buf.WriteString("Status: Poor - Significant concerns, action needed\n")
	default:
		buf.WriteString("Status: Critical - Immediate attention required\n")
	}

	return cli.WriteOutput(cmd, []byte(buf.String()))
}

func outputPCSJSON(cmd *cobra.Command, pcs float64) error {
	return cli.FormatOutputAs(cmd, cli.FormatJSON, map[string]any{
		"pcs":                  pcs,
		objects.FieldKeyStatus: getPCSStatus(pcs),
	})
}

func outputPCSYAML(cmd *cobra.Command, pcs float64) error {
	return cli.FormatOutputAs(cmd, cli.FormatYAML, map[string]any{
		"pcs":                  pcs,
		objects.FieldKeyStatus: getPCSStatus(pcs),
	})
}

func getPCSStatus(pcs float64) string {
	switch {
	case pcs >= 80:
		return "excellent"
	case pcs >= 60:
		return "good"
	case pcs >= 40:
		return "fair"
	case pcs >= 20:
		return "poor"
	default:
		return "critical"
	}
}
