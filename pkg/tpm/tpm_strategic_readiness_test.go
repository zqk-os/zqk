// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package tpm

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/accumulator"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// cycle231MockStorageProvider provides configurable mock behavior for testing storage scans and errors.
type cycle231MockStorageProvider struct {
	storage.ObjectStorageProvider
	failKind  string
	failError error
	plans     []map[string]any
	blis      []map[string]any
	tcs       []map[string]any
	reqs      []map[string]any
}

func (m *cycle231MockStorageProvider) List(
	ctx context.Context,
	secCtx *storage.SecurityContext,
	storageCtx *storage.StorageContext,
	filter storage.ListFilter,
) (*storage.QueryResult, error) {
	if m.failKind != "" && m.failKind == filter.Kind {
		if m.failError != nil {
			return nil, m.failError
		}
		return nil, errors.New("mock storage error: " + filter.Kind)
	}

	var objs []map[string]any
	switch filter.Kind {
	case objects.KindPriorityPlan:
		objs = m.plans
	case objects.KindBacklogItem:
		objs = m.blis
	case objects.KindTestCase:
		objs = m.tcs
	case objects.KindRequirement:
		objs = m.reqs
	}
	return &storage.QueryResult{Objects: objs}, nil
}

func TestStrategicReadinessLiteFilePath_Cycle231(t *testing.T) {
	root := filepath.Join("/tmp", "zqk-project")
	expected := filepath.Join(root, paths.ProjectDataDir, paths.StateDir, "strategic_readiness_lite.json")
	actual := StrategicReadinessLiteFilePath(root)
	if actual != expected {
		t.Fatalf("StrategicReadinessLiteFilePath() = %q; want %q", actual, expected)
	}
}

func TestEvaluatePhiReq_Cycle231(t *testing.T) {
	if EvaluatePhiReq(nil) {
		t.Fatal("expected EvaluatePhiReq(nil) to be false")
	}

	reqEmptyCriteria := &RequirementReadinessNode{
		ID:           "REQ-PHI-001",
		Status:       objects.ObjectStatusActive,
		CriteriaRefs: []string{},
	}
	if EvaluatePhiReq(reqEmptyCriteria) {
		t.Fatal("expected EvaluatePhiReq to be false for empty criteria")
	}

	reqInactive := &RequirementReadinessNode{
		ID:           "REQ-PHI-002",
		Status:       objects.ObjectStatusCompleted,
		CriteriaRefs: []string{"CRIT-001"},
	}
	if EvaluatePhiReq(reqInactive) {
		t.Fatal("expected EvaluatePhiReq to be false for non-active requirement")
	}

	reqActive := &RequirementReadinessNode{
		ID:           "REQ-PHI-003",
		Status:       objects.ObjectStatusActive,
		CriteriaRefs: []string{"CRIT-001"},
	}
	if !EvaluatePhiReq(reqActive) {
		t.Fatal("expected EvaluatePhiReq to be true for active requirement with criteria")
	}
}

func TestEvaluatePhiTrace_Cycle231(t *testing.T) {
	critMap := map[string][]string{
		"CRIT-001": {"TST-001"},
	}

	if EvaluatePhiTrace(nil, critMap) {
		t.Fatal("expected EvaluatePhiTrace(nil) to be false")
	}

	reqNoCrit := &RequirementReadinessNode{
		ID:           "REQ-TRACE-001",
		CriteriaRefs: nil,
	}
	if EvaluatePhiTrace(reqNoCrit, critMap) {
		t.Fatal("expected EvaluatePhiTrace to be false for requirement without criteria")
	}

	reqMissingTest := &RequirementReadinessNode{
		ID:           "REQ-TRACE-002",
		CriteriaRefs: []string{"CRIT-001", "CRIT-UNMAPPED"},
	}
	if EvaluatePhiTrace(reqMissingTest, critMap) {
		t.Fatal("expected EvaluatePhiTrace to be false when any criterion lacks test case")
	}

	reqFullyMapped := &RequirementReadinessNode{
		ID:           "REQ-TRACE-003",
		CriteriaRefs: []string{"CRIT-001"},
	}
	if !EvaluatePhiTrace(reqFullyMapped, critMap) {
		t.Fatal("expected EvaluatePhiTrace to be true when all criteria have test cases")
	}
}

