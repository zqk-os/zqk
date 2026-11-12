package community

import (
	"math"
	"testing"
)

func TestSemanticCapabilityIndex_Validation(t *testing.T) {
	idx := NewSemanticCapabilityIndex(3)

	err := idx.Register(CapabilityEntry{
		ID:     "cap-empty",
		Vector: []float64{},
	})
	if err != ErrEmptyVector {
		t.Fatalf("expected ErrEmptyVector, got %v", err)
	}

	err = idx.Register(CapabilityEntry{
		ID:     "cap-wrong-dim",
		Vector: []float64{1.0, 2.0},
	})
	if err != ErrDimensionMismatch {
		t.Fatalf("expected ErrDimensionMismatch, got %v", err)
	}
}

func TestSemanticCapabilityIndex_NearestSearch(t *testing.T) {
	idx := NewSemanticCapabilityIndex(3)

	if err := idx.Register(CapabilityEntry{
		ID:          "cap-math",
		Name:        "Vector Computation",
		Description: "Perform matrix and vector operations",
		Vector:      []float64{1.0, 0.0, 0.0},
	}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if err := idx.Register(CapabilityEntry{
		ID:          "cap-nlp",
		Name:        "Text Parsing",
		Description: "Perform linguistic and token parsing",
		Vector:      []float64{0.0, 1.0, 0.0},
	}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if err := idx.Register(CapabilityEntry{
		ID:          "cap-hybrid",
		Name:        "Hybrid Embeddings",
		Description: "Text with mathematical vectors",
		Vector:      []float64{0.8, 0.6, 0.0},
	}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Query closest to cap-math
	matches := idx.FindNearest([]float64{1.0, 0.1, 0.0}, 2, 0.5)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}

	if matches[0].Capability.ID != "cap-math" {
		t.Fatalf("expected 'cap-math' as top result, got %s", matches[0].Capability.ID)
	}

	if matches[1].Capability.ID != "cap-hybrid" {
		t.Fatalf("expected 'cap-hybrid' as second result, got %s", matches[1].Capability.ID)
	}
}

func TestCosineSimilarity_Calculations(t *testing.T) {
	// Identical vectors = 1.0
	simIdentical := CosineSimilarity([]float64{1.0, 2.0, 3.0}, []float64{1.0, 2.0, 3.0})
	if math.Abs(simIdentical-1.0) > 1e-6 {
		t.Fatalf("expected 1.0, got %f", simIdentical)
	}

	// Orthogonal vectors = 0.0
	simOrthogonal := CosineSimilarity([]float64{1.0, 0.0}, []float64{0.0, 1.0})
	if math.Abs(simOrthogonal-0.0) > 1e-6 {
		t.Fatalf("expected 0.0, got %f", simOrthogonal)
	}

	// Opposing vectors = -1.0
	simOpposing := CosineSimilarity([]float64{1.0, 0.0}, []float64{-1.0, 0.0})
	if math.Abs(simOpposing-(-1.0)) > 1e-6 {
		t.Fatalf("expected -1.0, got %f", simOpposing)
	}
}
