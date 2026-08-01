package object

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestStringSliceToAny(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   []string
		want []any
	}{
		{"empty", []string{}, []any{}},
		{"single", []string{"MIL-001"}, []any{"MIL-001"}},
		{"multiple", []string{"MIL-001", "MIL-002"}, []any{"MIL-001", "MIL-002"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stringSliceToAny(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("stringSliceToAny() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestObjectUpdate_AddRef_RemoveRef(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	tmpDir, cliBinary := setupCLITestEnvironment(t)
	// Create a milestone (target of reference)
	milID := "MIL-REF-TEST"
	milFile := filepath.Join(tmpDir, "milestone.yaml")
	milContent := map[string]any{
		objects.FieldKeyID:            milID,
		objects.FieldKeyKind:          "milestone",
		objects.FieldKeyTitle:         "Ref test milestone",
		objects.FieldKeyStatus:        objectStatusNotStarted,
		objects.FieldKeySchemaVersion: "2.0",
	}
	milData, _ := yaml.Marshal(milContent)
	if err := os.WriteFile(milFile, milData, paths.FilePerm644); err != nil {
		t.Fatalf("write milestone file: %v", err)
	}
	createMil := execCommand(t, cliBinary, tmpDir, "object", "create", "milestone", "--file", milFile)
	if createMil != emptyValue {
		t.Fatalf("create milestone failed: %s", createMil)
	}

	// Create a backlog_item (no refs initially)
	bliID := "ITEM-REF-TEST"
	bliFile := filepath.Join(tmpDir, "backlog_item.yaml")
	bliContent := map[string]any{
		objects.FieldKeyID:            bliID,
		objects.FieldKeyKind:          pplanKindBacklogItem,
		objects.FieldKeyTitle:         "Ref test item",
		objects.FieldKeyStatus:        objectStatusExploring,
		objects.FieldKeyGoalRefs:      []string{"G-123"},
		objects.FieldKeySchemaVersion: objectSchemaV2,
	}
	bliData, _ := yaml.Marshal(bliContent)
	if err := os.WriteFile(bliFile, bliData, paths.FilePerm644); err != nil {
		t.Fatalf("write backlog file: %v", err)
	}
	createBli := execCommand(t, cliBinary, tmpDir, "object", "create", pplanKindBacklogItem, "--file", bliFile)
	if createBli != emptyValue {
		t.Fatalf("create backlog_item failed: %s", createBli)
	}

	// Add reference via --add-ref
	addOut := execCommand(t, cliBinary, tmpDir, "object", "update", bliID, "--add-ref", "milestone_refs="+milID)
	if addOut != emptyValue {
		t.Fatalf("update --add-ref failed: %s", addOut)
	}

	// Verify object has the ref (read via CLI get or storage)
	st, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}
	obj, err := st.Read(ctxForTest(t), pkgctx.NewSystemSecurityContext(), bliID)
	if err != nil {
		t.Fatalf("Read after add-ref: %v", err)
	}
	refs, _ := obj[objects.FieldKeyMilestoneRefs].([]any)
	if len(refs) != 1 {
		t.Errorf("after add-ref: milestone_refs len = %d, want 1 (got %v)", len(refs), refs)
	}
	if len(refs) > 0 {
		if s, _ := refs[0].(string); s != milID {
			t.Errorf("after add-ref: milestone_refs[0] = %q, want %q", s, milID)
		}
	}

	// Remove reference via --remove-ref
	removeOut := execCommand(t, cliBinary, tmpDir, "object", "update", bliID, "--remove-ref", "milestone_refs="+milID)
	if removeOut != emptyValue {
		t.Fatalf("update --remove-ref failed: %s", removeOut)
	}
	// Use a fresh storage instance to avoid any in-process cache from the first read
	st2, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorage (after remove): %v", err)
	}
	obj2, err := st2.Read(ctxForTest(t), pkgctx.NewSystemSecurityContext(), bliID)
	if err != nil {
		t.Fatalf("Read after remove-ref: %v", err)
	}
	refs2, _ := obj2[objects.FieldKeyMilestoneRefs].([]any)
	if len(refs2) != 0 {
		t.Errorf("after remove-ref: milestone_refs len = %d, want 0 (got %v)", len(refs2), refs2)
	}
}

// TestObjectUpdate_ComplexFieldUpdate verifies ITEM-854: --field with complex (array) values.
func TestObjectUpdate_ComplexFieldUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	tmpDir, cliBinary := setupCLITestEnvironment(t)
	// Create backlog_item
	bliID := "ITEM-CFX-TEST"
	bliFile := filepath.Join(tmpDir, "backlog_item.yaml")
	bliContent := map[string]any{
		objects.FieldKeyID:            bliID,
		objects.FieldKeyKind:          pplanKindBacklogItem,
		objects.FieldKeyTitle:         "Complex field test",
		objects.FieldKeyStatus:        objectStatusExploring,
		objects.FieldKeyGoalRefs:      []string{"G-123"},
		objects.FieldKeySchemaVersion: objectSchemaV2,
	}
	bliData, _ := yaml.Marshal(bliContent)
	if err := os.WriteFile(bliFile, bliData, paths.FilePerm644); err != nil {
		t.Fatalf("write backlog file: %v", err)
	}
	if out := execCommand(t, cliBinary, tmpDir, "object", "create", pplanKindBacklogItem, "--file", bliFile); out != emptyValue {
		t.Fatalf("create backlog_item: %s", out)
	}

	// Update with complex (array) value via --field (ITEM-854). Value must be valid JSON
	// so the CLI parses a list, not a literal string (bare tokens like Comp1 are not JSON).
	if out := execCommand(t, cliBinary, tmpDir, "object", "update", bliID, "--field", `components=["Comp1","Comp2"]`); out != emptyValue {
		t.Fatalf("update --field components JSON array: %s", out)
	}

	st, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}
	obj, err := st.Read(ctxForTest(t), pkgctx.NewSystemSecurityContext(), bliID)
	if err != nil {
		t.Fatalf("Read after update: %v", err)
	}
	components, _ := obj[objects.FieldKeyComponents].([]any)
	if len(components) != 2 {
		t.Errorf("components len = %d, want 2 (got %v)", len(components), components)
	}
	if len(components) >= 1 {
		if s, _ := components[0].(string); s != "Comp1" {
			t.Errorf("components[0] = %q, want Comp1", components[0])
		}
	}
	if len(components) >= 2 {
		if s, _ := components[1].(string); s != "Comp2" {
			t.Errorf("components[1] = %q, want Comp2", components[1])
		}
	}
}

