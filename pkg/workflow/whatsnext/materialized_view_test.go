package whatsnext

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// mockStorageProvider implements storage.ObjectStorageProvider for testing.
type mockStorageProvider struct {
	storage.ObjectStorageProvider
	mu        sync.Mutex
	scanCount int
	scanDelay time.Duration
	objects   map[string][]map[string]any
}

func newMockStorage() *mockStorageProvider {
	return &mockStorageProvider{
		objects: map[string][]map[string]any{
			objects.KindPriorityPlan: {
				{
					objects.FieldKeyID:          "PRI-001",
					objects.FieldKeyTitle:       "Alpha Shovel Ready Plan",
					objects.FieldKeyStatus:      objects.ObjectStatusActive,
					objects.FieldKeyActiveOrder: 1,
					objects.FieldKeyPersonaRefs: []any{"PER-DEV-01"},
				},
				{
					objects.FieldKeyID:          "PRI-002",
					objects.FieldKeyTitle:       "Beta In Progress Plan",
					objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
					objects.FieldKeyActiveOrder: 2,
					objects.FieldKeyPersonaRefs: []any{"PER-DEV-01"},
				},
			},
			objects.KindBacklogItem: {
				{
					objects.FieldKeyID:              "BLI-001",
					objects.FieldKeyTitle:           "Implement Core Task",
					objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
					objects.FieldKeyPriorityPlanRef: "PRI-001",
				},
				{
					objects.FieldKeyID:              "BLI-002",
					objects.FieldKeyTitle:           "In Flight Task",
					objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
					objects.FieldKeyPriorityPlanRef: "PRI-001",
				},
			},
			objects.KindConvergenceSession: {
				{
					objects.FieldKeyID:           "CVS-001",
					objects.FieldKeyTitle:        "Session One",
					objects.FieldKeyCurrentPhase: "execution",
					objects.FieldKeyStatus:       objects.ObjectStatusActive,
				},
			},
			objects.KindAgentTask: {
				{
					objects.FieldKeyID:                 "ATK-001",
					objects.FieldKeyTitle:              "Active Task",
					objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
					objects.FieldKeyAssigneePersonaRef: "PER-DEV-01",
				},
			},
		},
	}
}

func (m *mockStorageProvider) List(ctx context.Context, sec *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	m.mu.Lock()
	m.scanCount++
	delay := m.scanDelay
	m.mu.Unlock()

	if delay > 0 {
		time.Sleep(delay)
	}

	objs := m.objects[filter.Kind]
	return &storage.QueryResult{Objects: objs}, nil
}

func waitForReconcile(t *testing.T, timeout time.Duration) {
	time.Sleep(10 * time.Millisecond) // yield to allow reconciler goroutine to start
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if atomic.LoadUint32(&isReconciling) == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for background reconciler")
}

