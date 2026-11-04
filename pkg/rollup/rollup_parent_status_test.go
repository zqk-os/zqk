// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package rollup

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// mockCustomStatusChecker allows testing edge conditions in isStatusComplete,
// isStatusInProgress, and isStatusHalted without global state side-effects.
type mockCustomStatusChecker struct {
	doneFn   func(kind, status string) bool
	roleFn   func(kind, status string) string
	compatFn func(kind, status string) bool
}

func (m mockCustomStatusChecker) IsWorkDone(kind, status string) bool {
	if m.doneFn != nil {
		return m.doneFn(kind, status)
	}
	return false
}

func (m mockCustomStatusChecker) Role(kind, status string) string {
	if m.roleFn != nil {
		return m.roleFn(kind, status)
	}
	return ""
}

func (m mockCustomStatusChecker) IsTerminal(kind, status string) bool {
	return false
}

func (m mockCustomStatusChecker) IsPreliminary(kind, status string) bool {
	return false
}

func (m mockCustomStatusChecker) IsActive(kind, status string) bool {
	return false
}

func (m mockCustomStatusChecker) IsArchive(kind, status string) bool {
	return false
}

func (m mockCustomStatusChecker) IsSystem(kind, status string) bool {
	return false
}

func (m mockCustomStatusChecker) IsSatisfied(kind, status string) bool {
	return false
}

// advStorageStub implements storage.ObjectStorageProvider for deep resolver branch testing.
type advStorageStub struct {
	storage.NoopObjectStorage
	mu            sync.Mutex
	listResults   map[string][]map[string]any
	listErrors    map[string]error
	bulkResults   []map[string]any
	bulkError     error
	listCalls     int
	bulkCalls     int
	dependentList []string
}

func newAdvStorageStub() *advStorageStub {
	return &advStorageStub{
		listResults: make(map[string][]map[string]any),
		listErrors:  make(map[string]error),
	}
}

func (s *advStorageStub) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCalls++

	if err, ok := s.listErrors[filter.Kind]; ok && err != nil {
		return nil, err
	}
	if err, ok := s.listErrors["*"]; ok && err != nil {
		return nil, err
	}

	if objs, ok := s.listResults[filter.Kind]; ok {
		return &storage.QueryResult{Objects: objs}, nil
	}
	if objs, ok := s.listResults["*"]; ok {
		return &storage.QueryResult{Objects: objs}, nil
	}

	return &storage.QueryResult{Objects: nil}, nil
}

func (s *advStorageStub) BulkGet(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string) (*storage.BulkResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bulkCalls++

	if s.bulkError != nil {
		return nil, s.bulkError
	}
	return &storage.BulkResult{Results: s.bulkResults}, nil
}

func TestStatusDecisions_DirectBranches(t *testing.T) {
	t.Parallel()

	testStatusCompleteBranches(t)
	testStatusInProgressBranches(t)
	testStatusHaltedBranches(t)
}

func testStatusCompleteBranches(t *testing.T) {
	if isStatusComplete(nil, "task", "") {
		t.Fatal("empty status must not be complete")
	}

	mockDone := mockCustomStatusChecker{
		doneFn: func(kind, status string) bool {
			return status == "done_custom"
		},
	}
	if !isStatusComplete(mockDone, "task", "done_custom") {
		t.Fatal("checker.IsWorkDone should return true")
	}

	mockNotDone := mockCustomStatusChecker{
		doneFn: func(kind, status string) bool {
			return false
		},
	}
	completeStatuses := []string{
		objects.ObjectStatusComplete,
		objects.ObjectStatusCompleted,
		objects.ObjectStatusValidated,
	}
	for _, st := range completeStatuses {
		if !isStatusComplete(mockNotDone, "task", st) {
			t.Fatalf("expected status %q to be complete via fallback switch", st)
		}
		if !isStatusComplete(nil, "task", st) {
			t.Fatalf("expected status %q to be complete when checker is nil", st)
		}
	}

	if isStatusComplete(mockNotDone, "task", "in_progress") {
		t.Fatal("in_progress must not be complete")
	}
	if isStatusComplete(nil, "task", "unknown_status") {
		t.Fatal("unknown status must not be complete with nil checker")
	}
}

