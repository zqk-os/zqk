package cli

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestDataLoader_LoadFromString(t *testing.T) {
	dl := NewDataLoader(logging.GetLogger())

	// Valid inline YAML
	data, path, err := dl.LoadFromString("id: GOAL-1\ntitle: My Goal\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "" {
		t.Errorf("expected empty path for inline string")
	}
	if data["id"] != "GOAL-1" || data["title"] != "My Goal" {
		t.Errorf("unexpected parsed data: %+v", data)
	}

	// Invalid inline YAML
	_, _, err = dl.LoadFromString(":\ninvalid: [")
	if err == nil {
		t.Errorf("expected parse error for invalid YAML")
	}
}

func TestDataLoader_LoadFromFile(t *testing.T) {
	dl := NewDataLoader(logging.GetLogger())
	tmpDir := t.TempDir()

	validFile := filepath.Join(tmpDir, "valid.yaml")
	if err := fileutil.WriteFile(validFile, []byte("name: test-scenario\nsteps:\n  - step1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	data, path, err := dl.LoadFromFile(validFile)
	if err != nil {
		t.Fatalf("unexpected error loading file: %v", err)
	}
	if path != validFile {
		t.Errorf("expected path %s, got %s", validFile, path)
	}
	if data["name"] != "test-scenario" {
		t.Errorf("expected test-scenario, got %+v", data)
	}

	// Non-existent file
	_, _, err = dl.LoadFromFile(filepath.Join(tmpDir, "does-not-exist.yaml"))
	if err == nil {
		t.Errorf("expected error for missing file")
	}

	// Invalid YAML file
	invalidFile := filepath.Join(tmpDir, "bad.yaml")
	if err := fileutil.WriteFile(invalidFile, []byte("{\ninvalid: json: ["), 0644); err != nil {
		t.Fatal(err)
	}
	_, _, err = dl.LoadFromFile(invalidFile)
	if err == nil {
		t.Errorf("expected error for invalid YAML file")
	}
}

func TestDataLoader_LoadData_CommandFlags(t *testing.T) {
	dl := NewDataLoader(logging.GetLogger())

	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("data", "", "")
	cmd.Flags().StringArray("field", nil, "")

	// 1. With --data
	cmd.Flags().Set("data", "id: REQ-1\ndescription: A requirement")
	data, _, err := dl.LoadData(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data["id"] != "REQ-1" {
		t.Errorf("expected REQ-1, got %+v", data)
	}

	// 2. With --field overrides
	cmd.Flags().Set("data", "id: REQ-1\ntitle: Old Title")
	cmd.Flags().Set("field", "title=New Title")
	data, _, err = dl.LoadData(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data["title"] != "New Title" {
		t.Errorf("expected overridden title 'New Title', got %v", data["title"])
	}
}

func TestDataLoader_DraftAndStdin(t *testing.T) {
	dl := NewDataLoader(logging.GetLogger())

	// Nil / empty hints
	p, err := dl.tryLastDraft(nil)
	if err != nil || p != "" {
		t.Errorf("expected empty path for nil hint")
	}
	p, err = dl.tryLastDraft(&LastDraftHint{})
	if err != nil || p != "" {
		t.Errorf("expected empty path for empty hint")
	}

	data, path, err, handled := dl.loadFromDraftIfAvailable(nil)
	if handled || data != nil || path != "" || err != nil {
		t.Errorf("expected handled=false for nil hint")
	}

	// With actual draft file and draft pointer
	tmpDir := t.TempDir()
	draftFile := filepath.Join(tmpDir, "draft.yaml")
	if err := fileutil.WriteFile(draftFile, []byte("title: Draft Object\nstatus: draft\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Write draft pointer in tmpDir
	t.Setenv("ZQK_TEST_ROOT", tmpDir)
	hint := &LastDraftHint{Scope: LastDraftScopeObject, Kind: "goal"}
	pointerErr := WriteLastDraftPointer(tmpDir, hint.Scope, hint.Kind, draftFile)
	if pointerErr == nil {
		data, path, err := dl.loadFromTerminalOrDraft(hint)
		if err != nil || path != draftFile || data["title"] != "Draft Object" {
			t.Errorf("expected draft data from loadFromTerminalOrDraft, got: %+v (err=%v)", data, err)
		}
	}
}