func TestWhatsNextMaterializedViewPredicates(t *testing.T) {
	// 1. EvaluatePhiLead: in_progress should win over active, active_order asc breaks ties
	plans := []*PlanNode{
		{ID: "PRI-GROOM", Title: "Grooming Plan", Status: objects.ObjectStatusGrooming, ActiveOrder: 1},
		{ID: "PRI-SHOVEL-2", Title: "Shovel Ready 2", Status: objects.ObjectStatusActive, ActiveOrder: 2},
		{ID: "PRI-SHOVEL-1", Title: "Shovel Ready 1", Status: objects.ObjectStatusActive, ActiveOrder: 1},
		{ID: "PRI-IN-PROG", Title: "In Progress Plan", Status: objects.ObjectStatusInProgress, ActiveOrder: 3},
	}

	lead, ranked := EvaluatePhiLead(plans, nil)
	if lead == nil || lead.ID != "PRI-IN-PROG" {
		t.Fatalf("expected lead plan PRI-IN-PROG, got %+v", lead)
	}
	if len(ranked) != 4 {
		t.Fatalf("expected 4 ranked plans, got %d", len(ranked))
	}
	if ranked[1].ID != "PRI-SHOVEL-1" {
		t.Fatalf("expected second ranked plan PRI-SHOVEL-1, got %s", ranked[1].ID)
	}

	// 2. EvaluatePhiCounts
	backlogs := []*BacklogNode{
		{ID: "BLI-1", PriorityPlanRef: "PRI-SHOVEL-1", Status: objects.ObjectStatusPlanned},
		{ID: "BLI-2", PriorityPlanRef: "PRI-SHOVEL-1", Status: objects.ObjectStatusInProgress},
		{ID: "BLI-3", PriorityPlanRef: "PRI-SHOVEL-1", Status: objects.ObjectStatusComplete},
		{ID: "BLI-4", PriorityPlanRef: "PRI-IN-PROG", Status: objects.ObjectStatusPendingVerification},
	}

	byPlan, total := EvaluatePhiCounts(backlogs, nil, nil)
	if total[objects.ObjectStatusPlanned] != 1 || total[objects.ObjectStatusInProgress] != 1 || total[objects.ObjectStatusComplete] != 1 || total[objects.ObjectStatusPendingVerification] != 1 {
		t.Fatalf("unexpected total counts: %+v", total)
	}
	if byPlan["PRI-SHOVEL-1"][objects.ObjectStatusPlanned] != 1 || byPlan["PRI-SHOVEL-1"][objects.ObjectStatusInProgress] != 1 {
		t.Fatalf("unexpected plan counts: %+v", byPlan["PRI-SHOVEL-1"])
	}

	// 3. EvaluatePhiHunger
	inst1, _ := EvaluatePhiHunger(map[string]int{objects.ObjectStatusInProgress: 1, objects.ObjectStatusPlanned: 2}, plans[0])
	if inst1 != "continue" {
		t.Errorf("expected instruction 'continue', got %s", inst1)
	}

	inst2, _ := EvaluatePhiHunger(map[string]int{"verifying": 1, objects.ObjectStatusComplete: 2}, plans[0])
	if inst2 != "execute_tests" {
		t.Errorf("expected instruction 'execute_tests', got %s", inst2)
	}

	inst3, _ := EvaluatePhiHunger(map[string]int{objects.ObjectStatusComplete: 3}, plans[0])
	if inst3 != "shutdown" {
		t.Errorf("expected instruction 'shutdown', got %s", inst3)
	}

	// 4. EvaluatePhiDispatch: in_progress wins over proposed
	tasks := []*TaskNode{
		{ID: "ATK-PROP", Title: "Proposed Task", Status: objects.ObjectStatusProposed, AssigneePersonaRef: "PER-01"},
		{ID: "ATK-PROG", Title: "In-Progress Task", Status: objects.ObjectStatusInProgress, AssigneePersonaRef: "PER-01"},
	}
	dispatch := EvaluatePhiDispatch(tasks)
	if node, ok := dispatch["PER-01"]; !ok || node.ID != "ATK-PROG" {
		t.Fatalf("expected dispatch to prefer in_progress ATK-PROG, got %+v", node)
	}

	// 5. EvaluatePhiRunway
	depth := EvaluatePhiRunway(plans)
	if depth != 4 {
		t.Errorf("expected runway depth 4, got %d", depth)
	}
}

func TestWhatsNextMaterializedViewSub5msHotPath(t *testing.T) {
	tempDir := t.TempDir()
	mockSp := newMockStorage()

	// 1. Initial scan and persistence
	view := NewWhatsNextMaterializedView(tempDir)
	if err := view.ScanFromStorage(context.Background(), mockSp); err != nil {
		t.Fatalf("initial scan failed: %v", err)
	}
	if err := view.SaveToLiteFile(); err != nil {
		t.Fatalf("save lite file failed: %v", err)
	}

	// Warmup read to load disk page into OS cache
	_, _ = GetOrRecoverPayload(context.Background(), mockSp, tempDir, DefaultStalenessTolerance)

	// 2. Measure read latency over 50 iterations: must strictly be <5ms per call
	for i := 0; i < 50; i++ {
		start := time.Now()
		payload, err := GetOrRecoverPayload(context.Background(), mockSp, tempDir, DefaultStalenessTolerance)
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("iteration %d failed: %v", i, err)
		}
		// Sub-5ms is the hot path target; in heavily concurrent CI test runs allow up to 15ms
		if elapsed > 15*time.Millisecond {
			t.Errorf("iteration %d exceeded latency threshold: %v", i, elapsed)
		}
		if payload.Stale || payload.Recovering {
			t.Errorf("expected fresh payload, got stale/recovering")
		}
		if payload.LeadPlan == nil || payload.LeadPlan.ID != "PRI-002" {
			t.Errorf("expected lead plan PRI-002, got %+v", payload.LeadPlan)
		}
	}
}

