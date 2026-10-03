package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zqk-os/zqk/cmd/zqk/state"
	"github.com/zqk-os/zqk/cmd/zqk/swarm"
	"github.com/zqk-os/zqk/cmd/zqk/test"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/daemon/overseer"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/objects/koi"
	"github.com/zqk-os/zqk/pkg/paths"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/tde"
	"github.com/zqk-os/zqk/pkg/tray"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// Editor profile constants for human experience levels.
const (
	ProfileNewb = "newb"
	ProfilePro  = "pro"
	ProfileJedi = "jedi"
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

// InboxItemRow represents an unacknowledged correspondence item or interrupt envelope in TUI.
type InboxItemRow struct {
	ID        string `json:"id"`
	Type      string `json:"type"` // "correspondence" or "tde_envelope"
	Sender    string `json:"sender"`
	Target    string `json:"target"`
	Summary   string `json:"summary"`
	Timestamp string `json:"timestamp"`
	Status    string `json:"status"`
}

// ActionCenterItem represents a triggerable action shortcut backed by the tray and scheduler.
type ActionCenterItem struct {
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	JobID       string    `json:"job_id"`
	IsTriggered bool      `json:"is_triggered"`
	Status      string    `json:"status,omitempty"` // "idle", "enqueued", "processing", "completed", "failed"
	TriggeredAt time.Time `json:"triggered_at,omitempty"`
}

// DaemonHealthRow captures the operational status of a supervised background daemon.
type DaemonHealthRow struct {
	Name         string `json:"name"`
	DesiredState string `json:"desired_state"`
	ActualState  string `json:"actual_state"`
	PID          int    `json:"pid"`
	PGID         int    `json:"pgid"`
	RestartCount int    `json:"restart_count"`
	Uptime       string `json:"uptime"`
	Status       string `json:"status"` // "HEALTHY", "STOPPED", "BACKOFF", "CRASHED"
}

// HealthSummary captures system integrity and hygiene indicators.
type HealthSummary struct {
	LastChecked      time.Time `json:"last_checked"`
	CheckFreshness   string    `json:"check_freshness"`
	OverallStatus    string    `json:"overall_status"`
	TotalViolations  int       `json:"total_violations"`
	Tier1Count       int       `json:"tier1_count"`
	Tier2Count       int       `json:"tier2_count"`
	Tier3Count       int       `json:"tier3_count"`
	AutoFixableCount int       `json:"auto_fixable_count"`
	StaleLocksCount  int       `json:"stale_locks_count"`
	StorageFiles     int       `json:"storage_files"`
	StorageSizeStr   string    `json:"storage_size_str"`
	OpenFileDesc     int       `json:"open_file_desc"`
	MaxFileDesc      int       `json:"max_file_desc"`
	OverseerRunning  bool      `json:"overseer_running"`
	DaemonsRunning   int       `json:"daemons_running"`
	DaemonsTotal     int       `json:"daemons_total"`
}

// SchedulerJobRow captures a job's operational state for display.
type SchedulerJobRow struct {
	ID            string
	Title         string
	Description   string
	Category      string
	JobType       string
	TriggerType   string
	ExecutionMode string
	MaxRuntimeSec int
	Schedule      string
	LastRunAt     string
	NextRunAt     string
	Status        string
	Command       string
	CommandArgs   []string
	LastError     string
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
	Kind       string            `json:"kind"`
	ID         string            `json:"id"`
	Status     string            `json:"status"`
	Title      string            `json:"title"`
	Timestamp  string            `json:"timestamp,omitempty"`
	Actor      string            `json:"actor,omitempty"`
	Summary    string            `json:"summary,omitempty"`
	Details    []string          `json:"details,omitempty"`
	Lineage    []string          `json:"lineage,omitempty"`
	Criteria   []string          `json:"criteria,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	RawPayload string            `json:"raw_payload,omitempty"`
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
	VerifiedBLICount  int
	TotalBLICount     int
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
	Storage      storage.ObjectStorageProvider
	SecCtx       *pkgctx.SecurityContext

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
	DaemonHealth     []DaemonHealthRow

	// Dynamic ambient message banner (Line 6, viewable on any tab)
	DynamicMessage string

	// Swarm & Operator Inbox
	InboxItems []InboxItemRow

	// Human Editor Experience Profile: "newb" (full help/legend/banners), "pro" (compact header/footer), "jedi" (zen mode - full table view)
	EditorProfile string

	// Inline interactive search filter
	IsSearching  bool
	SearchBuffer string
	SearchQuery  string

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
	case "scheduler", "sched", "jobs", "job":
		tab = TabScheduler
	case "qa", "test", "tests", "tui-test", "lineage", "dod":
		tab = TabQA
	case "health", "system", "action", "action-center", "integrity", "radar":
		tab = TabHealth
	case "state", "seismograph", "stream":
		tab = TabState
	}

	profile := ProfileNewb
	if envProfile := zqkenv.EditorProfile().Get(); envProfile != "" {
		switch strings.ToLower(envProfile) {
		case ProfilePro:
			profile = ProfilePro
		case ProfileJedi:
			profile = ProfileJedi
		}
	}

	return &UIModel{
		ProjectRoot:   projectRoot,
		ActiveTab:     tab,
		AutoScroll:    true,
		EditorProfile: profile,
		ObjectCounts:  make(map[string]int),
		LastUpdated:   time.Now(),
	}
}

// CycleEditorProfile toggles the human editor experience level between newb, pro, and jedi.
func (m *UIModel) CycleEditorProfile() {
	switch m.EditorProfile {
	case ProfileNewb, "":
		m.EditorProfile = ProfilePro
		m.DynamicMessage = "⚡ Profile switched to: PRO (compact header & footer)"
	case ProfilePro:
		m.EditorProfile = ProfileJedi
		m.DynamicMessage = "⚡ Profile switched to: JEDI (zen mode — maximum data view)"
	case ProfileJedi:
		m.EditorProfile = ProfileNewb
		m.DynamicMessage = "⚡ Profile switched to: NEWB (full header, footer & hints)"
	default:
		m.EditorProfile = ProfileNewb
	}
}

// SetEditorProfile safely assigns the editor profile to a valid preset.
func (m *UIModel) SetEditorProfile(profile string) {
	switch strings.ToLower(profile) {
	case ProfilePro:
		m.EditorProfile = ProfilePro
	case ProfileJedi:
		m.EditorProfile = ProfileJedi
	default:
		m.EditorProfile = ProfileNewb
	}
}

// ProfileSpacingBonus returns the vertical row budget bonus gained from compact or collapsed views.
func (m *UIModel) ProfileSpacingBonus() int {
	switch m.EditorProfile {
	case ProfileJedi:
		return 10
	case ProfilePro:
		return 5
	default:
		return 0
	}
}

// RefreshActiveTab refreshes only the data required by the currently active tab.
// This prevents thrashing storage and filesystem telemetry on inactive tabs.
func (m *UIModel) RefreshActiveTab(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext) {
	switch m.ActiveTab {
	case TabState:
		m.RefreshMutations()
	case TabAudit:
		m.RefreshAuditEvents()
	case TabSwarm:
		if sp != nil && sec != nil {
			m.RefreshSwarm(ctx, sp, sec)
		}
	case TabPM:
		if sp != nil && sec != nil {
			m.RefreshPM(ctx, sp, sec)
		}
	case TabMetrics:
		if sp != nil && sec != nil {
			m.RefreshMetrics(ctx, sp, sec)
		}
	case TabScheduler:
		if sp != nil && sec != nil {
			m.RefreshScheduler(ctx, sp, sec)
		}
	case TabQA:
		m.RefreshQA(ctx, sp, sec)
	case TabHealth:
		m.RefreshHealth()
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
	// Refresh operator and agent inbox items
	m.RefreshInbox()
	// If dynamic message is empty, auto-populate from latest mutation, agent instruction, or chat feed
	if m.DynamicMessage == "" {
		m.RefreshDynamicMessage()
	}
	m.LastUpdated = time.Now()
}

// RefreshInbox queries the agentfeed and TDE staging WAL for unacknowledged inbox items and staged interrupt envelopes.
func (m *UIModel) RefreshInbox() {
	if m.ProjectRoot == "" {
		return
	}
	var items []InboxItemRow
	// 1. Check correspondence for coordinator / operator
	snap, err := agentfeed.LoadCorrespondence(m.ProjectRoot, agentfeed.Seat{
		AgentID: "operator",
	}, 20)
	if err == nil {
		for _, u := range snap.InboxUnacked {
			items = append(items, InboxItemRow{
				ID:        u.EventID,
				Type:      "correspondence",
				Sender:    u.FromAgentID,
				Target:    u.ToAgentID,
				Summary:   u.Summary,
				Timestamp: u.Timestamp,
				Status:    "unacked",
			})
		}
	}
	// 2. Check staged TDE envelopes
	activeEnvs, err := tde.LoadActive(m.ProjectRoot)
	if err == nil {
		for _, env := range activeEnvs {
			items = append(items, InboxItemRow{
				ID:        env.ID,
				Type:      "tde_envelope",
				Sender:    "wal",
				Target:    env.TargetID,
				Summary:   fmt.Sprintf("%s %s (%s)", env.Operation, env.Kind, env.TargetID),
				Timestamp: env.CreatedAt.UTC().Format(time.RFC3339),
				Status:    string(env.Status),
			})
		}
	}
	m.InboxItems = items
}

// GetVisibleInboxItems returns inbox items matching the active search query filter.
func (m *UIModel) GetVisibleInboxItems() []InboxItemRow {
	if m.SearchQuery == "" {
		return m.InboxItems
	}
	var res []InboxItemRow
	for _, it := range m.InboxItems {
		if m.matchesQuery(it.ID, it.Sender, it.Target, it.Summary, it.Type, it.Status) {
			res = append(res, it)
		}
	}
	return res
}

// AcknowledgeInboxItem acknowledges an unacknowledged correspondence item or commits a staged TDE envelope.
func (m *UIModel) AcknowledgeInboxItem(itemID string) bool {
	if m.ProjectRoot == "" || strings.TrimSpace(itemID) == "" {
		return false
	}
	trimmedID := strings.TrimSpace(itemID)

	// Check if this is a staged TDE envelope
	activeEnvs, err := tde.LoadActive(m.ProjectRoot)
	if err == nil {
		for _, env := range activeEnvs {
			if env.ID == trimmedID {
				wal, wErr := tde.NewStagingWAL(m.ProjectRoot)
				if wErr != nil {
					m.DynamicMessage = fmt.Sprintf("Error opening staging WAL: %v", wErr)
					return false
				}
				defer wal.Close()
				if cErr := wal.MarkCommitted(env.ID); cErr != nil {
					m.DynamicMessage = fmt.Sprintf("Error committing envelope: %v", cErr)
					return false
				}
				if sErr := wal.Sync(); sErr != nil {
					m.DynamicMessage = fmt.Sprintf("Error syncing WAL: %v", sErr)
					return false
				}
				m.DynamicMessage = fmt.Sprintf("Committed staged envelope %s", env.ID)
				m.RefreshInbox()
				return true
			}
		}
	}

	// Otherwise treat as agentfeed correspondence
	agentID := "operator"
	if coord := agentfeed.CoordinatorSeatID(m.ProjectRoot); coord != "" {
		agentID = coord
	}
	personaRef := agentfeed.SeatPersonaRef(m.ProjectRoot, agentID)
	if personaRef == "" {
		personaRef = "PER-DEFAULT-OPERATOR"
	}

	_, pErr := agentfeed.AppendPeerAck(m.ProjectRoot, agentID, personaRef, trimmedID, "Acknowledged via Mission Control Console")
	if pErr != nil {
		m.DynamicMessage = fmt.Sprintf("Failed to ack inbox item: %v", pErr)
		return false
	}

	if _, cErr := agentfeed.CompletePeerAckAwaits(m.ProjectRoot, trimmedID, agentID); cErr != nil {
		m.DynamicMessage = fmt.Sprintf("Ack recorded; note: %v", cErr)
	} else {
		m.DynamicMessage = fmt.Sprintf("Acknowledged inbox correspondence %s", trimmedID)
	}
	m.RefreshInbox()
	return true
}

// RespondInboxItem dispatches a response message to the sender of an inbox correspondence item.
func (m *UIModel) RespondInboxItem(itemID, message string) bool {
	if m.ProjectRoot == "" || strings.TrimSpace(itemID) == "" || strings.TrimSpace(message) == "" {
		return false
	}
	trimmedID := strings.TrimSpace(itemID)
	trimmedMsg := strings.TrimSpace(message)

	agentID := "operator"
	if coord := agentfeed.CoordinatorSeatID(m.ProjectRoot); coord != "" {
		agentID = coord
	}

	// Find sender from loaded inbox items
	targetAgent := "coordinator"
	for _, it := range m.InboxItems {
		if it.ID == trimmedID {
			if it.Sender != "" && it.Sender != agentID && it.Sender != "wal" {
				targetAgent = it.Sender
			}
			break
		}
	}

	_, aErr := agentfeed.AppendEvent(agentfeed.AppendEventInput{
		ProjectRoot: m.ProjectRoot,
		Message:     trimmedMsg,
		AgentID:     agentID,
		ToAgentID:   targetAgent,
		Sender:      agentfeed.FeedSenderHumanSteer,
		EventType:   agentfeed.FeedEventTypeMeshStatus,
		SelfACK:     true,
	})
	if aErr != nil {
		m.DynamicMessage = fmt.Sprintf("Failed to dispatch inbox response: %v", aErr)
		return false
	}

	if _, cErr := agentfeed.CompletePeerAckAwaits(m.ProjectRoot, trimmedID, agentID); cErr != nil {
		m.DynamicMessage = fmt.Sprintf("Response dispatched to %s; note: %v", targetAgent, cErr)
	} else {
		m.DynamicMessage = fmt.Sprintf("Responded to %s (%s)", targetAgent, trimmedID)
	}
	m.RefreshInbox()
	return true
}

// RefreshDynamicMessage refreshes the ambient Line 6 dynamic message notification pipeline from
// recent agent chat events, kernel state mutations, or active scheduler triggers.
func (m *UIModel) RefreshDynamicMessage() {
	if m.ProjectRoot == "" {
		return
	}
	// Check agent chat channel for recent kernel events
	chatFile := filepath.Join(m.ProjectRoot, paths.ProjectDataDir, paths.LogsDir, "ide-hooks", "agent_chat_channel.jsonl")
	if data, err := os.ReadFile(chatFile); err == nil {
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if line == "" {
				continue
			}
			var entry struct {
				Type      string `json:"type"`
				Operation string `json:"operation"`
				Kind      string `json:"kind"`
				ObjectID  string `json:"object_id"`
				Message   string `json:"message"`
			}
			if err := json.Unmarshal([]byte(line), &entry); err == nil && entry.ObjectID != "" {
				if entry.Message != "" {
					msg := entry.Message
					if idx := strings.Index(msg, "Object "); idx != -1 {
						msg = msg[idx:]
					}
					m.DynamicMessage = "⚡ " + msg
					return
				}
				m.DynamicMessage = fmt.Sprintf("⚡ %s %s: %s", entry.Kind, entry.Operation, entry.ObjectID)
				return
			}
		}
	}
	// Fallback to latest mutation if available
	if len(m.Mutations) > 0 {
		latest := m.Mutations[len(m.Mutations)-1]
		summary := latest.DiffSummary
		if summary == "" {
			summary = latest.ChangeType
		}
		m.DynamicMessage = fmt.Sprintf("⚡ %s │ %s", latest.ObjectRef, summary)
	}
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
	if mis, err := sp.List(ctx, sec, nil, storage.ListFilter{
		Kind:   objects.KindMission,
		Limit:  1,
		Fields: []string{objects.FieldKeyTitle},
	}); err == nil && len(mis.Objects) > 0 {
		m.MissionTitle = koi.Title(mis.Objects[0])
	}
	if vis, err := sp.List(ctx, sec, nil, storage.ListFilter{
		Kind:   objects.KindVision,
		Limit:  1,
		Fields: []string{objects.FieldKeyTitle},
	}); err == nil && len(vis.Objects) > 0 {
		m.VisionTitle = koi.Title(vis.Objects[0])
	}

	// 2. Goals
	if gList, err := sp.List(ctx, sec, nil, storage.ListFilter{
		Kind:    objects.KindGoal,
		Filters: map[string]any{objects.FieldKeyStatus: map[string]any{"$ne": objects.ObjectStatusArchived}},
		Fields:  []string{objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus, objects.FieldKeyMetric, objects.FieldKeyTarget},
	}); err == nil {
		goals := make([]PMGoalRow, 0, len(gList.Objects))
		for _, g := range gList.Objects {
			k := koi.Wrap(g)
			goals = append(goals, PMGoalRow{
				ID:     k.ID(),
				Title:  k.Title(),
				Status: k.Status(),
				Metric: k.GetString(objects.FieldKeyMetric),
				Target: k.GetString(objects.FieldKeyTarget),
			})
		}
		sort.Slice(goals, func(i, j int) bool { return goals[i].ID < goals[j].ID })
		m.Goals = goals
	}

	// 3. Workstreams
	if wsList, err := sp.List(ctx, sec, nil, storage.ListFilter{
		Kind:    objects.KindWorkstream,
		Filters: map[string]any{objects.FieldKeyStatus: map[string]any{"$ne": objects.ObjectStatusArchived}},
		Fields:  []string{objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus},
	}); err == nil {
		workstreams := make([]PMWorkstreamRow, 0, len(wsList.Objects))
		for _, ws := range wsList.Objects {
			k := koi.Wrap(ws)
			workstreams = append(workstreams, PMWorkstreamRow{
				ID:     k.ID(),
				Title:  k.Title(),
				Status: k.Status(),
			})
		}
		sort.Slice(workstreams, func(i, j int) bool { return workstreams[i].ID < workstreams[j].ID })
		m.Workstreams = workstreams
	}

	// 4. Backlog Items
	if blis, err := sp.List(ctx, sec, nil, storage.ListFilter{
		Kind:    objects.KindBacklogItem,
		Filters: map[string]any{objects.FieldKeyStatus: map[string]any{"$ne": objects.ObjectStatusArchived}},
		Fields: []string{
			objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus,
			"claimed_by", "priority_tier", "priority", objects.FieldKeyPriorityPlanRef,
		},
	}); err == nil {
		var summary PMBacklogSummary
		summary.Total = len(blis.Objects)
		recent := make([]PMBacklogRow, 0, len(blis.Objects))

		for _, b := range blis.Objects {
			k := koi.Wrap(b)
			st := strings.ToLower(k.Status())
			claimed := k.GetString("claimed_by")
			if claimed != "" {
				summary.Claimed++
			} else {
				summary.Unclaimed++
			}

			switch st {
			case "draft":
				summary.Draft++
			case "planned", "ready", "originated":
				summary.Planned++
			case "in_progress", "inprog", "active", "claimed", "executing":
				summary.InProgress++
			case "blocked":
				summary.Blocked++
			case "complete", "completed", "done", "approved", "closed":
				summary.Done++
				summary.Completed++
			default:
				if claimed != "" {
					summary.InProgress++
				} else {
					summary.Planned++
				}
			}

			prio := k.GetString("priority_tier")
			if prio == "" {
				prio = k.GetString("priority")
			}
			if prio == "" {
				prio = "P2"
			}

			recent = append(recent, PMBacklogRow{
				ID:        k.ID(),
				Title:     k.Title(),
				Status:    st,
				Priority:  prio,
				ClaimedBy: claimed,
				PlanRef:   k.GetString(objects.FieldKeyPriorityPlanRef),
			})
		}

		statusRank := func(s string) int {
			switch s {
			case "in_progress", "inprog", "active", "claimed", "executing":
				return 0
			case "planned", "ready", "originated":
				return 1
			case "blocked":
				return 2
			case "draft":
				return 3
			case "complete", "completed", "done", "approved", "closed":
				return 4
			default:
				return 5
			}
		}

		sort.Slice(recent, func(i, j int) bool {
			ri, rj := statusRank(recent[i].Status), statusRank(recent[j].Status)
			if ri != rj {
				return ri < rj
			}
			return recent[i].ID > recent[j].ID
		})
		m.BacklogSummary = summary

		// Filter recent backlog: keep all active/in_progress/planned/blocked/draft units,
		// plus up to 5 completed items so operators can smoothly navigate into Technical Debt.
		var filtered []PMBacklogRow
		completedCount := 0
		for _, b := range recent {
			st := b.Status
			if st == "complete" || st == "completed" || st == "done" || st == "approved" || st == "closed" {
				if completedCount < 5 {
					filtered = append(filtered, b)
					completedCount++
				}
			} else {
				filtered = append(filtered, b)
			}
		}
		m.RecentBacklog = filtered
	}

	// 5. Priority Plans
	if plans, err := sp.List(ctx, sec, nil, storage.ListFilter{
		Kind:    objects.KindPriorityPlan,
		Filters: map[string]any{objects.FieldKeyStatus: map[string]any{"$ne": objects.ObjectStatusArchived}},
		Fields:  []string{objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus, objects.FieldKeyWorkstreamRefs},
	}); err == nil {
		planRows := make([]PMPlanRow, 0, len(plans.Objects))
		for _, p := range plans.Objects {
			k := koi.Wrap(p)
			pID := k.ID()
			st := k.Status()
			ws := k.GetStringSlice(objects.FieldKeyWorkstreamRefs)

			// Count BLIs referencing this plan
			bCount := 0
			for _, b := range m.RecentBacklog {
				if b.PlanRef == pID {
					bCount++
				}
			}

			planRows = append(planRows, PMPlanRow{
				ID:          pID,
				Title:       k.Title(),
				Status:      st,
				Workstreams: ws,
				BLICount:    bCount,
			})
		}
		sort.Slice(planRows, func(i, j int) bool { return planRows[i].ID < planRows[j].ID })
		m.PriorityPlans = planRows
	}

	// 6. Requirements
	if reqs, err := sp.List(ctx, sec, nil, storage.ListFilter{
		Kind:    objects.KindRequirement,
		Filters: map[string]any{objects.FieldKeyStatus: map[string]any{"$ne": objects.ObjectStatusArchived}},
		Fields:  []string{objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus},
	}); err == nil {
		reqRows := make([]PMRequirementRow, 0, len(reqs.Objects))
		for _, r := range reqs.Objects {
			k := koi.Wrap(r)
			reqRows = append(reqRows, PMRequirementRow{
				ID:     k.ID(),
				Title:  k.Title(),
				Status: k.Status(),
			})
		}
		sort.Slice(reqRows, func(i, j int) bool { return reqRows[i].ID < reqRows[j].ID })
		m.Requirements = reqRows
	}

	// 7. Active Blockers & Risks
	if blockers, err := sp.List(ctx, sec, nil, storage.ListFilter{
		Kind:    objects.KindRiskBlocker,
		Filters: map[string]any{objects.FieldKeyStatus: map[string]any{"$ne": objects.ObjectStatusArchived}},
		Fields:  []string{objects.FieldKeyID, objects.FieldKeyTitle, "severity", objects.FieldKeyStatus, "impact"},
	}); err == nil {
		blkRows := make([]PMBlockerRow, 0, len(blockers.Objects))
		for _, blk := range blockers.Objects {
			k := koi.Wrap(blk)
			blkRows = append(blkRows, PMBlockerRow{
				ID:       k.ID(),
				Title:    k.Title(),
				Severity: k.GetString("severity"),
				Status:   k.Status(),
				Impact:   k.GetString("impact"),
			})
		}
		sort.Slice(blkRows, func(i, j int) bool { return blkRows[i].ID < blkRows[j].ID })
		m.Blockers = blkRows
	}

	// 8. Technical Debt
	if debts, err := sp.List(ctx, sec, nil, storage.ListFilter{
		Kind:    objects.KindTechnicalDebt,
		Filters: map[string]any{objects.FieldKeyStatus: map[string]any{"$ne": objects.ObjectStatusArchived}},
		Fields:  []string{objects.FieldKeyID, objects.FieldKeyTitle, "debt_category", objects.FieldKeyStatus, "priority"},
	}); err == nil {
		debtRows := make([]PMDebtRow, 0, len(debts.Objects))
		for _, d := range debts.Objects {
			k := koi.Wrap(d)
			debtRows = append(debtRows, PMDebtRow{
				ID:       k.ID(),
				Title:    k.Title(),
				Category: k.GetString("debt_category"),
				Status:   k.Status(),
				Priority: k.GetString("priority"),
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
			k := koi.Wrap(obj)
			cmdMetrics = append(cmdMetrics, CommandMetricRow{
				ID:          k.ID(),
				CommandName: k.GetString("command_name"),
				ExecCount:   k.GetIntOr("execution_count", 0),
				AvgDuration: k.GetString("duration"),
				LastRunAt:   formatTimeVal(obj["last_executed_at"]),
				Status:      k.Status(),
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
			k := koi.Wrap(obj)
			hb := formatTimeVal(obj["heartbeat_at"])
			if hb == "--" {
				hb = formatTimeVal(obj["last_seen"])
			}
			if hb == "--" {
				hb = formatTimeVal(obj["timestamp"])
			}
			execs := k.GetIntOr("total_executions", 0)
			if execs == 0 {
				execs = k.GetIntOr("collection_count", 0)
			}
			if execs == 0 {
				execs = k.GetIntOr("executions", 0)
			}
			fails := k.GetIntOr("failure_count", 0)
			if fails == 0 {
				fails = k.GetIntOr("failures", 0)
			}
			st := k.Status()
			if st == "" {
				st = k.GetString("status")
			}
			if st == "" {
				st = "healthy"
			}
			shRows = append(shRows, SchedulerHealthRow{
				ID:          k.ID(),
				HeartbeatAt: hb,
				Status:      st,
				Executions:  execs,
				Failures:    fails,
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
			k := koi.Wrap(obj)
			flRows = append(flRows, FileLockMetricRow{
				ID:         k.ID(),
				TargetKind: k.GetString("target_kind"),
				Contention: k.GetIntOr("contention_count", 0),
				Duration:   k.GetString("lock_duration"),
				Status:     k.Status(),
			})
		}
		m.LockMetrics = flRows
	}

	// 4. Quality & Audit Aggregation Metrics
	if res, err := sp.List(ctx, sec, nil, storage.ListFilter{Kind: objects.KindAuditAggregationMetric}); err == nil {
		qmRows := make([]QualityMetricRow, 0, len(res.Objects))
		for _, obj := range res.Objects {
			k := koi.Wrap(obj)
			qmRows = append(qmRows, QualityMetricRow{
				ID:         k.ID(),
				MetricType: "Audit Aggregation",
				Value:      fmt.Sprintf("%v events", obj["events_processed"]),
				Status:     k.Status(),
				Window:     k.GetString("aggregation_window"),
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
		k := koi.Wrap(obj)
		id := k.ID()
		trigType := k.GetString("trigger_type")
		if trigType == "" {
			trigType = "timer"
		}

		sch := k.GetString("schedule_expression")
		if sch == "" {
			sch = k.GetString("schedule")
		}
		if sch == "" {
			sch = k.GetString("interval")
		}
		if sch == "" || sch == "--" {
			if trigType == "event" {
				filter := k.GetString("event_filter")
				if filter != "" {
					sch = "⚡ event: " + filter
				} else {
					sch = "⚡ on-event"
				}
			} else if trigType == "manual" {
				sch = "⚡ manual"
			} else {
				sch = "--"
			}
		}

		lastRun := formatTimeVal(obj["last_run_at"])
		nextRun := formatTimeVal(obj["next_run_at"])

		status := k.GetString("last_status")
		if status == "" {
			status = k.Status()
		}
		if status == "" {
			status = "active"
		}

		title := k.Title()
		if title == "" {
			title = id
		}
		desc := k.GetString(objects.FieldKeyDescription)
		cat := k.GetString("category")
		if cat == "" {
			cat = "system"
		}
		jType := k.GetString("job_type")
		if jType == "" {
			jType = "standard"
		}
		execMode := k.GetString("execution_mode")
		if execMode == "" {
			execMode = "standard"
		}
		maxRun := k.GetIntOr("max_runtime_seconds", 0)
		cmd := k.GetString("command")
		cmdArgs := k.GetStringSlice("command_args")
		lastErr := k.GetString("last_error")

		jobs = append(jobs, SchedulerJobRow{
			ID:            id,
			Title:         title,
			Description:   desc,
			Category:      cat,
			JobType:       jType,
			TriggerType:   trigType,
			ExecutionMode: execMode,
			MaxRuntimeSec: maxRun,
			Schedule:      sch,
			LastRunAt:     lastRun,
			NextRunAt:     nextRun,
			Status:        status,
			Command:       cmd,
			CommandArgs:   cmdArgs,
			LastError:     lastErr,
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
// and falls back to storage scanning when sp is provided or self-heals from ProjectRoot.
func (m *UIModel) RefreshQA(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext) {
	if m.ProjectRoot == "" {
		return
	}

	dState := test.NewDashboardStateWithProjectRoot(m.ProjectRoot)
	loaded, err := dState.LoadFromLiteFile(m.ProjectRoot)
	hasBrokenData := false
	if loaded && err == nil && len(dState.TestCases) > 0 {
		for _, tc := range dState.TestCases {
			if tc.Title == "" && tc.Lineage == nil {
				hasBrokenData = true
				break
			}
		}
	}
	if !loaded || err != nil || len(dState.TestCases) == 0 || hasBrokenData {
		if (sp == nil || sec == nil) && m.ProjectRoot != "" {
			if factory, fErr := storage.NewStorageFactory(ctx, m.ProjectRoot); fErr == nil {
				sp = factory.GetStorage()
				sec = pkgctx.NewSystemSecurityContext()
			}
		}
		if sp != nil {
			_ = dState.ScanFromStorage(ctx, sp)
			if len(dState.TestCases) > 0 {
				_ = dState.SaveToLiteFile(m.ProjectRoot)
			}
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
	dodOk := len(tcs) > 0
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

	// Calculate BLI test coverage
	verifiedBLIs := make(map[string]bool)
	for _, tc := range tcs {
		if tc.Lineage != nil {
			for _, bli := range tc.Lineage.BacklogItems {
				if bli.ID != "" {
					verifiedBLIs[bli.ID] = true
				}
			}
		}
		for _, ref := range tc.BacklogItemRefs {
			if ref != "" {
				verifiedBLIs[ref] = true
			}
		}
	}
	totalBLIs := m.BacklogSummary.Total
	if totalBLIs == 0 && sp != nil && sec != nil {
		if c, err := sp.Count(ctx, sec, storage.ListFilter{
			Kind:    objects.KindBacklogItem,
			Filters: map[string]any{objects.FieldKeyStatus: map[string]any{"$ne": objects.ObjectStatusArchived}},
		}); err == nil {
			totalBLIs = c
		}
	}

	m.QASummary = QASummaryRow{
		TotalTestCases:    payload.TotalTestCases,
		InFlightCount:     payload.InFlightCount,
		RegressionCount:   payload.RegressionCount,
		SatisfiedCriteria: payload.SatisfiedCriteria,
		TotalCriteria:     payload.TotalCriteria,
		IntactChains:      payload.IntactChains,
		UnboundCriteria:   len(payload.UnboundTestCriteria),
		VerifiedBLICount:  len(verifiedBLIs),
		TotalBLICount:     totalBLIs,
		DoDCompliant:      dodOk,
	}
	m.TestCases = tcs
	m.UnboundCriteria = payload.UnboundTestCriteria
	m.RecentQAEvents = payload.RecentEvents
}

// TriggerQARescan forces a full re-scan of the QA test matrix and criteria from storage into the Lite file and UIModel.
func (m *UIModel) TriggerQARescan() {
	if m.ProjectRoot == "" {
		m.DynamicMessage = "🧪 QA re-scan skipped (no project root configured)"
		return
	}
	ctx := context.Background()
	sp := m.Storage
	sec := m.SecCtx
	if (sp == nil || sec == nil) && m.ProjectRoot != "" {
		if factory, fErr := storage.NewStorageFactory(ctx, m.ProjectRoot); fErr == nil {
			sp = factory.GetStorage()
			sec = pkgctx.NewSystemSecurityContext()
		}
	}
	dState := test.NewDashboardStateWithProjectRoot(m.ProjectRoot)
	if sp != nil {
		if err := dState.ScanFromStorage(ctx, sp); err == nil && len(dState.TestCases) > 0 {
			_ = dState.SaveToLiteFile(m.ProjectRoot)
		}
	}
	m.RefreshQA(ctx, sp, sec)
	m.DynamicMessage = fmt.Sprintf("🧪 QA test matrix rescanned (%d test suites, %d criteria)", m.QASummary.TotalTestCases, m.QASummary.TotalCriteria)
}

// RefreshHealth loads the zero-cost validation cache snapshot, tray shortcuts, and resource hygiene.
func (m *UIModel) RefreshHealth() {
	if m.ProjectRoot == "" {
		return
	}

	// 1. Initialize Action Center items from Tray manifest + native scheduler bindings
	if len(m.ActionItems) == 0 {
		trayEntries, _ := tray.Load(m.ProjectRoot)
		keyMap := []string{"c", "a", "w", "d", "p", "m", "s", "b"}
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
			{"Regression Test Suite", "Run full regression test matrix via scheduler", "SCH-passive-test-sweeper"},
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

	// 1b. Update live status of triggered Action Center items
	for i := range m.ActionItems {
		item := &m.ActionItems[i]
		if !item.IsTriggered {
			continue
		}

		// Check if still in trigger queue
		inQueue := false
		tq := schedulerpkg.NewJobTriggerQueue(m.ProjectRoot)
		if pending, pErr := tq.PeekTriggerRequests(); pErr == nil {
			for _, pr := range pending {
				if pr.JobID == item.JobID {
					inQueue = true
					break
				}
			}
		}

		if inQueue {
			item.Status = "enqueued"
			continue
		}

		// Dequeued by scheduler — inspect scheduler diagnostics log for outcome
		statusFound := false
		diagPath := filepath.Join(m.ProjectRoot, paths.ProjectDataDir, paths.SchedulerDir, "diagnostics.jsonl")
		if data, dErr := os.ReadFile(diagPath); dErr == nil && len(data) > 0 {
			scanData := data
			if len(scanData) > 262144 {
				scanData = scanData[len(scanData)-262144:]
			}
			lines := strings.Split(string(scanData), "\n")
			for j := len(lines) - 1; j >= 0; j-- {
				line := strings.TrimSpace(lines[j])
				if line == "" || !strings.Contains(line, item.JobID) {
					continue
				}
				var ev struct {
					Status      string `json:"status"`
					EventType   string `json:"event_type"`
					JobID       string `json:"job_id"`
					OperationID string `json:"operation_id"`
				}
				if json.Unmarshal([]byte(line), &ev) == nil {
					match := ev.JobID == item.JobID || strings.Contains(ev.OperationID, item.JobID) || strings.Contains(line, item.JobID)
					if match {
						if ev.Status == "completed" || ev.EventType == "scheduler_job_completed" {
							item.Status = "completed"
							statusFound = true
							break
						} else if ev.Status == "failed" || ev.EventType == "scheduler_job_failed" {
							item.Status = "failed"
							statusFound = true
							break
						} else if ev.Status == "started" || ev.EventType == "scheduler_job_started" || ev.EventType == "trigger_queue_processing" {
							item.Status = "processing"
							statusFound = true
							break
						}
					}
				}
			}
		}

		if !statusFound {
			if !item.TriggeredAt.IsZero() && time.Since(item.TriggeredAt) < 3*time.Second {
				item.Status = "processing"
			} else {
				item.Status = "completed"
			}
		}
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

	// 3. Query Daemon Process Group Overseer & Managed Daemons (< 5ms)
	var daemonRows []DaemonHealthRow
	overseerRunning := false
	daemonsRunning := 0
	daemonsTotal := 0

	sockPath := overseer.SocketPath(m.ProjectRoot)
	ipcClient := overseer.NewIPCClient(sockPath)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	resp, err := ipcClient.Send(ctx, overseer.IPCRequest{Action: "status"})
	cancel()

	if err == nil && resp != nil && resp.Success {
		overseerRunning = true
		daemonsTotal = len(resp.Daemons)
		for _, d := range resp.Daemons {
			statusStr := "STOPPED"
			uptimeStr := "--"
			if d.ActualState == overseer.ActualStateRunning {
				statusStr = "HEALTHY"
				daemonsRunning++
				if !d.StartedAt.IsZero() {
					uptimeStr = time.Since(d.StartedAt).Round(time.Second).String()
				}
			} else if d.ActualState == overseer.ActualStateBackoff {
				statusStr = "BACKOFF"
			} else if d.ActualState == overseer.ActualStateCrashed {
				statusStr = "CRASHED"
			}

			daemonRows = append(daemonRows, DaemonHealthRow{
				Name:         d.Name,
				DesiredState: string(d.DesiredState),
				ActualState:  string(d.ActualState),
				PID:          d.PID,
				PGID:         d.PGID,
				RestartCount: d.RestartCount,
				Uptime:       uptimeStr,
				Status:       statusStr,
			})
		}
	} else {
		// Fallback to registry on disk if overseer is offline / standby
		regPath := overseer.DefaultRegistryPath(m.ProjectRoot)
		reg := overseer.NewRegistry(regPath)
		if reg.Load() == nil {
			daemons := reg.List()
			daemonsTotal = len(daemons)
			for _, spec := range daemons {
				daemonRows = append(daemonRows, DaemonHealthRow{
					Name:         spec.Name,
					DesiredState: string(spec.DesiredState),
					ActualState:  string(overseer.ActualStateStopped),
					PID:          0,
					PGID:         0,
					RestartCount: 0,
					Uptime:       "--",
					Status:       "STOPPED",
				})
			}
		}
	}

	sort.Slice(daemonRows, func(i, j int) bool {
		return daemonRows[i].Name < daemonRows[j].Name
	})
	m.DaemonHealth = daemonRows

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
		OverseerRunning:  overseerRunning,
		DaemonsRunning:   daemonsRunning,
		DaemonsTotal:     daemonsTotal,
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
	target.Status = "enqueued"
	target.TriggeredAt = time.Now()
	m.DynamicMessage = fmt.Sprintf("⚡ Enqueued [%s] to scheduler trigger queue (%s)", target.Name, target.JobID)
	return true
}

// TriggerScheduledJob enqueues a scheduled job into the JobTriggerQueue immediately or with a delay.
func (m *UIModel) TriggerScheduledJob(jobID string, delaySeconds int) bool {
	if m.ProjectRoot == "" || jobID == "" {
		return false
	}
	tq := schedulerpkg.NewJobTriggerQueue(m.ProjectRoot)
	if delaySeconds <= 0 {
		err := tq.EnqueueTriggerRequest(jobID)
		if err != nil {
			m.DynamicMessage = fmt.Sprintf("✗ Trigger queue error for %s: %v", jobID, err)
			return true
		}
		m.DynamicMessage = fmt.Sprintf("⚡ Enqueued [%s] to scheduler trigger queue (immediate)", jobID)
		return true
	}

	m.DynamicMessage = fmt.Sprintf("⏳ Scheduled [%s] to trigger in %d seconds...", jobID, delaySeconds)
	time.AfterFunc(time.Duration(delaySeconds)*time.Second, func() {
		tqInner := schedulerpkg.NewJobTriggerQueue(m.ProjectRoot)
		if err := tqInner.EnqueueTriggerRequest(jobID); err != nil {
			m.DynamicMessage = fmt.Sprintf("✗ Delayed trigger failed for %s: %v", jobID, err)
		} else {
			m.DynamicMessage = fmt.Sprintf("✓ Delayed trigger enqueued [%s] to scheduler queue", jobID)
		}
	})
	return true
}

func (m *UIModel) matchesQuery(targets ...string) bool {
	if m.SearchQuery == "" {
		return true
	}
	q := strings.ToLower(m.SearchQuery)
	for _, t := range targets {
		if strings.Contains(strings.ToLower(t), q) {
			return true
		}
	}
	return false
}

// GetVisibleMutations returns mutations matching active search query.
func (m *UIModel) GetVisibleMutations() []state.JournalMutation {
	if m.SearchQuery == "" {
		return m.Mutations
	}
	var res []state.JournalMutation
	for _, mut := range m.Mutations {
		if m.matchesQuery(mut.ID, mut.ObjectRef, mut.ChangeType, mut.Actor, mut.DiffSummary) {
			res = append(res, mut)
		}
	}
	return res
}

// GetVisibleAuditEvents returns audit events matching active search query.
func (m *UIModel) GetVisibleAuditEvents() []state.JournalMutation {
	if m.SearchQuery == "" {
		return m.AuditEvents
	}
	var res []state.JournalMutation
	for _, a := range m.AuditEvents {
		if m.matchesQuery(a.ID, a.ObjectRef, a.ChangeType, a.Actor, a.CreatedBy, a.DiffSummary) {
			res = append(res, a)
		}
	}
	return res
}

// GetVisibleBacklog returns backlog items matching active search query.
func (m *UIModel) GetVisibleBacklog() []PMBacklogRow {
	if m.SearchQuery == "" {
		return m.RecentBacklog
	}
	var res []PMBacklogRow
	for _, b := range m.RecentBacklog {
		if m.matchesQuery(b.ID, b.Title, b.Status, b.Priority, b.ClaimedBy, b.PlanRef) {
			res = append(res, b)
		}
	}
	return res
}

// GetVisibleTechnicalDebt returns technical debt items matching active search query.
func (m *UIModel) GetVisibleTechnicalDebt() []PMDebtRow {
	if m.SearchQuery == "" {
		return m.TechnicalDebt
	}
	var res []PMDebtRow
	for _, d := range m.TechnicalDebt {
		if m.matchesQuery(d.ID, d.Title, d.Category, d.Status, d.Priority) {
			res = append(res, d)
		}
	}
	return res
}

// GetVisibleCommandMetrics returns command metrics matching active search query.
func (m *UIModel) GetVisibleCommandMetrics() []CommandMetricRow {
	if m.SearchQuery == "" {
		return m.CommandMetrics
	}
	var res []CommandMetricRow
	for _, cm := range m.CommandMetrics {
		if m.matchesQuery(cm.ID, cm.CommandName, cm.Status) {
			res = append(res, cm)
		}
	}
	return res
}

// GetVisibleSchedulerJobs returns scheduler jobs matching active search query.
func (m *UIModel) GetVisibleSchedulerJobs() []SchedulerJobRow {
	if m.SearchQuery == "" {
		return m.SchedulerJobs
	}
	var res []SchedulerJobRow
	for _, j := range m.SchedulerJobs {
		if m.matchesQuery(j.ID, j.Title, j.Description, j.Category, j.Schedule, j.Status, j.Command) {
			res = append(res, j)
		}
	}
	return res
}

// GetVisibleTestCases returns test cases matching active search query.
func (m *UIModel) GetVisibleTestCases() []*test.TestCaseModel {
	if m.SearchQuery == "" {
		return m.TestCases
	}
	var res []*test.TestCaseModel
	for _, tc := range m.TestCases {
		if tc != nil && m.matchesQuery(tc.ID, tc.Title, tc.Status, tc.PathOrID, tc.Scope, tc.Category) {
			res = append(res, tc)
		}
	}
	return res
}

// GetVisibleHealthViolations returns health violations matching active search query.
func (m *UIModel) GetVisibleHealthViolations() []HealthViolationRow {
	if m.SearchQuery == "" {
		return m.HealthViolations
	}
	var res []HealthViolationRow
	for _, v := range m.HealthViolations {
		if m.matchesQuery(v.ObjectID, v.Kind, v.Severity, v.Category, v.Message, v.Path) {
			res = append(res, v)
		}
	}
	return res
}

// GetVisiblePriorityPlans returns priority plans matching active search query.
func (m *UIModel) GetVisiblePriorityPlans() []PMPlanRow {
	if m.SearchQuery == "" {
		return m.PriorityPlans
	}
	var res []PMPlanRow
	for _, p := range m.PriorityPlans {
		if m.matchesQuery(p.ID, p.Title, p.Status, strings.Join(p.Workstreams, " ")) {
			res = append(res, p)
		}
	}
	return res
}

// GetVisibleBlockers returns active risks/blockers matching active search query.
func (m *UIModel) GetVisibleBlockers() []PMBlockerRow {
	if m.SearchQuery == "" {
		return m.Blockers
	}
	var res []PMBlockerRow
	for _, b := range m.Blockers {
		if m.matchesQuery(b.ID, b.Title, b.Impact, b.Status, b.Severity) {
			res = append(res, b)
		}
	}
	return res
}

// GetCurrentRowCount returns the number of selectable rows in the active tab.
func (m *UIModel) GetCurrentRowCount() int {
	switch m.ActiveTab {
	case TabState:
		return len(m.GetVisibleMutations())
	case TabAudit:
		return len(m.GetVisibleAuditEvents())
	case TabSwarm:
		return len(m.GetVisibleInboxItems())
	case TabPM:
		return len(m.GetVisiblePriorityPlans()) + len(m.GetVisibleBlockers()) + len(m.GetVisibleBacklog()) + len(m.GetVisibleTechnicalDebt())
	case TabMetrics:
		return len(m.GetVisibleCommandMetrics())
	case TabScheduler:
		return len(m.GetVisibleSchedulerJobs())
	case TabQA:
		return len(m.GetVisibleTestCases())
	case TabHealth:
		return len(m.GetVisibleHealthViolations())
	default:
		return 0
	}
}

// openSwarmInboxDetail constructs an ItemDetailModel for the selected inbox item on TabSwarm.
func (m *UIModel) openSwarmInboxDetail(idx int) {
	items := m.GetVisibleInboxItems()
	if idx < 0 || idx >= len(items) {
		return
	}
	item := items[idx]
	details := []string{
		fmt.Sprintf("Item ID       : %s", item.ID),
		fmt.Sprintf("Item Type     : %s", item.Type),
		fmt.Sprintf("Sender        : %s", item.Sender),
		fmt.Sprintf("Target        : %s", item.Target),
		fmt.Sprintf("Status        : %s", item.Status),
		fmt.Sprintf("Timestamp     : %s", item.Timestamp),
	}
	if item.Summary != "" {
		details = append(details, fmt.Sprintf("Summary       : %s", item.Summary))
	}
	m.DetailModal = &ItemDetailModel{
		Kind:      item.Type,
		ID:        item.ID,
		Status:    item.Status,
		Title:     fmt.Sprintf("%s -> %s", item.Sender, item.Target),
		Timestamp: item.Timestamp,
		Actor:     item.Sender,
		Summary:   item.Summary,
		Details:   details,
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
		mutations := m.GetVisibleMutations()
		if idx < len(mutations) {
			mut := mutations[idx]
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
		audits := m.GetVisibleAuditEvents()
		if idx < len(audits) {
			aud := audits[idx]
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

	case TabSwarm:
		m.openSwarmInboxDetail(idx)

	case TabPM:
		plans := m.GetVisiblePriorityPlans()
		blks := m.GetVisibleBlockers()
		backlog := m.GetVisibleBacklog()
		debt := m.GetVisibleTechnicalDebt()

		if idx < len(plans) {
			p := plans[idx]
			details := []string{
				fmt.Sprintf("Plan ID       : %s", p.ID),
				fmt.Sprintf("Status        : %s", p.Status),
				fmt.Sprintf("Workstreams   : %s", strings.Join(p.Workstreams, ", ")),
				fmt.Sprintf("Backlog Units : %d", p.BLICount),
			}
			m.DetailModal = &ItemDetailModel{
				Kind:    objects.KindPriorityPlan,
				ID:      p.ID,
				Status:  p.Status,
				Title:   p.Title,
				Details: details,
			}
		} else if blkIdx := idx - len(plans); blkIdx < len(blks) {
			blk := blks[blkIdx]
			details := []string{
				fmt.Sprintf("Risk ID       : %s", blk.ID),
				fmt.Sprintf("Severity      : %s", blk.Severity),
				fmt.Sprintf("Status        : %s", blk.Status),
			}
			if blk.Impact != "" {
				details = append(details, fmt.Sprintf("Impact        : %s", blk.Impact))
			}
			m.DetailModal = &ItemDetailModel{
				Kind:    objects.KindRiskBlocker,
				ID:      blk.ID,
				Status:  blk.Status,
				Title:   blk.Title,
				Details: details,
			}
		} else if bliIdx := idx - len(plans) - len(blks); bliIdx < len(backlog) {
			bli := backlog[bliIdx]
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
		} else if debtIdx := idx - len(plans) - len(blks) - len(backlog); debtIdx < len(debt) {
			d := debt[debtIdx]
			details := []string{
				fmt.Sprintf("Debt ID       : %s", d.ID),
				fmt.Sprintf("Category      : %s", d.Category),
				fmt.Sprintf("Priority      : %s", d.Priority),
				fmt.Sprintf("Workflow State: %s", d.Status),
			}
			m.DetailModal = &ItemDetailModel{
				Kind:    objects.KindTechnicalDebt,
				ID:      d.ID,
				Status:  d.Status,
				Title:   d.Title,
				Details: details,
			}
		}

	case TabMetrics:
		cmdMetrics := m.GetVisibleCommandMetrics()
		if idx < len(cmdMetrics) {
			cm := cmdMetrics[idx]
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
		jobs := m.GetVisibleSchedulerJobs()
		if idx < len(jobs) {
			job := jobs[idx]
			details := []string{
				fmt.Sprintf("Job Title     : %s", job.Title),
				fmt.Sprintf("Category      : %s", job.Category),
				fmt.Sprintf("Job Type      : %s", job.JobType),
				fmt.Sprintf("Trigger Type  : %s", job.TriggerType),
				fmt.Sprintf("Execution Mode: %s", job.ExecutionMode),
				fmt.Sprintf("Max Runtime   : %d seconds", job.MaxRuntimeSec),
				fmt.Sprintf("Cron Schedule : %s", job.Schedule),
				fmt.Sprintf("Last Executed : %s", job.LastRunAt),
				fmt.Sprintf("Next Scheduled: %s", job.NextRunAt),
				fmt.Sprintf("Job Status    : %s", job.Status),
			}
			if job.Command != "" {
				cmdStr := job.Command
				if len(job.CommandArgs) > 0 {
					cmdStr += " " + strings.Join(job.CommandArgs, " ")
				}
				details = append(details, fmt.Sprintf("Command Exec  : %s", cmdStr))
			}
			if job.LastError != "" {
				details = append(details, fmt.Sprintf("Last Error    : %s", job.LastError))
			}
			var actions []string
			actions = append(actions, "Trigger Now   : Press [t] to enqueue immediate execution")
			actions = append(actions, "Trigger Delay : Press [d] to enqueue execution with 10s delay")

			m.DetailModal = &ItemDetailModel{
				Kind:      objects.KindSchedulerJob,
				ID:        job.ID,
				Status:    job.Status,
				Title:     job.Title,
				Timestamp: job.LastRunAt,
				Summary:   job.Description,
				Details:   details,
				Criteria:  actions,
			}
		}

	case TabQA:
		testCases := m.GetVisibleTestCases()
		if idx < len(testCases) {
			tc := testCases[idx]
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
		violations := m.GetVisibleHealthViolations()
		if idx < len(violations) {
			v := violations[idx]
			details := []string{
				fmt.Sprintf("Violation Tier: Tier %d (%s)", v.Tier, v.Severity),
				fmt.Sprintf("Category      : %s", v.Category),
				fmt.Sprintf("Object Target : [%s] %s", v.Kind, v.ObjectID),
				fmt.Sprintf("Full Message  : %s", v.Message),
				fmt.Sprintf("Auto-Fixable  : %v", v.AutoFixable),
				fmt.Sprintf("CAS File Path : %s", v.Path),
			}
			var actions []string
			if v.AutoFixable {
				actions = append(actions, "Remediation  : Press [a] in Health Tab to run batch auto-fix")
			} else {
				actions = append(actions, fmt.Sprintf("Remediation  : Run '%s' for targeted diagnosis", paths.CLIInvocation("system check "+v.ObjectID)))
			}

			m.DetailModal = &ItemDetailModel{
				Kind:     v.Kind,
				ID:       v.ObjectID,
				Status:   v.Severity,
				Title:    v.ObjectID,
				Summary:  v.Message,
				Details:  details,
				Criteria: actions,
			}
		}
	}
}