func TestEvaluatePhiFuel_Cycle231(t *testing.T) {
	reqToBLIs := map[string][]string{
		"REQ-001": {"BLI-001"},
		"REQ-002": {"BLI-EMPTY-PLAN"},
		"REQ-003": {"BLI-MISSING-STATUS"},
		"REQ-004": {"BLI-INACTIVE"},
		"REQ-005": {"BLI-SHOVEL"},
		"REQ-006": {"BLI-GROOMING"},
	}
	bliToPlan := map[string]string{
		"BLI-001":            "PRI-001",
		"BLI-EMPTY-PLAN":     "",
		"BLI-MISSING-STATUS": "PRI-NOT-IN-STATUS",
		"BLI-INACTIVE":       "PRI-INACTIVE",
		"BLI-SHOVEL":         "PRI-SHOVEL",
		"BLI-GROOMING":       "PRI-GROOMING",
	}
	planStatuses := map[string]string{
		"PRI-001":      objects.ObjectStatusInProgress,
		"PRI-INACTIVE": objects.ObjectStatusComplete,
		"PRI-SHOVEL":   "shovel_ready",
		"PRI-GROOMING": "grooming",
	}

	// 1. Missing requirement in reqToBLIs
	isFueled, planID, bliID := EvaluatePhiFuel("REQ-UNKNOWN", reqToBLIs, bliToPlan, planStatuses)
	if isFueled || planID != "" || bliID != "" {
		t.Fatal("expected unmapped requirement to have no fuel")
	}

	// 2. Empty BLIs
	reqToBLIs["REQ-EMPTY"] = []string{}
	isFueled, planID, bliID = EvaluatePhiFuel("REQ-EMPTY", reqToBLIs, bliToPlan, planStatuses)
	if isFueled || planID != "" || bliID != "" {
		t.Fatal("expected requirement with empty BLIs to have no fuel")
	}

	// 3. BLI not in bliToPlan
	reqToBLIs["REQ-NO-BLI-MAP"] = []string{"BLI-NOT-IN-MAP"}
	isFueled, planID, bliID = EvaluatePhiFuel("REQ-NO-BLI-MAP", reqToBLIs, bliToPlan, planStatuses)
	if isFueled || planID != "" || bliID != "" {
		t.Fatal("expected unmapped BLI to have no fuel")
	}

	// 4. BLI mapped to empty plan ID
	isFueled, planID, bliID = EvaluatePhiFuel("REQ-002", reqToBLIs, bliToPlan, planStatuses)
	if isFueled || planID != "" || bliID != "" {
		t.Fatal("expected empty plan ID to have no fuel")
	}

	// 5. Plan not found in planStatuses
	isFueled, planID, bliID = EvaluatePhiFuel("REQ-003", reqToBLIs, bliToPlan, planStatuses)
	if isFueled || planID != "" || bliID != "" {
		t.Fatal("expected unknown plan status to have no fuel")
	}

	// 6. Plan status completed/inactive
	isFueled, planID, bliID = EvaluatePhiFuel("REQ-004", reqToBLIs, bliToPlan, planStatuses)
	if isFueled || planID != "" || bliID != "" {
		t.Fatal("expected completed plan to have no fuel")
	}

	// 7. Shovel-ready plan
	isFueled, planID, bliID = EvaluatePhiFuel("REQ-005", reqToBLIs, bliToPlan, planStatuses)
	if !isFueled || planID != "PRI-SHOVEL" || bliID != "BLI-SHOVEL" {
		t.Fatalf("expected shovel_ready plan to fuel requirement, got %v, %s, %s", isFueled, planID, bliID)
	}

	// 8. Grooming plan
	isFueled, planID, bliID = EvaluatePhiFuel("REQ-006", reqToBLIs, bliToPlan, planStatuses)
	if !isFueled || planID != "PRI-GROOMING" || bliID != "BLI-GROOMING" {
		t.Fatalf("expected grooming plan to fuel requirement, got %v, %s, %s", isFueled, planID, bliID)
	}
}

