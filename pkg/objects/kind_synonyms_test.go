package objects

import (
	"testing"
)

// mockSynonymLoader is a test implementation of SynonymLoader
type mockSynonymLoader struct {
	synonyms []SynonymData
	err      error
}

func (m *mockSynonymLoader) LoadSynonyms() ([]SynonymData, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.synonyms, nil
}

func TestKindSynonymResolver_LoadFromStorage(t *testing.T) {
	t.Parallel()
	loader := &mockSynonymLoader{
		synonyms: []SynonymData{
			{Kind: "backlog_item", Synonym: "bi", Priority: 10},
			{Kind: "backlog_item", Synonym: "bli", Priority: 5},
			{Kind: "priority_plan", Synonym: "pp", Priority: 10},
		},
	}

	resolver := NewKindSynonymResolver()
	resolver.SetSynonymLoader(loader)

	if err := resolver.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Test resolving synonyms
	tests := []struct {
		input    string
		expected string
	}{
		{"bi", "backlog_item"},
		{"BI", "backlog_item"}, // Case insensitive
		{"bli", "backlog_item"},
		{"pp", "priority_plan"},
		{"backlog_item", "backlog_item"}, // Canonical name
		{"unknown", "unknown"},           // Not found
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := resolver.ResolveKind(tt.input)
			if result != tt.expected {
				t.Errorf("ResolveKind(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}

	// Test GetSynonyms
	synonyms := resolver.GetSynonyms("backlog_item")
	expectedSynonyms := []string{"bi", "bli"}
	if len(synonyms) != len(expectedSynonyms) {
		t.Errorf("GetSynonyms('backlog_item') returned %d synonyms, want %d", len(synonyms), len(expectedSynonyms))
	}

	// Check that both synonyms are present
	synonymMap := make(map[string]bool)
	for _, s := range synonyms {
		synonymMap[s] = true
	}
	for _, expected := range expectedSynonyms {
		if !synonymMap[expected] {
			t.Errorf("GetSynonyms('backlog_item') missing synonym %q", expected)
		}
	}
}

func TestKindSynonymResolver_DeduplicatePerTargetKind(t *testing.T) {
	t.Parallel()
	// Test that duplicates per target_kind are handled correctly
	// Higher priority should win
	loader := &mockSynonymLoader{
		synonyms: []SynonymData{
			{Kind: "backlog_item", Synonym: "bi", Priority: 5},  // Lower priority
			{Kind: "backlog_item", Synonym: "bi", Priority: 10}, // Higher priority - should win
			{Kind: "backlog_item", Synonym: "bi", Priority: 8},  // Middle priority - should be ignored
		},
	}

	resolver := NewKindSynonymResolver()
	resolver.SetSynonymLoader(loader)

	if err := resolver.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Should resolve to backlog_item
	result := resolver.ResolveKind("bi")
	if result != "backlog_item" {
		t.Errorf("ResolveKind('bi') = %q, want 'backlog_item'", result)
	}

	// Should only have one "bi" synonym (not duplicates)
	synonyms := resolver.GetSynonyms("backlog_item")
	biCount := 0
	for _, s := range synonyms {
		if s == "bi" {
			biCount++
		}
	}
	if biCount != 1 {
		t.Errorf("GetSynonyms('backlog_item') has %d 'bi' entries, want 1", biCount)
	}

	// Priority should be 10 (highest)
	priority := resolver.synonymToPriority["bi"]
	if priority != 10 {
		t.Errorf("Priority for 'bi' = %d, want 10", priority)
	}
}

func TestKindSynonymResolver_CrossKindConflictResolution(t *testing.T) {
	t.Parallel()
	// Test that when the same synonym maps to different kinds,
	// the highest priority wins
	loader := &mockSynonymLoader{
		synonyms: []SynonymData{
			{Kind: "backlog_item", Synonym: "bi", Priority: 5},
			{Kind: "priority_plan", Synonym: "bi", Priority: 10}, // Higher priority
		},
	}

	resolver := NewKindSynonymResolver()
	resolver.SetSynonymLoader(loader)

	if err := resolver.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// "bi" should resolve to priority_plan (higher priority)
	result := resolver.ResolveKind("bi")
	if result != "priority_plan" {
		t.Errorf("ResolveKind('bi') = %q, want 'priority_plan'", result)
	}

	// Both kinds should have "bi" in their synonyms list
	backlogSynonyms := resolver.GetSynonyms("backlog_item")
	planSynonyms := resolver.GetSynonyms("priority_plan")

	backlogHasBi := false
	for _, s := range backlogSynonyms {
		if s == "bi" {
			backlogHasBi = true
			break
		}
	}

	planHasBi := false
	for _, s := range planSynonyms {
		if s == "bi" {
			planHasBi = true
			break
		}
	}

	if !backlogHasBi {
		t.Error("backlog_item should have 'bi' in its synonyms list")
	}
	if !planHasBi {
		t.Error("priority_plan should have 'bi' in its synonyms list")
	}
}

func TestKindSynonymResolver_SetSynonymLoader_ResetsState(t *testing.T) {
	t.Parallel()
	// Test that setting a loader after initialization resets state
	resolver := NewKindSynonymResolver()

	// Initialize with generated synonyms (no loader)
	if err := resolver.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Verify it's initialized
	if !resolver.initialized {
		t.Error("Resolver should be initialized")
	}

	// Set a loader with storage synonyms
	loader := &mockSynonymLoader{
		synonyms: []SynonymData{
			{Kind: "backlog_item", Synonym: "bi", Priority: 10},
		},
	}
	resolver.SetSynonymLoader(loader)

	// State should be reset (initialized = false)
	if resolver.initialized {
		t.Error("Resolver should be reset after SetSynonymLoader when previously initialized without loader")
	}

	// Re-initialize should load from storage
	if err := resolver.Initialize(); err != nil {
		t.Fatalf("Re-initialize failed: %v", err)
	}

	// Should now resolve using storage synonyms
	result := resolver.ResolveKind("bi")
	if result != "backlog_item" {
		t.Errorf("ResolveKind('bi') = %q, want 'backlog_item'", result)
	}
}

func TestKindSynonymResolver_ValidateUniqueness(t *testing.T) {
	t.Parallel()
	loader := &mockSynonymLoader{
		synonyms: []SynonymData{
			{Kind: "backlog_item", Synonym: "bi", Priority: 10},
			{Kind: "priority_plan", Synonym: "bi", Priority: 5}, // Same synonym, different kind
			{Kind: "backlog_item", Synonym: "bli", Priority: 5},
		},
	}

	resolver := NewKindSynonymResolver()
	resolver.SetSynonymLoader(loader)

	if err := resolver.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// ValidateUniqueness should find conflicts (same synonym for different kinds)
	conflicts := resolver.ValidateUniqueness()

	// "bi" should be in conflicts (maps to both backlog_item and priority_plan)
	if kinds, exists := conflicts["bi"]; !exists {
		t.Error("ValidateUniqueness() should report 'bi' as a conflict")
	} else if len(kinds) != 2 {
		t.Errorf("Conflict 'bi' should map to 2 kinds, got %d", len(kinds))
	}
}

func TestKindSynonymResolver_EmptyLoader(t *testing.T) {
	t.Parallel()
	// Test behavior when loader returns empty list
	loader := &mockSynonymLoader{
		synonyms: []SynonymData{},
	}

	resolver := NewKindSynonymResolver()
	resolver.SetSynonymLoader(loader)

	if err := resolver.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Should fall back to generated synonyms
	// backlog_item should have generated synonyms
	synonyms := resolver.GetSynonyms("backlog_item")
	if len(synonyms) == 0 {
		t.Error("Should have generated synonyms for backlog_item")
	}
}

func TestKindSynonymResolver_CaseInsensitive(t *testing.T) {
	t.Parallel()
	loader := &mockSynonymLoader{
		synonyms: []SynonymData{
			{Kind: "backlog_item", Synonym: "bi", Priority: 10},
		},
	}

	resolver := NewKindSynonymResolver()
	resolver.SetSynonymLoader(loader)

	if err := resolver.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Test case insensitive resolution
	tests := []struct {
		input    string
		expected string
	}{
		{"bi", "backlog_item"},
		{"BI", "backlog_item"},
		{"Bi", "backlog_item"},
		{"bI", "backlog_item"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := resolver.ResolveKind(tt.input)
			if result != tt.expected {
				t.Errorf("ResolveKind(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestKindSynonymResolver_GeneratedSynonyms_IncludeDefaultAliases(t *testing.T) {
	t.Parallel()

	resolver := NewKindSynonymResolver()

	// This exercises generateSynonyms() directly (no storage/config needed).
	synonyms := resolver.generateSynonyms("backlog_item")

	// Canonical always present
	foundCanonical := false
	for _, s := range synonyms {
		if s == "backlog_item" {
			foundCanonical = true
			break
		}
	}
	if !foundCanonical {
		t.Fatalf("expected generated synonyms to include canonical kind")
	}

	// Baseline alias shared with AliasRegistry
	foundTask := false
	for _, s := range synonyms {
		if s == "task" {
			foundTask = true
			break
		}
	}
	if !foundTask {
		t.Fatalf("expected generated synonyms for backlog_item to include default alias %q; got %v", "task", synonyms)
	}

	synPT := resolver.generateSynonyms("prompt_template")
	var hasPrompt bool
	for _, s := range synPT {
		if s == "prompt" {
			hasPrompt = true
			break
		}
	}
	if !hasPrompt {
		t.Fatalf("expected generated synonyms for prompt_template to include default alias %q; got %v", "prompt", synPT)
	}
}
