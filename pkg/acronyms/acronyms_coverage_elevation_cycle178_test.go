// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package acronyms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/mutation"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// mockStorageProvider implements storage.ObjectStorageProvider for acronyms tests.
type mockStorageProvider struct {
	storage.ObjectStorageProvider
	mu         sync.RWMutex
	items      map[string]map[string]any
	failCreate bool
	failUpdate bool
	failList   bool
	failRead   bool
	createErr  error
	updateErr  error
	listErr    error
	readErr    error
}

func newMockStorageProvider() *mockStorageProvider {
	return &mockStorageProvider{
		items: make(map[string]map[string]any),
	}
}

func (m *mockStorageProvider) Create(_ context.Context, _ *pkgctx.SecurityContext, obj map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failCreate {
		if m.createErr != nil {
			return m.createErr
		}
		return errors.New("mock create error")
	}
	id, _ := obj["id"].(string)
	if id == "" {
		return errors.New("missing id")
	}
	m.items[id] = obj
	return nil
}

func (m *mockStorageProvider) Read(_ context.Context, _ *pkgctx.SecurityContext, id string) (map[string]any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.failRead {
		if m.readErr != nil {
			return nil, m.readErr
		}
		return nil, errors.New("mock read error")
	}
	item, ok := m.items[id]
	if !ok {
		return nil, storage.ErrObjectNotFound
	}
	return item, nil
}

func (m *mockStorageProvider) Update(_ context.Context, _ *pkgctx.SecurityContext, id string, updates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failUpdate {
		if m.updateErr != nil {
			return m.updateErr
		}
		return errors.New("mock update error")
	}
	item, ok := m.items[id]
	if !ok {
		return storage.ErrObjectNotFound
	}
	for k, v := range updates {
		item[k] = v
	}
	return nil
}

func (m *mockStorageProvider) Delete(_ context.Context, _ *pkgctx.SecurityContext, id string, _ bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, id)
	return nil
}

func (m *mockStorageProvider) List(_ context.Context, _ *pkgctx.SecurityContext, _ *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.failList {
		if m.listErr != nil {
			return nil, m.listErr
		}
		return nil, errors.New("mock list error")
	}
	var results []map[string]any
	for _, obj := range m.items {
		if filter.Kind != "" && obj["kind"] != filter.Kind {
			continue
		}
		results = append(results, obj)
	}
	return &storage.QueryResult{Objects: results}, nil
}

func (m *mockStorageProvider) Query(_ context.Context, _ *pkgctx.SecurityContext, _ *pkgctx.StorageContext, _ storage.Query) (*storage.QueryResult, error) {
	return &storage.QueryResult{}, nil
}

func TestCanonicalSeedTerms_Integrity(t *testing.T) {
	terms := CanonicalSeedTerms()
	if len(terms) != 16 {
		t.Fatalf("expected 16 canonical terms, got %d", len(terms))
	}
	expectedCodes := map[string]string{
		"BLI": "work", "PRI": "work", "REQ": "specification", "CRIT": "verification",
		"VDS": "verification", "TCFG": "organization", "CVS": "coordination",
		"ATK": "execution", "PPLAN": "cli", "ZPARQL": "graph", "ZQL": "query",
		"CAS": "storage", "WAL": "storage", "CAP": "governance", "CEF": "evaluation",
		"TDE": "hygiene",
	}

	for _, term := range terms {
		cat, ok := expectedCodes[term.Code]
		if !ok {
			t.Errorf("unexpected seed code: %s", term.Code)
		}
		if term.Category != cat {
			t.Errorf("code %s: expected category %q, got %q", term.Code, cat, term.Category)
		}
		if term.FullName == "" || term.Definition == "" || term.Context != "operational" {
			t.Errorf("code %s has incomplete fields: %+v", term.Code, term)
		}
		if term.SchemeRef != KernelAcronymsSchemeID {
			t.Errorf("code %s: expected scheme %q, got %q", term.Code, KernelAcronymsSchemeID, term.SchemeRef)
		}
		if len(term.RelatedRefs) == 0 {
			t.Errorf("code %s: expected non-empty RelatedRefs", term.Code)
		}
	}
}

