package processhygiene

import (
	"fmt"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestHandCASMateDetection verifies that scanning for hand-CAS materialization patterns works.
func TestHandCASMateDetection(t *testing.T) {
	rules, err := LoadEmbeddedDefaultRules()
	if err != nil {
		t.Fatal(err)
	}

	// Verify the expected default rules count is correct
	if len(rules) == 0 {
		t.Fatalf("expected some default rules, got none")
	}

	// Look for hand_cas_materialize rule - it should exist after we add it.
	// Before that, test the concept manually to ensure logic works.
	testPlaceholderOwnerRef(t)
}

// testPlaceholderOwnerRef tests detection with a directly-compiled rule.
func testPlaceholderOwnerRef(t *testing.T) {
	rule, err := compileRule(RuleConfig{
		ID:          "hand_cas_materialize",
		Description: "detected hand-CAS materialization pattern (placeholder ACC-* reference)",
		Match: Match{
			Field:  objects.FieldKeyOwnerRef,
			Prefix: "ACC-901-placeholder", // a known fake pattern
		},
	})
	if err != nil {
		t.Fatalf("compile test rule: %v", err)
	}

	hcObj := map[string]any{
		objects.FieldKeyID:       "BLI-DUMMY-TEST",
		objects.FieldKeyKind:     "decision",
		objects.FieldKeyTitle:    "Test decision",
		objects.FieldKeyOwnerRef: "ACC-901-placeholder", // fake placeholder
	}
	detail, matched := rule.Evaluate(hcObj)
	if !matched {
		t.Fatalf("rule should match ACC-901-placeholder owner_ref")
	}
	if detail == "" {
		t.Fatalf("expected non-empty detail, got empty")
	}

	// Test that a real owner_ref does not match
	realObj := map[string]any{
		objects.FieldKeyOwnerRef: "ACC-1785920548450214012-68b850c0", // real pattern
	}
	detail2, matched2 := rule.Evaluate(realObj)
	if matched2 {
		t.Fatalf("real owner_ref should not match fake placeholder")
	}
	_ = detail2
}

// TestDuplicateIDDetection verifies duplicate-id detection across an object list.
func TestDuplicateIDDetection(t *testing.T) {
	// Test objects with duplicate IDs (simulating the case where someone
	// creates BLI-626 as a backlog_item, then as a requirement).
	objs := []map[string]any{
		{
			objects.FieldKeyID:    "BLI-626",
			objects.FieldKeyKind:  "backlog_item",
			objects.FieldKeyTitle: "Test item 1",
		},
		{
			objects.FieldKeyID:    "BLI-626",
			objects.FieldKeyKind:  "requirement",
			objects.FieldKeyTitle: "Test item 2",
		},
		{
			objects.FieldKeyID:    "GOAL-OSS-CORE-999",
			objects.FieldKeyKind:  "goal",
			objects.FieldKeyTitle: "Unique goal",
		},
	}

	dupFindings := ScanObjectsForDupIDs(objs)
	if len(dupFindings) == 0 {
		t.Fatalf("expected duplicate finding for BLI-626, got none")
	}

	// Verify the first finding corresponds to the duplicated ID
	gotID := dupFindings[0].ID
	if gotID != "BLI-626" {
		t.Fatalf("expected BLI-626, got %q", gotID)
	}

	// Test with unique IDs - no findings expected
	uniqueObjs := []map[string]any{
		{objects.FieldKeyID: "X1", objects.FieldKeyKind: "backlog_item"},
		{objects.FieldKeyID: "Y2", objects.FieldKeyKind: "requirement"},
		{objects.FieldKeyID: "Z3", objects.FieldKeyKind: "goal"},
	}
	dupFindings = ScanObjectsForDupIDs(uniqueObjs)
	if len(dupFindings) != 0 {
		t.Fatalf("expected no duplicates for unique IDs, got %d (findings=%+v)", len(dupFindings), dupFindings)
	}

	// Test with empty list
	dupFindings = ScanObjectsForDupIDs(nil)
	if len(dupFindings) != 0 {
		t.Fatalf("expected no findings for empty input")
	}
}

// TestScanObjectsIntegration verifies the full pipeline with embedded rules.
func TestScanObjectsIntegration(t *testing.T) {
	rules, err := LoadEmbeddedDefaultRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) == 0 {
		t.Fatalf("LoadEmbeddedDefaultRules returned no rules")
	}

	// These embedded rules should work correctly on the test data.
	objs := []map[string]any{
		{
			objects.FieldKeyID:    "BLI-TEST-INT-001",
			objects.FieldKeyKind:  "backlog_item",
			objects.FieldKeyTitle: "Fixture: integration test #1",
		},
	}

	findings := ScanObjects(objs, rules)
	if findings == nil {
		t.Fatal("ScanObjects returned nil")
	}
	// At least the fixture_title_prefix rule should match.
	hasFixtureRule := false
	for _, f := range findings {
		if f.RuleID == "fixture_title_prefix" {
			hasFixtureRule = true
			break
		}
	}
	if !hasFixtureRule {
		t.Fatalf("expected fixture_title_prefix finding, got: %+v", findings)
	}
}

