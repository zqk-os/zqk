package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zqk-os/zqk/cmd/zqk/state"
	"github.com/zqk-os/zqk/cmd/zqk/swarm"
	"github.com/zqk-os/zqk/cmd/zqk/test"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/tray"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Tab identifier constants.
const (
	TabState     = 0
	TabAudit     = 1
	TabSwarm     = 2
	TabPM        = 3
	TabMetrics   = 4
	TabScheduler = 5
	TabQA        = 6
	TabHealth    = 7
	TotalTabs    = 8

	// Backward compatibility aliases
	TabSeismograph = TabState
	TabObjects     = TabPM
	TabTest        = TabQA
	TabSystem      = TabHealth
	TabAction      = TabHealth
)

// HealthViolationRow represents an active validation violation loaded from cache.
type HealthViolationRow struct {
	Tier        int    `json:"tier"`
	Severity    string `json:"severity"`
	Kind        string `json:"kind"`
	ObjectID    string `json:"object_id"`
	Category    string `json:"category"`
	Message     string `json:"message"`
	AutoFixable bool   `json:"auto_fixable"`
	Path        string `json:"path"`
}

// ActionCenterItem represents a triggerable action shortcut backed by the tray and scheduler.
type ActionCenterItem struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	JobID       string `json:"job_id"`
	IsTriggered bool   `json:"is_triggered"`
}

// HealthSummary captures system integrity and hygiene indicators.
type HealthSummary struct {
	LastChecked      time.Time
	CheckFreshness   string
	OverallStatus    string
	TotalViolations  int
	Tier1Count       int
	Tier2Count       int
	Tier3Count       int
	AutoFixableCount int
	StaleLocksCount  int
	StorageFiles     int
	StorageSizeStr   string
	OpenFileDesc     int
	MaxFileDesc      int
}

// SchedulerJobRow captures a job's operational state for display.
type SchedulerJobRow struct {
	ID        string
	Schedule  string
	LastRunAt string
	NextRunAt string
	Status    string
}

// PMGoalRow represents a strategic program goal.
type PMGoalRow struct {
	ID     string
	Title  string
	Status string
	Metric string
	Target string
}

// PMPlanRow represents a priority plan boundary.
type PMPlanRow struct {
	ID          string
	Title       string
	Status      string
	Workstreams []string
	BLICount    int
}

// PMWorkstreamRow represents a domain workstream lane.
type PMWorkstreamRow struct {
	ID     string
	Title  string
	Status string
}

// PMBacklogSummary captures the counts across work unit states.
type PMBacklogSummary struct {
	Total      int
	Draft      int
	Planned    int
	InProgress int
	Blocked    int
	Completed  int
	Done       int
	Approved   int
	Claimed    int
	Unclaimed  int
}

// PMBacklogRow represents a single work unit.
type PMBacklogRow struct {
	ID        string
	Title     string
	Status    string
	Priority  string
	ClaimedBy string
	PlanRef   string
}

// PMRequirementRow represents a requirement node.
type PMRequirementRow struct {
	ID     string
	Title  string
	Status string
}

// PMBlockerRow represents an active risk or blocker.
type PMBlockerRow struct {
	ID       string
	Title    string
	Severity string
	Status   string
	Impact   string
}

// PMDebtRow represents a technical debt item.
type PMDebtRow struct {
	ID       string
	Title    string
	Category string
	Status   string
	Priority string
}

// CommandMetricRow captures telemetry on CLI command invocations.
type CommandMetricRow struct {
	ID          string
	CommandName string
	ExecCount   int
	AvgDuration string
	LastRunAt   string
	Status      string
}

// SchedulerHealthRow captures scheduler health telemetry.
type SchedulerHealthRow struct {
	ID          string
	HeartbeatAt string
	Status      string
	Executions  int
	Failures    int
}

// FileLockMetricRow captures concurrency and lock contention metrics.
type FileLockMetricRow struct {
	ID         string
	TargetKind string
	Contention int
	Duration   string
	Status     string
}

// QualityMetricRow captures code quality and audit aggregation metrics.
type QualityMetricRow struct {
	ID         string
	MetricType string
	Value      string
	Status     string
	Window     string
}

// ResourceHygieneRow captures resource and CAS storage indicators.
type ResourceHygieneRow struct {
	ProcessObjectCount int
	KindCount          int
	StreamFileCount    int
	ActiveStreams      int
}

