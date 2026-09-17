package workflow

import (
	"context"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

var _ workflowStorage = (*memoryWorkflowStore)(nil)

// memoryWorkflowStore is a minimal in-memory workflowStorage for isolated selection/enrichment tests.
type memoryWorkflowStore struct {
	byID map[string]map[string]any
}

func newMemoryWorkflowStore(objs ...map[string]any) *memoryWorkflowStore {
	m := &memoryWorkflowStore{byID: make(map[string]map[string]any, len(objs))}
	for _, o := range objs {
		id, _ := o[objects.FieldKeyID].(string)
		if id == "" {
			panic("memoryWorkflowStore: object missing id")
		}
		m.byID[id] = o
	}
	return m
}

func shallowCopyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func stringInSlice(s string, list []string) bool {
	for _, e := range list {
		if s == e {
			return true
		}
	}
	return false
}

func asStringSliceFromAny(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func fieldMatches(actual any, cond any) bool {
	switch c := cond.(type) {
	case map[string]any:
		if nin, ok := c["$nin"]; ok {
			excl := asStringSliceFromAny(nin)
			st, _ := actual.(string)
			return !stringInSlice(st, excl)
		}
		if inVal, ok := c["$in"]; ok {
			incl := asStringSliceFromAny(inVal)
			st, _ := actual.(string)
			return stringInSlice(st, incl)
		}
		return false
	default:
		return valuesEqualWorkflowFilter(actual, c)
	}
}

func valuesEqualWorkflowFilter(a, b any) bool {
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case float64:
		switch bv := b.(type) {
		case float64:
			return av == bv
		case int:
			return av == float64(bv)
		case int64:
			return av == float64(bv)
		}
	case int:
		switch bv := b.(type) {
		case int:
			return av == bv
		case float64:
			return float64(av) == bv
		}
	}
	return a == b
}

func matchesListFilter(obj map[string]any, lf storage.ListFilter) bool {
	k, _ := obj[objects.FieldKeyKind].(string)
	if k != lf.Kind {
		return false
	}
	for field, want := range lf.Filters {
		if !fieldMatches(obj[field], want) {
			return false
		}
	}
	return true
}

func (m *memoryWorkflowStore) Read(_ context.Context, _ *pkgctx.SecurityContext, id string) (map[string]any, error) {
	obj, ok := m.byID[id]
	if !ok {
		return nil, nil
	}
	return shallowCopyMap(obj), nil
}

func (m *memoryWorkflowStore) List(_ context.Context, _ *pkgctx.SecurityContext, _ *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	var out []map[string]any
	for _, obj := range m.byID {
		if matchesListFilter(obj, filter) {
			out = append(out, shallowCopyMap(obj))
		}
	}
	return &storage.QueryResult{Objects: out}, nil
}

func (m *memoryWorkflowStore) Count(_ context.Context, _ *pkgctx.SecurityContext, filter storage.ListFilter) (int, error) {
	n := 0
	for _, obj := range m.byID {
		if matchesListFilter(obj, filter) {
			n++
		}
	}
	return n, nil
}

func (m *memoryWorkflowStore) Update(_ context.Context, _ *storage.SecurityContext, id string, data map[string]any) error {
	obj, ok := m.byID[id]
	if !ok {
		return nil // Or an error depending on storage semantics
	}
	for k, v := range data {
		obj[k] = v
	}
	m.byID[id] = obj
	return nil
}

// simulateWorkflowNextBacklogBranch mirrors runNext when no policy interrupt or active convergence
// applies: resolve backlog selection, apply decision, then enrich on the backlog branch.
func simulateWorkflowNextBacklogBranch(ctx context.Context, store workflowStorage) workflowNextResult {
	result := workflowNextResult{
		Precedence:         []string{decisionCriticalPolicyInterrupt, decisionActiveConvergence, decisionPriorityPlanBacklog},
		Decision:           decisionNone,
		Rationale:          decisionSpecs[decisionNone].Rationale,
		RecommendedCommand: decisionSpecs[decisionNone].CommandBuilder(emptyValue),
		Sources:            []sourceRef{},
	}
	planID, item := selectBacklogNext(ctx, store)
	applyDecision(&result, nil, nil, planID, item)
	result.OpenQuestionsCount = countOpenQuestions(ctx, store)
	if result.Decision == decisionPriorityPlanBacklog && planID != emptyValue && item != nil {
		enrichPriorityPlanBacklog(ctx, store, &result, planID, item)
	}
	return result
}

func TestIsolated_resolveCurrentPlanID_PrefersLowerActiveOrder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:        objects.KindPriorityPlan,
			objects.FieldKeyID:          "PRI-A",
			objects.FieldKeyStatus:      statusInProcess,
			objects.FieldKeyActiveOrder: 2,
			objects.FieldKeyUpdatedAt:   "2026-01-02T00:00:00Z",
		},
		map[string]any{
			objects.FieldKeyKind:        objects.KindPriorityPlan,
			objects.FieldKeyID:          "PRI-B",
			objects.FieldKeyStatus:      statusInProcess,
			objects.FieldKeyActiveOrder: 1,
			objects.FieldKeyUpdatedAt:   "2026-01-01T00:00:00Z",
		},
	)
	if got := resolveCurrentPlanID(ctx, store); got != "PRI-B" {
		t.Fatalf("expected PRI-B (active_order 1), got %q", got)
	}
}

