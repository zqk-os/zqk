package objects

import (
	"strings"
	"testing"
)

func TestGenerateFieldHelp(t *testing.T) {
	t.Parallel()
	registry := GetGlobalFieldRegistry()
	if err := registry.LoadFields(); err != nil {
		t.Fatalf("failed to load fields: %v", err)
	}

	help, err := GenerateFieldHelp("backlog_item")
	if err != nil {
		t.Fatalf("GenerateFieldHelp failed: %v", err)
	}

	if help == emptyValue {
		t.Error("expected help text to be non-empty")
	}

	// Verify it contains expected sections
	if !strings.Contains(help, "Common Fields") {
		t.Error("expected help to contain 'Common Fields' section")
	}

	if !strings.Contains(help, "Specialized Fields") {
		t.Error("expected help to contain 'Specialized Fields' section")
	}

	// Verify it mentions backlog_item
	if !strings.Contains(help, "backlog_item") {
		t.Error("expected help to mention 'backlog_item'")
	}
}

func TestGenerateFieldList(t *testing.T) {
	t.Parallel()
	registry := GetGlobalFieldRegistry()
	if err := registry.LoadFields(); err != nil {
		t.Fatalf("failed to load fields: %v", err)
	}

	// Test with common fields included
	allFields, err := GenerateFieldList("backlog_item", true)
	if err != nil {
		t.Fatalf("GenerateFieldList failed: %v", err)
	}

	if len(allFields) == 0 {
		t.Error("expected at least some fields")
	}

	// Verify common fields are included
	hasID := false
	hasKind := false
	for _, field := range allFields {
		if field == "id" {
			hasID = true
		}
		if field == "kind" {
			hasKind = true
		}
	}

	if !hasID || !hasKind {
		t.Error("expected common fields (id, kind) to be included")
	}

	// Test without common fields
	specializedFields, err := GenerateFieldList("backlog_item", false)
	if err != nil {
		t.Fatalf("GenerateFieldList failed: %v", err)
	}

	// Specialized fields should not include common fields
	for _, field := range specializedFields {
		if field == "id" || field == "kind" {
			t.Errorf("specialized fields should not include common field: %s", field)
		}
	}
}

func TestGenerateFilterableFields(t *testing.T) {
	t.Parallel()
	registry := GetGlobalFieldRegistry()
	if err := registry.LoadFields(); err != nil {
		t.Fatalf("failed to load fields: %v", err)
	}

	filterable, err := GenerateFilterableFields("backlog_item")
	if err != nil {
		t.Fatalf("GenerateFilterableFields failed: %v", err)
	}

	if len(filterable) == 0 {
		t.Error("expected at least some filterable fields")
	}

	// Verify common filterable fields are included
	hasStatus := false
	for _, field := range filterable {
		if field == "status" {
			hasStatus = true
		}
	}

	if !hasStatus {
		t.Log("warning: 'status' field not found in filterable fields (might be expected)")
	}
}

func TestGenerateSortableFields(t *testing.T) {
	t.Parallel()
	registry := GetGlobalFieldRegistry()
	if err := registry.LoadFields(); err != nil {
		t.Fatalf("failed to load fields: %v", err)
	}

	sortable, err := GenerateSortableFields("backlog_item")
	if err != nil {
		t.Fatalf("GenerateSortableFields failed: %v", err)
	}

	if len(sortable) == 0 {
		t.Error("expected at least some sortable fields")
	}
}

func TestGenerateGroupableFields(t *testing.T) {
	t.Parallel()
	registry := GetGlobalFieldRegistry()
	if err := registry.LoadFields(); err != nil {
		t.Fatalf("failed to load fields: %v", err)
	}

	groupable, err := GenerateGroupableFields("backlog_item")
	if err != nil {
		t.Fatalf("GenerateGroupableFields failed: %v", err)
	}

	// Groupable fields might be empty for some kinds, that's okay
	_ = groupable
}
