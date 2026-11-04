// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package predicate

import (
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestValidatePredicateSyntax_EdgeCasesAndIdentifiers(t *testing.T) {
	validCases := []string{
		"object_exists:REQ-1234",
		"object_exists:BLI-5678",
		"field_nonempty:title",
		"field_nonempty:REQ-1234:title",
		"field_cleared:priority_plan_ref",
		"field_cleared:OBJ-999:error_state",
		"role_is:developer",
		"active_ref:milestone_ref",
		"content_size_positive:pkg/path/test.go",
		"command_exit_code:go test -v",
		"title_body_cohesion:5",
		"title_body_cohesion:0",
		"criteria_linked_or_acceptance_present:REQ-1",
		"at_least:3:milestone_refs",
		"ast_semantic_match:pkg/storage:symbol_present:Cache",
		"ast_semantic_match:pkg/graph:symbol_absent:OldGraph",
		"ast_semantic_match:pkg/engine:type_implements:Executor",
		"ast_semantic_match:pkg/labels:no_raw_panics",
		"ast_semantic_match:pkg/sub/dir:no_raw_panics",
	}

	for _, expr := range validCases {
		if err := ValidatePredicateSyntax(expr); err != nil {
			t.Fatalf("expected valid syntax for %q, got error: %v", expr, err)
		}
	}
}

func TestValidatePredicateSyntax_InvalidCases(t *testing.T) {
	invalidCases := []string{
		"object_exists:",
		"object_exists:bad!id",
		"field_nonempty:",
		"field_nonempty:bad field!",
		"field_nonempty::title",
		"field_nonempty:REQ-1:",
		"field_nonempty:A:B:C",
		"field_cleared:",
		"field_cleared:bad/field",
		"field_cleared::field",
		"field_cleared:OBJ-1:",
		"field_cleared:A:B:C",
		"role_is:bad*role",
		"active_ref:bad!ref",
		"content_size_positive:",
		"content_size_positive:bad!path?",
		"title_body_cohesion:notanumber",
		"at_least:5",
		"at_least:five:milestone_refs",
		"at_least:-2:milestone_refs",
		"at_least:1:",
		"at_least:1:bad*field",
		"ast_semantic_match:onlyone",
		"ast_semantic_match:pkg/storage:unknown_rule:Cache",
		"unknown_custom_pred:arg",
		"no_argument_pred_without_colon",
	}

	for _, expr := range invalidCases {
		if err := ValidatePredicateSyntax(expr); err == nil {
			t.Fatalf("expected syntax error for %q, got nil", expr)
		}
	}
}

func TestValidateLinkAndMatchPredicates(t *testing.T) {
	validCases := []string{
		"link_back:milestone_refs:goal_refs",
		"field_matches:status:^active$",
		"field_matches:code:[0-9]+",
		"path_exists:pkg/vds/predicates.go",
		"content_hash_matches:pkg/vds/predicates.go",
	}
	for _, expr := range validCases {
		if err := ValidatePredicateSyntax(expr); err != nil {
			t.Fatalf("expected valid syntax for %q, got error: %v", expr, err)
		}
	}

	invalidCases := []string{
		"link_back:onlyone",
		"link_back:a:b:c",
		"link_back::target",
		"link_back:subject:",
		"link_back:bad*sub:target",
		"link_back:sub:bad*target",
		"field_matches:status",
		"field_matches::regex",
		"field_matches:field:",
		"field_matches:field:[a-z(",
	}
	for _, expr := range invalidCases {
		if err := ValidatePredicateSyntax(expr); err == nil {
			t.Fatalf("expected error for %q, got nil", expr)
		}
	}
}

func TestValidateAnyNonemptyPredicate(t *testing.T) {
	valid := []string{
		"any_nonempty:field_a",
		"any_nonempty:field_a,field_b",
		"any_nonempty:field_a:field_b",
		"any_nonempty:field_a,field_b:field_c",
	}
	for _, expr := range valid {
		if err := ValidatePredicateSyntax(expr); err != nil {
			t.Fatalf("expected valid for %q, got %v", expr, err)
		}
	}

	invalid := []string{
		"any_nonempty:",
		"any_nonempty:,,,",
		"any_nonempty:field_a,bad!field",
	}
	for _, expr := range invalid {
		if err := ValidatePredicateSyntax(expr); err == nil {
			t.Fatalf("expected error for %q, got nil", expr)
		}
	}
}

func TestSplitPredicates_QuotingAndDelimiters(t *testing.T) {
	t.Run("quotes and semicolons", func(t *testing.T) {
		expr := `field_matches:summary:'first;second'; standard_checks_pass; "quoted;semi"`
		preds, err := SplitPredicates(expr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(preds) != 3 {
			t.Fatalf("expected 3 predicates, got %d: %v", len(preds), preds)
		}
	})

	t.Run("quotes and commas", func(t *testing.T) {
		expr := "`backtick,comma`, standard_checks_pass, \"double,comma\""
		preds, err := SplitPredicates(expr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(preds) != 3 {
			t.Fatalf("expected 3 predicates, got %d: %v", len(preds), preds)
		}
	})

	t.Run("any_nonempty preserved comma", func(t *testing.T) {
		expr := "any_nonempty:workstream_ref,milestone_ref; standard_checks_pass"
		preds, err := SplitPredicates(expr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(preds) != 2 {
			t.Fatalf("expected 2 predicates, got %d: %v", len(preds), preds)
		}
		if preds[0] != "any_nonempty:workstream_ref,milestone_ref" {
			t.Fatalf("unexpected first predicate: %q", preds[0])
		}
	})

	t.Run("empty and delimiters only", func(t *testing.T) {
		if _, err := SplitPredicates(""); err == nil {
			t.Fatal("expected error on empty string")
		}
		if _, err := SplitPredicates("   "); err == nil {
			t.Fatal("expected error on whitespace")
		}
		if _, err := SplitPredicates(" ; ; , , "); err == nil {
			t.Fatal("expected error on delimiters only")
		}
	})
}

func TestParseMetricPredicate_AllOperators(t *testing.T) {
	cases := []struct {
		arg     string
		wantN   string
		wantOp  string
		wantVal float64
	}{
		{"coverage>=80", "coverage", ">=", 80.0},
		{"loc_count<=5000.5", "loc_count", "<=", 5000.5},
		{"errors==0", "errors", "==", 0.0},
		{"latency!=10.25", "latency", "!=", 10.25},
		{"warnings>0", "warnings", ">", 0.0},
		{"score<-5.5", "score", "<", -5.5},
	}

	for _, tc := range cases {
		name, op, val, err := ParseMetricPredicate(tc.arg)
		if err != nil {
			t.Fatalf("ParseMetricPredicate(%q) returned error: %v", tc.arg, err)
		}
		if name != tc.wantN || op != tc.wantOp || math.Abs(val-tc.wantVal) > 0.0001 {
			t.Fatalf("ParseMetricPredicate(%q) = (%q, %q, %f), want (%q, %q, %f)",
				tc.arg, name, op, val, tc.wantN, tc.wantOp, tc.wantVal)
		}
	}

	invalidCases := []string{
		"invalid_metric",
		"coverage>>80",
		"loc_count<=abc",
		"",
	}
	for _, arg := range invalidCases {
		if _, _, _, err := ParseMetricPredicate(arg); err == nil {
			t.Fatalf("expected error for ParseMetricPredicate(%q), got nil", arg)
		}
	}
}

func TestCompilePrecondition_DocAndFieldBranches(t *testing.T) {
	tests := []struct {
		input     string
		wantCanon string
		wantOk    bool
	}{
		{"", "", true},
		{"target file reachable and readable", "path_exists:file_path", true},
		{"cryptographic content_hash matches target file on disk", "content_hash_matches:file_path", true},
		{"document content_size measured", "content_size_positive:content_size", true},
		{"title and summary are populated", "field_nonempty:title;field_nonempty:summary", true},
		{"field1, field2, field3 are populated", "field_nonempty:field1;field_nonempty:field2;field_nonempty:field3", true},
		{"   is populated", "", false},
	}

	for _, tt := range tests {
		canon, ok := CompilePrecondition(tt.input)
		if ok != tt.wantOk {
			t.Fatalf("CompilePrecondition(%q) ok = %v, want %v", tt.input, ok, tt.wantOk)
		}
		if ok && canon != tt.wantCanon {
			t.Fatalf("CompilePrecondition(%q) = %q, want %q", tt.input, canon, tt.wantCanon)
		}
	}
}

func TestCompilePrecondition_RoleAndClearedBranches(t *testing.T) {
	tests := []struct {
		input     string
		wantCanon string
		wantOk    bool
	}{
		{"role is developer", "role_is:developer", true},
		{"role is senior developer", "", false},
		{"owner_ref is cleared", "field_cleared:owner_ref", true},
		{"owner_ref is unset", "field_cleared:owner_ref", true},
		{"clear status", "field_cleared:status", true},
		{"unset priority", "field_cleared:priority", true},
		{"title is set", "field_nonempty:title", true},
		{"summary is not empty", "field_nonempty:summary", true},
		{"multi word field is set", "", false},
		{"owner identified", "field_nonempty:owner_ref", true},
		{"owner is set", "field_nonempty:owner", true},
		{"problem statement must align with acceptance", "field_nonempty:problem_statement;field_nonempty:acceptance_considerations", true},
	}

	for _, tt := range tests {
		canon, ok := CompilePrecondition(tt.input)
		if ok != tt.wantOk {
			t.Fatalf("CompilePrecondition(%q) ok = %v, want %v", tt.input, ok, tt.wantOk)
		}
		if ok && canon != tt.wantCanon {
			t.Fatalf("CompilePrecondition(%q) = %q, want %q", tt.input, canon, tt.wantCanon)
		}
	}
}

func TestCompilePrecondition_StatusAndEvidencePrefixes(t *testing.T) {
	tests := []struct {
		input     string
		wantCanon string
	}{
		{"work is done", GateWorkDone},
		{"tdd red phase", GateTddTestRedPhase},
		{"all linked criteria_refs bound to active test_case_refs", GateTddTestRedPhase},
		{"must link to an active test_case", GateCriteriaActiveTestCase},
		{"commit_refs have git mutation evidence", GateGitMutationEvidencePresent},
		{"branch_ref is an ancestor of trunk", GateBranchIsAncestorOfTrunk},
		{"all linked criteria_refs are validated or complete", GateLinkedCriteriaValidatedOrComplete},
		{"linked priority_plan is archived when priority_plan_ref is set", GatePriorityPlanArchivedWhenSet},
		{"priority_plan_ref target must be in active or in_progress status", GatePriorityPlanExecutionFacing},
	}

	for _, tt := range tests {
		canon, ok := CompilePrecondition(tt.input)
		if !ok || canon != tt.wantCanon {
			t.Fatalf("CompilePrecondition(%q) = (%q, %v), want (%q, true)", tt.input, canon, ok, tt.wantCanon)
		}
	}
}

func TestCompilePrecondition_AllStandaloneGates(t *testing.T) {
	for gate := range StandaloneGatePredicates {
		// Test exact match
		canon, ok := CompilePrecondition(gate)
		if !ok || canon != gate {
			t.Fatalf("expected exact match for %q, got (%q, %v)", gate, canon, ok)
		}

		// Test space-substituted match
		spaced := strings.ReplaceAll(gate, "_", " ")
		canonSpaced, okSpaced := CompilePrecondition(spaced)
		if !okSpaced || canonSpaced != gate {
			t.Fatalf("expected spaced match for %q (%q), got (%q, %v)", gate, spaced, canonSpaced, okSpaced)
		}
	}
}

func TestCompilePrecondition_RefAndDisjunctionBranches(t *testing.T) {
	tests := []struct {
		input     string
		wantCanon string
		wantOk    bool
	}{
		{"milestone_refs link back to goal_refs", "link_back:milestone_refs:goal_refs", true},
		{"milestone_refs links back to goal_refs", "link_back:milestone_refs:goal_refs", true},
		{"milestone_refs belongs to goal_refs", "link_back:milestone_refs:goal_refs", true},
		{"at least milestone_ref", "at_least:1:milestone_ref", true},
		{"at least 5 milestone_refs", "at_least:5:milestone_refs", true},
		{"bad link back to something", "", false},
		{"something belongs to bad", "", false},
	}

	for _, tt := range tests {
		canon, ok := CompilePrecondition(tt.input)
		if ok != tt.wantOk {
			t.Fatalf("CompilePrecondition(%q) ok = %v, want %v", tt.input, ok, tt.wantOk)
		}
		if ok && canon != tt.wantCanon {
			t.Fatalf("CompilePrecondition(%q) = %q, want %q", tt.input, canon, tt.wantCanon)
		}
	}
}

func TestStem_ComprehensiveCoverage(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"a", "a"},
		{"cat", "cat"},
		{"classes", "class"},
		{"parties", "parti"},
		{"grass", "grass"},
		{"tasks", "task"},
		{"exceeded", "exceed"},
		{"jumped", "jump"},
		{"walking", "walk"},
		{"relational", "rel"},
		{"conditional", "condi"},
		{"creation", "cre"},
		{"connection", "connect"},
		{"decision", "decis"},
		{"optimizer", "optim"},
		{"optimiser", "optim"},
		{"nationalism", "nation"},
		{"personality", "person"},
		{"statement", "stat"},
		{"blindness", "blind"},
		{"washable", "wash"},
		{"flexible", "flex"},
		{"activate", "activ"},
		{"combat", "comb"},
		{"static", "stat"},
		{"guide", "guid"},
	}

	for _, tc := range cases {
		got := Stem(tc.input)
		if got != tc.want {
			t.Fatalf("Stem(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestExtractSalientStems_EdgeCases(t *testing.T) {
	t.Run("empty and stop words only", func(t *testing.T) {
		if stems := ExtractSalientStems("", 10); stems != nil {
			t.Fatalf("expected nil stems for empty text, got %v", stems)
		}
		if stems := ExtractSalientStems("the a an and or must shall", 10); stems != nil {
			t.Fatalf("expected nil stems for stop words, got %v", stems)
		}
	})

	t.Run("topK bounds and tie breaking", func(t *testing.T) {
		text := "zebra alpha zebra alpha beta gamma"
		stemsAll := ExtractSalientStems(text, 0)
		if len(stemsAll) != 4 {
			t.Fatalf("expected 4 stems with topK=0, got %d: %v", len(stemsAll), stemsAll)
		}
		if stemsAll[0] != "alpha" && stemsAll[0] != "zebra" {
			t.Fatalf("expected top frequency terms first, got %q", stemsAll[0])
		}

		stemsBounded := ExtractSalientStems(text, 2)
		if len(stemsBounded) != 2 {
			t.Fatalf("expected 2 stems with topK=2, got %d", len(stemsBounded))
		}
	})
}

func TestCalculateTitleBodyOverlap_EdgeCases(t *testing.T) {
	overlap, stems := CalculateTitleBodyOverlap("", "some body content", 10)
	if overlap != 0 || stems != nil {
		t.Fatalf("expected 0 overlap and nil stems on empty title, got (%f, %v)", overlap, stems)
	}

	overlap, stems = CalculateTitleBodyOverlap("Valid Title Stems", "", 10)
	if overlap != 0 || stems != nil {
		t.Fatalf("expected 0 overlap and nil stems on empty body, got (%f, %v)", overlap, stems)
	}

	overlap, stems = CalculateTitleBodyOverlap("Compiler Architecture", "The compiler subsystem defines the core architecture", 10)
	if overlap <= 0 || len(stems) == 0 {
		t.Fatalf("expected positive overlap, got %f with stems: %v", overlap, stems)
	}
}

func TestVerifyTitleBodyCohesion_Branches(t *testing.T) {
	title := "Compiler Engine Parser"
	body := "This subsystem provides an AST compiler, parser engine, and evaluation pipeline."

	// Test default minSharedStems when <= 0
	ok, shared := VerifyTitleBodyCohesion(title, body, 0)
	if !ok || len(shared) == 0 {
		t.Fatalf("expected cohesion ok with default minStems, got (%v, %v)", ok, shared)
	}

	// Test high threshold
	okHigh, _ := VerifyTitleBodyCohesion(title, body, 100)
	if okHigh {
		t.Fatal("expected cohesion failure with threshold 100")
	}
}

func TestConcurrentPredicateEvaluation(t *testing.T) {
	const workers = 8
	const iterations = 50
	var wg sync.WaitGroup
	wg.Add(workers)

	expressions := []string{
		"standard_checks_pass; path_exists:pkg/vds/predicates.go",
		"field_nonempty:title; field_cleared:error_state",
		"query_metric:coverage>=85; query_metric:loc_count<=1000",
		"at_least:2:milestone_refs; active_ref:goal_ref",
	}

	for i := 0; i < workers; i++ {
		workerID := i
		builder := goroutinelabels.NewGoroutine("test_concurrent_predicates", "evaluating predicates across concurrent workers")
		builder.StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				expr := expressions[(workerID+j)%len(expressions)]
				if err := ValidatePredicateSyntax(expr); err != nil {
					t.Errorf("worker %d iteration %d failed: %v", workerID, j, err)
					return
				}
				canon, ok := CompilePrecondition("standard checks pass")
				if !ok || canon != GateStandardChecksPass {
					t.Errorf("worker %d iteration %d compile failed", workerID, j)
					return
				}
				overlap, _ := CalculateTitleBodyOverlap("Predicate Subsystem", "The predicate subsystem validates AST syntax", 10)
				if overlap <= 0 {
					t.Errorf("worker %d iteration %d overlap failed", workerID, j)
					return
				}
			}
		})
	}

	wg.Wait()
}

func TestTruthTableEvaluation_HeterogeneousObjects(t *testing.T) {
	type evalCase struct {
		name       string
		predicate  string
		objectData map[string]any
		expectMet  bool
	}

	cases := []evalCase{
		{
			name:      "field present and nonempty string",
			predicate: "field_nonempty:title",
			objectData: map[string]any{
				"title": "Subsystem Architectural Guide",
			},
			expectMet: true,
		},
		{
			name:      "field absent for field_nonempty",
			predicate: "field_nonempty:title",
			objectData: map[string]any{
				"summary": "Some summary without title",
			},
			expectMet: false,
		},
		{
			name:      "field cleared when nil or empty",
			predicate: "field_cleared:priority_plan_ref",
			objectData: map[string]any{
				"priority_plan_ref": "",
			},
			expectMet: true,
		},
		{
			name:      "field cleared when key absent",
			predicate: "field_cleared:priority_plan_ref",
			objectData: map[string]any{
				"other_field": "val",
			},
			expectMet: true,
		},
		{
			name:      "field cleared fails when set",
			predicate: "field_cleared:priority_plan_ref",
			objectData: map[string]any{
				"priority_plan_ref": "PLAN-1234",
			},
			expectMet: false,
		},
		{
			name:      "metric comparison satisfies floor",
			predicate: "query_metric:coverage>=85",
			objectData: map[string]any{
				"coverage": 92.5,
			},
			expectMet: true,
		},
		{
			name:      "metric comparison violates floor",
			predicate: "query_metric:coverage>=85",
			objectData: map[string]any{
				"coverage": 80.2,
			},
			expectMet: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidatePredicateSyntax(tc.predicate); err != nil {
				t.Fatalf("syntax validation failed: %v", err)
			}
			met := evaluateSimpleObjectPredicate(tc.predicate, tc.objectData)
			if met != tc.expectMet {
				t.Fatalf("predicate %q against data %v: got met=%v, want %v",
					tc.predicate, tc.objectData, met, tc.expectMet)
			}
		})
	}
}

func evaluateSimpleObjectPredicate(pred string, obj map[string]any) bool {
	name, arg, _ := strings.Cut(pred, ":")
	switch name {
	case "field_nonempty":
		val, exists := obj[arg]
		if !exists || val == nil {
			return false
		}
		str, isStr := val.(string)
		return isStr && strings.TrimSpace(str) != ""
	case "field_cleared":
		val, exists := obj[arg]
		if !exists || val == nil {
			return true
		}
		str, isStr := val.(string)
		return isStr && strings.TrimSpace(str) == ""
	case "query_metric":
		metricName, op, threshold, err := ParseMetricPredicate(arg)
		if err != nil {
			return false
		}
		rawVal, exists := obj[metricName]
		if !exists {
			return false
		}
		var num float64
		switch v := rawVal.(type) {
		case float64:
			num = v
		case int:
			num = float64(v)
		default:
			return false
		}
		return evalMetricOp(op, num, threshold)
	default:
		return true
	}
}

func evalMetricOp(op string, val, threshold float64) bool {
	switch op {
	case ">=":
		return val >= threshold
	case "<=":
		return val <= threshold
	case ">":
		return val > threshold
	case "<":
		return val < threshold
	case "==":
		return math.Abs(val-threshold) < 0.0001
	case "!=":
		return math.Abs(val-threshold) >= 0.0001
	default:
		return false
	}
}