func TestEvaluateRunwayDepth_Cycle231(t *testing.T) {
	statuses := map[string]string{
		"PRI-1": objects.ObjectStatusActive,
		"PRI-2": objects.ObjectStatusInProgress,
		"PRI-3": "shovel_ready",
		"PRI-4": objects.ObjectStatusGrooming,
		"PRI-5": objects.ObjectStatusComplete,
		"PRI-6": objects.ObjectStatusBlocked,
	}
	depth := EvaluateRunwayDepth(statuses)
	if depth != 4 {
		t.Fatalf("EvaluateRunwayDepth() = %d; want 4", depth)
	}
}

func TestIsRunwayDepleted_Cycle231(t *testing.T) {
	if !IsRunwayDepleted(0) {
		t.Fatal("expected depth 0 to be depleted")
	}
	if !IsRunwayDepleted(1) {
		t.Fatal("expected depth 1 to be depleted")
	}
	if IsRunwayDepleted(2) {
		t.Fatal("expected depth 2 to not be depleted")
	}
	if IsRunwayDepleted(5) {
		t.Fatal("expected depth 5 to not be depleted")
	}
}

func TestDetectDomain_Cycle231(t *testing.T) {
	cases := []struct {
		id       string
		title    string
		expected string
	}{
		{"REQ-01", "CAS Git lock storage mechanism", "storage"},
		{"REQ-02", "Cron daemon retention scheduler", "scheduler"},
		{"REQ-03", "MCP agent mesh streaming feed", "mesh_agent"},
		{"REQ-04", "TPM CAP orchestration whats-next priority plan", "tpm_orchestration"},
		{"REQ-05", "Audit policy governance document", "governance"},
		{"REQ-06", "Timeseries metric cache performance evaluation", "performance"},
		{"REQ-07", "Criteria test trace attestation suite", "qa_traceability"},
		{"REQ-08", "Generic system initialization sequence", "core_kernel"},
	}
	for _, c := range cases {
		domain := DetectDomain(c.id, c.title)
		if domain != c.expected {
			t.Errorf("DetectDomain(%q, %q) = %q; want %q", c.id, c.title, domain, c.expected)
		}
	}
}

func TestCheckRunwayReplenishment_Cycle231(t *testing.T) {
	tempDir := t.TempDir()
	view := NewStrategicReadinessView(tempDir)

	// Case 1: Empty candidates, depleted depth
	view.planStatuses["PRI-001"] = objects.ObjectStatusInProgress
	depleted, candidates := view.CheckRunwayReplenishment()
	if depleted || len(candidates) != 0 {
		t.Fatalf("expected false and empty candidates when synthesisCandidates is empty")
	}

	// Case 2: Candidates present, runway not depleted
	view.planStatuses["PRI-002"] = objects.ObjectStatusInProgress
	view.synthesisCandidates = append(view.synthesisCandidates, &SynthesisCandidate{
		Domain: "storage",
	})
	depleted, candidates = view.CheckRunwayReplenishment()
	if depleted || candidates != nil {
		t.Fatalf("expected false when runway depth > 1")
	}

	// Case 3: Candidates present and runway depleted (depth <= 1)
	delete(view.planStatuses, "PRI-002")
	depleted, candidates = view.CheckRunwayReplenishment()
	if !depleted || len(candidates) != 1 {
		t.Fatalf("expected true and candidates when runway depleted")
	}
}

func TestAppendRecentEventLocked_Cycle231(t *testing.T) {
	tempDir := t.TempDir()
	view := NewStrategicReadinessView(tempDir)

	for i := 0; i < 25; i++ {
		ev := &lifecycle.LifecycleEvent{
			EventType:  lifecycle.EventTypeStatusTransition,
			Kind:       objects.KindPriorityPlan,
			ID:         "PRI-CYCLE-231",
			FromStatus: "grooming",
			ToStatus:   objects.ObjectStatusInProgress,
			Ts:         time.Now(),
		}
		view.ApplyLifecycleEvent(ev)
	}

	snap := view.GetSnapshot()
	if len(snap.RecentEvents) != 20 {
		t.Fatalf("expected recentEvents to cap at 20, got %d", len(snap.RecentEvents))
	}
}

