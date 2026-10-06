package strutil

import "testing"

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		s1, s2 string
		want   int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "b", 1},
		{"kitten", "sitting", 3},
		{"sitting", "kitten", 3},
		{"book", "back", 2},
		{"same", "same", 0},
		{"short", "a much longer string", 16},
		{"a much longer string", "short", 16},
		{"abcdef", "azcedf", 3},
	}

	for _, tt := range tests {
		got := LevenshteinDistance(tt.s1, tt.s2)
		if got != tt.want {
			t.Errorf("LevenshteinDistance(%q, %q) = %d, want %d", tt.s1, tt.s2, got, tt.want)
		}
	}

	// Test boundary limit
	longStrA := make([]byte, maxLevenshteinStringLen+10)
	longStrB := make([]byte, maxLevenshteinStringLen+5)
	if got := LevenshteinDistance(string(longStrA), string(longStrB)); got != len(longStrA) {
		t.Errorf("expected max length fallback, got %d", got)
	}
}
