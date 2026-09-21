package whatsnext

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/git"
	"github.com/zqk-os/zqk/pkg/interactionpolicy"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/resourcehygiene"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// DraftPlaneStewardWarn matches system-check draft-plane backlog warning (≥50).
const DraftPlaneStewardWarn = 50

const stewardProjectionPrefix = "KERNEL STEWARD:"

// MaxEmployedWorkflowHints caps ambient workflow rows on the whats-next hot path.
const MaxEmployedWorkflowHints = 8

// MaxEmployedWorkflowSummaryLen truncates workflow description for ambient JSON.
const MaxEmployedWorkflowSummaryLen = 160

// Seat operating modes for whats-next (POL-AGENT-TPM-PROCESS-ADMIN-001).
const (
	SeatModeTPMProcessAdmin = "tpm_process_admin"
	SeatModePeerExecution   = "peer_execution"
)

// AlignLatestRelativePath is where TPM cadence persists `zqk system align --format json`.
const AlignLatestRelativePath = "state/ambient/align-latest.json"

// KernelAmbience is the cheap thought-projector slice for whats-next:
// last compact system-check cache + live draft-plane inventory + employed workflows.
// Does not run system check (too heavy for the composite hot path).
// TRACK: ambient verification on whats-next.
type KernelAmbience struct {
	Available          bool                        `json:"available"`
	BlockingIssues     int                         `json:"blocking_issues"`
	Warnings           int                         `json:"warnings"`
	TotalIssues        int                         `json:"total_issues"`
	TotalObjects       int                         `json:"total_objects,omitempty"`
	ObjectComplianceOK bool                        `json:"object_compliance_ok"`
	GhostRefCount      int                         `json:"ghost_ref_count"`
	MeasuredAt         string                      `json:"measured_at,omitempty"`
	SourcePath         string                      `json:"source_path,omitempty"`
	DraftPlaneTotal    int                         `json:"draft_plane_total"`
	DraftPlaneByKind   map[string]int              `json:"draft_plane_by_kind,omitempty"`
	EmployedWorkflows  []EmployedWorkflowHint      `json:"employed_workflows,omitempty"`
	StrategicAlignment *StrategicAlignmentSnapshot `json:"strategic_alignment,omitempty"`
	SeatMode           string                      `json:"seat_mode,omitempty"`
	StewardFocus       string                      `json:"steward_focus,omitempty"`
	// BranchAhead is local commits not on the tracked upstream (TPM push-ahead gland).
	BranchAhead       int                    `json:"branch_ahead,omitempty"`
	Note              string                 `json:"note,omitempty"`
	TopIssueClusters  []IssueCluster         `json:"top_issue_clusters,omitempty"`
	MetricsRollup     *MetricsRollupSnapshot `json:"metrics_rollup,omitempty"`
	StaleTestCatalyst *StaleTestCatalyst     `json:"stale_test_catalyst,omitempty"`
	StaleAgentTask    *StaleAgentTask        `json:"stale_agent_task,omitempty"`
}

// StaleAgentTask holds information and command hints for stalled in-progress agent tasks.
type StaleAgentTask struct {
	TaskID      string `json:"task_id"`
	PlanID      string `json:"plan_id,omitempty"`
	ClaimedBy   string `json:"claimed_by,omitempty"`
	CommandHint string `json:"command_hint"`
}

// StaleTestCatalyst holds single-click test execution hints for stalled in-progress items.
type StaleTestCatalyst struct {
	BacklogItemID string `json:"backlog_item_id"`
	TestCaseID    string `json:"test_case_id"`
	CommandHint   string `json:"command_hint"`
}

