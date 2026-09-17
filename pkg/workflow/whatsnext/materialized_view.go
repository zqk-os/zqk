package whatsnext

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/accumulator"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/lifecycle"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/walutil"
)

const (
	// DefaultStalenessTolerance is the maximum duration before a materialized view is flagged stale.
	DefaultStalenessTolerance = 2 * time.Minute

	// MaterializedViewSchemaVersion defines the schema for whats_next_lite.json.
	MaterializedViewSchemaVersion = "whats_next_lite_v1"
)

// WhatsNextLiteFilePath returns the canonical path to the materialized view projection.
func WhatsNextLiteFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, ".zqk", "state", "whats_next_lite.json")
}

// PlanNode represents an in-memory tracked priority plan.
type PlanNode struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	ActiveOrder int      `json:"active_order"`
	PersonaRefs []string `json:"persona_refs,omitempty"`
	Shaped      bool     `json:"shaped,omitempty"`
}

// BacklogNode represents an in-memory tracked backlog item.
type BacklogNode struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Status          string   `json:"status"`
	PriorityPlanRef string   `json:"priority_plan_ref"`
	PersonaRefs     []string `json:"persona_refs,omitempty"`
}

// TaskNode represents an in-memory tracked agent task.
type TaskNode struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	Status             string `json:"status"`
	AssigneePersonaRef string `json:"assignee_persona_ref"`
	PipelineRef        string `json:"pipeline_ref,omitempty"`
}

// CVSNode represents an active or paused convergence session.
type CVSNode struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	CurrentPhase string `json:"current_phase"`
	Status       string `json:"status"`
}

// ActiveTaskNode describes a seat's active or best proposed task.
type ActiveTaskNode struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Title  string `json:"title"`
}

// WhatsNextEventSummary records recent lifecycle events in the projection.
type WhatsNextEventSummary struct {
	Timestamp time.Time `json:"timestamp"`
	EventType string    `json:"event_type"`
	ObjectID  string    `json:"object_id,omitempty"`
	Kind      string    `json:"kind,omitempty"`
	Message   string    `json:"message"`
}

// WhatsNextLitePayload represents the zero-cost materialized state persisted to disk.
type WhatsNextLitePayload struct {
	SchemaVersion           string                     `json:"schema_version"`
	MaterializedAt          time.Time                  `json:"materialized_at"`
	Stale                   bool                       `json:"stale,omitempty"`
	Recovering              bool                       `json:"recovering,omitempty"`
	DegradedReason          string                     `json:"degraded_reason,omitempty"`
	LeadPlan                *WhatsNextPriorityPlan     `json:"lead_plan,omitempty"`
	ActivePlans             []WhatsNextPriorityPlan    `json:"active_plans,omitempty"`
	BacklogCountsByPlan     map[string]map[string]int  `json:"backlog_counts_by_plan,omitempty"`
	TotalBacklogCounts      map[string]int             `json:"total_backlog_counts,omitempty"`
	RunwayDepth             int                        `json:"runway_depth"`
	IsRunwayDepleted        bool                       `json:"is_runway_depleted"`
	ConvergenceSessions     []WhatsNextCVSRow          `json:"convergence_sessions,omitempty"`
	ActiveTasksByAssignee   map[string]*ActiveTaskNode `json:"active_tasks_by_assignee,omitempty"`
	AgentInstructionsByPlan map[string]string          `json:"agent_instructions_by_plan,omitempty"`
	PackagingCuesByPlan     map[string]string          `json:"packaging_cues_by_plan,omitempty"`
	RecentEvents            []WhatsNextEventSummary    `json:"recent_events,omitempty"`
}

// Global debounced reconciler state across goroutines.
var (
	isReconciling uint32
)

// WhatsNextMaterializedView maintains the reactive in-memory execution graph.
type WhatsNextMaterializedView struct {
	mu           sync.RWMutex
	projectRoot  string
	plans        map[string]*PlanNode
	backlogs     map[string]*BacklogNode
	tasks        map[string]*TaskNode
	cvsSessions  map[string]*CVSNode
	recentEvents []WhatsNextEventSummary
	lastUpdated  time.Time
	engine       *accumulator.Engine[*WhatsNextLitePayload]
}