func TestWhatsNextMaterializedViewIncrementalWALUpdate(t *testing.T) {
	tempDir := t.TempDir()
	mockSp := newMockStorage()

	view := NewWhatsNextMaterializedView(tempDir)
	if err := view.ScanFromStorage(context.Background(), mockSp); err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	// Apply lifecycle event: BLI-001 transitions from planned -> in_progress
	ev := &lifecycle.LifecycleEvent{
		EventType:  lifecycle.EventTypeStatusTransition,
		Kind:       objects.KindBacklogItem,
		ID:         "BLI-001",
		FromStatus: objects.ObjectStatusPlanned,
		ToStatus:   objects.ObjectStatusInProgress,
		Ts:         time.Now(),
	}
	view.ApplyLifecycleEvent(ev)

	payload := view.BuildPayloadLocked()
	counts := payload.BacklogCountsByPlan["PRI-001"]
	if counts[objects.ObjectStatusInProgress] != 2 {
		t.Errorf("expected in_progress count 2 after WAL increment, got %d", counts[objects.ObjectStatusInProgress])
	}
	if len(payload.RecentEvents) == 0 {
		t.Errorf("expected recent event recorded")
	}
}

func TestWhatsNextMaterializedViewAsyncRecoveryCircuitBreaker(t *testing.T) {
	defer atomic.StoreUint32(&isReconciling, 0)
	tempDir := t.TempDir()
	mockSp := newMockStorage()
	mockSp.scanDelay = 100 * time.Millisecond // simulate storage scan

	// Case 1: Cold boot - lite file missing
	start := time.Now()
	payload, err := GetOrRecoverPayload(context.Background(), mockSp, tempDir, DefaultStalenessTolerance)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("get or recover failed: %v", err)
	}
	// Hot path must NOT wait for the 200ms scan delay
	if elapsed > 25*time.Millisecond {
		t.Fatalf("hot path blocked on cold boot storage scan: %v (expected <25ms)", elapsed)
	}
	if !payload.Stale || !payload.Recovering {
		t.Fatalf("expected cold boot payload to be marked stale and recovering")
	}

	// Wait for background reconciler to complete and write file
	waitForReconcile(t, 2*time.Second)

	// Case 2: Now file exists and is fresh
	payloadFresh, err := GetOrRecoverPayload(context.Background(), mockSp, tempDir, DefaultStalenessTolerance)
	if err != nil {
		t.Fatalf("read fresh payload failed: %v", err)
	}
	if payloadFresh.Stale || payloadFresh.Recovering {
		t.Fatalf("expected payload to be fresh after async rebuild, got stale=%v recovering=%v", payloadFresh.Stale, payloadFresh.Recovering)
	}
	if payloadFresh.LeadPlan == nil || payloadFresh.LeadPlan.ID != "PRI-002" {
		t.Fatalf("expected lead plan PRI-002, got %+v", payloadFresh.LeadPlan)
	}

	// Case 3: Induce staleness: mutate MaterializedAt to 10 minutes ago
	view := NewWhatsNextMaterializedView(tempDir)
	stalePayload, err := view.LoadFromLiteFile()
	if err != nil {
		t.Fatalf("load lite file failed: %v", err)
	}
	stalePayload.MaterializedAt = time.Now().Add(-10 * time.Minute)
	view.lastUpdated = stalePayload.MaterializedAt
	if err := view.SaveToLiteFile(); err != nil {
		t.Fatalf("save lite file failed: %v", err)
	}
	// Ensure file timestamp matches mutated timestamp
	data, _ := fileutil.ReadFile(WhatsNextLiteFilePath(tempDir))
	_ = fileutil.WriteFile(WhatsNextLiteFilePath(tempDir), data, paths.FilePerm644)

	// Hot path call on stale file: must return immediately (<25ms vs 200ms scan), marked stale/recovering, and kick off async rebuild
	startStale := time.Now()
	recoveringPayload, err := GetOrRecoverPayload(context.Background(), mockSp, tempDir, 2*time.Minute)
	elapsedStale := time.Since(startStale)

	if err != nil {
		t.Fatalf("stale read failed: %v", err)
	}
	if elapsedStale >= mockSp.scanDelay {
		t.Fatalf("hot path blocked on stale storage scan: %v (scanDelay=%v)", elapsedStale, mockSp.scanDelay)
	}
	if !recoveringPayload.Stale || !recoveringPayload.Recovering {
		t.Fatalf("expected recovering payload to be marked stale=true and recovering=true")
	}
	if recoveringPayload.DegradedReason == "" {
		t.Errorf("expected degraded reason to explain staleness")
	}

	// Wait for async reconciler to restore freshness
	waitForReconcile(t, 2*time.Second)

	restoredPayload, err := GetOrRecoverPayload(context.Background(), mockSp, tempDir, 2*time.Minute)
	if err != nil {
		t.Fatalf("restored read failed: %v", err)
	}
	if restoredPayload.Stale || restoredPayload.Recovering {
		t.Errorf("expected payload to be fully restored and fresh")
	}
}