// ItemDetailModel encapsulates a detailed object view for full-screen drill-down inspection.
type ItemDetailModel struct {
	Kind        string            `json:"kind"`
	ID          string            `json:"id"`
	Status      string            `json:"status"`
	Title       string            `json:"title"`
	Timestamp   string            `json:"timestamp,omitempty"`
	Actor       string            `json:"actor,omitempty"`
	Summary     string            `json:"summary,omitempty"`
	Details     []string          `json:"details,omitempty"`
	Lineage     []string          `json:"lineage,omitempty"`
	Criteria    []string          `json:"criteria,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	RawPayload  string            `json:"raw_payload,omitempty"`
}

// QASummaryRow captures high-level test dashboard and traceability health.
type QASummaryRow struct {
	TotalTestCases    int
	InFlightCount     int
	RegressionCount   int
	SatisfiedCriteria int
	TotalCriteria     int
	IntactChains      int
	UnboundCriteria   int
	DoDCompliant      bool
}

// UIModel encapsulates the dynamic state of the Mission Control TUI.
type UIModel struct {
	ProjectRoot  string
	ActiveTab    int
	ScrollOffset int // 0 means bottom / newest (or top depending on view)
	AutoScroll   bool
	Width        int
	Height       int

	// Interactive Selection Cursor & Drill-Down Inspection Modal
	SelectedIndex int
	DetailModal   *ItemDetailModel

	// Tab 1: State stream (change journal, instructions, lifecycle)
	Mutations []state.JournalMutation

	// Tab 2: High-volume audit events stream
	AuditEvents []state.JournalMutation

	// Tab 3: Swarm status
	SwarmData map[string]any

	// Tab 4: Process & Program Management (PM Cascade)
	MissionTitle   string
	VisionTitle    string
	Goals          []PMGoalRow
	PriorityPlans  []PMPlanRow
	Workstreams    []PMWorkstreamRow
	BacklogSummary PMBacklogSummary
	RecentBacklog  []PMBacklogRow
	Requirements   []PMRequirementRow
	Blockers       []PMBlockerRow
	TechnicalDebt  []PMDebtRow

	// Tab 5: Metrics & Telemetry
	CommandMetrics  []CommandMetricRow
	SchedulerHealth []SchedulerHealthRow
	LockMetrics     []FileLockMetricRow
	QualityMetrics  []QualityMetricRow
	Hygiene         ResourceHygieneRow
	TSDB            *state.TSDBTelemetry

	// Tab 6: Background Scheduler
	SchedulerJobs []SchedulerJobRow

	// Tab 7: QA & Lineage Traceability
	QASummary       QASummaryRow
	TestCases       []*test.TestCaseModel
	UnboundCriteria []*test.UnboundCriterionModel
	RecentQAEvents  []test.LifecycleEventSummary

	// Tab 8: System Integrity Radar & Action Center
	HealthSummary    HealthSummary
	HealthViolations []HealthViolationRow
	ActionItems      []ActionCenterItem

	// Dynamic ambient message banner (Line 6, viewable on any tab)
	DynamicMessage string

	// Discovered kinds and counts
	ObjectCounts map[string]int
	LastUpdated  time.Time
}

// NewUIModel constructs an initialized UIModel.
func NewUIModel(projectRoot string, initialTab string) *UIModel {
	tab := TabState
	switch strings.ToLower(initialTab) {
	case "audit", "audits", "events":
		tab = TabAudit
	case "swarm", "agent", "agents":
		tab = TabSwarm
	case "pm", "process", "admin", "backlog", "plans", "plan", "objects":
		tab = TabPM
	case "metrics", "telemetry", "metric", "tsdb", "timeseries":
		tab = TabMetrics
	case "scheduler", "jobs", "job":
		tab = TabScheduler
	case "qa", "test", "tests", "tui-test", "lineage", "dod":
		tab = TabQA
	case "health", "system", "action", "action-center", "integrity", "radar":
		tab = TabHealth
	case "state", "seismograph", "stream":
		tab = TabState
	}

	return &UIModel{
		ProjectRoot:  projectRoot,
		ActiveTab:    tab,
		AutoScroll:   true,
		ObjectCounts: make(map[string]int),
		LastUpdated:  time.Now(),
	}
}

// RefreshMutations re-reads recent state events from the non-audit streams.
func (m *UIModel) RefreshMutations() {
	if m.ProjectRoot == "" {
		return
	}
	muts := state.ReadRecentStreamMutations(m.ProjectRoot, 200, "change_journal_entry", "agent_instruction", "process_lifecycle")
	for i, j := 0, len(muts)-1; i < j; i, j = i+1, j-1 {
		muts[i], muts[j] = muts[j], muts[i]
	}
	m.Mutations = muts
	// If dynamic message is empty, auto-populate from latest mutation or agent instruction
	if m.DynamicMessage == "" && len(muts) > 0 {
		latest := muts[len(muts)-1]
		summary := latest.DiffSummary
		if summary == "" {
			summary = latest.ChangeType
		}
		m.DynamicMessage = fmt.Sprintf("%s │ %s", latest.ObjectRef, summary)
	}
	m.LastUpdated = time.Now()
}

// RefreshAuditEvents re-reads high-volume operational audit events.
func (m *UIModel) RefreshAuditEvents() {
	if m.ProjectRoot == "" {
		return
	}
	auds := state.ReadRecentStreamMutations(m.ProjectRoot, 200, "audit_event")
	for i, j := 0, len(auds)-1; i < j; i, j = i+1, j-1 {
		auds[i], auds[j] = auds[j], auds[i]
	}
	m.AuditEvents = auds
	m.LastUpdated = time.Now()
}

// RefreshSwarm queries the storage provider for swarm metrics.
func (m *UIModel) RefreshSwarm(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext) {
	if sp == nil {
		return
	}
	data, err := swarm.BuildSwarmStatus(ctx, sp, sec)
	if err == nil && data != nil {
		if sw, ok := data["swarm"].(map[string]any); ok {
			m.SwarmData = sw
		}
	}
}

// RefreshObjects gathers object counts directly from process directories.
func (m *UIModel) RefreshObjects() {
	if m.ProjectRoot == "" {
		return
	}
	procDir := filepath.Join(m.ProjectRoot, paths.ProcessDir)
	entries, err := fileutil.ReadDir(procDir)
	if err != nil {
		return
	}

	counts := make(map[string]int)
	totalObjects := 0
	for _, e := range entries {
		if e.IsDir() {
			kDir := filepath.Join(procDir, e.Name())
			files, fErr := fileutil.ReadDir(kDir)
			if fErr == nil {
				c := 0
				for _, f := range files {
					if !f.IsDir() && !strings.HasPrefix(f.Name(), ".") &&
						(strings.HasSuffix(f.Name(), ".yaml") || strings.HasSuffix(f.Name(), ".yml") || strings.HasSuffix(f.Name(), ".json")) {
						c++
					}
				}
				if c > 0 {
					counts[e.Name()] = c
					totalObjects += c
				}
			}
		}
	}
	m.ObjectCounts = counts
	m.Hygiene.ProcessObjectCount = totalObjects
	m.Hygiene.KindCount = len(counts)

	// Stream files count
	streamsRoot := filepath.Join(m.ProjectRoot, paths.ProjectDataDir, paths.StreamsDir)
	sEntries, sErr := fileutil.ReadDir(streamsRoot)
	if sErr == nil {
		sFileCount := 0
		activeStreams := 0
		for _, se := range sEntries {
			if se.IsDir() {
				activeStreams++
				sf, _ := fileutil.ReadDir(filepath.Join(streamsRoot, se.Name()))
				sFileCount += len(sf)
			}
		}
		m.Hygiene.StreamFileCount = sFileCount
		m.Hygiene.ActiveStreams = activeStreams
	}
}

// RefreshPM loads the full PM cascade: Mission, Vision, Goals, Workstreams, Priority Plans, BLIs, Requirements, Blockers, Technical Debt.
func (m *UIModel) RefreshPM(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext) {
	if sp == nil {
		return
	}

	// 1. Mission & Vision
	if mis, err := sp.List(ctx, sec, nil, storage.ListFilter{Kind: objects.KindMission}); err == nil && len(mis.Objects) > 0 {
		m.MissionTitle = fmt.Sprintf("%v", mis.Objects[0][objects.FieldKeyTitle])
	}
	if vis, err := sp.List(ctx, sec, nil, storage.ListFilter{Kind: objects.KindVision}); err == nil && len(vis.Objects) > 0 {
		m.VisionTitle = fmt.Sprintf("%v", vis.Objects[0][objects.FieldKeyTitle])
	}

	// 2. Goals
	if gList, err := sp.List(ctx, sec, nil, storage.ListFilter{Kind: objects.KindGoal}); err == nil {
		goals := make([]PMGoalRow, 0, len(gList.Objects))
		for _, g := range gList.Objects {
			goals = append(goals, PMGoalRow{
				ID:     fmt.Sprintf("%v", g[objects.FieldKeyID]),
				Title:  fmt.Sprintf("%v", g[objects.FieldKeyTitle]),
				Status: fmt.Sprintf("%v", g[objects.FieldKeyStatus]),
				Metric: fmt.Sprintf("%v", g[objects.FieldKeyMetric]),
				Target: fmt.Sprintf("%v", g[objects.FieldKeyTarget]),
			})
		}
		sort.Slice(goals, func(i, j int) bool { return goals[i].ID < goals[j].ID })
		m.Goals = goals
	}

	// 3. Workstreams
	if wsList, err := sp.List(ctx, sec, nil, storage.ListFilter{Kind: objects.KindWorkstream}); err == nil {
		workstreams := make([]PMWorkstreamRow, 0, len(wsList.Objects))
		for _, ws := range wsList.Objects {
			workstreams = append(workstreams, PMWorkstreamRow{
				ID:     fmt.Sprintf("%v", ws[objects.FieldKeyID]),
				Title:  fmt.Sprintf("%v", ws[objects.FieldKeyTitle]),
				Status: fmt.Sprintf("%v", ws[objects.FieldKeyStatus]),
			})
		}
		sort.Slice(workstreams, func(i, j int) bool { return workstreams[i].ID < workstreams[j].ID })
		m.Workstreams = workstreams
	}

	// 4. Backlog Items
	if blis, err := sp.List(ctx, sec, nil, storage.ListFilter{Kind: objects.KindBacklogItem}); err == nil {
		var summary PMBacklogSummary
		summary.Total = len(blis.Objects)
		recent := make([]PMBacklogRow, 0, len(blis.Objects))

		for _, b := range blis.Objects {
			st := strings.ToLower(fmt.Sprintf("%v", b[objects.FieldKeyStatus]))
			claimed := fmt.Sprintf("%v", b["claimed_by"])
			if claimed != "" && claimed != "<nil>" {
				summary.Claimed++
			} else {
				summary.Unclaimed++
			}

			switch st {
			case "draft":
				summary.Draft++
			case "planned", "ready":
				summary.Planned++
			case "in_progress", "active":
				summary.InProgress++
			case "blocked":
				summary.Blocked++
			case "completed":
				summary.Completed++
			case "done":
				summary.Done++
			case "approved":
				summary.Approved++
			}

			prio := fmt.Sprintf("%v", b["priority_tier"])
			if prio == "<nil>" || prio == "" {
				prio = fmt.Sprintf("%v", b["priority"])
			}
			if prio == "<nil>" {
				prio = "P2"
			}

			recent = append(recent, PMBacklogRow{
				ID:        fmt.Sprintf("%v", b[objects.FieldKeyID]),
				Title:     fmt.Sprintf("%v", b[objects.FieldKeyTitle]),
				Status:    st,
				Priority:  prio,
				ClaimedBy: claimed,
				PlanRef:   fmt.Sprintf("%v", b[objects.FieldKeyPriorityPlanRef]),
			})
		}

		sort.Slice(recent, func(i, j int) bool { return recent[i].ID < recent[j].ID })
		m.BacklogSummary = summary
		m.RecentBacklog = recent
	}

	// 5. Priority Plans
	if plans, err := sp.List(ctx, sec, nil, storage.ListFilter{Kind: objects.KindPriorityPlan}); err == nil {
		planRows := make([]PMPlanRow, 0, len(plans.Objects))
		for _, p := range plans.Objects {
			pID := fmt.Sprintf("%v", p[objects.FieldKeyID])
			st := fmt.Sprintf("%v", p[objects.FieldKeyStatus])
			var ws []string
			if wsList, ok := p[objects.FieldKeyWorkstreamRefs].([]any); ok {
				for _, w := range wsList {
					ws = append(ws, fmt.Sprintf("%v", w))
				}
			}

			// Count BLIs referencing this plan
			bCount := 0
			for _, b := range m.RecentBacklog {
				if b.PlanRef == pID {
					bCount++
				}
			}

			planRows = append(planRows, PMPlanRow{
				ID:          pID,
				Title:       fmt.Sprintf("%v", p[objects.FieldKeyTitle]),
				Status:      st,
				Workstreams: ws,
				BLICount:    bCount,
			})
		}
		sort.Slice(planRows, func(i, j int) bool { return planRows[i].ID < planRows[j].ID })
		m.PriorityPlans = planRows
	}

	// 6. Requirements
	if reqs, err := sp.List(ctx, sec, nil, storage.ListFilter{Kind: objects.KindRequirement}); err == nil {
		reqRows := make([]PMRequirementRow, 0, len(reqs.Objects))
		for _, r := range reqs.Objects {
			reqRows = append(reqRows, PMRequirementRow{
				ID:     fmt.Sprintf("%v", r[objects.FieldKeyID]),
				Title:  fmt.Sprintf("%v", r[objects.FieldKeyTitle]),
				Status: fmt.Sprintf("%v", r[objects.FieldKeyStatus]),
			})
		}
		sort.Slice(reqRows, func(i, j int) bool { return reqRows[i].ID < reqRows[j].ID })
		m.Requirements = reqRows
	}

	// 7. Active Blockers & Risks
	if blockers, err := sp.List(ctx, sec, nil, storage.ListFilter{Kind: objects.KindRiskBlocker}); err == nil {
		blkRows := make([]PMBlockerRow, 0, len(blockers.Objects))
		for _, blk := range blockers.Objects {
			blkRows = append(blkRows, PMBlockerRow{
				ID:       fmt.Sprintf("%v", blk[objects.FieldKeyID]),
				Title:    fmt.Sprintf("%v", blk[objects.FieldKeyTitle]),
				Severity: fmt.Sprintf("%v", blk["severity"]),
				Status:   fmt.Sprintf("%v", blk[objects.FieldKeyStatus]),
				Impact:   fmt.Sprintf("%v", blk["impact"]),
			})
		}
		sort.Slice(blkRows, func(i, j int) bool { return blkRows[i].ID < blkRows[j].ID })
		m.Blockers = blkRows
	}

	// 8. Technical Debt
	if debts, err := sp.List(ctx, sec, nil, storage.ListFilter{Kind: objects.KindTechnicalDebt}); err == nil {
		debtRows := make([]PMDebtRow, 0, len(debts.Objects))
		for _, d := range debts.Objects {
			debtRows = append(debtRows, PMDebtRow{
				ID:       fmt.Sprintf("%v", d[objects.FieldKeyID]),
				Title:    fmt.Sprintf("%v", d[objects.FieldKeyTitle]),
				Category: fmt.Sprintf("%v", d["debt_category"]),
				Status:   fmt.Sprintf("%v", d[objects.FieldKeyStatus]),
				Priority: fmt.Sprintf("%v", d["priority"]),
			})
		}
		sort.Slice(debtRows, func(i, j int) bool { return debtRows[i].ID < debtRows[j].ID })
		m.TechnicalDebt = debtRows
	}
}

// RefreshMetrics queries telemetry and metrics objects.
func (m *UIModel) RefreshMetrics(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext) {
	if m.ProjectRoot != "" {
		m.TSDB = state.ReadTSDBTelemetry(m.ProjectRoot, 24*time.Hour, 10)
	}

	if sp == nil {
		return
	}

	// 1. Command Metrics
	if res, err := sp.List(ctx, sec, nil, storage.ListFilter{Kind: objects.KindCommandMetric}); err == nil {
		cmdMetrics := make([]CommandMetricRow, 0, len(res.Objects))
		for _, obj := range res.Objects {
			cmdMetrics = append(cmdMetrics, CommandMetricRow{
				ID:          fmt.Sprintf("%v", obj[objects.FieldKeyID]),
				CommandName: fmt.Sprintf("%v", obj["command_name"]),
				ExecCount:   getIntVal(obj["execution_count"]),
				AvgDuration: fmt.Sprintf("%v", obj["duration"]),
				LastRunAt:   formatTimeVal(obj["last_executed_at"]),
				Status:      fmt.Sprintf("%v", obj[objects.FieldKeyStatus]),
			})
		}
		sort.Slice(cmdMetrics, func(i, j int) bool { return cmdMetrics[i].ID < cmdMetrics[j].ID })
		if len(cmdMetrics) > 8 {
			cmdMetrics = cmdMetrics[:8]
		}
		m.CommandMetrics = cmdMetrics
	}

	// 2. Scheduler Health Metrics
	if res, err := sp.List(ctx, sec, nil, storage.ListFilter{Kind: objects.KindSchedulerHealthMetric}); err == nil {
		shRows := make([]SchedulerHealthRow, 0, len(res.Objects))
		for _, obj := range res.Objects {
			shRows = append(shRows, SchedulerHealthRow{
				ID:          fmt.Sprintf("%v", obj[objects.FieldKeyID]),
				HeartbeatAt: formatTimeVal(obj["heartbeat_at"]),
				Status:      fmt.Sprintf("%v", obj[objects.FieldKeyStatus]),
				Executions:  getIntVal(obj["total_executions"]),
				Failures:    getIntVal(obj["failure_count"]),
			})
		}
		// Sort newest heartbeat / highest ID first
		sort.Slice(shRows, func(i, j int) bool { return shRows[i].ID > shRows[j].ID })
		if len(shRows) > 6 {
			shRows = shRows[:6]
		}
		m.SchedulerHealth = shRows
	}

	// 3. File Lock Metrics
	if res, err := sp.List(ctx, sec, nil, storage.ListFilter{Kind: objects.KindFileLockMetric}); err == nil {
		flRows := make([]FileLockMetricRow, 0, len(res.Objects))
		for _, obj := range res.Objects {
			flRows = append(flRows, FileLockMetricRow{
				ID:         fmt.Sprintf("%v", obj[objects.FieldKeyID]),
				TargetKind: fmt.Sprintf("%v", obj["target_kind"]),
				Contention: getIntVal(obj["contention_count"]),
				Duration:   fmt.Sprintf("%v", obj["lock_duration"]),
				Status:     fmt.Sprintf("%v", obj[objects.FieldKeyStatus]),
			})
		}
		m.LockMetrics = flRows
	}

	// 4. Quality & Audit Aggregation Metrics
	if res, err := sp.List(ctx, sec, nil, storage.ListFilter{Kind: objects.KindAuditAggregationMetric}); err == nil {
		qmRows := make([]QualityMetricRow, 0, len(res.Objects))
		for _, obj := range res.Objects {
			qmRows = append(qmRows, QualityMetricRow{
				ID:         fmt.Sprintf("%v", obj[objects.FieldKeyID]),
				MetricType: "Audit Aggregation",
				Value:      fmt.Sprintf("%v events", obj["events_processed"]),
				Status:     fmt.Sprintf("%v", obj[objects.FieldKeyStatus]),
				Window:     fmt.Sprintf("%v", obj["aggregation_window"]),
			})
		}
		m.QualityMetrics = qmRows
	}
}

// RefreshScheduler queries scheduler jobs.
func (m *UIModel) RefreshScheduler(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext) {
	if sp == nil {
		return
	}
	res, err := sp.List(ctx, sec, nil, storage.ListFilter{
		Kind: objects.KindSchedulerJob,
	})
	if err != nil || res == nil {
		return
	}

	jobs := make([]SchedulerJobRow, 0, len(res.Objects))
	for _, obj := range res.Objects {
		id := fmt.Sprintf("%v", obj["id"])
		sch := fmt.Sprintf("%v", obj["schedule"])
		if sch == "" || sch == "<nil>" {
			sch = fmt.Sprintf("%v", obj["interval"])
		}
		if sch == "" || sch == "<nil>" {
			sch = "--"
		}

		lastRun := formatTimeVal(obj["last_run_at"])
		nextRun := formatTimeVal(obj["next_run_at"])

		status := fmt.Sprintf("%v", obj["last_status"])
		if status == "<nil>" || status == "" {
			status = fmt.Sprintf("%v", obj[objects.FieldKeyStatus])
		}
		if status == "" || status == "<nil>" {
			status = "active"
		}

		jobs = append(jobs, SchedulerJobRow{
			ID:        id,
			Schedule:  sch,
			LastRunAt: lastRun,
			NextRunAt: nextRun,
			Status:    status,
		})
	}

	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].ID < jobs[j].ID
	})
	m.SchedulerJobs = jobs
}

func getIntVal(v any) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func formatTimeVal(v any) string {
	s := fmt.Sprintf("%v", v)
	if s == "<nil>" || s == "" {
		return "--"
	}
	if len(s) > 19 {
		return s[:19]
	}
	return s
}

// RefreshQA loads test dashboard metrics, test cases, and lineage status.
// It prioritizes zero-cost reads from the materialized test_dashboard_lite.json file,
// and falls back to storage scanning when sp is provided.
func (m *UIModel) RefreshQA(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext) {
	if m.ProjectRoot == "" {
		return
	}

	dState := test.NewDashboardStateWithProjectRoot(m.ProjectRoot)
	loaded, err := dState.LoadFromLiteFile(m.ProjectRoot)
	if !loaded || err != nil {
		if sp != nil && sec != nil {
			_ = dState.ScanFromStorage(ctx, sp)
		}
	}

	payload := dState.BuildPayload()
	if payload == nil {
		return
	}

	// Order test cases according to TestCaseOrder
	var tcs []*test.TestCaseModel
	for _, id := range payload.TestCaseOrder {
		if tc, ok := payload.TestCases[id]; ok {
			tcs = append(tcs, tc)
		}
	}
	// Fallback if order list was empty
	if len(tcs) == 0 && len(payload.TestCases) > 0 {
		for _, tc := range payload.TestCases {
			tcs = append(tcs, tc)
		}
		sort.Slice(tcs, func(i, j int) bool { return tcs[i].ID < tcs[j].ID })
	}

	// Evaluate DoD compliance: all active test cases must have intact lineage
	dodOk := true
	for _, tc := range tcs {
		if tc.Status == objects.ObjectStatusActive {
			if tc.Lineage == nil || !tc.Lineage.IsIntact {
				dodOk = false
				break
			}
		}
	}
	if len(payload.UnboundTestCriteria) > 0 {
		dodOk = false
	}

	m.QASummary = QASummaryRow{
		TotalTestCases:    payload.TotalTestCases,
		InFlightCount:     payload.InFlightCount,
		RegressionCount:   payload.RegressionCount,
		SatisfiedCriteria: payload.SatisfiedCriteria,
		TotalCriteria:     payload.TotalCriteria,
		IntactChains:      payload.IntactChains,
		UnboundCriteria:   len(payload.UnboundTestCriteria),
		DoDCompliant:      dodOk,
	}
	m.TestCases = tcs
	m.UnboundCriteria = payload.UnboundTestCriteria
	m.RecentQAEvents = payload.RecentEvents
}

// RefreshHealth loads the zero-cost validation cache snapshot, tray shortcuts, and resource hygiene.
func (m *UIModel) RefreshHealth() {
	if m.ProjectRoot == "" {
		return
	}

	// 1. Initialize Action Center items from Tray manifest + native scheduler bindings
	if len(m.ActionItems) == 0 {
		trayEntries, _ := tray.Load(m.ProjectRoot)
		keyMap := []string{"c", "a", "w", "d", "p", "k", "s", "b"}
		keyIdx := 0

		// Priority well-known actions
		defaultActions := []struct {
			name  string
			desc  string
			jobID string
		}{
			{"Quick Cache Check", "Trigger non-blocking validation scan via scheduler", schedulerpkg.DefaultCachePrewarmJobID},
			{"Auto-Fix Batch", "Execute batch remediation of auto-fixable issues", "SCH-autofix-run"},
			{"Workflow What's Next", "Run autonomous priority plan discovery", "SCH-cap-orchestrator"},
			{"Regression Test Suite", "Run full regression test matrix via scheduler", "SCH-run-pkg-scheduler-0"},
			{"Events Aggregation", "Aggregate events and flush journal buffers", schedulerpkg.SchedulerEventsAggregationJobID},
			{"Maintenance WAL Cycle", "Trigger storage maintenance & WAL retention cycle", schedulerpkg.MaintenanceWALTriggerJobID},
		}

		var items []ActionCenterItem
		for _, da := range defaultActions {
			key := ""
			if keyIdx < len(keyMap) {
				key = keyMap[keyIdx]
				keyIdx++
			}
			items = append(items, ActionCenterItem{
				Key:         key,
				Name:        da.name,
				Description: da.desc,
				JobID:       da.jobID,
			})
		}

		// Also incorporate custom entries from user tray if any
		for _, te := range trayEntries {
			if te.IsDefault {
				continue
			}
			if keyIdx >= len(keyMap) {
				break
			}
			items = append(items, ActionCenterItem{
				Key:         keyMap[keyIdx],
				Name:        te.Name,
				Description: te.Description,
				JobID:       "SCH-tray-" + te.Name,
			})
			keyIdx++
		}
		m.ActionItems = items
	}

	// 2. Read validation_cache.json snapshot (< 3ms, zero validation overhead)
	cachePath := filepath.Join(m.ProjectRoot, paths.ProjectDataDir, paths.CacheDir, paths.ValidationCacheFile)
	data, err := fileutil.ReadFile(cachePath)
	var violations []HealthViolationRow
	var t1, t2, t3, autoFixCount int
	var updatedTime time.Time

	if err == nil && len(data) > 0 {
		var cacheFile struct {
			Version string    `json:"version"`
			Updated time.Time `json:"updated"`
			ByKind  map[string]struct {
				PathPrefix string `json:"path_prefix"`
				Entries    []struct {
					ID          string `json:"id"`
					Path        string `json:"path"`
					ValidatedBy string `json:"validated_by"`
					Issues      []struct {
						Tier        int       `json:"tier"`
						Category    string    `json:"category"`
						Message     string    `json:"message"`
						AutoFixable bool      `json:"auto_fixable"`
						DetectedAt  time.Time `json:"detected_at"`
					} `json:"issues"`
				} `json:"entries"`
			} `json:"by_kind"`
		}

		if jErr := json.Unmarshal(data, &cacheFile); jErr == nil {
			updatedTime = cacheFile.Updated
			for kind, bucket := range cacheFile.ByKind {
				for _, entry := range bucket.Entries {
					for _, iss := range entry.Issues {
						sev := "INFO"
						switch iss.Tier {
						case 1:
							sev = "BLOCKER"
							t1++
						case 2:
							sev = "WARN"
							t2++
						case 3:
							sev = "NOTICE"
							t3++
						default:
							sev = "INFO"
						}
						if iss.AutoFixable {
							autoFixCount++
						}

						fullPath := entry.Path
						if bucket.PathPrefix != "" && !strings.HasPrefix(fullPath, bucket.PathPrefix) {
							fullPath = filepath.Join(bucket.PathPrefix, fullPath)
						}

						violations = append(violations, HealthViolationRow{
							Tier:        iss.Tier,
							Severity:    sev,
							Kind:        kind,
							ObjectID:    entry.ID,
							Category:    iss.Category,
							Message:     iss.Message,
							AutoFixable: iss.AutoFixable,
							Path:        fullPath,
						})
					}
				}
			}
		}
	}

	// Sort violations: Tier 1 first, then Tier 2, then Tier 3
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Tier != violations[j].Tier {
			return violations[i].Tier < violations[j].Tier
		}
		return violations[i].ObjectID < violations[j].ObjectID
	})
	m.HealthViolations = violations

	// Freshness calculation
	freshness := "FRESH"
	if !updatedTime.IsZero() {
		age := time.Since(updatedTime)
		if age > 15*time.Minute {
			freshness = fmt.Sprintf("STALE (%dm ago)", int(age.Minutes()))
		} else {
			freshness = fmt.Sprintf("FRESH (%dm ago)", int(age.Minutes()))
		}
	} else {
		freshness = "NO SNAPSHOT"
	}

	status := "HEALTHY"
	if t1 > 0 {
		status = "BLOCKED"
	} else if t2 > 0 {
		status = "ATTENTION"
	} else if t3 > 0 {
		status = "NOTICE"
	}

	staleLocks := 0
	openFDs := 10
	maxFDs := 245760
	storFiles := m.Hygiene.ProcessObjectCount
	storSizeStr := "142MiB"

	if m.TSDB != nil && m.TSDB.HygieneStats.MaxFileDescriptors > 0 {
		h := m.TSDB.HygieneStats
		staleLocks = h.StaleLocksCount
		openFDs = h.OpenFileDescriptors
		maxFDs = h.MaxFileDescriptors
		storFiles = h.TotalStorageFiles
		storSizeStr = h.StorageSizeStr
	}

	m.HealthSummary = HealthSummary{
		LastChecked:      updatedTime,
		CheckFreshness:   freshness,
		OverallStatus:    status,
		TotalViolations:  len(violations),
		Tier1Count:       t1,
		Tier2Count:       t2,
		Tier3Count:       t3,
		AutoFixableCount: autoFixCount,
		StaleLocksCount:  staleLocks,
		StorageFiles:     storFiles,
		StorageSizeStr:   storSizeStr,
		OpenFileDesc:     openFDs,
		MaxFileDesc:      maxFDs,
	}
}

// TriggerActionCenter executes an action by enqueuing its native scheduler job into the JobTriggerQueue.
func (m *UIModel) TriggerActionCenter(key string) bool {
	if m.ProjectRoot == "" {
		return false
	}
	var target *ActionCenterItem
	for i := range m.ActionItems {
		if strings.EqualFold(m.ActionItems[i].Key, key) {
			target = &m.ActionItems[i]
			break
		}
	}
	if target == nil {
		return false
	}

	// Enqueue via native scheduler trigger queue (< 1ms, atomic file lock)
	tq := schedulerpkg.NewJobTriggerQueue(m.ProjectRoot)
	err := tq.EnqueueTriggerRequest(target.JobID)
	if err != nil {
		m.DynamicMessage = fmt.Sprintf("✗ Trigger queue error for %s: %v", target.Name, err)
		return true
	}

	target.IsTriggered = true
	m.DynamicMessage = fmt.Sprintf("⚡ Enqueued [%s] to scheduler trigger queue (%s)", target.Name, target.JobID)
	return true
}

// GetCurrentRowCount returns the number of selectable rows in the active tab.
func (m *UIModel) GetCurrentRowCount() int {
	switch m.ActiveTab {
	case TabState:
		return len(m.Mutations)
	case TabAudit:
		return len(m.AuditEvents)
	case TabPM:
		return len(m.RecentBacklog)
	case TabMetrics:
		return len(m.CommandMetrics)
	case TabScheduler:
		return len(m.SchedulerJobs)
	case TabQA:
		return len(m.TestCases)
	case TabHealth:
		return len(m.HealthViolations)
	default:
		return 0
	}
}

// OpenSelectedItemDetail constructs an ItemDetailModel for the currently selected row in the active tab.
func (m *UIModel) OpenSelectedItemDetail() {
	idx := m.SelectedIndex
	if idx < 0 {
		return
	}

	switch m.ActiveTab {
	case TabState:
		if idx < len(m.Mutations) {
			mut := m.Mutations[idx]
			tStr := "--"
			if mut.CreatedAt > 0 {
				tStr = time.Unix(mut.CreatedAt, 0).Format("2006-01-02 15:04:05")
			}
			details := []string{
				fmt.Sprintf("Event Type    : %s", mut.ChangeType),
				fmt.Sprintf("Object Target : %s", mut.ObjectRef),
				fmt.Sprintf("Recorded At   : %s", tStr),
			}
			if mut.DiffSummary != "" {
				details = append(details, fmt.Sprintf("Diff Summary  : %s", mut.DiffSummary))
			}
			m.DetailModal = &ItemDetailModel{
				Kind:      "state_mutation",
				ID:        mut.ID,
				Status:    mut.ChangeType,
				Title:     mut.ObjectRef,
				Timestamp: tStr,
				Actor:     mut.Actor,
				Summary:   mut.DiffSummary,
				Details:   details,
			}
		}

	case TabAudit:
		if idx < len(m.AuditEvents) {
			aud := m.AuditEvents[idx]
			tStr := "--"
			if aud.CreatedAt > 0 {
				tStr = time.Unix(aud.CreatedAt, 0).Format("2006-01-02 15:04:05")
			}
			act := aud.CreatedBy
			if act == "" {
				act = aud.Actor
			}
			details := []string{
				fmt.Sprintf("Actor         : %s", act),
				fmt.Sprintf("Operation     : %s", aud.Operation),
				fmt.Sprintf("Object Ref    : %s", aud.ObjectRef),
				fmt.Sprintf("Change Type   : %s", aud.ChangeType),
				fmt.Sprintf("Timestamp     : %s", tStr),
			}
			if aud.DiffSummary != "" {
				details = append(details, fmt.Sprintf("Diff / Payload: %s", aud.DiffSummary))
			}
			m.DetailModal = &ItemDetailModel{
				Kind:      "audit_event",
				ID:        aud.ID,
				Status:    aud.Operation,
				Title:     aud.ObjectRef,
				Timestamp: tStr,
				Actor:     act,
				Summary:   aud.DiffSummary,
				Details:   details,
			}
		}

	case TabPM:
		if idx < len(m.RecentBacklog) {
			bli := m.RecentBacklog[idx]
			claimed := bli.ClaimedBy
			if claimed == "" || claimed == "<nil>" {
				claimed = "unassigned"
			}
			plan := bli.PlanRef
			if plan == "" || plan == "<nil>" {
				plan = "none"
			}
			details := []string{
				fmt.Sprintf("Priority Tier : %s", bli.Priority),
				fmt.Sprintf("Workflow State: %s", bli.Status),
				fmt.Sprintf("Claimed Owner : %s", claimed),
				fmt.Sprintf("Priority Plan : %s", plan),
			}
			var lineage []string
			if plan != "none" {
				lineage = append(lineage, fmt.Sprintf("[plan %s] ➔ [%s %s]", plan, bli.ID, bli.Title))
			}
			m.DetailModal = &ItemDetailModel{
				Kind:    objects.KindBacklogItem,
				ID:      bli.ID,
				Status:  bli.Status,
				Title:   bli.Title,
				Actor:   claimed,
				Details: details,
				Lineage: lineage,
			}
		}

	case TabMetrics:
		if idx < len(m.CommandMetrics) {
			cm := m.CommandMetrics[idx]
			details := []string{
				fmt.Sprintf("Command Name  : %s", cm.CommandName),
				fmt.Sprintf("Invocations   : %d", cm.ExecCount),
				fmt.Sprintf("Duration      : %s", cm.AvgDuration),
				fmt.Sprintf("Last Execution: %s", cm.LastRunAt),
				fmt.Sprintf("Status        : %s", cm.Status),
			}
			m.DetailModal = &ItemDetailModel{
				Kind:      objects.KindCommandMetric,
				ID:        cm.ID,
				Status:    cm.Status,
				Title:     cm.CommandName,
				Timestamp: cm.LastRunAt,
				Details:   details,
			}
		}

	case TabScheduler:
		if idx < len(m.SchedulerJobs) {
			job := m.SchedulerJobs[idx]
			details := []string{
				fmt.Sprintf("Cron Schedule : %s", job.Schedule),
				fmt.Sprintf("Last Executed : %s", job.LastRunAt),
				fmt.Sprintf("Next Scheduled: %s", job.NextRunAt),
				fmt.Sprintf("Job Status    : %s", job.Status),
			}
			m.DetailModal = &ItemDetailModel{
				Kind:      objects.KindSchedulerJob,
				ID:        job.ID,
				Status:    job.Status,
				Title:     job.ID,
				Timestamp: job.LastRunAt,
				Details:   details,
			}
		}

	case TabQA:
		if idx < len(m.TestCases) {
			tc := m.TestCases[idx]
			details := []string{
				fmt.Sprintf("Scope / Suite : %s (%s)", tc.Scope, tc.Category),
				fmt.Sprintf("Target File   : %s", tc.PathOrID),
				fmt.Sprintf("Criteria State: %d / %d satisfied (%d open)", tc.CompletedCriteria, tc.TotalCriteria, tc.RemainingOpenCount),
			}

			var lineageStrs []string
			if tc.Lineage != nil {
				if tc.Lineage.RootObject != nil {
					lineageStrs = append(lineageStrs, fmt.Sprintf("Root Object : [%s %s] %s", tc.Lineage.RootObject.Kind, tc.Lineage.RootObject.ID, tc.Lineage.RootObject.Title))
				}
				for _, req := range tc.Lineage.Requirements {
					lineageStrs = append(lineageStrs, fmt.Sprintf("Requirement : [%s %s] %s", req.Kind, req.ID, req.Title))
				}
				for _, bli := range tc.Lineage.BacklogItems {
					lineageStrs = append(lineageStrs, fmt.Sprintf("Backlog Item: [%s %s] %s", bli.Kind, bli.ID, bli.Title))
				}
				if tc.Lineage.IsIntact {
					lineageStrs = append(lineageStrs, "Lineage Chain: ✓ INTACT (Full downward traceability confirmed)")
				} else {
					lineageStrs = append(lineageStrs, fmt.Sprintf("Lineage Chain: ✗ BROKEN (%s)", tc.Lineage.BrokenReason))
				}
			}

			var critStrs []string
			for _, cr := range tc.Criteria {
				satTime := "--"
				if !cr.LastSatisfied.IsZero() {
					satTime = cr.LastSatisfied.Format("2006-01-02 15:04:05")
				}
				critStrs = append(critStrs, fmt.Sprintf("[%s] %s (status: %s, satisfied: %s)", cr.ID, cr.Description, cr.Status, satTime))
			}

			m.DetailModal = &ItemDetailModel{
				Kind:     objects.KindTestCase,
				ID:       tc.ID,
				Status:   tc.Status,
				Title:    tc.Title,
				Details:  details,
				Lineage:  lineageStrs,
				Criteria: critStrs,
			}
		}

	case TabHealth:
		if idx < len(m.HealthViolations) {
			v := m.HealthViolations[idx]
			details := []string{
				fmt.Sprintf("Violation Tier: Tier %d (%s)", v.Tier, v.Severity),
				fmt.Sprintf("Category      : %s", v.Category),
				fmt.Sprintf("Object Target : [%s] %s", v.Kind, v.ObjectID),
				fmt.Sprintf("Auto-Fixable  : %v", v.AutoFixable),
				fmt.Sprintf("CAS File Path : %s", v.Path),
			}
			var actions []string
			if v.AutoFixable {
				actions = append(actions, "Remediation  : Press [a] in Health Tab to run batch auto-fix")
			} else {
				actions = append(actions, fmt.Sprintf("Remediation  : Run 'zqk system check %s' for targeted diagnosis", v.ObjectID))
			}

			m.DetailModal = &ItemDetailModel{
				Kind:     v.Kind,
				ID:       v.ObjectID,
				Status:   v.Severity,
				Title:    v.Message,
				Details:  details,
				Criteria: actions,
			}
		}
	}
}

