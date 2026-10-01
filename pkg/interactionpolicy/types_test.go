package interactionpolicy

import (
	"reflect"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestBoundaryConverters(t *testing.T) {
	t.Run("nil and empty maps", func(t *testing.T) {
		bli := BacklogItemSummaryFromMap(nil)
		if bli.ID != "" || bli.Status != "" {
			t.Errorf("expected empty BacklogItemSummary, got %+v", bli)
		}
		if blis := BacklogItemSummariesFromMaps(nil); blis != nil {
			t.Errorf("expected nil slice, got %+v", blis)
		}

		tc := TestCaseSummaryFromMap(nil)
		if tc.ID != "" || tc.Title != "" {
			t.Errorf("expected empty TestCaseSummary, got %+v", tc)
		}
		if tcs := TestCaseSummariesFromMaps(nil); tcs != nil {
			t.Errorf("expected nil slice, got %+v", tcs)
		}

		task := AgentTaskSummaryFromMap(nil)
		if task.ID != "" || task.Status != "" {
			t.Errorf("expected empty AgentTaskSummary, got %+v", task)
		}
		if tasks := AgentTaskSummariesFromMaps(nil); tasks != nil {
			t.Errorf("expected nil slice, got %+v", tasks)
		}

		pol := PolicyDocumentFromMap(nil)
		if pol.ID != "" || pol.Body != "" {
			t.Errorf("expected empty PolicyDocument, got %+v", pol)
		}
	})

	t.Run("populated maps conversion", func(t *testing.T) {
		rawBLI := map[string]any{
			objects.FieldKeyID:              "BLI-001",
			objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
			objects.FieldKeyUpdatedAt:       "2026-10-01T12:00:00Z",
			objects.FieldKeyCreatedAt:       "2026-10-01T10:00:00Z",
			objects.FieldKeyCriteriaRefs:    []any{"CRIT-001", "CRIT-002"},
			objects.FieldKeyTitle:           "Implement Typing",
			objects.FieldKeyPriorityPlanRef: "PRI-001",
		}
		bli := BacklogItemSummaryFromMap(rawBLI)
		if bli.ID != "BLI-001" || bli.Status != objects.ObjectStatusInProgress {
			t.Errorf("unexpected BLI conversion: %+v", bli)
		}
		if !reflect.DeepEqual(bli.CriteriaRefs, []string{"CRIT-001", "CRIT-002"}) {
			t.Errorf("unexpected criteria refs: %v", bli.CriteriaRefs)
		}

		rawTC := map[string]any{
			objects.FieldKeyID:              "TST-001",
			objects.FieldKeyStatus:          "active",
			objects.FieldKeyBacklogItemRefs: []string{"BLI-001"},
			objects.FieldKeyCriteriaRefs:    []string{"CRIT-001"},
			objects.FieldKeyTitle:           "Verify Typing",
		}
		tc := TestCaseSummaryFromMap(rawTC)
		if tc.ID != "TST-001" || len(tc.BacklogItemRefs) != 1 || tc.BacklogItemRefs[0] != "BLI-001" {
			t.Errorf("unexpected TC conversion: %+v", tc)
		}

		rawTask := map[string]any{
			objects.FieldKeyID:              "ATK-001",
			objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
			objects.FieldKeyBacklogItemRef:  "BLI-001",
			objects.FieldKeyPriorityPlanRef: "PRI-001",
			objects.FieldKeyClaimedBy:       "ACC-AGENT-1",
			objects.FieldKeyUpdatedAt:       "2026-10-01T11:00:00Z",
			objects.FieldKeyClaimedAt:       "2026-10-01T10:30:00Z",
			objects.FieldKeyCreatedAt:       "2026-10-01T10:00:00Z",
		}
		task := AgentTaskSummaryFromMap(rawTask)
		if task.ID != "ATK-001" || task.ClaimedBy != "ACC-AGENT-1" || task.BacklogItemRef != "BLI-001" {
			t.Errorf("unexpected Task conversion: %+v", task)
		}

		rawPol := map[string]any{
			objects.FieldKeyID:          "POL-001",
			objects.FieldKeyTitle:       "Sample Policy",
			objects.FieldKeyStatus:      "active",
			objects.FieldKeyBody:        "## Response\nDo the needful with typing.",
			objects.FieldKeyDescription: "Policy description",
		}
		pol := PolicyDocumentFromMap(rawPol)
		if pol.ID != "POL-001" || pol.Title != "Sample Policy" || pol.Body == "" {
			t.Errorf("unexpected Policy conversion: %+v", pol)
		}
	})
}

func TestDetectStaleInProgressTyped(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	staleThreshold := 15 * time.Minute

	blis := []BacklogItemSummary{
		{
			ID:           "BLI-STALE-1",
			Status:       objects.ObjectStatusInProgress,
			UpdatedAt:    now.Add(-20 * time.Minute).Format(time.RFC3339),
			CriteriaRefs: []string{"CRIT-101"},
		},
		{
			ID:           "BLI-FRESH-1",
			Status:       objects.ObjectStatusInProgress,
			UpdatedAt:    now.Add(-5 * time.Minute).Format(time.RFC3339),
			CriteriaRefs: []string{"CRIT-102"},
		},
		{
			ID:        "BLI-PLANNED-1",
			Status:    objects.ObjectStatusPlanned,
			UpdatedAt: now.Add(-30 * time.Minute).Format(time.RFC3339),
		},
	}

	testCases := []TestCaseSummary{
		{
			ID:           "TST-101",
			CriteriaRefs: []string{"CRIT-101"},
		},
	}

	stale := DetectStaleInProgressTyped(blis, testCases, now, staleThreshold)
	if len(stale) != 1 {
		t.Fatalf("expected 1 stale item, got %d", len(stale))
	}
	if stale[0].BacklogItemID != "BLI-STALE-1" {
		t.Errorf("expected BLI-STALE-1, got %s", stale[0].BacklogItemID)
	}
	if stale[0].TestCaseID != "TST-101" {
		t.Errorf("expected matching test case TST-101, got %s", stale[0].TestCaseID)
	}
}

func TestDetectStaleAgentTasksTyped(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	staleThreshold := 15 * time.Minute

	tasks := []AgentTaskSummary{
		{
			ID:             "ATK-STALE-1",
			Status:         objects.ObjectStatusInProgress,
			BacklogItemRef: "BLI-101",
			UpdatedAt:      now.Add(-25 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:             "ATK-FRESH-1",
			Status:         objects.ObjectStatusInProgress,
			BacklogItemRef: "BLI-102",
			UpdatedAt:      now.Add(-2 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:             "ATK-COMPLETE-1",
			Status:         objects.ObjectStatusComplete,
			BacklogItemRef: "BLI-103",
			UpdatedAt:      now.Add(-60 * time.Minute).Format(time.RFC3339),
		},
	}

	stale := DetectStaleAgentTasksTyped(tasks, now, staleThreshold)
	if len(stale) != 1 {
		t.Fatalf("expected 1 stale task, got %d", len(stale))
	}
	if stale[0].TaskID != "ATK-STALE-1" {
		t.Errorf("expected ATK-STALE-1, got %s", stale[0].TaskID)
	}
	if stale[0].BacklogItemID != "BLI-101" {
		t.Errorf("expected BLI-101, got %s", stale[0].BacklogItemID)
	}
}

func TestOverlayFromPolicyTyped(t *testing.T) {
	step := &Step{
		GuidingStep: "default catalog action",
	}

	doc := PolicyDocument{
		ID:   "POL-001",
		Body: "## Response\nAlways maintain type safety and zero reflection.",
	}

	ok := OverlayFromPolicyTyped(step, doc)
	if !ok {
		t.Fatalf("expected OverlayFromPolicyTyped to return true")
	}
	if step.GuidingStep != "Always maintain type safety and zero reflection." {
		t.Errorf("unexpected guiding step: %s", step.GuidingStep)
	}

	// Empty body should return false and not modify step
	stepEmpty := &Step{GuidingStep: "initial"}
	if OverlayFromPolicyTyped(stepEmpty, PolicyDocument{}) {
		t.Errorf("expected false for empty policy document")
	}
	if stepEmpty.GuidingStep != "initial" {
		t.Errorf("step modified on empty policy: %s", stepEmpty.GuidingStep)
	}
}
