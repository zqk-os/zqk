package tpm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/accumulator"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/walutil"
)

// StrategicReadinessLiteFilePath returns the canonical path for the materialized strategic readiness view.
func StrategicReadinessLiteFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, "strategic_readiness_lite.json")
}

// RequirementReadinessNode tracks the functional readiness and fuel status of a single requirement.
type RequirementReadinessNode struct {
	ID                   string   `json:"id"`
	Title                string   `json:"title"`
	Domain               string   `json:"domain"`
	Status               string   `json:"status"`
	CriteriaRefs         []string `json:"criteria_refs"`
	HasCriteria          bool     `json:"has_criteria"`           // Phi_req
	HasAttestationSuites bool     `json:"has_attestation_suites"` // Phi_trace
	HasExecutionLane     bool     `json:"has_execution_lane"`     // Phi_fuel
	PriorityPlanRef      string   `json:"priority_plan_ref,omitempty"`
	BacklogItemRef       string   `json:"backlog_item_ref,omitempty"`
	IsShovelReady        bool     `json:"is_shovel_ready"`
	IsFullyAligned       bool     `json:"is_fully_aligned"`
	MissingElements      []string `json:"missing_elements,omitempty"`
}

// DomainCluster aggregates requirements by architectural subsystem.
type DomainCluster struct {
	Domain               string                      `json:"domain"`
	TotalRequirements    int                         `json:"total_requirements"`
	AlignedRequirements  int                         `json:"aligned_requirements"`
	UnmappedRequirements int                         `json:"unmapped_requirements"`
	Requirements         []*RequirementReadinessNode `json:"requirements,omitempty"`
}

// SynthesisCandidate represents a bundle of unmapped requirements ready for autonomous priority plan staging.
type SynthesisCandidate struct {
	Domain             string   `json:"domain"`
	RequirementIDs     []string `json:"requirement_ids"`
	Title              string   `json:"title"`
	EstimatedBLIs      int      `json:"estimated_blis"`
	SuggestedPlanTitle string   `json:"suggested_plan_title"`
}

// StrategicLifecycleEventSummary records recent strategic state transitions.
type StrategicLifecycleEventSummary struct {
	Timestamp time.Time `json:"timestamp"`
	EventType string    `json:"event_type"`
	ObjectID  string    `json:"object_id,omitempty"`
	Kind      string    `json:"kind,omitempty"`
	Message   string    `json:"message"`
}

// StrategicReadinessLitePayload represents the zero-cost materialized state persisted to disk.
type StrategicReadinessLitePayload struct {
	SchemaVersion           string                               `json:"schema_version"`
	MaterializedAt          time.Time                            `json:"materialized_at"`
	RunwayDepth             int                                  `json:"runway_depth"`
	IsRunwayDepleted        bool                                 `json:"is_runway_depleted"`
	TotalActiveRequirements int                                  `json:"total_active_requirements"`
	AlignedRequirements     int                                  `json:"aligned_requirements"`
	GapRequirementsCount    int                                  `json:"gap_requirements_count"`
	DomainClusters          map[string]*DomainCluster            `json:"domain_clusters"`
	SynthesisCandidates     []*SynthesisCandidate                `json:"synthesis_candidates"`
	Requirements            map[string]*RequirementReadinessNode `json:"requirements,omitempty"`
	RecentEvents            []StrategicLifecycleEventSummary     `json:"recent_events,omitempty"`
	PlanStatuses            map[string]string                    `json:"plan_statuses,omitempty"`
}

// StrategicReadinessView maintains the in-memory graph of requirements, criteria, test cases, and priority plans.
type StrategicReadinessView struct {
	mu                  sync.RWMutex
	projectRoot         string
	requirements        map[string]*RequirementReadinessNode
	critToReqs          map[string][]string // criterionID -> []reqID
	critToTests         map[string][]string // criterionID -> []testID
	reqToBLIs           map[string][]string // reqID -> []bliID
	bliToPlan           map[string]string   // bliID -> planID
	planStatuses        map[string]string   // planID -> status
	domainClusters      map[string]*DomainCluster
	synthesisCandidates []*SynthesisCandidate
	recentEvents        []StrategicLifecycleEventSummary
	lastUpdated         time.Time
	engine              *accumulator.Engine[*StrategicReadinessLitePayload]
}

