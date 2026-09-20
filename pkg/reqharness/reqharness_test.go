package reqharness

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func tempDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	return d
}

func sampleRequirements(t *testing.T) []Requirement {
	t.Helper()
	return []Requirement{
		{
			RequirementID: "REQ-001",
			Claim:         "The harness runs the predicate named in test_criteria and records a verdict",
			TestCriteria: []Criterion{
				{ID: "REQ-001-C1", Predicate: "test:TestVerify_passPredicate"},
			},
		},
		{
			RequirementID: "REQ-002",
			Claim:         "Failing predicates fail the requirement and the overall report",
			TestCriteria: []Criterion{
				{ID: "REQ-002-C1", Predicate: "test:TestVerify_failPredicate"},
			},
		},
	}
}

func TestVerify_passPredicate(t *testing.T) {
	if err := RegisterPredicate("test:TestVerify_passPredicate", func(context.Context) error {
		return nil
	}); err != nil {
		t.Fatalf("register pass predicate: %v", err)
	}
	if err := RegisterPredicate("test:TestVerify_failPredicate", func(context.Context) error {
		return errPredFail
	}); err != nil {
		t.Fatalf("register fail predicate: %v", err)
	}

	rep := Verify(context.Background(), sampleRequirements(t))
	if rep.Verdict != "FAIL" {
		t.Fatalf("verdict = %q, want FAIL (REQ-002 predicate fails)", rep.Verdict)
	}
	if len(rep.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(rep.Results))
	}
	if rep.Results[0].Verdict != "PASS" {
		t.Fatalf("req-001 verdict = %q, want PASS", rep.Results[0].Verdict)
	}
	if rep.Results[1].Verdict != "FAIL" {
		t.Fatalf("req-002 verdict = %q, want FAIL", rep.Results[1].Verdict)
	}
	if rep.Results[1].Criteria[0].Skipped {
		t.Fatal("failed criterion should not be skipped")
	}
	if rep.Results[1].Criteria[0].Detail == "" {
		t.Fatal("failed criterion should carry detail from the predicate error")
	}
}

func TestVerify_allPass(t *testing.T) {
	reqs := []Requirement{{
		RequirementID: "REQ-100",
		Claim:         "All-pass requirement",
		TestCriteria: []Criterion{
			{ID: "REQ-100-C1", Predicate: "test:TestVerify_passPredicate"},
		},
	}}
	rep := Verify(context.Background(), reqs)
	if rep.Verdict != "PASS" {
		t.Fatalf("verdict = %q, want PASS", rep.Verdict)
	}
	if len(rep.FailedRequirementIDs) != 0 {
		t.Fatalf("failed ids = %v, want none", rep.FailedRequirementIDs)
	}
	if rep.NextActions == nil {
		t.Fatal("next_actions should be non-nil (empty) on pass")
	}
}

func TestVerify_unknownPredicateFailsWithDetail(t *testing.T) {
	reqs := []Requirement{{
		RequirementID: "REQ-200",
		Claim:         "Unknown predicate",
		TestCriteria: []Criterion{
			{ID: "REQ-200-C1", Predicate: "test:not_registered_anywhere"},
		},
	}}
	rep := Verify(context.Background(), reqs)
	if rep.Verdict != "FAIL" {
		t.Fatalf("verdict = %q, want FAIL", rep.Verdict)
	}
	got := rep.Results[0].Criteria[0]
	if !got.Skipped {
		t.Fatal("unknown predicate should be treated as skipped=not-runnable")
	}
	if got.Detail == "" {
		t.Fatal("unknown predicate should carry detail (registered-name hint)")
	}
	if len(rep.NextActions) == 0 {
		t.Fatal("expected next actions for failed requirement")
	}
}

func TestVerify_duplicatePredicateRejected(t *testing.T) {
	f := func(context.Context) error { return nil }
	if err := RegisterPredicate("test:dup", f); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := RegisterPredicate("test:dup", f); err == nil {
		t.Fatal("second registration must be rejected")
	}
}

func TestVerify_rejectsMalformedRequirements(t *testing.T) {
	rep := Verify(context.Background(), []Requirement{
		{RequirementID: "", Claim: "x"},
		{RequirementID: "REQ-1", Claim: ""},
		{RequirementID: "REQ-2", Claim: "no criteria"},
		{RequirementID: "REQ-3", Claim: "bad criterion shape",
			TestCriteria: []Criterion{{ID: "C", Predicate: "missing:colon:in:name"}}},
	})
	if rep.Verdict != "FAIL" {
		t.Fatalf("verdict = %q, want FAIL for malformed input", rep.Verdict)
	}
	if len(rep.Results) != 4 {
		t.Fatalf("results = %d, want 4 (one per requirement, incl. structural)", len(rep.Results))
	}
}

func TestPredicateName_validations(t *testing.T) {
	ok := []string{"test:Foobar", "cmd:zqk_admin_run", "a:b", "scope:CamelCase_2"}
	for _, n := range ok {
		if err := PredicateName(n); err != nil {
			t.Errorf("PredicateName(%q) = %v, want nil", n, err)
		}
	}
	bad := []string{"", "no_scope", "a:b:", ":b", "a: b", "a b:c", "a:-dash", "a:b..c"}
	for _, n := range bad {
		if err := PredicateName(n); err == nil {
			t.Errorf("PredicateName(%q) = nil, want error", n)
		}
	}
}

func TestLoadAndSaveRequirements_roundTrip(t *testing.T) {
	d := tempDir(t)
	path := filepath.Join(d, "requirements.json")
	in := []Requirement{{
		RequirementID: "REQ-RTR-1",
		Claim:         "Round-trip through JSON preserves criteria order",
		TestCriteria: []Criterion{
			{ID: "REQ-RTR-1-A", Predicate: "test:TestVerify_passPredicate"},
			{ID: "REQ-RTR-1-B", Predicate: "test:TestVerify_passPredicate"},
		},
	}}
	if err := SaveRequirements(path, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := LoadRequirements(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(out) != 1 || len(out[0].TestCriteria) != 2 {
		t.Fatalf("round trip lost data: %+v", out)
	}
	if out[0].TestCriteria[1].ID != "REQ-RTR-1-B" {
		t.Fatalf("criteria order not preserved: %+v", out[0].TestCriteria)
	}
}

func TestLoadRequirements_rejectsEmptyAndMissing(t *testing.T) {
	d := tempDir(t)
	missing := filepath.Join(d, "nope.json")
	if _, err := LoadRequirements(missing); err == nil {
		t.Fatal("missing file must error")
	}
	empty := filepath.Join(d, "empty.json")
	if err := os.WriteFile(empty, []byte("[]"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadRequirements(empty); err == nil {
		t.Fatal("empty requirement list must be rejected")
	}
}

func TestVerify_reportIsJSONStable(t *testing.T) {
	reqs := []Requirement{{
		RequirementID: "REQ-JSON-1",
		Claim:         "Report encodes stably",
		TestCriteria: []Criterion{
			{ID: "REQ-JSON-1-C1", Predicate: "test:TestVerify_passPredicate"},
		},
	}}
	rep := Verify(context.Background(), reqs)
	if rep.Schema != SchemaRequirementsV1 {
		t.Fatalf("schema = %q, want %q", rep.Schema, SchemaRequirementsV1)
	}
	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Report
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Verdict != "PASS" || len(back.FailedRequirementIDs) != 0 {
		t.Fatalf("round trip mismatch: %+v", back)
	}
}
