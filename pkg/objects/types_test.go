package objects

import (
	"testing"
	"time"
)

func TestGetStatusHistory(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		FieldKeyID: "ITEM-001",
		FieldKeyStatusHistory: []any{
			map[string]any{
				FieldKeyStatus: "exploring",
				"timestamp":    "2025-01-01T00:00:00Z",
				FieldKeyReason: "Initial creation",
				"changed_by":   "account:user",
			},
			map[string]any{
				FieldKeyStatus: "validated",
				"timestamp":    "2025-01-02T00:00:00Z",
				FieldKeyReason: "Requirements clarified",
			},
		},
	}

	history, err := GetStatusHistory(obj)
	if err != nil {
		t.Fatalf("GetStatusHistory failed: %v", err)
	}

	if len(history) != 2 {
		t.Errorf("expected 2 entries, got %d", len(history))
	}

	if history[0].Status != "exploring" {
		t.Errorf("expected status 'exploring', got %s", history[0].Status)
	}

	if history[0].Reason != "Initial creation" {
		t.Errorf("expected reason 'Initial creation', got %s", history[0].Reason)
	}

	if history[1].Status != "validated" {
		t.Errorf("expected status 'validated', got %s", history[1].Status)
	}
}

func TestGetStatusHistory_Empty(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		FieldKeyID: "ITEM-001",
	}

	history, err := GetStatusHistory(obj)
	if err != nil {
		t.Fatalf("GetStatusHistory failed: %v", err)
	}

	if len(history) != 0 {
		t.Errorf("expected empty history, got %d entries", len(history))
	}
}

func TestSetStatusHistory(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		FieldKeyID: "ITEM-001",
	}

	history := []StatusHistoryEntry{
		{
			Status:    "exploring",
			Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Reason:    "Initial creation",
			ChangedBy: "account:user",
		},
	}

	SetStatusHistory(obj, history)

	// Verify it was set
	historyRaw, ok := obj[FieldKeyStatusHistory]
	if !ok {
		t.Fatal("status_history was not set")
	}

	historyList, ok := historyRaw.([]any)
	if !ok {
		t.Fatalf("status_history has wrong type: %T", historyRaw)
	}

	if len(historyList) != 1 {
		t.Errorf("expected 1 entry, got %d", len(historyList))
	}
}

func TestAddStatusHistoryEntry(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		FieldKeyID: "ITEM-001",
	}

	err := AddStatusHistoryEntry(obj, "exploring", "Initial creation", "account:user")
	if err != nil {
		t.Fatalf("AddStatusHistoryEntry failed: %v", err)
	}

	history, err := GetStatusHistory(obj)
	if err != nil {
		t.Fatalf("GetStatusHistory failed: %v", err)
	}

	if len(history) != 1 {
		t.Errorf("expected 1 entry, got %d", len(history))
	}

	if history[0].Status != "exploring" {
		t.Errorf("expected status 'exploring', got %s", history[0].Status)
	}

	if history[0].ChangedBy != "account:user" {
		t.Errorf("expected changed_by 'account:user', got %s", history[0].ChangedBy)
	}
}

func TestGetChangeLog(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		FieldKeyID: "ITEM-001",
		FieldKeyChangeLog: []any{
			map[string]any{
				"timestamp":  "2025-01-01T00:00:00Z",
				"changed_by": "account:user",
				"changes": map[string]any{
					FieldKeyTitle: map[string]any{
						"old": "Old Title",
						"new": "New Title",
					},
				},
				FieldKeyReason: "Updated title",
			},
		},
	}

	changeLog, err := GetChangeLog(obj)
	if err != nil {
		t.Fatalf("GetChangeLog failed: %v", err)
	}

	if len(changeLog) != 1 {
		t.Errorf("expected 1 entry, got %d", len(changeLog))
	}

	if changeLog[0].ChangedBy != "account:user" {
		t.Errorf("expected changed_by 'account:user', got %s", changeLog[0].ChangedBy)
	}

	if changeLog[0].Reason != "Updated title" {
		t.Errorf("expected reason 'Updated title', got %s", changeLog[0].Reason)
	}
}

func TestAddChangeLogEntry(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		FieldKeyID: "ITEM-001",
	}

	changes := map[string]any{
		FieldKeyTitle: map[string]any{
			"old": "Old Title",
			"new": "New Title",
		},
	}

	AddChangeLogEntry(obj, changes, "Updated title", "account:user")

	changeLog, err := GetChangeLog(obj)
	if err != nil {
		t.Fatalf("GetChangeLog failed: %v", err)
	}

	if len(changeLog) != 1 {
		t.Errorf("expected 1 entry, got %d", len(changeLog))
	}

	if changeLog[0].ChangedBy != "account:user" {
		t.Errorf("expected changed_by 'account:user', got %s", changeLog[0].ChangedBy)
	}
}
