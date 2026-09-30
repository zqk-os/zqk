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
		// Compound expressions
		{"standard_checks_pass; path_exists:pkg/vds/predicates.go; query_metric:loc_count<=1000", false},
		// Legacy prose compilable to canonical
		{"standard checks pass", false},
		{"target document file exists and is reachable on disk", false},
		{"title, summary, and path are populated", false},
		{"cryptographic content_hash computed and sealed", false},
		{"content_size measured", false},
		{"title is populated", false},
		{"title, summary are populated", false},
		// Invalid expressions
		{"", true},
		{"unknown_predicate_without_args", true},
		{"field_matches:invalid_regex:[", true},
		{"query_metric:invalid_syntax", true},
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
