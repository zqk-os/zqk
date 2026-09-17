package system

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/execwrap"
	"github.com/lanceman/zqk/pkg/zqkenv"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	bldr_cli_cmd_v1 "github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/scheduler"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqktime"
)

const (
	improvementReportStatusTimeout    = 8 * time.Second
	improvementReportCountTimeout     = 60 * time.Second
	improvementReportRetentionTimeout = 15 * time.Second
	improvementReportCadenceMinChange = 6 * time.Hour
)

// ImprovementReportSnapshot is the data collected in one run for comparison and prioritization.
type ImprovementReportSnapshot struct {
	GeneratedAt     string                 `json:"generated_at"`
	ProjectRoot     string                 `json:"project_root,omitempty"`
	AdaptivePlan    *improvementAdaptive   `json:"adaptive_plan,omitempty"`
	Status          map[string]any         `json:"status,omitempty"`
	ObjectCount     map[string]any         `json:"object_count,omitempty"`
	Retention       map[string]any         `json:"retention,omitempty"`
	HealthData      map[string]any         `json:"health_data,omitempty"`
	MetricsAnalysis *improvementMetricsSum `json:"metrics_analysis,omitempty"`
	StreamCounts    map[string]int         `json:"stream_counts,omitempty"`
}

type improvementMetricsSum struct {
	HighFailureCount int `json:"high_failure_count"`
	TimeoutCount     int `json:"timeout_count"`
	SlowCount        int `json:"slow_count"`
	ChurnCount       int `json:"churn_count"`
}

// improvementAdaptive captures how report cadence/scope should adapt to current system state.
// This keeps observability bounded so the system can execute work while still steering itself.
type improvementAdaptive struct {
	Mode                     string   `json:"mode"` // lite, standard, focused
	RecommendedNextCadence   string   `json:"recommended_next_cadence"`
	Reasons                  []string `json:"reasons,omitempty"`
	CollectStatus            bool     `json:"collect_status"`
	CollectObjectCount       bool     `json:"collect_object_count"`
	CollectRetention         bool     `json:"collect_retention"`
	CollectMetricsAnalysis   bool     `json:"collect_metrics_analysis"`
	CollectStreamCounts      bool     `json:"collect_stream_counts"`
	RespectObservationBudget bool     `json:"respect_observation_budget"`
}

// ImprovementReportComparative holds deltas vs previous run.
type ImprovementReportComparative struct {
	PreviousAt         string   `json:"previous_at,omitempty"`
	ObjectCountDelta   int      `json:"object_count_delta,omitempty"`
	RetentionOver      []string `json:"retention_over_kinds,omitempty"`
	RetentionOverDelta int      `json:"retention_over_delta,omitempty"` // change in number of kinds over target
	NewBlocking        bool     `json:"new_blocking,omitempty"`
	BlockingDelta      int      `json:"blocking_delta,omitempty"`
}

// ImprovementWorkItem is a prioritized action to steer system performance, behavior, accuracy, or quality.
type ImprovementWorkItem struct {
	Priority        string `json:"priority"` // P0, P1, P2, P3
	Category        string `json:"category"` // performance, accuracy, quality, behavior
	Title           string `json:"title"`
	Description     string `json:"description,omitempty"`
	SuggestedAction string `json:"suggested_action,omitempty"`
	Metric          string `json:"metric,omitempty"`
	Value           string `json:"value,omitempty"`
}

// ImprovementReportOutput is the full report written to JSON for next-run comparison and consumption.
type ImprovementReportOutput struct {
	GeneratedAt string                       `json:"generated_at"`
	ProjectRoot string                       `json:"project_root,omitempty"`
	Snapshot    ImprovementReportSnapshot    `json:"snapshot"`
	Comparative ImprovementReportComparative `json:"comparative,omitempty"`
	WorkItems   []ImprovementWorkItem        `json:"work_items"`
	Summary     string                       `json:"summary,omitempty"`
}

type cadenceApplyResult struct {
	Applied         bool   `json:"applied"`
	SkippedReason   string `json:"skipped_reason,omitempty"`
	Recommended     string `json:"recommended,omitempty"`
	Previous        string `json:"previous,omitempty"`
	NextAllowedAt   string `json:"next_allowed_at,omitempty"`
	UpdatedSchedule string `json:"updated_schedule,omitempty"`
}

// NewImprovementReportCmd creates a command that generates a comprehensive system improvement report
// with comparative analysis and prioritized work items. Intended for hourly runs (e.g. scheduler).
func NewImprovementReportCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemImprovementReportCommandBuilder()
	cli.BindAsyncProgress(cmd, runImprovementReport)
	return cmd
}

