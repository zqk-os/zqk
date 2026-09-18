package storage

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestFirstLetterBucketStrategy_GetBucketKey(t *testing.T) {
	s := &FirstLetterBucketStrategy{Field: "title"}
	tests := []struct {
		name string
		obj  map[string]any
		want string
	}{
		{"letter lowercase", map[string]any{objects.FieldKeyTitle: "process data"}, "p"},
		{"letter uppercase", map[string]any{objects.FieldKeyTitle: "Agent guidelines"}, "a"},
		{"digit", map[string]any{objects.FieldKeyTitle: "42 things"}, "0"},
		{"empty title", map[string]any{objects.FieldKeyTitle: ""}, "_"},
		{"missing field", map[string]any{}, "_"},
		{"non-string field", map[string]any{objects.FieldKeyTitle: 123}, "_"},
		{"symbol first", map[string]any{objects.FieldKeyTitle: "& something"}, "_"},
		{"whitespace trimmed", map[string]any{objects.FieldKeyTitle: "  hot path  "}, "h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.GetBucketKey(tt.obj, "")
			if got != tt.want {
				t.Errorf("GetBucketKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFirstLetterBucketStrategy_GetBucketDirectory(t *testing.T) {
	s := &FirstLetterBucketStrategy{Field: "title"}
	base := datacell.CellCASPrimaryDir(string(filepath.Separator), "glossary_terms")
	if got := s.GetBucketDirectory(base, "a"); got != filepath.Join(base, "a") {
		t.Errorf("GetBucketDirectory() = %q, want %q", got, filepath.Join(base, "a"))
	}
	if got := s.GetBucketDirectory(base, ""); got != base {
		t.Errorf("GetBucketDirectory(empty) = %q, want %q", got, base)
	}
}

func TestFirstLetterBucketStrategy_Name(t *testing.T) {
	s := &FirstLetterBucketStrategy{Field: "title"}
	if got := s.Name(); got != "first-letter" {
		t.Errorf("Name() = %q, want %q", got, "first-letter")
	}
}
