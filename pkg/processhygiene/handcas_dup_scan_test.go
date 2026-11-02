package processhygiene

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

// Static Floor Invariants: ScanObjectsForHandCASMate, ScanAllObjects, sortFindings, and getAnyStringField
// uphold defensive contracts on nil, empty, or degenerate structures without panic.
func TestHandCASDupScan_StaticFloorInvariants(t *testing.T) {
	// Invariant 1: ScanObjectsForHandCASMate on nil or empty input returns empty findings slice without panic.
	if findings := ScanObjectsForHandCASMate(nil); len(findings) != 0 {
		t.Fatalf("expected 0 findings for nil object list, got %d", len(findings))
	}
	if findings := ScanObjectsForHandCASMate([]map[string]any{}); len(findings) != 0 {
		t.Fatalf("expected 0 findings for empty object list, got %d", len(findings))
	}

	// Invariant 2: ScanAllObjects on nil or empty input returns empty findings slice without panic.
	if findings := ScanAllObjects(nil); len(findings) != 0 {
		t.Fatalf("expected 0 findings for nil object list, got %d", len(findings))
	}
	if findings := ScanAllObjects([]map[string]any{}); len(findings) != 0 {
		t.Fatalf("expected 0 findings for empty object list, got %d", len(findings))
	}

	// Invariant 3: sortFindings handles empty and single-element slices safely.
	var emptyFindings []Finding
	sortFindings(emptyFindings)
	if len(emptyFindings) != 0 {
		t.Fatalf("expected empty slice to remain empty")
	}

	single := []Finding{{RuleID: "ruleB", ID: "id1"}}
	sortFindings(single)
	if len(single) != 1 || single[0].ID != "id1" {
		t.Fatalf("unexpected single finding after sort: %+v", single)
	}

	// Invariant 4: getAnyStringField returns empty string for missing keys and converts types cleanly.
	if val := getAnyStringField(nil, objects.FieldKeyOwnerRef); val != "" {
		t.Fatalf("expected empty string for nil map, got %q", val)
	}
	emptyMap := map[string]any{}
	if val := getAnyStringField(emptyMap, objects.FieldKeyOwnerRef); val != "" {
		t.Fatalf("expected empty string for missing key, got %q", val)
	}
	intMap := map[string]any{"num": 42}
	if val := getAnyStringField(intMap, "num"); val != "42" {
		t.Fatalf("expected '42' string representation, got %q", val)
	}
}

