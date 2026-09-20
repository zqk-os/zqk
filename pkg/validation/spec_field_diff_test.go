package validation

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func findProcessDocsRoot(t *testing.T) string {
	t.Helper()
	cwd, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := cwd
	for {
		if _, err := fileutil.Stat(filepath.Join(root, paths.ProcessDir)); err == nil {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Skip(paths.ProcessDir + " not found")
		}
		root = parent
	}
}

func forEachProcessObject(t *testing.T, processDir string, fn func(filePath string, obj map[string]any)) {
	t.Helper()
	entries, err := fileutil.ReadDir(processDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), "_") {
			continue
		}
		kindDir := filepath.Join(processDir, entry.Name())
		files, err := fileutil.ReadDir(kindDir)
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".yaml") {
				continue
			}
			filePath := filepath.Join(kindDir, file.Name())
			data, err := fileutil.ReadFile(filepath.Clean(filePath))
			if err != nil {
				continue
			}
			var obj map[string]any
			if err := yaml.Unmarshal(data, &obj); err != nil {
				continue
			}
			fn(filePath, obj)
		}
	}
}

func TestSpecFieldDiff_InventoryExtras(t *testing.T) {
	root := findProcessDocsRoot(t)
	specsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
	specLoader := objects.NewSpecLoader(specsDir)
	processDir := filepath.Join(root, paths.ProcessDir)

	extrasByKind := make(map[string]map[string]int)

	forEachProcessObject(t, processDir, func(_ string, obj map[string]any) {
		kind, _ := obj[objects.FieldKeyKind].(string)
		if kind == "" {
			return
		}
		spec, err := specLoader.LoadSpecWithInheritance(kind + ".yaml")
		if err != nil || spec == nil || spec.ResolvedFields == nil {
			return
		}
		for k := range obj {
			if _, ok := spec.ResolvedFields[k]; !ok {
				if extrasByKind[kind] == nil {
					extrasByKind[kind] = make(map[string]int)
				}
				extrasByKind[kind][k]++
			}
		}
	})

	var kinds []string
	for k := range extrasByKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)

	for _, k := range kinds {
		t.Logf("Kind %s extras:", k)
		var fields []string
		for f := range extrasByKind[k] {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		for _, f := range fields {
			t.Logf("  %s: %d instances", f, extrasByKind[k][f])
		}
	}
}

func TestSpecFieldDiff_InventoryDuplicateRefs(t *testing.T) {
	root := findProcessDocsRoot(t)
	processDir := filepath.Join(root, paths.ProcessDir)

	forEachProcessObject(t, processDir, func(_ string, obj map[string]any) {
		id, _ := obj[objects.FieldKeyID].(string)

		refOccurrences := make(map[string][]string) // targetID -> list of field names
		for k, v := range obj {
			if !strings.HasSuffix(k, "_refs") && !strings.HasSuffix(k, "_ref") && k != objects.FieldKeyDependencies {
				continue
			}
			switch val := v.(type) {
			case []any:
				for _, item := range val {
					if s, ok := item.(string); ok && s != "" {
						refOccurrences[s] = append(refOccurrences[s], k)
					}
				}
			case []string:
				for _, s := range val {
					if s != "" {
						refOccurrences[s] = append(refOccurrences[s], k)
					}
				}
			case string:
				if val != "" {
					refOccurrences[val] = append(refOccurrences[val], k)
				}
			}
		}

		for targetID, fields := range refOccurrences {
			if len(fields) > 1 {
				t.Logf("Object %s has duplicate ref %s across fields: %v", id, targetID, fields)
			}
		}
	})
}