func TestToGlossaryTerm_Variations(t *testing.T) {
	// 1. With empty SchemeRef (defaults to KernelAcronymsSchemeID)
	a1 := Acronym{
		Code:        "DEMO",
		FullName:    "Demonstration Unit",
		Category:    "test",
		Definition:  "A unit for testing",
		Context:     "operational",
		RelatedRefs: []string{"BLI", "PRI"},
	}
	m1 := a1.ToGlossaryTerm()
	if m1[objects.FieldKeySchemeRef] != KernelAcronymsSchemeID {
		t.Errorf("expected default scheme %q, got %v", KernelAcronymsSchemeID, m1[objects.FieldKeySchemeRef])
	}
	if m1[objects.FieldKeyID] != "GLS-ACRONYM-DEMO" {
		t.Errorf("expected ID GLS-ACRONYM-DEMO, got %v", m1[objects.FieldKeyID])
	}
	if m1[objects.FieldKeyTitle] != "DEMO (Demonstration Unit)" {
		t.Errorf("expected title DEMO (Demonstration Unit), got %v", m1[objects.FieldKeyTitle])
	}

	// Verify machine_hints parsing
	hintsStr, ok := m1["machine_hints"].(string)
	if !ok || hintsStr == "" {
		t.Fatalf("expected non-empty machine_hints string")
	}
	var hints map[string]any
	if err := json.Unmarshal([]byte(hintsStr), &hints); err != nil {
		t.Fatalf("failed to unmarshal machine_hints: %v", err)
	}
	if hints["acronym"] != "DEMO" || hints["full_name"] != "Demonstration Unit" {
		t.Errorf("hints mismatch: %+v", hints)
	}

	// 2. With custom SchemeRef
	a2 := Acronym{
		Code:      "CUST",
		FullName:  "Custom Scheme Term",
		SchemeRef: "VOC-CUSTOM",
	}
	m2 := a2.ToGlossaryTerm()
	if m2[objects.FieldKeySchemeRef] != "VOC-CUSTOM" {
		t.Errorf("expected scheme VOC-CUSTOM, got %v", m2[objects.FieldKeySchemeRef])
	}
}

func TestFromGlossaryTerm_ValidationAndFallbacks(t *testing.T) {
	testFromGlossaryTerm_Rejections(t)
	testFromGlossaryTerm_HintsExtraction(t)
	testFromGlossaryTerm_TitleAndIDFallbacks(t)
}

func testFromGlossaryTerm_Rejections(t *testing.T) {
	// Nil object
	if _, ok := FromGlossaryTerm(nil); ok {
		t.Error("expected false for nil map")
	}
	// Wrong kind
	if _, ok := FromGlossaryTerm(map[string]any{"kind": "backlog_item"}); ok {
		t.Error("expected false for non-glossary_term kind")
	}
	// Non-matching scheme, id, category
	badTerm := map[string]any{
		"kind":       "glossary_term",
		"id":         "GLS-OTHER-123",
		"scheme_ref": "VOC-OTHER",
		"category":   "other",
	}
	if _, ok := FromGlossaryTerm(badTerm); ok {
		t.Error("expected false for unmatched scheme, id, and category")
	}
}

func testFromGlossaryTerm_HintsExtraction(t *testing.T) {
	// Hints containing related refs with strings and non-strings
	hintsJSON := `{"acronym":"HNT","full_name":"Hint Name","related_refs":["A", 123, "B"]}`
	obj := map[string]any{
		"kind":          "glossary_term",
		"id":            "GLS-ACRONYM-HNT",
		"scheme_ref":    KernelAcronymsSchemeID,
		"category":      "testing",
		"definition":    "Test hint def",
		"context_scope": "operational",
		"machine_hints": hintsJSON,
	}
	term, ok := FromGlossaryTerm(obj)
	if !ok {
		t.Fatalf("expected FromGlossaryTerm to succeed")
	}
	if term.Code != "HNT" || term.FullName != "Hint Name" {
		t.Errorf("unexpected term from hints: %+v", term)
	}
	if len(term.RelatedRefs) != 2 || term.RelatedRefs[0] != "A" || term.RelatedRefs[1] != "B" {
		t.Errorf("expected related refs [A, B], got: %v", term.RelatedRefs)
	}

	// Malformed JSON machine_hints handled gracefully
	obj["machine_hints"] = "{malformed-json"
	termMalformed, ok := FromGlossaryTerm(obj)
	if !ok || termMalformed.Code != "HNT" {
		t.Errorf("expected fallback code HNT from ID, got: %+v", termMalformed)
	}
}

