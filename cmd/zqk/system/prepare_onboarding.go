package system

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// PrepareOnboardingResult is the result of prepare-onboarding for --format json/yaml (BLI-806).
type PrepareOnboardingResult struct {
	OnboardingJobEnsured bool                       `json:"onboarding_job_ensured" yaml:"onboarding_job_ensured"`
	Message              string                     `json:"message" yaml:"message"`
	Maintenance          *EnsureRetentionJobsResult `json:"maintenance,omitempty" yaml:"maintenance,omitempty"`
}

// NewPrepareOnboardingCmd creates the prepare-onboarding command (BLI-806).
// Ensures the onboarding roadmap seed scheduler job exists; optionally ensures maintenance bundle via --with-maintenance.
func NewPrepareOnboardingCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemPrepareOnboardingCommandBuilder(), &cobra.Command{
		Use:   "prepare-onboarding",
		Short: "Prepare project for agent onboarding (roadmap seed job, optional maintenance)",
		Long: `Ensure the onboarding roadmap seed scheduler job exists so that when the scheduler runs,
the curriculum (priority plan, workstream, backlog items) is created. Optionally ensure the maintenance
bundle (retention, audit aggregation, etc.) with --with-maintenance.`,
		Args: cobra.NoArgs,
		RunE: runPrepareOnboarding,
	})
	cmd.Flags().Bool("with-maintenance", false, "Also run ensure-retention-jobs to create the maintenance bundle")
	cli.BindAsyncProgress(cmd, runPrepareOnboarding)
	return cmd
}

func runPrepareOnboarding(cmd *cobra.Command, _ []string) error {
	ctx, logger, err := resolveContextAndLogger(cmd, string(pkgctx.ProfileSystem))
	if err != nil {
		return cli.Guard(cmd).Err(err).Return()
	}
	projectRoot := ProjectRootOrResolve(ctx.ProjectRoot)
	if projectRoot == emptyValue {
		return cli.Guard(cmd).Err(errfmt.Errorf("project root not found; run from project dir or set ZQK_PROJECT_ROOT")).Return()
	}

	if err := EnsureOnboardingRoadmapJobInProject(projectRoot, logger); err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("ensure onboarding roadmap job: %w").Return()
	}

	result := PrepareOnboardingResult{
		OnboardingJobEnsured: true,
		Message:              "Onboarding roadmap seed job ensured. Start the scheduler to create the curriculum (priority plan, workstream, backlog items).",
	}

	withMaintenance, _ := cmd.Flags().GetBool("with-maintenance")
	if withMaintenance {
		maintenanceResult, err := EnsureRetentionJobsInProject(projectRoot, logger, nil)
		if err != nil {
			return cli.Guard(cmd).Err(err).Wrapf("ensure retention jobs: %w").Return()
		}
		result.Maintenance = maintenanceResult
		if maintenanceResult.Message != emptyValue {
			result.Message += "\n" + maintenanceResult.Message
		}
	}

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, result)
	default:
		return cli.WriteOutput(cmd, []byte(result.Message+"\n"))
	}
}