func runImprovementReport(cmd *cobra.Command, _ []string) error {
	projectRoot := ""
	if ctx := cli.GetContext(cmd); ctx != nil {
		projectRoot = ctx.ProjectRoot
	}
	projectRoot = ProjectRootOrResolve(projectRoot)
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	reportDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, paths.LogsReportsSubdir)
	if err := fileutil.MkdirAll(reportDir, paths.DirPerm750); err != nil {
		return errfmt.Newf("mkdir reports").Wrap(err)
	}

	ts := zqktime.NowLayoutUTC(zqktime.LayoutLogRotateStamp)
	outputPath, _ := cmd.Flags().GetString("report-file")
	if outputPath == emptyValue {
		outputPath = filepath.Join(reportDir, "improvement-report-"+ts+".txt")
	}
	if filepath.Ext(outputPath) != ".txt" && filepath.Ext(outputPath) != ".json" {
		outputPath = filepath.Join(reportDir, "improvement-report-"+ts+".txt")
	}
	jsonPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".json"
	if filepath.Ext(outputPath) == ".json" {
		jsonPath = outputPath
		outputPath = strings.TrimSuffix(outputPath, ".json") + ".txt"
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	var storageProvider storage.ObjectStorageProvider
	if proc, err := cli.NewProcessor(cmd); err == nil {
		storageProvider = proc.Storage()
	}
	previous := loadPreviousImprovementReport(reportDir)
	adaptive := deriveAdaptivePlan(previous)
	snapshot, err := gatherImprovementSnapshot(cmd.Context(), projectRoot, logger, storageProvider, adaptive)
	if err != nil {
		return errfmt.Newf("gather snapshot").Wrap(err)
	}

	comparative := buildComparative(previous, snapshot)
	workItems := prioritizeWorkItems(snapshot, comparative, previous)
	summary := buildSummary(snapshot, comparative, workItems)

	out := ImprovementReportOutput{
		GeneratedAt: zqktime.NowRFC3339UTC(),
		ProjectRoot: projectRoot,
		Snapshot:    snapshot,
		Comparative: comparative,
		WorkItems:   workItems,
		Summary:     summary,
	}

	txtBytes := formatImprovementReportTxt(out)
	if err := fileutil.WriteFile(outputPath, txtBytes, paths.FilePerm600); err != nil {
		return errfmt.Newf("write report").Wrap(err)
	}
	logging.Fluent(logger).Info("Improvement report written").
		Path(outputPath).
		Log()

	jsonBytes, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return errfmt.Newf("marshal report").Wrap(err)
	}
	if err := fileutil.WriteFile(jsonPath, jsonBytes, paths.FilePerm600); err != nil {
		return errfmt.Newf("write report json").Wrap(err)
	}
	logging.Fluent(logger).Info("Improvement report JSON written").
		Path(jsonPath).
		Log()

	emitEvents, _ := cmd.Flags().GetBool("emit-events")
	if emitEvents {
		// Best-effort: emit completion event for dashboards (same pattern as object-count-report).
		emitImprovementReportEvent(cmd, projectRoot, out, logger)
	}

	triggerRemediation, _ := cmd.Flags().GetBool("trigger-remediation")
	if triggerRemediation && len(out.WorkItems) > 0 {
		triggerRemediationFromWorkItems(cmd.Context(), storageProvider, projectRoot, out.WorkItems, logger)
	}
	applyCadence, _ := cmd.Flags().GetBool("apply-adaptive-cadence")
	if applyCadence {
		cadenceResult := applyAdaptiveCadenceRecommendation(cmd.Context(), storageProvider, out, previous, logger)
		if cadenceResult.Applied {
			logging.Fluent(logger).Info("Adaptive cadence applied").
				String("previous", cadenceResult.Previous).
				String("recommended", cadenceResult.Recommended).
				String("updated_schedule", cadenceResult.UpdatedSchedule).
				Log()
		} else if cadenceResult.SkippedReason != emptyValue {
			logging.Fluent(logger).Debug("Adaptive cadence not applied").
				String("reason", cadenceResult.SkippedReason).
				String("recommended", cadenceResult.Recommended).
				String("previous", cadenceResult.Previous).
				Log()
		}
	}
	persistAudit, _ := cmd.Flags().GetBool("persist-snapshot-audit")
	if persistAudit {
		persistImprovementSnapshotAuditEvent(cmd.Context(), projectRoot, storageProvider, out, logger)
	}

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, out)
	default:
		return cli.WriteOutput(cmd, txtBytes)
	}
}