func TestScanFromStorageWithSecurity_StorageErrors_Cycle231(t *testing.T) {
	tempDir := t.TempDir()
	secCtx := pkgctx.NewSystemSecurityContext()

	kindsToFail := []string{
		objects.KindPriorityPlan,
		objects.KindBacklogItem,
		objects.KindTestCase,
		objects.KindRequirement,
	}

	for _, kind := range kindsToFail {
		mockSP := &cycle231MockStorageProvider{
			failKind:  kind,
			failError: errors.New("simulated list failure for " + kind),
		}
		view := NewStrategicReadinessView(tempDir)
		err := view.ScanFromStorageWithSecurity(context.Background(), mockSP, secCtx)
		if err == nil {
			t.Fatalf("expected error when scanning %s, got nil", kind)
		}
	}
}

func buildMixedRequirementsMockStorage() *cycle231MockStorageProvider {
	return &cycle231MockStorageProvider{
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
				objects.FieldKeyRequirementRefs: []any{"REQ-ALIGNED-001"},
			},
		},
		tcs: []map[string]any{
			{
				objects.FieldKeyID:           "TST-001",
				objects.FieldKeyCriteriaRefs: []any{"CRIT-001"},
			},
			{
				objects.FieldKeyID:           "TST-002",
				objects.FieldKeyCriteriaRefs: []any{"CRIT-002"},
			},
		},
		reqs: []map[string]any{
			{
				// Fully aligned: has criteria, has test case, has BLI attached to active plan
				objects.FieldKeyID:           "REQ-ALIGNED-001",
				objects.FieldKeyTitle:        "Storage CAS Layer Invariant",
				objects.FieldKeyStatus:       objects.ObjectStatusActive,
				objects.FieldKeyCriteriaRefs: []any{"CRIT-001"},
			},
			{
				// Shovel ready: has criteria and test case, but unmapped execution lane
				objects.FieldKeyID:           "REQ-SHOVEL-001",
				objects.FieldKeyTitle:        "Storage Atomic Write Verification",
				objects.FieldKeyStatus:       objects.ObjectStatusActive,
				objects.FieldKeyCriteriaRefs: []any{"CRIT-002"},
			},
			{
				// Missing criteria
				objects.FieldKeyID:           "REQ-NOCRIT-001",
				objects.FieldKeyTitle:        "Governance Policy Tracking",
				objects.FieldKeyStatus:       objects.ObjectStatusActive,
				objects.FieldKeyCriteriaRefs: []any{},
			},
			{
				// Missing test attestation
				objects.FieldKeyID:           "REQ-NOTEST-001",
				objects.FieldKeyTitle:        "Scheduler Retention Policy",
				objects.FieldKeyStatus:       objects.ObjectStatusActive,
				objects.FieldKeyCriteriaRefs: []any{"CRIT-UNATTESTED-001"},
			},
			{
				// Inactive requirement
				objects.FieldKeyID:           "REQ-INACTIVE-001",
				objects.FieldKeyTitle:        "Deprecated Mesh Routing",
				objects.FieldKeyStatus:       objects.ObjectStatusCompleted,
				objects.FieldKeyCriteriaRefs: []any{"CRIT-001"},
			},
		},
	}
}

