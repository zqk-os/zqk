// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package aliases

import (
	"fmt"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestResolveStatic_BasicAndBoundaryCases(t *testing.T) {
	t.Parallel()

	tab := map[string]string{
		"define": "create",
		"add":    "create",
		"rm":     "delete",
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "known alias define", input: "define", expected: "create"},
		{name: "known alias add", input: "add", expected: "create"},
		{name: "known alias with whitespace and case", input: "  DEFINE  ", expected: "create"},
		{name: "canonical pass-through uppercase", input: "CREATE", expected: "create"},
		{name: "canonical pass-through mixed case", input: "  Create  ", expected: "create"},
		{name: "unknown term pass-through", input: "modify", expected: "modify"},
		{name: "unknown term with casing and whitespace", input: "  FOO_BAR  ", expected: "foo_bar"},
		{name: "empty string", input: "", expected: ""},
		{name: "whitespace spaces only", input: "   ", expected: ""},
		{name: "whitespace tabs and newlines", input: "\t\r\n ", expected: ""},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ResolveStatic(tc.input, tab)
			if got != tc.expected {
				t.Fatalf("ResolveStatic(%q) = %q; expected %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestResolveStatic_NilAndEmptyTables(t *testing.T) {
	t.Parallel()

	// Nil table must not panic and should normalize input cleanly
	if got := ResolveStatic("  SOMETHING  ", nil); got != "something" {
		t.Fatalf("expected 'something' for nil table, got %q", got)
	}
	if got := ResolveStatic("", nil); got != "" {
		t.Fatalf("expected empty string for nil table, got %q", got)
	}

	// Empty table
	emptyTab := map[string]string{}
	if got := ResolveStatic("  TEST_VAL  ", emptyTab); got != "test_val" {
		t.Fatalf("expected 'test_val' for empty table, got %q", got)
	}
	if got := ResolveStatic("", emptyTab); got != "" {
		t.Fatalf("expected empty string for empty table, got %q", got)
	}
}

func TestResolveStatic_UnicodeAndSpecialCharacters(t *testing.T) {
	t.Parallel()

	tab := map[string]string{
		"über":   "super",
		"straße": "street",
		"café":   "coffee",
	}

	tests := []struct {
		input    string
		expected string
	}{
		{input: "  ÜBER  ", expected: "super"},
		{input: "CAFÉ", expected: "coffee"},
		{input: "  unknown_ü  ", expected: "unknown_ü"},
		{input: "emoji_🚀", expected: "emoji_🚀"},
	}

	for _, tc := range tests {
		got := ResolveStatic(tc.input, tab)
		if got != tc.expected {
			t.Fatalf("ResolveStatic(%q) = %q; expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestDomainIsolation_CrossSubsystemCollisionSafety(t *testing.T) {
	t.Parallel()

	// Domain 1: Generic CRUD operation aliases (e.g. CLI operations)
	crudAliases := map[string]string{
		"modify": "update",
		"change": "update",
		"add":    "create",
	}

	// Domain 2: Spec Field Lifecycle operations (where 'modify' is a canonical immutable verb)
	specFieldAliases := map[string]string{
		"define": "create",
		"add":    "create",
	}

	// In CRUD domain, "modify" maps to "update"
	if got := ResolveStatic("modify", crudAliases); got != "update" {
		t.Fatalf("expected crud 'modify' -> 'update', got %q", got)
	}

	// In Spec Field domain, "modify" MUST stay canonical "modify"
	if got := ResolveStatic("modify", specFieldAliases); got != "modify" {
		t.Fatalf("expected spec field 'modify' -> 'modify', got %q", got)
	}

	// Both share "add" -> "create" without interference
	if got := ResolveStatic("add", crudAliases); got != "create" {
		t.Fatalf("expected crud 'add' -> 'create', got %q", got)
	}
	if got := ResolveStatic("add", specFieldAliases); got != "create" {
		t.Fatalf("expected spec field 'add' -> 'create', got %q", got)
	}
}

func TestNormalizeMap(t *testing.T) {
	t.Parallel()

	// Nil map returns non-nil empty map
	nilNorm := NormalizeMap(nil)
	if nilNorm == nil || len(nilNorm) != 0 {
		t.Fatalf("expected empty non-nil map for nil input, got %#v", nilNorm)
	}

	// Empty map returns empty map
	emptyNorm := NormalizeMap(map[string]string{})
	if len(emptyNorm) != 0 {
		t.Fatalf("expected empty map for empty input, got %#v", emptyNorm)
	}

	// Mixed case keys, whitespace keys, whitespace values, empty keys
	raw := map[string]string{
		"  Define  ": "  create  ",
		"ADD":        "create",
		"   ":        "ignored",
		"":           "also_ignored",
		"UPPER":      "CANONICAL",
	}

	norm := NormalizeMap(raw)
	if len(norm) != 3 {
		t.Fatalf("expected 3 entries in normalized map, got %d (%#v)", len(norm), norm)
	}
	if norm["define"] != "create" {
		t.Fatalf("expected norm['define'] == 'create', got %q", norm["define"])
	}
	if norm["add"] != "create" {
		t.Fatalf("expected norm['add'] == 'create', got %q", norm["add"])
	}
	if norm["upper"] != "CANONICAL" {
		t.Fatalf("expected norm['upper'] == 'CANONICAL', got %q", norm["upper"])
	}
}

func TestTable_MethodsAndNilSafety(t *testing.T) {
	t.Parallel()

	// Nil receiver safety
	var nilTable *Table
	if d := nilTable.Domain(); d != "" {
		t.Fatalf("expected empty domain for nil Table, got %q", d)
	}
	if r := nilTable.Resolve("  HELLO  "); r != "hello" {
		t.Fatalf("expected 'hello' for nil Table Resolve, got %q", r)
	}
	if has := nilTable.HasAlias("define"); has {
		t.Fatalf("expected false for nil Table HasAlias, got true")
	}
	if l := nilTable.Len(); l != 0 {
		t.Fatalf("expected 0 for nil Table Len, got %d", l)
	}
	if m := nilTable.Mappings(); len(m) != 0 {
		t.Fatalf("expected empty map for nil Table Mappings, got %#v", m)
	}

	// Instantiated Table
	tbl := NewTable("  spec_field_ops  ", map[string]string{
		"  Define  ": "create",
		"ADD":        "create",
	})

	if tbl.Domain() != "spec_field_ops" {
		t.Fatalf("expected domain 'spec_field_ops', got %q", tbl.Domain())
	}
	if tbl.Len() != 2 {
		t.Fatalf("expected Len 2, got %d", tbl.Len())
	}
	if !tbl.HasAlias("define") {
		t.Fatalf("expected HasAlias('define') to be true")
	}
	if !tbl.HasAlias("  ADD  ") {
		t.Fatalf("expected HasAlias('  ADD  ') to be true")
	}
	if tbl.HasAlias("unknown") {
		t.Fatalf("expected HasAlias('unknown') to be false")
	}
	if tbl.HasAlias("   ") {
		t.Fatalf("expected HasAlias('   ') to be false")
	}

	// Resolve through Table
	if r := tbl.Resolve("  DEFINE  "); r != "create" {
		t.Fatalf("expected Resolve('  DEFINE  ') = 'create', got %q", r)
	}
	if r := tbl.Resolve("modify"); r != "modify" {
		t.Fatalf("expected Resolve('modify') = 'modify', got %q", r)
	}

	// Mappings defensive copy
	mappings := tbl.Mappings()
	mappings["injected"] = "tampered"
	if tbl.HasAlias("injected") {
		t.Fatalf("table mapping was mutated through defensive copy")
	}
}

func TestResolveStatic_HighConcurrencySafety(t *testing.T) {
	t.Parallel()

	tab := map[string]string{
		"define":    "create",
		"add":       "create",
		"rm":        "delete",
		"erase":     "delete",
		"show":      "get",
		"inspect":   "get",
		"deprecate": "deprecate",
	}

	const workers = 32
	const iterations = 500

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		workerID := i
		goroutinelabels.NewGoroutine("aliases_concurrency_test", fmt.Sprintf("worker-%d", workerID)).
			StartSimple(func() {
				defer wg.Done()
				runConcurrentWorker(t, tab, iterations)
			})
	}

	wg.Wait()
}

func runConcurrentWorker(t *testing.T, tab map[string]string, iterations int) {
	candidates := []struct {
		input    string
		expected string
	}{
		{"define", "create"},
		{"  ADD  ", "create"},
		{"RM", "delete"},
		{"erase", "delete"},
		{"  SHOW  ", "get"},
		{"inspect", "get"},
		{"modify", "modify"},
		{"", ""},
		{"   ", ""},
	}

	for j := 0; j < iterations; j++ {
		for _, c := range candidates {
			got := ResolveStatic(c.input, tab)
			if got != c.expected {
				t.Errorf("concurrent worker mismatch: ResolveStatic(%q) = %q; expected %q", c.input, got, c.expected)
			}
		}
	}
}

func TestTable_HighConcurrencySafety(t *testing.T) {
	t.Parallel()

	tbl := NewTable("concurrency_domain", map[string]string{
		"define": "create",
		"add":    "create",
		"rm":     "delete",
	})

	const workers = 32
	const iterations = 500

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		workerID := i
		goroutinelabels.NewGoroutine("table_concurrency_test", fmt.Sprintf("worker-%d", workerID)).
			StartSimple(func() {
				defer wg.Done()
				runTableConcurrentWorker(t, tbl, iterations)
			})
	}

	wg.Wait()
}

func runTableConcurrentWorker(t *testing.T, tbl *Table, iterations int) {
	for j := 0; j < iterations; j++ {
		if got := tbl.Resolve("  DEFINE  "); got != "create" {
			t.Errorf("tbl.Resolve mismatch: got %q, expected 'create'", got)
		}
		if !tbl.HasAlias("add") {
			t.Errorf("tbl.HasAlias('add') expected true")
		}
		if tbl.HasAlias("modify") {
			t.Errorf("tbl.HasAlias('modify') expected false")
		}
		if tbl.Domain() != "concurrency_domain" {
			t.Errorf("tbl.Domain mismatch")
		}
	}
}