func gatherImprovementSnapshot(
	ctx context.Context,
	projectRoot string,
	logger logging.Logger,
	storageProvider storage.ObjectStorageProvider,
	adaptive *improvementAdaptive,
) (ImprovementReportSnapshot, error) {
	if adaptive == nil {
		adaptive = defaultAdaptivePlan()
	}
	snap := ImprovementReportSnapshot{
		GeneratedAt:  zqktime.NowRFC3339UTC(),
		ProjectRoot:  projectRoot,
		AdaptivePlan: adaptive,
		StreamCounts: make(map[string]int),
	}

	zqkBin := filepath.Join(projectRoot, "bin", "zqk")
	if _, err := fileutil.Stat(zqkBin); err != nil {
		zqkBin = "zqk"
	}

	if adaptive.CollectStatus {
		// System status (short timeout)
		statusCtx, cancel := context.WithTimeout(ctx, improvementReportStatusTimeout)
		statusCmd := execwrap.CommandContext(statusCtx, zqkBin, "system", "status", "--format", "json")
		zqkenv.WireExecForIsolatedProject(statusCmd, projectRoot)
		statusOut, err := statusCmd.CombinedOutput()
		timedOut := statusCtx.Err() == context.DeadlineExceeded
		cancel()
		if err == nil && !timedOut {
			var statusMap map[string]any
			if json.Unmarshal(statusOut, &statusMap) == nil {
				snap.Status = statusMap
			}
		} else if timedOut {
			logging.Fluent(logger).Warn("system status timed out; snapshot partial").Log()
		}
	}

	// Object count: use same source as object-count-report so counts are deterministic and comparable to retention targets.
	if adaptive.CollectObjectCount {
		if storageProvider != nil {
			countsByKind := GatherObjectCountByKindForReport(ctx, projectRoot, storageProvider, false)
			if countsByKind != nil {
				var total int
				for _, n := range countsByKind {
					total += n
				}
				snap.ObjectCount = map[string]any{
					"counts_by_kind": countsByKind,
					"total_objects":  total,
					"total_kinds":    len(countsByKind),
				}
				if adaptive.CollectStreamCounts {
					for _, kind := range StreamBackedKindsForReport() {
						if n, ok := countsByKind[kind]; ok {
							snap.StreamCounts[kind] = n
						}
					}
				}
			}
		}
		if snap.ObjectCount == nil {
			// Fallback: subprocess count (e.g. when no storage in context)
			countCtx, cancel2 := context.WithTimeout(ctx, improvementReportCountTimeout)
			countCmd := execwrap.CommandContext(countCtx, zqkBin, "object", "count", "--format", "json")
			zqkenv.WireExecForIsolatedProject(countCmd, projectRoot)
			countOut, err := countCmd.CombinedOutput()
			cancel2()
			if err == nil {
				var countMap map[string]any
				if json.Unmarshal(countOut, &countMap) == nil {
					snap.ObjectCount = countMap
				}
			}
		}
	}

	if adaptive.CollectRetention {
		// Retention status
		retCtx, cancel3 := context.WithTimeout(ctx, improvementReportRetentionTimeout)
		retCmd := execwrap.CommandContext(retCtx, zqkBin, "system", "retention-status", "--format", "json")
		zqkenv.WireExecForIsolatedProject(retCmd, projectRoot)
		retOut, err := retCmd.CombinedOutput()
		cancel3()
		if err == nil {
			var retMap map[string]any
			if json.Unmarshal(retOut, &retMap) == nil {
				snap.Retention = retMap
			}
		}
	}

	// Health data (in-process)
	quarantine, _ := BuildQuarantineReportData(projectRoot)
	snap.HealthData = map[string]any{}
	if quarantine != nil {
		snap.HealthData["quarantine_total_files"] = quarantine.TotalFiles
	}
	if cs := loadCheckSummaryFromSystemHealth(projectRoot); cs != nil {
		snap.HealthData["check_blocking"] = cs.Blocking
		snap.HealthData["check_warnings"] = cs.Warnings
		snap.HealthData["check_total_objects"] = cs.TotalObjects
	}
	if sm := loadSchedulerMetricsForHealthData(); sm != nil {
		snap.HealthData["scheduler_pool_declined"] = sm.PoolCreationDeclinedCount
	}

	if adaptive.CollectMetricsAnalysis {
		// Metrics analysis
		metricsPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir, paths.CommandMetricsFile)
		store, err := clipkg.NewFileMetricsStore(metricsPath)
		if err == nil {
			analyzer := clipkg.NewMetricsAnalyzer(store)
			analysis, err := analyzer.Analyze()
			if err == nil {
				snap.MetricsAnalysis = &improvementMetricsSum{
					HighFailureCount: len(analysis.HighFailureRateCommands),
					TimeoutCount:     len(analysis.FrequentTimeouts),
					SlowCount:        len(analysis.SlowCommands),
					ChurnCount:       len(analysis.ChurnIndicators),
				}
			}
		}
	}

	// StreamCounts is populated above from the same source as object-count-report (stream-backed kinds only).
	// We no longer count lines in stream_registry_*.jsonl; that grew unbounded and did not match retention or object counts.

	return snap, nil
}

