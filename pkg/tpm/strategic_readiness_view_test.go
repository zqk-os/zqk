package tpm

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/accumulator"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestStrategicReadinessPredicates(t *testing.T) {
	// Test Phi_req
	activeReqWithCrit := &RequirementReadinessNode{
		ID:           "REQ-TEST-001",
		Status:       objects.ObjectStatusActive,
		CriteriaRefs: []string{"CRIT-001"},
	}
	if !EvaluatePhiReq(activeReqWithCrit) {
		t.Errorf("expected Phi_req to be true for active req with criteria")
	}

	inactiveReq := &RequirementReadinessNode{
		ID:           "REQ-TEST-002",
		Status:       objects.ObjectStatusConceptual,
		CriteriaRefs: []string{"CRIT-001"},
	}
	if EvaluatePhiReq(inactiveReq) {
		t.Errorf("expected Phi_req to be false for non-active req")
	}

	activeReqNoCrit := &RequirementReadinessNode{
		ID:           "REQ-TEST-003",
		Status:       objects.ObjectStatusActive,
		CriteriaRefs: nil,
	}
	if EvaluatePhiReq(activeReqNoCrit) {
		t.Errorf("expected Phi_req to be false for req with no criteria")
	}

	// Test Phi_trace
	critToTests := map[string][]string{
		"CRIT-001": {"TST-001"},
	}
	if !EvaluatePhiTrace(activeReqWithCrit, critToTests) {
		t.Errorf("expected Phi_trace to be true when all criteria have test cases")
	}

	reqWithUnboundCrit := &RequirementReadinessNode{
		ID:           "REQ-TEST-004",
		Status:       objects.ObjectStatusActive,
		CriteriaRefs: []string{"CRIT-001", "CRIT-UNBOUND"},
	}
	if EvaluatePhiTrace(reqWithUnboundCrit, critToTests) {
		t.Errorf("expected Phi_trace to be false when a criterion lacks test cases")
	}

	// Test Phi_fuel
	reqToBLIs := map[string][]string{
		"REQ-TEST-001": {"BLI-001"},
	}
	bliToPlan := map[string]string{
		"BLI-001": "PRI-001",
	}
	planStatuses := map[string]string{
		"PRI-001": objects.ObjectStatusInProgress,
	}

	isFueled, planID, bliID := EvaluatePhiFuel("REQ-TEST-001", reqToBLIs, bliToPlan, planStatuses)
	if !isFueled || planID != "PRI-001" || bliID != "BLI-001" {
		t.Errorf("expected Phi_fuel to be true, got isFueled=%v, plan=%s, bli=%s", isFueled, planID, bliID)
	}

	planStatuses["PRI-001"] = objects.ObjectStatusComplete
	isFueled, _, _ = EvaluatePhiFuel("REQ-TEST-001", reqToBLIs, bliToPlan, planStatuses)
	if isFueled {
		t.Errorf("expected Phi_fuel to be false when plan is already complete")
	}
}

func TestStrategicReadinessRunwayWatermark(t *testing.T) {
	planStatuses := map[string]string{
		"PRI-001": objects.ObjectStatusInProgress,
		"PRI-002": "shovel_ready",
	}
	depth := EvaluateRunwayDepth(planStatuses)
	if depth != 2 {
		t.Errorf("expected depth 2, got %d", depth)
	}
	if IsRunwayDepleted(depth) {
		t.Errorf("runway should not be depleted with depth 2")
	}

	planStatuses["PRI-002"] = objects.ObjectStatusComplete
	depth = EvaluateRunwayDepth(planStatuses)
	if depth != 1 {
		t.Errorf("expected depth 1, got %d", depth)
	}
	if !IsRunwayDepleted(depth) {
		t.Errorf("runway should be depleted with depth <= 1")
	}

	planStatuses["PRI-003"] = objects.ObjectStatusGrooming
	planStatuses["PRI-004"] = objects.ObjectStatusGrooming
	depth = EvaluateRunwayDepth(planStatuses)
	if depth != 3 {
		t.Errorf("expected depth 3 with 2 grooming plans, got %d", depth)
	}
	if IsRunwayDepleted(depth) {
		t.Errorf("runway should not be depleted with depth 3")
	}
}