// Operational Dynamic Proofs: Verify hand-CAS detection across owner_ref and id,
// ScanAllObjects pipeline integration, and sortFindings multi-key ordering.
func TestHandCASDupScan_OperationalDynamicProofs(t *testing.T) {
	t.Run("placeholder owner_ref prefixes detected", func(t *testing.T) {
		objs := []map[string]any{
			{
				objects.FieldKeyID:       "BLI-TEST-1",
				objects.FieldKeyKind:     "backlog_item",
				objects.FieldKeyOwnerRef: "ACC-901-placeholder-agent",
			},
			{
				objects.FieldKeyID:       "BLI-TEST-2",
				objects.FieldKeyKind:     "backlog_item",
				objects.FieldKeyOwnerRef: "FAKE-OWNER-worker-1",
			},
			{
				objects.FieldKeyID:       "BLI-TEST-3",
				objects.FieldKeyKind:     "backlog_item",
				objects.FieldKeyOwnerRef: "ACC-VALID-OPERATOR",
			},
		}

		findings := ScanObjectsForHandCASMate(objs)
		if len(findings) != 2 {
			t.Fatalf("expected 2 findings for placeholder owner_refs, got %d: %+v", len(findings), findings)
		}

		f1 := findings[0]
		if f1.RuleID != RuleIDHandCASMaterialize || f1.ID != "BLI-TEST-1" || f1.Field != objects.FieldKeyOwnerRef {
			t.Fatalf("unexpected finding 1: %+v", f1)
		}
		if f1.Value != "ACC-901-placeholder-agent" {
			t.Fatalf("unexpected finding value: %q", f1.Value)
		}

		f2 := findings[1]
		if f2.RuleID != RuleIDHandCASMaterialize || f2.ID != "BLI-TEST-2" || f2.Field != objects.FieldKeyOwnerRef {
			t.Fatalf("unexpected finding 2: %+v", f2)
		}
		if f2.Value != "FAKE-OWNER-worker-1" {
			t.Fatalf("unexpected finding value: %q", f2.Value)
		}
	})

	t.Run("placeholder account ID shapes detected", func(t *testing.T) {
		objs := []map[string]any{
			{
				objects.FieldKeyID:   "ACC-901",
				objects.FieldKeyKind: "account",
			},
			{
				objects.FieldKeyID:   "ACC-901/sub-service",
				objects.FieldKeyKind: "account",
			},
		}

		// Test exact match
		findingsExact := ScanObjectsForHandCASMate([]map[string]any{objs[0]})
		if len(findingsExact) != 1 {
			t.Fatalf("expected 1 finding for exact ACC-901, got %d", len(findingsExact))
		}
		if findingsExact[0].Field != objects.FieldKeyID || findingsExact[0].ID != "ACC-901" {
			t.Fatalf("unexpected finding: %+v", findingsExact[0])
		}

		// Test prefix match with slash
		findingsPrefix := ScanObjectsForHandCASMate([]map[string]any{objs[1]})
		if len(findingsPrefix) != 1 {
			t.Fatalf("expected 1 finding for ACC-901/sub-service, got %d", len(findingsPrefix))
		}
		if findingsPrefix[0].Field != objects.FieldKeyID || findingsPrefix[0].ID != "ACC-901/sub-service" {
			t.Fatalf("unexpected finding: %+v", findingsPrefix[0])
		}
	})

	t.Run("sortFindings primary and secondary key ordering", func(t *testing.T) {
		findings := []Finding{
			{RuleID: "rule_z", ID: "BLI-10"},
			{RuleID: "rule_a", ID: "BLI-99"},
			{RuleID: "rule_a", ID: "BLI-01"},
			{RuleID: "rule_m", ID: "BLI-50"},
		}

		sortFindings(findings)

		expected := []struct {
			ruleID string
			id     string
		}{
			{"rule_a", "BLI-01"},
			{"rule_a", "BLI-99"},
			{"rule_m", "BLI-50"},
			{"rule_z", "BLI-10"},
		}

		for i, exp := range expected {
			if findings[i].RuleID != exp.ruleID || findings[i].ID != exp.id {
				t.Fatalf("at index %d: expected {%s, %s}, got {%s, %s}",
					i, exp.ruleID, exp.id, findings[i].RuleID, findings[i].ID)
			}
		}
	})

	t.Run("ScanAllObjects combines duplicate ID and hand-CAS mate checks with sorting", func(t *testing.T) {
		objs := []map[string]any{
			// Duplicate ID
			{
				objects.FieldKeyID:   "BLI-DUP-1",
				objects.FieldKeyKind: "backlog_item",
			},
			{
				objects.FieldKeyID:   "BLI-DUP-1",
				objects.FieldKeyKind: "requirement",
			},
			// Hand-CAS owner ref
			{
				objects.FieldKeyID:       "BLI-HANDCAS-1",
				objects.FieldKeyKind:     "backlog_item",
				objects.FieldKeyOwnerRef: "FAKE-OWNER-99",
			},
			// Normal clean object
			{
				objects.FieldKeyID:       "BLI-CLEAN-1",
				objects.FieldKeyKind:     "backlog_item",
				objects.FieldKeyOwnerRef: "ACC-VALID-99",
			},
		}

		findings := ScanAllObjects(objs)
		if len(findings) != 2 {
			t.Fatalf("expected 2 combined findings, got %d: %+v", len(findings), findings)
		}

		// Deterministic sort: duplicate_id_orphan comes before hand_cas_materialize
		if findings[0].RuleID != RuleIDDuplicateIDOrphan || findings[0].ID != "BLI-DUP-1" {
			t.Fatalf("expected first finding duplicate_id_orphan, got: %+v", findings[0])
		}
		if findings[1].RuleID != RuleIDHandCASMaterialize || findings[1].ID != "BLI-HANDCAS-1" {
			t.Fatalf("expected second finding hand_cas_materialize, got: %+v", findings[1])
		}
	})
}

