package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestParseYAMLFile(t *testing.T) {
	t.Parallel()
	// Create a temporary YAML file
	tempDir := t.TempDir()
	yamlContent := `id: ITEM-001
kind: backlog_item
title: Test Backlog Item
status: planned
category: System
priority: high
milestone_refs:
  - MIL-001
  - MIL-002
description: |
  This is a test backlog item for migration testing.
context: |
  Testing YAML parsing for migration tools.
created_at: "2025-12-24T10:00:00Z"
created_by: account:test
updated_at: "2025-12-24T10:00:00Z"
updated_by: account:test
`

	filePath := filepath.Join(tempDir, "ITEM-001.yaml")
	//nolint:errcheck // Test cleanup - errors are acceptable
	os.WriteFile(filePath, []byte(yamlContent), paths.FilePerm644)

	// Test parsing
	parser := NewYAMLParser()
	obj, err := parser.ParseFile(filePath)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	// Verify object properties
	if obj.ID != "ITEM-001" {
		t.Errorf("Expected ID 'ITEM-001', got '%s'", obj.ID)
	}

	if obj.Kind != "backlog_item" {
		t.Errorf("Expected kind 'backlog_item', got '%s'", obj.Kind)
	}

	if obj.Title != "Test Backlog Item" {
		t.Errorf("Expected title 'Test Backlog Item', got '%s'", obj.Title)
	}

	// Verify extended fields
	if obj.Properties[objects.FieldKeyCategory] != "System" {
		t.Errorf("Expected category 'System', got '%v'", obj.Properties[objects.FieldKeyCategory])
	}

	if obj.Properties[objects.FieldKeyPriority] != "high" {
		t.Errorf("Expected priority 'high', got '%v'", obj.Properties[objects.FieldKeyPriority])
	}

	// Verify reference fields
	milestoneRefs, ok := obj.Properties[objects.FieldKeyMilestoneRefs].([]any)
	if !ok {
		t.Fatal("Expected milestone_refs to be a slice")
	}

	if len(milestoneRefs) != 2 {
		t.Errorf("Expected 2 milestone refs, got %d", len(milestoneRefs))
	}
}

func TestParseYAMLFile_InvalidYAML(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	invalidYAML := `id: ITEM-001
kind: backlog_item
title: Test
invalid: [unclosed bracket
`

	filePath := filepath.Join(tempDir, "invalid.yaml")
	//nolint:errcheck // Test cleanup - errors are acceptable
	os.WriteFile(filePath, []byte(invalidYAML), paths.FilePerm644)

	parser := NewYAMLParser()
	_, err := parser.ParseFile(filePath)
	if err == nil {
		t.Error("Expected error for invalid YAML, got nil")
	}
}

func TestParseYAMLFile_MissingRequiredFields(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	incompleteYAML := `title: Test Item
# Missing id and kind
`

	filePath := filepath.Join(tempDir, "incomplete.yaml")
	//nolint:errcheck // Test cleanup - errors are acceptable
	os.WriteFile(filePath, []byte(incompleteYAML), paths.FilePerm644)

	parser := NewYAMLParser()
	obj, err := parser.ParseFile(filePath)
	if err != nil {
		t.Fatalf("ParseFile should handle missing fields gracefully: %v", err)
	}

	// Should still parse, but with empty ID/kind
	if obj.ID != emptyValue {
		t.Errorf("Expected empty ID for missing field, got '%s'", obj.ID)
	}
}

func TestExtractReferenceFields(t *testing.T) {
	t.Parallel()
	parser := NewYAMLParser()

	properties := map[string]any{
		objects.FieldKeyMilestoneRefs:   []any{"MIL-001", "MIL-002"},
		objects.FieldKeyGoalRefs:        []any{"GOAL-001"},
		objects.FieldKeyPriorityPlanRef: "PLAN-001",
		objects.FieldKeyWorkstreamRef:   "WS-001",
		objects.FieldKeyTitle:           "Test",
	}

	refs := parser.ExtractReferenceFields(properties)

	// Should find all reference fields
	expectedRefs := 4
	if len(refs) != expectedRefs {
		t.Errorf("Expected %d reference fields, got %d", expectedRefs, len(refs))
	}

	// Verify specific references
	if refs[objects.FieldKeyMilestoneRefs] == nil {
		t.Error("Expected milestone_refs to be found")
	}

	milestoneRefs, ok := refs[objects.FieldKeyMilestoneRefs].([]any)
	if !ok || len(milestoneRefs) != 2 {
		t.Error("Expected milestone_refs to contain 2 items")
	}
}

func TestExtractReferenceFields_NoReferences(t *testing.T) {
	t.Parallel()
	parser := NewYAMLParser()

	properties := map[string]any{
		objects.FieldKeyTitle:    "Test",
		objects.FieldKeyStatus:   "active",
		objects.FieldKeyCategory: "System",
	}

	refs := parser.ExtractReferenceFields(properties)

	if len(refs) != 0 {
		t.Errorf("Expected 0 reference fields, got %d", len(refs))
	}
}