// TestCompileWithRegex verifies the regex match mode works.
func TestCompileWithRegex(t *testing.T) {
	rule, err := compileRule(RuleConfig{
		ID:          "test_regex_rule",
		Description: "test regex compilation",
		Match: Match{
			Field: objects.FieldKeyID,
			Regex: `(FAIL-CLOSED|hand-cas)`,
		},
	})
	if err != nil {
		t.Fatalf("compile with regex: %v", err)
	}

	matchedObj := map[string]any{objects.FieldKeyID: "BLI-626-FAIL-CLOSED"}
	detail, matched := rule.Evaluate(matchedObj)
	if !matched || detail == "" {
		t.Fatalf("should match regex pattern; got matched=%v detail=%q", matched, detail)
	}

	noMatchObj := map[string]any{objects.FieldKeyID: "BLI-626-HARMLESS"}
	detail2, matched2 := rule.Evaluate(noMatchObj)
	if matched2 {
		t.Fatalf("should not match harmless value")
	}
	_ = detail2
}

// TestScanObjectsForDupIDsEdgeCases tests edge cases for duplicate detection.
func TestScanObjectsForDupIDsEdgeCases(t *testing.T) {
	// Single object with nil ID
	noIDObjs := []map[string]any{
		{objects.FieldKeyKind: "backlog_item"}, // missing ID field
	}
	findings := ScanObjectsForDupIDs(noIDObjs)
	if len(findings) != 0 {
		t.Fatalf("expected no findings for objects without proper ID, got: %+v", findings)
	}

	// Single object with empty string ID
	emptyIDObj := []map[string]any{
		{objects.FieldKeyID: "", objects.FieldKeyKind: "backlog_item"},
	}
	findings = ScanObjectsForDupIDs(emptyIDObj)
	if len(findings) != 0 {
		t.Fatalf("expected no findings for empty ID")
	}

	// Triple duplicate (should still produce one finding)
	triple := []map[string]any{
		{objects.FieldKeyID: "DUPX", objects.FieldKeyKind: "backlog_item"},
		{objects.FieldKeyID: "DUPX", objects.FieldKeyKind: "requirement"},
		{objects.FieldKeyID: "DUPX", objects.FieldKeyKind: "goal"},
	}
	findings = ScanObjectsForDupIDs(triple)
	if len(findings) != 1 {
		t.Fatalf("expected exactly one finding for triple duplicate, got %d", len(findings))
	}
	if findings[0].ID != "DUPX" {
		t.Fatalf("unexpected ID in triple dup: %q", findings[0].ID)
	}
}

// TestRuleConfigDescription ensures descriptions are non-empty for all rules.
func TestRuleConfigDescription(t *testing.T) {
	rules, err := LoadEmbeddedDefaultRules()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rules {
		desc := r.Description()
		if desc == "" {
			t.Fatalf("rule %q has empty description", r.ID())
		}
	}
}

// TestEvaluateReturnsErrorStrings verifies that matched findings produce error-level diagnostics.
func TestFindingsAreActionable(t *testing.T) {
	rule, err := compileRule(RuleConfig{
		ID:          "test_actionable_rule",
		Description: "produces actionable diagnostic output",
		Match: Match{
			Field:  objects.FieldKeyOwnerRef,
			Prefix: "FAKE-OWNER-",
		},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	obj := map[string]any{
		objects.FieldKeyID:       "BLI-ACTION-TEST",
		objects.FieldKeyKind:     "backlog_item",
		objects.FieldKeyOwnerRef: "FAKE-OWNER-ref-placeholder",
	}
	detail, matched := rule.Evaluate(obj)
	if !matched {
		t.Fatalf("should match FAKE-OWNER- prefix")
	}

	// The detail should be something an operator can act on.
	expectedDetail := fmt.Sprintf("%s has prefix %q", objects.FieldKeyOwnerRef, "FAKE-OWNER-")
	if detail != expectedDetail {
		t.Fatalf("got detail=%q, want %q", detail, expectedDetail)
	}
}

// TestDuplicateIDFindingsContainRuleIDAndInfo verifies that duplicate findings have proper structure.
func TestDuplicateIDFindingsContainRuleIDAndInfo(t *testing.T) {
	objs := []map[string]any{
		{objects.FieldKeyID: "DUP1", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "item 1"},
		{objects.FieldKeyID: "DUP1", objects.FieldKeyKind: "requirement", objects.FieldKeyTitle: "item 2"},
	}

	findings := ScanObjectsForDupIDs(objs)
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %d", len(findings))
	}

	f := findings[0]
	if f.RuleID != "duplicate_id_orphan" {
		t.Fatalf("expected RuleID 'duplicate_id_orphan', got %q", f.RuleID)
	}
	if f.ID != "DUP1" {
		t.Fatalf("expected ID 'DUP1', got %q", f.ID)
	}
	if len(f.Detail) == 0 {
		t.Fatalf("finding should have non-empty detail with count info")
	}
}

// TestEmptyAndNilRuleConfigs verifies compileRule rejects bad input.
func TestRejectsBadInput(t *testing.T) {
	_, err := compileRule(RuleConfig{ID: "bad1", Match: Match{}})
	if err == nil {
		t.Fatal("expected error for config with no match type")
	}

	_, err = compileRule(RuleConfig{ID: "bad2"})
	if err == nil {
		t.Fatal("expected error for config with no match")
	}
}