func defaultAdaptivePlan() *improvementAdaptive {
	return &improvementAdaptive{
		Mode:                     "standard",
		RecommendedNextCadence:   "1h",
		CollectStatus:            true,
		CollectObjectCount:       true,
		CollectRetention:         true,
		CollectMetricsAnalysis:   true,
		CollectStreamCounts:      true,
		RespectObservationBudget: true,
	}
}

func deriveAdaptivePlan(previous *ImprovementReportOutput) *improvementAdaptive {
	plan := defaultAdaptivePlan()
	if previous == nil {
		plan.Reasons = []string{"no previous report found; using standard cadence and full snapshot scope"}
		return plan
	}

	var reasons []string
	highLoadSignals := 0
	unstableSignals := 0

	if previous.Snapshot.HealthData != nil {
		if v, ok := previous.Snapshot.HealthData["scheduler_pool_declined"].(float64); ok && int64(v) > 0 {
			highLoadSignals++
			reasons = append(reasons, "scheduler reported pool creation declines")
		}
		if v, ok := previous.Snapshot.HealthData["scheduler_pool_declined"].(int64); ok && v > 0 {
			highLoadSignals++
			reasons = append(reasons, "scheduler reported pool creation declines")
		}
		if b, ok := previous.Snapshot.HealthData["check_blocking"].(int); ok && b > 0 {
			unstableSignals++
			reasons = append(reasons, "blocking validation issues present")
		}
		if b, ok := previous.Snapshot.HealthData["check_blocking"].(float64); ok && int(b) > 0 {
			unstableSignals++
			reasons = append(reasons, "blocking validation issues present")
		}
	}
	if previous.Snapshot.MetricsAnalysis != nil {
		if previous.Snapshot.MetricsAnalysis.TimeoutCount >= 10 || previous.Snapshot.MetricsAnalysis.SlowCount >= 10 {
			highLoadSignals++
			reasons = append(reasons, "high timeout/slow-command pressure in prior metrics analysis")
		}
		if previous.Snapshot.MetricsAnalysis.HighFailureCount > 0 {
			unstableSignals++
			reasons = append(reasons, "command reliability issues detected")
		}
	}
	if len(retentionOverKinds(previous.Snapshot.Retention)) > 0 {
		unstableSignals++
		reasons = append(reasons, "retention drift detected (kinds over target)")
	}

	switch {
	case highLoadSignals > 0:
		plan.Mode = "lite"
		plan.RecommendedNextCadence = "2h"
		plan.CollectStatus = false
		plan.CollectObjectCount = false
		plan.CollectRetention = false
		plan.CollectStreamCounts = false
	case unstableSignals > 0:
		plan.Mode = "focused"
		plan.RecommendedNextCadence = "30m"
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "system stable in prior snapshot; maintain standard cadence")
	}
	plan.Reasons = reasons
	return plan
}

func cadenceToCron(cadence string) string {
	switch cadence {
	case "30m":
		return "*/30 * * * *"
	case "2h":
		return "0 */2 * * *"
	default:
		// 1h default
		return "0 * * * *"
	}
}

