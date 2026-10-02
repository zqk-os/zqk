package reports

import (
	"github.com/spf13/cobra"
	cli "github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewReportsCmd creates a new reports command group
func NewReportsCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Generate AI metrics reports (PCS, EDD, D&B)",
		"Generate AI metrics reports including Project Confidence Score (PCS),",
		"Effort Distribution Discrepancy (EDD), and Dependencies & Blockers (D&B).",
		"",
		"These metrics are enhanced with Git commit data when available, providing",
		"predictive insights into project health, effort distribution, and potential",
		"blockers or dependencies.",
	).
		AddExample("Get Project Confidence Score", "%s reports pcs").
		AddExample("Get Effort Distribution Discrepancy", "%s reports edd").
		AddExample("Get Dependencies & Blockers", "%s reports blockers").
		AddExample("Get all metrics as JSON", "%s reports pcs --format json").
		AddExample("Quick lifecycle report (questions or milestones-overdue)", "%s reports quick --preset questions")

	reportsCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewReportsCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use: "reports",
	})

	helpBuilder.ApplyToCommand(reportsCmd)

	reportsCmd.AddCommand(NewPCSCmd())
	reportsCmd.AddCommand(NewEDDCmd())
	reportsCmd.AddCommand(NewBlockersCmd())
	reportsCmd.AddCommand(NewQuickCmd())

	return reportsCmd
}

func bindReportCommand(cmd *cobra.Command, helpBuilder *clipkg.HelpBuilder, runFn func(cmd *cobra.Command, args []string) error) *cobra.Command {
	helpBuilder.ApplyToCommand(cmd)
	cli.BindAsyncProgress(cmd, runFn)
	cmd.Flags().Bool("include-commit-data", false, "Explicitly include Git commit data (auto-detected by default)")
	cli.AddCommonFlags(cmd)
	return cmd
}

func calculateReportMetrics(cmd *cobra.Command, metricLabel string) (*metrics.ProjectMetrics, error) {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return nil, errfmt.Errorf("failed to get context")
	}

	projectRoot := ctx.ProjectRoot
	if projectRoot == "" {
		projectRoot = cli.ResolveProjectRoot(".")
		if projectRoot == "" {
			return nil, errfmt.Errorf("project root not found")
		}
	}

	storageFactory, err := storage.NewStorageFactory(cmd.Context(), projectRoot)
	if err != nil {
		return nil, errfmt.Newf("failed to initialize storage").Wrap(err)
	}
	storageProvider := storageFactory.GetStorage()

	secCtx := pkgctx.NewSystemSecurityContext()

	//nolint:errcheck // Flag get - error indicates flag not set, default used
	includeCommitData, _ := cmd.Flags().GetBool("include-commit-data")

	projectMetrics, err := metrics.CalculateProjectMetrics(
		pkgctx.NewSystemContext(),
		storageProvider,
		secCtx,
		"default",
		includeCommitData,
	)
	if err != nil {
		return nil, errfmt.Newf("failed to calculate %s", metricLabel).Wrap(err)
	}
	return projectMetrics, nil
}
