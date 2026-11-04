// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package strutil

import (
	"fmt"
	"strings"
	"testing"
)

// TestCycle174_OrDefaultComprehensive verifies fallback resolution with various string patterns.
func TestCycle174_OrDefaultComprehensive(t *testing.T) {
	testCases := []struct {
		name       string
		val        string
		defaultVal string
		want       string
	}{
		{name: "non-empty-standard", val: "active", defaultVal: "pending", want: "active"},
		{name: "empty-value", val: "", defaultVal: "fallback", want: "fallback"},
		{name: "both-empty", val: "", defaultVal: "", want: ""},
		{name: "whitespace-preserved", val: "   ", defaultVal: "fallback", want: "   "},
		{name: "newline-preserved", val: "\n", defaultVal: "fallback", want: "\n"},
		{name: "unicode-value", val: "こんにちは", defaultVal: "hello", want: "こんにちは"},
		{name: "unicode-default", val: "", defaultVal: "世界", want: "世界"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := OrDefault(tc.val, tc.defaultVal)
			if got != tc.want {
				t.Errorf("OrDefault(%q, %q) = %q; want %q", tc.val, tc.defaultVal, got, tc.want)
			}
		})
	}
}

// TestCycle174_CoalesceFallbackChain exercises chained fallback evaluation.
func TestCycle174_CoalesceFallbackChain(t *testing.T) {
	// Demonstrates coalescing through sequential OrDefault calls
	coalesce := func(candidates ...string) string {
		result := ""
		for _, cand := range candidates {
			result = OrDefault(result, cand)
		}
		return result
	}

	if got := coalesce("", "", "primary", "secondary"); got != "primary" {
		t.Errorf("coalesce() = %q; want primary", got)
	}
	if got := coalesce("", "", ""); got != "" {
		t.Errorf("coalesce() = %q; want empty", got)
	}
	if got := coalesce("first", "second"); got != "first" {
		t.Errorf("coalesce() = %q; want first", got)
	}
}

// TestCycle174_DefaultWhenNonEmptyComprehensive tests negation fallback logic.
func TestCycle174_DefaultWhenNonEmptyComprehensive(t *testing.T) {
	testCases := []struct {
		name       string
		val        string
		defaultVal string
		want       string
	}{
		{name: "non-empty-overridden", val: "user-input", defaultVal: "replacement", want: "replacement"},
		{name: "empty-retained", val: "", defaultVal: "fallback", want: ""},
		{name: "whitespace-overridden", val: "\t", defaultVal: "cleared", want: "cleared"},
		{name: "both-empty", val: "", defaultVal: "", want: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := DefaultWhenNonEmpty(tc.val, tc.defaultVal)
			if got != tc.want {
				t.Errorf("DefaultWhenNonEmpty(%q, %q) = %q; want %q", tc.val, tc.defaultVal, got, tc.want)
			}
		})
	}
}

// TestCycle174_PtrOrDefaultGenerics tests PtrOrDefault with diverse concrete and composite types.
func TestCycle174_PtrOrDefaultGenerics(t *testing.T) {
	type configNode struct {
		id      string
		retries int
	}

	// String pointer
	strVal := "configured"
	if got := PtrOrDefault(&strVal, "default"); got != "configured" {
		t.Errorf("PtrOrDefault string = %q; want configured", got)
	}
	if got := PtrOrDefault((*string)(nil), "default"); got != "default" {
		t.Errorf("PtrOrDefault nil string = %q; want default", got)
	}

	// Struct pointer
	cfg := configNode{id: "node-1", retries: 3}
	defCfg := configNode{id: "fallback-node", retries: 1}
	if got := PtrOrDefault(&cfg, defCfg); got.id != "node-1" || got.retries != 3 {
		t.Errorf("PtrOrDefault struct = %+v; want node-1", got)
	}
	if got := PtrOrDefault((*configNode)(nil), defCfg); got.id != "fallback-node" || got.retries != 1 {
		t.Errorf("PtrOrDefault nil struct = %+v; want fallback-node", got)
	}

	// Boolean pointer
	bTrue := true
	if got := PtrOrDefault(&bTrue, false); !got {
		t.Errorf("PtrOrDefault bool = %v; want true", got)
	}
	if got := PtrOrDefault((*bool)(nil), true); !got {
		t.Errorf("PtrOrDefault nil bool = %v; want true", got)
	}
}