func TestIsolated_resolveCurrentPlanID_FallbackRecencyWhenNoActiveOrder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:      objects.KindPriorityPlan,
			objects.FieldKeyID:        "PRI-OLD",
			objects.FieldKeyStatus:    statusInProcess,
			objects.FieldKeyUpdatedAt: "2026-01-01T00:00:00Z",
		},
		map[string]any{
			objects.FieldKeyKind:      objects.KindPriorityPlan,
			objects.FieldKeyID:        "PRI-NEW",
			objects.FieldKeyStatus:    statusInProcess,
			objects.FieldKeyUpdatedAt: "2026-01-03T00:00:00Z",
		},
	)
	if got := resolveCurrentPlanID(ctx, store); got != "PRI-NEW" {
		t.Fatalf("expected newer plan, got %q", got)
	}
}

func TestIsolated_selectBacklogNext_PrefersP0OverP1(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:        objects.KindPriorityPlan,
			objects.FieldKeyID:          "PRI-1",
			objects.FieldKeyStatus:      statusInProcess,
			objects.FieldKeyActiveOrder: 1,
			objects.FieldKeyUpdatedAt:   "2026-01-01T00:00:00Z",
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-P1",
			objects.FieldKeyPriorityPlanRef: "PRI-1",
			objects.FieldKeyPriorityTier:    "p1",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyUpdatedAt:       "2026-01-10T00:00:00Z",
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-P0",
			objects.FieldKeyPriorityPlanRef: "PRI-1",
			objects.FieldKeyPriorityTier:    "p0",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyUpdatedAt:       "2026-01-01T00:00:00Z",
		},
	)
	planID, item := selectBacklogNext(ctx, store)
	if planID != "PRI-1" || item == nil || item.ID != "BLI-P0" {
		t.Fatalf("expected BLI-P0, got planID=%q item=%v", planID, item)
	}
	if item.PriorityTier != "P0" {
		t.Fatalf("tier: got %q", item.PriorityTier)
	}
}

func TestIsolated_selectBacklogNext_ExcludesTerminalStatuses(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:      objects.KindPriorityPlan,
			objects.FieldKeyID:        "PRI-1",
			objects.FieldKeyStatus:    statusInProcess,
			objects.FieldKeyUpdatedAt: "2026-01-01T00:00:00Z",
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-DONE",
			objects.FieldKeyPriorityPlanRef: "PRI-1",
			objects.FieldKeyPriorityTier:    "p0",
			objects.FieldKeyStatus:          statusComplete,
			objects.FieldKeyUpdatedAt:       "2026-01-10T00:00:00Z",
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-OPEN",
			objects.FieldKeyPriorityPlanRef: "PRI-1",
			objects.FieldKeyPriorityTier:    "p1",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyUpdatedAt:       "2026-01-01T00:00:00Z",
		},
	)
	planID, item := selectBacklogNext(ctx, store)
	if planID != "PRI-1" || item == nil || item.ID != "BLI-OPEN" {
		t.Fatalf("expected BLI-OPEN, got planID=%q item=%v", planID, item)
	}
}

func TestIsolated_selectActiveConvergence_PicksMostRecentlyUpdated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:      objects.KindConvergenceSession,
			objects.FieldKeyID:        "CVS-OLD",
			objects.FieldKeyTitle:     "old",
			objects.FieldKeyStatus:    statusInProcess,
			objects.FieldKeyUpdatedAt: "2026-01-01T00:00:00Z",
		},
		map[string]any{
			objects.FieldKeyKind:      objects.KindConvergenceSession,
			objects.FieldKeyID:        "CVS-NEW",
			objects.FieldKeyTitle:     "newer",
			objects.FieldKeyStatus:    statusInProcess,
			objects.FieldKeyUpdatedAt: "2026-02-01T00:00:00Z",
		},
	)
	cv := selectActiveConvergence(ctx, store)
	if cv == nil || cv.ID != "CVS-NEW" {
		t.Fatalf("expected CVS-NEW, got %#v", cv)
	}
}

