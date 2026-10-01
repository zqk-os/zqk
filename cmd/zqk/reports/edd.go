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

// NewEDDCmd creates a command to get Effort Distribution Discrepancy
func NewEDDCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Get Effort Distribution Discrepancy (EDD)",
		"Calculate and display the Effort Distribution Discrepancy (EDD), a metric",
		"that indicates variance between estimated and actual effort distribution.",
		"",
		"The EDD is enhanced with Git commit data when available:",
		"  - Commit-to-work-item ratios",
		"  - Code change patterns relative to work items",
		"  - Activity distribution across work items",
		"",
		"Positive EDD values indicate:",
		"  - Underestimation (more effort than estimated)",
		"  - Work items may be under-scoped",
		"",
		"Negative EDD values indicate:",
		"  - Overestimation (less effort than estimated)",
		"  - Work items may be over-scoped",
		"",
		"Values closer to zero indicate better estimation accuracy.",
	).
		AddExample("Get EDD score", "%s reports edd").
		AddExample("Get EDD as JSON", "%s reports edd --format json").
		AddExample("Get EDD with commit data explicitly enabled", "%s reports edd --include-commit-data").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewReportsEddCommandBuilder(), &cobra.Command{
		Use:  "edd",
		Args: cobra.NoArgs,
		Annotations: map[string]string{
			"mcp.permissions": "read:metrics",
		},
	})

	helpBuilder.ApplyToCommand(cmd)
	cli.BindAsyncProgress(cmd, runEDD)
	cmd.Flags().Bool("include-commit-data", false, "Explicitly include Git commit data (auto-detected by default)")
	cli.AddCommonFlags(cmd)

	return cmd
}

func runEDD(cmd *cobra.Command, args []string) error {
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
		return errfmt.Newf("failed to calculate EDD").Wrap(err)
	}

	result := map[string]any{
		"edd":                  projectMetrics.EDD,
		objects.FieldKeyStatus: getEDDStatus(projectMetrics.EDD),
	}
	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, result)
	default:
		return outputEDDTable(cmd, projectMetrics.EDD)
	}
}

func outputEDDTable(cmd *cobra.Command, edd float64) error {
	var buf strings.Builder

	buf.WriteString("\nEffort Distribution Discrepancy (EDD)\n")
	buf.WriteString("=====================================\n\n")
	fmt.Fprintf(&buf, "Score: %.2f%%\n\n", edd)

	// Provide interpretation
	switch {
	case edd > 20:
		buf.WriteString("Status: Significant Underestimation\n")
		buf.WriteString("  - Work items are consistently under-scoped\n")
		buf.WriteString("  - Consider reviewing estimation process\n")
	case edd > 5:
		buf.WriteString("Status: Moderate Underestimation\n")
		buf.WriteString("  - Some work items may be under-scoped\n")
		buf.WriteString("  - Monitor estimation accuracy\n")
	case edd > -5:
		buf.WriteString("Status: Good Estimation Accuracy\n")
		buf.WriteString("  - Effort distribution aligns well with estimates\n")
	case edd > -20:
		buf.WriteString("Status: Moderate Overestimation\n")
		buf.WriteString("  - Some work items may be over-scoped\n")
		buf.WriteString("  - Consider refining estimation process\n")
	default:
		buf.WriteString("Status: Significant Overestimation\n")
		buf.WriteString("  - Work items are consistently over-scoped\n")
		buf.WriteString("  - Review estimation process and adjust\n")
	}

	return cli.WriteOutput(cmd, []byte(buf.String()))
}

func getEDDStatus(edd float64) string {
	switch {
	case edd > 20:
		return "significant_underestimation"
	case edd > 5:
		return "moderate_underestimation"
	case edd > -5:
		return "good_accuracy"
	case edd > -20:
		return "moderate_overestimation"
	default:
		return "significant_overestimation"
	}
}
