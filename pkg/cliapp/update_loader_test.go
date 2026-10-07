package cli

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestDataLoader_LoadUpdates(t *testing.T) {
	dl := NewDataLoader(logging.GetLogger())

	// Empty inputs return nil, nil
	d1, err1 := dl.LoadUpdatesFromFile("")
	if d1 != nil || err1 != nil {
		t.Errorf("expected nil, nil for empty file path")
	}

	d2, err2 := dl.LoadUpdatesFromData("")
	if d2 != nil || err2 != nil {
		t.Errorf("expected nil, nil for empty inline data")
	}

	// Valid inline updates
	d3, err3 := dl.LoadUpdatesFromData("title: Updated Plan\nestimated_effort: 3\n")
	if err3 != nil {
		t.Fatalf("unexpected error loading inline updates: %v", err3)
	}
	if d3["title"] != "Updated Plan" {
		t.Errorf("expected 'Updated Plan', got %v", d3["title"])
	}

	// Valid file updates
	tmpDir := t.TempDir()
	fpath := filepath.Join(tmpDir, "updates.yaml")
	if err := fileutil.WriteFile(fpath, []byte("description: From file\n"), 0644); err != nil {
		t.Fatal(err)
	}
	d4, err4 := dl.LoadUpdatesFromFile(fpath)
	if err4 != nil {
		t.Fatalf("unexpected error loading file updates: %v", err4)
	}
	if d4["description"] != "From file" {
		t.Errorf("expected 'From file', got %v", d4["description"])
	}
}

func TestApplyAutoStatusFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "update"}
	cmd.Flags().Bool("auto-status", false, "")

	// 1. auto-status not set -> no-op
	updates := make(map[string]any)
	err := ApplyAutoStatusFlag(cmd, map[string]any{"kind": "criteria"}, updates)
	if err != nil || len(updates) > 0 {
		t.Errorf("expected no-op when auto-status flag is false")
	}

	// 2. auto-status set without existing object -> error
	_ = cmd.Flags().Set("auto-status", "true")
	err = ApplyAutoStatusFlag(cmd, nil, updates)
	if err == nil {
		t.Errorf("expected error when currentObj is nil")
	}

	// 3. auto-status with explicit status update -> conflict error
	updates[objects.FieldKeyStatus] = "passed"
	err = ApplyAutoStatusFlag(cmd, map[string]any{"kind": "criteria"}, updates)
	if err == nil {
		t.Errorf("expected conflict error when updates contains status")
	}

	// 4. unsupported kind (missing auto_status_transitionable trait)
	delete(updates, objects.FieldKeyStatus)
	err = ApplyAutoStatusFlag(cmd, map[string]any{"kind": "criteria"}, updates)
	if err == nil {
		t.Errorf("expected error for missing auto_status_transitionable trait")
	}

	// 5. valid kind (backlog_item with status planned) -> updates to testing
	err = ApplyAutoStatusFlag(cmd, map[string]any{"kind": "backlog_item", "status": "planned"}, updates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updates[objects.FieldKeyStatus] != "testing" {
		t.Errorf("expected status 'testing', got %v", updates[objects.FieldKeyStatus])
	}
}