// MetricsRollupSnapshot contains the aggregated metrics from ambient wave.
type MetricsRollupSnapshot struct {
	CommandErrors           int                                  `json:"command_errors"`
	SchedulerStuckCount     int                                  `json:"scheduler_stuck_count"`
	AuditEventsCount        int                                  `json:"audit_events_count"`
	GhostRefCount           int                                  `json:"ghost_ref_count"`
	SystemCheckIssues       int                                  `json:"system_check_issues"`
	NextAdminAction         string                               `json:"next_admin_action"`
	RankedActions           []string                             `json:"ranked_actions"`
	MeasuredAt              string                               `json:"measured_at"`
	HighFailureRateCommands int                                  `json:"high_failure_rate_commands,omitempty"`
	SlowCommands            int                                  `json:"slow_commands,omitempty"`
	FrequentTimeouts        int                                  `json:"frequent_timeouts,omitempty"`
	ChurnIndicators         int                                  `json:"churn_indicators,omitempty"`
	TestBundleEvidence      string                               `json:"test_bundle_evidence,omitempty"`
	TopErrorClusters        []IssueCluster                       `json:"top_error_clusters,omitempty"`
	TopWarnClusters         []IssueCluster                       `json:"top_warn_clusters,omitempty"`
	IOResourceTelemetry     *resourcehygiene.IOResourceTelemetry `json:"io_resource_telemetry,omitempty"`
}

// IssueCluster represents a rollup of similar system-check issues.
type IssueCluster struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

// StrategicAlignmentSnapshot is a cached `zqk system align` result (not a live full scan).
type StrategicAlignmentSnapshot struct {
	Available         bool    `json:"available"`
	AlignmentScore    float64 `json:"alignment_score,omitempty"`
	GoalsCount        int     `json:"goals_count,omitempty"`
	ItemsWithGoals    int     `json:"items_with_goals,omitempty"`
	ItemsWithoutGoals int     `json:"items_without_goals,omitempty"`
	SourcePath        string  `json:"source_path,omitempty"`
	MeasuredAt        string  `json:"measured_at,omitempty"`
	Note              string  `json:"note,omitempty"`
}

// EmployedWorkflowHint keeps active/plan-linked workflows fresh in agent context.
type EmployedWorkflowHint struct {
	ID      string `json:"id"`
	Title   string `json:"title,omitempty"`
	Summary string `json:"summary,omitempty"`
	Source  string `json:"source,omitempty"` // plan_ref | related | active
}

type checkSummaryFile struct {
	Summary struct {
		TotalObjects          int `json:"total_objects"`
		TotalIssues           int `json:"total_issues"`
		BlockingIssues        int `json:"blocking_issues"`
		Warnings              int `json:"warnings"`
		GhostRefCount         int `json:"ghost_ref_count"`
		PendingAutofixBatches int `json:"pending_autofix_batches"`
	} `json:"summary"`
	ResultsByKind map[string][]struct {
		ID     string `json:"id"`
		Path   string `json:"path"`
		Issues []struct {
			Tier     int    `json:"tier"`
			Category string `json:"category"`
			Message  string `json:"message"`
		} `json:"issues,omitempty"`
	} `json:"results_by_kind,omitempty"`
}

func preferredCheckSummaryPaths(projectRoot string) []string {
	return []string{
		filepath.Join(projectRoot, paths.ProjectDataDir, paths.PreCommitDir, "system-check.json"),
		filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, "system-check-autofix-dangling.json"),
		filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, "system-check.json"),
	}
}