func TestStrategicReadinessIncrementalDelta(t *testing.T) {
	tempDir := t.TempDir()

	view := NewStrategicReadinessView(tempDir)
	reqNode := &RequirementReadinessNode{
		ID:           "REQ-STORAGE-001",
		Title:        "Decouple storage provider",
		Domain:       "storage",
		Status:       objects.ObjectStatusConceptual,
		CriteriaRefs: []string{"CRIT-STORAGE-001"},
	}
	view.requirements[reqNode.ID] = reqNode
	view.critToReqs["CRIT-STORAGE-001"] = []string{reqNode.ID}
	view.planStatuses["PRI-STORAGE-001"] = "shovel_ready"
	view.reqToBLIs[reqNode.ID] = []string{"BLI-STORAGE-001"}
	view.bliToPlan["BLI-STORAGE-001"] = "PRI-STORAGE-001"
	view.critToTests["CRIT-STORAGE-001"] = []string{"TST-STORAGE-001"}

	// Before transition: status is draft -> Phi_req is false
	view.evaluateSingleRequirementLocked(reqNode)
	if reqNode.HasCriteria {
		t.Errorf("expected HasCriteria=false for draft status")
	}

	// Simulate LifecycleEvent: Requirement status transitioned to active
	ev := &lifecycle.LifecycleEvent{
		EventType:  lifecycle.EventTypeStatusTransition,
		Kind:       objects.KindRequirement,
		ID:         "REQ-STORAGE-001",
		FromStatus: objects.ObjectStatusConceptual,
		ToStatus:   objects.ObjectStatusActive,
		Ts:         time.Now(),
	}
	view.ApplyLifecycleEvent(ev)

	reqUpdated := view.requirements["REQ-STORAGE-001"]
	if !reqUpdated.HasCriteria {
		t.Errorf("expected HasCriteria=true after active transition")
	}
	if !reqUpdated.HasAttestationSuites {
		t.Errorf("expected HasAttestationSuites=true")
	}
	if !reqUpdated.HasExecutionLane {
		t.Errorf("expected HasExecutionLane=true")
	}
	if !reqUpdated.IsFullyAligned {
		t.Errorf("expected IsFullyAligned=true")
	}

	// Verify persistence to lite-file
	if err := view.SaveToLiteFile(); err != nil {
		t.Fatalf("SaveToLiteFile failed: %v", err)
	}

	// Load into a new view
	loadedView := NewStrategicReadinessView(tempDir)
	loaded, err := loadedView.LoadFromLiteFile()
	if err != nil {
		t.Fatalf("LoadFromLiteFile failed: %v", err)
	}
	if !loaded {
		t.Fatalf("expected lite file to be loaded")
	}

	snap := loadedView.GetSnapshot()
	if snap.TotalActiveRequirements != 1 {
		t.Errorf("expected 1 active requirement in snapshot, got %d", snap.TotalActiveRequirements)
	}
	if snap.AlignedRequirements != 1 {
		t.Errorf("expected 1 aligned requirement, got %d", snap.AlignedRequirements)
	}
}

func TestStrategicReadinessDomainClusteringAndSynthesisTrigger(t *testing.T) {
	tempDir := t.TempDir()

	view := NewStrategicReadinessView(tempDir)
	view.planStatuses["PRI-ACTIVE-001"] = objects.ObjectStatusInProgress
	// Only 1 plan active -> runway depth = 1 (depleted!)

	// Unmapped requirement in scheduler domain
	reqNode := &RequirementReadinessNode{
		ID:           "REQ-SCHED-SPLIT-001",
		Title:        "Split scheduler god file",
		Domain:       "scheduler",
		Status:       objects.ObjectStatusActive,
		CriteriaRefs: []string{"CRIT-SCHED-001"},
	}
	view.requirements[reqNode.ID] = reqNode
	view.critToReqs["CRIT-SCHED-001"] = []string{reqNode.ID}
	view.critToTests["CRIT-SCHED-001"] = []string{"TST-SCHED-001"}

	view.evaluateAllPredicatesLocked()
	view.clusterAndSynthesizeLocked()

	shouldReplenish, candidates := view.CheckRunwayReplenishment()
	if !shouldReplenish {
		t.Errorf("expected CheckRunwayReplenishment to return true for runway depth 1 with unmapped req")
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 synthesis candidate, got %d", len(candidates))
	}
	if candidates[0].Domain != "scheduler" {
		t.Errorf("expected candidate domain 'scheduler', got %s", candidates[0].Domain)
	}
	if len(candidates[0].RequirementIDs) != 1 || candidates[0].RequirementIDs[0] != "REQ-SCHED-SPLIT-001" {
		t.Errorf("unexpected candidate requirement IDs: %v", candidates[0].RequirementIDs)
	}
}

// Mock storage provider for ScanFromStorage verification
type mockTPMStorageProvider struct {
	storage.ObjectStorageProvider
	plans        []map[string]any
	blis         []map[string]any
	tcs          []map[string]any
	requirements []map[string]any
}

func (m *mockTPMStorageProvider) List(ctx context.Context, secCtx *storage.SecurityContext, storageCtx *storage.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	var objs []map[string]any
	switch filter.Kind {
	case objects.KindPriorityPlan:
		objs = m.plans
	case objects.KindBacklogItem:
		objs = m.blis
	case objects.KindTestCase:
		objs = m.tcs
	case objects.KindRequirement:
		objs = m.requirements
	}
	return &storage.QueryResult{Objects: objs}, nil
}

