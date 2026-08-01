package processhygiene

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestLoadEmbeddedDefaultRules(t *testing.T) {
	rules, err := LoadEmbeddedDefaultRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) < 3 {
		t.Fatalf("expected default rules, got %d", len(rules))
	}
}

func TestScanObjects(t *testing.T) {
	rules, err := LoadEmbeddedDefaultRules()
	if err != nil {
		t.Fatal(err)
	}
	objs := []map[string]any{
		{objects.FieldKeyID: "X", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Fixture: backlog_item #1"},
		{objects.FieldKeyID: "Y", objects.FieldKeyKind: "account", objects.FieldKeyTitle: "Real account title"},
	}
	findings := ScanObjects(objs, rules)
	if len(findings) != 1 {
		t.Fatalf("want 1 finding, got %d (%+v)", len(findings), findings)
	}
	if findings[0].RuleID != "fixture_title_prefix" {
		t.Fatalf("got %q", findings[0].RuleID)
	}
}

func TestCountByRule(t *testing.T) {
	m := CountByRule([]Finding{
		{RuleID: "a"},
		{RuleID: "a"},
		{RuleID: "b"},
	})
	if m["a"] != 2 || m["b"] != 1 {
		t.Fatalf("%v", m)
	}
}

func TestCompileRuleInvalid(t *testing.T) {
	_, err := compileRule(RuleConfig{ID: "x", Match: Match{Field: "title"}})
	if err == nil {
		t.Fatal("expected error")
	}
}
