package predicate

import (
	"testing"
)

func TestValidatePredicateSyntax(t *testing.T) {
	tests := []struct {
		expr    string
		wantErr bool
	}{
		{"object_exists:REQ-1234", false},
		{"field_nonempty:title", false},
		{"field_nonempty:REQ-1234:title", false},
		{"field_matches:summary:.*[0-9]+.*", false},
		{"path_exists:pkg/vds/predicates.go", false},
		{"content_hash_matches:pkg/vds/predicates.go", false},
		{"content_size_positive:pkg/vds/predicates.go", false},
		{"query_metric:coverage>=80", false},
		{"query_metric:loc_count<=5000.5", false},
		{"command_exit_code:go test -v ./...", false},
		{"ast_semantic_match:pkg/storage:symbol_present:ResourceCache", false},
		{"ast_semantic_match:pkg/goroutinelabels:no_raw_panics", false},
		{"ast_semantic_match:pkg/graph:symbol_absent:LegacySymbol", false},
		{"tests_ok_per_customization", false},
		{"lint_ok_per_customization", false},
		{"standard_checks_pass", false},
		{"path_exists", false},
		{"content_hash_matches", false},
		{"content_size_positive", false},
		{"role_is:shovel_ready", false},
		{"role_is:realign", false},
		{"field_cleared:priority_plan_ref", false},
		// Compound expressions
		{"standard_checks_pass; path_exists:pkg/vds/predicates.go; query_metric:loc_count<=1000", false},
		{"role_is:shovel_ready; field_cleared:priority_plan_ref", false},
		// Legacy prose compilable to canonical
		{"standard checks pass", false},
		{"target document file exists and is reachable on disk", false},
		{"title, summary, and path are populated", false},
		{"cryptographic content_hash computed and sealed", false},
		{"content_size measured", false},
		{"title is populated", false},
		{"title, summary are populated", false},
		{"role is shovel_ready", false},
		{"role is realign", false},
		{"priority_plan_ref is cleared", false},
		{"clear priority_plan_ref", false},
		{"owner identified", false},
		{"priority assigned", false},
		{"shovel_ready", false},
		{"tdd_test_red_phase", false},
		{"criteria_active_test_case", false},
		{"ready_backlog_references_plan", false},
		{"linked_backlog_ready_or_later", false},
		{"linked_backlog_all_terminal", false},
		{"no_linked_backlog_in_progress_or_complete", false},
		{"workflow_constraints_if_set", false},
		{"priority_plan_validated", false},
		{"team_or_persona_dispatch_refs", false},
		{"git_mutation_evidence_present", false},
		{"branch_is_ancestor_of_trunk", false},
		{"machine_checkable_closure_evidence", false},
		{"work_done", false},
		{"active_ref:milestone_refs", false},
		{"link_back:milestone_refs:goal_refs", false},
		{"at_least:1:milestone_refs", false},
		{"at_least:0:milestone_refs", false},
		{"any_nonempty:workstream_ref,milestone_ref", false},
		{"any_nonempty:workstream_ref:milestone_ref", false},
		// Invalid expressions
		{"", true},
		{"unknown_predicate_without_args", true},
		{"field_matches:invalid_regex:[", true},
		{"query_metric:invalid_syntax", true},
		{"link_back:onlyone", true},
		{"at_least:invalid:milestone_refs", true},
		{"at_least:-1:milestone_refs", true},
		{"at_least:1:", true},
		{"any_nonempty:", true},
		{"any_nonempty:invalid field!", true},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			err := ValidatePredicateSyntax(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePredicateSyntax(%q) error = %v, wantErr %v", tt.expr, err, tt.wantErr)
			}
		})
	}
}

func TestCompilePrecondition(t *testing.T) {
	tests := []struct {
		input     string
		wantCanon string
		wantOk    bool
	}{
		{"standard checks pass", "standard_checks_pass", true},
		{"target document file exists and is reachable on disk", "path_exists:file_path", true},
		{"title, summary, and path are populated", "field_nonempty:title;field_nonempty:summary;field_nonempty:path", true},
		{"cryptographic content_hash computed and sealed", "field_nonempty:content_hash", true},
		{"content_size measured", "content_size_positive:content_size", true},
		{"name is populated", "field_nonempty:name", true},
		{"role is shovel_ready", "role_is:shovel_ready", true},
		{"priority_plan_ref is cleared", "field_cleared:priority_plan_ref", true},
		{"clear priority_plan_ref", "field_cleared:priority_plan_ref", true},
		{"active_order is unset", "field_cleared:active_order", true},
		{"unset active_order", "field_cleared:active_order", true},
		{"owner identified", "field_nonempty:owner_ref", true},
		{"priority assigned", "field_matches:priority:^(high|medium|low)$", true},
		{"work_done", "work_done", true},
		{"shovel ready", "shovel_ready", true},
		{"all linked criteria_refs bound to active test_case_refs (tdd red phase)", "tdd_test_red_phase", true},
		{"at least one active test_case_ref linked", "criteria_active_test_case", true},
		{"at least one ready backlog_item references this plan via priority_plan_ref", "ready_backlog_references_plan", true},
		{"all linked backlog_items referencing this plan are ready or later", "linked_backlog_ready_or_later", true},
		{"all linked backlog_items referencing this plan are terminal", "linked_backlog_all_terminal", true},
		{"no linked backlog_items referencing this plan are in progress or complete", "no_linked_backlog_in_progress_or_complete", true},
		{"workflow constraints validated (if workflow_ref is set)", "workflow_constraints_if_set", true},
		{"priority plan validated", "priority_plan_validated", true},
		{"at least one team_configuration_ref or persona_refs", "team_or_persona_dispatch_refs", true},
		{"commit_hashes have git mutation evidence for this backlog_item", "git_mutation_evidence_present", true},
		{"branch_name is an ancestor of trunk", "branch_is_ancestor_of_trunk", true},
		{"machine-checkable evidence with green scheduler fingerprint is verified", "machine_checkable_closure_evidence", true},
		{"at least one active milestone_ref linked", "active_ref:milestone_ref", true},
		{"at least one milestone_ref linked", "at_least:1:milestone_ref", true},
		{"at least one workstream_ref or milestone_ref linked", "any_nonempty:workstream_ref,milestone_ref", true},
		{"at least one owner_ref or author_ref", "any_nonempty:owner_ref,author_ref", true},
		{"at least 2 milestone_refs", "at_least:2:milestone_refs", true},
		{"milestone_refs must link back to goal_refs", "link_back:milestone_refs:goal_refs", true},
		{"unrecognized arbitrary condition", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			canon, ok := CompilePrecondition(tt.input)
			if ok != tt.wantOk {
				t.Fatalf("CompilePrecondition(%q) ok = %v, want %v", tt.input, ok, tt.wantOk)
			}
			if ok && canon != tt.wantCanon {
				t.Fatalf("CompilePrecondition(%q) = %q, want %q", tt.input, canon, tt.wantCanon)
			}
		})
	}
}