// NewStrategicReadinessView initializes an empty in-memory strategic view.
func NewStrategicReadinessView(projectRoot string) *StrategicReadinessView {
	view := &StrategicReadinessView{
		projectRoot:         projectRoot,
		requirements:        make(map[string]*RequirementReadinessNode),
		critToReqs:          make(map[string][]string),
		critToTests:         make(map[string][]string),
		reqToBLIs:           make(map[string][]string),
		bliToPlan:           make(map[string]string),
		planStatuses:        make(map[string]string),
		domainClusters:      make(map[string]*DomainCluster),
		synthesisCandidates: make([]*SynthesisCandidate, 0),
		recentEvents:        make([]StrategicLifecycleEventSummary, 0, 20),
		lastUpdated:         time.Now(),
	}
	spec := accumulator.AccumulatorSpec{
		Name:               "strategic_readiness",
		SchemaVersion:      "1.0.0",
		ProjectRoot:        projectRoot,
		StoragePath:        StrategicReadinessLiteFilePath(projectRoot),
		StalenessTolerance: 5 * time.Minute,
	}
	eng, _ := accumulator.NewEngine[*StrategicReadinessLitePayload](spec, view)
	view.engine = eng
	return view
}

// Name satisfies accumulator.Accumulator interface.
func (v *StrategicReadinessView) Name() string {
	return "strategic_readiness"
}

// DefaultPayload satisfies accumulator.Accumulator interface.
func (v *StrategicReadinessView) DefaultPayload() *StrategicReadinessLitePayload {
	return &StrategicReadinessLitePayload{
		SchemaVersion:       "1.0.0",
		MaterializedAt:      time.Now().UTC(),
		DomainClusters:      make(map[string]*DomainCluster),
		SynthesisCandidates: make([]*SynthesisCandidate, 0),
		RecentEvents:        make([]StrategicLifecycleEventSummary, 0),
	}
}

// ApplyEvent satisfies accumulator.Accumulator interface.
func (v *StrategicReadinessView) ApplyEvent(ev *lifecycle.LifecycleEvent) bool {
	v.ApplyLifecycleEvent(ev)
	return true
}

// BuildPayload satisfies accumulator.Accumulator interface.
func (v *StrategicReadinessView) BuildPayload() *StrategicReadinessLitePayload {
	return v.buildPayloadLocked()
}

// ScanFromStorage satisfies accumulator.Accumulator interface.
func (v *StrategicReadinessView) ScanFromStorage(ctx context.Context, sp storage.ObjectStorageProvider) error {
	secCtx := pkgctx.NewSystemSecurityContext()
	return v.ScanFromStorageWithSecurity(ctx, sp, secCtx)
}

// Engine returns the underlying generic accumulator engine.
func (v *StrategicReadinessView) Engine() *accumulator.Engine[*StrategicReadinessLitePayload] {
	return v.engine
}

// EvaluatePhiReq evaluates Requirement Completeness: len(criteria_refs) > 0 and status is active.
func EvaluatePhiReq(req *RequirementReadinessNode) bool {
	if req == nil {
		return false
	}
	return len(req.CriteriaRefs) > 0 && strings.EqualFold(req.Status, objects.ObjectStatusActive)
}

// EvaluatePhiTrace evaluates Traceability Integrity: all criteria have at least one test case binding.
func EvaluatePhiTrace(req *RequirementReadinessNode, critToTests map[string][]string) bool {
	if req == nil || len(req.CriteriaRefs) == 0 {
		return false
	}
	for _, critID := range req.CriteriaRefs {
		if len(critToTests[critID]) == 0 {
			return false
		}
	}
	return true
}

