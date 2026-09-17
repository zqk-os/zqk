package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestFindNextChangeJournalID_NonStreamPath(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	journalDir := filepath.Join(dir, "change_journal", "2030-01")
	if err := fileutil.MkdirAll(journalDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Empty projectRoot or stream disabled => generator path (scan or sequence in journalDir)
	id, err := findNextChangeJournalID(ctx, "", journalDir)
	if err != nil {
		t.Fatalf("findNextChangeJournalID: %v", err)
	}
	if id == emptyValue || !strings.HasPrefix(id, "CHA-") {
		t.Errorf("findNextChangeJournalID() = %q, want CHA- prefix", id)
	}
}

func TestFindNextChangeJournalID_StreamPath_UsesCache(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	stateDir := filepath.Join(root, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.MkdirAll(stateDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	journalDir := filepath.Join(datacell.CellCASPrimaryDir(root, "change_journal"), "2030-01")
	if err := fileutil.MkdirAll(journalDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// With projectRoot and stream enabled, uses per-project ID cache (sequence file under stateDir)
	id1, err := findNextChangeJournalID(ctx, root, journalDir)
	if err != nil {
		t.Fatalf("findNextChangeJournalID: %v", err)
	}
	if id1 == emptyValue || !strings.HasPrefix(id1, "CHA-") {
		t.Errorf("findNextChangeJournalID() = %q, want CHA- prefix", id1)
	}
	id2, err := findNextChangeJournalID(ctx, root, journalDir)
	if err != nil {
		t.Fatalf("findNextChangeJournalID second call: %v", err)
	}
	if id2 == id1 {
		t.Errorf("findNextChangeJournalID() returned same ID twice: %q", id1)
	}
}

func TestFlattenUpdatePaths(t *testing.T) {
	tests := []struct {
		name    string
		updates map[string]any
		want    []string
	}{
		{
			name:    "empty",
			updates: nil,
			want:    nil,
		},
		{
			name:    "flat keys",
			updates: map[string]any{objects.FieldKeyTitle: "x", objects.FieldKeyStatus: objects.ObjectStatusActive},
			want:    []string{"status", "title"},
		},
		{
			name: "nested one level",
			updates: map[string]any{
				"meta": map[string]any{
					objects.FieldKeyTags: []any{"a", "b"},
					"count":              2,
				},
			},
			want: []string{"meta.count", "meta.tags"},
		},
		{
			name: "skips metadata",
			updates: map[string]any{
				objects.FieldKeyUpdatedAt: "2030-01-01T00:00:00Z",
				objects.FieldKeyUpdatedBy: "ACC-1785920548450214012-68b850c0",
				objects.FieldKeyTitle:     "t",
			},
			want: []string{"title"},
		},
		{
			name: "three levels",
			updates: map[string]any{
				"a": map[string]any{
					"b": map[string]any{
						"c": "leaf",
					},
				},
			},
			want: []string{"a.b.c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := flattenUpdatePaths(tt.updates)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("flattenUpdatePaths() = %v, want %v", got, tt.want)
			}
		})
	}
}
