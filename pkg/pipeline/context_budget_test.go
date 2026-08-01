package pipeline

import (
	"testing"
)

func TestAllocator_Allocate(t *testing.T) {
	// A simple graph: A -> B -> C
	// A is Core
	// So B and C should be Mandatory Structurals (because A depends on B and B depends on C)
	// Or rather, if A has DependsOn=[B], and B has DependsOn=[C].

	nodes := []ContextNode{
		{
			ID:        "A",
			Tokens:    10,
			Weight:    0.9,
			IsCore:    true,
			DependsOn: []string{"B"},
		},
		{
			ID:        "B",
			Tokens:    20,
			Weight:    0.5,
			IsCore:    false,
			DependsOn: []string{"C"},
		},
		{
			ID:        "C",
			Tokens:    30,
			Weight:    0.2,
			IsCore:    false,
			DependsOn: nil,
		},
		{
			ID:     "D", // Backfill
			Tokens: 50,
			Weight: 0.8,
			IsCore: false,
		},
		{
			ID:     "E", // Backfill
			Tokens: 100, // Won't fit
			Weight: 0.99,
			IsCore: false,
		},
	}

	alloc := NewAllocator(110) // Should fit A(10), B(20), C(30), and D(50). E is 100, so it won't fit if A+B+C takes 60. Wait: 110 - 60 = 50. So D fits! E has 100, E is evaluated first among backfill (weight 0.99). But E doesn't fit (100 > 50). So it skips E, and tries D. D fits!

	result := alloc.Allocate(nodes)

	if len(result) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(result))
	}

	// Order should be C, B, A, D
	// Why? C is dependency of B, B is dependency of A. So C -> B -> A.
	// D has no dependencies. It was added after mandatory and core.
	expectedIDs := []string{"C", "B", "A", "D"}
	for i, expected := range expectedIDs {
		if result[i].ID != expected {
			t.Errorf("at index %d: expected %s, got %s", i, expected, result[i].ID)
		}
	}
}

func TestAllocator_Cycle(t *testing.T) {
	nodes := []ContextNode{
		{
			ID:        "A",
			Tokens:    10,
			Weight:    0.9,
			IsCore:    true,
			DependsOn: []string{"B"},
		},
		{
			ID:        "B",
			Tokens:    10,
			Weight:    0.5,
			IsCore:    true,
			DependsOn: []string{"A"},
		},
	}

	alloc := NewAllocator(100)
	result := alloc.Allocate(nodes)

	if len(result) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(result))
	}
	// As long as it doesn't hang, cycle breaking worked.
}