// EvaluatePhiFuel evaluates Execution Fuel: requirement is linked to an active/shovel_ready/grooming priority plan via a BLI.
func EvaluatePhiFuel(reqID string, reqToBLIs map[string][]string, bliToPlan map[string]string, planStatuses map[string]string) (bool, string, string) {
	blis, ok := reqToBLIs[reqID]
	if !ok || len(blis) == 0 {
		return false, "", ""
	}
	for _, bliID := range blis {
		planID, ok := bliToPlan[bliID]
		if !ok || planID == "" {
			continue
		}
		status, ok := planStatuses[planID]
		if !ok {
			continue
		}
		switch strings.ToLower(status) {
		case objects.ObjectStatusActive, objects.ObjectStatusInProgress, "shovel_ready", "grooming":
			return true, planID, bliID
		}
	}
	return false, "", ""
}

// EvaluateRunwayDepth counts priority plans currently in active, in_progress, shovel_ready, or grooming states.
func EvaluateRunwayDepth(planStatuses map[string]string) int {
	depth := 0
	for _, st := range planStatuses {
		switch strings.ToLower(st) {
		case objects.ObjectStatusActive, objects.ObjectStatusInProgress, "shovel_ready", objects.ObjectStatusGrooming:
			depth++
		}
	}
	return depth
}

// IsRunwayDepleted checks if the execution runway is at or below the replenishment threshold (<= 1).
func IsRunwayDepleted(depth int) bool {
	return depth <= 1
}

// DetectDomain classifies a requirement into an architectural domain based on ID, title, and keywords.
func DetectDomain(id, title string) string {
	lower := strings.ToLower(id + " " + title)
	switch {
	case strings.Contains(lower, "storage") || strings.Contains(lower, "cas") || strings.Contains(lower, "git") || strings.Contains(lower, "lock"):
		return "storage"
	case strings.Contains(lower, "scheduler") || strings.Contains(lower, "daemon") || strings.Contains(lower, "cron") || strings.Contains(lower, "retention"):
		return "scheduler"
	case strings.Contains(lower, "mcp") || strings.Contains(lower, "mesh") || strings.Contains(lower, "agent") || strings.Contains(lower, "feed"):
		return "mesh_agent"
	case strings.Contains(lower, "cap") || strings.Contains(lower, "orchestrat") || strings.Contains(lower, "whats-next") || strings.Contains(lower, "tpm") || strings.Contains(lower, "plan"):
		return "tpm_orchestration"
	case strings.Contains(lower, "doc") || strings.Contains(lower, "policy") || strings.Contains(lower, "pol-") || strings.Contains(lower, "audit") || strings.Contains(lower, "governance"):
		return "governance"
	case strings.Contains(lower, "perf") || strings.Contains(lower, "metric") || strings.Contains(lower, "timeseries") || strings.Contains(lower, "cache"):
		return "performance"
	case strings.Contains(lower, "test") || strings.Contains(lower, "trace") || strings.Contains(lower, "attest") || strings.Contains(lower, "criteria"):
		return "qa_traceability"
	default:
		return "core_kernel"
	}
}