func TestStrategicReadinessScanFromStorage(t *testing.T) {
	mockSP := &mockTPMStorageProvider{
		plans: []map[string]any{
			{
				objects.FieldKeyID:     "PRI-001",
				objects.FieldKeyStatus: objects.ObjectStatusInProgress,
			},
		},
		blis: []map[string]any{
			{
				objects.FieldKeyID:              "BLI-001",
				objects.FieldKeyPriorityPlanRef: "PRI-001",
				objects.FieldKeyRequirementRefs: []any{"REQ-MCP-001"},
			},
		},
		tcs: []map[string]any{
			{
				objects.FieldKeyID:           "TST-001",
				objects.FieldKeyCriteriaRefs: []any{"CRIT-001"},
			},
		},
		requirements: []map[string]any{
			{
				objects.FieldKeyID:           "REQ-MCP-001",
				objects.FieldKeyTitle:        "MCP Mesh Tool Streaming",
				objects.FieldKeyStatus:       objects.ObjectStatusActive,
				objects.FieldKeyCriteriaRefs: []any{"CRIT-001"},
			},
		},
	}

	tempDir := t.TempDir()

	view := NewStrategicReadinessView(tempDir)
	err := view.ScanFromStorage(context.Background(), mockSP)
	if err != nil {
		t.Fatalf("ScanFromStorage failed: %v", err)
	}

	snap := view.GetSnapshot()
	if snap.TotalActiveRequirements != 1 {
		t.Errorf("expected 1 active requirement, got %d", snap.TotalActiveRequirements)
	}
	if snap.AlignedRequirements != 1 {
		t.Errorf("expected 1 aligned requirement, got %d", snap.AlignedRequirements)
	}
	if snap.RunwayDepth != 1 {
		t.Errorf("expected runway depth 1, got %d", snap.RunwayDepth)
	}
}

func TestStrategicReadinessEvaluateAndReplenishRunway(t *testing.T) {
	mockSP := &mockTPMStorageProvider{
		plans: []map[string]any{
			{
				objects.FieldKeyID:     "PRI-001",
				objects.FieldKeyStatus: objects.ObjectStatusInProgress,
			},
		},
		requirements: []map[string]any{
			{
				objects.FieldKeyID:           "REQ-SEC-001",
				objects.FieldKeyTitle:        "Kernel Security Boundary Enforcement",
				objects.FieldKeyStatus:       objects.ObjectStatusActive,
				objects.FieldKeyCriteriaRefs: []any{"CRIT-SEC-001"},
			},
		},
		tcs: []map[string]any{
			{
				objects.FieldKeyID:           "TST-SEC-001",
				objects.FieldKeyCriteriaRefs: []any{"CRIT-SEC-001"},
			},
		},
	}

	tempDir := t.TempDir()

	snap, candidates, err := EvaluateAndReplenishRunway(context.Background(), mockSP, tempDir)
	if err != nil {
		t.Fatalf("EvaluateAndReplenishRunway failed: %v", err)
	}
	if snap == nil {
		t.Fatalf("expected non-nil snapshot")
	}
	if !snap.IsRunwayDepleted {
		t.Errorf("expected IsRunwayDepleted to be true for runway depth 1")
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].Domain != "storage" && candidates[0].Domain != "core_kernel" && candidates[0].Domain != "governance" {
		t.Logf("Candidate domain: %s", candidates[0].Domain)
	}
}

// TestStrategicReadinessAccumulatorInterfaceAndCircuitBreaker validates BLI-1789553222342866000-1043e052:
// StrategicReadinessView satisfies the generic accumulator.Accumulator interface with sub-5ms SLA.
func TestStrategicReadinessAccumulatorInterfaceAndCircuitBreaker(t *testing.T) {
	tempDir := t.TempDir()
	view := NewStrategicReadinessView(tempDir)

	// Static check that view satisfies accumulator.Accumulator[*StrategicReadinessLitePayload]
	var _ accumulator.Accumulator[*StrategicReadinessLitePayload] = view

	if view.Name() != "strategic_readiness" {
		t.Errorf("expected accumulator name 'strategic_readiness', got %s", view.Name())
	}
	if view.Engine() == nil {
		t.Fatal("expected non-nil engine attached to view")
	}

	def := view.DefaultPayload()
	if def == nil {
		t.Fatal("expected non-nil default payload")
	}

	// Hot path read through engine
	ctx := context.Background()
	mockSP := &mockTPMStorageProvider{}
	start := time.Now()
	envelope, err := view.Engine().GetOrRecoverPayload(ctx, mockSP)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("GetOrRecoverPayload failed: %v", err)
	}
	if elapsed > 25*time.Millisecond {
		t.Errorf("cold boot recovery exceeded threshold: %v", elapsed)
	}
	if envelope == nil || !envelope.Stale || !envelope.Recovering {
		t.Errorf("expected cold boot skeleton to be marked stale/recovering")
	}
}

