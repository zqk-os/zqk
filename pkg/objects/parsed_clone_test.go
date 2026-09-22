package objects

import (
	"reflect"
	"testing"
	"time"
)

func TestParsedObject_Clone(t *testing.T) {
	var nilObj *ParsedObject
	if nilObj.Clone() != nil {
		t.Fatal("expected nil Clone() for nil ParsedObject")
	}

	now := time.Now()
	orig := &ParsedObject{
		ID:            "BLI-101",
		Kind:          "backlog_item",
		NamespaceID:   "NS-1",
		Status:        "in_progress",
		Title:         "Original Title",
		Category:      "feature",
		Priority:      "high",
		SchemaVersion: "1.0.0",
		TargetKind:    "goal",
		TargetID:      "GOAL-1",
		Operation:     "create",
		Severity:      "critical",
		CreatedAt:     now,
		UpdatedAt:     now,
		Raw: map[string]any{
			"custom_key": "custom_val",
			"count":      42,
		},
		StatusHistory: []StatusHistoryEntry{
			{Status: "planned", Timestamp: now, ChangedBy: "user1", Reason: "initial"},
		},
		ChangeLog: []ChangeLogEntry{
			{Changes: map[string]any{"status": "in_progress"}, Timestamp: now, ChangedBy: "user2"},
		},
		GoalRefs:        []string{"GOAL-1"},
		MilestoneRefs:   []string{"MS-1"},
		RequirementRefs: []string{"REQ-1"},
		WorkstreamRefs:  []string{"WS-1"},
		BacklogItemRefs: []string{"BLI-1"},
		CriteriaRefs:    []string{"CRIT-1"},
		TestCaseRefs:    []string{"TC-1"},
		DocEntryRefs:    []string{"DOC-1"},
		Artifacts:       []string{"art-1"},
		Dependencies:    []string{"dep-1"},
		Stakeholders:    []string{"user@test.org"},
		Questions:       []string{"question 1"},
	}

	clone := orig.Clone()

	if clone == nil {
		t.Fatal("expected non-nil clone")
	}
	if clone.ID != orig.ID || clone.Title != orig.Title || clone.Status != orig.Status {
		t.Errorf("scalar fields did not match: got %v, want %v", clone, orig)
	}

	// Verify deep copy of map
	clone.Raw["custom_key"] = "modified"
	if orig.Raw["custom_key"] == "modified" {
		t.Errorf("modifying clone Raw mutated original")
	}

	// Verify deep copy of status history
	clone.StatusHistory[0].Status = "modified"
	if orig.StatusHistory[0].Status == "modified" {
		t.Errorf("modifying clone StatusHistory mutated original")
	}

	// Verify deep copy of change log
	clone.ChangeLog[0].ChangedBy = "modified_user"
	if orig.ChangeLog[0].ChangedBy == "modified_user" {
		t.Errorf("modifying clone ChangeLog mutated original")
	}

	// Verify deep copy of string slices
	clone.GoalRefs[0] = "MOD-GOAL"
	if orig.GoalRefs[0] == "MOD-GOAL" {
		t.Errorf("modifying clone GoalRefs mutated original")
	}

	clone.WorkstreamRefs[0] = "MOD-WS"
	if orig.WorkstreamRefs[0] == "MOD-WS" {
		t.Errorf("modifying clone WorkstreamRefs mutated original")
	}

	// Test cloneStringSlice helper directly
	if cloneStringSlice(nil) != nil {
		t.Errorf("cloneStringSlice(nil) should return nil")
	}
	s := []string{"a", "b"}
	cs := cloneStringSlice(s)
	if !reflect.DeepEqual(s, cs) {
		t.Errorf("cloneStringSlice failed: got %v, want %v", cs, s)
	}
	cs[0] = "z"
	if s[0] == "z" {
		t.Errorf("mutating cloneStringSlice result mutated input")
	}
}

func TestParsedObject_WorkstreamLaneIDs(t *testing.T) {
	po := &ParsedObject{
		WorkstreamRefs: []string{"ws-alpha", "ws-beta"},
	}
	lanes := po.WorkstreamLaneIDs()
	if len(lanes) != 2 || lanes[0] != "ws-alpha" || lanes[1] != "ws-beta" {
		t.Errorf("unexpected WorkstreamLaneIDs: %v", lanes)
	}
}