// LoadFromLiteFile reads the materialized strategic readiness payload from disk via accumulator Engine.
func (v *StrategicReadinessView) LoadFromLiteFile() (bool, error) {
	if v.engine != nil {
		env, err := v.engine.LoadFromLiteFile()
		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, fmt.Errorf("read strategic readiness lite file: %w", err)
		}
		if env == nil || env.Payload == nil {
			return false, nil
		}
		payload := env.Payload
		v.mu.Lock()
		defer v.mu.Unlock()

		v.domainClusters = payload.DomainClusters
		if v.domainClusters == nil {
			v.domainClusters = make(map[string]*DomainCluster)
		}
		v.synthesisCandidates = payload.SynthesisCandidates
		if v.synthesisCandidates == nil {
			v.synthesisCandidates = make([]*SynthesisCandidate, 0)
		}
		v.recentEvents = payload.RecentEvents
		if payload.Requirements != nil {
			v.requirements = payload.Requirements
		}
		v.planStatuses = payload.PlanStatuses
		if v.planStatuses == nil {
			v.planStatuses = make(map[string]string)
		}
		v.lastUpdated = env.MaterializedAt
		return true, nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	litePath := StrategicReadinessLiteFilePath(v.projectRoot)
	data, err := fileutil.ReadFile(litePath)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read strategic readiness lite file: %w", err)
	}

	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return false, fmt.Errorf("unmarshal strategic readiness lite file: %w", err)
	}

	var payload *StrategicReadinessLitePayload
	var matAt time.Time

	if rawPayload, ok := rawMap[objects.FieldKeyPayload]; ok && len(rawPayload) > 0 && string(rawPayload) != "null" {
		var env accumulator.Envelope[*StrategicReadinessLitePayload]
		if err := json.Unmarshal(data, &env); err == nil && env.Payload != nil {
			payload = env.Payload
			matAt = env.MaterializedAt
		}
	}

	if payload == nil {
		var flat StrategicReadinessLitePayload
		if err := json.Unmarshal(data, &flat); err != nil {
			return false, fmt.Errorf("unmarshal strategic readiness payload: %w", err)
		}
		payload = &flat
		matAt = flat.MaterializedAt
		if matAt.IsZero() {
			if fi, err := fileutil.Stat(litePath); err == nil {
				matAt = fi.ModTime().UTC()
			}
		}
	}

	v.domainClusters = payload.DomainClusters
	if v.domainClusters == nil {
		v.domainClusters = make(map[string]*DomainCluster)
	}
	v.synthesisCandidates = payload.SynthesisCandidates
	if v.synthesisCandidates == nil {
		v.synthesisCandidates = make([]*SynthesisCandidate, 0)
	}
	v.recentEvents = payload.RecentEvents
	if payload.Requirements != nil {
		v.requirements = payload.Requirements
	}
	v.planStatuses = payload.PlanStatuses
	if v.planStatuses == nil {
		v.planStatuses = make(map[string]string)
	}
	v.lastUpdated = matAt
	return true, nil
}