// NewWhatsNextMaterializedView initializes an empty in-memory view.
func NewWhatsNextMaterializedView(projectRoot string) *WhatsNextMaterializedView {
	view := &WhatsNextMaterializedView{
		projectRoot:  projectRoot,
		plans:        make(map[string]*PlanNode),
		backlogs:     make(map[string]*BacklogNode),
		tasks:        make(map[string]*TaskNode),
		cvsSessions:  make(map[string]*CVSNode),
		recentEvents: make([]WhatsNextEventSummary, 0, 20),
		lastUpdated:  time.Now(),
	}
	spec := accumulator.AccumulatorSpec{
		Name:               "whats_next",
		SchemaVersion:      MaterializedViewSchemaVersion,
		ProjectRoot:        projectRoot,
		StoragePath:        WhatsNextLiteFilePath(projectRoot),
		StalenessTolerance: DefaultStalenessTolerance,
	}
	eng, _ := accumulator.NewEngine[*WhatsNextLitePayload](spec, view)
	view.engine = eng
	return view
}

// Name satisfies accumulator.Accumulator interface.
func (v *WhatsNextMaterializedView) Name() string {
	return "whats_next"
}

// DefaultPayload satisfies accumulator.Accumulator interface.
func (v *WhatsNextMaterializedView) DefaultPayload() *WhatsNextLitePayload {
	return &WhatsNextLitePayload{
		SchemaVersion:           MaterializedViewSchemaVersion,
		MaterializedAt:          time.Now().UTC(),
		Stale:                   true,
		Recovering:              true,
		DegradedReason:          "cold_boot_materialized_projection_missing",
		BacklogCountsByPlan:     make(map[string]map[string]int),
		TotalBacklogCounts:      make(map[string]int),
		ActivePlans:             make([]WhatsNextPriorityPlan, 0),
		ActiveTasksByAssignee:   make(map[string]*ActiveTaskNode),
		AgentInstructionsByPlan: make(map[string]string),
		PackagingCuesByPlan:     make(map[string]string),
	}
}

// ApplyEvent satisfies accumulator.Accumulator interface.
func (v *WhatsNextMaterializedView) ApplyEvent(ev *lifecycle.LifecycleEvent) bool {
	v.ApplyLifecycleEvent(ev)
	return true
}

// BuildPayload satisfies accumulator.Accumulator interface.
func (v *WhatsNextMaterializedView) BuildPayload() *WhatsNextLitePayload {
	return v.BuildPayloadLocked()
}

// ScanFromStorage satisfies accumulator.Accumulator interface.
func (v *WhatsNextMaterializedView) ScanFromStorage(ctx context.Context, sp storage.ObjectStorageProvider) error {
	secCtx := pkgctx.NewSystemSecurityContext()
	return v.ScanFromStorageWithSecurity(ctx, sp, secCtx)
}

// Engine returns the underlying generic accumulator engine.
func (v *WhatsNextMaterializedView) Engine() *accumulator.Engine[*WhatsNextLitePayload] {
	return v.engine
}

// ---------------------------------------------------------------------------
// Pure Static Execution Predicates
// ---------------------------------------------------------------------------

// EvaluatePhiLead selects the lead priority plan based on execution priority:
// In-progress > shovel-ready (ordered by active_order asc) > grooming.
func EvaluatePhiLead(plans []*PlanNode, targetPersonaIDs []string) (*PlanNode, []*PlanNode) {
	if len(plans) == 0 {
		return nil, nil
	}

	hasPersonaMatch := func(p *PlanNode) bool {
		if len(targetPersonaIDs) == 0 || len(p.PersonaRefs) == 0 {
			return true
		}
		for _, target := range targetPersonaIDs {
			for _, ref := range p.PersonaRefs {
				if ref == target {
					return true
				}
			}
		}
		return false
	}

	var matched []*PlanNode
	for _, p := range plans {
		if hasPersonaMatch(p) {
			matched = append(matched, p)
		}
	}

	if len(matched) == 0 {
		return nil, nil
	}

	sort.SliceStable(matched, func(i, j int) bool {
		pi, pj := matched[i], matched[j]
		ri := rankPlanStatus(pi.Status)
		rj := rankPlanStatus(pj.Status)
		if ri != rj {
			return ri < rj
		}
		if pi.ActiveOrder != pj.ActiveOrder {
			if pi.ActiveOrder == 0 {
				return false
			}
			if pj.ActiveOrder == 0 {
				return true
			}
			return pi.ActiveOrder < pj.ActiveOrder
		}
		return pi.ID < pj.ID
	})

	return matched[0], matched
}

