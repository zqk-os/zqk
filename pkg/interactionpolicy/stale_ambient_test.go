package interactionpolicy

import (
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestStaleInProgressAmbient(t *testing.T) {
	t.Parallel()

	now := time.Now()
	oldTime := now.Add(-24 * time.Hour).Format(time.RFC3339)
	recentTime := now.Add(-10 * time.Minute).Format(time.RFC3339)

	blis := []map[string]any{
		{
			objects.FieldKeyID:           "BLI-STALE-001",
			objects.FieldKeyStatus:       objects.ObjectStatusInProgress,
			objects.FieldKeyUpdatedAt:    oldTime,
			objects.FieldKeyCriteriaRefs: []string{"CRIT-STALE-001"},
		},
		{
			objects.FieldKeyID:           "BLI-RECENT-002",
			objects.FieldKeyStatus:       objects.ObjectStatusInProgress,
			objects.FieldKeyUpdatedAt:    recentTime,
			objects.FieldKeyCriteriaRefs: []string{"CRIT-RECENT-002"},
		},
		{
			objects.FieldKeyID:        "BLI-PLANNED-003",
			objects.FieldKeyStatus:    objects.ObjectStatusPlanned,
			objects.FieldKeyUpdatedAt: oldTime,
		},
	}

	testCases := []map[string]any{
		{
			objects.FieldKeyID:              "TST-STALE-001",
			objects.FieldKeyStatus:          objects.ObjectStatusActive,
			objects.FieldKeyBacklogItemRefs: []string{"BLI-STALE-001"},
			objects.FieldKeyCriteriaRefs:    []string{"CRIT-STALE-001"},
		},
	}

	// 1. Detect stale in-progress BLIs
	staleItems := DetectStaleInProgress(blis, testCases, now, 12*time.Hour)
	if len(staleItems) != 1 {
		t.Fatalf("expected 1 stale item, got %d", len(staleItems))
	}

	stale := staleItems[0]
	if stale.BacklogItemID != "BLI-STALE-001" {
		t.Errorf("expected BLI-STALE-001, got %q", stale.BacklogItemID)
	}
	if stale.TestCaseID != "TST-STALE-001" {
		t.Errorf("expected TST-STALE-001, got %q", stale.TestCaseID)
	}

	// 2. Verify single-click command hint
	hint := StaleTestCatalystHint(stale.TestCaseID)
	expectedHint := "zqk test run TST-STALE-001"
	if hint != expectedHint {
		t.Errorf("expected hint %q, got %q", expectedHint, hint)
	}

	// 3. Verify Ambience struct integration
	amb := Ambience{
		Planned:         0,
		InProgress:      1,
		StaleInProgress: staleItems,
	}

	if len(amb.StaleInProgress) != 1 {
		t.Errorf("expected amb.StaleInProgress len 1, got %d", len(amb.StaleInProgress))
	}
}

func TestDetectStaleAgentTasks(t *testing.T) {
	t.Parallel()

	now := time.Now()
	oldTime := now.Add(-24 * time.Hour).Format(time.RFC3339)
	recentTime := now.Add(-10 * time.Minute).Format(time.RFC3339)

	tasks := []map[string]any{
		{
			objects.FieldKeyID:             "ATK-STALE-001",
			objects.FieldKeyStatus:         objects.ObjectStatusInProgress,
			objects.FieldKeyUpdatedAt:      oldTime,
			objects.FieldKeyBacklogItemRef: "BLI-001",
		},
		{
			objects.FieldKeyID:             "ATK-RECENT-002",
			objects.FieldKeyStatus:         objects.ObjectStatusInProgress,
			objects.FieldKeyUpdatedAt:      recentTime,
			objects.FieldKeyBacklogItemRef: "BLI-002",
		},
		{
			objects.FieldKeyID:             "ATK-APPROVED-003",
			objects.FieldKeyStatus:         objects.ObjectStatusApproved,
			objects.FieldKeyUpdatedAt:      oldTime,
			objects.FieldKeyBacklogItemRef: "BLI-003",
		},
	}

	staleItems := DetectStaleAgentTasks(tasks, now, 2*time.Hour)
	if len(staleItems) != 1 {
		t.Fatalf("expected 1 stale task, got %d", len(staleItems))
	}
	if staleItems[0].TaskID != "ATK-STALE-001" {
		t.Errorf("expected ATK-STALE-001, got %q", staleItems[0].TaskID)
	}
	if staleItems[0].BacklogItemID != "BLI-001" {
		t.Errorf("expected BLI-001, got %q", staleItems[0].BacklogItemID)
	}
}
