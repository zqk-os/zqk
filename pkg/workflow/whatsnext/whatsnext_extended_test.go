package whatsnext

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

type extendedMockStorage struct {
	storagepkg.ObjectStorageProvider
	items  map[string]map[string]any
	byKind map[string][]map[string]any
}

func newExtendedMockStorage() *extendedMockStorage {
	return &extendedMockStorage{
		items:  make(map[string]map[string]any),
		byKind: make(map[string][]map[string]any),
	}
}

func (m *extendedMockStorage) add(obj map[string]any) {
	id, _ := obj[objects.FieldKeyID].(string)
	kind, _ := obj[objects.FieldKeyKind].(string)
	if id != "" {
		m.items[id] = obj
	}
	if kind != "" {
		m.byKind[kind] = append(m.byKind[kind], obj)
	}
}

func (m *extendedMockStorage) Read(ctx context.Context, secCtx *storagepkg.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.items[id]; ok {
		return obj, nil
	}
	return nil, nil
}

func (m *extendedMockStorage) List(ctx context.Context, secCtx *storagepkg.SecurityContext, storageCtx *storagepkg.StorageContext, filter storagepkg.ListFilter) (*storagepkg.QueryResult, error) {
	var results []map[string]any
	candidates := m.byKind[filter.Kind]
	if len(candidates) == 0 && filter.Kind == "" {
		for _, v := range m.items {
			candidates = append(candidates, v)
		}
	}
	for _, obj := range candidates {
		match := true
		for k, v := range filter.Filters {
			objVal, exists := obj[k]
			if opMap, ok := v.(map[string]any); ok {
				if ninList, hasNin := opMap["$nin"].([]any); hasNin {
					for _, item := range ninList {
						if objVal == item {
							match = false
							break
						}
					}
				}
				if neVal, hasNe := opMap["$ne"]; hasNe {
					if objVal == neVal {
						match = false
						break
					}
				}
				if inList, hasIn := opMap["$in"].([]any); hasIn {
					found := false
					for _, item := range inList {
						if objVal == item {
							found = true
							break
						}
					}
					if !found {
						match = false
						break
					}
				}
			} else if !exists || objVal != v {
				match = false
				break
			}
		}
		if match {
			results = append(results, obj)
		}
	}
	return &storagepkg.QueryResult{Objects: results}, nil
}