func TestScanFromStorage_MixedRequirementsAndClustering_Cycle231(t *testing.T) {
	mockSP := buildMixedRequirementsMockStorage()
	tempDir := t.TempDir()
	view := NewStrategicReadinessView(tempDir)
	if err := view.ScanFromStorage(context.Background(), mockSP); err != nil {
		t.Fatalf("ScanFromStorage failed: %v", err)
	}

	snap := view.GetSnapshot()
	if snap.TotalActiveRequirements != 4 {
		t.Fatalf("expected 4 active requirements, got %d", snap.TotalActiveRequirements)
	}
	if snap.AlignedRequirements != 1 {
		t.Fatalf("expected 1 aligned requirement, got %d", snap.AlignedRequirements)
	}
	if snap.GapRequirementsCount != 3 {
		t.Fatalf("expected 3 gap requirements, got %d", snap.GapRequirementsCount)
	}

	reqAligned := snap.Requirements["REQ-ALIGNED-001"]
	if !reqAligned.IsFullyAligned || !reqAligned.IsShovelReady {
		t.Fatalf("expected REQ-ALIGNED-001 to be fully aligned and shovel ready")
	}

	reqShovel := snap.Requirements["REQ-SHOVEL-001"]
	if !reqShovel.IsShovelReady || reqShovel.IsFullyAligned {
		t.Fatalf("expected REQ-SHOVEL-001 to be shovel ready but not fully aligned")
	}

	reqNoCrit := snap.Requirements["REQ-NOCRIT-001"]
	if reqNoCrit.HasCriteria {
		t.Fatalf("expected REQ-NOCRIT-001 to have HasCriteria=false")
	}

	reqNoTest := snap.Requirements["REQ-NOTEST-001"]
	if reqNoTest.HasAttestationSuites {
		t.Fatalf("expected REQ-NOTEST-001 to have HasAttestationSuites=false")
	}

	// Verify domain clustering
	storageCluster := snap.DomainClusters["storage"]
	if storageCluster == nil {
		t.Fatal("expected storage cluster in domainClusters")
	}
	if storageCluster.TotalRequirements != 2 || storageCluster.AlignedRequirements != 1 {
		t.Fatalf("unexpected storage cluster counts: total=%d, aligned=%d",
			storageCluster.TotalRequirements, storageCluster.AlignedRequirements)
	}
}

func TestSaveToLiteFile_ErrorBranches_Cycle231(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Error creating state directory (a regular file blocks directory creation)
	blockerPath := filepath.Join(tempDir, paths.ProjectDataDir)
	if err := fileutil.WriteFile(blockerPath, []byte("blocker"), paths.FilePerm644); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}

	viewBlockedDir := &StrategicReadinessView{
		projectRoot:  tempDir,
		requirements: make(map[string]*RequirementReadinessNode),
		planStatuses: make(map[string]string),
	}
	err := viewBlockedDir.SaveToLiteFile()
	if err == nil {
		t.Fatal("expected error when state dir creation fails, got nil")
	}

	// 2. Error writing tmp file (a directory exists where tmp file should be written)
	tempDir2 := t.TempDir()
	litePath := StrategicReadinessLiteFilePath(tempDir2)
	tmpPath := litePath + ".tmp"
	if err := fileutil.MkdirAll(tmpPath, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir tmp blocker: %v", err)
	}

	viewBlockedTmp := &StrategicReadinessView{
		projectRoot:  tempDir2,
		requirements: make(map[string]*RequirementReadinessNode),
		planStatuses: make(map[string]string),
	}
	err = viewBlockedTmp.SaveToLiteFile()
	if err == nil {
		t.Fatal("expected error when tmp file write fails, got nil")
	}
}