func parseTimeRFC3339(v string) (time.Time, bool) {
	if v == emptyValue {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func applyAdaptiveCadenceRecommendation(
	ctx context.Context,
	storageProvider storage.ObjectStorageProvider,
	current ImprovementReportOutput,
	previous *ImprovementReportOutput,
	logger logging.Logger,
) cadenceApplyResult {
	res := cadenceApplyResult{
		Recommended: current.Snapshot.AdaptivePlan.RecommendedNextCadence,
	}
	if storageProvider == nil {
		res.SkippedReason = "storage provider unavailable"
		return res
	}
	if current.Snapshot.AdaptivePlan == nil {
		res.SkippedReason = "adaptive plan missing"
		return res
	}
	if res.Recommended == emptyValue {
		res.SkippedReason = "adaptive recommendation empty"
		return res
	}

	// Avoid thrashing cadence changes: only allow changes after cooldown from previous report generation.
	if previous != nil {
		prevCadence := ""
		if previous.Snapshot.AdaptivePlan != nil {
			prevCadence = previous.Snapshot.AdaptivePlan.RecommendedNextCadence
		}
		res.Previous = prevCadence
		if prevCadence != emptyValue && prevCadence != res.Recommended {
			if prevAt, ok := parseTimeRFC3339(previous.GeneratedAt); ok {
				nextAllowed := prevAt.Add(improvementReportCadenceMinChange)
				res.NextAllowedAt = zqktime.FormatRFC3339UTC(nextAllowed)
				if time.Now().UTC().Before(nextAllowed) {
					res.SkippedReason = "cadence change cooldown active"
					return res
				}
			}
		}
	}

	targetCron := cadenceToCron(res.Recommended)

	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{
		Kind: objects.KindSchedulerJob,
		Filters: map[string]any{
			objects.FieldKeyID: map[string]any{"$eq": "SCH-improvement-report"},
		},
		Limit: 1,
	}
	qr, err := storageProvider.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		res.SkippedReason = "failed to list SCH-improvement-report"
		return res
	}
	if len(qr.Objects) == 0 {
		res.SkippedReason = "SCH-improvement-report job not found"
		return res
	}
	job := qr.Objects[0]
	currentExpr, _ := job[objects.FieldKeyScheduleExpression].(string)
	res.Previous = currentExpr
	if currentExpr == targetCron {
		res.SkippedReason = "schedule already matches recommendation"
		return res
	}

	if err := storageProvider.Update(ctx, secCtx, "SCH-improvement-report", map[string]any{
		objects.FieldKeyScheduleExpression: targetCron,
		objects.FieldKeyNotes:              fmt.Sprintf("Adaptive cadence updated by improvement-report at %s (recommended=%s)", zqktime.NowRFC3339UTC(), res.Recommended),
	}); err != nil {
		res.SkippedReason = "failed to update scheduler cadence"
		logging.Fluent(logger).Warn("Failed to apply adaptive cadence").
			String("target", targetCron).
			WithError(err).
			Log()
		return res
	}
	res.Applied = true
	res.UpdatedSchedule = targetCron
	return res
}

func countWorkItemsByPriority(workItems []ImprovementWorkItem) map[string]int {
	counts := map[string]int{"P0": 0, "P1": 0, "P2": 0, "P3": 0}
	for _, w := range workItems {
		if _, ok := counts[w.Priority]; ok {
			counts[w.Priority]++
		}
	}
	return counts
}

func compactSnapshotMetadata(out ImprovementReportOutput) map[string]any {
	md := map[string]any{
		"report_generated_at":   out.GeneratedAt,
		objects.FieldKeySummary: out.Summary,
	}
	if out.Snapshot.AdaptivePlan != nil {
		md["adaptive_mode"] = out.Snapshot.AdaptivePlan.Mode
		md["recommended_next_cadence"] = out.Snapshot.AdaptivePlan.RecommendedNextCadence
		md["adaptive_reasons"] = out.Snapshot.AdaptivePlan.Reasons
	}
	if out.Snapshot.MetricsAnalysis != nil {
		md["metrics_analysis"] = map[string]any{
			"high_failure_count":         out.Snapshot.MetricsAnalysis.HighFailureCount,
			objects.FieldKeyTimeoutCount: out.Snapshot.MetricsAnalysis.TimeoutCount,
			"slow_count":                 out.Snapshot.MetricsAnalysis.SlowCount,
			"churn_count":                out.Snapshot.MetricsAnalysis.ChurnCount,
		}
	}
	if out.Snapshot.ObjectCount != nil {
		if total, ok := out.Snapshot.ObjectCount["total_objects"]; ok {
			md["total_objects"] = total
		}
		if kinds, ok := out.Snapshot.ObjectCount["total_kinds"]; ok {
			md["total_kinds"] = kinds
		}
	}
	retOver := retentionOverKinds(out.Snapshot.Retention)
	md["retention_over_kinds"] = retOver
	md["work_items_total"] = len(out.WorkItems)
	md["work_items_by_priority"] = countWorkItemsByPriority(out.WorkItems)
	return md
}

func persistImprovementSnapshotAuditEvent(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	out ImprovementReportOutput,
	logger logging.Logger,
) {
	secCtx := pkgctx.NewSystemSecurityContext()
	options := &storage.AuditEventOptions{
		// Use existing allowlisted type and put the report semantics in operation/metadata.
		EventType:  "system_config_change",
		Operation:  "improvement_report_snapshot_persisted",
		Severity:   "low",
		TargetKind: objects.KindImprovementReport,
		TargetID:   "system-improvement-report",
		Metadata:   compactSnapshotMetadata(out),
	}
	if err := storage.CreateAuditEventWithBuilder(ctx, projectRoot, secCtx, storageProvider, options); err != nil {
		logging.Fluent(logger).Debug("Failed to persist improvement snapshot audit event (best-effort)").WithError(err).Log()
		return
	}
	logging.Fluent(logger).Debug("Persisted compact improvement snapshot to audit_event history").Log()
}

func loadPreviousImprovementReport(reportDir string) *ImprovementReportOutput {
	entries, err := fileutil.ReadDir(reportDir)
	if err != nil {
		return nil
	}
	var candidates []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), "improvement-report-") && strings.HasSuffix(e.Name(), ".json") {
			candidates = append(candidates, e.Name())
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Sort(sort.Reverse(sort.StringSlice(candidates)))
	path := filepath.Join(reportDir, candidates[0])
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return nil
	}
	var prev ImprovementReportOutput
	if json.Unmarshal(data, &prev) != nil {
		return nil
	}
	return &prev
}