// SaveToLiteFile atomically flushes the current in-memory view to .zqk/state/strategic_readiness_lite.json via accumulator Engine.
func (v *StrategicReadinessView) SaveToLiteFile() error {
	if v.engine != nil {
		v.engine.SetLastUpdated(v.lastUpdated)
		return v.engine.SaveToLiteFile()
	}

	v.mu.RLock()
	payload := v.buildPayloadLocked()
	v.mu.RUnlock()

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal strategic readiness payload: %w", err)
	}

	stateDir := filepath.Join(v.projectRoot, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.MkdirAll(stateDir, paths.DirPerm755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	litePath := StrategicReadinessLiteFilePath(v.projectRoot)
	tmpPath := litePath + ".tmp"
	if err := fileutil.WriteFile(tmpPath, data, paths.FilePerm644); err != nil {
		return fmt.Errorf("write tmp strategic readiness file: %w", err)
	}
	return fileutil.Rename(tmpPath, litePath)
}

func (v *StrategicReadinessView) buildPayloadLocked() *StrategicReadinessLitePayload {
	runwayDepth := EvaluateRunwayDepth(v.planStatuses)
	totalActive := 0
	aligned := 0
	gapCount := 0

	for _, req := range v.requirements {
		if strings.EqualFold(req.Status, objects.ObjectStatusActive) {
			totalActive++
			if req.IsFullyAligned {
				aligned++
			} else {
				gapCount++
			}
		}
	}

	return &StrategicReadinessLitePayload{
		SchemaVersion:           "1.0.0",
		MaterializedAt:          time.Now().UTC(),
		RunwayDepth:             runwayDepth,
		IsRunwayDepleted:        IsRunwayDepleted(runwayDepth),
		TotalActiveRequirements: totalActive,
		AlignedRequirements:     aligned,
		GapRequirementsCount:    gapCount,
		DomainClusters:          v.domainClusters,
		SynthesisCandidates:     v.synthesisCandidates,
		Requirements:            v.requirements,
		RecentEvents:            v.recentEvents,
		PlanStatuses:            v.planStatuses,
	}
}

// ScanFromStorageWithSecurity performs a full load from the Knowledge Kernel storage and re-evaluates all predicates.
func (v *StrategicReadinessView) ScanFromStorageWithSecurity(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *storage.SecurityContext) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// 1. Load priority plans (non-archived)
	planRes, err := sp.List(ctx, secCtx, nil, storage.DefaultQueryFactory.
		NotArchived(objects.KindPriorityPlan).
		IncludeFields(objects.FieldKeyID, objects.FieldKeyStatus).
		Build())
	if err != nil {
		return fmt.Errorf("list priority plans: %w", err)
	}
	v.planStatuses = make(map[string]string, len(planRes.Objects))
	for _, p := range planRes.Objects {
		id, _ := p[objects.FieldKeyID].(string)
		status, _ := p[objects.FieldKeyStatus].(string)
		if id != "" {
			v.planStatuses[id] = status
		}
	}

	// 2. Load backlog items (non-archived)
	bliRes, err := sp.List(ctx, secCtx, nil, storage.DefaultQueryFactory.
		NotArchived(objects.KindBacklogItem).
		IncludeFields(objects.FieldKeyID, objects.FieldKeyPriorityPlanRef, objects.FieldKeyRequirementRefs).
		Build())
	if err != nil {
		return fmt.Errorf("list backlog items: %w", err)
	}
	v.reqToBLIs = make(map[string][]string)
	v.bliToPlan = make(map[string]string)
	for _, b := range bliRes.Objects {
		bID, _ := b[objects.FieldKeyID].(string)
		pRef, _ := b[objects.FieldKeyPriorityPlanRef].(string)
		if bID != "" && pRef != "" {
			v.bliToPlan[bID] = pRef
		}
		reqRefs := lifecycle.StringRefsFromAny(b[objects.FieldKeyRequirementRefs])
		for _, rID := range reqRefs {
			v.reqToBLIs[rID] = append(v.reqToBLIs[rID], bID)
		}
	}

	// 3. Load test cases (non-archived)
	tcRes, err := sp.List(ctx, secCtx, nil, storage.DefaultQueryFactory.
		NotArchived(objects.KindTestCase).
		IncludeFields(objects.FieldKeyID, objects.FieldKeyCriteriaRefs).
		Build())
	if err != nil {
		return fmt.Errorf("list test cases: %w", err)
	}
	v.critToTests = make(map[string][]string)
	for _, tc := range tcRes.Objects {
		tcID, _ := tc[objects.FieldKeyID].(string)
		cRefs := lifecycle.StringRefsFromAny(tc[objects.FieldKeyCriteriaRefs])
		for _, cID := range cRefs {
			v.critToTests[cID] = append(v.critToTests[cID], tcID)
		}
	}

	// 4. Load requirements (non-archived)
	reqRes, err := sp.List(ctx, secCtx, nil, storage.DefaultQueryFactory.
		NotArchived(objects.KindRequirement).
		IncludeFields(objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus, objects.FieldKeyCriteriaRefs).
		Build())
	if err != nil {
		return fmt.Errorf("list requirements: %w", err)
	}
	v.requirements = make(map[string]*RequirementReadinessNode, len(reqRes.Objects))
	v.critToReqs = make(map[string][]string)

	for _, r := range reqRes.Objects {
		rID, _ := r[objects.FieldKeyID].(string)
		title, _ := r[objects.FieldKeyTitle].(string)
		status, _ := r[objects.FieldKeyStatus].(string)
		cRefs := lifecycle.StringRefsFromAny(r[objects.FieldKeyCriteriaRefs])

		for _, cID := range cRefs {
			v.critToReqs[cID] = append(v.critToReqs[cID], rID)
		}

		node := &RequirementReadinessNode{
			ID:           rID,
			Title:        title,
			Domain:       DetectDomain(rID, title),
			Status:       status,
			CriteriaRefs: cRefs,
		}
		v.requirements[rID] = node
	}

	// 5. Evaluate predicates across all requirements
	v.evaluateAllPredicatesLocked()
	v.clusterAndSynthesizeLocked()
	v.lastUpdated = time.Now()
	return nil
}

// evaluateAllPredicatesLocked calculates Phi_req, Phi_trace, and Phi_fuel for all nodes.
func (v *StrategicReadinessView) evaluateAllPredicatesLocked() {
	for _, req := range v.requirements {
		v.evaluateSingleRequirementLocked(req)
	}
}

