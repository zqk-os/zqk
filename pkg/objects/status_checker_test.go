package objects

import (
	"testing"
)

func TestStatusChecker_IsTerminal(t *testing.T) {
	checker := GetGlobalStatusChecker()

	// Test known kinds
	if !checker.IsTerminal("backlog_item", "complete") {
		t.Error("Expected backlog_item 'complete' to be terminal")
	}
	if checker.IsTerminal("backlog_item", "in_progress") {
		t.Error("Expected backlog_item 'in_progress' not to be terminal")
	}

	// Test fallback for unknown kinds (resolves to base_object_lifecycle.yaml)
	if !checker.IsTerminal("unknown_kind", "implemented") {
		t.Error("Expected unknown_kind 'implemented' to be terminal via base lifecycle fallback")
	}
	if checker.IsTerminal("unknown_kind", "complete") {
		t.Error("Expected unknown_kind 'complete' not to be terminal since base lifecycle uses 'implemented'")
	}
	if !checker.IsTerminal("backlog_item", "completed") {
		t.Error("Expected backlog_item 'completed' to resolve to terminal via alias mapping")
	}
}

func TestStatusChecker_IsPreliminary(t *testing.T) {
	checker := GetGlobalStatusChecker()

	// Test known kinds
	if !checker.IsPreliminary("backlog_item", "exploring") {
		t.Error("Expected backlog_item 'exploring' to be preliminary")
	}
	if !checker.IsPreliminary("backlog_item", "deferred") {
		t.Error("Expected backlog_item 'deferred' to be preliminary")
	}
	if checker.IsPreliminary("backlog_item", "in_progress") {
		t.Error("Expected backlog_item 'in_progress' not to be preliminary")
	}

	// Test fallback for unknown kinds (resolves to base_object_lifecycle.yaml)
	if !checker.IsPreliminary("unknown_kind", "proposed") {
		t.Error("Expected unknown_kind 'proposed' to be preliminary via base lifecycle fallback")
	}
}

func TestStatusChecker_IsActive(t *testing.T) {
	checker := GetGlobalStatusChecker()

	// Test known kinds
	if !checker.IsActive("backlog_item", "in_progress") {
		t.Error("Expected backlog_item 'in_progress' to be active")
	}
	if checker.IsActive("backlog_item", "exploring") {
		t.Error("Expected backlog_item 'exploring' not to be active")
	}
	if checker.IsActive("backlog_item", "complete") {
		t.Error("Expected backlog_item 'complete' not to be active")
	}

	// Test fallback for unknown kinds (resolves to base_object_lifecycle.yaml)
	if !checker.IsActive("unknown_kind", "approved") {
		t.Error("Expected unknown_kind 'approved' to be active via base lifecycle fallback")
	}
}