// getTotalObjectsFromCountMap returns total_objects from an object count map (from in-process or JSON).
// Handles both int (in-process) and float64 (JSON unmarshal). Returns -1 when missing or invalid.
func getTotalObjectsFromCountMap(m map[string]any) int {
	if m == nil {
		return -1
	}
	v, ok := m["total_objects"]
	if !ok {
		return -1
	}
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return -1
	}
}

func buildComparative(previous *ImprovementReportOutput, current ImprovementReportSnapshot) ImprovementReportComparative {
	var comp ImprovementReportComparative
	if previous == nil {
		return comp
	}
	comp.PreviousAt = previous.GeneratedAt

	curCount := getTotalObjectsFromCountMap(current.ObjectCount)
	if curCount >= 0 && previous.Snapshot.ObjectCount != nil {
		prevCount := getTotalObjectsFromCountMap(previous.Snapshot.ObjectCount)
		if prevCount >= 0 {
			comp.ObjectCountDelta = curCount - prevCount
		}
	}

	curOver := retentionOverKinds(current.Retention)
	comp.RetentionOver = curOver
	if previous.Snapshot.Retention != nil {
		prevOver := retentionOverKinds(previous.Snapshot.Retention)
		comp.RetentionOverDelta = len(curOver) - len(prevOver)
	}

	curBlocking := 0
	if b, ok := current.HealthData["check_blocking"].(int); ok {
		curBlocking = b
	}
	prevBlocking := 0
	if previous.Snapshot.HealthData != nil {
		if b, ok := previous.Snapshot.HealthData["check_blocking"].(int); ok {
			prevBlocking = b
		}
	}
	comp.BlockingDelta = curBlocking - prevBlocking
	comp.NewBlocking = curBlocking > 0 && curBlocking > prevBlocking
	return comp
}

func retentionOverKinds(ret map[string]any) []string {
	over, _ := ret["over_target"].([]any)
	if over == nil {
		return nil
	}
	var kinds []string
	for _, v := range over {
		m, _ := v.(map[string]any)
		if m == nil {
			continue
		}
		if k, ok := m[objects.FieldKeyKind].(string); ok {
			kinds = append(kinds, k)
		}
	}
	return kinds
}