// LoadKernelAmbience reads cached system-check summary + live draft-plane counts.
func LoadKernelAmbience(projectRoot string) KernelAmbience {
	inv := storage.InventoryObjectDraftPlane(projectRoot)
	amb := KernelAmbience{
		Available:        false,
		DraftPlaneTotal:  inv.Total,
		DraftPlaneByKind: inv.ByKind,
		Note:             paths.RewriteCanonicalCLIInvocations("No compact system-check JSON found; run zqk system check --format json -o .zqk/logs/system-check.json"),
	}

	var (
		src  string
		file checkSummaryFile
		fi   fileutil.FileInfo
	)
	for _, p := range preferredCheckSummaryPaths(projectRoot) {
		st, err := fileutil.Stat(p)
		if err != nil || st.IsDir() {
			continue
		}
		b, err := fileutil.ReadFile(p)
		if err != nil {
			continue
		}
		var parsed checkSummaryFile
		if err := json.Unmarshal(b, &parsed); err != nil {
			continue
		}
		if parsed.Summary.TotalObjects == 0 {
			continue
		}
		if fi == nil || st.ModTime().After(fi.ModTime()) {
			src = p
			file = parsed
			fi = st
		}
	}
	if src != "" {
		s := file.Summary
		measuredAt := ""
		if fi != nil {
			measuredAt = fi.ModTime().UTC().Format(time.RFC3339)
		}
		ok := s.BlockingIssues == 0 && s.PendingAutofixBatches == 0
		amb.Available = true
		amb.BlockingIssues = s.BlockingIssues
		amb.Warnings = s.Warnings
		amb.TotalIssues = s.TotalIssues
		amb.TotalObjects = s.TotalObjects
		amb.ObjectComplianceOK = ok
		amb.GhostRefCount = s.GhostRefCount
		amb.MeasuredAt = measuredAt
		amb.SourcePath = src
		amb.Note = "Instance validation from last system-check cache + live draft-plane walk — not a fresh full check"

		// Calculate top issue clusters
		clusterMap := make(map[string]int)
		for _, items := range file.ResultsByKind {
			for _, item := range items {
				for _, issue := range item.Issues {
					msg := strings.TrimSpace(issue.Message)
					if msg != "" {
						clusterMap[msg]++
					}
				}
			}
		}
		var clusters []IssueCluster
		for msg, count := range clusterMap {
			clusters = append(clusters, IssueCluster{Message: msg, Count: count})
		}
		sort.Slice(clusters, func(i, j int) bool {
			return clusters[i].Count > clusters[j].Count
		})
		if len(clusters) > 3 {
			clusters = clusters[:3]
		}
		amb.TopIssueClusters = clusters
	}

	EnrichStrategicAlignment(&amb, projectRoot)
	EnrichMetricsRollup(&amb, projectRoot)
	if n := git.NewFacade(projectRoot).CountAheadUpstream(); n > 0 {
		amb.BranchAhead = n
	}
	amb.StewardFocus = ProjectStewardFocus(amb, "")
	return amb
}

// EnrichMetricsRollup loads the metrics rollup from state/ambient/metrics-rollup.json
func EnrichMetricsRollup(amb *KernelAmbience, projectRoot string) {
	if amb == nil {
		return
	}
	path := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, "ambient", "metrics-rollup.json")
	if b, err := fileutil.ReadFile(path); err == nil {
		var rollup MetricsRollupSnapshot
		if err := json.Unmarshal(b, &rollup); err == nil {
			amb.MetricsRollup = &rollup
		}
	}
}

// EnrichStrategicAlignment loads TPM-persisted align JSON (cheap; no live BLI walk).
func EnrichStrategicAlignment(amb *KernelAmbience, projectRoot string) {
	if amb == nil {
		return
	}
	path := filepath.Join(projectRoot, paths.ProjectDataDir, AlignLatestRelativePath)
	st, err := fileutil.Stat(path)
	if err != nil || st.IsDir() {
		amb.StrategicAlignment = &StrategicAlignmentSnapshot{
			Available: false,
			Note:      paths.RewriteCanonicalCLIInvocations("no align cache — TPM: zqk system align --format json -o .zqk/state/ambient/align-latest.json"),
		}
		return
	}
	b, err := fileutil.ReadFile(path)
	if err != nil {
		amb.StrategicAlignment = &StrategicAlignmentSnapshot{Available: false, Note: "align cache unreadable"}
		return
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		amb.StrategicAlignment = &StrategicAlignmentSnapshot{Available: false, Note: "align cache JSON invalid"}
		return
	}
	snap := &StrategicAlignmentSnapshot{
		Available:  true,
		SourcePath: path,
		MeasuredAt: st.ModTime().UTC().Format(time.RFC3339),
		Note:       paths.RewriteCanonicalCLIInvocations("from zqk system align cache — peers trust whats-next priority_plan; TPM refreshes align + active_order"),
	}
	snap.AlignmentScore = firstFloat(raw, "overall_alignment_score", "alignment_score")
	if snap.AlignmentScore == 0 {
		if nested, ok := raw["alignment"].(map[string]any); ok {
			snap.AlignmentScore = firstFloat(nested, "alignment_score", "overall_alignment_score")
		}
	}
	snap.GoalsCount = firstInt(raw, "goals_count")
	if gw, ok := raw["goal_work"].(map[string]any); ok {
		snap.ItemsWithGoals = firstInt(gw, "items_with_goals")
		snap.ItemsWithoutGoals = firstInt(gw, "items_without_goals")
	} else if nested, ok := raw["alignment"].(map[string]any); ok {
		if gw, ok := nested["goal_work"].(map[string]any); ok {
			snap.ItemsWithGoals = firstInt(gw, "items_with_goals")
			snap.ItemsWithoutGoals = firstInt(gw, "items_without_goals")
		}
	}
	amb.StrategicAlignment = snap
}

