package objects

import (
	"testing"
)

type mockSynonymProvider struct {
	synonyms map[string][]string
}

func (m *mockSynonymProvider) GetSynonymsForKind(kind string) []string {
	return m.synonyms[kind]
}

type mockSynonymLoaderExt struct {
	data []SynonymData
}

func (m *mockSynonymLoaderExt) LoadSynonyms() ([]SynonymData, error) {
	return m.data, nil
}

func TestKindSynonymResolver_Extended(t *testing.T) {
	ksr := NewKindSynonymResolver()

	// Provider setup
	provider := &mockSynonymProvider{
		synonyms: map[string][]string{
			"goal": {"obj", "target"},
		},
	}
	ksr.SetSynonymProvider(provider)

	// Custom loader with priority conflict resolution
	loader := &mockSynonymLoaderExt{
		data: []SynonymData{
			{Kind: "goal", Synonym: "target", Priority: 10},
			{Kind: "milestone", Synonym: "target", Priority: 20}, // higher priority wins for "target"
			{Kind: "goal", Synonym: "obj", Priority: 5},
			{Kind: "goal", Synonym: "obj", Priority: 15}, // duplicate within same kind with higher priority
			{Kind: "", Synonym: "invalid", Priority: 1},  // empty kind, skipped
			{Kind: "task", Synonym: "", Priority: 1},     // empty synonym, skipped
		},
	}
	ksr.SetSynonymLoader(loader)

	if err := ksr.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// "target" should resolve to milestone because priority 20 > 10
	resolvedTarget := ksr.ResolveKind("target")
	if resolvedTarget != "milestone" {
		t.Errorf("ResolveKind('target') = %s, want milestone", resolvedTarget)
	}

	// "obj" should resolve to goal
	resolvedObj := ksr.ResolveKind("obj")
	if resolvedObj != "goal" {
		t.Errorf("ResolveKind('obj') = %s, want goal", resolvedObj)
	}

	// Direct kind lookup (case-insensitive)
	if resolvedGoal := ksr.ResolveKind("GOAL"); resolvedGoal != "goal" {
		t.Errorf("ResolveKind('GOAL') = %s, want goal", resolvedGoal)
	}

	// Unknown input returned as-is
	if unknown := ksr.ResolveKind("completely_unknown_xyz"); unknown != "completely_unknown_xyz" {
		t.Errorf("ResolveKind('completely_unknown_xyz') = %s, want completely_unknown_xyz", unknown)
	}

	// GetAllSynonyms
	all := ksr.GetAllSynonyms()
	if all == nil || len(all["goal"]) == 0 {
		t.Errorf("GetAllSynonyms missing goal: %v", all)
	}

	// GetSynonyms
	goalSyns := ksr.GetSynonyms("goal")
	if len(goalSyns) == 0 {
		t.Errorf("GetSynonyms('goal') empty")
	}

	// ValidateUniqueness
	conflicts := ksr.ValidateUniqueness()
	if conflicts == nil {
		t.Errorf("expected non-nil conflicts map")
	}

	// Reset via SetSynonymLoader
	ksr.SetSynonymLoader(&mockSynonymLoaderExt{data: []SynonymData{
		{Kind: "backlog_item", Synonym: "bli", Priority: 1},
	}})

	// Algorithmic synonym generation check
	synsTwoWord := ksr.generateSynonyms("code_review")
	hasCR := false
	for _, s := range synsTwoWord {
		if s == "cr" {
			hasCR = true
		}
	}
	if !hasCR {
		t.Errorf("expected 'cr' abbreviation for two-word kind 'code_review', got %v", synsTwoWord)
	}

	synsMultiWord := ksr.generateSynonyms("test_case_result")
	hasTCR := false
	for _, s := range synsMultiWord {
		if s == "tcr" {
			hasTCR = true
		}
	}
	if !hasTCR {
		t.Errorf("expected 'tcr' abbreviation for multi-word kind 'test_case_result', got %v", synsMultiWord)
	}

	synsSingleWord := ksr.generateSynonyms("artifact")
	hasArti := false
	for _, s := range synsSingleWord {
		if s == "arti" {
			hasArti = true
		}
	}
	if !hasArti {
		t.Errorf("expected 'arti' prefix for single-word kind 'artifact', got %v", synsSingleWord)
	}

	// GetCanonicalKind helper
	canon := GetCanonicalKind("backlog_item")
	if canon != "backlog_item" {
		t.Errorf("GetCanonicalKind(backlog_item) = %s", canon)
	}
}