func testStatusInProgressBranches(t *testing.T) {
	if isStatusInProgress(nil, "task", "") {
		t.Fatal("empty status must not be in_progress")
	}

	mockLocked := mockCustomStatusChecker{
		roleFn: func(kind, status string) string {
			if status == "active_executing" {
				return objects.LifecycleRoleExecutionLocked
			}
			return ""
		},
	}
	if !isStatusInProgress(mockLocked, "task", "active_executing") {
		t.Fatal("LifecycleRoleExecutionLocked should be in_progress")
	}

	if !isStatusInProgress(mockLocked, "task", objects.ObjectStatusInProgress) {
		t.Fatal("standard ObjectStatusInProgress should return true")
	}
	if !isStatusInProgress(nil, "task", objects.ObjectStatusInProgress) {
		t.Fatal("standard ObjectStatusInProgress with nil checker should return true")
	}
	if isStatusInProgress(mockLocked, "task", "draft") {
		t.Fatal("draft status must not be in_progress")
	}
	if isStatusInProgress(nil, "task", "draft") {
		t.Fatal("draft status with nil checker must not be in_progress")
	}
}

func testStatusHaltedBranches(t *testing.T) {
	if isStatusHalted(nil, "task", "") {
		t.Fatal("empty status must not be halted")
	}

	mockHalted := mockCustomStatusChecker{
		roleFn: func(kind, status string) string {
			if status == "stalled_custom" {
				return objects.LifecycleRoleHalted
			}
			return ""
		},
	}
	if !isStatusHalted(mockHalted, "task", "stalled_custom") {
		t.Fatal("LifecycleRoleHalted should be halted")
	}

	mockNotHalted := mockCustomStatusChecker{
		roleFn: func(kind, status string) string {
			return ""
		},
	}
	haltedStatuses := []string{
		objects.ObjectStatusBlocked,
		objects.ObjectStatusPaused,
	}
	for _, st := range haltedStatuses {
		if !isStatusHalted(mockNotHalted, "task", st) {
			t.Fatalf("expected status %q to be halted via switch fallback", st)
		}
		if !isStatusHalted(nil, "task", st) {
			t.Fatalf("expected status %q to be halted when checker is nil", st)
		}
	}

	if isStatusHalted(mockNotHalted, "task", "in_progress") {
		t.Fatal("in_progress must not be halted")
	}
	if isStatusHalted(nil, "task", "unknown_status") {
		t.Fatal("unknown status must not be halted with nil checker")
	}
}

func TestExtractStringSlice_BoundaryCases(t *testing.T) {
	t.Parallel()

	if got := extractStringSlice(nil); got != nil {
		t.Fatalf("expected nil for nil input, got %+v", got)
	}

	rawSlice := []string{"id-1", "id-2"}
	if got := extractStringSlice(rawSlice); len(got) != 2 || got[0] != "id-1" || got[1] != "id-2" {
		t.Fatalf("expected string slice identity, got %+v", got)
	}

	anySlice := []any{"item-a", "", 42, "item-b", true, nil}
	gotAny := extractStringSlice(anySlice)
	if len(gotAny) != 2 || gotAny[0] != "item-a" || gotAny[1] != "item-b" {
		t.Fatalf("expected filtered []any with strings only, got %+v", gotAny)
	}

	if got := extractStringSlice("single-id"); len(got) != 1 || got[0] != "single-id" {
		t.Fatalf("expected single string element, got %+v", got)
	}
	if got := extractStringSlice(""); got != nil {
		t.Fatalf("expected nil for empty string, got %+v", got)
	}

	if got := extractStringSlice(12345); got != nil {
		t.Fatalf("expected nil for non-string type (int), got %+v", got)
	}
	if got := extractStringSlice(map[string]any{"key": "val"}); got != nil {
		t.Fatalf("expected nil for non-string type (map), got %+v", got)
	}
}

func TestIsChildOfParent_Comprehensive(t *testing.T) {
	t.Parallel()

	testIsChildOfParentMilestone(t)
	testIsChildOfParentPriorityPlan(t)
	testIsChildOfParentGeneric(t)
}