func TestLoadFromLiteFile_EdgeCasesAndErrors_Cycle231(t *testing.T) {
	tempDir := t.TempDir()
	litePath := StrategicReadinessLiteFilePath(tempDir)
	if err := fileutil.MkdirAll(filepath.Dir(litePath), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}

	// 1. Corrupted JSON file in engine mode
	if err := fileutil.WriteFile(litePath, []byte("{invalid-json"), paths.FilePerm644); err != nil {
		t.Fatalf("write corrupted lite file: %v", err)
	}
	viewWithEngine := NewStrategicReadinessView(tempDir)
	loaded, err := viewWithEngine.LoadFromLiteFile()
	if err == nil {
		t.Fatal("expected error on corrupted JSON with engine, got nil")
	}
	if loaded {
		t.Fatal("expected loaded=false on error")
	}

	// 2. Corrupted JSON file in fallback mode (engine == nil)
	viewFallback := &StrategicReadinessView{projectRoot: tempDir}
	loaded, err = viewFallback.LoadFromLiteFile()
	if err == nil {
		t.Fatal("expected error on corrupted JSON without engine, got nil")
	}
	if loaded {
		t.Fatal("expected loaded=false on error")
	}

	// 3. Fallback mode with missing file returns false, nil
	if err := fileutil.Remove(litePath); err != nil {
		t.Fatalf("remove litePath: %v", err)
	}
	loaded, err = viewFallback.LoadFromLiteFile()
	if err != nil {
		t.Fatalf("expected nil error on missing file, got %v", err)
	}
	if loaded {
		t.Fatal("expected loaded=false on missing file")
	}

	// 4. Null payload in lite file with engine
	nullEnvelope := []byte("null")
	if err := fileutil.WriteFile(litePath, nullEnvelope, paths.FilePerm644); err != nil {
		t.Fatalf("write null envelope: %v", err)
	}
	loaded, err = viewWithEngine.LoadFromLiteFile()
	if err != nil {
		t.Fatalf("unexpected error on null payload envelope: %v", err)
	}
	if loaded {
		t.Fatal("expected loaded=false on null payload envelope")
	}
}

