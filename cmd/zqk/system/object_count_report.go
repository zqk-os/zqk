package system

import (
	"strconv"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/systemcheck/congruence"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const pipelineKindRunObjectCountReport = "system.run_object_count_report"

// Backward-compatible type aliases for any legacy internal callers
type (
	CacheStatus                  = congruence.CacheStatus
	ProcessIntegrity             = congruence.ProcessIntegrity
	ObjectCountReportEntry       = congruence.ObjectCountReportEntry
	ObjectCountReportsIndex      = congruence.ObjectCountReportsIndex
	ObjectCountDashboardSnapshot = congruence.ObjectCountDashboardSnapshot
)

// NewObjectCountReportCmd creates a command that runs operational congruence report
// (disk vs object vs internal counts), writes to file, and can emit metrics/alerts via coordinator.
// Command structure and flags are from .zqk/cli/specs/system/object_count_report_command.yaml.
func NewObjectCountReportCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemObjectCountReportCommandBuilder(), &cobra.Command{Use: "object-count-report"})
	cmd.Flags().Int("metrics-chunk-retention-days", 0,
		"Prune metrics time-series .chunk files older than N days under .zqk/metrics/ (object_volume, stream_volume, filesystem_snapshot). 0 uses "+zqkenv.MetricsChunkRetentionDays().Name()+" or default "+strconv.Itoa(congruence.DefaultMetricsChunkRetentionDays))
	cli.BindAsyncProgress(cmd, runObjectCountReport)
	return cmd
}

func runObjectCountReport(cmd *cobra.Command, args []string) error {
	type state struct {
		err error
	}
	st := &state{}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	runCtx := cmd.Context()
	if runCtx == nil {
		runCtx = pkgctx.NewSystemContext()
	}

	pl := pipeline.NewBuilder(pipelineKindRunObjectCountReport, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage(pipeline.StageIngest, func(pctx *pipeline.Context, payload any) (any, error) {
			st.err = runObjectCountReportImpl(cmd, args)
			return st, nil
		}).
		AddStage(pipeline.StageFinalize, func(pctx *pipeline.Context, payload any) (any, error) {
			return st, nil
		}).
		Build()

	if _, runErr := pl.RunWithContext(runCtx, st); runErr != nil {
		return runErr
	}
	return st.err
}

func runObjectCountReportImpl(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		var flags clipkg.FlagBag
		outputPath := flags.String(cmd, "report-file")
		emitEvents := flags.Bool(cmd, "emit-events")
		includeInternal := flags.Bool(cmd, "include-internal")
		disparityThreshold := flags.Int(cmd, "disparity-threshold")
		noCache := flags.Bool(cmd, "no-cache")
		includeFilesystemSnapshot := flags.Bool(cmd, "include-filesystem-snapshot")
		filesystemSnapshotScopeStr := flags.String(cmd, "filesystem-snapshot-scope")
		retentionDays := flags.Int(cmd, "metrics-chunk-retention-days")
		if err := flags.Err(); err != nil {
			return err
		}

		projectRoot := resolveProjectRootFromCommand(cmd)

		reportFileFlag := cmd.Flags().Lookup("report-file")
		userProvidedReportFile := reportFileFlag != nil && reportFileFlag.Changed

		storageProvider := proc.Storage()
		secCtx := proc.SecurityContext()

		opts := congruence.Options{
			OutputPath:                 outputPath,
			EmitEvents:                 emitEvents,
			IncludeInternal:            includeInternal,
			DisparityThreshold:         disparityThreshold,
			NoCache:                    noCache,
			IncludeFilesystemSnapshot:  includeFilesystemSnapshot,
			FilesystemSnapshotScopeStr: filesystemSnapshotScopeStr,
			MetricsChunkRetentionDays:  retentionDays,
			UserProvidedReportFile:     userProvidedReportFile,
		}

		_, err := congruence.RunCongruence(cmd.Context(), projectRoot, storageProvider, secCtx, opts)
		return err
	})(cmd, nil)
}
