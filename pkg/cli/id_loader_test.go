package cli

import (
	"reflect"
	"testing"
)

func TestExpandCommaSeparatedIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		tokens []string
		want   []string
	}{
		{name: "nil", tokens: nil, want: nil},
		{name: "single", tokens: []string{"ATK-1"}, want: []string{"ATK-1"}},
		{
			name:   "comma_joined_positional",
			tokens: []string{"ATK-1,ATK-2,ATK-3"},
			want:   []string{"ATK-1", "ATK-2", "ATK-3"},
		},
		{
			name:   "mixed_space_and_commas",
			tokens: []string{"ATK-1", "ATK-2, ATK-3", " ATK-4 "},
			want:   []string{"ATK-1", "ATK-2", "ATK-3", "ATK-4"},
		},
		{
			name:   "empty_segments_dropped",
			tokens: []string{"ATK-1,,ATK-2,", ""},
			want:   []string{"ATK-1", "ATK-2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ExpandCommaSeparatedIDs(tt.tokens...)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ExpandCommaSeparatedIDs(%v) = %#v, want %#v", tt.tokens, got, tt.want)
			}
		})
	}
}
