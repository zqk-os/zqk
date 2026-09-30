package objects

import (
	"testing"
)

func TestDynamicKindMapper_GetDirectoryFromKind(t *testing.T) {
	t.Parallel()
	mapper := GetGlobalKindMapper()

	// Test known mappings
	testCases := []struct {
		kind     string
		expected string
	}{
		{"backlog_item", "backlog_items"},
		{"goal", "goals"},
		{"component", "components"},
		{"priority_plan", "priority_plans"},
	}

	for _, tc := range testCases {
		dir := mapper.GetDirectoryFromKind(tc.kind)
		if dir != tc.expected {
			t.Errorf("GetDirectoryFromKind(%s) = %s, expected %s", tc.kind, dir, tc.expected)
		}
	}
}

func TestDynamicKindMapper_GetKindFromDirectory(t *testing.T) {
	t.Parallel()
	mapper := GetGlobalKindMapper()

	// Test known mappings
	testCases := []struct {
		dir      string
		expected string
	}{
		{"backlog", "backlog_item"},
		{"backlog_items", "backlog_item"},
		{"goals", "goal"},
		{"components", "component"},
		{"priority_plans", "priority_plan"},
	}

	for _, tc := range testCases {
		kind := mapper.GetKindFromDirectory(tc.dir)
		if kind != tc.expected {
			t.Errorf("GetKindFromDirectory(%s) = %s, expected %s", tc.dir, kind, tc.expected)
		}
	}
}

func TestDynamicKindMapper_GetAllKinds(t *testing.T) {
	t.Parallel()
	mapper := GetGlobalKindMapper()

	kinds := mapper.GetAllKinds()
	if len(kinds) == 0 {
		t.Error("expected at least some kinds to be discovered")
	}

	// Verify some expected kinds are present (at least the ones that should always exist)
	expectedKinds := []string{"backlog_item", "goal"}
	found := make(map[string]bool)
	for _, kind := range kinds {
		found[kind] = true
	}

	for _, expected := range expectedKinds {
		if !found[expected] {
			t.Errorf("expected kind %s not found in discovered kinds", expected)
		}
	}

	// Component might not be discovered if components directory doesn't exist
	// That's okay - the dynamic mapper only discovers kinds for existing directories
	t.Logf("Discovered %d kinds: %v", len(kinds), kinds)
}

func TestDynamicKindMapper_NonMappedDirectory(t *testing.T) {
	t.Parallel()
	mapper := GetGlobalKindMapper()

	// "decision" is not a mapped folder (the canonical folder is "decisions").
	// Without spec-filename inference, GetKindFromDirectory("decision") must return "".
	if got := mapper.GetKindFromDirectory("decision"); got != "" {
		t.Errorf("GetKindFromDirectory(\"decision\") = %q, want \"\"", got)
	}
}