// IsTPMProcessAdminSeat reports seats that must not absorb peer whats-next execution.
func IsTPMProcessAdminSeat(agentID string) bool {
	return IsTPMProcessAdminSeatIn("", agentID)
}

// IsTPMProcessAdminSeatIn uses seating duty/wake when projectRoot is set.
// Without seating, only peer-tpm-* / tpm tokens qualify — not vendor product names.
func IsTPMProcessAdminSeatIn(projectRoot, agentID string) bool {
	id := strings.ToLower(strings.TrimSpace(agentID))
	if id == "" {
		return false
	}
	if strings.HasPrefix(id, "peer-tpm-") || id == "tpm" || strings.HasSuffix(id, "-tpm") {
		return true
	}
	if strings.TrimSpace(projectRoot) == "" {
		return false
	}
	f, err := agentfeed.LoadPeerSeats(projectRoot)
	if err != nil {
		return false
	}
	rec, ok := f.Seats[strings.TrimSpace(agentID)]
	if !ok {
		return false
	}
	if agentfeed.NormalizeSeatDuty(rec.Duty) == agentfeed.SeatKindCoordinator {
		return true
	}
	return agentfeed.NormalizeWakeMembrane(rec.Wake) == agentfeed.WakeMembraneMCP
}

// ApplySeatOperatingMode sets seat_mode and reprojects steward focus for TPM vs peer.
func ApplySeatOperatingMode(amb *KernelAmbience, agentID, correspondenceHint string) {
	ApplySeatOperatingModeIn(amb, "", agentID, correspondenceHint)
}

// ApplySeatOperatingModeIn is seating-aware when projectRoot is set.
func ApplySeatOperatingModeIn(amb *KernelAmbience, projectRoot, agentID, correspondenceHint string) {
	if amb == nil {
		return
	}
	if IsTPMProcessAdminSeatIn(projectRoot, agentID) {
		amb.SeatMode = SeatModeTPMProcessAdmin
	} else {
		amb.SeatMode = SeatModePeerExecution
	}
	amb.StewardFocus = ProjectStewardFocus(*amb, correspondenceHint)
}

// EnrichKernelAmbienceWithWorkflows attaches active/plan-linked workflows so agents
// keep employed WFL contracts in context (not only health metrics).
func EnrichKernelAmbienceWithWorkflows(ctx context.Context, sp storage.ObjectStorageProvider, amb *KernelAmbience, primaryPlanID string, activePlanIDs []string) {
	if amb == nil || sp == nil {
		return
	}
	hints := LoadEmployedWorkflowHints(ctx, sp, primaryPlanID, activePlanIDs)
	amb.EmployedWorkflows = hints
	amb.StewardFocus = ProjectStewardFocus(*amb, "")
	if len(hints) > 0 && amb.Available {
		amb.Note = strings.TrimSpace(amb.Note + "; employed_workflows keep active WFL contracts fresh in context")
	} else if len(hints) > 0 && amb.Note == "" {
		amb.Note = "employed_workflows from active workflow objects + plan refs"
	}
}