// Negative Adversarial Boundaries: Verify suppression of invalid objects,
// deduplication guards, and adversarial inputs.
func TestHandCASDupScan_NegativeAdversarialBoundaries(t *testing.T) {
	t.Run("missing or empty id and kind skipped", func(t *testing.T) {
		objs := []map[string]any{
			// Missing ID
			{
				objects.FieldKeyKind:     "backlog_item",
				objects.FieldKeyOwnerRef: "ACC-901-placeholder",
			},
			// Empty ID string
			{
				objects.FieldKeyID:       "",
				objects.FieldKeyKind:     "backlog_item",
				objects.FieldKeyOwnerRef: "ACC-901-placeholder",
			},
			// Missing Kind
			{
				objects.FieldKeyID:       "BLI-NO-KIND",
				objects.FieldKeyOwnerRef: "ACC-901-placeholder",
			},
			// Empty Kind string
			{
				objects.FieldKeyID:       "BLI-EMPTY-KIND",
				objects.FieldKeyKind:     "",
				objects.FieldKeyOwnerRef: "ACC-901-placeholder",
			},
		}

		findings := ScanObjectsForHandCASMate(objs)
		if len(findings) != 0 {
			t.Fatalf("expected 0 findings for objects with empty/missing id or kind, got %d: %+v", len(findings), findings)
		}
	})

	t.Run("placeholderOwnerRefFinding deduplication guard", func(t *testing.T) {
		reported := make(map[string]bool)
		obj := map[string]any{
			objects.FieldKeyOwnerRef: "ACC-901-placeholder-test",
		}

		f1, ok1 := placeholderOwnerRefFinding(obj, "BLI-DEDUP", "backlog_item", reported)
		if !ok1 || f1.RuleID != RuleIDHandCASMaterialize {
			t.Fatalf("expected first report to succeed, got ok=%v, finding=%+v", ok1, f1)
		}

		// Second invocation with same reported map must be deduplicated
		f2, ok2 := placeholderOwnerRefFinding(obj, "BLI-DEDUP", "backlog_item", reported)
		if ok2 {
			t.Fatalf("expected second report to be suppressed by deduplication, got %+v", f2)
		}
	})

	t.Run("placeholderObjectIDFinding deduplication guard", func(t *testing.T) {
		reported := make(map[string]bool)

		f1, ok1 := placeholderObjectIDFinding("ACC-901", "account", reported)
		if !ok1 || f1.RuleID != RuleIDHandCASMaterialize {
			t.Fatalf("expected first report to succeed, got ok=%v, finding=%+v", ok1, f1)
		}

		// Second invocation with same reported map must be deduplicated
		f2, ok2 := placeholderObjectIDFinding("ACC-901/sub", "account", reported)
		if ok2 {
			t.Fatalf("expected second report to be suppressed by deduplication, got %+v", f2)
		}
	})

	t.Run("adversarial IDs resembling placeholder prefixes but not matching", func(t *testing.T) {
		objs := []map[string]any{
			// Near-misses on account ID: ACC-9010 (not ACC-901 or ACC-901/)
			{
				objects.FieldKeyID:   "ACC-9010",
				objects.FieldKeyKind: "account",
			},
			{
				objects.FieldKeyID:   "ACC-90",
				objects.FieldKeyKind: "account",
			},
			// Near-miss on owner_ref
			{
				objects.FieldKeyID:       "BLI-OK-1",
				objects.FieldKeyKind:     "backlog_item",
				objects.FieldKeyOwnerRef: "ACC-901-real-agent", // not ACC-901-placeholder
			},
			{
				objects.FieldKeyID:       "BLI-OK-2",
				objects.FieldKeyKind:     "backlog_item",
				objects.FieldKeyOwnerRef: "NOT-FAKE-OWNER-worker",
			},
		}

		findings := ScanObjectsForHandCASMate(objs)
		if len(findings) != 0 {
			t.Fatalf("expected 0 findings for near-miss IDs and owner refs, got %d: %+v", len(findings), findings)
		}
	})
}