func rankPlanStatus(status string) int {
	switch strings.ToLower(status) {
	case objects.ObjectStatusInProgress:
		return 1
	case objects.ObjectStatusActive, "shovel_ready":
		return 2
	case objects.ObjectStatusGrooming:
		return 3
	default:
		return 4
	}
}

// EvaluatePhiCounts calculates O(1) backlog counts by status per plan and aggregate.
func EvaluatePhiCounts(backlogs []*BacklogNode, targetPlanIDs []string, targetPersonaIDs []string) (map[string]map[string]int, map[string]int) {
	byPlan := make(map[string]map[string]int)
	total := make(map[string]int)

	planFilter := make(map[string]bool, len(targetPlanIDs))
	for _, pid := range targetPlanIDs {
		planFilter[pid] = true
	}

	hasPersonaMatch := func(b *BacklogNode) bool {
		if len(targetPersonaIDs) == 0 || len(b.PersonaRefs) == 0 {
			return true
		}
		for _, target := range targetPersonaIDs {
			for _, ref := range b.PersonaRefs {
				if ref == target {
					return true
				}
			}
		}
		return false
	}

	for _, b := range backlogs {
		if len(targetPlanIDs) > 0 && !planFilter[b.PriorityPlanRef] {
			continue
		}
		if !hasPersonaMatch(b) {
			continue
		}

		st := strings.ToLower(strings.TrimSpace(b.Status))
		if st == "" {
			st = "unknown"
		}

		if _, ok := byPlan[b.PriorityPlanRef]; !ok {
			byPlan[b.PriorityPlanRef] = make(map[string]int)
		}
		byPlan[b.PriorityPlanRef][st]++
		total[st]++
	}

	return byPlan, total
}

// EvaluatePhiHunger derives agent instruction and packaging cue from backlog status counts.
func EvaluatePhiHunger(counts map[string]int, plan *PlanNode) (string, string) {
	if counts == nil {
		counts = map[string]int{}
	}

	totalWork := counts["in_progress"] + counts["verifying"] + counts["testing"] + counts["planned"] + counts["blocked"] + counts["validated"] + counts["exploring"]
	var instruction string
	if totalWork == 0 && counts["completed"] > 0 {
		instruction = "shutdown"
	} else if counts["in_progress"] > 0 {
		instruction = "continue"
	} else if counts["verifying"] > 0 || counts["testing"] > 0 {
		instruction = "execute_tests"
	} else if totalWork == 0 {
		instruction = "shutdown"
	} else {
		instruction = "continue"
	}

	var cue string
	if plan != nil {
		cue = PriorityPlanPackagingCue(plan.ID, plan.Title, plan.Status, counts)
	}

	return instruction, cue
}

// EvaluatePhiDispatch maps active or candidate tasks to assignees (in_progress > proposed).
func EvaluatePhiDispatch(tasks []*TaskNode) map[string]*ActiveTaskNode {
	out := make(map[string]*ActiveTaskNode)
	for _, t := range tasks {
		if t.AssigneePersonaRef == "" {
			continue
		}
		st := strings.ToLower(t.Status)
		if st != objects.ObjectStatusInProgress && st != objects.ObjectStatusProposed {
			continue
		}

		existing, exists := out[t.AssigneePersonaRef]
		if !exists {
			out[t.AssigneePersonaRef] = &ActiveTaskNode{ID: t.ID, Status: t.Status, Title: t.Title}
			continue
		}

		// in_progress wins over proposed
		if existing.Status != objects.ObjectStatusInProgress && st == objects.ObjectStatusInProgress {
			out[t.AssigneePersonaRef] = &ActiveTaskNode{ID: t.ID, Status: t.Status, Title: t.Title}
		}
	}
	return out
}

// EvaluatePhiRunway counts priority plans in active, in_progress, shovel_ready, or grooming.
func EvaluatePhiRunway(plans []*PlanNode) int {
	depth := 0
	for _, p := range plans {
		switch strings.ToLower(p.Status) {
		case objects.ObjectStatusActive, objects.ObjectStatusInProgress, "shovel_ready", objects.ObjectStatusGrooming:
			depth++
		}
	}
	return depth
}

// ---------------------------------------------------------------------------
// Storage Scanning & Graph Maintenance
// ---------------------------------------------------------------------------