func TestWhatsNextExtended_Execute(t *testing.T) {
	tmp := t.TempDir()
	mock := newExtendedMockStorage()

	plan := map[string]any{
		objects.FieldKeyID:          "PRI-EXT-01",
		objects.FieldKeyKind:        objects.KindPriorityPlan,
		objects.FieldKeyTitle:       "Extended Priority Plan",
		objects.FieldKeyStatus:      "in_progress",
		objects.FieldKeyActiveOrder: 0,
		objects.FieldKeyWorkflowRef: "WFL-TEST",
	}
	mock.add(plan)

	bli1 := map[string]any{
		objects.FieldKeyID:              "BLI-001",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: "PRI-EXT-01",
		objects.FieldKeyStatus:          "in_progress",
	}
	bli2 := map[string]any{
		objects.FieldKeyID:              "BLI-002",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: "PRI-EXT-01",
		objects.FieldKeyStatus:          "completed",
	}
	mock.add(bli1)
	mock.add(bli2)

	wfl := map[string]any{
		objects.FieldKeyID:          "WFL-TEST",
		objects.FieldKeyKind:        objects.KindWorkflow,
		objects.FieldKeyTitle:       "Test Workflow",
		objects.FieldKeyStatus:      "active",
		"trigger_on":                []string{"whats-next"},
		objects.FieldKeyDescription: "Automated test workflow",
	}
	mock.add(wfl)

	cvs := map[string]any{
		objects.FieldKeyID:           "CVS-001",
		objects.FieldKeyKind:         objects.KindConvergenceSession,
		objects.FieldKeyTitle:        "Data Cell Session",
		objects.FieldKeyStatus:       "active",
		objects.FieldKeyCurrentPhase: "execute",
	}
	mock.add(cvs)

	req := &QueryRequest{
		PriorityPlanID:    "PRI-EXT-01",
		SkipPersonaFilter: true,
		SkipMeasure:       true,
	}

	out, err := Execute(context.Background(), mock, tmp, req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out.PriorityPlan == nil || out.PriorityPlan.ID != "PRI-EXT-01" {
		t.Fatalf("expected PriorityPlan PRI-EXT-01, got %v", out.PriorityPlan)
	}
	if out.BacklogCountsByStatus["in_progress"] != 1 {
		t.Fatalf("expected 1 in_progress, got %d", out.BacklogCountsByStatus["in_progress"])
	}
	if len(out.ConvergenceSessionsActive) != 1 {
		t.Fatalf("expected 1 active convergence session, got %d", len(out.ConvergenceSessionsActive))
	}
}

func TestWhatsNextExtended_ExecuteWithSkipMeasureReason(t *testing.T) {
	tmp := t.TempDir()
	mock := newExtendedMockStorage()

	req := &QueryRequest{
		SkipMeasure: true,
	}

	out, err := Execute(context.Background(), mock, tmp, req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out.MeasureSkipReason != "skipped via --skip-measure" {
		t.Fatalf("expected skipped reason, got %s", out.MeasureSkipReason)
	}
}

func TestWhatsNextExtended_ResolvePriorityPlan(t *testing.T) {
	ctx := context.Background()

	// 1. Empty storage -> no plan found
	emptyMock := newExtendedMockStorage()
	planID, planSumm, _ := resolvePriorityPlanForWhatsNext(ctx, emptyMock, "NON_EXISTENT", nil)
	if planID != "" || planSumm != nil {
		t.Fatalf("expected empty for non-existent plan, got %s, %v", planID, planSumm)
	}

	mock := newExtendedMockStorage()
	stratPlan := map[string]any{
		objects.FieldKeyID:     "STRAT-01",
		objects.FieldKeyKind:   objects.KindStrategicPlan,
		objects.FieldKeyTitle:  "Strat Plan",
		objects.FieldKeyStatus: "active",
	}
	mock.add(stratPlan)

	// Explicit request found
	planID, planSumm, _ = resolvePriorityPlanForWhatsNext(ctx, mock, "STRAT-01", nil)
	if planID != "STRAT-01" || planSumm == nil {
		t.Fatalf("expected STRAT-01, got %s, %v", planID, planSumm)
	}

	// Selection from candidate plans
	p1 := map[string]any{
		objects.FieldKeyID:          "PRI-GROOM",
		objects.FieldKeyKind:        objects.KindPriorityPlan,
		objects.FieldKeyTitle:       "Grooming Plan",
		objects.FieldKeyStatus:      "grooming",
		objects.FieldKeyActiveOrder: 5,
	}
	p2 := map[string]any{
		objects.FieldKeyID:          "PRI-ACTIVE",
		objects.FieldKeyKind:        objects.KindPriorityPlan,
		objects.FieldKeyTitle:       "Active Plan",
		objects.FieldKeyStatus:      "active",
		objects.FieldKeyActiveOrder: 1,
	}
	mock.add(p1)
	mock.add(p2)

	planID, planSumm, activePlans := resolvePriorityPlanForWhatsNext(ctx, mock, "", nil)
	if planID != "PRI-ACTIVE" {
		t.Fatalf("expected PRI-ACTIVE selected as best eligible, got %s", planID)
	}
	if len(activePlans) == 0 {
		t.Fatal("expected non-empty activePlans")
	}
}

func TestWhatsNextExtended_HasPersonaMatch(t *testing.T) {
	// No persona filter
	if !hasPersonaMatch(map[string]any{}, nil) {
		t.Fatal("empty personaIDs should match")
	}

	// Nil persona_refs
	if !hasPersonaMatch(map[string]any{}, []string{"dev"}) {
		t.Fatal("nil persona_refs should match")
	}

	// Empty slice
	if !hasPersonaMatch(map[string]any{objects.FieldKeyPersonaRefs: []any{}}, []string{"dev"}) {
		t.Fatal("empty refs should match")
	}
	if !hasPersonaMatch(map[string]any{objects.FieldKeyPersonaRefs: []string{}}, []string{"dev"}) {
		t.Fatal("empty string refs should match")
	}

	// []any matching
	if !hasPersonaMatch(map[string]any{objects.FieldKeyPersonaRefs: []any{"DEV", "ARCH"}}, []string{"dev"}) {
		t.Fatal("case-insensitive match expected for []any")
	}
	// []any non-matching
	if hasPersonaMatch(map[string]any{objects.FieldKeyPersonaRefs: []any{"QA"}}, []string{"dev"}) {
		t.Fatal("non-matching persona should return false")
	}

	// []string matching
	if !hasPersonaMatch(map[string]any{objects.FieldKeyPersonaRefs: []string{"dev"}}, []string{"dev"}) {
		t.Fatal("match expected for []string")
	}
	// []string non-matching
	if hasPersonaMatch(map[string]any{objects.FieldKeyPersonaRefs: []string{"qa"}}, []string{"dev"}) {
		t.Fatal("non-matching []string should return false")
	}
}

func TestWhatsNextExtended_DefaultMeasureSessionID(t *testing.T) {
	rows := []WhatsNextCVSRow{
		{ID: "CVS-1", Title: "Regular Session"},
		{ID: "CVS-2", Title: "Data Cell Session"},
	}
	id := pickDefaultMeasureSessionID(rows)
	if id != "CVS-2" {
		t.Fatalf("expected CVS-2 with data cell, got %s", id)
	}

	rows2 := []WhatsNextCVSRow{
		{ID: "CVS-1", Title: "Regular Session"},
	}
	id2 := pickDefaultMeasureSessionID(rows2)
	if id2 != "CVS-1" {
		t.Fatalf("expected CVS-1, got %s", id2)
	}

	if id3 := pickDefaultMeasureSessionID(nil); id3 != "" {
		t.Fatalf("expected empty for nil rows, got %s", id3)
	}
}

func TestWhatsNextExtended_SummarizePriorityPlan(t *testing.T) {
	id, summ := summarizePriorityPlan(nil)
	if id != "" || summ != nil {
		t.Fatalf("expected empty for nil, got %q, %v", id, summ)
	}

	id, summ = summarizePriorityPlan(map[string]any{
		objects.FieldKeyID:     "PRI-TEST",
		objects.FieldKeyTitle:  "Title",
		objects.FieldKeyStatus: "active",
	})
	if id != "PRI-TEST" || summ == nil || summ.Title != "Title" {
		t.Fatalf("expected PRI-TEST summary, got %s, %v", id, summ)
	}
}

func TestWhatsNextExtended_ObserverTips(t *testing.T) {
	tips := getObserverTips()
	_ = tips
}

func TestWhatsNextExtended_IsTPMProcessAdminSeat(t *testing.T) {
	if IsTPMProcessAdminSeat("") {
		t.Fatal("empty should not be TPM")
	}
	if !IsTPMProcessAdminSeat("peer-tpm-01") {
		t.Fatal("peer-tpm-01 should be TPM")
	}
	if !IsTPMProcessAdminSeat("tpm") {
		t.Fatal("tpm should be TPM")
	}
	if !IsTPMProcessAdminSeat("dev-tpm") {
		t.Fatal("dev-tpm should be TPM")
	}
	if IsTPMProcessAdminSeat("developer-seat") {
		t.Fatal("developer-seat should not be TPM")
	}
}

func TestWhatsNextExtended_ApplySeatOperatingMode(t *testing.T) {
	ApplySeatOperatingMode(nil, "tpm", "")

	amb := &KernelAmbience{}
	ApplySeatOperatingMode(amb, "tpm", "hint")
	if amb.SeatMode != SeatModeTPMProcessAdmin {
		t.Fatalf("expected SeatModeTPMProcessAdmin, got %v", amb.SeatMode)
	}

	ApplySeatOperatingMode(amb, "dev-worker", "hint")
	if amb.SeatMode != SeatModePeerExecution {
		t.Fatalf("expected SeatModePeerExecution, got %v", amb.SeatMode)
	}
}

func TestWhatsNextExtended_EnrichWorkflowsAndStaleTasks(t *testing.T) {
	ctx := context.Background()
	mock := newExtendedMockStorage()

	wfl := map[string]any{
		objects.FieldKeyID:                 "WFL-DEPLOY",
		objects.FieldKeyKind:               objects.KindWorkflow,
		objects.FieldKeyTitle:              "Deploy Pipeline",
		objects.FieldKeyStatus:             "active",
		"trigger_on":                       []string{"whats-next"},
		objects.FieldKeyRelatedObjectRefs: []string{"WFL-EXTRA"},
	}
	mock.add(wfl)

	task := map[string]any{
		objects.FieldKeyID:              "TASK-001",
		objects.FieldKeyKind:            objects.KindAgentTask,
		objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
		objects.FieldKeyPriorityPlanRef: "PRI-1",
		objects.FieldKeyClaimedBy:       "agent-alpha",
		objects.FieldKeyUpdatedAt:       time.Now().Add(-5 * time.Hour).Format(time.RFC3339),
	}
	mock.add(task)

	amb := &KernelAmbience{Available: true}
	EnrichKernelAmbienceWithWorkflows(ctx, mock, amb, "PRI-1", []string{"PRI-1"})
	if len(amb.EmployedWorkflows) == 0 {
		t.Fatal("expected employed workflows to be populated")
	}

	EnrichKernelAmbienceWithStaleTasks(ctx, mock, amb, time.Now())
	if amb.StaleAgentTask == nil || amb.StaleAgentTask.TaskID != "TASK-001" {
		t.Fatalf("expected stale task TASK-001, got %v", amb.StaleAgentTask)
	}
}

func TestWhatsNextExtended_EnrichStrategicAlignmentAndMetrics(t *testing.T) {
	tmp := t.TempDir()

	// 1. Unset directories (missing cache)
	amb := &KernelAmbience{}
	EnrichStrategicAlignment(amb, tmp)
	if amb.StrategicAlignment == nil || amb.StrategicAlignment.Available {
		t.Fatal("expected unavailable alignment when cache missing")
	}

	EnrichMetricsRollup(amb, tmp)
	if amb.MetricsRollup != nil {
		t.Fatal("expected nil rollup when file missing")
	}

	// 2. Create valid files
	alignDir := filepath.Join(tmp, ".zqk", "state", "ambient")
	if err := os.MkdirAll(alignDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	alignData := map[string]any{
		"overall_alignment_score": 98.5,
		"goals_count":             5,
		"goal_work": map[string]any{
			"items_with_goals":    10,
			"items_without_goals": 2,
		},
	}
	b, _ := json.Marshal(alignData)
	_ = os.WriteFile(filepath.Join(alignDir, "align-latest.json"), b, 0644)

	rollupData := map[string]any{
		"total_runs": 100,
	}
	b2, _ := json.Marshal(rollupData)
	_ = os.WriteFile(filepath.Join(alignDir, "metrics-rollup.json"), b2, 0644)

	amb2 := &KernelAmbience{}
	EnrichStrategicAlignment(amb2, tmp)
	if amb2.StrategicAlignment == nil || !amb2.StrategicAlignment.Available {
		t.Fatal("expected available alignment")
	}
	if amb2.StrategicAlignment.AlignmentScore != 98.5 {
		t.Fatalf("expected 98.5, got %v", amb2.StrategicAlignment.AlignmentScore)
	}

	EnrichMetricsRollup(amb2, tmp)
	if amb2.MetricsRollup == nil {
		t.Fatal("expected non-nil rollup")
	}
}

func TestWhatsNextExtended_MaterializedView(t *testing.T) {
	tmp := t.TempDir()
	mv := NewWhatsNextMaterializedView(tmp)

	if mv.Name() != "whats_next" {
		t.Fatalf("expected name whats_next, got %s", mv.Name())
	}
	if mv.Engine() == nil {
		t.Fatal("expected non-nil engine")
	}

	defPayload := mv.DefaultPayload()
	if defPayload == nil {
		t.Fatal("expected non-nil default payload")
	}

	// Save to lite file and load back
	if err := mv.SaveToLiteFile(); err != nil {
		t.Fatalf("SaveToLiteFile failed: %v", err)
	}

	loaded, err := mv.LoadFromLiteFile()
	if err != nil {
		t.Fatalf("LoadFromLiteFile failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil loaded payload")
	}

	// Apply lifecycle event
	_ = mv.ApplyEvent(&lifecycle.LifecycleEvent{
		ID:       "OBJ-1",
		Kind:     "task",
		ToStatus: "done",
	})

	// SubscribeWAL with canceled context
	cancCtx, cancel := context.WithCancel(context.Background())
	cancel()
	ch := make(chan struct{})
	mv.SubscribeWAL(cancCtx, ch)
}

func TestWhatsNextExtended_Helpers(t *testing.T) {
	// firstFloat
	m := map[string]any{"rate": 42.5, "intVal": 10, "num": json.Number("99.5")}
	if val := firstFloat(m, "none", "rate"); val != 42.5 {
		t.Fatalf("expected 42.5, got %v", val)
	}
	if val := firstFloat(m, "intVal"); val != 10.0 {
		t.Fatalf("expected 10.0 from int, got %v", val)
	}
	if val := firstFloat(m, "num"); val != 99.5 {
		t.Fatalf("expected 99.5 from json.Number, got %v", val)
	}
	if val := firstFloat(m, "missing"); val != 0.0 {
		t.Fatalf("expected 0.0 from missing, got %v", val)
	}

	// firstInt
	if val := firstInt(m, "intVal"); val != 10 {
		t.Fatalf("expected 10, got %v", val)
	}
	if val := firstInt(m, "rate"); val != 42 {
		t.Fatalf("expected 42 from float, got %v", val)
	}
	if val := firstInt(m, "missing"); val != 0 {
		t.Fatalf("expected 0 from missing, got %v", val)
	}

	// stringSliceField
	slice := stringSliceField([]any{"a", "b", 123})
	if len(slice) != 2 || slice[0] != "a" || slice[1] != "b" {
		t.Fatalf("expected [a, b], got %v", slice)
	}

	// firstNonEmptyLine
	line := firstNonEmptyLine("\n  \n  hello world  \n next line")
	if line != "hello world" {
		t.Fatalf("expected 'hello world', got %q", line)
	}
}

func TestWhatsNextExtended_PlanWorkAndSeated(t *testing.T) {
	ctx := context.Background()
	mock := newExtendedMockStorage()

	bli := map[string]any{
		objects.FieldKeyID:              "BLI-WORK",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: "PRI-WORK",
		objects.FieldKeyStatus:          "in_progress",
		objects.FieldKeyPersonaRefs:     []string{"dev"},
	}
	mock.add(bli)

	if !planHasWork(ctx, mock, "PRI-WORK") {
		t.Fatal("expected plan to have work")
	}
	if planHasWork(ctx, mock, "PRI-NO-WORK") {
		t.Fatal("expected plan to have no work")
	}

	cand1 := map[string]any{objects.FieldKeyID: "PRI-WORK"}
	cand2 := map[string]any{objects.FieldKeyID: "PRI-NO-WORK"}

	filtered := preferSeatedPlansWithOpenWork(ctx, mock, []map[string]any{cand1, cand2}, []string{"dev"})
	if len(filtered) != 1 || filtered[0][objects.FieldKeyID] != "PRI-WORK" {
		t.Fatalf("expected PRI-WORK to be kept, got %v", filtered)
	}

	// When none are fueled, return candidates
	unfueled := preferSeatedPlansWithOpenWork(ctx, mock, []map[string]any{cand2}, []string{"dev"})
	if len(unfueled) != 1 {
		t.Fatal("expected fallback to all candidates when none fueled")
	}
}

func TestWhatsNextExtended_MVNoEngine(t *testing.T) {
	tmp := t.TempDir()
	mv := &WhatsNextMaterializedView{
		projectRoot: tmp,
		plans:       make(map[string]*PlanNode),
		backlogs:    make(map[string]*BacklogNode),
	}

	// Apply lifecycle event to both kinds
	mv.ApplyLifecycleEvent(&lifecycle.LifecycleEvent{
		EventType:  lifecycle.EventTypeStatusTransition,
		Kind:       objects.KindPriorityPlan,
		ID:         "PRI-1",
		FromStatus: "grooming",
		ToStatus:   "active",
	})
	// Transition existing plan
	mv.ApplyLifecycleEvent(&lifecycle.LifecycleEvent{
		EventType:  lifecycle.EventTypeStatusTransition,
		Kind:       objects.KindPriorityPlan,
		ID:         "PRI-1",
		FromStatus: "active",
		ToStatus:   "complete",
	})

	mv.ApplyLifecycleEvent(&lifecycle.LifecycleEvent{
		EventType:  lifecycle.EventTypeStatusTransition,
		Kind:       objects.KindBacklogItem,
		ID:         "BLI-1",
		FromStatus: "planned",
		ToStatus:   "in_progress",
	})
	// Transition existing BLI
	mv.ApplyLifecycleEvent(&lifecycle.LifecycleEvent{
		EventType:  lifecycle.EventTypeStatusTransition,
		Kind:       objects.KindBacklogItem,
		ID:         "BLI-1",
		FromStatus: "in_progress",
		ToStatus:   "complete",
	})

	// Save and load directly via no-engine branch
	if err := mv.SaveToLiteFile(); err != nil {
		t.Fatalf("SaveToLiteFile failed: %v", err)
	}
	loaded, err := mv.LoadFromLiteFile()
	if err != nil {
		t.Fatalf("LoadFromLiteFile failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected loaded payload")
	}
}

func TestWhatsNextExtended_WorkflowAndStringHelpers(t *testing.T) {
	// isFixtureWorkflow
	if !isFixtureWorkflow(nil, "WFL-001") {
		t.Fatal("expected true for WFL-001")
	}
	if !isFixtureWorkflow(nil, "CUSTOM-FIXTURE") {
		t.Fatal("expected true for suffix -FIXTURE")
	}
	if !isFixtureWorkflow(map[string]any{objects.FieldKeyTitle: "My Fixture Test"}, "WFL-999") {
		t.Fatal("expected true for title containing fixture")
	}
	if isFixtureWorkflow(map[string]any{objects.FieldKeyTitle: "Production"}, "WFL-PROD") {
		t.Fatal("expected false for prod workflow")
	}

	// isCoreOperationalWorkflow
	if !isCoreOperationalWorkflow("WFL-SUBAGENT-DISPATCH") {
		t.Fatal("expected true for WFL-SUBAGENT-DISPATCH")
	}
	if isCoreOperationalWorkflow("WFL-RANDOM") {
		t.Fatal("expected false for WFL-RANDOM")
	}

	// truncateRunes
	if s := truncateRunes("", 10); s != "" {
		t.Fatalf("expected empty, got %q", s)
	}
	if s := truncateRunes("hello", 0); s != "hello" {
		t.Fatalf("expected hello for max 0, got %q", s)
	}
	if s := truncateRunes("hello", 10); s != "hello" {
		t.Fatalf("expected hello for max 10, got %q", s)
	}
	if s := truncateRunes("hello", 1); s != "h" {
		t.Fatalf("expected h for max 1, got %q", s)
	}
	if s := truncateRunes("hello world", 5); s != "hell…" {
		t.Fatalf("expected hell… for max 5, got %q", s)
	}
}

func TestWhatsNextExtended_PersonaResolution(t *testing.T) {
	ctx := context.Background()
	mock := newExtendedMockStorage()

	// 1. Explicit persona
	res := resolvePersonaIDs(ctx, mock, "", "my-persona", "")
	if len(res) != 1 || res[0] != "my-persona" {
		t.Fatalf("expected my-persona, got %v", res)
	}

	// 2. getAgentPersonaIDs with explicit
	res2 := getAgentPersonaIDs(ctx, mock, "explicit-p")
	if len(res2) != 1 || res2[0] != "explicit-p" {
		t.Fatalf("expected explicit-p, got %v", res2)
	}

	// 3. getAgentPersonaIDs without security context roles -> returns nil
	res3 := getAgentPersonaIDs(ctx, mock, "")
	if res3 != nil {
		t.Fatalf("expected nil for no security context, got %v", res3)
	}

	// 4. getAgentPersonaIDs with matching security context role
	prs := map[string]any{
		objects.FieldKeyID:   "PRS-DEV",
		objects.FieldKeyKind: objects.KindPersona,
		objects.FieldKeyRole: "developer",
	}
	mock.add(prs)

	secCtx := &pkgctx.SecurityContext{Roles: []string{"developer"}}
	ctxWithSec := pkgctx.WithSecurityContext(ctx, secCtx)
	res4 := getAgentPersonaIDs(ctxWithSec, mock, "")
	if len(res4) != 1 || res4[0] != "PRS-DEV" {
		t.Fatalf("expected PRS-DEV, got %v", res4)
	}
}

func TestWhatsNextExtended_EvaluatePhiCounts(t *testing.T) {
	nodes := []*BacklogNode{
		{
			ID:              "BLI-1",
			PriorityPlanRef: "PRI-1",
			Status:          "in_progress",
			PersonaRefs:     []string{"dev"},
		},
		{
			ID:              "BLI-2",
			PriorityPlanRef: "PRI-1",
			Status:          "", // unknown status branch
			PersonaRefs:     []string{"dev"},
		},
		{
			ID:              "BLI-3",
			PriorityPlanRef: "PRI-2",
			Status:          "planned",
			PersonaRefs:     []string{"qa"},
		},
	}

	// Filter by plan and persona
	byPlan, total := EvaluatePhiCounts(nodes, []string{"PRI-1"}, []string{"dev"})
	if total["in_progress"] != 1 {
		t.Fatalf("expected 1 in_progress, got %d", total["in_progress"])
	}
	if total["unknown"] != 1 {
		t.Fatalf("expected 1 unknown, got %d", total["unknown"])
	}
	if len(byPlan) != 1 {
		t.Fatalf("expected 1 plan in byPlan, got %d", len(byPlan))
	}

	// Filter with persona mismatch
	_, totalMismatch := EvaluatePhiCounts(nodes, []string{"PRI-1"}, []string{"nobody"})
	if len(totalMismatch) != 0 {
		t.Fatalf("expected 0 for mismatched persona, got %d", len(totalMismatch))
	}
}

func TestWhatsNextExtended_SeatLookup(t *testing.T) {
	tmp := t.TempDir()
	// Non-TPM seat in temp dir without seating file
	if IsTPMProcessAdminSeatIn(tmp, "normal-worker") {
		t.Fatal("expected false for normal worker without seating file")
	}
}

func TestWhatsNextExtended_LoadKernelAmbienceWithCheckSummary(t *testing.T) {
	tmp := t.TempDir()
	logsDir := filepath.Join(tmp, ".zqk", "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	content := `{
  "summary": {
    "total_objects": 42,
    "blocking_issues": 0,
    "warnings": 1,
    "total_issues": 1
  },
  "results_by_kind": {
    "task": [
      {
        "issues": [
          {"message": "missing owner"}
        ]
      }
    ]
  }
}`
	_ = os.WriteFile(filepath.Join(logsDir, "system-check.json"), []byte(content), 0644)

	amb := LoadKernelAmbience(tmp)
	if !amb.Available {
		t.Fatal("expected available ambience when check summary exists")
	}
	if amb.TotalObjects != 42 {
		t.Fatalf("expected 42 objects, got %d", amb.TotalObjects)
	}
	if len(amb.TopIssueClusters) != 1 || amb.TopIssueClusters[0].Message != "missing owner" {
		t.Fatalf("expected top issue cluster 'missing owner', got %v", amb.TopIssueClusters)
	}
}

func TestWhatsNextExtended_ExtractLogFieldAndKeyWord(t *testing.T) {
	// extractLogField
	if v := extractLogField("no key here", "foo"); v != "" {
		t.Fatalf("expected empty for missing key, got %q", v)
	}
	if v := extractLogField("foo=", "foo"); v != "" {
		t.Fatalf("expected empty for empty value, got %q", v)
	}
	if v := extractLogField(`foo="quoted value" bar=baz`, "foo"); v != "quoted value" {
		t.Fatalf("expected 'quoted value', got %q", v)
	}
	if v := extractLogField(`foo="unclosed quote`, "foo"); v != "unclosed quote" {
		t.Fatalf("expected 'unclosed quote', got %q", v)
	}
	if v := extractLogField(`foo=unquoted value bar=baz`, "foo"); v != "unquoted value" {
		t.Fatalf("expected 'unquoted value', got %q", v)
	}

	// isLogKeyWord
	if !isLogKeyWord("foo=bar") {
		t.Fatal("expected true for foo=bar")
	}
	if isLogKeyWord("=bar") {
		t.Fatal("expected false for =bar")
	}
	if isLogKeyWord("foo!=bar") {
		t.Fatal("expected false for foo!=bar")
	}
	if isLogKeyWord("plain") {
		t.Fatal("expected false for plain")
	}
}