// EnrichKernelAmbienceWithStaleTasks scans for stale in_progress agent tasks and enriches ambience.
func EnrichKernelAmbienceWithStaleTasks(ctx context.Context, sp storage.ObjectStorageProvider, amb *KernelAmbience, now time.Time) {
	if amb == nil || sp == nil {
		return
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	res, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind: objects.KindAgentTask,
		Filters: map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		},
	})
	if err != nil || len(res.Objects) == 0 {
		return
	}

	staleThreshold := 2 * time.Hour
	staleItems := interactionpolicy.DetectStaleAgentTasks(res.Objects, now, staleThreshold)
	if len(staleItems) > 0 {
		topStale := staleItems[0]
		planRef := ""
		claimedBy := ""
		for _, obj := range res.Objects {
			if id, _ := obj[objects.FieldKeyID].(string); id == topStale.TaskID {
				planRef, _ = obj[objects.FieldKeyPriorityPlanRef].(string)
				claimedBy, _ = obj[objects.FieldKeyClaimedBy].(string)
				break
			}
		}
		cmdHint := paths.CLIUsage("agent", "recover")
		if planRef != "" {
			cmdHint = paths.CLIUsage("agent", "recover", planRef)
		}
		amb.StaleAgentTask = &StaleAgentTask{
			TaskID:      topStale.TaskID,
			PlanID:      planRef,
			ClaimedBy:   claimedBy,
			CommandHint: cmdHint,
		}
		if amb.Note != "" {
			amb.Note = fmt.Sprintf("%s; %d stale in_progress agent_task(s) detected", amb.Note, len(staleItems))
		} else {
			amb.Note = fmt.Sprintf("%d stale in_progress agent_task(s) detected", len(staleItems))
		}
	}
}

func firstFloat(m map[string]any, keys ...string) float64 {
	for _, k := range keys {
		switch v := m[k].(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		case int64:
			return float64(v)
		case json.Number:
			f, _ := v.Float64()
			return f
		}
	}
	return 0
}

func firstInt(m map[string]any, keys ...string) int {
	for _, k := range keys {
		switch v := m[k].(type) {
		case float64:
			return int(v)
		case int:
			return v
		case int64:
			return int(v)
		case json.Number:
			i, _ := v.Int64()
			return int(i)
		}
	}
	return 0
}

