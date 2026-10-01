package acronyms

import (
	"strings"
	"testing"
)

// TestStaticFloor_RequiredAcronymCoverage verifies all 16 required kernel acronyms exist and have complete metadata.
func TestStaticFloor_RequiredAcronymCoverage(t *testing.T) {
	requiredCodes := []string{
		"BLI", "PRI", "REQ", "CRIT", "VDS", "TCFG", "CVS", "ATK",
		"PPLAN", "ZPARQL", "ZQL", "CAS", "WAL", "CAP", "CEF", "TDE",
	}

	for _, code := range requiredCodes {
		item, exists := Lookup(code)
		if !exists {
			t.Fatalf("required acronym %q is missing from registry", code)
		}
		if item.Code != code {
			t.Errorf("expected code %q, got %q", code, item.Code)
		}
		if strings.TrimSpace(item.FullName) == "" {
			t.Errorf("acronym %q has empty FullName", code)
		}
		if strings.TrimSpace(item.Definition) == "" {
			t.Errorf("acronym %q has empty Definition", code)
		}
		if strings.TrimSpace(item.Category) == "" {
			t.Errorf("acronym %q has empty Category", code)
		}
	}
}

// TestOperationalProof_CaseInsensitiveLookupAndListing verifies case-insensitive lookup and sorting.
func TestOperationalProof_CaseInsensitiveLookupAndListing(t *testing.T) {
	cases := []string{"bli", "Bli", "BLI", "  bli  "}
	for _, tc := range cases {
		item, found := Lookup(tc)
		if !found {
			t.Fatalf("expected to find acronym for %q", tc)
		}
		if item.Code != "BLI" || item.FullName != "Backlog Item" {
			t.Errorf("unexpected lookup result for %q: %+v", tc, item)
		}
	}

	all := ListAll()
	if len(all) < 16 {
		t.Errorf("expected at least 16 acronyms, got %d", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].Code < all[i-1].Code {
			t.Errorf("ListAll() is not sorted: %q comes after %q", all[i].Code, all[i-1].Code)
		}
	}

	table := FormatTable(all)
	if !strings.Contains(table, "CODE") || !strings.Contains(table, "Backlog Item") {
		t.Errorf("FormatTable output missing expected content:\n%s", table)
	}
}

// TestNegativeBoundary_UnknownAndFuzzySuggestions verifies behavior for unknown queries.
func TestNegativeBoundary_UnknownAndFuzzySuggestions(t *testing.T) {
	_, found := Lookup("NONEXISTENT_XYZ")
	if found {
		t.Fatal("expected NONEXISTENT_XYZ to not be found")
	}

	// Suggestion test for near match "BL" -> "BLI"
	suggestions := FindClosest("BL")
	if len(suggestions) == 0 {
		t.Fatal("expected suggestions for query 'BL'")
	}
	foundBLI := false
	for _, s := range suggestions {
		if s == "BLI" {
			foundBLI = true
			break
		}
	}
	if !foundBLI {
		t.Errorf("expected 'BLI' in suggestions for 'BL', got: %v", suggestions)
	}
}

// TestGlossaryTermProjection_RoundTrip verifies that Acronym functions as a spec-compliant projection of glossary_term.
func TestGlossaryTermProjection_RoundTrip(t *testing.T) {
	bli, found := Lookup("BLI")
	if !found {
		t.Fatal("BLI not found")
	}
	if bli.SchemeRef != KernelAcronymsSchemeID {
		t.Errorf("expected SchemeRef %q, got %q", KernelAcronymsSchemeID, bli.SchemeRef)
	}

	obj := bli.ToGlossaryTerm()
	if obj["kind"] != "glossary_term" {
		t.Errorf("expected kind 'glossary_term', got %v", obj["kind"])
	}
	if obj["scheme_ref"] != KernelAcronymsSchemeID {
		t.Errorf("expected scheme_ref %q, got %v", KernelAcronymsSchemeID, obj["scheme_ref"])
	}

	restored, ok := FromGlossaryTerm(obj)
	if !ok {
		t.Fatal("failed to restore Acronym from glossary_term object map")
	}
	if restored.Code != bli.Code || restored.FullName != bli.FullName || restored.SchemeRef != bli.SchemeRef {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", restored, bli)
	}
}