func TestGoValidator_StrictMode_UnknownFields(t *testing.T) {
	gv := NewGoValidator()
	ctx := pkgctx.NewSystemContext()

	obj := map[string]any{
		objects.FieldKeyID:            "GOAL-TEST-001",
		objects.FieldKeyKind:          objects.KindGoal,
		objects.FieldKeyTitle:         "Test goal with unknown field",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     "2026-01-01T00:00:00Z",
		objects.FieldKeyCreatedBy:     "ACC-TEST",
		objects.FieldKeyUpdatedAt:     "2026-01-01T00:00:00Z",
		objects.FieldKeyUpdatedBy:     "ACC-TEST",
		"invented_stray_field":        "some value",
	}

	// Normal options (StrictMode = false) should ignore the unknown field for kinds without overlay
	resNormal, err := gv.Validate(ctx, obj, objects.KindGoal, &ValidationOptions{
		StrictMode: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, e := range resNormal.Errors {
		if e.Rule == "unknown_field" {
			t.Fatalf("StrictMode: false should not report unknown_field error: %v", e)
		}
	}

	// StrictMode = true must fail with unknown_field error
	resStrict, err := gv.Validate(ctx, obj, objects.KindGoal, &ValidationOptions{
		StrictMode: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, e := range resStrict.Errors {
		if e.Rule == "unknown_field" && e.Field == "invented_stray_field" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected unknown_field error for invented_stray_field under StrictMode, got: %v", resStrict.Errors)
	}
}

func TestCompose_BacklogItem_RefusesUnknownFields(t *testing.T) {
	gv := NewGoValidator()
	ctx := pkgctx.NewSystemContext()

	obj := map[string]any{
		objects.FieldKeyID:            "BLI-TEST-001",
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         "Test item with unknown field",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     "2026-01-01T00:00:00Z",
		objects.FieldKeyCreatedBy:     "ACC-TEST",
		objects.FieldKeyUpdatedAt:     "2026-01-01T00:00:00Z",
		objects.FieldKeyUpdatedBy:     "ACC-TEST",
		"invented_stray_field":        "some value",
	}

	// Even with StrictMode = false, backlog_item overlay refuses unknown fields
	res, err := gv.Validate(ctx, obj, objects.KindBacklogItem, &ValidationOptions{
		StrictMode: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, e := range res.Errors {
		if e.Rule == "unknown_field" && e.Field == "invented_stray_field" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected unknown_field error from backlog_item overlay, got: %v", res.Errors)
	}
}

func TestGoValidator_StrictMode_DuplicateRefs(t *testing.T) {
	gv := NewGoValidator()
	ctx := pkgctx.NewSystemContext()

	// Intra-object duplicate across sibling ref fields: same ID in related_object_refs and dependencies
	obj := map[string]any{
		objects.FieldKeyID:            "BLI-TEST-002",
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         "Test item with duplicate refs",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     "2026-01-01T00:00:00Z",
		objects.FieldKeyCreatedBy:     "ACC-TEST",
		objects.FieldKeyUpdatedAt:     "2026-01-01T00:00:00Z",
		objects.FieldKeyUpdatedBy:     "ACC-TEST",
		objects.FieldKeyRelatedObjectRefs: []any{"BLI-TARGET-001"},
		objects.FieldKeyDependencies:      []any{"BLI-TARGET-001"},
	}

	resStrict, err := gv.Validate(ctx, obj, objects.KindBacklogItem, &ValidationOptions{
		StrictMode: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, e := range resStrict.Errors {
		if e.Rule == "intra_object_duplicate_ref" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected intra_object_duplicate_ref error under StrictMode, got: %v", resStrict.Errors)
	}

	// Duplicate within the same list
	objWithin := map[string]any{
		objects.FieldKeyID:            "BLI-TEST-003",
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         "Test item with duplicate within field",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     "2026-01-01T00:00:00Z",
		objects.FieldKeyCreatedBy:     "ACC-TEST",
		objects.FieldKeyUpdatedAt:     "2026-01-01T00:00:00Z",
		objects.FieldKeyUpdatedBy:     "ACC-TEST",
		objects.FieldKeyRelatedObjectRefs: []any{"BLI-TARGET-002", "BLI-TARGET-002"},
	}
	resWithin, err := gv.Validate(ctx, objWithin, objects.KindBacklogItem, &ValidationOptions{
		StrictMode: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	foundWithin := false
	for _, e := range resWithin.Errors {
		if e.Rule == "duplicate_reference" {
			foundWithin = true
			break
		}
	}
	if !foundWithin {
		t.Fatalf("expected duplicate_reference error under StrictMode, got: %v", resWithin.Errors)
	}
}

func TestCompose_Workflow_RefusesPercentComplete(t *testing.T) {
	gv := NewGoValidator()
	ctx := pkgctx.NewSystemContext()

	obj := map[string]any{
		objects.FieldKeyID:               "WFL-TEST-001",
		objects.FieldKeyKind:             objects.KindWorkflow,
		objects.FieldKeyTitle:            "Workflow with percent complete",
		objects.FieldKeyStatus:           objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:    objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:        "2026-01-01T00:00:00Z",
		objects.FieldKeyCreatedBy:        "ACC-TEST",
		objects.FieldKeyUpdatedAt:        "2026-01-01T00:00:00Z",
		objects.FieldKeyUpdatedBy:        "ACC-TEST",
		objects.FieldKeyPercentComplete:  100,
	}

	res, err := gv.Validate(ctx, obj, objects.KindWorkflow, &ValidationOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, e := range res.Errors {
		if e.Field == objects.FieldKeyPercentComplete {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error on percent_complete from workflow compose overlay, got: %v", res.Errors)
	}
}

func TestCompose_BacklogItem_RefusesDuplicateRefs(t *testing.T) {
	gv := NewGoValidator()
	ctx := pkgctx.NewSystemContext()

	obj := map[string]any{
		objects.FieldKeyID:            "BLI-TEST-004",
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         "Backlog item with duplicate refs",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     "2026-01-01T00:00:00Z",
		objects.FieldKeyCreatedBy:     "ACC-TEST",
		objects.FieldKeyUpdatedAt:     "2026-01-01T00:00:00Z",
		objects.FieldKeyUpdatedBy:     "ACC-TEST",
		objects.FieldKeyRelatedObjectRefs: []any{"BLI-DUP-001"},
		objects.FieldKeyDependencies:      []any{"BLI-DUP-001"},
	}

	res, err := gv.Validate(ctx, obj, objects.KindBacklogItem, &ValidationOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, e := range res.Errors {
		if e.Rule == "intra_object_duplicate_ref" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected intra_object_duplicate_ref error on backlog_item from compose overlay, got: %v", res.Errors)
	}
}