// LoadEmployedWorkflowHints lists active workflows, preferring those linked from the
// current / active priority plans (workflow_ref + related_object_refs).
func LoadEmployedWorkflowHints(ctx context.Context, sp storage.ObjectStorageProvider, primaryPlanID string, activePlanIDs []string) []EmployedWorkflowHint {
	if sp == nil {
		return nil
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	preferred := map[string]string{} // id -> source
	collectPlanWorkflowRefs := func(planID string) {
		planID = strings.TrimSpace(planID)
		if planID == "" {
			return
		}
		obj, err := sp.Read(ctx, secCtx, planID)
		if err != nil || obj == nil {
			return
		}
		if ref, _ := obj[objects.FieldKeyWorkflowRef].(string); strings.HasPrefix(strings.TrimSpace(ref), "WFL-") {
			preferred[strings.TrimSpace(ref)] = "plan_ref"
		}
		for _, ref := range stringSliceField(obj[objects.FieldKeyRelatedObjectRefs]) {
			if strings.HasPrefix(ref, "WFL-") {
				if _, ok := preferred[ref]; !ok {
					preferred[ref] = "related"
				}
			}
		}
	}
	collectPlanWorkflowRefs(primaryPlanID)
	for _, id := range activePlanIDs {
		if id != primaryPlanID {
			collectPlanWorkflowRefs(id)
		}
	}

	res, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindWorkflow})
	if err != nil || len(res.Objects) == 0 {
		return nil
	}

	type cand struct {
		hint  EmployedWorkflowHint
		rank  int // lower = better
		title string
	}
	var cands []cand
	for _, obj := range res.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		id = strings.TrimSpace(id)
		if id == "" || isFixtureWorkflow(obj, id) {
			continue
		}
		st, _ := obj[objects.FieldKeyStatus].(string)
		if !strings.EqualFold(strings.TrimSpace(st), "active") {
			continue
		}
		title, _ := obj[objects.FieldKeyTitle].(string)
		if title == "" {
			title, _ = obj[objects.FieldKeyName].(string)
		}
		summary := truncateRunes(firstNonEmptyLine(fmt.Sprint(obj[objects.FieldKeyDescription])), MaxEmployedWorkflowSummaryLen)
		src := "active"
		rank := 100
		if s, ok := preferred[id]; ok {
			src = s
			if s == "plan_ref" {
				rank = 0
			} else {
				rank = 10
			}
		} else if isCoreOperationalWorkflow(id) {
			rank = 20
		}
		cands = append(cands, cand{
			hint:  EmployedWorkflowHint{ID: id, Title: strings.TrimSpace(title), Summary: summary, Source: src},
			rank:  rank,
			title: strings.TrimSpace(title),
		})
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].rank != cands[j].rank {
			return cands[i].rank < cands[j].rank
		}
		return cands[i].hint.ID < cands[j].hint.ID
	})
	if len(cands) > MaxEmployedWorkflowHints {
		cands = cands[:MaxEmployedWorkflowHints]
	}
	out := make([]EmployedWorkflowHint, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.hint)
	}
	return out
}

func isFixtureWorkflow(obj map[string]any, id string) bool {
	if id == "WFL-001" || strings.HasSuffix(id, "-FIXTURE") {
		return true
	}
	title, _ := obj[objects.FieldKeyTitle].(string)
	return strings.Contains(strings.ToLower(title), "fixture")
}

func isCoreOperationalWorkflow(id string) bool {
	switch id {
	case "WFL-TPM-AGY-MESH-001",
		"WFL-MMORCH-OPERATIONAL-RUNBOOK",
		"WFL-MULTI-AGENT-WORK-CLAIM",
		"WFL-SUBAGENT-DISPATCH",
		"WFL-AGENT-BOOTSTRAP":
		return true
	default:
		return false
	}
}

