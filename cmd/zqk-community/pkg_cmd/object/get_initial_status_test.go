package object

// ITEM-177483 inventory: SetupTestEnvironment → testkit.RunStandardTeardown (TempProjectTeardown) in test_helpers.go.

import (
	"os"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestGetOriginStatusForKind_BacklogItem(t *testing.T) {
	got := getInitialStatusForKind(pplanKindBacklogItem)
	if got != objectStatusExploring {
		// Help diagnose lifecycle parsing by also listing lifecycle origin statuses.
		lc, err := objects.NewLifecycleLoader("").LoadLifecycle(pplanKindBacklogItem)
		if err != nil {
			t.Fatalf("expected origin status %s, got %q (lifecycle load err: %v)", objectStatusExploring, got, err)
		}
		var origins []string
		for _, s := range lc.Statuses {
			if s.Origin {
				origins = append(origins, s.Value)
			}
		}
		t.Fatalf("expected origin status %s, got %q; lifecycle origin statuses=%v", objectStatusExploring, got, origins)
	}
}

func TestCreateTestObject_BacklogItem_SetsStatusExploring(t *testing.T) {
	_ = SetupTestEnvironment(t) // Ensure consistent ZQK_TEST_ROOT + spec tree.
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.Reload(); err != nil {
		t.Fatalf("failed to reload field registry: %v", err)
	}

	kindFields, err := fieldRegistry.GetFieldsForKind(pplanKindBacklogItem)
	if err != nil {
		t.Fatalf("failed to get fields for backlog_item: %v", err)
	}

	obj := createTestObject(pplanKindBacklogItem, "ITEM-001", kindFields, 0)
	status, _ := obj[objects.FieldKeyStatus].(string)
	if status != objectStatusExploring {
		t.Fatalf("expected createTestObject status=%s, got %q (obj keys: %v)", objectStatusExploring, status, keys(obj))
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestBacklogItemCreate_UsesStatusExploring(t *testing.T) {
	testEnv := SetupTestEnvironment(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("failed to load field registry: %v", err)
	}
	kindFields, err := fieldRegistry.GetFieldsForKind(pplanKindBacklogItem)
	if err != nil {
		t.Fatalf("failed to get fields for backlog_item: %v", err)
	}

	obj := createTestObject(pplanKindBacklogItem, "ITEM-001", kindFields, 0)
	status, _ := obj[objects.FieldKeyStatus].(string)
	if status != objectStatusExploring {
		t.Fatalf("expected createTestObject status=%s, got %q", objectStatusExploring, status)
	}

	// Write to temp file used by CLI.
	tmpFile := t.TempDir() + "/backlog_item.yaml"
	//nolint:gosec // Test files - temp dir controlled
	data, err := yaml.Marshal(obj)
	if err != nil {
		t.Fatalf("failed to marshal yaml: %v", err)
	}
	if err := os.WriteFile(tmpFile, data, paths.FilePerm644); err != nil { //nolint:gosec // test-only file write
		t.Fatalf("failed to write yaml: %v", err)
	}

	cmd := testEnv.CreateCLICommand("object", "create", pplanKindBacklogItem, "--file", tmpFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected backlog_item create to succeed; got err=%v output=%s", err, string(out))
	}
}