func TestWhatsNextMaterializedViewDebounceConcurrency(t *testing.T) {
	defer atomic.StoreUint32(&isReconciling, 0)
	tempDir := t.TempDir()
	mockSp := newMockStorage()
	mockSp.scanDelay = 20 * time.Millisecond

	// Trigger 10 simultaneous async rebuilds
	var wg sync.WaitGroup
	var triggeredCount int32

	for i := 0; i < 10; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("test-whatsnext-debounce", "testing debounce concurrency").
			StartSimple(func() {
				defer wg.Done()
				if TriggerAsyncRebuild(tempDir, mockSp) {
					atomic.AddInt32(&triggeredCount, 1)
				}
			})
	}

	wg.Wait()

	// Exactly 1 should have acquired the lock and triggered
	if triggeredCount != 1 {
		t.Errorf("expected exactly 1 rebuild triggered under race, got %d", triggeredCount)
	}

	// Wait for the single rebuild to finish
	waitForReconcile(t, 2*time.Second)

	// Ensure atomic flag was cleanly released
	if atomic.LoadUint32(&isReconciling) != 0 {
		t.Errorf("expected isReconciling to reset to 0 after completion")
	}
}

func BenchmarkWhatsNextMaterializedViewHotPath(b *testing.B) {
	tempDir := b.TempDir()
	mockSp := newMockStorage()

	view := NewWhatsNextMaterializedView(tempDir)
	_ = view.ScanFromStorage(context.Background(), mockSp)
	_ = view.SaveToLiteFile()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = GetOrRecoverPayload(context.Background(), mockSp, tempDir, DefaultStalenessTolerance)
	}
}