func (v *StrategicReadinessView) evaluateSingleRequirementLocked(req *RequirementReadinessNode) {
	req.HasCriteria = EvaluatePhiReq(req)
	req.HasAttestationSuites = EvaluatePhiTrace(req, v.critToTests)
	isFueled, planID, bliID := EvaluatePhiFuel(req.ID, v.reqToBLIs, v.bliToPlan, v.planStatuses)
	req.HasExecutionLane = isFueled
	req.PriorityPlanRef = planID
	req.BacklogItemRef = bliID

	req.IsShovelReady = req.HasCriteria && req.HasAttestationSuites
	req.IsFullyAligned = req.IsShovelReady && req.HasExecutionLane

	var missing []string
	if !req.HasCriteria {
		missing = append(missing, "missing_criteria")
	}
	if !req.HasAttestationSuites {
		missing = append(missing, "missing_test_attestation")
	}
	if !req.HasExecutionLane {
		missing = append(missing, "unmapped_execution_lane")
	}
	req.MissingElements = missing
}

// clusterAndSynthesizeLocked groups requirements by domain and derives synthesis candidates when runway is depleted.
func (v *StrategicReadinessView) clusterAndSynthesizeLocked() {
	v.domainClusters = make(map[string]*DomainCluster)
	for _, req := range v.requirements {
		if !strings.EqualFold(req.Status, objects.ObjectStatusActive) {
			continue
		}
		cluster, ok := v.domainClusters[req.Domain]
		if !ok {
			cluster = &DomainCluster{
				Domain:       req.Domain,
				Requirements: make([]*RequirementReadinessNode, 0),
			}
			v.domainClusters[req.Domain] = cluster
		}
		cluster.TotalRequirements++
		if req.IsFullyAligned {
			cluster.AlignedRequirements++
		} else {
			cluster.UnmappedRequirements++
		}
		cluster.Requirements = append(cluster.Requirements, req)
	}

	// Synthesis Candidates: For unmapped active requirements meeting synthesis threshold
	v.synthesisCandidates = make([]*SynthesisCandidate, 0)
	runwayDepth := EvaluateRunwayDepth(v.planStatuses)

	// Sort domain keys for deterministic ordering
	domainKeys := make([]string, 0, len(v.domainClusters))
	for k := range v.domainClusters {
		domainKeys = append(domainKeys, k)
	}
	sort.Strings(domainKeys)

	for _, dom := range domainKeys {
		cluster := v.domainClusters[dom]
		var unmappedIDs []string
		for _, r := range cluster.Requirements {
			if !r.HasExecutionLane {
				unmappedIDs = append(unmappedIDs, r.ID)
			}
		}
		if len(unmappedIDs) > 0 {
			candidate := &SynthesisCandidate{
				Domain:             dom,
				RequirementIDs:     unmappedIDs,
				Title:              fmt.Sprintf("Autonomous Runway Replenishment: %s Subsystem", strings.Title(strings.ReplaceAll(dom, "_", " "))),
				EstimatedBLIs:      len(unmappedIDs),
				SuggestedPlanTitle: fmt.Sprintf("Autonomous %s Alignment & Stabilization Runway", strings.Title(strings.ReplaceAll(dom, "_", " "))),
			}
			v.synthesisCandidates = append(v.synthesisCandidates, candidate)
		}
	}

	_ = runwayDepth
}

