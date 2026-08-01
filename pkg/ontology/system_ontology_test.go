package ontology_test

import (
	"testing"

	"github.com/lanceman/zqk/pkg/ontology"
)

func TestSystemOntology(t *testing.T) {
	o := ontology.SystemOntology()

	if o.ID != "zqk-system-ontology" {
		t.Errorf("Expected ID zqk-system-ontology, got %s", o.ID)
	}
	if o.Version != "1.0" {
		t.Errorf("Expected Version 1.0, got %s", o.Version)
	}

	expectedClasses := []string{
		"goal", "milestone", "workstream", "priority_plan", "backlog_item",
		"requirement", "criteria", "test_case", "roadmap", "mission",
		"vision", "decision", "component", "account",
	}

	for _, class := range expectedClasses {
		if _, ok := o.Classes[class]; !ok {
			t.Errorf("Missing expected class in SystemOntology: %s", class)
		}
	}
}
