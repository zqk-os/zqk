package test

import (
	"testing"
)

func TestRankCandidates(t *testing.T) {
	candidates := []map[string]any{
		{"id": "REQ-CAS-DRAFT-SWEEP-RBAC-001", "title": "Draft Sweep RBAC Gate Enforcing Isolation"},
		{"id": "REQ-COMMS-RELIABLE-001", "title": "Reliable Mesh Interrupt Delivery"},
		{"id": "REQ-SECURITY-CRYPTO-001", "title": "Cryptographic Hash Validation"},
	}

	// Case 1: Match for Draft-Sweep RBAC
	matches := rankCandidates("Test for Draft-sweep RBAC gate", candidates)
	if len(matches) == 0 {
		t.Fatalf("expected candidate matches, got none")
	}
	if matches[0].ID != "REQ-CAS-DRAFT-SWEEP-RBAC-001" {
		t.Errorf("expected top candidate to be REQ-CAS-DRAFT-SWEEP-RBAC-001, got %s (score: %d)", matches[0].ID, matches[0].Score)
	}

	// Case 2: Match for Mesh Interrupt
	matches2 := rankCandidates("Test for Reliable mesh interrupt", candidates)
	if len(matches2) == 0 {
		t.Fatalf("expected candidate matches, got none")
	}
	if matches2[0].ID != "REQ-COMMS-RELIABLE-001" {
		t.Errorf("expected top candidate to be REQ-COMMS-RELIABLE-001, got %s (score: %d)", matches2[0].ID, matches2[0].Score)
	}
}

func TestExtractKeywords(t *testing.T) {
	text := "Test Case: Full-Lineage Traceability from Test Case to Root Object on Test Dashboard!"
	keywords := extractKeywords(text)

	// Stopwords like 'the', 'from', 'to', 'on' should be filtered out
	for _, kw := range keywords {
		if kw == "from" || kw == "to" || kw == "on" || kw == "the" {
			t.Errorf("unexpected stopword in extracted keywords: %s", kw)
		}
	}

	// Significant keywords should be present
	expectedPresent := []string{"full-lineage", "traceability", "root", "object", "dashboard"}
	kwMap := make(map[string]bool)
	for _, kw := range keywords {
		kwMap[kw] = true
	}

	for _, exp := range expectedPresent {
		if !kwMap[exp] {
			t.Errorf("expected keyword %q to be extracted, got: %v", exp, keywords)
		}
	}
}

func TestCalculateWordOverlap(t *testing.T) {
	target := []string{"traceability", "dashboard", "lineage"}
	candidate1 := []string{"traceability", "lineage", "kernel"}
	candidate2 := []string{"security", "crypto"}

	score1 := calculateWordOverlap(target, candidate1)
	score2 := calculateWordOverlap(target, candidate2)

	if score1 <= 0 {
		t.Errorf("expected positive score for overlapping words, got %d", score1)
	}
	if score2 != 0 {
		t.Errorf("expected 0 score for disjoint words, got %d", score2)
	}
	if score1 <= score2 {
		t.Errorf("expected score1 (%d) > score2 (%d)", score1, score2)
	}
}