// ScanFromStorageWithSecurity populates the in-memory execution graph from storage and rebuilds the projection.
func (v *WhatsNextMaterializedView) ScanFromStorageWithSecurity(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	storageCtx := pkgctx.NewStorageContext()

	// 1. Scan Priority Plans
	planRes, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindPriorityPlan})
	if err != nil {
		return fmt.Errorf("list priority plans: %w", err)
	}
	v.plans = make(map[string]*PlanNode, len(planRes.Objects))
	for _, obj := range planRes.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		if id == "" {
			continue
		}
		title, _ := obj[objects.FieldKeyTitle].(string)
		status, _ := obj[objects.FieldKeyStatus].(string)
		order := 0
		if oVal, ok := obj[objects.FieldKeyActiveOrder].(int); ok {
			order = oVal
		} else if oVal, ok := obj[objects.FieldKeyActiveOrder].(float64); ok {
			order = int(oVal)
		}
		personaRefs := lifecycle.StringRefsFromAny(obj[objects.FieldKeyPersonaRefs])
		v.plans[id] = &PlanNode{
			ID:          id,
			Title:       title,
			Status:      status,
			ActiveOrder: order,
			PersonaRefs: personaRefs,
		}
	}

	// 2. Scan Backlog Items
	bliRes, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindBacklogItem})
	if err != nil {
		return fmt.Errorf("list backlog items: %w", err)
	}
	v.backlogs = make(map[string]*BacklogNode, len(bliRes.Objects))
	for _, obj := range bliRes.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		if id == "" {
			continue
		}
		title, _ := obj[objects.FieldKeyTitle].(string)
		status, _ := obj[objects.FieldKeyStatus].(string)
		planRef, _ := obj[objects.FieldKeyPriorityPlanRef].(string)
		personaRefs := lifecycle.StringRefsFromAny(obj[objects.FieldKeyPersonaRefs])
		v.backlogs[id] = &BacklogNode{
			ID:              id,
			Title:           title,
			Status:          status,
			PriorityPlanRef: planRef,
			PersonaRefs:     personaRefs,
		}
	}

	// 3. Scan Convergence Sessions
	cvsRes, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindConvergenceSession})
	if err == nil {
		v.cvsSessions = make(map[string]*CVSNode, len(cvsRes.Objects))
		for _, obj := range cvsRes.Objects {
			id, _ := obj[objects.FieldKeyID].(string)
			if id == "" {
				continue
			}
			st, _ := obj[objects.FieldKeyStatus].(string)
			ls := strings.ToLower(strings.TrimSpace(st))
			if ls != "active" && ls != "paused" {
				continue
			}
			title, _ := obj[objects.FieldKeyTitle].(string)
			phase, _ := obj[objects.FieldKeyCurrentPhase].(string)
			v.cvsSessions[id] = &CVSNode{
				ID:           id,
				Title:        title,
				CurrentPhase: phase,
				Status:       st,
			}
		}
	}

	// 4. Scan Agent Tasks
	taskRes, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindAgentTask})
	if err == nil {
		v.tasks = make(map[string]*TaskNode, len(taskRes.Objects))
		for _, obj := range taskRes.Objects {
			id, _ := obj[objects.FieldKeyID].(string)
			if id == "" {
				continue
			}
			title, _ := obj[objects.FieldKeyTitle].(string)
			status, _ := obj[objects.FieldKeyStatus].(string)
			assignee, _ := obj[objects.FieldKeyAssigneePersonaRef].(string)
			pRef, _ := obj[objects.FieldKeyPipelineRef].(string)
			v.tasks[id] = &TaskNode{
				ID:                 id,
				Title:              title,
				Status:             status,
				AssigneePersonaRef: assignee,
				PipelineRef:        pRef,
			}
		}
	}

	v.lastUpdated = time.Now()
	return nil
}