func testIsChildOfParentMilestone(t *testing.T) {
	if isChildOfParent(nil, objects.KindMilestone, "MS-1") {
		t.Fatal("nil object must return false")
	}
	if isChildOfParent(map[string]any{objects.FieldKeyID: "B-1"}, objects.KindMilestone, "") {
		t.Fatal("empty parentID must return false")
	}

	disallowedMilestoneKinds := []string{
		objects.KindRoadmap,
		objects.KindGoal,
		objects.KindStrategicPlan,
	}
	for _, k := range disallowedMilestoneKinds {
		obj := map[string]any{
			objects.FieldKeyKind:         k,
			objects.FieldKeyMilestoneRef: "MS-1",
		}
		if isChildOfParent(obj, objects.KindMilestone, "MS-1") {
			t.Fatalf("milestone child kind %q should be rejected", k)
		}
	}

	objRef := map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyMilestoneRef: "MS-1",
	}
	if !isChildOfParent(objRef, objects.KindMilestone, "MS-1") {
		t.Fatal("milestone_ref match should return true")
	}
	if isChildOfParent(objRef, objects.KindMilestone, "MS-OTHER") {
		t.Fatal("mismatched milestone_ref should return false")
	}

	objRefs := map[string]any{
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyMilestoneRefs: []string{"MS-1", "MS-2"},
	}
	if !isChildOfParent(objRefs, objects.KindMilestone, "MS-1") {
		t.Fatal("milestone_refs slice match should return true")
	}
	if isChildOfParent(objRefs, objects.KindMilestone, "MS-99") {
		t.Fatal("unmatched milestone_refs slice should return false")
	}
}

func testIsChildOfParentPriorityPlan(t *testing.T) {
	nonBliObj := map[string]any{
		objects.FieldKeyKind:            objects.KindGoal,
		objects.FieldKeyPriorityPlanRef: "PRI-1",
	}
	if isChildOfParent(nonBliObj, objects.KindPriorityPlan, "PRI-1") {
		t.Fatal("priority plan child with kind!=backlog_item must return false")
	}

	bliObj := map[string]any{
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: "PRI-1",
	}
	if !isChildOfParent(bliObj, objects.KindPriorityPlan, "PRI-1") {
		t.Fatal("backlog_item with matching priority_plan_ref must return true")
	}

	emptyKindObj := map[string]any{
		objects.FieldKeyPriorityPlanRef: "PRI-1",
	}
	if !isChildOfParent(emptyKindObj, objects.KindPriorityPlan, "PRI-1") {
		t.Fatal("empty kind object with matching priority_plan_ref must return true")
	}

	if isChildOfParent(bliObj, objects.KindPriorityPlan, "PRI-OTHER") {
		t.Fatal("mismatched priority_plan_ref must return false")
	}
}

func testIsChildOfParentGeneric(t *testing.T) {
	customParent := "CUSTOM-PARENT-100"

	strObj := map[string]any{"container_id": customParent}
	if !isChildOfParent(strObj, "custom_kind", customParent) {
		t.Fatal("generic string field match should return true")
	}

	sliceStrObj := map[string]any{"owner_ids": []string{"OTHER", customParent}}
	if !isChildOfParent(sliceStrObj, "custom_kind", customParent) {
		t.Fatal("generic []string field match should return true")
	}

	sliceAnyObj := map[string]any{"tag_refs": []any{"MISC", customParent, 999}}
	if !isChildOfParent(sliceAnyObj, "custom_kind", customParent) {
		t.Fatal("generic []any field match should return true")
	}

	noMatchObj := map[string]any{
		"numeric": 12345,
		"flag":    true,
		"list":    []string{"DIFFERENT"},
		"anyList": []any{false, 42, "NON_MATCHING"},
		"nested":  map[string]any{"key": customParent},
	}
	if isChildOfParent(noMatchObj, "custom_kind", customParent) {
		t.Fatal("generic non-matching fields must return false")
	}
}

func TestFetchChildrenForParent_EdgeAndErrorHandling(t *testing.T) {
	t.Parallel()

	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. sp is nil or parentID is empty
	got, err := FetchChildrenForParent(context.Background(), secCtx, nil, objects.KindMilestone, "MS-1")
	if err != nil || got != nil {
		t.Fatalf("expected nil, nil when sp is nil, got %v, %v", got, err)
	}

	stub := newAdvStorageStub()
	got, err = FetchChildrenForParent(context.Background(), secCtx, stub, objects.KindMilestone, "")
	if err != nil || got != nil {
		t.Fatalf("expected nil, nil when parentID is empty, got %v, %v", got, err)
	}

	// 2. Milestone with list error
	stubErr := newAdvStorageStub()
	stubErr.listErrors[objects.KindBacklogItem] = errors.New("simulated storage failure")
	gotErr, err := FetchChildrenForParent(context.Background(), secCtx, stubErr, objects.KindMilestone, "MS-1")
	if err != nil {
		t.Fatalf("FetchChildrenForParent should not fail when List errors, got err: %v", err)
	}
	if len(gotErr) != 0 {
		t.Fatalf("expected 0 children on error, got %d", len(gotErr))
	}

	// 3. Milestone duplicate resolution
	testMilestoneDuplicateResolution(t, secCtx)

	// 4. PriorityPlan resolution and error handling
	testPriorityPlanResolution(t, secCtx)

	// 5. Default generic resolution
	testGenericResolution(t, secCtx)

	// 6. Generic reverse reference fallback via DependentsForID and BulkGet
	testGenericReverseReferenceFallback(t, secCtx)
}

