package objects

import (
	"testing"
	"time"
)

func TestParseTime_AllFormats(t *testing.T) {
	// 1. time.Time instance
	now := time.Now().UTC()
	if got, err := parseTime(now); err != nil || !got.Equal(now) {
		t.Fatalf("parseTime(time.Time) = (%v, %v), want %v", got, err, now)
	}

	// 2. RFC3339 string
	rfcStr := "2026-09-22T14:30:00Z"
	if got, err := parseTime(rfcStr); err != nil || got.Year() != 2026 {
		t.Fatalf("parseTime(RFC3339) = (%v, %v)", got, err)
	}

	// 3. RFC3339Nano string
	nanoStr := "2026-09-22T14:30:00.123456789Z"
	if got, err := parseTime(nanoStr); err != nil || got.Nanosecond() == 0 {
		t.Fatalf("parseTime(RFC3339Nano) = (%v, %v)", got, err)
	}

	// 4. Space separated string
	spaceStr := "2026-09-22 14:30:00"
	if got, err := parseTime(spaceStr); err != nil || got.Year() != 2026 {
		t.Fatalf("parseTime(spaceSeparated) = (%v, %v)", got, err)
	}

	// 5. Nil value
	if _, err := parseTime(nil); err == nil {
		t.Fatal("expected error for nil time value")
	}

	// 6. Invalid string format
	if _, err := parseTime("not-a-date"); err == nil {
		t.Fatal("expected error for unparseable time string")
	}

	// 7. Unsupported type (e.g. integer)
	if _, err := parseTime(123456789); err == nil {
		t.Fatal("expected error for unsupported time type")
	}
}

func TestParsedObject_GetField_AllTypedBranches(t *testing.T) {
	p := &ParsedObject{
		ID:              "BLI-1",
		Kind:            "backlog_item",
		NamespaceID:     "NS-1",
		Status:          "active",
		Title:           "Item",
		Category:        "feature",
		Priority:        "high",
		SchemaVersion:   "1.0.0",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		GoalRefs:        []string{"GOAL-1"},
		MilestoneRefs:   []string{"MIL-1"},
		RequirementRefs: []string{"REQ-1"},
		WorkstreamRefs:  []string{"WS-1"},
		BacklogItemRefs: []string{"BLI-2"},
		CriteriaRefs:    []string{"CRIT-1"},
		TestCaseRefs:    []string{"TC-1"},
		DocEntryRefs:    []string{"DOC-1"},
		StatusHistory:   []StatusHistoryEntry{{Status: "active"}},
		ChangeLog:       []ChangeLogEntry{{Reason: "init"}},
		TargetKind:      "criteria",
		TargetID:        "CRIT-2",
		Operation:       "update",
		Severity:        "critical",
		Raw: map[string]any{
			"custom_extra": "hello",
		},
	}

	fields := []string{
		FieldKeyID,
		FieldKeyKind,
		FieldKeyNamespaceID,
		FieldKeyStatus,
		FieldKeyTitle,
		FieldKeyCategory,
		FieldKeyPriority,
		FieldKeySchemaVersion,
		FieldKeyCreatedAt,
		FieldKeyUpdatedAt,
		FieldKeyGoalRefs,
		FieldKeyMilestoneRefs,
		FieldKeyRequirementRefs,
		FieldKeyWorkstreamRefs,
		FieldKeyBacklogItemRefs,
		FieldKeyCriteriaRefs,
		FieldKeyTestCaseRefs,
		FieldKeyDocEntryRefs,
		FieldKeyDocumentRefs,
		FieldKeyStatusHistory,
		FieldKeyChangeLog,
		FieldKeyTargetKind,
		FieldKeyTargetID,
		FieldKeyOperation,
		FieldKeySeverity,
		"custom_extra",
	}

	for _, f := range fields {
		if val, ok := p.GetField(f); !ok || val == nil {
			t.Errorf("GetField(%q) = (%v, %v), want present", f, val, ok)
		}
	}

	if _, ok := p.GetField("nonexistent_field_xyz"); ok {
		t.Error("expected ok=false for nonexistent field")
	}
}

func TestParseObjectMinimal(t *testing.T) {
	raw := map[string]any{
		FieldKeyID:          "BLI-MIN-1",
		FieldKeyKind:        "backlog_item",
		FieldKeyNamespaceID: "NS-DEFAULT",
		FieldKeyStatus:      "proposed",
		"other_unneeded":    "data",
	}

	parsed := ParseObjectMinimal(raw)
	if parsed.ID != "BLI-MIN-1" {
		t.Errorf("expected ID BLI-MIN-1, got %s", parsed.ID)
	}
	if parsed.Kind != "backlog_item" {
		t.Errorf("expected kind backlog_item, got %s", parsed.Kind)
	}
	if parsed.NamespaceID != "NS-DEFAULT" {
		t.Errorf("expected namespace NS-DEFAULT, got %s", parsed.NamespaceID)
	}
	if parsed.Status != "proposed" {
		t.Errorf("expected status proposed, got %s", parsed.Status)
	}
}