func prioritizeWorkItems(snap ImprovementReportSnapshot, comp ImprovementReportComparative, previous *ImprovementReportOutput) []ImprovementWorkItem {
	var items []ImprovementWorkItem

	// P0: Blocking issues
	if b, ok := snap.HealthData["check_blocking"].(int); ok && b > 0 {
		items = append(items, ImprovementWorkItem{
			Priority:        "P0",
			Category:        "quality",
			Title:           "Resolve blocking validation issues",
			Description:     fmt.Sprintf("%d blocking issue(s) from last check.", b),
			SuggestedAction: "Run 'zqk system check all' and fix Tier-1 issues; then 'zqk system check all --auto-fix' for auto-fixable.",
			Metric:          "blocking_issues",
			Value:           fmt.Sprintf("%d", b),
		})
	}

	// P1: Retention over target
	if len(comp.RetentionOver) > 0 {
		items = append(items, ImprovementWorkItem{
			Priority:        "P1",
			Category:        "performance",
			Title:           "Reduce internal object count over retention targets",
			Description:     fmt.Sprintf("%d kind(s) over target: %s.", len(comp.RetentionOver), strings.Join(comp.RetentionOver, ", ")),
			SuggestedAction: "Run 'zqk system aggregate-audit --window 7d --delete' then 'zqk system retention-tolerance'. Ensure retention_tolerance scheduler job is enabled.",
			Metric:          "retention_over_target",
			Value:           strings.Join(comp.RetentionOver, ";"),
		})
	}

	// P1: High failure or timeout rate (from metrics)
	if snap.MetricsAnalysis != nil && (snap.MetricsAnalysis.HighFailureCount > 0 || snap.MetricsAnalysis.TimeoutCount > 0) {
		desc := ""
		if snap.MetricsAnalysis.HighFailureCount > 0 {
			desc += fmt.Sprintf("%d command(s) with high failure rate. ", snap.MetricsAnalysis.HighFailureCount)
		}
		if snap.MetricsAnalysis.TimeoutCount > 0 {
			desc += fmt.Sprintf("%d command(s) with frequent timeouts. ", snap.MetricsAnalysis.TimeoutCount)
		}
		items = append(items, ImprovementWorkItem{
			Priority:        "P1",
			Category:        "behavior",
			Title:           "Address high-failure or timeout-prone commands",
			Description:     strings.TrimSpace(desc),
			SuggestedAction: "Run 'zqk system audit-report --format json' to list commands; fix or extend timeouts.",
			Metric:          "metrics_issues",
			Value:           fmt.Sprintf("failure=%d timeout=%d", snap.MetricsAnalysis.HighFailureCount, snap.MetricsAnalysis.TimeoutCount),
		})
	}

	// P2: Warnings
	if w, ok := snap.HealthData["check_warnings"].(int); ok && w > 0 {
		items = append(items, ImprovementWorkItem{
			Priority:        "P2",
			Category:        "quality",
			Title:           "Reduce validation warnings",
			Description:     fmt.Sprintf("%d warning(s) from last check.", w),
			SuggestedAction: "Run 'zqk system check all' and address Tier-2 issues.",
			Metric:          "warnings",
			Value:           fmt.Sprintf("%d", w),
		})
	}

	// P2: Slow commands
	if snap.MetricsAnalysis != nil && snap.MetricsAnalysis.SlowCount > 0 {
		items = append(items, ImprovementWorkItem{
			Priority:        "P2",
			Category:        "performance",
			Title:           "Optimize slow commands",
			Description:     fmt.Sprintf("%d command(s) flagged as slow.", snap.MetricsAnalysis.SlowCount),
			SuggestedAction: "Run 'zqk system audit-report' for details; consider caching or batching.",
			Metric:          "slow_commands",
			Value:           fmt.Sprintf("%d", snap.MetricsAnalysis.SlowCount),
		})
	}

	// P2: Object count growth (comparative)
	if comp.ObjectCountDelta > 100 && previous != nil {
		items = append(items, ImprovementWorkItem{
			Priority:        "P2",
			Category:        "accuracy",
			Title:           "Review object count growth",
			Description:     fmt.Sprintf("Object count increased by %d since last report.", comp.ObjectCountDelta),
			SuggestedAction: "Run 'zqk system object-count-report' to confirm congruence; run retention if internal kinds grew.",
			Metric:          "object_count_delta",
			Value:           fmt.Sprintf("%d", comp.ObjectCountDelta),
		})
	}

	// P3: Churn indicators
	if snap.MetricsAnalysis != nil && snap.MetricsAnalysis.ChurnCount > 0 {
		items = append(items, ImprovementWorkItem{
			Priority:        "P3",
			Category:        "behavior",
			Title:           "Reduce CLI churn and user confusion",
			Description:     fmt.Sprintf("%d churn indicator(s) in command metrics.", snap.MetricsAnalysis.ChurnCount),
			SuggestedAction: "Run 'zqk system audit-report' and improve docs or UX for frequently failed command patterns.",
			Metric:          "churn_indicators",
			Value:           fmt.Sprintf("%d", snap.MetricsAnalysis.ChurnCount),
		})
	}

	// P3: Quarantine
	if q, ok := snap.HealthData["quarantine_total_files"].(int); ok && q > 0 {
		items = append(items, ImprovementWorkItem{
			Priority:        "P3",
			Category:        "quality",
			Title:           "Review quarantine folder",
			Description:     fmt.Sprintf("%d file(s) in quarantine.", q),
			SuggestedAction: "Run 'zqk system quarantine-report' and 'zqk system cleanup-quarantine' if appropriate.",
			Metric:          "quarantine_files",
			Value:           fmt.Sprintf("%d", q),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		order := map[string]int{"P0": 0, "P1": 1, "P2": 2, "P3": 3}
		return order[items[i].Priority] < order[items[j].Priority]
	})
	return items
}

func buildSummary(snap ImprovementReportSnapshot, comp ImprovementReportComparative, workItems []ImprovementWorkItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "System improvement report at %s.\n", snap.GeneratedAt)
	if comp.PreviousAt != emptyValue {
		fmt.Fprintf(&b, "Comparative vs %s: object_count_delta=%d, retention_over_kinds=%d, blocking_delta=%d.\n",
			comp.PreviousAt, comp.ObjectCountDelta, len(comp.RetentionOver), comp.BlockingDelta)
	}
	fmt.Fprintf(&b, "Prioritized work items: %d total (P0=%d P1=%d P2=%d P3=%d).\n",
		len(workItems), countByP(workItems, "P0"), countByP(workItems, "P1"), countByP(workItems, "P2"), countByP(workItems, "P3"))
	return strings.TrimSpace(b.String())
}

