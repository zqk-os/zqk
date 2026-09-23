package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zqk-os/zqk/cmd/zqk/state"
	"github.com/zqk-os/zqk/cmd/zqk/swarm"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
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
	TotalTabs    = 6

	// Backward compatibility aliases
	TabSeismograph = TabState
	TabObjects     = TabPM
)

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

// UIModel encapsulates the dynamic state of the Mission Control TUI.
type UIModel struct {
	ProjectRoot  string
	ActiveTab    int
	ScrollOffset int // 0 means bottom / newest (or top depending on view)
	AutoScroll   bool
	Width        int
	Height       int

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

	// Tab 6: Background Scheduler
	SchedulerJobs []SchedulerJobRow

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
	case "metrics", "telemetry", "metric":
		tab = TabMetrics
	case "scheduler", "jobs", "job":
		tab = TabScheduler
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