// ApplyLifecycleEvent incrementally updates the in-memory execution graph from a WAL event.
func (v *WhatsNextMaterializedView) ApplyLifecycleEvent(ev *lifecycle.LifecycleEvent) {
	if ev == nil {
		return
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	ts := ev.Ts
	if ts.IsZero() {
		ts = time.Now()
	}

	switch ev.EventType {
	case lifecycle.EventTypeStatusTransition:
		switch strings.ToLower(ev.Kind) {
		case objects.KindPriorityPlan:
			if p, exists := v.plans[ev.ID]; exists {
				p.Status = ev.ToStatus
			} else {
				v.plans[ev.ID] = &PlanNode{
					ID:     ev.ID,
					Status: ev.ToStatus,
				}
			}
			v.appendRecentEventLocked(WhatsNextEventSummary{
				Timestamp: ts,
				EventType: string(ev.EventType),
				ObjectID:  ev.ID,
				Kind:      objects.KindPriorityPlan,
				Message:   fmt.Sprintf("PLAN %s: %s -> %s", ev.ID, ev.FromStatus, ev.ToStatus),
			})

		case objects.KindBacklogItem:
			if b, exists := v.backlogs[ev.ID]; exists {
				b.Status = ev.ToStatus
			} else {
				v.backlogs[ev.ID] = &BacklogNode{
					ID:     ev.ID,
					Status: ev.ToStatus,
				}
			}
			v.appendRecentEventLocked(WhatsNextEventSummary{
				Timestamp: ts,
				EventType: string(ev.EventType),
				ObjectID:  ev.ID,
				Kind:      objects.KindBacklogItem,
				Message:   fmt.Sprintf("BLI %s: %s -> %s", ev.ID, ev.FromStatus, ev.ToStatus),
			})

		case objects.KindAgentTask:
			if t, exists := v.tasks[ev.ID]; exists {
				t.Status = ev.ToStatus
			} else {
				v.tasks[ev.ID] = &TaskNode{
					ID:     ev.ID,
					Status: ev.ToStatus,
				}
			}
			v.appendRecentEventLocked(WhatsNextEventSummary{
				Timestamp: ts,
				EventType: string(ev.EventType),
				ObjectID:  ev.ID,
				Kind:      objects.KindAgentTask,
				Message:   fmt.Sprintf("TASK %s: %s -> %s", ev.ID, ev.FromStatus, ev.ToStatus),
			})

		case objects.KindConvergenceSession:
			if c, exists := v.cvsSessions[ev.ID]; exists {
				c.Status = ev.ToStatus
			} else {
				v.cvsSessions[ev.ID] = &CVSNode{
					ID:     ev.ID,
					Status: ev.ToStatus,
				}
			}
		}
	}

	v.lastUpdated = time.Now()
}

func (v *WhatsNextMaterializedView) appendRecentEventLocked(ev WhatsNextEventSummary) {
	v.recentEvents = append(v.recentEvents, ev)
	if len(v.recentEvents) > 20 {
		v.recentEvents = v.recentEvents[len(v.recentEvents)-20:]
	}
}

// BuildPayloadLocked creates a snapshot payload from the current graph state.
func (v *WhatsNextMaterializedView) BuildPayloadLocked() *WhatsNextLitePayload {
	plansList := make([]*PlanNode, 0, len(v.plans))
	for _, p := range v.plans {
		plansList = append(plansList, p)
	}

	lead, ranked := EvaluatePhiLead(plansList, nil)

	activePlans := make([]WhatsNextPriorityPlan, 0, len(ranked))
	for _, p := range ranked {
		activePlans = append(activePlans, WhatsNextPriorityPlan{
			ID:     p.ID,
			Title:  p.Title,
			Status: p.Status,
		})
	}

	var leadPlanSumm *WhatsNextPriorityPlan
	if lead != nil {
		leadPlanSumm = &WhatsNextPriorityPlan{
			ID:     lead.ID,
			Title:  lead.Title,
			Status: lead.Status,
		}
	}

	backlogsList := make([]*BacklogNode, 0, len(v.backlogs))
	for _, b := range v.backlogs {
		backlogsList = append(backlogsList, b)
	}

	byPlan, total := EvaluatePhiCounts(backlogsList, nil, nil)

	tasksList := make([]*TaskNode, 0, len(v.tasks))
	for _, t := range v.tasks {
		tasksList = append(tasksList, t)
	}
	dispatch := EvaluatePhiDispatch(tasksList)

	depth := EvaluatePhiRunway(plansList)

	cvsList := make([]WhatsNextCVSRow, 0, len(v.cvsSessions))
	for _, c := range v.cvsSessions {
		cvsList = append(cvsList, WhatsNextCVSRow{
			ID:           c.ID,
			Title:        c.Title,
			CurrentPhase: c.CurrentPhase,
			Status:       c.Status,
		})
	}
	sort.Slice(cvsList, func(i, j int) bool {
		return cvsList[i].ID < cvsList[j].ID
	})

	instructionsByPlan := make(map[string]string)
	packagingCues := make(map[string]string)
	for _, p := range ranked {
		inst, cue := EvaluatePhiHunger(byPlan[p.ID], p)
		instructionsByPlan[p.ID] = inst
		packagingCues[p.ID] = cue
	}

	return &WhatsNextLitePayload{
		SchemaVersion:           MaterializedViewSchemaVersion,
		MaterializedAt:          v.lastUpdated,
		LeadPlan:                leadPlanSumm,
		ActivePlans:             activePlans,
		BacklogCountsByPlan:     byPlan,
		TotalBacklogCounts:      total,
		RunwayDepth:             depth,
		IsRunwayDepleted:        depth <= 1,
		ConvergenceSessions:     cvsList,
		ActiveTasksByAssignee:   dispatch,
		AgentInstructionsByPlan: instructionsByPlan,
		PackagingCuesByPlan:     packagingCues,
		RecentEvents:            v.recentEvents,
	}
}

// SaveToLiteFile atomically serializes the payload to .zqk/state/whats_next_lite.json via the accumulator Engine.
func (v *WhatsNextMaterializedView) SaveToLiteFile() error {
	if v.engine != nil {
		v.engine.SetLastUpdated(v.lastUpdated)
		return v.engine.SaveToLiteFile()
	}

	v.mu.RLock()
	payload := v.BuildPayloadLocked()
	v.mu.RUnlock()

	targetPath := WhatsNextLiteFilePath(v.projectRoot)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("mkdir .zqk/state: %w", err)
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal whats_next_lite: %w", err)
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", targetPath, time.Now().UnixNano())
	if err := fileutil.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("write temp whats_next_lite: %w", err)
	}

	if err := os.Rename(tmpFile, targetPath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("rename whats_next_lite: %w", err)
	}

	return nil
}

