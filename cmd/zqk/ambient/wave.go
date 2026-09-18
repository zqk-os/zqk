package ambient

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

func newWaveCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAmbientWaveCommandBuilder()
	cmd.RunE = runWave
	return cmd
}

func runWave(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		ctx := proc.OperationContext()
		secCtx := proc.SecurityContext()
		storageCtx := proc.StorageContext()
		sp := proc.Storage()
		root := proc.ProjectRoot()

		// 1. Gather command_metrics
		cmdRes, _ := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: "command_metric"})
		cmdErrs := 0
		for _, obj := range cmdRes.Objects {
			if f, ok := obj[objects.FieldKeyFailureCount].(float64); ok {
				cmdErrs += int(f)
			} else if i, ok := obj[objects.FieldKeyFailureCount].(int); ok {
				cmdErrs += i
			}
		}

		// 2. Gather scheduler_health_metric
		schRes, _ := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: "scheduler_health_metric"})
		schStuck := 0
		for _, obj := range schRes.Objects {
			st, _ := obj[objects.FieldKeyStatus].(string)
			if strings.EqualFold(st, "error") || strings.EqualFold(st, "stuck") {
				schStuck++
			}
		}

		// 3. Gather audit_aggregation_metric
		audRes, _ := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: "audit_aggregation_metric"})
		audCount := len(audRes.Objects)

		// 4. Gather system-check from KernelAmbience
		amb := whatsnext.LoadKernelAmbience(root)

		// 4b. Human-log error/warn clusters (BLI-METRICS-ERROR-WARN-SIGNAL-001)
		logClusters := whatsnext.ParseHumanLogClusters(root)

		// 5. Compute ranked next_admin_action
		var actions []string
		if amb.GhostRefCount > 0 {
			actions = append(actions, fmt.Sprintf("heal-dangling: %d GhostRefs detected", amb.GhostRefCount))
		}
		if amb.BlockingIssues > 0 {
			actions = append(actions, fmt.Sprintf("triage-system-check: %d blocking issues", amb.BlockingIssues))
		}
		actions = append(actions, whatsnext.RankedHumanLogActions(logClusters)...)
		if schStuck > 0 {
			actions = append(actions, fmt.Sprintf("triage-scheduler: %d stuck queues", schStuck))
		}
		if cmdErrs > 0 {
			actions = append(actions, fmt.Sprintf("triage-commands: %d command failures", cmdErrs))
		}
		if len(actions) == 0 {
			actions = append(actions, "monitor: kernel healthy")
		}

		metricsPath := filepath.Join(root, paths.ProjectDataDir, paths.MetricsDir, paths.CommandMetricsFile)
		var highFail, slow, timeouts, churn int
		if store, err := clipkg.NewFileMetricsStore(metricsPath); err == nil {
			if analyzer := clipkg.NewMetricsAnalyzer(store); analyzer != nil {
				if analysis, err := analyzer.Analyze(); err == nil && analysis != nil {
					highFail = len(analysis.HighFailureRateCommands)
					slow = len(analysis.SlowCommands)
					timeouts = len(analysis.FrequentTimeouts)
					churn = len(analysis.ChurnIndicators)
				}
			}
		}

		rollup := whatsnext.MetricsRollupSnapshot{
			CommandErrors:           cmdErrs,
			SchedulerStuckCount:     schStuck,
			AuditEventsCount:        audCount,
			GhostRefCount:           amb.GhostRefCount,
			SystemCheckIssues:       amb.TotalIssues,
			NextAdminAction:         actions[0],
			RankedActions:           actions,
			MeasuredAt:              time.Now().UTC().Format(time.RFC3339),
			HighFailureRateCommands: highFail,
			SlowCommands:            slow,
			FrequentTimeouts:        timeouts,
			ChurnIndicators:         churn,
			TestBundleEvidence:      "non-metric",
			TopErrorClusters:        logClusters.Errors,
			TopWarnClusters:         logClusters.Warns,
		}

		// Write to .zqk/state/ambient/metrics-rollup.json
		rollupPath := filepath.Join(root, paths.ProjectDataDir, "state", "ambient", "metrics-rollup.json")
		_ = fileutil.MkdirAll(filepath.Dir(rollupPath), 0755)
		if b, err := json.MarshalIndent(rollup, "", "  "); err == nil {
			_ = fileutil.WriteFile(rollupPath, b, 0644)
		}

		// Post to agent_feed
		summary := fmt.Sprintf("Overnight Metrics Wave: %s | CmdErrs:%d SchStuck:%d Audits:%d Ghost:%d",
			rollup.NextAdminAction, rollup.CommandErrors, rollup.SchedulerStuckCount, rollup.AuditEventsCount, rollup.GhostRefCount)
		_, _ = agentfeed.AppendEvent(agentfeed.AppendEventInput{
			ProjectRoot: root,
			Message:     summary,
			AgentID:     "kernel-ambient-wave",
			PersonaRef:  objects.ConstPersonaDefaultAgent,
			Sender:      agentfeed.FeedSenderMeshStatus,
			EventType:   agentfeed.FeedEventTypeMeshStatus,
			SelfACK:     true,
		})

		return cli.FormatOutput(cmd, map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusSuccess,
			"rollup":               rollup,
		})
	})(cmd, args)
}
