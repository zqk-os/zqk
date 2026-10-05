package workflow

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	observerpkg "github.com/zqk-os/zqk/pkg/observer"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewCoachCmd creates the coach command.
func NewCoachCmd() *cobra.Command {
	var updateTips bool
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewWorkflowCoachCommandBuilder(), &cobra.Command{
		Use:   "coach",
		Short: "Observer coach for generating dynamic workflow tips",
		Long:  "Acts as the real-time observer coach, generating dynamic tips by evaluating the system state.",
	})
	cli.RequireStorage(cmd, true)
	cli.BindAsyncProgress(cmd, cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		ctx, sp, _ := processorContexts(proc)
		projectRoot := proc.ResolvedProjectRoot()

		if updateTips {
			logging.Fluent(logger).Info("Generating dynamic observer tips").Log()

			secCtx := pkgctx.GetSecurityContext(ctx)

			// 1. Get active priority plans
			var activePlans []string
			listRes, err := sp.List(ctx, secCtx, pkgctx.NewStorageContext(), storage.ListFilter{
				Kind: objects.KindPriorityPlan,
				Filters: map[string]any{
					objects.FieldKeyStatus: map[string]any{
						"$in": []string{"active", "in_progress", "grooming"},
					},
				},
				Fields: []string{objects.FieldKeyID, objects.FieldKeyStatus},
			})
			if err == nil {
				for _, obj := range listRes.Objects {
					status, _ := obj[objects.FieldKeyStatus].(string)
					if status == "active" || status == "in_progress" || status == "grooming" {
						if pid, _ := obj[objects.FieldKeyID].(string); pid != "" {
							activePlans = append(activePlans, pid)
						}
					}
				}
			}

			// 2. Get active convergence sessions
			var activeSessions []string
			listSess, err := sp.List(ctx, secCtx, pkgctx.NewStorageContext(), storage.ListFilter{
				Kind: objects.KindConvergenceSession,
				Filters: map[string]any{
					objects.FieldKeyStatus: map[string]any{
						"$in": []string{"active", "paused"},
					},
				},
				Fields: []string{objects.FieldKeyID, objects.FieldKeyStatus},
			})
			if err == nil {
				for _, obj := range listSess.Objects {
					status, _ := obj[objects.FieldKeyStatus].(string)
					if status == "active" || status == "paused" {
						if sid, _ := obj[objects.FieldKeyID].(string); sid != "" {
							activeSessions = append(activeSessions, sid)
						}
					}
				}
			}

			// 3. Get scheduler status (check if running)
			schedulerStatus := "inactive"
			schedPidFile := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerSubdir, "scheduler.pid")
			if _, err := fileutil.Stat(schedPidFile); err == nil {
				schedulerStatus = "active"
			}

			// 4. Construct a dynamic system state summary
			activePlansStr := "none"
			if len(activePlans) > 0 {
				activePlansStr = strings.Join(activePlans, ", ")
			}
			activeSessionsStr := "none"
			if len(activeSessions) > 0 {
				activeSessionsStr = strings.Join(activeSessions, ", ")
			}

			systemStateSummary := fmt.Sprintf(
				"Active plans: %s. Scheduler status: %s. Convergence Session: %s. Swarm parallelism: moderate.",
				activePlansStr,
				schedulerStatus,
				activeSessionsStr,
			)

			llmClient := llm.NewClient(ctx, nil)
			tips, err := observerpkg.GenerateDynamicTips(ctx, llmClient, projectRoot, systemStateSummary)
			if err != nil {
				return err
			}

			if err := observerpkg.WriteCachedTips(projectRoot, tips); err != nil {
				return err
			}
			logging.Fluent(logger).Info("Updated observer tips").Int("count", len(tips)).Log()
			return cli.FormatOutput(cmd, map[string]any{"tips": tips})
		}

		// Just read
		tips := observerpkg.ReadCachedTips(projectRoot)
		return cli.FormatOutput(cmd, map[string]any{"tips": tips})
	}))
	cmd.Flags().BoolVar(&updateTips, "update-tips", false, "Generate new tips and write to state cache")
	cli.AddCommonFlags(cmd)
	return cmd
}
