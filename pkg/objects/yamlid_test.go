package objects

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
)

func TestExtractIDAndKindFromYAMLPrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		data     []byte
		wantID   string
		wantKind string
	}{
		{"simple", []byte("id: ITEM-010\nkind: backlog_item\n"), "ITEM-010", "backlog_item"},
		{"quoted id", []byte("id: \"ITEM-010\"\nkind: backlog_item\n"), "ITEM-010", "backlog_item"},
		{"single quote", []byte("id: 'REQ-035'\nkind: requirement\n"), "REQ-035", "requirement"},
		{"id only", []byte("id: MIL-022\n"), "MIL-022", ""},
		{"with comment", []byte("# comment\nid: ITEM-001\nkind: backlog_item\n"), "ITEM-001", "backlog_item"},
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
	if err := os.WriteFile(path, []byte("id: ITEM-999\nkind: backlog_item\ntitle: Test\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	id, kind := ReadIDAndKindFromYAMLFile(path)
	if id != "ITEM-999" || kind != "backlog_item" {
		t.Errorf("ReadIDAndKindFromYAMLFile() = %q, %q; want ITEM-999, backlog_item", id, kind)
	}
}