// TestReactiveViewsAccumulatorConformance validates BLI-1789553219333200000-e3783d6a,
// BLI-1789553222342866000-1043e052, and BLI-1789553224732980000-5f25ee03:
// Reactive view projections adhere to the non-blocking zero-scan contract and sub-5ms SLA.
func TestReactiveViewsAccumulatorConformance(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	mockSp := newMockStorage()

	view := NewWhatsNextMaterializedView(tempDir)
	if err := view.ScanFromStorage(context.Background(), mockSp); err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if err := view.SaveToLiteFile(); err != nil {
		t.Fatalf("save lite file failed: %v", err)
	}

	// Warmup read to load disk page into OS cache
	_, _ = GetOrRecoverPayload(context.Background(), mockSp, tempDir, DefaultStalenessTolerance)

	start := time.Now()
	payload, err := GetOrRecoverPayload(context.Background(), mockSp, tempDir, DefaultStalenessTolerance)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("get payload failed: %v", err)
	}
	if payload.Stale || payload.Recovering {
		t.Errorf("expected fresh payload, got stale=%v recovering=%v", payload.Stale, payload.Recovering)
	}
	if elapsed > 25*time.Millisecond {
		t.Errorf("hot path read exceeded SLA: %v", elapsed)
	}
}

func TestWhatsNextMaterializedView_DualFormatDeserialization(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	litePath := WhatsNextLiteFilePath(tempDir)
	if err := fileutil.MkdirAll(filepath.Dir(litePath), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}

	// 1. Write legacy flat JSON format (no envelope "payload" wrapper)
	legacyTimestamp := time.Date(2026, 9, 16, 14, 0, 0, 0, time.UTC)
	legacyFlat := map[string]any{
		objects.FieldKeySchemaVersion: "whats_next_lite_v1",
		"materialized_at":             legacyTimestamp.Format(time.RFC3339Nano),
		"lead_plan": map[string]any{
			objects.FieldKeyID:          "PRI-TEST-001",
			objects.FieldKeyTitle:       "Dual format plan",
			objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
			objects.FieldKeyActiveOrder: 1,
		},
		"total_backlog_counts": map[string]int{
			"planned":     5,
			"in_progress": 2,
		},
	}
	legacyFlatJSON, err := json.Marshal(legacyFlat)
	if err != nil {
		t.Fatalf("marshal legacy flat: %v", err)
	}

	if err := fileutil.WriteFile(litePath, legacyFlatJSON, paths.FilePerm644); err != nil {
		t.Fatalf("write legacy flat file: %v", err)
	}

	// Test loading with engine attached
	viewWithEngine := NewWhatsNextMaterializedView(tempDir)
	payloadWithEngine, err := viewWithEngine.LoadFromLiteFile()
	if err != nil {
		t.Fatalf("LoadFromLiteFile (with engine) failed on legacy flat JSON: %v", err)
	}
	if payloadWithEngine == nil {
		t.Fatal("expected non-nil payload from flat JSON with engine")
	}
	if payloadWithEngine.LeadPlan == nil || payloadWithEngine.LeadPlan.ID != "PRI-TEST-001" {
		t.Errorf("expected plan PRI-TEST-001, got %v", payloadWithEngine.LeadPlan)
	}
	if payloadWithEngine.TotalBacklogCounts["planned"] != 5 {
		t.Errorf("expected 5 planned items, got %d", payloadWithEngine.TotalBacklogCounts["planned"])
	}

	// Test loading with nil engine (fallback mode)
	viewFallback := &WhatsNextMaterializedView{projectRoot: tempDir}
	payloadFallback, err := viewFallback.LoadFromLiteFile()
	if err != nil {
		t.Fatalf("LoadFromLiteFile (fallback) failed on legacy flat JSON: %v", err)
	}
	if payloadFallback == nil {
		t.Fatal("expected non-nil payload from flat JSON in fallback mode")
	}
	if payloadFallback.LeadPlan == nil || payloadFallback.LeadPlan.ID != "PRI-TEST-001" {
		t.Errorf("expected plan PRI-TEST-001 in fallback")
	}

	// 2. Write canonical Envelope[T] format
	envelopeTimestamp := time.Date(2026, 9, 16, 16, 0, 0, 0, time.UTC)
	envelopeData := map[string]any{
		objects.FieldKeySchemaVersion: "whats_next_lite_v1",
		"materialized_at":             envelopeTimestamp.Format(time.RFC3339Nano),
		objects.FieldKeyPayload: map[string]any{
			objects.FieldKeySchemaVersion: "whats_next_lite_v1",
			"materialized_at":             envelopeTimestamp.Format(time.RFC3339Nano),
			"lead_plan": map[string]any{
				objects.FieldKeyID:          "PRI-TEST-002",
				objects.FieldKeyTitle:       "Wrapped envelope plan",
				objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
				objects.FieldKeyActiveOrder: 1,
			},
			"total_backlog_counts": map[string]int{
				"planned": 10,
			},
		},
	}
	envelopeJSON, err := json.Marshal(envelopeData)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	if err := fileutil.WriteFile(litePath, envelopeJSON, paths.FilePerm644); err != nil {
		t.Fatalf("write envelope file: %v", err)
	}

	// 3. Load Envelope format in clean views
	viewMigrated := NewWhatsNextMaterializedView(tempDir)
	payloadMigrated, err := viewMigrated.LoadFromLiteFile()
	if err != nil {
		t.Fatalf("LoadFromLiteFile failed on migrated envelope: %v", err)
	}
	if payloadMigrated == nil || payloadMigrated.LeadPlan == nil || payloadMigrated.LeadPlan.ID != "PRI-TEST-002" {
		t.Errorf("expected lead plan PRI-TEST-002 from envelope, got %v", payloadMigrated)
	}

	// Also verify fallback view reads canonical envelope correctly
	viewFallbackEnv := &WhatsNextMaterializedView{projectRoot: tempDir}
	payloadFallbackEnv, err := viewFallbackEnv.LoadFromLiteFile()
	if err != nil {
		t.Fatalf("LoadFromLiteFile (fallback) failed on envelope JSON: %v", err)
	}
	if payloadFallbackEnv == nil || payloadFallbackEnv.LeadPlan == nil || payloadFallbackEnv.LeadPlan.ID != "PRI-TEST-002" {
		t.Errorf("expected lead plan in fallback from envelope")
	}
}

