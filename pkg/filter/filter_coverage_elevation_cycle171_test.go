// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package filter

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestParseFilterString_TypesAndLiterals(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantField string
		wantValue any
	}{
		{
			name:      "integer value",
			input:     "count=42",
			wantField: "count",
			wantValue: 42,
		},
		{
			name:      "negative integer value",
			input:     "offset=-15",
			wantField: "offset",
			wantValue: -15,
		},
		{
			name:      "floating point value",
			input:     "threshold=3.14",
			wantField: "threshold",
			wantValue: 3.14,
		},
		{
			name:      "boolean true",
			input:     "active=true",
			wantField: "active",
			wantValue: true,
		},
		{
			name:      "boolean false",
			input:     "archived=false",
			wantField: "archived",
			wantValue: false,
		},
		{
			name:      "null value",
			input:     "parent=null",
			wantField: "parent",
			wantValue: nil,
		},
		{
			name:      "YAML map structure",
			input:     `meta={"tier":"production"}`,
			wantField: "meta",
			wantValue: map[string]any{"tier": "production"},
		},
		{
			name:      "YAML list structure",
			input:     `tags=["backend","vds"]`,
			wantField: "tags",
			wantValue: []any{"backend", "vds"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, value, err := ParseFilterString(tt.input)
			if err != nil {
				t.Fatalf("unexpected error parsing %q: %v", tt.input, err)
			}
			if field != tt.wantField {
				t.Errorf("field = %q, want %q", field, tt.wantField)
			}
			if !reflect.DeepEqual(value, tt.wantValue) {
				t.Errorf("value = %#v, want %#v", value, tt.wantValue)
			}
		})
	}
}

