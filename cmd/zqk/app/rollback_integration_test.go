package app_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/cmd/zqk/app"
	"github.com/zqk-os/zqk/pkg/storage"
)

const (
	rollbackScenarioList  = "rollback-list-integration"
	rollbackScenarioApply = "rollback-apply-integration"
	rollbackScenarioKeep  = "rollback-retain-integration"
	rollbackMissingPoint  = "rb-nonexistent-point-id"
)

// TestRollbackList_Integration runs "zqk rollback list" with a test project root and asserts JSON output.
func TestRollbackList_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testRoot, _ := setupIntegrationTestWithSpecsApp(t, rollbackScenarioList)
	t.Cleanup(func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, nil))
	})

	root := app.NewRootCommand()
	root.SetArgs([]string{"rollback", "list", "--format", "json"})

	out := captureStdoutApp(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("rollback list: %v", err)
		}
	})

	// Expect valid JSON with "points" key
	var decoded struct {
		Points interface{} `json:"points"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Errorf("rollback list output should be JSON with points key: %v\noutput: %s", err, out)
	}
}

// TestRollbackApply_NotFound_Integration runs "zqk rollback apply <fake-id>" and asserts "not found" error.
func TestRollbackApply_NotFound_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testRoot, _ := setupIntegrationTestWithSpecsApp(t, rollbackScenarioApply)
	t.Cleanup(func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, nil))
	})

	root := app.NewRootCommand()
	root.SetArgs([]string{"rollback", "apply", rollbackMissingPoint, "--format", "json"})

	err := root.Execute()
	if err == nil {
		t.Fatal("rollback apply with nonexistent point should fail")
	}
	if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), rollbackMissingPoint) {
		t.Errorf("expected 'not found' or point id in error, got: %v", err)
	}
}

// TestRollbackRetain_Integration runs "zqk rollback retain" with a test project root (no-op when store empty).
func TestRollbackRetain_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testRoot, _ := setupIntegrationTestWithSpecsApp(t, rollbackScenarioKeep)
	t.Cleanup(func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, nil))
	})

	root := app.NewRootCommand()
	root.SetArgs([]string{"rollback", "retain", "--format", "json"})

	out := captureStdoutApp(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("rollback retain: %v", err)
		}
	})

	var decoded struct {
		Retained bool `json:"retained"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Errorf("rollback retain output should be JSON: %v\noutput: %s", err, out)
	}
	if !decoded.Retained {
		t.Errorf("expected retained: true, got %+v", decoded)
	}
}