// LoadFromLiteFile reads the materialized payload directly from disk via the accumulator Engine.
func (v *WhatsNextMaterializedView) LoadFromLiteFile() (*WhatsNextLitePayload, error) {
	if v.engine != nil {
		env, err := v.engine.LoadFromLiteFile()
		if err != nil {
			return nil, err
		}
		if env == nil || env.Payload == nil {
			return nil, fmt.Errorf("empty payload in envelope")
		}
		v.mu.Lock()
		v.lastUpdated = env.MaterializedAt
		v.mu.Unlock()
		return env.Payload, nil
	}

	targetPath := WhatsNextLiteFilePath(v.projectRoot)
	data, err := fileutil.ReadFile(targetPath)
	if err != nil {
		return nil, err
	}

	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return nil, fmt.Errorf("unmarshal whats_next_lite: %w", err)
	}

	var payload *WhatsNextLitePayload
	var matAt time.Time

	if rawPayload, ok := rawMap[objects.FieldKeyPayload]; ok && len(rawPayload) > 0 && string(rawPayload) != "null" {
		var env accumulator.Envelope[*WhatsNextLitePayload]
		if err := json.Unmarshal(data, &env); err == nil && env.Payload != nil {
			payload = env.Payload
			matAt = env.MaterializedAt
		}
	}

	if payload == nil {
		var flat WhatsNextLitePayload
		if err := json.Unmarshal(data, &flat); err != nil {
			return nil, fmt.Errorf("unmarshal whats_next_lite: %w", err)
		}
		payload = &flat
		matAt = flat.MaterializedAt
		if matAt.IsZero() {
			if fi, err := os.Stat(targetPath); err == nil {
				matAt = fi.ModTime().UTC()
			}
		}
	}

	v.mu.Lock()
	v.lastUpdated = matAt
	v.mu.Unlock()

	return payload, nil
}

// ---------------------------------------------------------------------------
// Zero-Cost Hot Path & Async Recovery Circuit Breaker
// ---------------------------------------------------------------------------