func TestIsolated_enrichPriorityPlanBacklog_IncludesConvergenceSessionFields(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:        objects.KindPriorityPlan,
			objects.FieldKeyID:          "PRI-CVS",
			objects.FieldKeyTitle:       "Plan",
			objects.FieldKeyDescription: "Plan body.",
		},
		map[string]any{
			objects.FieldKeyKind:                      objects.KindBacklogItem,
			objects.FieldKeyID:                        "BLI-CVS",
			objects.FieldKeyDescription:               "Item body.",
			objects.FieldKeyConvergenceSessionRef:     "CVS-EX-1",
			objects.FieldKeyConvergenceSessionProfile: "vetting_matrix",
		},
	)
	result := &workflowNextResult{}
	item := &backlogInfo{ID: "BLI-CVS", PriorityTier: "P0"}
	enrichPriorityPlanBacklog(ctx, store, result, "PRI-CVS", item)
	if result.ConvergenceSessionRef != "CVS-EX-1" || result.ConvergenceSessionProfile != "vetting_matrix" {
		t.Fatalf("convergence fields: ref=%q profile=%q", result.ConvergenceSessionRef, result.ConvergenceSessionProfile)
	}
}

func TestIsolated_countOpenQuestions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:   objects.KindQuestion,
			objects.FieldKeyID:     "QUE-A",
			objects.FieldKeyStatus: statusQuestionOpen,
		},
		map[string]any{
			objects.FieldKeyKind:   objects.KindQuestion,
			objects.FieldKeyID:     "QUE-B",
			objects.FieldKeyStatus: statusQuestionOpen,
		},
		map[string]any{
			objects.FieldKeyKind:   objects.KindQuestion,
			objects.FieldKeyID:     "QUE-C",
			objects.FieldKeyStatus: "resolved",
		},
	)
	if n := countOpenQuestions(ctx, store); n != 2 {
		t.Fatalf("open questions: got %d want 2", n)
	}
}

func TestIsolated_enrichPriorityPlanBacklog_FillsSummariesAndGoalRefs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:        objects.KindPriorityPlan,
			objects.FieldKeyID:          "PRI-X",
			objects.FieldKeyTitle:       "Plan title",
			objects.FieldKeyDescription: "Plan body text for enrichment.",
		},
		map[string]any{
			objects.FieldKeyKind:        objects.KindBacklogItem,
			objects.FieldKeyID:          "BLI-X",
			objects.FieldKeyDescription: "Item body text.",
			objects.FieldKeyGoalRefs:    []string{" G1 ", "G2"},
		},
	)
	result := &workflowNextResult{}
	item := &backlogInfo{ID: "BLI-X", PriorityTier: "P0"}
	enrichPriorityPlanBacklog(ctx, store, result, "PRI-X", item)
	if result.BacklogPriorityTier != "P0" {
		t.Fatalf("tier: got %q", result.BacklogPriorityTier)
	}
	if result.PriorityPlanTitle != "Plan title" {
		t.Fatalf("plan title: got %q", result.PriorityPlanTitle)
	}
	if result.PriorityPlanSummary == "" || result.BacklogItemSummary == "" {
		t.Fatalf("expected summaries, plan=%q item=%q", result.PriorityPlanSummary, result.BacklogItemSummary)
	}
	if len(result.GoalRefs) != 2 || result.GoalRefs[0] != "G1" || result.GoalRefs[1] != "G2" {
		t.Fatalf("goal_refs: %#v", result.GoalRefs)
	}
}

func TestIsolated_selectBacklogNext_EndToEndWithEnrichment(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:        objects.KindPriorityPlan,
			objects.FieldKeyID:          "PRI-E2E",
			objects.FieldKeyStatus:      statusInProcess,
			objects.FieldKeyTitle:       "E2E plan",
			objects.FieldKeyDescription: "Plan desc.",
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-E2E",
			objects.FieldKeyPriorityPlanRef: "PRI-E2E",
			objects.FieldKeyPriorityTier:    "p0",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyUpdatedAt:       "2026-01-05T00:00:00Z",
			objects.FieldKeyDescription:     "Do the thing.",
			objects.FieldKeyGoalRefs:        []string{"G-E2E"},
		},
	)
	planID, item := selectBacklogNext(ctx, store)
	if planID != "PRI-E2E" || item == nil {
		t.Fatalf("selection: planID=%q item=%v", planID, item)
	}
	result := &workflowNextResult{}
	enrichPriorityPlanBacklog(ctx, store, result, planID, item)
	if result.PriorityPlanTitle != "E2E plan" || !strings.Contains(result.BacklogItemSummary, "Do the thing") {
		t.Fatalf("enrichment incomplete: %#v", result)
	}
	if len(result.GoalRefs) != 1 || result.GoalRefs[0] != "G-E2E" {
		t.Fatalf("goal_refs: %#v", result.GoalRefs)
	}
}