func countByP(items []ImprovementWorkItem, p string) int {
	n := 0
	for _, w := range items {
		if w.Priority == p {
			n++
		}
	}
	return n
}

func formatImprovementReportTxt(out ImprovementReportOutput) []byte {
	var b strings.Builder
	b.WriteString("=== System Improvement Report ===\n\n")
	b.WriteString(out.Summary + "\n\n")
	b.WriteString("--- Prioritized Work Items ---\n\n")
	for _, w := range out.WorkItems {
		fmt.Fprintf(&b, "[%s] %s (%s)\n", w.Priority, w.Title, w.Category)
		if w.Description != emptyValue {
			b.WriteString("  " + w.Description + "\n")
		}
		if w.SuggestedAction != emptyValue {
			b.WriteString("  Action: " + w.SuggestedAction + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("--- Snapshot summary ---\n")
	if out.Snapshot.ObjectCount != nil {
		if t, ok := out.Snapshot.ObjectCount["total_objects"].(float64); ok {
			fmt.Fprintf(&b, "  Total objects: %.0f\n", t)
		}
	}
	if out.Snapshot.Retention != nil {
		if at, ok := out.Snapshot.Retention["at_or_under_target"].(bool); ok {
			fmt.Fprintf(&b, "  Retention at/under target: %v\n", at)
		}
	}
	if out.Comparative.PreviousAt != emptyValue {
		fmt.Fprintf(&b, "  Previous report: %s\n", out.Comparative.PreviousAt)
		fmt.Fprintf(&b, "  Object count delta: %d\n", out.Comparative.ObjectCountDelta)
	}
	return []byte(b.String())
}

func emitImprovementReportEvent(cmd *cobra.Command, projectRoot string, out ImprovementReportOutput, logger logging.Logger) {
	// Best-effort: log so dashboards or log aggregators can pick up completion.
	logging.Fluent(logger).Info("Improvement report completed").
		Int("work_items_count", len(out.WorkItems)).
		String("summary", out.Summary).
		Log()
}

// Well-known job IDs used when triggering remediation from report work items (must match scheduler jobs in scripts/scheduler_jobs/).
const (
	autofixRunJobID = "SCH-autofix-run" // runs system check --auto-fix every 5 min; trigger once so issues get addressed soon
)

// triggerRemediationFromWorkItems enqueues scheduler job trigger requests so the daemon runs remediation
// (autofix, maintenance/retention) in response to report outcomes. Without this, reports only document issues.
func triggerRemediationFromWorkItems(ctx context.Context, storageProvider storage.ObjectStorageProvider, projectRoot string, workItems []ImprovementWorkItem, logger logging.Logger) {
	if projectRoot == emptyValue || len(workItems) == 0 {
		return
	}
	var toTrigger []string
	for _, w := range workItems {
		if w.Priority == "P1" || w.Priority == "P3" {
			if storageProvider != nil {
				ensureWorkItemBLI(ctx, storageProvider, w, logger)
			}
		}
		switch w.Metric {
		case "blocking_issues":
			toTrigger = append(toTrigger, autofixRunJobID)
		case "retention_over_target":
		default:
			continue
		}
	}
	if len(toTrigger) == 0 {
		return
	}
	// Dedupe so we enqueue each job at most once
	seen := make(map[string]bool)
	unique := make([]string, 0, len(toTrigger))
	for _, id := range toTrigger {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	queue := scheduler.NewJobTriggerQueue(projectRoot)
	if err := queue.EnqueueTriggerRequests(unique, ""); err != nil {
		logging.Fluent(logger).Warn("Failed to enqueue remediation triggers (scheduler may still run on its schedule)").
			WithError(err).
			String("job_ids", strings.Join(unique, ",")).
			Log()
		return
	}
	logging.Fluent(logger).Info("Enqueued remediation triggers from improvement report so scheduler addresses work items").
		String("job_ids", strings.Join(unique, ",")).
		Log()
}

func ensureWorkItemBLI(_ context.Context, _ storage.ObjectStorageProvider, item ImprovementWorkItem, logger logging.Logger) {
	// Do not Create. ensureCreateLifecycleStatus coerces planned → exploring (no
	// --promote), so the object parks on the draft plane. List omits drafts, so the
	// hourly job remints the same title every run. Autofix still fires via the
	// trigger queue above. TRACK: BLI-CAS-HAND-DUP-CHECK-001
	if logger != nil {
		logging.Fluent(logger).Info("skipping improvement-report BLI mint (would park on object draft plane)").
			String("title", item.Title).
			String("metric", item.Metric).
			Log()
	}
}