func TestEvaluateAndReplenishRunway_Branches_Cycle231(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Scan storage error branch
	mockErrSP := &cycle231MockStorageProvider{
		failKind:  objects.KindPriorityPlan,
		failError: errors.New("simulated plan list failure"),
	}
	_, _, err := EvaluateAndReplenishRunway(context.Background(), mockErrSP, tempDir)
	if err == nil {
		t.Fatal("expected error on storage failure in EvaluateAndReplenishRunway, got nil")
	}

	// 2. Non-depleted runway branch (runway depth > 1)
	mockDeepSP := &cycle231MockStorageProvider{
		plans: []map[string]any{
			{
				objects.FieldKeyID:     "PRI-001",
				objects.FieldKeyStatus: objects.ObjectStatusInProgress,
			},
			{
				objects.FieldKeyID:     "PRI-002",
				objects.FieldKeyStatus: objects.ObjectStatusActive,
			},
		},
		reqs: []map[string]any{
			{
				objects.FieldKeyID:           "REQ-001",
				objects.FieldKeyTitle:        "Kernel Core",
				objects.FieldKeyStatus:       objects.ObjectStatusActive,
				objects.FieldKeyCriteriaRefs: []any{"CRIT-001"},
			},
		},
		tcs: []map[string]any{
			{
				objects.FieldKeyID:           "TST-001",
				objects.FieldKeyCriteriaRefs: []any{"CRIT-001"},
			},
		},
	}
	tempDirDeep := t.TempDir()
	snap, candidates, err := EvaluateAndReplenishRunway(context.Background(), mockDeepSP, tempDirDeep)
	if err != nil {
		t.Fatalf("EvaluateAndReplenishRunway failed: %v", err)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if snap.IsRunwayDepleted {
		t.Fatal("expected runway to not be depleted with depth 2")
	}
	if candidates != nil {
		t.Fatal("expected nil candidates when runway is not depleted")
	}
}

func TestSubscribeWAL_EngineAndFullChannel_Cycle231(t *testing.T) {
	tempDir := t.TempDir()
	view := NewStrategicReadinessView(tempDir)

	// Call SubscribeWAL with engine != nil and canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	updateCh := make(chan struct{}, 1)
	view.SubscribeWAL(ctx, updateCh)

	// Test fallback with full update channel to trigger default select branch
	tempDir2 := t.TempDir()
	viewFallback := &StrategicReadinessView{
		projectRoot:  tempDir2,
		requirements: make(map[string]*RequirementReadinessNode),
		planStatuses: make(map[string]string),
	}

	fullCh := make(chan struct{}, 1)
	fullCh <- struct{}{} // fill the channel

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	viewFallback.StartBackgroundWALSubscriber(ctx2, fullCh)
	lifecycle.AppendStatusTransition(tempDir2, objects.KindRequirement, "REQ-CHAN-001", "open", "active")
	time.Sleep(250 * time.Millisecond)
}

func TestCheckReadinessCacheStaleness_Cycle231(t *testing.T) {
	tempDir := t.TempDir()
	mockSP := &cycle231MockStorageProvider{
		plans: []map[string]any{
			{
				objects.FieldKeyID:     "PRI-001",
				objects.FieldKeyStatus: objects.ObjectStatusInProgress,
			},
		},
		reqs: []map[string]any{
			{
				objects.FieldKeyID:           "REQ-001",
				objects.FieldKeyTitle:        "Core Kernel Invariant",
				objects.FieldKeyStatus:       objects.ObjectStatusActive,
				objects.FieldKeyCriteriaRefs: []any{"CRIT-001"},
			},
		},
		tcs: []map[string]any{
			{
				objects.FieldKeyID:           "TST-001",
				objects.FieldKeyCriteriaRefs: []any{"CRIT-001"},
			},
		},
	}

	// 1. First run: cold cache, storage scanned
	snap1, candidates1, err := EvaluateAndReplenishRunway(context.Background(), mockSP, tempDir)
	if err != nil {
		t.Fatalf("first EvaluateAndReplenishRunway failed: %v", err)
	}
	if snap1 == nil || len(candidates1) == 0 {
		t.Fatal("expected snapshot and candidates from cold run")
	}

	// 2. Second run: cache is fresh (mtime < 5m), storage not scanned
	mockSP.failKind = objects.KindPriorityPlan
	mockSP.failError = errors.New("storage must not be accessed when cache is fresh")
	snap2, _, err := EvaluateAndReplenishRunway(context.Background(), mockSP, tempDir)
	if err != nil {
		t.Fatalf("fresh cache EvaluateAndReplenishRunway failed: %v", err)
	}
	if snap2.TotalActiveRequirements != snap1.TotalActiveRequirements {
		t.Fatalf("expected identical snapshot from fresh cache")
	}

	// 3. Stale cache: modify file time to > 10 minutes ago
	litePath := StrategicReadinessLiteFilePath(tempDir)
	oldTime := time.Now().Add(-15 * time.Minute)
	if err := fileutil.Chtimes(litePath, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes failed: %v", err)
	}

	// Remove simulated failure so refresh succeeds
	mockSP.failKind = ""
	mockSP.failError = nil
	snap3, _, err := EvaluateAndReplenishRunway(context.Background(), mockSP, tempDir)
	if err != nil {
		t.Fatalf("stale cache refresh failed: %v", err)
	}
	if snap3 == nil {
		t.Fatal("expected non-nil snapshot after stale refresh")
	}
}

func TestStrategicReadinessSummaryMetrics_Cycle231(t *testing.T) {
	tempDir := t.TempDir()
	view := NewStrategicReadinessView(tempDir)

	view.requirements["REQ-001"] = &RequirementReadinessNode{
		ID:             "REQ-001",
		Status:         objects.ObjectStatusActive,
		IsShovelReady:  true,
		IsFullyAligned: true,
	}
	view.requirements["REQ-002"] = &RequirementReadinessNode{
		ID:             "REQ-002",
		Status:         objects.ObjectStatusActive,
		IsShovelReady:  true,
		IsFullyAligned: false,
	}
	view.requirements["REQ-003"] = &RequirementReadinessNode{
		ID:             "REQ-003",
		Status:         objects.ObjectStatusActive,
		IsShovelReady:  false,
		IsFullyAligned: false,
	}

	snap := view.GetSnapshot()
	if snap.TotalActiveRequirements != 3 {
		t.Fatalf("expected 3 active requirements, got %d", snap.TotalActiveRequirements)
	}
	if snap.AlignedRequirements != 1 {
		t.Fatalf("expected 1 aligned requirement, got %d", snap.AlignedRequirements)
	}
	if snap.GapRequirementsCount != 2 {
		t.Fatalf("expected 2 gap requirements, got %d", snap.GapRequirementsCount)
	}

	alignedRatio := float64(snap.AlignedRequirements) / float64(snap.TotalActiveRequirements)
	if alignedRatio < 0.33 || alignedRatio > 0.34 {
		t.Fatalf("unexpected aligned ratio: %f", alignedRatio)
	}
}

func TestSubscribeWAL_FallbackNilEngine_Cycle231(t *testing.T) {
	tempDir := t.TempDir()
	view := &StrategicReadinessView{
		projectRoot:  tempDir,
		requirements: make(map[string]*RequirementReadinessNode),
		planStatuses: make(map[string]string),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	updateCh := make(chan struct{}, 2)
	view.StartBackgroundWALSubscriber(ctx, updateCh)

	// Append a lifecycle transition to WAL
	lifecycle.AppendStatusTransition(tempDir, objects.KindRequirement, "REQ-WAL-001", "open", "active")

	// Allow subscriber goroutine brief moment to poll
	time.Sleep(300 * time.Millisecond)
	cancel()
}

func TestAccumulatorRegistry_Cycle231(t *testing.T) {
	names := accumulator.RegisteredSubscribers()

	found := false
	for _, n := range names {
		if n == "strategic_readiness" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'strategic_readiness' to be registered in accumulator registry")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subscribers := accumulator.StartAllSubscribers(ctx, "/test/project")
	if len(subscribers) == 0 {
		t.Fatal("expected at least one subscriber started from registry")
	}
	cancel()
	time.Sleep(50 * time.Millisecond)
}

func TestStrategicReadinessConcurrencySafety_Cycle231(t *testing.T) {
	tempDir := t.TempDir()
	view := NewStrategicReadinessView(tempDir)

	mockSP := &cycle231MockStorageProvider{
		plans: []map[string]any{
			{
				objects.FieldKeyID:     "PRI-CONC-001",
				objects.FieldKeyStatus: objects.ObjectStatusInProgress,
			},
		},
		reqs: []map[string]any{
			{
				objects.FieldKeyID:           "REQ-CONC-001",
				objects.FieldKeyTitle:        "Concurrent Access Safety",
				objects.FieldKeyStatus:       objects.ObjectStatusActive,
				objects.FieldKeyCriteriaRefs: []any{"CRIT-CONC-001"},
			},
		},
		tcs: []map[string]any{
			{
				objects.FieldKeyID:           "TST-CONC-001",
				objects.FieldKeyCriteriaRefs: []any{"CRIT-CONC-001"},
			},
		},
	}

	if err := view.ScanFromStorage(context.Background(), mockSP); err != nil {
		t.Fatalf("initial scan failed: %v", err)
	}

	var wg sync.WaitGroup
	var saveMu sync.Mutex
	workers := 8
	iterations := 20

	for i := 0; i < workers; i++ {
		workerID := i
		wg.Add(1)
		goroutinelabels.NewGoroutine("test-tpm-concurrency", "concurrent view operations").
			StartSimple(func() {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					switch workerID % 4 {
					case 0:
						snap := view.GetSnapshot()
						if snap == nil {
							t.Error("expected non-nil snapshot")
						}
					case 1:
						ev := &lifecycle.LifecycleEvent{
							EventType:  lifecycle.EventTypeStatusTransition,
							Kind:       objects.KindPriorityPlan,
							ID:         "PRI-CONC-001",
							FromStatus: objects.ObjectStatusInProgress,
							ToStatus:   objects.ObjectStatusActive,
							Ts:         time.Now(),
						}
						view.ApplyLifecycleEvent(ev)
					case 2:
						depleted, candList := view.CheckRunwayReplenishment()
						if depleted && len(candList) == 0 {
							t.Error("expected non-empty candidate list when depleted")
						}
					case 3:
						if j%5 == 0 {
							saveMu.Lock()
							err := view.SaveToLiteFile()
							saveMu.Unlock()
							if err != nil {
								t.Errorf("concurrent SaveToLiteFile failed: %v", err)
							}
						} else {
							loaded, err := view.LoadFromLiteFile()
							if err != nil {
								t.Errorf("concurrent LoadFromLiteFile failed: %v", err)
							}
							if !loaded {
								// ok if file not yet written on initial iterations
							}
						}
					}
				}
			})
	}

	wg.Wait()
}
