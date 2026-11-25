package test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type stubCriteriaReadStorage struct {
	storage.ObjectStorageProvider
	objs map[string]map[string]any
}

func (s *stubCriteriaReadStorage) Read(_ context.Context, _ *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := s.objs[id]; ok {
		return obj, nil
	}
	return nil, fmt.Errorf("object %s not found", id)
}

func TestResolveAutoBindRequirements_PrefersCriteriaGraphEdgeOverLexicalFallback(t *testing.T) {
	sp := &stubCriteriaReadStorage{
		objs: map[string]map[string]any{
			"CRIT-GTM-001": {
				objects.FieldKeyID:              "CRIT-GTM-001",
				objects.FieldKeyRequirementRefs: []any{"REQ-GTM-OUTBOUND-001"},
			},
			"CRIT-GTM-002": {
				objects.FieldKeyID:              "CRIT-GTM-002",
				objects.FieldKeyRequirementRefs: []any{"REQ-GTM-OUTBOUND-001", "REQ-GTM-GATE-002"},
			},
		},
	}

	// Title intentionally has maximal lexical overlap with REQ-STARTER-COMMUNITY-001
	// to prove deterministic TST -> CRIT -> REQ traversal wins over lexical similarity.
	tcObj := map[string]any{
		objects.FieldKeyID:           "TST-GTM-001",
		objects.FieldKeyCriteriaRefs: []any{"CRIT-GTM-001", "CRIT-GTM-002"},
	}
	reqObjects := []map[string]any{
		{objects.FieldKeyID: "REQ-STARTER-COMMUNITY-001", objects.FieldKeyTitle: "Starter Community Core Launch Smoke Test"},
		{objects.FieldKeyID: "REQ-GTM-OUTBOUND-001", objects.FieldKeyTitle: "Adversarial Scoring Rubric"},
		{objects.FieldKeyID: "REQ-GTM-GATE-002", objects.FieldKeyTitle: "SHA-256 Human Approval Gate"},
	}

	got, viaCriteria := resolveAutoBindRequirements(
		context.Background(),
		sp,
		&pkgctx.SecurityContext{},
		"Starter Community Core Launch Smoke Test",
		tcObj,
		reqObjects,
	)
	if !viaCriteria {
		t.Fatalf("expected deterministic criteria_refs resolution, got lexical fallback (%v)", got)
	}
	want := []string{"REQ-GTM-OUTBOUND-001", "REQ-GTM-GATE-002"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved reqs mismatch: got %v, want %v", got, want)
	}
}

func TestResolveAutoBindRequirements_FallsBackToLexicalWhenNoCriteriaReqs(t *testing.T) {
	sp := &stubCriteriaReadStorage{
		objs: map[string]map[string]any{
			"CRIT-ORPHAN-001": {
				objects.FieldKeyID: "CRIT-ORPHAN-001",
			},
		},
	}
	tcObj := map[string]any{
		objects.FieldKeyID:           "TST-LEX-001",
		objects.FieldKeyCriteriaRefs: []any{"CRIT-ORPHAN-001"},
	}
	reqObjects := []map[string]any{
		{objects.FieldKeyID: "REQ-CRYPTO-001", objects.FieldKeyTitle: "Cryptographic Hash Validation"},
		{objects.FieldKeyID: "REQ-MESH-001", objects.FieldKeyTitle: "Reliable Mesh Interrupt Delivery"},
	}

	got, viaCriteria := resolveAutoBindRequirements(
		context.Background(),
		sp,
		&pkgctx.SecurityContext{},
		"Test for Cryptographic Hash Validation",
		tcObj,
		reqObjects,
	)
	if viaCriteria {
		t.Fatalf("expected lexical fallback when criteria has no requirement_refs")
	}
	want := []string{"REQ-CRYPTO-001"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fallback reqs mismatch: got %v, want %v", got, want)
	}
}

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