// GetOrRecoverPayload implements the strict Zero-Hot-Path-Scan Invariant.
// Hot path reads the pre-computed lite file in sub-5ms.
// If missing or stale, it serves available state with degraded flags and dispatches
// a non-blocking background reconciler to rebuild without delaying the caller.
func GetOrRecoverPayload(ctx context.Context, sp storage.ObjectStorageProvider, projectRoot string, stalenessTolerance time.Duration) (*WhatsNextLitePayload, error) {
	if stalenessTolerance <= 0 {
		stalenessTolerance = DefaultStalenessTolerance
	}

	view := NewWhatsNextMaterializedView(projectRoot)
	payload, err := view.LoadFromLiteFile()
	if err != nil || payload == nil {
		bootstrap := &WhatsNextLitePayload{
			SchemaVersion:       MaterializedViewSchemaVersion,
			MaterializedAt:      time.Now(),
			Stale:               true,
			Recovering:          true,
			DegradedReason:      "cold_boot_materialized_projection_missing",
			BacklogCountsByPlan: make(map[string]map[string]int),
			TotalBacklogCounts:  make(map[string]int),
			ActivePlans:         make([]WhatsNextPriorityPlan, 0),
		}
		TriggerAsyncRebuild(projectRoot, sp)
		return bootstrap, nil
	}

	age := time.Since(payload.MaterializedAt)
	if age > stalenessTolerance {
		payload.Stale = true
		payload.Recovering = true
		payload.DegradedReason = fmt.Sprintf("materialized view watermark exceeds tolerance (%s > %s)", age.Round(time.Second), stalenessTolerance)
		TriggerAsyncRebuild(projectRoot, sp)
		return payload, nil
	}

	payload.Stale = false
	payload.Recovering = false
	payload.DegradedReason = ""
	return payload, nil
}

// TriggerAsyncRebuild launches a debounced background scan to rebuild the projection.
func TriggerAsyncRebuild(projectRoot string, sp storage.ObjectStorageProvider) bool {
	if sp == nil || projectRoot == "" {
		return false
	}

	if !atomic.CompareAndSwapUint32(&isReconciling, 0, 1) {
		return false
	}

	goroutinelabels.NewGoroutine("whatsnext-materialized-view-reconciler", "rebuilding whats-next projection").
		StartSimple(func() {
			defer atomic.StoreUint32(&isReconciling, 0)

			bgCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()

			secCtx := pkgctx.NewSystemSecurityContext()
			rebuildView := NewWhatsNextMaterializedView(projectRoot)
			if scanErr := rebuildView.ScanFromStorageWithSecurity(bgCtx, sp, secCtx); scanErr != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
					Error("async whats-next projection rebuild failed", scanErr).
					Log()
				return
			}

			if saveErr := rebuildView.SaveToLiteFile(); saveErr != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
					Error("async whats-next projection save failed", saveErr).
					Log()
				return
			}
		})

	return true
}

// SubscribeWAL starts an incremental background listener on lifecycle_events.wal.
func (v *WhatsNextMaterializedView) SubscribeWAL(ctx context.Context, updateCh chan<- struct{}) {
	if v.engine != nil {
		v.engine.SubscribeWAL(ctx, updateCh)
		return
	}

	wal, err := lifecycle.GetOrCreateLifecycleWAL(v.projectRoot)
	if err != nil {
		return
	}

	var cursor walutil.ReplayCursor
	pollInterval := 200 * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		newEvents := 0
		newCursor, err := wal.ReplayFromCursor(cursor, func(ev *lifecycle.LifecycleEvent) error {
			if ev == nil {
				return nil
			}
			newEvents++
			v.ApplyLifecycleEvent(ev)
			return nil
		})

		if err == nil {
			cursor = newCursor
		}

		if newEvents > 0 {
			_ = v.SaveToLiteFile()
			if updateCh != nil {
				select {
				case updateCh <- struct{}{}:
				default:
				}
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(pollInterval):
		}
	}
}

// StartBackgroundWALSubscriber helper to launch SubscribeWAL with proper goroutine tracking.
func (v *WhatsNextMaterializedView) StartBackgroundWALSubscriber(ctx context.Context, updateCh chan<- struct{}) {
	if v.engine != nil {
		v.engine.StartBackgroundWALSubscriber(ctx, updateCh)
		return
	}

	goroutinelabels.NewGoroutine("whatsnext-wal-subscriber", "subscribing to whats-next lifecycle events").
		StartSimple(func() {
			v.SubscribeWAL(ctx, updateCh)
		})
}

func init() {
	accumulator.RegisterSubscriber("whats_next", func(projectRoot string) accumulator.WALSubscriber {
		return NewWhatsNextMaterializedView(projectRoot)
	})
}