func testFromGlossaryTerm_TitleAndIDFallbacks(t *testing.T) {
	// Title with parentheses extraction
	objParen := map[string]any{
		"kind":       "glossary_term",
		"id":         "GLS-ACRONYM-PAR",
		"scheme_ref": KernelAcronymsSchemeID,
		"title":      "PAR (Parentheses Title)",
	}
	termParen, ok := FromGlossaryTerm(objParen)
	if !ok || termParen.Code != "PAR" || termParen.FullName != "Parentheses Title" {
		t.Errorf("unexpected paren extraction: %+v", termParen)
	}

	// Title without parentheses (single word title)
	objSingle := map[string]any{
		"kind":     "glossary_term",
		"category": "acronym",
		"title":    "SINGLEWORD",
	}
	termSingle, ok := FromGlossaryTerm(objSingle)
	if !ok || termSingle.Code != "SINGLEWORD" || termSingle.FullName != "SINGLEWORD" {
		t.Errorf("unexpected singleword extraction: %+v", termSingle)
	}

	// Title with space but no parentheses
	objMulti := map[string]any{
		"kind":     "glossary_term",
		"category": "acronym",
		"title":    "MULTI word title",
	}
	termMulti, ok := FromGlossaryTerm(objMulti)
	if !ok || termMulti.Code != "MULTI" || termMulti.FullName != "MULTI word title" {
		t.Errorf("unexpected multiword extraction: %+v", termMulti)
	}

	// Completely empty code returns false
	objEmpty := map[string]any{
		"kind":     "glossary_term",
		"category": "acronym",
		"title":    "",
	}
	if _, okEmpty := FromGlossaryTerm(objEmpty); okEmpty {
		t.Error("expected false for empty title/code")
	}
}

func TestExprToValue_Scenarios(t *testing.T) {
	if val := exprToValue(nil); val != nil {
		t.Errorf("expected nil for nil expression, got %v", val)
	}

	litVal := mutation.LiteralExpr{Value: "hello"}
	if val := exprToValue(litVal); val != "hello" {
		t.Errorf("expected 'hello' for LiteralExpr, got %v", val)
	}

	litPtr := &mutation.LiteralExpr{Value: 12345}
	if val := exprToValue(litPtr); val != 12345 {
		t.Errorf("expected 12345 for *LiteralExpr, got %v", val)
	}

	varRef := mutation.VariableRefExpr{VariableName: "var"}
	if val := exprToValue(varRef); val != nil {
		t.Errorf("expected nil for non-literal expression, got %v", val)
	}
}

func TestLoadFromZQLSeed_SyntaxAndEdgeCases(t *testing.T) {
	// 1. Malformed syntax
	err := LoadFromZQLSeed("INVALID SYNTAX SCRIPT ???;")
	if err == nil {
		t.Error("expected error for malformed ZQL script")
	}

	// 2. Non-upsert and non-glossary_term upserts
	scriptIgnored := `
		BEGIN TRANSACTION;
		UPSERT priority_plan ID "PRI-TEST-001" {
			title: "Ignored Plan"
		};
		COMMIT TRANSACTION;
	`
	if err := LoadFromZQLSeed(scriptIgnored); err != nil {
		t.Fatalf("unexpected error for non-glossary script: %v", err)
	}

	// 3. Valid glossary_term upsert
	scriptGlossary := `
		UPSERT glossary_term ID "GLS-ACRONYM-ZQLTEST" {
			title: "ZQLTEST (ZQL Test Acronym)",
			category: "testing",
			definition: "An acronym loaded via ZQL seed script"
		};
	`
	if err := LoadFromZQLSeed(scriptGlossary); err != nil {
		t.Fatalf("unexpected error for valid glossary script: %v", err)
	}
	item, found := Lookup("ZQLTEST")
	if !found {
		t.Fatal("expected ZQLTEST to be registered in memory")
	}
	if item.FullName != "ZQL Test Acronym" || item.Definition != "An acronym loaded via ZQL seed script" {
		t.Errorf("unexpected loaded term: %+v", item)
	}

	// 4. Upsert glossary_term that fails FromGlossaryTerm (empty title & no id prefix)
	scriptInvalidGlossary := `
		UPSERT glossary_term ID "OTHER-ID" {
			title: ""
		};
	`
	if err := LoadFromZQLSeed(scriptInvalidGlossary); err != nil {
		t.Fatalf("unexpected error for invalid glossary upsert: %v", err)
	}
}