// ApplyLifecycleEvent performs an incremental O(degree) in-memory delta update without storage scans.
func (v *StrategicReadinessView) ApplyLifecycleEvent(ev *lifecycle.LifecycleEvent) {
	if ev == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()

	ts := ev.Timestamp()

	switch ev.EventType {
	case lifecycle.EventTypeCriterionSatisfied:
		// Criterion satisfied: check affected requirements
		critID := ev.CriterionID
		if reqIDs, ok := v.critToReqs[critID]; ok {
			for _, rID := range reqIDs {
				if req, exists := v.requirements[rID]; exists {
					v.evaluateSingleRequirementLocked(req)
				}
			}
		}
		v.appendRecentEventLocked(StrategicLifecycleEventSummary{
			Timestamp: ts,
			EventType: string(ev.EventType),
			ObjectID:  critID,
			Kind:      objects.KindCriteria,
			Message:   fmt.Sprintf("CRITERION SATISFIED: %s", critID),
		})

	case lifecycle.EventTypeStatusTransition:
		switch strings.ToLower(ev.Kind) {
		case objects.KindRequirement:
			rID := ev.ID
			if req, exists := v.requirements[rID]; exists {
				req.Status = ev.ToStatus
				v.evaluateSingleRequirementLocked(req)
			}
			v.appendRecentEventLocked(StrategicLifecycleEventSummary{
				Timestamp: ts,
				EventType: string(ev.EventType),
				ObjectID:  rID,
				Kind:      objects.KindRequirement,
				Message:   fmt.Sprintf("REQUIREMENT %s: %s -> %s", rID, ev.FromStatus, ev.ToStatus),
			})

		case objects.KindPriorityPlan:
			pID := ev.ID
			v.planStatuses[pID] = ev.ToStatus
			// Re-evaluate fuel for affected requirements
			for _, req := range v.requirements {
				if req.PriorityPlanRef == pID || !req.HasExecutionLane {
					v.evaluateSingleRequirementLocked(req)
				}
			}
			v.appendRecentEventLocked(StrategicLifecycleEventSummary{
				Timestamp: ts,
				EventType: string(ev.EventType),
				ObjectID:  pID,
				Kind:      objects.KindPriorityPlan,
				Message:   fmt.Sprintf("PLAN %s: %s -> %s", pID, ev.FromStatus, ev.ToStatus),
			})
		}
	}

	v.clusterAndSynthesizeLocked()
	v.lastUpdated = time.Now()
}

func (v *StrategicReadinessView) appendRecentEventLocked(ev StrategicLifecycleEventSummary) {
	v.recentEvents = append(v.recentEvents, ev)
	if len(v.recentEvents) > 20 {
		v.recentEvents = v.recentEvents[len(v.recentEvents)-20:]
	}
}

// CheckRunwayReplenishment returns true and the synthesis candidate bundles if the runway watermark <= 1.
func (v *StrategicReadinessView) CheckRunwayReplenishment() (bool, []*SynthesisCandidate) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	depth := EvaluateRunwayDepth(v.planStatuses)
	if IsRunwayDepleted(depth) && len(v.synthesisCandidates) > 0 {
		return true, v.synthesisCandidates
	}
	return false, nil
}

// GetSnapshot returns a copy of the materialized payload.
func (v *StrategicReadinessView) GetSnapshot() *StrategicReadinessLitePayload {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.buildPayloadLocked()
}

// SubscribeWAL starts a background listener on the lifecycle WAL for continuous incremental updates.
func (v *StrategicReadinessView) SubscribeWAL(ctx context.Context, updateCh chan<- struct{}) {
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
func (v *StrategicReadinessView) StartBackgroundWALSubscriber(ctx context.Context, updateCh chan<- struct{}) {
	goroutinelabels.NewGoroutine("strategic-readiness-wal-subscriber", "subscribing to strategic lifecycle events").
		StartSimple(func() {
			v.SubscribeWAL(ctx, updateCh)
		})
}

func init() {
	accumulator.RegisterSubscriber("strategic_readiness", func(projectRoot string) accumulator.WALSubscriber {
		return NewStrategicReadinessView(projectRoot)
	})
}

// EvaluateAndReplenishRunway loads the strategic readiness view, refreshes from storage if needed,
// persists the lite file, and returns whether the runway is depleted (depth <= 1) along with synthesis candidates.
func EvaluateAndReplenishRunway(ctx context.Context, sp storage.ObjectStorageProvider, projectRoot string) (*StrategicReadinessLitePayload, []*SynthesisCandidate, error) {
	view := NewStrategicReadinessView(projectRoot)
	loaded, err := view.LoadFromLiteFile()
	if err != nil || !loaded || len(view.planStatuses) == 0 || time.Since(view.lastUpdated) > 5*time.Minute {
		secCtx := pkgctx.NewSystemSecurityContext()
		if scanErr := view.ScanFromStorageWithSecurity(ctx, sp, secCtx); scanErr != nil {
			return nil, nil, fmt.Errorf("scan strategic readiness from storage: %w", scanErr)
		}
		_ = view.SaveToLiteFile()
	}
	depleted, candidates := view.CheckRunwayReplenishment()
	snap := view.GetSnapshot()
	if depleted {
		return snap, candidates, nil
	}
	return snap, nil, nil
}