// TestCycle174_FirstNonNilGenerics tests FirstNonNil across varying chain lengths.
func TestCycle174_FirstNonNilGenerics(t *testing.T) {
	v1 := 10
	v2 := 20
	v3 := 30

	if got := FirstNonNil(&v1, &v2, &v3); got == nil || *got != 10 {
		t.Errorf("FirstNonNil(v1, v2, v3) = %v; want 10", got)
	}
	if got := FirstNonNil((*int)(nil), &v2, &v3); got == nil || *got != 20 {
		t.Errorf("FirstNonNil(nil, v2, v3) = %v; want 20", got)
	}
	if got := FirstNonNil((*int)(nil), (*int)(nil), &v3); got == nil || *got != 30 {
		t.Errorf("FirstNonNil(nil, nil, v3) = %v; want 30", got)
	}
	if got := FirstNonNil((*int)(nil), (*int)(nil)); got != nil {
		t.Errorf("FirstNonNil(nil, nil) = %v; want nil", got)
	}
	if got := FirstNonNil[int](); got != nil {
		t.Errorf("FirstNonNil() = %v; want nil", got)
	}
}

// TestCycle174_SplitLinesEdgeCases tests SplitLines across various line-ending styles.
func TestCycle174_SplitLinesEdgeCases(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "unix-newlines",
			input: "alpha\nbeta\ngamma",
			want:  []string{"alpha", "beta", "gamma"},
		},
		{
			name:  "windows-newlines",
			input: "alpha\r\nbeta\r\ngamma",
			want:  []string{"alpha", "beta", "gamma"},
		},
		{
			name:  "classic-mac-newlines",
			input: "alpha\rbeta\rgamma",
			want:  []string{"alpha", "beta", "gamma"},
		},
		{
			name:  "mixed-newlines",
			input: "alpha\r\nbeta\ngamma\rdelta",
			want:  []string{"alpha", "beta", "gamma", "delta"},
		},
		{
			name:  "trailing-newline",
			input: "line1\nline2\n",
			want:  []string{"line1", "line2", ""},
		},
		{
			name:  "empty-input",
			input: "",
			want:  []string{""},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := SplitLines(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("SplitLines(%q) returned %d elements, want %d", tc.input, len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("SplitLines[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestCycle174_LevenshteinDistanceExhaustive covers Levenshtein distances and limits.
func TestCycle174_LevenshteinDistanceExhaustive(t *testing.T) {
	tests := []struct {
		name string
		s1   string
		s2   string
		want int
	}{
		{name: "both-empty", s1: "", s2: "", want: 0},
		{name: "s1-empty", s1: "", s2: "hello", want: 5},
		{name: "s2-empty", s1: "world", s2: "", want: 5},
		{name: "identical", s1: "kernel", s2: "kernel", want: 0},
		{name: "single-substitution", s1: "cat", s2: "bat", want: 1},
		{name: "single-insertion", s1: "cat", s2: "cats", want: 1},
		{name: "single-deletion", s1: "cats", s2: "cat", want: 1},
		{name: "swapped-symmetry", s1: "sitting", s2: "kitten", want: 3},
		{name: "case-difference", s1: "ZQK", s2: "zqk", want: 3},
		{name: "completely-disjoint", s1: "abc", s2: "xyz", want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LevenshteinDistance(tt.s1, tt.s2)
			if got != tt.want {
				t.Errorf("LevenshteinDistance(%q, %q) = %d, want %d", tt.s1, tt.s2, got, tt.want)
			}
		})
	}
}

// TestCycle174_LevenshteinDistanceUnicode tests UTF-8 byte-level distance behavior.
func TestCycle174_LevenshteinDistanceUnicode(t *testing.T) {
	// Unicode runes occupy multiple bytes; LevenshteinDistance computes byte-level edit distance.
	s1 := "café"
	s2 := "cafe"
	// 'é' is 2 bytes (0xc3, 0xa9), 'e' is 1 byte (0x65). Distance is 2 byte edits.
	got := LevenshteinDistance(s1, s2)
	if got <= 0 {
		t.Errorf("LevenshteinDistance(café, cafe) expected > 0, got %d", got)
	}

	// Identical Unicode strings have 0 distance
	if gotZero := LevenshteinDistance("日本語", "日本語"); gotZero != 0 {
		t.Errorf("LevenshteinDistance(日本語, 日本語) = %d, want 0", gotZero)
	}

	// Single rune substitution
	distCJK := LevenshteinDistance("日本語", "日本国")
	if distCJK <= 0 {
		t.Errorf("LevenshteinDistance(日本語, 日本国) expected > 0, got %d", distCJK)
	}
}

// TestCycle174_LevenshteinMaxLenBranches hits both branches of length limit fallback.
func TestCycle174_LevenshteinMaxLenBranches(t *testing.T) {
	// Case 1: la > maxLevenshteinStringLen AND la > lb -> returns la
	strLong4100 := strings.Repeat("x", maxLevenshteinStringLen+100)
	strShort4000 := strings.Repeat("x", maxLevenshteinStringLen-96)
	if got := LevenshteinDistance(strLong4100, strShort4000); got != len(strLong4100) {
		t.Errorf("LevenshteinDistance(la > lb) = %d, want %d", got, len(strLong4100))
	}

	// Case 2: lb > maxLevenshteinStringLen AND lb > la -> returns lb (hits lb > la branch!)
	if got := LevenshteinDistance(strShort4000, strLong4100); got != len(strLong4100) {
		t.Errorf("LevenshteinDistance(lb > la) = %d, want %d", got, len(strLong4100))
	}

	// Case 3: both > maxLevenshteinStringLen with la == lb -> returns lb
	strLongEqualA := strings.Repeat("a", maxLevenshteinStringLen+10)
	strLongEqualB := strings.Repeat("b", maxLevenshteinStringLen+10)
	if got := LevenshteinDistance(strLongEqualA, strLongEqualB); got != len(strLongEqualB) {
		t.Errorf("LevenshteinDistance(la == lb > max) = %d, want %d", got, len(strLongEqualB))
	}
}

// TestCycle174_SimilarityRatioMetric calculates normalized similarity [0.0, 1.0].
func TestCycle174_SimilarityRatioMetric(t *testing.T) {
	// Demonstrates similarity ratio formula based on LevenshteinDistance
	calculateSimilarity := func(s1, s2 string) float64 {
		if s1 == s2 {
			return 1.0
		}
		maxLen := len(s1)
		if len(s2) > maxLen {
			maxLen = len(s2)
		}
		if maxLen == 0 {
			return 1.0
		}
		dist := LevenshteinDistance(s1, s2)
		return 1.0 - (float64(dist) / float64(maxLen))
	}

	if sim := calculateSimilarity("deploy", "deploy"); sim != 1.0 {
		t.Errorf("identical similarity = %f; want 1.0", sim)
	}
	if sim := calculateSimilarity("", ""); sim != 1.0 {
		t.Errorf("empty similarity = %f; want 1.0", sim)
	}
	if sim := calculateSimilarity("cat", "bat"); sim <= 0.6 || sim >= 0.7 {
		t.Errorf("similarity('cat', 'bat') = %f; want approx 0.666", sim)
	}
	if sim := calculateSimilarity("abc", "xyz"); sim != 0.0 {
		t.Errorf("disjoint similarity = %f; want 0.0", sim)
	}
}

// TestCycle174_DecodeEscapeRuneComprehensive covers all known and default escape runes.
func TestCycle174_DecodeEscapeRuneComprehensive(t *testing.T) {
	testCases := []struct {
		name string
		in   rune
		want rune
	}{
		{name: "newline", in: 'n', want: '\n'},
		{name: "tab", in: 't', want: '\t'},
		{name: "carriage-return", in: 'r', want: '\r'},
		{name: "backslash", in: '\\', want: '\\'},
		{name: "double-quote", in: '"', want: '"'},
		{name: "single-quote", in: '\'', want: '\''},
		{name: "null-byte-default", in: 0, want: 0},
		{name: "alphanumeric-default-a", in: 'a', want: 'a'},
		{name: "alphanumeric-default-z", in: 'z', want: 'z'},
		{name: "digit-default", in: '5', want: '5'},
		{name: "symbol-default-dollar", in: '$', want: '$'},
		{name: "unicode-default-emoji", in: '🚀', want: '🚀'},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DecodeEscapeRune(tc.in); got != tc.want {
				t.Errorf("DecodeEscapeRune(%c) = %c; want %c", tc.in, got, tc.want)
			}
		})
	}
}

// TestCycle174_UnescapeStringPattern demonstrates string unescaping using DecodeEscapeRune.
func TestCycle174_UnescapeStringPattern(t *testing.T) {
	unescapeString := func(raw string) string {
		var sb strings.Builder
		runes := []rune(raw)
		for i := 0; i < len(runes); i++ {
			if runes[i] == '\\' && i+1 < len(runes) {
				sb.WriteRune(DecodeEscapeRune(runes[i+1]))
				i++
			} else {
				sb.WriteRune(runes[i])
			}
		}
		return sb.String()
	}

	tests := []struct {
		input string
		want  string
	}{
		{input: `hello\nworld`, want: "hello\nworld"},
		{input: `col1\tcol2`, want: "col1\tcol2"},
		{input: `path\\to\\file`, want: `path\to\file`},
		{input: `quoted \"value\"`, want: `quoted "value"`},
		{input: `plain text`, want: "plain text"},
	}

	for _, tt := range tests {
		got := unescapeString(tt.input)
		if got != tt.want {
			t.Errorf("unescapeString(%q) = %q; want %q", tt.input, got, tt.want)
		}
	}
}

// TestCycle174_ExtractFirstPrefixedValueComprehensive tests prefix scanning and value extraction.
func TestCycle174_ExtractFirstPrefixedValueComprehensive(t *testing.T) {
	lines := []string{
		"  # Header comment  ",
		"  TITLE: System Initialization  ",
		"  STATUS: active  ",
		"  EMPTY_VAL:   ",
	}

	// Found on second line
	if got := ExtractFirstPrefixedValue(lines, "TITLE:"); got != "System Initialization" {
		t.Errorf("ExtractFirstPrefixedValue(TITLE:) = %q; want System Initialization", got)
	}

	// Found on third line with multiple prefix options
	if got := ExtractFirstPrefixedValue(lines, "UNKNOWN:", "STATUS:"); got != "active" {
		t.Errorf("ExtractFirstPrefixedValue(UNKNOWN, STATUS) = %q; want active", got)
	}

	// Empty value after prefix
	if got := ExtractFirstPrefixedValue(lines, "EMPTY_VAL:"); got != "" {
		t.Errorf("ExtractFirstPrefixedValue(EMPTY_VAL:) = %q; want empty string", got)
	}

	// Non-matching prefix
	if got := ExtractFirstPrefixedValue(lines, "NONEXISTENT:"); got != "" {
		t.Errorf("ExtractFirstPrefixedValue(NONEXISTENT:) = %q; want empty string", got)
	}

	// Empty lines slice
	if got := ExtractFirstPrefixedValue([]string{}, "TITLE:"); got != "" {
		t.Errorf("ExtractFirstPrefixedValue(empty lines) = %q; want empty string", got)
	}
}

// TestCycle174_FindLineWithPrefixComprehensive covers matching and non-matching lines.
func TestCycle174_FindLineWithPrefixComprehensive(t *testing.T) {
	lines := []string{
		"   info: system check passed   ",
		"   warn: high latency detected   ",
		"   error: fatal disk corruption   ",
	}

	// Matching first prefix
	if got := FindLineWithPrefix(lines, "info:"); got != "info: system check passed" {
		t.Errorf("FindLineWithPrefix(info:) = %q; want info: system check passed", got)
	}

	// Matching second prefix
	if got := FindLineWithPrefix(lines, "debug:", "warn:"); got != "warn: high latency detected" {
		t.Errorf("FindLineWithPrefix(debug:, warn:) = %q; want warn: high latency detected", got)
	}

	// Non-matching prefix returns empty string (hits line 30 of extraction.go!)
	if got := FindLineWithPrefix(lines, "missing:", "notfound:"); got != "" {
		t.Errorf("FindLineWithPrefix(missing:) = %q; want empty string", got)
	}

	// Empty lines slice returns empty string
	if got := FindLineWithPrefix([]string{}, "any:"); got != "" {
		t.Errorf("FindLineWithPrefix(empty lines) = %q; want empty string", got)
	}
}

// TestCycle174_IsAllDigitsExhaustive thoroughly validates digit checking invariants.
func TestCycle174_IsAllDigitsExhaustive(t *testing.T) {
	testCases := []struct {
		name string
		val  string
		want bool
	}{
		{name: "empty-string", val: "", want: false},
		{name: "single-zero", val: "0", want: true},
		{name: "single-nine", val: "9", want: true},
		{name: "all-decimal-digits", val: "0123456789", want: true},
		{name: "alphanumeric-mix-leading", val: "a123", want: false},
		{name: "alphanumeric-mix-trailing", val: "123z", want: false},
		{name: "alphanumeric-mix-internal", val: "12x34", want: false},
		{name: "space-leading", val: " 123", want: false},
		{name: "space-trailing", val: "123 ", want: false},
		{name: "space-internal", val: "12 34", want: false},
		{name: "negative-sign", val: "-123", want: false},
		{name: "positive-sign", val: "+123", want: false},
		{name: "floating-point-dot", val: "12.34", want: false},
		{name: "null-byte-inside", val: fmt.Sprintf("12%c34", 0), want: false},
		{name: "fullwidth-unicode-digit", val: "１２３", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsAllDigits(tc.val); got != tc.want {
				t.Errorf("IsAllDigits(%q) = %v; want %v", tc.val, got, tc.want)
			}
		})
	}
}