func TestListFromStore_AllScenarios(t *testing.T) {
	ctx := context.Background()

	// 1. Nil store fallback
	items, err := ListFromStore(ctx, nil, nil, "")
	if err != nil {
		t.Fatalf("unexpected error on nil store: %v", err)
	}
	if len(items) == 0 {
		t.Error("expected items from ListAll on nil store")
	}

	// 2. Store List error propagation
	spErr := newMockStorageProvider()
	spErr.failList = true
	spErr.listErr = errors.New("simulated list failure")
	if _, err := ListFromStore(ctx, nil, spErr, ""); err == nil {
		t.Error("expected error when store List fails")
	}

	// 3. Empty store with custom scheme returns empty slice without falling back to ListAll
	spEmpty := newMockStorageProvider()
	itemsEmpty, err := ListFromStore(ctx, nil, spEmpty, "VOC-NONEXISTENT")
	if err != nil {
		t.Fatalf("unexpected error on empty custom scheme: %v", err)
	}
	if len(itemsEmpty) != 0 {
		t.Errorf("expected 0 items for empty custom scheme, got %d", len(itemsEmpty))
	}

	// 4. Custom scheme, wildcard filtering, deduplication, sorting
	testListFromStore_FilteringAndDeduplication(t)
}

func testListFromStore_FilteringAndDeduplication(t *testing.T) {
	ctx := context.Background()
	sp := newMockStorageProvider()

	// Add item 1: custom scheme ALFA
	obj1 := map[string]any{
		"kind":          "glossary_term",
		"id":            "GLS-CUSTOM-ALFA",
		"scheme_ref":    "VOC-CUSTOM",
		"category":      "acronym",
		"title":         "ALFA (Alfa Term)",
		"context_scope": "operational",
	}
	if err := sp.Create(ctx, nil, obj1); err != nil {
		t.Fatalf("sp.Create obj1: %v", err)
	}

	// Add item 2: other scheme BETA
	obj2 := map[string]any{
		"kind":          "glossary_term",
		"id":            "GLS-OTHER-BETA",
		"scheme_ref":    "VOC-OTHER",
		"category":      "acronym",
		"title":         "BETA (Beta Term)",
		"context_scope": "operational",
	}
	if err := sp.Create(ctx, nil, obj2); err != nil {
		t.Fatalf("sp.Create obj2: %v", err)
	}

	// Add item 3: duplicate of ALFA in custom scheme
	obj3 := map[string]any{
		"kind":          "glossary_term",
		"id":            "GLS-CUSTOM-ALFA-DUP",
		"scheme_ref":    "VOC-CUSTOM",
		"category":      "acronym",
		"title":         "ALFA (Alfa Dup)",
		"context_scope": "operational",
	}
	if err := sp.Create(ctx, nil, obj3); err != nil {
		t.Fatalf("sp.Create obj3: %v", err)
	}

	// Add item 4: non-matching scheme without GLS- prefix
	obj4 := map[string]any{
		"kind":       "glossary_term",
		"id":         "OTHER-NONMATCH",
		"scheme_ref": "VOC-UNRELATED",
		"category":   "other",
		"title":      "UNRELATED",
	}
	if err := sp.Create(ctx, nil, obj4); err != nil {
		t.Fatalf("sp.Create obj4: %v", err)
	}

	// Query with wildcard "*"
	wildcardItems, err := ListFromStore(ctx, nil, sp, "*")
	if err != nil {
		t.Fatalf("unexpected error with wildcard: %v", err)
	}
	// Should have ALFA and BETA, deduplicated and sorted
	if len(wildcardItems) != 2 || wildcardItems[0].Code != "ALFA" || wildcardItems[1].Code != "BETA" {
		t.Errorf("unexpected wildcard results: %+v", wildcardItems)
	}

	// Query with custom scheme "VOC-CUSTOM"
	customItems, err := ListFromStore(ctx, nil, sp, "VOC-CUSTOM")
	if err != nil {
		t.Fatalf("unexpected error with custom scheme: %v", err)
	}
	if len(customItems) != 1 || customItems[0].Code != "ALFA" {
		t.Errorf("unexpected custom scheme results: %+v", customItems)
	}
}

