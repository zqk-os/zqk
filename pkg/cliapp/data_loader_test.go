package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/logging"
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
	if err := os.WriteFile(validFile, []byte("name: test-scenario\nsteps:\n  - step1\n"), 0644); err != nil {
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
	if err := os.WriteFile(invalidFile, []byte("{\ninvalid: json: ["), 0644); err != nil {
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