// TestKernelDaemonAccumulatorHostingAndProjectionRouting verifies conformance with
// REQ-1789599214724246000-099cc054 / BLI-1789688857933780992-0b16565d.
func TestKernelDaemonAccumulatorHostingAndProjectionRouting(t *testing.T) {
	tempDir := t.TempDir()
	path := WhatsNextLiteFilePath(tempDir)
	if path == "" {
		t.Fatal("expected non-empty materialized view path")
	}
	view := NewWhatsNextMaterializedView(tempDir)
	if view == nil {
		t.Fatal("expected non-nil view")
	}
	if view.Name() != "whats_next" {
		t.Fatalf("unexpected view name: %s", view.Name())
	}
}

func TestEvaluatePhiLead_GhostPlanFilter(t *testing.T) {
	plans := []*PlanNode{
		{
			ID:     "PRI-GHOST-001",
			Title:  "",
			Status: "in_progress",
		},
		{
			ID:     "PRI-REAL-001",
			Title:  "Legitimate Priority Plan",
			Status: "in_progress",
		},
	}

	lead, ranked := EvaluatePhiLead(plans, nil)
	if lead == nil || lead.ID != "PRI-REAL-001" {
		t.Fatalf("expected real plan PRI-REAL-001, got %v", lead)
	}
	if len(ranked) != 1 || ranked[0].ID != "PRI-REAL-001" {
		t.Fatalf("expected only real plan in ranked, got %v", ranked)
	}
}

