package system

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/idebridge"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

// NewMetricsFeedCmd creates a command to push metrics digests to agent_feed
func NewMetricsFeedCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Push metrics digest to agent_feed with notify",
		"Analyzes command metrics and pushes a digest to agent_feed so seats can self-direct.",
		"",
		"This command satisfies CAP metrics-plane directive feeder facets (GLS-1786416188034709000).",
	).
		AddExample("Push metrics digest", "%s system metrics feed").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use:  "feed",
		Args: cobra.NoArgs,
		RunE: runMetricsFeed,
	}

	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().String("agent-id", "kernel-metrics-feeder", "Agent ID for the feed sender")
	cmd.Flags().Bool("notify", true, "Queue IDE proof of life (notify)")

	return cmd
}

func runMetricsFeed(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}

	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	agentID, _ := cmd.Flags().GetString("agent-id")
	notify, _ := cmd.Flags().GetBool("notify")

	metricsPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir, paths.CommandMetricsFile)

	dir := filepath.Dir(metricsPath)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create metrics directory").Wrap(err)
	}

	store, err := clipkg.NewFileMetricsStore(metricsPath)
	if err != nil {
		return errfmt.Newf("failed to load metrics store").Wrap(err)
	}

	analyzer := clipkg.NewMetricsAnalyzer(store)
	analysis, err := analyzer.Analyze()
	if err != nil {
		return errfmt.Newf("failed to analyze metrics").Wrap(err)
	}

	logClusters := whatsnext.ParseHumanLogClusters(projectRoot)
	clusterStr := whatsnext.FormatHumanLogClusterDigest(logClusters)

	summaryMsg := fmt.Sprintf("METRICS_DIGEST: High Failure: %d, Slow: %d, Timeouts: %d, Churn: %d. Run `zqk system metrics --summary` for details.%s",
		len(analysis.HighFailureRateCommands),
		len(analysis.SlowCommands),
		len(analysis.FrequentTimeouts),
		len(analysis.ChurnIndicators),
		clusterStr,
	)

	amb := whatsnext.LoadKernelAmbience(projectRoot)
	if amb.Available {
		var clusters []string
		for _, c := range amb.TopIssueClusters {
			clusters = append(clusters, fmt.Sprintf("%dx %s", c.Count, c.Message))
		}
		if len(clusters) > 0 {
			summaryMsg += fmt.Sprintf(" SYSTEM_HEALTH: Blocks: %d, Warns: %d, GhostRefs: %d. Top issues: %s.", amb.BlockingIssues, amb.Warnings, amb.GhostRefCount, strings.Join(clusters, " | "))
		} else {
			summaryMsg += fmt.Sprintf(" SYSTEM_HEALTH: Blocks: %d, Warns: %d, GhostRefs: %d.", amb.BlockingIssues, amb.Warnings, amb.GhostRefCount)
		}
	}

	rollup := whatsnext.MetricsRollupSnapshot{}
	if amb.MetricsRollup != nil {
		rollup = *amb.MetricsRollup
	}
	rollup.HighFailureRateCommands = len(analysis.HighFailureRateCommands)
	rollup.SlowCommands = len(analysis.SlowCommands)
	rollup.FrequentTimeouts = len(analysis.FrequentTimeouts)
	rollup.ChurnIndicators = len(analysis.ChurnIndicators)
	rollup.TestBundleEvidence = "non-metric"
	rollup.TopErrorClusters = logClusters.Errors
	rollup.TopWarnClusters = logClusters.Warns

	// Persist to .zqk/state/ambient/metrics-rollup.json for ambient stream consumers
	rollupPath := filepath.Join(projectRoot, paths.ProjectDataDir, "state", "ambient", "metrics-rollup.json")
	_ = fileutil.MkdirAll(filepath.Dir(rollupPath), paths.DirPerm755)
	if b, err := json.MarshalIndent(rollup, "", "  "); err == nil {
		_ = fileutil.WriteFile(rollupPath, b, 0644)
	}

	res, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
		ProjectRoot: projectRoot,
		Message:     summaryMsg,
		AgentID:     agentID,
		Sender:      agentfeed.FeedSenderMeshStatus,
		EventType:   agentfeed.FeedEventTypeMeshStatus,
		SelfACK:     false,
	})
	if err != nil {
		return errfmt.Newf("failed to append metrics feed event").Wrap(err)
	}

	if notify {
		idebridge.QueueProofOfLife(projectRoot, summaryMsg)
	}

	out := map[string]any{
		"event_id":       res.EventID,
		"event_path":     res.EventPath,
		"message":        summaryMsg,
		"notified":       notify,
		"metrics_rollup": rollup,
	}

	return cli.FormatOutput(cmd, out)
}
