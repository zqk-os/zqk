package cli

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
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

func TestLoadIDsFromFlags(t *testing.T) {
	t.Parallel()

	setupCmd := func() *cobra.Command {
		cmd := &cobra.Command{}
		cmd.Flags().String("ids", "", "Comma-separated IDs")
		cmd.Flags().String("file", "", "File path containing IDs")
		return cmd
	}

	t.Run("from_ids_flag", func(t *testing.T) {
		cmd := setupCmd()
		_ = cmd.Flags().Set("ids", "ID-1, ID-2")

		ids, err := LoadIDsFromFlags(cmd, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ids) != 2 || ids[0] != "ID-1" || ids[1] != "ID-2" {
			t.Fatalf("unexpected ids: %v", ids)
		}
	})

	t.Run("from_file_flag", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "ids.yaml")
		if err := fileutil.WriteFile(filePath, []byte("- ID-A\n- ID-B\n"), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		cmd := setupCmd()
		_ = cmd.Flags().Set("file", filePath)

		ids, err := LoadIDsFromFlags(cmd, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ids) != 2 || ids[0] != "ID-A" || ids[1] != "ID-B" {
			t.Fatalf("unexpected ids: %v", ids)
		}
	})

	t.Run("missing_flags_returns_error", func(t *testing.T) {
		cmd := setupCmd()

		_, err := LoadIDsFromFlags(cmd, nil)
		if err == nil {
			t.Fatal("expected error when no flags provided, got nil")
		}
	})
}