func stringSliceField(v any) []string {
	switch t := v.(type) {
	case []string:
		out := make([]string, 0, len(t))
		for _, s := range t {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			s, _ := x.(string)
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func truncateRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max < 2 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

// ProjectStewardFocus is the TPM thought projector: what kernel health demands next.
// correspondenceHint is optional (ack_then_continue / await_peer_ack_keep_working).
func ProjectStewardFocus(amb KernelAmbience, correspondenceHint string) string {
	var parts []string
	switch amb.SeatMode {
	case SeatModeTPMProcessAdmin:
		parts = append(parts, "TPM seat: process administration only (POL-AGENT-TPM-PROCESS-ADMIN-001) — maintain active_order, shovel-ready columns, align cache; do NOT claim/orchestrate peer whats-next BLIs")
	case SeatModePeerExecution:
		parts = append(parts, "peer seat: execute the whats-next priority_plan the system proposes; claim BLIs; prepare-context + orchestrate/subagent scale-up (WFL-SUBAGENT-DISPATCH); hourglass on handoffs")
	}
	if amb.BranchAhead > 0 {
		parts = append(parts, fmt.Sprintf(
			"branch_ahead=%d — push seated integration/pri-* trunk before groom-ahead or align (POL-AGENT-TPM-PROCESS-ADMIN-001)",
			amb.BranchAhead,
		))
	}
	if amb.StrategicAlignment != nil {
		if amb.StrategicAlignment.Available {
			sa := interactionpolicy.StratplanAmbient{
				AlignOK:         true,
				AlignMeasuredAt: amb.StrategicAlignment.MeasuredAt,
				AlignScore:      amb.StrategicAlignment.AlignmentScore,
				GoalGaps:        amb.StrategicAlignment.ItemsWithoutGoals,
			}
			interactionpolicy.FinalizeAlignFreshness(&sa, time.Now().UTC())
			if sa.AlignFresh {
				parts = append(parts, fmt.Sprintf(
					"align score %.1f %s age=%s goal_gaps=%d — do not remint align; if the next grooming priority_plan is already shaped, hourglass the lead in_progress PRI (do not re-get it)",
					sa.AlignScore, "fresh", sa.AlignAge, sa.GoalGaps,
				))
			} else {
				parts = append(parts, paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("align score %.1f STALE age=%s gaps=%d — persist zqk system align --format json -o .zqk/state/ambient/align-latest.json; trust active_order only after refresh against mission/vision/strat", sa.AlignScore, sa.AlignAge, sa.GoalGaps)))
			}
		} else if note := strings.TrimSpace(amb.StrategicAlignment.Note); note != "" {
			parts = append(parts, note)
		}
	}
	if amb.Available && amb.BlockingIssues > 0 {
		parts = append(parts, fmt.Sprintf(
			"%d blocking system-check issues — triage those (false-complete PRIs, dangling refs) before wake/CAP theater",
			amb.BlockingIssues,
		))
	}
	if amb.GhostRefCount > 0 {
		parts = append(parts, paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("heal-dangling required: %d GhostRefs detected — run `zqk system check autofix dangling`", amb.GhostRefCount)))
	}
	if amb.DraftPlaneTotal >= DraftPlaneStewardWarn {
		switch amb.SeatMode {
		case SeatModePeerExecution:
			parts = append(parts, fmt.Sprintf(
				"draft plane %d (≥%d) — promote/fix sticks; NEVER draft sweep apply (need RBAC %s)",
				amb.DraftPlaneTotal, DraftPlaneStewardWarn, pkgctx.PermissionDeleteObjectDraftPlane,
			))
		default:
			parts = append(parts, fmt.Sprintf(
				"draft plane %d (≥%d) — classify; promote intentional; sweep apply only with RBAC %s (POL-AGENT-DRAFT-SWEEP-TPM-001)",
				amb.DraftPlaneTotal, DraftPlaneStewardWarn, pkgctx.PermissionDeleteObjectDraftPlane,
			))
		}
	} else if amb.DraftPlaneTotal > 0 {
		switch amb.SeatMode {
		case SeatModePeerExecution:
			parts = append(parts, fmt.Sprintf(
				"draft plane %d — promote intentional drafts; sweep apply forbidden without RBAC %s",
				amb.DraftPlaneTotal, pkgctx.PermissionDeleteObjectDraftPlane,
			))
		default:
			parts = append(parts, fmt.Sprintf(
				"draft plane %d — classify; promote intentional; sweep apply via RBAC %s (not seat-name strings)",
				amb.DraftPlaneTotal, pkgctx.PermissionDeleteObjectDraftPlane,
			))
		}
	}
	if !amb.Available {
		parts = append(parts, paths.RewriteCanonicalCLIInvocations("no system-check cache — run zqk system check --format json -o .zqk/logs/system-check.json before claiming kernel healthy"))
	}
	switch strings.TrimSpace(correspondenceHint) {
	case "ack_then_continue":
		parts = append(parts, "inbox unacked: feed ack as YOUR exact --agent-id (peer_seats.json), then whats-next --agent-id <you>; never echo nonces into IDE chat")
	case "await_peer_ack_keep_working":
		parts = append(parts, "outbox awaiting peer_ack: do not remint the same wake; keep working other kernel items; TPM must ack own inbox first")
	}
	if len(amb.TopIssueClusters) > 0 {
		var clusterStrs []string
		for _, c := range amb.TopIssueClusters {
			clusterStrs = append(clusterStrs, fmt.Sprintf("%dx %s", c.Count, c.Message))
		}
		parts = append(parts, fmt.Sprintf("top issues to triage: %s", strings.Join(clusterStrs, " | ")))
	}
	if n := len(amb.EmployedWorkflows); n > 0 {
		ids := make([]string, 0, n)
		for _, w := range amb.EmployedWorkflows {
			if w.ID != "" {
				ids = append(ids, w.ID)
			}
		}
		parts = append(parts, paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("employed workflows (%d): %s — keep contracts fresh via kernel_ambience.employed_workflows / zqk object get <WFL>", n, strings.Join(ids, ", "))))
	}
	if amb.MetricsRollup != nil {
		if amb.MetricsRollup.NextAdminAction != "" {
			parts = append(parts, fmt.Sprintf("metrics wave action: %s", amb.MetricsRollup.NextAdminAction))
		}
		if len(amb.MetricsRollup.TopErrorClusters) > 0 || len(amb.MetricsRollup.TopWarnClusters) > 0 {
			parts = append(parts, fmt.Sprintf(
				"human-log clusters: errors=%d warns=%d (ranked in metrics_rollup)",
				len(amb.MetricsRollup.TopErrorClusters),
				len(amb.MetricsRollup.TopWarnClusters),
			))
		}
	}
	if len(parts) == 0 {
		if amb.Available && amb.ObjectComplianceOK && amb.DraftPlaneTotal == 0 {
			return "kernel compliance cache clean + draft plane empty — advance active PRI BLIs to terminal with evidence"
		}
		return ""
	}
	return strings.Join(parts, "; ")
}