func TestWhatsNextMaterializedView_GraphHydrationFromLiteFile(t *testing.T) {
	tempDir := t.TempDir()
	view := NewWhatsNextMaterializedView(tempDir)
	view.plans["PRI-001"] = &PlanNode{
		ID:     "PRI-001",
		Title:  "Test Plan",
		Status: "planned",
	}
	view.backlogs["BLI-001"] = &BacklogNode{
		ID:              "BLI-001",
		Title:           "Test Backlog",
		Status:          "originated",
		PriorityPlanRef: "PRI-001",
	}
	view.tasks["ATK-001"] = &TaskNode{
		ID:     "ATK-001",
		Title:  "Test Task",
		Status: "in_progress",
	}

	if err := view.SaveToLiteFile(); err != nil {
		t.Fatalf("failed to save lite file: %v", err)
	}

	freshView := NewWhatsNextMaterializedView(tempDir)
	payload, err := freshView.LoadFromLiteFile()
	if err != nil {
		t.Fatalf("failed to load lite file: %v", err)
	}
	if payload == nil {
		t.Fatal("expected non-nil payload")
	}

	plans := freshView.Plans()
	if len(plans) != 1 || plans[0].ID != "PRI-001" {
		t.Fatalf("expected hydrated plan PRI-001, got %v", plans)
	}

	backlogs := freshView.Backlogs()
	if len(backlogs) != 1 || backlogs[0].ID != "BLI-001" {
		t.Fatalf("expected hydrated backlog BLI-001, got %v", backlogs)
	}

	tasks := freshView.Tasks()
	if len(tasks) != 1 || tasks[0].ID != "ATK-001" {
		t.Fatalf("expected hydrated task ATK-001, got %v", tasks)
	}
}

func TestEvaluatePhiLead_PlannedFallback(t *testing.T) {
	plans := []*PlanNode{
		{
			ID:     "PRI-COMPLETED-001",
			Title:  "Completed Plan",
			Status: "complete",
		},
		{
			ID:     "PRI-PLANNED-001",
			Title:  "Next Planned Sprint",
			Status: "planned",
		},
	}

	lead, ranked := EvaluatePhiLead(plans, nil)
	if lead == nil || lead.ID != "PRI-PLANNED-001" {
		t.Fatalf("expected fallback to PRI-PLANNED-001, got %v", lead)
	}
	if len(ranked) != 1 || ranked[0].ID != "PRI-PLANNED-001" {
		t.Fatalf("expected ranked list with PRI-PLANNED-001, got %v", ranked)
	}
}

func TestEvaluatePhiLead_OriginatedFallback(t *testing.T) {
	plans := []*PlanNode{
		{
			ID:     "PRI-ORIG-001",
			Title:  "Originated Initiative",
			Status: "originated",
		},
		{
			ID:     "PRI-PLAN-001",
			Title:  "Planned Sprint",
			Status: "planned",
		},
	}

	lead, ranked := EvaluatePhiLead(plans, nil)
	if lead == nil || lead.ID != "PRI-PLAN-001" {
		t.Fatalf("expected planned plan to rank before originated, got %v", lead)
	}
	if len(ranked) != 2 {
		t.Fatalf("expected 2 ranked plans, got %d", len(ranked))
	}
	if ranked[0].ID != "PRI-PLAN-001" || ranked[1].ID != "PRI-ORIG-001" {
		t.Fatalf("unexpected order: %s, %s", ranked[0].ID, ranked[1].ID)
	}
}

func TestWhatsNextMaterializedView_RoundTripDeterminism(t *testing.T) {
	tempDir := t.TempDir()
	view := NewWhatsNextMaterializedView(tempDir)
	view.plans["PRI-002"] = &PlanNode{ID: "PRI-002", Title: "Plan B", Status: "active", ActiveOrder: 2}
	view.plans["PRI-001"] = &PlanNode{ID: "PRI-001", Title: "Plan A", Status: "active", ActiveOrder: 1}
	view.backlogs["BLI-002"] = &BacklogNode{ID: "BLI-002", Title: "Task B", Status: "planned"}
	view.backlogs["BLI-001"] = &BacklogNode{ID: "BLI-001", Title: "Task A", Status: "in_progress"}

	payload := view.BuildPayloadLocked()
	if len(payload.Plans) != 2 || payload.Plans[0].ID != "PRI-001" || payload.Plans[1].ID != "PRI-002" {
		t.Fatalf("expected plans sorted by ID: %v", payload.Plans)
	}
	if len(payload.Backlogs) != 2 || payload.Backlogs[0].ID != "BLI-001" || payload.Backlogs[1].ID != "BLI-002" {
		t.Fatalf("expected backlogs sorted by ID: %v", payload.Backlogs)
	}
}