func TestParseFilterString_DelimitersAndFallbacks(t *testing.T) {
	t.Run("colon operator with spaces and quotes", func(t *testing.T) {
		field, value, err := ParseFilterString(`category : "system design"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if field != "category" || value != "system design" {
			t.Errorf("got field=%q, val=%#v", field, value)
		}
	})

	t.Run("colon operator does not split pipe values", func(t *testing.T) {
		field, value, err := ParseFilterString(`status:active|pending`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if field != "status" || value != "active|pending" {
			t.Errorf("got field=%q, val=%#v", field, value)
		}
	})

	t.Run("invalid YAML syntax falls back to raw string", func(t *testing.T) {
		field, value, err := ParseFilterString(`query=[unclosed`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if field != "query" || value != "[unclosed" {
			t.Errorf("got field=%q, val=%#v; expected raw string fallback", field, value)
		}
	})

	t.Run("pipe separated with empty elements trimmed", func(t *testing.T) {
		field, value, err := ParseFilterString(`status="active||in_progress|"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]any{"$in": []string{"active", "in_progress"}}
		if field != "status" || !reflect.DeepEqual(value, want) {
			t.Errorf("got field=%q, val=%#v, want %#v", field, value, want)
		}
	})

	t.Run("not equal pipe separated with empty elements trimmed", func(t *testing.T) {
		field, value, err := ParseFilterString(`status!="closed||cancelled|"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]any{"$nin": []string{"closed", "cancelled"}}
		if field != "status" || !reflect.DeepEqual(value, want) {
			t.Errorf("got field=%q, val=%#v, want %#v", field, value, want)
		}
	})
}

func TestParseFilterString_MissingFieldNameErrors(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantSubstr string
	}{
		{
			name:       "missing field before colon",
			input:      ":active",
			wantSubstr: "missing field name before : operator",
		},
		{
			name:       "missing field before double equal",
			input:      "==active",
			wantSubstr: "missing field name before == operator",
		},
		{
			name:       "whitespace field before equal",
			input:      "  =active",
			wantSubstr: "missing field name before = operator",
		},
		{
			name:       "whitespace field before not-equal",
			input:      "  !=active",
			wantSubstr: "missing field name before != operator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ParseFilterString(tt.input)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tt.input)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("error %q does not contain expected substring %q", err.Error(), tt.wantSubstr)
			}
		})
	}
}

func TestResolveTimeTokens_ExtendedUnits(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name       string
		token      string
		expectPast time.Duration
		isCalendar bool
	}{
		{name: "last 30 seconds", token: "last 30 seconds", expectPast: 30 * time.Second},
		{name: "last 1 second", token: "last 1 second", expectPast: 1 * time.Second},
		{name: "last 45 minutes", token: "last 45 minutes", expectPast: 45 * time.Minute},
		{name: "last 1 minute", token: "last 1 minute", expectPast: 1 * time.Minute},
		{name: "last 5 hours", token: "last 5 hours", expectPast: 5 * time.Hour},
		{name: "last 1 hour", token: "last 1 hour", expectPast: 1 * time.Hour},
		{name: "last 10 days", token: "last 10 days", isCalendar: true},
		{name: "last 1 day", token: "last 1 day", isCalendar: true},
		{name: "last 3 months", token: "last 3 months", isCalendar: true},
		{name: "last 1 month", token: "last 1 month", isCalendar: true},
		{name: "last 2 years", token: "last 2 years", isCalendar: true},
		{name: "last 1 year", token: "last 1 year", isCalendar: true},
		{name: "last_month singular", token: "last_month", isCalendar: true},
		{name: "last_year singular", token: "last_year", isCalendar: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved := ResolveTimeTokens(tt.token)
			resStr, ok := resolved.(string)
			if !ok {
				t.Fatalf("expected resolved string, got: %T (%#v)", resolved, resolved)
			}
			parsedTime, err := time.Parse(time.RFC3339, resStr)
			if err != nil {
				t.Fatalf("invalid RFC3339 output %q: %v", resStr, err)
			}
			if parsedTime.After(now.Add(time.Second)) {
				t.Errorf("resolved time %v is unexpectedly in future relative to %v", parsedTime, now)
			}
		})
	}
}

func TestResolveTimeTokens_EdgeAndOverflow(t *testing.T) {
	t.Run("number overflow falls back to original string", func(t *testing.T) {
		overflow := "last 999999999999999999999999999999 hours"
		res := ResolveTimeTokens(overflow)
		if res != overflow {
			t.Errorf("expected overflow string unchanged, got %v", res)
		}
	})

	t.Run("unrecognized relative words remain unchanged", func(t *testing.T) {
		tokens := []string{"tomorrow", "next week", "later", "unknown_token"}
		for _, tok := range tokens {
			if res := ResolveTimeTokens(tok); res != tok {
				t.Errorf("expected %q unchanged, got %v", tok, res)
			}
		}
	})

	t.Run("nested structures resolved recursively", func(t *testing.T) {
		input := map[string]any{
			"created": "now",
			"history": []any{
				map[string]any{"at": "yesterday"},
				"today",
				42,
			},
			"count": 100,
		}
		res, ok := ResolveTimeTokens(input).(map[string]any)
		if !ok {
			t.Fatalf("expected map[string]any result")
		}

		createdStr, ok := res["created"].(string)
		if !ok || createdStr == "now" {
			t.Errorf("expected created to be resolved, got %v", res["created"])
		}

		histList, ok := res["history"].([]any)
		if !ok || len(histList) != 3 {
			t.Fatalf("expected history list of 3 items")
		}

		innerMap, ok := histList[0].(map[string]any)
		if !ok {
			t.Fatalf("expected inner map")
		}
		if innerMap["at"] == "yesterday" {
			t.Errorf("expected inner map 'at' to be resolved")
		}
		if histList[1] == "today" {
			t.Errorf("expected histList[1] to be resolved")
		}
		if histList[2] != 42 {
			t.Errorf("expected 42 unchanged, got %v", histList[2])
		}
	})

	t.Run("integration through ParseFilterString", func(t *testing.T) {
		field, value, err := ParseFilterString(`created_at="now"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if field != "created_at" {
			t.Errorf("field = %q, want created_at", field)
		}
		timeStr, ok := value.(string)
		if !ok {
			t.Fatalf("expected string value, got %T", value)
		}
		if _, err := time.Parse(time.RFC3339, timeStr); err != nil {
			t.Errorf("expected RFC3339 timestamp, got %q: %v", timeStr, err)
		}
	})
}

func TestValidateFilterFields_UniversalAndEdgeCases(t *testing.T) {
	t.Run("empty kind or empty filters returns nil", func(t *testing.T) {
		if err := ValidateFilterFields("", map[string]any{"status": "active"}); err != nil {
			t.Errorf("expected nil for empty kind, got: %v", err)
		}
		if err := ValidateFilterFields("backlog_item", nil); err != nil {
			t.Errorf("expected nil for nil filters, got: %v", err)
		}
		if err := ValidateFilterFields("backlog_item", map[string]any{}); err != nil {
			t.Errorf("expected nil for empty filters, got: %v", err)
		}
	})

	t.Run("nonexistent kind returns nil gracefully", func(t *testing.T) {
		err := ValidateFilterFields("completely_unknown_unregistered_kind_xyz", map[string]any{"status": "active"})
		if err != nil {
			t.Errorf("expected nil for unregistered kind, got: %v", err)
		}
	})

	t.Run("skip mongo operator prefixes", func(t *testing.T) {
		filters := map[string]any{
			"$or":  []any{map[string]any{"status": "active"}},
			"$and": []any{map[string]any{"status": "in_progress"}},
		}
		if err := ValidateFilterFields("backlog_item", filters); err != nil {
			t.Errorf("expected mongo operators to be skipped, got: %v", err)
		}
	})

	t.Run("universal dynamic fields are recognized as valid", func(t *testing.T) {
		universalFields := []string{
			"id", "kind", "title", "description", "status", "created_at", "created_by",
			"updated_at", "updated_by", "namespace_id", "schema_version", "tags",
			"related_object_refs", "persona_refs", "milestone_refs", "workstream_refs",
			"meta", "child_rollup",
		}
		filters := make(map[string]any, len(universalFields))
		for _, f := range universalFields {
			filters[f] = "val"
		}
		if err := ValidateFilterFields("backlog_item", filters); err != nil {
			t.Errorf("expected all universal fields to be valid, got: %v", err)
		}
	})

	t.Run("unmatched field without close suggestions returns template guidance", func(t *testing.T) {
		filters := map[string]any{
			"completely_alien_and_unmatched_field_name_xyz": "value",
		}
		err := ValidateFilterFields("backlog_item", filters)
		if err == nil {
			t.Fatal("expected error for alien field, got nil")
		}
		if !strings.Contains(err.Error(), "Check available fields with") {
			t.Errorf("expected 'Check available fields with' message, got: %v", err)
		}
	})
}

func TestSuggestSimilarFields_ScoringAndPruning(t *testing.T) {
	validFields := []string{
		"status", "priority", "priority_tier", "created_at", "updated_at",
		"description", "estimated_effort", "acceptance_criteria",
	}

	t.Run("empty or whitespace input returns nil", func(t *testing.T) {
		if res := SuggestSimilarFields("", validFields); res != nil {
			t.Errorf("expected nil for empty input, got: %v", res)
		}
		if res := SuggestSimilarFields("   ", validFields); res != nil {
			t.Errorf("expected nil for whitespace input, got: %v", res)
		}
	})

	t.Run("exact case insensitive match", func(t *testing.T) {
		res := SuggestSimilarFields("STATUS", validFields)
		if len(res) != 1 || res[0] != "status" {
			t.Errorf("expected ['status'], got: %v", res)
		}
	})

	t.Run("long word distance threshold", func(t *testing.T) {
		// "descripton" missing 'i' (distance 1, len 10)
		res := SuggestSimilarFields("descripton", validFields)
		if len(res) == 0 || res[0] != "description" {
			t.Errorf("expected description suggestion, got: %v", res)
		}
	})

	t.Run("prefix match deduplicated with levenshtein", func(t *testing.T) {
		// "prio" prefixes priority and priority_tier
		res := SuggestSimilarFields("prio", validFields)
		if len(res) == 0 {
			t.Fatalf("expected suggestions for prio")
		}
		foundPriority := false
		for _, s := range res {
			if s == "priority" {
				foundPriority = true
			}
		}
		if !foundPriority {
			t.Errorf("expected 'priority' in suggestions, got: %v", res)
		}
	})
}

func TestComputeLevenshtein_Calculations(t *testing.T) {
	tests := []struct {
		a    string
		b    string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "def", 3},
		{"status", "status", 0},
		{"staus", "status", 1},
		{"kitten", "sitting", 3},
	}

	for _, tt := range tests {
		got := computeLevenshtein(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("computeLevenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestFilterSubsystem_Concurrency(t *testing.T) {
	const workerCount = 16
	var wg sync.WaitGroup
	errCh := make(chan error, workerCount*4)

	for i := range workerCount {
		workerID := i
		wg.Add(1)
		builder := goroutinelabels.NewGoroutine(
			fmt.Sprintf("filter_worker_%d", workerID),
			"testing concurrent filter parsing and time resolution",
		)
		builder.StartSimple(func() {
			defer wg.Done()
			for j := range 20 {
				query := fmt.Sprintf("status_%d=active|in_progress", j)
				field, val, err := ParseFilterString(query)
				if err != nil {
					errCh <- fmt.Errorf("worker %d failed parse %q: %w", workerID, query, err)
					return
				}
				if field != fmt.Sprintf("status_%d", j) || val == nil {
					errCh <- fmt.Errorf("worker %d got unexpected field/val: %s, %#v", workerID, field, val)
					return
				}

				timeQuery := "updated_at=last 2 hours"
				tField, tVal, tErr := ParseFilterString(timeQuery)
				if tErr != nil {
					errCh <- fmt.Errorf("worker %d failed time query: %w", workerID, tErr)
					return
				}
				if tField != "updated_at" || tVal == nil {
					errCh <- fmt.Errorf("worker %d got invalid time field: %s", workerID, tField)
					return
				}

				suggs := SuggestSimilarFields("staus", []string{"status", "priority"})
				if len(suggs) == 0 || suggs[0] != "status" {
					errCh <- fmt.Errorf("worker %d got invalid suggestion: %v", workerID, suggs)
					return
				}
			}
		})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent execution error: %v", err)
	}
}

func TestInternalDelimitedParsers_MissingCut(t *testing.T) {
	t.Run("parseNotEqualFilter without operator", func(t *testing.T) {
		f, v, err := parseNotEqualFilter("no_op_string")
		if err == nil {
			t.Errorf("expected error, got field=%q, val=%#v", f, v)
		}
	})

	t.Run("parseDelimitedFilter without operator", func(t *testing.T) {
		f, v, err := parseDelimitedFilter("no_op_string", "=", true)
		if err == nil {
			t.Errorf("expected error, got field=%q, val=%#v", f, v)
		}
	})

	t.Run("suggestSimilarFields already scored deduplication", func(t *testing.T) {
		// "status" prefixes "statusx" (dist 0), and Levenshtein distance between "status" and "statusx" is 1 (<= 2).
		// This exercises the 'already = true' branch in the Levenshtein pass.
		res := SuggestSimilarFields("status", []string{"statusx"})
		if len(res) == 0 || res[0] != "statusx" {
			t.Errorf("expected ['statusx'], got %v", res)
		}
	})
}