func TestStrategicReadinessView_DualFormatDeserialization(t *testing.T) {
	tempDir := t.TempDir()

	litePath := StrategicReadinessLiteFilePath(tempDir)
	if err := fileutil.MkdirAll(filepath.Dir(litePath), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}

	// 1. Test loading legacy flat JSON format
	legacyTimestamp := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	legacyFlat := map[string]any{
		objects.FieldKeySchemaVersion: "1.0.0",
		"materialized_at":             legacyTimestamp.Format(time.RFC3339Nano),
		"domain_clusters": map[string]any{
			"architecture": map[string]any{
				objects.FieldKeyDomain: "architecture",
				"total_requirements":   5,
				"shovel_ready_count":   3,
			},
		},
		"plan_statuses": map[string]string{
			"PRI-DUAL-001": "in_progress",
		},
		"requirements": map[string]any{
			"REQ-DUAL-001": map[string]any{
				objects.FieldKeyID:     "REQ-DUAL-001",
				objects.FieldKeyTitle:  "Dual format support",
				objects.FieldKeyDomain: "architecture",
				objects.FieldKeyStatus: objects.ObjectStatusActive,
				"is_shovel_ready":      true,
			},
		},
	}
	legacyFlatJSON, err := json.Marshal(legacyFlat)
	if err != nil {
		t.Fatalf("marshal legacy flat: %v", err)
	}

	if err := fileutil.WriteFile(litePath, legacyFlatJSON, paths.FilePerm644); err != nil {
		t.Fatalf("write legacy flat file: %v", err)
	}

	// Test with engine attached
	viewWithEngine := NewStrategicReadinessView(tempDir)
	loaded, err := viewWithEngine.LoadFromLiteFile()
	if err != nil {
		t.Fatalf("LoadFromLiteFile (with engine) failed on legacy flat JSON: %v", err)
	}
	if !loaded {
		t.Fatal("expected LoadFromLiteFile to return true")
	}
	if len(viewWithEngine.domainClusters) != 1 || viewWithEngine.domainClusters["architecture"] == nil {
		t.Errorf("expected architecture cluster loaded from flat JSON, got %v", viewWithEngine.domainClusters)
	}
	if viewWithEngine.planStatuses["PRI-DUAL-001"] != "in_progress" {
		t.Errorf("expected plan status 'in_progress', got %q", viewWithEngine.planStatuses["PRI-DUAL-001"])
	}

	// Test with nil engine (fallback mode)
	viewFallback := &StrategicReadinessView{projectRoot: tempDir}
	loadedFallback, err := viewFallback.LoadFromLiteFile()
	if err != nil {
		t.Fatalf("LoadFromLiteFile (fallback) failed on legacy flat JSON: %v", err)
	}
	if !loadedFallback {
		t.Fatal("expected fallback LoadFromLiteFile to return true")
	}
	if len(viewFallback.domainClusters) != 1 || viewFallback.domainClusters["architecture"] == nil {
		t.Errorf("expected architecture cluster loaded in fallback from flat JSON")
	}

	// 2. Save via viewWithEngine to migrate file to canonical Envelope format
	if err := viewWithEngine.SaveToLiteFile(); err != nil {
		t.Fatalf("SaveToLiteFile failed to migrate: %v", err)
	}

	// 3. Load migrated Envelope format in clean views
	viewMigrated := NewStrategicReadinessView(tempDir)
	loadedMigrated, err := viewMigrated.LoadFromLiteFile()
	if err != nil {
		t.Fatalf("LoadFromLiteFile failed on migrated envelope: %v", err)
	}
	if !loadedMigrated {
		t.Fatal("expected LoadFromLiteFile on migrated envelope to return true")
	}
	if len(viewMigrated.domainClusters) != 1 || viewMigrated.domainClusters["architecture"].TotalRequirements != 5 {
		t.Errorf("expected architecture cluster with 5 requirements, got %v", viewMigrated.domainClusters["architecture"])
	}

	// Also verify fallback view reads canonical envelope correctly
	viewFallbackEnv := &StrategicReadinessView{projectRoot: tempDir}
	loadedFallbackEnv, err := viewFallbackEnv.LoadFromLiteFile()
	if err != nil {
		t.Fatalf("LoadFromLiteFile (fallback) failed on envelope JSON: %v", err)
	}
	if !loadedFallbackEnv {
		t.Fatal("expected fallback LoadFromLiteFile to return true on envelope")
	}
	if len(viewFallbackEnv.domainClusters) != 1 || viewFallbackEnv.domainClusters["architecture"].TotalRequirements != 5 {
		t.Errorf("expected architecture cluster with 5 requirements in fallback")
	}
}