// TestIsolated_workflowNextCyclesPlanAndBacklogToCompletion simulates repeated workflow-next-equivalent
// evaluations while mutating storage: two P0 items (newer first), then both complete, then plan complete.
// Asserts recommended commands and enrichment stay aligned with the current best backlog item, then
// fall back to the global "none" decision with list-backlog guidance.
func TestIsolated_workflowNextCyclesPlanAndBacklogToCompletion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	planID := "PRI-CYCLE-WORKFLOW"
	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:        objects.KindPriorityPlan,
			objects.FieldKeyID:          planID,
			objects.FieldKeyStatus:      statusInProcess,
			objects.FieldKeyTitle:       "Cycle plan",
			objects.FieldKeyDescription: "Plan body for enrichment signal.",
			objects.FieldKeyActiveOrder: 1,
			objects.FieldKeyUpdatedAt:   "2026-04-01T00:00:00Z",
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-CYCLE-1",
			objects.FieldKeyPriorityPlanRef: planID,
			objects.FieldKeyPriorityTier:    "p0",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyUpdatedAt:       "2026-04-02T00:00:00Z",
			objects.FieldKeyDescription:     "First milestone description.",
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-CYCLE-2",
			objects.FieldKeyPriorityPlanRef: planID,
			objects.FieldKeyPriorityTier:    "p0",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyUpdatedAt:       "2026-04-01T00:00:00Z",
			objects.FieldKeyDescription:     "Second milestone description.",
		},
	)

	r1 := simulateWorkflowNextBacklogBranch(ctx, store)
	if r1.Decision != decisionPriorityPlanBacklog {
		t.Fatalf("step1 decision: got %q want %q", r1.Decision, decisionPriorityPlanBacklog)
	}
	if !strings.Contains(r1.RecommendedCommand, "BLI-CYCLE-1") {
		t.Fatalf("step1 command should reference BLI-CYCLE-1: %q", r1.RecommendedCommand)
	}
	if !strings.Contains(r1.BacklogItemSummary, "First milestone") {
		t.Fatalf("step1 backlog item summary unclear: %q", r1.BacklogItemSummary)
	}
	if r1.PriorityPlanTitle != "Cycle plan" || !strings.Contains(r1.PriorityPlanSummary, "Plan body") {
		t.Fatalf("step1 plan enrichment weak: title=%q plan_summary=%q", r1.PriorityPlanTitle, r1.PriorityPlanSummary)
	}

	store.byID["BLI-CYCLE-1"][objects.FieldKeyStatus] = statusComplete

	r2 := simulateWorkflowNextBacklogBranch(ctx, store)
	if r2.Decision != decisionPriorityPlanBacklog {
		t.Fatalf("step2 decision: got %q", r2.Decision)
	}
	if !strings.Contains(r2.RecommendedCommand, "BLI-CYCLE-2") {
		t.Fatalf("step2 command should reference BLI-CYCLE-2: %q", r2.RecommendedCommand)
	}
	if !strings.Contains(r2.BacklogItemSummary, "Second milestone") {
		t.Fatalf("step2 backlog item summary unclear: %q", r2.BacklogItemSummary)
	}

	store.byID["BLI-CYCLE-2"][objects.FieldKeyStatus] = statusComplete

	r3 := simulateWorkflowNextBacklogBranch(ctx, store)
	if r3.Decision != decisionNone {
		t.Fatalf("step3 expected %q, got %q", decisionNone, r3.Decision)
	}
	if r3.Rationale != rationaleNoAction {
		t.Fatalf("step3 rationale: got %q", r3.Rationale)
	}
	if r3.RecommendedCommand != cmdObjectListInProgressBacklog {
		t.Fatalf("step3 recommended command: got %q", r3.RecommendedCommand)
	}

	store.byID[planID][objects.FieldKeyStatus] = statusComplete

	r4 := simulateWorkflowNextBacklogBranch(ctx, store)
	if r4.Decision != decisionNone {
		t.Fatalf("step4 expected %q after plan complete, got %q", decisionNone, r4.Decision)
	}
	if r4.Rationale != rationaleNoAction {
		t.Fatalf("step4 rationale: got %q", r4.Rationale)
	}
}
