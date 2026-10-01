package databook

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildDataBookFromObjects_Basic(t *testing.T) {
	objs := []map[string]any{
		{
			"id":                        "GOAL-001",
			"kind":                      "goal",
			"title":                     "Enterprise Microkernel Launch",
			"status":                    "complete",
			"acceptance_considerations": "Zero telemetry gaps and pass all gates",
		},
		{
			"id":            "REQ-001",
			"kind":          "requirement",
			"title":         "W3C Holon Serialization Support",
			"status":        "active",
			"goal_refs":     []string{"GOAL-001"},
			"criteria_refs": []string{"CRIT-001"},
		},
	}

	db, err := BuildDataBookFromObjects(objs, "zqk/test-v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if db.Type != DataBookType {
		t.Errorf("expected type %q, got %q", DataBookType, db.Type)
	}
	if len(db.Holons) != 2 {
		t.Fatalf("expected 2 holons, got %d", len(db.Holons))
	}

	// Verify REQ-001 Holon
	var reqHolon *Holon
	for i := range db.Holons {
		if db.Holons[i].ID == "REQ-001" {
			reqHolon = &db.Holons[i]
			break
		}
	}
	if reqHolon == nil {
		t.Fatal("REQ-001 holon not found")
	}

	if reqHolon.AtID != "urn:zqk:object:requirement:REQ-001" {
		t.Errorf("expected @id urn:zqk:object:requirement:REQ-001, got %q", reqHolon.AtID)
	}
	if len(reqHolon.Wholes) != 1 || reqHolon.Wholes[0] != "urn:zqk:object:ref:GOAL-001" {
		t.Errorf("expected wholes with GOAL-001, got %v", reqHolon.Wholes)
	}
	if len(reqHolon.Parts) != 1 || reqHolon.Parts[0] != "urn:zqk:object:ref:CRIT-001" {
		t.Errorf("expected parts with CRIT-001, got %v", reqHolon.Parts)
	}

	// Test JSON-LD export
	jsonBytes, err := ExportToJSONLD(db)
	if err != nil {
		t.Fatalf("failed to export JSON-LD: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("invalid JSON produced: %v", err)
	}
	if parsed["@type"] != "DataBook" {
		t.Errorf("expected JSON @type DataBook, got %v", parsed["@type"])
	}

	// Test Turtle RDF export
	turtleStr := ExportToTurtle(db)
	if !strings.Contains(turtleStr, "@prefix holon: <https://www.w3.org/ns/holon/v1#>") {
		t.Errorf("expected holon prefix in Turtle output")
	}
	if !strings.Contains(turtleStr, "<urn:zqk:object:requirement:REQ-001> a holon:Holon, zqk:Requirement") {
		t.Errorf("expected requirement holon statement in Turtle output, got:\n%s", turtleStr)
	}
	if !strings.Contains(turtleStr, "holon:whole <urn:zqk:object:ref:GOAL-001>") {
		t.Errorf("expected whole link in Turtle output, got:\n%s", turtleStr)
	}
	if !strings.Contains(turtleStr, "holon:part <urn:zqk:object:ref:CRIT-001>") {
		t.Errorf("expected part link in Turtle output, got:\n%s", turtleStr)
	}
}