func TestLookupInStore_Scenarios(t *testing.T) {
	ctx := context.Background()

	// 1. Nil store fallback
	term, found, err := LookupInStore(ctx, nil, nil, "BLI", "")
	if err != nil || !found || term.Code != "BLI" {
		t.Fatalf("unexpected nil store lookup result: found=%v, err=%v, term=%+v", found, err, term)
	}

	sp := newMockStorageProvider()
	a := Acronym{Code: "ZOOM", FullName: "Zoom Object Optimization Membrane", SchemeRef: KernelAcronymsSchemeID}
	if err := sp.Create(ctx, nil, a.ToGlossaryTerm()); err != nil {
		t.Fatalf("sp.Create ZOOM: %v", err)
	}

	// 2. Direct ID lookup without prefix
	termDirect, foundDirect, err := LookupInStore(ctx, nil, sp, "ZOOM", "")
	if err != nil || !foundDirect || termDirect.Code != "ZOOM" {
		t.Errorf("expected direct ID lookup to find ZOOM: found=%v, err=%v", foundDirect, err)
	}

	// 3. Direct ID lookup with prefix
	termPrefixed, foundPrefixed, err := LookupInStore(ctx, nil, sp, "GLS-ACRONYM-ZOOM", "")
	if err != nil || !foundPrefixed || termPrefixed.Code != "ZOOM" {
		t.Errorf("expected direct prefixed ID lookup to find ZOOM: found=%v, err=%v", foundPrefixed, err)
	}

	// 4. Lookup by FullName fallback
	termName, foundName, err := LookupInStore(ctx, nil, sp, "Zoom Object Optimization Membrane", "")
	if err != nil || !foundName || termName.Code != "ZOOM" {
		t.Errorf("expected lookup by full name to find ZOOM: found=%v, err=%v", foundName, err)
	}

	// 5. Lookup non-existent
	_, foundMissing, err := LookupInStore(ctx, nil, sp, "NONEXISTENT", "")
	if err != nil || foundMissing {
		t.Errorf("expected missing term not found: found=%v, err=%v", foundMissing, err)
	}

	// 6. Direct ID returns invalid object, falls through to ListFromStore
	invalidObj := map[string]any{"id": "GLS-ACRONYM-BAD", "kind": "not_glossary"}
	if err := sp.Create(ctx, nil, invalidObj); err != nil {
		t.Fatalf("sp.Create bad obj: %v", err)
	}
	_, foundBad, errBad := LookupInStore(ctx, nil, sp, "BAD", "")
	if errBad != nil {
		t.Fatalf("unexpected error looking up BAD: %v", errBad)
	}
	if foundBad {
		t.Error("expected bad object not to be recognized as acronym")
	}

	// 7. Store List error propagation
	spErr := newMockStorageProvider()
	spErr.failList = true
	if _, _, err := LookupInStore(ctx, nil, spErr, "UNKNOWN", ""); err == nil {
		t.Error("expected error when store List fails")
	}
}

func TestFindClosestInStore_Scenarios(t *testing.T) {
	ctx := context.Background()

	// 1. Store List error falls back to FindClosest(query)
	spErr := newMockStorageProvider()
	spErr.failList = true
	resFallback, err := FindClosestInStore(ctx, nil, spErr, "BL", "")
	if err != nil || len(resFallback) == 0 {
		t.Errorf("expected fallback suggestions on store list error: %v, %v", err, resFallback)
	}

	sp := newMockStorageProvider()
	terms := []Acronym{
		{Code: "ALPHA", FullName: "Alpha", SchemeRef: KernelAcronymsSchemeID},
		{Code: "ALPHABET", FullName: "Alphabet", SchemeRef: KernelAcronymsSchemeID},
		{Code: "BETA", FullName: "Beta", SchemeRef: KernelAcronymsSchemeID},
		{Code: "DELTA", FullName: "Delta", SchemeRef: KernelAcronymsSchemeID},
	}
	for _, term := range terms {
		if err := sp.Create(ctx, nil, term.ToGlossaryTerm()); err != nil {
			t.Fatalf("sp.Create term: %v", err)
		}
	}

	// 2. Prefix match
	matchesPrefix, err := FindClosestInStore(ctx, nil, sp, "AL", "")
	if err != nil || len(matchesPrefix) != 2 || matchesPrefix[0] != "ALPHA" || matchesPrefix[1] != "ALPHABET" {
		t.Errorf("unexpected prefix matches: %+v, err=%v", matchesPrefix, err)
	}

	// 3. Substring match
	matchesSub, err := FindClosestInStore(ctx, nil, sp, "ETA", "")
	if err != nil || len(matchesSub) != 1 || matchesSub[0] != "BETA" {
		t.Errorf("unexpected substring matches: %+v, err=%v", matchesSub, err)
	}

	// 4. Levenshtein distance fallback (distance <= 2)
	// "DELTO" has distance 1 to "DELTA"
	matchesLev, err := FindClosestInStore(ctx, nil, sp, "DELTO", "")
	if err != nil || len(matchesLev) != 1 || matchesLev[0] != "DELTA" {
		t.Errorf("unexpected levenshtein matches for DELTO: %+v, err=%v", matchesLev, err)
	}

	// 5. Distant query (distance > 2) returns empty
	matchesDistant, err := FindClosestInStore(ctx, nil, sp, "ZZZZZZZZ", "")
	if err != nil || len(matchesDistant) != 0 {
		t.Errorf("expected empty matches for distant query: %+v", matchesDistant)
	}
}