func execCommand(t *testing.T, binary, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(binary, args...)
	zqkenv.WireExecForIsolatedProject(cmd, dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out) + err.Error()
	}
	return ""
}

func ctxForTest(_ *testing.T) context.Context {
	return pkgctx.NewSystemContext()
}

func TestExpandDottedFieldKeys(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		input  map[string]any
		expect map[string]any
	}{
		{
			name:   "no dots unchanged",
			input:  map[string]any{objects.FieldKeyTitle: "x", objects.FieldKeyStatus: objectStatusActive},
			expect: map[string]any{objects.FieldKeyTitle: "x", objects.FieldKeyStatus: objectStatusActive},
		},
		{
			name:   "single dotted key",
			input:  map[string]any{"meta.tags": []any{"a", "b"}},
			expect: map[string]any{"meta": map[string]any{objects.FieldKeyTags: []any{"a", "b"}}},
		},
		{
			name: "two dotted keys same prefix",
			input: map[string]any{
				"meta.tags":  []any{"a"},
				"meta.count": 2,
			},
			expect: map[string]any{
				"meta": map[string]any{
					objects.FieldKeyTags: []any{"a"},
					"count":              2,
				},
			},
		},
		{
			name:  "three levels",
			input: map[string]any{"a.b.c": "leaf"},
			expect: map[string]any{
				"a": map[string]any{
					"b": map[string]any{
						"c": "leaf",
					},
				},
			},
		},
		{
			name: "mixed dotted and flat",
			input: map[string]any{
				objects.FieldKeyTitle: "t",
				"meta.x":              "v",
			},
			expect: map[string]any{
				objects.FieldKeyTitle: "t",
				"meta":                map[string]any{"x": "v"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandDottedFieldKeys(tt.input)
			if !reflect.DeepEqual(got, tt.expect) {
				t.Errorf("expandDottedFieldKeys() = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestDeriveNextLifecycleStatus_Criteria(t *testing.T) {
	t.Parallel()

	next, err := deriveNextLifecycleStatus(map[string]any{
		objects.FieldKeyKind:   "criteria",
		objects.FieldKeyStatus: objectStatusNotStarted,
	})
	if err != nil {
		t.Fatalf("deriveNextLifecycleStatus(%s) error: %v", objectStatusNotStarted, err)
	}
	if next != objectStatusInProgress {
		t.Fatalf("expected next status %s, got %q", objectStatusInProgress, next)
	}

	next, err = deriveNextLifecycleStatus(map[string]any{
		objects.FieldKeyKind:   "criteria",
		objects.FieldKeyStatus: objectStatusInProgress,
	})
	if err != nil {
		t.Fatalf("deriveNextLifecycleStatus(%s) error: %v", objectStatusInProgress, err)
	}
	// lifecycle order prefers validated before complete/blocked.
	if next != objectStatusValidated {
		t.Fatalf("expected next status %s, got %q", objectStatusValidated, next)
	}
}

func TestApplyAutoStatusFlag_RequiresTrait(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.Flags().Bool("auto-status", false, "")
	if err := cmd.Flags().Set("auto-status", "true"); err != nil {
		t.Fatalf("failed to set auto-status flag: %v", err)
	}
	updates := map[string]any{}
	err := applyAutoStatusFlag(cmd, map[string]any{
		objects.FieldKeyKind:   "scheduler_job",
		objects.FieldKeyStatus: objectStatusActive,
	}, updates)
	if err == nil {
		t.Fatalf("expected trait-gating error for scheduler_job")
	}
	if !strings.Contains(err.Error(), "not supported for kind") {
		t.Fatalf("expected unsupported-kind message, got: %v", err)
	}
}