// PrependStewardProjection puts the thought projector ahead of ATK/onboarding prompts
// so assigned work cannot hide kernel health. Idempotent if already prefixed.
func PrependStewardProjection(instruction, focus string) string {
	return PrependStewardProjectionForSeat(instruction, focus, "")
}

// PrependStewardProjectionForSeat varies seat cues (TPM process admin vs peer execution).
func PrependStewardProjectionForSeat(instruction, focus, seatMode string) string {
	focus = strings.TrimSpace(focus)
	if focus == "" {
		return instruction
	}
	seatCue := paths.RewriteCanonicalCLIInvocations("Seat: always pass --agent-id matching peer_seats.json (opaque seat id, not a vendor nickname). " +
		"COMMS-CHECK / STEWARD: zqk feed ack --agent-id <you> --in-reply-to <AFE>; echo nonce via " +
		"zqk feed steer --to-agent-id <coordinator seat> --await-peer-ack (not IDE chat). " +
		"Workflows: honor kernel_ambience.employed_workflows (mesh/MMORCH/subagent/bootstrap) — do not drift from active WFL contracts.\n\n")
	switch seatMode {
	case SeatModeTPMProcessAdmin:
		seatCue = "TPM: process admin only — if align-latest.json is stale (>30m) persist a new align cache; if fresh, do not remint align. Shape an unshaped grooming priority_plan; if it is already shaped, hourglass the lead in_progress PRI and classify draft plane. " +
			"Set priority_plan.active_order to match mission/vision/strategic_plan; hourglass steer peers to pull whats-next — do not agent orchestrate their PRI. " + seatCue
	case SeatModePeerExecution:
		seatCue = paths.RewriteCanonicalCLIInvocations("Peer: run zqk workflow whats-next --agent-id <you> --format json; execute the proposed priority_plan/BLIs; ") +
			"prepare-context then orchestrate/subagent scale-up; agent next --on-validation-failure wake; " +
			"draft plane: promote/fix only — never draft sweep apply without RBAC delete:object_draft_plane. " + seatCue
	}
	block := stewardProjectionPrefix + " " + focus + "\n\n" + seatCue
	if strings.HasPrefix(strings.TrimSpace(instruction), stewardProjectionPrefix) {
		return instruction
	}
	if strings.TrimSpace(instruction) == "" {
		return strings.TrimSpace(block)
	}
	return block + instruction
}
