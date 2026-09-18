package system

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// High-volume kinds are omitted from object-id-cache by design; Exists+miss must not
// emit CacheLag (see checkReferenceAgainstCache / IsHighVolumeKindForCache).
func TestHighVolumeKindsSuppressObjectIDCacheLag(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{objects.KindSchedulerJob, objects.KindAuditEvent} {
		if !storage.IsHighVolumeKindForCache(kind) {
			t.Fatalf("expected %s to be high-volume for cache exclusion", kind)
		}
	}
	if storage.IsHighVolumeKindForCache(objects.KindPriorityPlan) {
		t.Fatal("priority_plan must not be treated as high-volume OID-cache exclusion")
	}
}

func TestShouldSkipReferenceField_BranchRef(t *testing.T) {
	t.Parallel()
	if !shouldSkipReferenceField(objects.KindPriorityPlan, objects.FieldKeyBranchRef) {
		t.Fatal("branch_ref is a git branch string; check must not treat it as an object ID")
	}
	if !shouldSkipReferenceField(objects.KindBacklogItem, objects.FieldKeyBranchRef) {
		t.Fatal("backlog_item.branch_ref is also a git branch string")
	}
	if shouldSkipReferenceField(objects.KindPriorityPlan, objects.FieldKeyPriorityPlanRef) {
		t.Fatal("real object pointers must still be checked")
	}
}

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
