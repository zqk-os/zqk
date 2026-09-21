package tray

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestMerge_replaceAndAppend(t *testing.T) {
	def := []Entry{
		{Name: "a", Argv: []string{"x"}},
		{Name: "b", Argv: []string{"y"}},
	}
	user := []Entry{
		{Name: "a", Description: "over", Argv: []string{"z"}},
		{Name: "c", Argv: []string{"w"}},
	}
	got := Merge(def, user)
	if len(got) != 3 {
		t.Fatalf("len=%d want 3", len(got))
	}
	if got[0].Name != "a" || got[0].Argv[0] != "z" {
		t.Fatalf("first entry: %+v", got[0])
	}
	if got[1].Name != "b" {
		t.Fatalf("second: %+v", got[1])
	}
	if got[2].Name != "c" {
		t.Fatalf("third: %+v", got[2])
	}
}

func TestLoad_embeddedOnly(t *testing.T) {
	entries, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 3 {
		t.Fatalf("expected several default entries, got %d", len(entries))
	}
}

func TestLoad_userFile_invalidSchema(t *testing.T) {
	tmp := t.TempDir()
	zqkDir := filepath.Join(tmp, paths.ProjectDataDir)
	if err := fileutil.EnsureDir(zqkDir); err != nil {
		t.Fatal(err)
	}
	path := datacell.TrayYAMLPath(tmp)
	// Uppercase / underscores violate name pattern
	content := `
version: 1
entries:
  - name: Bad_Name
    argv: [object, count]
`
	if err := fileutil.WriteSecureFile(path, []byte(content)); err != nil {
		t.Fatal(err)
	}
	_, err := Load(tmp)
	if err == nil {
		t.Fatal("expected schema validation error for invalid entry name")
	}
}

func TestLoad_userFile(t *testing.T) {
	tmp := t.TempDir()
	zqkDir := filepath.Join(tmp, paths.ProjectDataDir)
	if err := fileutil.EnsureDir(zqkDir); err != nil {
		t.Fatal(err)
	}
	path := datacell.TrayYAMLPath(tmp)
	content := `
version: 1
entries:
  - name: custom-one
    description: Custom
    argv: [object, count]
`
	if err := fileutil.WriteSecureFile(path, []byte(content)); err != nil {
		t.Fatal(err)
	}
	entries, err := Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if !containsName(entries, "custom-one") {
		t.Fatalf("missing custom-one: %#v", entries)
	}
	if !containsName(entries, "scheduler-status") {
		t.Fatal("expected default entries still present")
	}
}

func TestLoad_reloadsWhenStampMoves(t *testing.T) {
	tmp := t.TempDir()
	zqkDir := filepath.Join(tmp, paths.ProjectDataDir)
	if err := fileutil.EnsureDir(zqkDir); err != nil {
		t.Fatal(err)
	}
	path := datacell.TrayYAMLPath(tmp)
	write := func(name string) {
		t.Helper()
		content := `
version: 1
entries:
  - name: ` + name + `
    argv: [object, count]
`
		if err := fileutil.WriteSecureFile(path, []byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	write("custom-one")
	first, err := Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if !containsName(first, "custom-one") {
		t.Fatalf("missing custom-one: %#v", first)
	}
	write("custom-two")
	later := time.Now().Add(2 * time.Second)
	if err := fileutil.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	second, err := Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if !containsName(second, "custom-two") {
		t.Fatalf("expected reload after stamp move, got %#v", second)
	}
	if containsName(second, "custom-one") {
		t.Fatal("stale custom-one still present after stamp move")
	}
}

func containsName(entries []Entry, name string) bool {
	for _, e := range entries {
		if e.Name == name {
			return true
		}
	}
	return false
}
