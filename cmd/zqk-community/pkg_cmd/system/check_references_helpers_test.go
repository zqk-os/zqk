package system

import "testing"

func TestParseReferenceID_domainOrganizational(t *testing.T) {
	t.Parallel()
	tests := []struct {
		refID      string
		wantKind   string
		wantActual string
	}{
		{"domain:organizational:department:DEP-001", "department", "DEP-001"},
		{"domain:organizational:division:DIV-001", "division", "DIV-001"},
		{"domain:organizational:organization:ORG-001", "organization", "ORG-001"},
		{"domain:organizational:team:TEA-001", "team", "TEA-001"},
	}
	for _, tt := range tests {
		gotKind, gotActual := parseReferenceID(tt.refID)
		if gotKind != tt.wantKind || gotActual != tt.wantActual {
			t.Errorf("parseReferenceID(%q) = (%q, %q), want (%q, %q)", tt.refID, gotKind, gotActual, tt.wantKind, tt.wantActual)
		}
	}
}

func TestGetCacheKeyForReference_domainOrganizational(t *testing.T) {
	t.Parallel()
	// After parseReferenceID, getCacheKeyForReference should return the short id for cache lookup
	refID := "domain:organizational:department:DEP-001"
	refKind, actualRefID := parseReferenceID(refID)
	if refKind != "department" || actualRefID != "DEP-001" {
		t.Fatalf("parseReferenceID precondition: got kind=%q actual=%q", refKind, actualRefID)
	}
	cacheKey := getCacheKeyForReference(refID, actualRefID)
	if cacheKey != "DEP-001" {
		t.Errorf("getCacheKeyForReference(%q, %q) = %q, want DEP-001", refID, actualRefID, cacheKey)
	}
}