func TestLoadFromKernel_Scenarios(t *testing.T) {
	ctx := context.Background()

	// 1. Error on list failure
	spErr := newMockStorageProvider()
	spErr.failList = true
	if err := LoadFromKernel(ctx, nil, spErr); err == nil {
		t.Error("expected error from LoadFromKernel when List fails")
	}

	// 2. Successful load from kernel into registry
	sp := newMockStorageProvider()
	kernelTerm := Acronym{
		Code:        "KRNLTEST",
		FullName:    "Kernel State Test",
		SchemeRef:   KernelAcronymsSchemeID,
		Category:    "test",
		Definition:  "Loaded dynamically from kernel store",
		Context:     "operational",
		RelatedRefs: []string{"CAS"},
	}
	if err := sp.Create(ctx, nil, kernelTerm.ToGlossaryTerm()); err != nil {
		t.Fatalf("sp.Create kernelTerm: %v", err)
	}

	if err := LoadFromKernel(ctx, nil, sp); err != nil {
		t.Fatalf("LoadFromKernel failed: %v", err)
	}
	item, found := Lookup("KRNLTEST")
	if !found || item.FullName != "Kernel State Test" {
		t.Errorf("expected KRNLTEST in registry after LoadFromKernel: found=%v, item=%+v", found, item)
	}
}

func TestLookupAndListAll_DirectCache(t *testing.T) {
	populateFallbackSeed()

	// Lookup case sensitivity and whitespace
	item, found := Lookup("   bli   ")
	if !found || item.Code != "BLI" {
		t.Errorf("expected to find BLI with whitespace padding: found=%v, item=%+v", found, item)
	}

	// Unknown term
	_, foundMissing := Lookup("COMPLETELY_UNKNOWN_KEY")
	if foundMissing {
		t.Error("expected completely unknown key to not be found")
	}

	// ListAll alphabetical sorting
	all := ListAll()
	if len(all) < 16 {
		t.Errorf("expected at least 16 items in ListAll, got %d", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].Code < all[i-1].Code {
			t.Errorf("ListAll not sorted at index %d: %s comes after %s", i, all[i].Code, all[i-1].Code)
		}
	}
}

func TestFormatTable(t *testing.T) {
	sample := []Acronym{
		{Code: "BLI", FullName: "Backlog Item", Category: "work", Definition: "Unit of work"},
		{Code: "PRI", FullName: "Priority Plan", Category: "work", Definition: "Cycle of work"},
	}
	out := FormatTable(sample)
	if !strings.Contains(out, "CODE") || !strings.Contains(out, "FULL NAME") ||
		!strings.Contains(out, "CATEGORY") || !strings.Contains(out, "DEFINITION") {
		t.Errorf("table missing headers: %s", out)
	}
	if !strings.Contains(out, "Backlog Item") || !strings.Contains(out, "Priority Plan") {
		t.Errorf("table missing items: %s", out)
	}
}