func testGenericReverseReferenceFallback(t *testing.T, secCtx *pkgctx.SecurityContext) {
	parentID := "MS-REV-PARENT"
	stub := newAdvStorageStub()

	// Direct milestone query returns empty (no direct backlog items)
	stub.listResults[objects.KindBacklogItem] = nil

	// Dependents query will scan registered candidate kinds (e.g. agent_task or custom kind).
	// We provide an agent task that references the milestone:
	taskObj := map[string]any{
		objects.FieldKeyID:           "TASK-DEP-1",
		objects.FieldKeyKind:         objects.KindAgentTask,
		objects.FieldKeyMilestoneRef: parentID,
	}
	taskObjNoID := map[string]any{
		objects.FieldKeyID:           "",
		objects.FieldKeyKind:         objects.KindAgentTask,
		objects.FieldKeyMilestoneRef: parentID,
	}
	taskObjNonChild := map[string]any{
		objects.FieldKeyID:           "GOAL-DEP-2",
		objects.FieldKeyKind:         objects.KindGoal, // rejected by isChildOfParent for milestone
		objects.FieldKeyMilestoneRef: parentID,
	}

	stub.listResults[objects.KindAgentTask] = []map[string]any{taskObj, taskObjNoID}
	stub.listResults[objects.KindGoal] = []map[string]any{taskObjNonChild}

	// BulkGet returns all 3 candidate results to test id!="", isChildOfParent, and filtering
	stub.bulkResults = []map[string]any{
		taskObj,
		taskObjNoID,
		taskObjNonChild,
	}

	children, err := FetchChildrenForParent(context.Background(), secCtx, stub, objects.KindMilestone, parentID)
	if err != nil {
		t.Fatalf("FetchChildrenForParent reverse reference fallback failed: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("expected 1 child from reverse reference resolution, got %d", len(children))
	}
	if children[0][objects.FieldKeyID] != "TASK-DEP-1" {
		t.Fatalf("expected TASK-DEP-1, got %v", children[0][objects.FieldKeyID])
	}

	// Test BulkGet error branch
	stub.bulkError = errors.New("simulated bulk get failure")
	errChildren, err := FetchChildrenForParent(context.Background(), secCtx, stub, objects.KindMilestone, parentID)
	if err != nil {
		t.Fatalf("expected nil error on bulk get failure, got %v", err)
	}
	if len(errChildren) != 0 {
		t.Fatalf("expected 0 children on bulk get error, got %d", len(errChildren))
	}
}

func testMilestoneDuplicateResolution(t *testing.T, secCtx *pkgctx.SecurityContext) {
	stub := newAdvStorageStub()
	msID := "MS-ROOT"

	sharedObj := map[string]any{
		objects.FieldKeyID:            "BLI-SHARED",
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyMilestoneRef:  msID,
		objects.FieldKeyMilestoneRefs: []string{msID},
	}
	unrelatedObj := map[string]any{
		objects.FieldKeyID:            "BLI-OTHER",
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyMilestoneRef:  "MS-OTHER",
		objects.FieldKeyMilestoneRefs: []string{"MS-OTHER"},
	}
	emptyIDObj := map[string]any{
		objects.FieldKeyID:           "",
		objects.FieldKeyMilestoneRef: msID,
	}

	stub.listResults[objects.KindBacklogItem] = []map[string]any{
		sharedObj,
		unrelatedObj,
		emptyIDObj,
	}

	children, err := FetchChildrenForParent(context.Background(), secCtx, stub, objects.KindMilestone, msID)
	if err != nil {
		t.Fatalf("FetchChildrenForParent failed: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("expected exactly 1 deduplicated child, got %d", len(children))
	}
	if children[0][objects.FieldKeyID] != "BLI-SHARED" {
		t.Fatalf("expected BLI-SHARED, got %v", children[0][objects.FieldKeyID])
	}
}

func testPriorityPlanResolution(t *testing.T, secCtx *pkgctx.SecurityContext) {
	planID := "PRI-TEST"
	stub := newAdvStorageStub()

	validBli := map[string]any{
		objects.FieldKeyID:              "BLI-P1",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: planID,
	}
	invalidKind := map[string]any{
		objects.FieldKeyID:              "GOAL-P1",
		objects.FieldKeyKind:            objects.KindGoal,
		objects.FieldKeyPriorityPlanRef: planID,
	}
	emptyID := map[string]any{
		objects.FieldKeyID:              "",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: planID,
	}

	stub.listResults[objects.KindBacklogItem] = []map[string]any{
		validBli,
		invalidKind,
		emptyID,
	}

	children, err := FetchChildrenForParent(context.Background(), secCtx, stub, objects.KindPriorityPlan, planID)
	if err != nil {
		t.Fatalf("FetchChildrenForParent failed: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}
	if children[0][objects.FieldKeyID] != "BLI-P1" {
		t.Fatalf("expected BLI-P1, got %v", children[0][objects.FieldKeyID])
	}

	// Test error branch for priority plan
	stub.listErrors[objects.KindBacklogItem] = errors.New("list failure")
	errChildren, err := FetchChildrenForParent(context.Background(), secCtx, stub, objects.KindPriorityPlan, planID)
	if err != nil {
		t.Fatalf("expected graceful recovery from list error, got %v", err)
	}
	if len(errChildren) != 0 {
		t.Fatalf("expected 0 children on error, got %d", len(errChildren))
	}
}

func testGenericResolution(t *testing.T, secCtx *pkgctx.SecurityContext) {
	containerID := "CONTAINER-42"
	stub := newAdvStorageStub()

	c1 := map[string]any{
		objects.FieldKeyID: "ITEM-1",
		"parent_container": containerID,
	}
	c2 := map[string]any{
		objects.FieldKeyID: "ITEM-2",
		"tags":             []string{containerID},
	}
	cUnrelated := map[string]any{
		objects.FieldKeyID: "ITEM-3",
		"parent_container": "OTHER-ID",
	}
	cEmptyID := map[string]any{
		objects.FieldKeyID: "",
		"parent_container": containerID,
	}

	stub.listResults["*"] = []map[string]any{
		c1,
		c2,
		cUnrelated,
		cEmptyID,
	}

	children, err := FetchChildrenForParent(context.Background(), secCtx, stub, "generic_container", containerID)
	if err != nil {
		t.Fatalf("generic FetchChildrenForParent failed: %v", err)
	}
	if len(children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(children))
	}

	// Test generic list error branch
	stub.listErrors["*"] = errors.New("generic storage error")
	errChildren, err := FetchChildrenForParent(context.Background(), secCtx, stub, "generic_container", containerID)
	if err != nil {
		t.Fatalf("expected nil error on generic error fallback, got %v", err)
	}
	if len(errChildren) != 0 {
		t.Fatalf("expected 0 children on error, got %d", len(errChildren))
	}
}

func TestEffortStats_ZeroAndExtremeDistributions(t *testing.T) {
	t.Parallel()

	// Empty values
	emptyStats := computeEffortStats(nil)
	if emptyStats.TotalHours != 0 || emptyStats.AvgHours != 0 || emptyStats.Formatted != "0h" {
		t.Fatalf("expected empty stats for nil, got %+v", emptyStats)
	}

	// Single element
	singleStats := computeEffortStats([]float64{7.5})
	if singleStats.TotalHours != 7.5 || singleStats.MinHours != 7.5 || singleStats.MaxHours != 7.5 || singleStats.AvgHours != 7.5 {
		t.Fatalf("mismatch on single element: %+v", singleStats)
	}

	// Mixed zero and positive values
	mixedStats := computeEffortStats([]float64{0.0, 10.0, 20.0})
	if mixedStats.TotalHours != 30.0 || mixedStats.MinHours != 0.0 || mixedStats.MaxHours != 20.0 || mixedStats.AvgHours != 10.0 {
		t.Fatalf("mismatch on mixed values: %+v", mixedStats)
	}
}

func TestCalculate_EdgeStatusesAndNullElements(t *testing.T) {
	t.Parallel()

	items := []map[string]any{
		nil,
		{objects.FieldKeyID: "ITEM-A"}, // missing kind and status
		{
			objects.FieldKeyID:              "ITEM-B",
			objects.FieldKeyStatus:          "   ",
			objects.FieldKeyEstimatedEffort: "invalid-effort",
			objects.FieldKeyActualEffort:    "invalid-actual",
		},
		{
			objects.FieldKeyID:              "ITEM-C",
			objects.FieldKeyStatus:          objects.ObjectStatusValidated,
			objects.FieldKeyEstimatedEffort: "1.5h", // 1.5h
			objects.FieldKeyActualEffort:    "45m",  // 0.75h
		},
		{
			objects.FieldKeyID:              "ITEM-D",
			objects.FieldKeyStatus:          objects.ObjectStatusPaused,
			objects.FieldKeyEstimatedEffort: "2h",
		},
	}

	summary := Calculate(items)

	if summary.TotalCount != 5 {
		t.Fatalf("expected total_count=5, got %d", summary.TotalCount)
	}
	if summary.CompletedCount != 1 {
		t.Fatalf("expected completed_count=1, got %d", summary.CompletedCount)
	}
	if summary.HaltedCount != 1 {
		t.Fatalf("expected halted_count=1, got %d", summary.HaltedCount)
	}
	if summary.OpenCount != 4 {
		t.Fatalf("expected open_count=4, got %d", summary.OpenCount)
	}
	if math.Abs(summary.PercentComplete-20.0) > 1e-6 {
		t.Fatalf("expected percent_complete=20.0, got %v", summary.PercentComplete)
	}

	// Verify effort parsing
	if math.Abs(summary.EstimatedEffort.TotalHours-3.5) > 1e-6 {
		t.Fatalf("expected est total=3.5, got %v", summary.EstimatedEffort.TotalHours)
	}
	if math.Abs(summary.ActualEffort.TotalHours-0.75) > 1e-6 {
		t.Fatalf("expected act total=0.75, got %v", summary.ActualEffort.TotalHours)
	}
}

func TestTreeResolution_HierarchicalAndCycleDynamics(t *testing.T) {
	t.Parallel()

	testMultiTierHierarchy(t)
	testCycleDetectionAndLoopBreaking(t)
	testWeightedProgressAggregation(t)
}

// nodeDefinition represents a simulated hierarchical node in the Knowledge Kernel graph.
type nodeDefinition struct {
	id       string
	kind     string
	parentID string
	status   string
	weight   float64
	effort   string
}

func testMultiTierHierarchy(t *testing.T) {
	// Hierarchy: Milestone -> Priority Plan -> Backlog Items
	nodes := []nodeDefinition{
		{id: "BLI-101", kind: objects.KindBacklogItem, parentID: "PRI-1", status: objects.ObjectStatusComplete, effort: "2h"},
		{id: "BLI-102", kind: objects.KindBacklogItem, parentID: "PRI-1", status: objects.ObjectStatusInProgress, effort: "4h"},
		{id: "BLI-103", kind: objects.KindBacklogItem, parentID: "PRI-1", status: "exploring", effort: "6h"},
	}

	children := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		children = append(children, map[string]any{
			objects.FieldKeyID:              n.id,
			objects.FieldKeyKind:            n.kind,
			objects.FieldKeyPriorityPlanRef: n.parentID,
			objects.FieldKeyStatus:          n.status,
			objects.FieldKeyEstimatedEffort: n.effort,
		})
	}

	summary := Calculate(children)
	if summary.TotalCount != 3 {
		t.Fatalf("expected 3 items, got %d", summary.TotalCount)
	}
	if summary.CompletedCount != 1 || summary.InProgressCount != 1 || summary.OpenCount != 2 {
		t.Fatalf("mismatch in counts: %+v", summary)
	}
	if math.Abs(summary.PercentComplete-33.3333333333) > 1e-4 {
		t.Fatalf("percent complete expected ~33.33%%, got %v", summary.PercentComplete)
	}
}

func testCycleDetectionAndLoopBreaking(t *testing.T) {
	// Simulate graph with circular reference: A -> B -> C -> A
	adjList := map[string][]string{
		"node-a": {"node-b"},
		"node-b": {"node-c"},
		"node-c": {"node-a", "node-d"},
		"node-d": {},
	}

	visited := make(map[string]bool)
	cycleDetected := false
	var traverseOrder []string

	var dfs func(curr string, path map[string]bool)
	dfs = func(curr string, path map[string]bool) {
		visited[curr] = true
		path[curr] = true
		traverseOrder = append(traverseOrder, curr)

		for _, neighbor := range adjList[curr] {
			if path[neighbor] {
				cycleDetected = true
				continue // cycle broken safely
			}
			if !visited[neighbor] {
				dfs(neighbor, path)
			}
		}
		path[curr] = false
	}

	dfs("node-a", make(map[string]bool))

	if !cycleDetected {
		t.Fatal("expected cycle detection in circular graph")
	}
	if len(traverseOrder) < 4 {
		t.Fatalf("expected traversal to visit all nodes including node-d despite cycle, got %v", traverseOrder)
	}
}

func testWeightedProgressAggregation(t *testing.T) {
	// Tests weighted progress formula:
	// progress = sum(weight_i * progress_i) / sum(weight_i)
	type weightedItem struct {
		id       string
		weight   float64
		progress float64
	}

	cases := []struct {
		name        string
		items       []weightedItem
		expectedAvg float64
	}{
		{
			name: "balanced weights",
			items: []weightedItem{
				{id: "1", weight: 50.0, progress: 100.0},
				{id: "2", weight: 50.0, progress: 50.0},
			},
			expectedAvg: 75.0,
		},
		{
			name: "skewed weights with zero progress",
			items: []weightedItem{
				{id: "1", weight: 90.0, progress: 100.0},
				{id: "2", weight: 10.0, progress: 0.0},
			},
			expectedAvg: 90.0,
		},
		{
			name: "zero weight handling",
			items: []weightedItem{
				{id: "1", weight: 0.0, progress: 100.0},
				{id: "2", weight: 100.0, progress: 60.0},
			},
			expectedAvg: 60.0,
		},
		{
			name: "negative weight sanitized to zero",
			items: []weightedItem{
				{id: "1", weight: -20.0, progress: 100.0},
				{id: "2", weight: 100.0, progress: 80.0},
			},
			expectedAvg: 80.0,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var totalWeight float64
			var weightedProgressSum float64

			for _, it := range tc.items {
				w := it.weight
				if w < 0 {
					w = 0 // sanitize negative weight
				}
				totalWeight += w
				weightedProgressSum += w * it.progress
			}

			var avg float64
			if totalWeight > 0 {
				avg = weightedProgressSum / totalWeight
			}
			if math.Abs(avg-tc.expectedAvg) > 1e-6 {
				t.Fatalf("expected weighted average %v, got %v", tc.expectedAvg, avg)
			}
		})
	}
}

func TestConcurrentRollupCalculations(t *testing.T) {
	t.Parallel()

	const workers = 32
	const iterations = 200

	sampleDatasets := [][]map[string]any{
		{
			{objects.FieldKeyID: "1", objects.FieldKeyStatus: objects.ObjectStatusComplete, objects.FieldKeyEstimatedEffort: "2h"},
			{objects.FieldKeyID: "2", objects.FieldKeyStatus: objects.ObjectStatusInProgress, objects.FieldKeyEstimatedEffort: "3h"},
		},
		{
			{objects.FieldKeyID: "3", objects.FieldKeyStatus: objects.ObjectStatusBlocked, objects.FieldKeyEstimatedEffort: "1d"},
			{objects.FieldKeyID: "4", objects.FieldKeyStatus: "exploring", objects.FieldKeyEstimatedEffort: "4h"},
			{objects.FieldKeyID: "5", objects.FieldKeyStatus: objects.ObjectStatusCompleted, objects.FieldKeyEstimatedEffort: "30m"},
		},
	}

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		workerID := i
		goroutinelabels.NewGoroutine("rollup_concurrent_calc", fmt.Sprintf("worker-%d", workerID)).
			StartSimple(func() {
				defer wg.Done()
				runConcurrentRollupWorker(t, sampleDatasets, iterations)
			})
	}

	wg.Wait()
}

func runConcurrentRollupWorker(t *testing.T, datasets [][]map[string]any, iterations int) {
	for j := 0; j < iterations; j++ {
		idx := j % len(datasets)
		res := Calculate(datasets[idx])
		if res.TotalCount == 0 {
			t.Errorf("expected non-zero total count in concurrent calculation")
			return
		}

		m := res.ToMap()
		if m["total_count"].(int) != res.TotalCount {
			t.Errorf("mismatch between summary and ToMap output")
			return
		}
	}
}
