package storage

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestReferenceStringMatchesObjectID(t *testing.T) {
	if !ReferenceStringMatchesObjectID("BLI-1", "BLI-1") {
		t.Fatal("bare id")
	}
	if !ReferenceStringMatchesObjectID("backlog_item:BLI-1", "BLI-1") {
		t.Fatal("kind prefix")
	}
	if ReferenceStringMatchesObjectID("BLI-2", "BLI-1") {
		t.Fatal("mismatch")
	}
}

func TestStripReferenceFieldsRemovingID(t *testing.T) {
	obj := map[string]any{
		objects.FieldKeyID:           "REQ-1",
		objects.FieldKeyKind:         "requirement",
		objects.FieldKeyCriteriaRefs: []any{"CRIT-1", "CRIT-2"},
		"strategic_plan_ref":         "STRAT-1",
		objects.FieldKeyCommitRefs:   []any{"abc123"},
	}
	updates := StripReferenceFieldsRemovingID(obj, "CRIT-1")
	if updates == nil {
		t.Fatal("expected updates")
	}
	refs, _ := updates[objects.FieldKeyCriteriaRefs].([]any)
	if len(refs) != 1 || refs[0] != "CRIT-2" {
		t.Fatalf("criteria_refs: %#v", updates[objects.FieldKeyCriteriaRefs])
	}
	if _, ok := updates["strategic_plan_ref"]; ok {
		t.Fatalf("unexpected strategic_plan change: %#v", updates["strategic_plan_ref"])
	}
	if _, ok := updates[objects.FieldKeyCommitRefs]; ok {
		t.Fatal("commit_refs must not be stripped")
	}
}

type mockStorageForUnlinkTest struct {
	ObjectStorageProvider
	objects map[string]map[string]any
}

func (m *mockStorageForUnlinkTest) Read(_ context.Context, _ *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.objects[id]; ok {
		return obj, nil
	}
	return nil, ErrObjectNotFound
}

func (m *mockStorageForUnlinkTest) Update(_ context.Context, _ *pkgctx.SecurityContext, id string, updates map[string]any) error {
	if obj, ok := m.objects[id]; ok {
		for k, v := range updates {
			obj[k] = v
		}
		return nil
	}
	return ErrObjectNotFound
}

func TestUnlinkReferencesFromDependents_RefusesArchivedCriteriaLineageUnderCompleteBLI(t *testing.T) {
	mockStore := &mockStorageForUnlinkTest{
		objects: map[string]map[string]any{
			"BLI-COMPLETE-1": {
				objects.FieldKeyID:           "BLI-COMPLETE-1",
				objects.FieldKeyKind:         objects.KindBacklogItem,
				objects.FieldKeyStatus:       objects.ObjectStatusComplete,
				objects.FieldKeyCriteriaRefs: []any{"CRIT-1"},
			},
		},
	}
	err := UnlinkReferencesFromDependents(context.Background(), pkgctx.NewSystemSecurityContext(), mockStore, "CRIT-1", []string{"BLI-COMPLETE-1"})
	if err == nil {
		t.Fatal("expected error refusing to unlink criteria from complete BLI, got nil")
	}
}

func TestUnlinkWouldStripArchivedCriteriaLineage_AllowsInProgress(t *testing.T) {
	dep := map[string]any{
		objects.FieldKeyID:     "BLI-WIP",
		objects.FieldKeyKind:   objects.KindBacklogItem,
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
	}
	if err := UnlinkWouldStripArchivedCriteriaLineage(dep, "CRIT-1"); err != nil {
		t.Fatalf("in_progress BLI should allow CRIT unlink, got %v", err)
	}
}
