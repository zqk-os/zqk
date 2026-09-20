package objects

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtractIDAndKindFromYAMLPrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		data     []byte
		wantID   string
		wantKind string
	}{
		{"simple", []byte("id: BLI-010\nkind: backlog_item\n"), "BLI-010", "backlog_item"},
		{"quoted id", []byte("id: \"BLI-010\"\nkind: backlog_item\n"), "BLI-010", "backlog_item"},
		{"single quote", []byte("id: 'REQ-035'\nkind: requirement\n"), "REQ-035", "requirement"},
		{"id only", []byte("id: MIL-022\n"), "MIL-022", ""},
		{"with comment", []byte("# comment\nid: BLI-001\nkind: backlog_item\n"), "BLI-001", "backlog_item"},
		{"empty", []byte(""), "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotKind := ExtractIDAndKindFromYAMLPrefix(tt.data)
			if gotID != tt.wantID || gotKind != tt.wantKind {
				t.Errorf("ExtractIDAndKindFromYAMLPrefix() = %q, %q; want %q, %q", gotID, gotKind, tt.wantID, tt.wantKind)
			}
		})
	}
}

func TestReadIDAndKindFromYAMLFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "obj.yaml")
	if err := fileutil.WriteFile(path, []byte("id: BLI-999\nkind: backlog_item\ntitle: Test\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	id, kind := ReadIDAndKindFromYAMLFile(path)
	if id != "BLI-999" || kind != "backlog_item" {
		t.Errorf("ReadIDAndKindFromYAMLFile() = %q, %q; want BLI-999, backlog_item", id, kind)
	}
}