func TestSyncToKernel_AllScenarios(t *testing.T) {
	ctx := context.Background()

	// 1. Nil store error
	if _, err := SyncToKernel(ctx, nil, nil); err == nil {
		t.Error("expected error when store is nil")
	}

	// 2. Normal sync: Create succeeds for scheme and all seed items
	sp := newMockStorageProvider()
	synced, err := SyncToKernel(ctx, nil, sp)
	if err != nil || synced != 16 {
		t.Fatalf("expected 16 synced terms, got %d, err: %v", synced, err)
	}

	// 3. Scheme create fails, falls back to scheme update
	testSyncToKernel_CreateFallbackToUpdate(t)

	// 4. Seed term create and update both fail
	testSyncToKernel_UpdateFailure(t)
}

func testSyncToKernel_CreateFallbackToUpdate(t *testing.T) {
	ctx := context.Background()
	sp := newMockStorageProvider()
	// Pre-populate scheme and all terms so Create fails on all of them
	// but Update succeeds.
	sp.failCreate = true
	sp.createErr = errors.New("already exists")

	// Pre-create the items in map so Update doesn't return not found
	sp.items[KernelAcronymsSchemeID] = map[string]any{"id": KernelAcronymsSchemeID}
	for _, seed := range CanonicalSeedTerms() {
		sp.items[fmt.Sprintf("GLS-ACRONYM-%s", seed.Code)] = map[string]any{"id": fmt.Sprintf("GLS-ACRONYM-%s", seed.Code)}
	}

	synced, err := SyncToKernel(ctx, nil, sp)
	if err != nil || synced != 16 {
		t.Fatalf("expected 16 terms synced via update fallback, got %d, err: %v", synced, err)
	}
}

func testSyncToKernel_UpdateFailure(t *testing.T) {
	ctx := context.Background()
	sp := newMockStorageProvider()
	sp.failCreate = true
	sp.failUpdate = true
	sp.updateErr = errors.New("fatal update failure")

	synced, err := SyncToKernel(ctx, nil, sp)
	if err == nil {
		t.Error("expected error when both create and update fail")
	}
	if synced != 0 {
		t.Errorf("expected 0 terms synced on fatal failure, got %d", synced)
	}
}

func TestLevenshteinDistance(t *testing.T) {
	if dist := levenshtein("kitten", "sitting"); dist != 3 {
		t.Errorf("expected distance 3, got %d", dist)
	}
	if dist := levenshtein("BLI", "BLI"); dist != 0 {
		t.Errorf("expected distance 0, got %d", dist)
	}
}

func TestConcurrency_SafeOperations(t *testing.T) {
	const readers = 6
	const writers = 4
	const iterations = 50

	var wg sync.WaitGroup
	errCh := make(chan error, (readers+writers)*iterations)

	// Launch reader goroutines
	for r := 0; r < readers; r++ {
		workerID := r
		wg.Add(1)
		goroutinelabels.NewGoroutine("acronyms_test_reader", fmt.Sprintf("reader-%d", workerID)).
			StartSimple(func() {
				defer wg.Done()
				for i := 0; i < iterations; i++ {
					item, found := Lookup("BLI")
					if !found || item.Code != "BLI" {
						errCh <- fmt.Errorf("reader %d: BLI not found", workerID)
					}
					all := ListAll()
					if len(all) == 0 {
						errCh <- fmt.Errorf("reader %d: ListAll returned empty", workerID)
					}
					matches := FindClosest("BL")
					if len(matches) == 0 {
						errCh <- fmt.Errorf("reader %d: FindClosest returned empty", workerID)
					}
				}
			})
	}

	// Launch writer goroutines
	for w := 0; w < writers; w++ {
		workerID := w
		wg.Add(1)
		goroutinelabels.NewGoroutine("acronyms_test_writer", fmt.Sprintf("writer-%d", workerID)).
			StartSimple(func() {
				defer wg.Done()
				for i := 0; i < iterations; i++ {
					populateFallbackSeed()
					script := fmt.Sprintf(`
						UPSERT glossary_term ID "GLS-ACRONYM-CONC%d" {
							title: "CONC%d (Concurrent Acronym)",
							definition: "Concurrent test definition"
						};
					`, workerID, workerID)
					if err := LoadFromZQLSeed(script); err != nil {
						errCh <- err
					}
				}
			})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent test error: %v", err)
		}
	}

	// Reset fallback seed for subsequent tests
	populateFallbackSeed()
}
