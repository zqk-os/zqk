package storage

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TRACK: BLI-1786689721908382000-6402a858

func TestOmitDraftPlaneOnlyFromList_NilAndEmpty(t *testing.T) {
	t.Parallel()
	f := &FileObjectStorage{projectRoot: t.TempDir()}

	// nil result
	f.omitDraftPlaneOnlyFromList(nil)

	// empty result
	res := &QueryResult{Objects: []map[string]any{}}
	f.omitDraftPlaneOnlyFromList(res)
	if len(res.Objects) != 0 {
		t.Fatalf("expected 0 objects, got %d", len(res.Objects))
	}
}

func TestListedObjectIsDraftPlaneOnly_NilAndMalformed(t *testing.T) {
	t.Parallel()
	f := &FileObjectStorage{projectRoot: t.TempDir()}

	if f.listedObjectIsDraftPlaneOnly(nil) {
		t.Fatal("expected false for nil object")
	}
	if f.listedObjectIsDraftPlaneOnly(map[string]any{}) {
		t.Fatal("expected false for empty map")
	}
	if f.listedObjectIsDraftPlaneOnly(map[string]any{objects.FieldKeyID: "SOME-ID"}) {
		t.Fatal("expected false for missing kind")
	}
	if f.listedObjectIsDraftPlaneOnly(map[string]any{objects.FieldKeyKind: objects.KindBacklogItem}) {
		t.Fatal("expected false for missing id")
	}
}

func TestDropDraftPlaneOnlyIDs_Empty(t *testing.T) {
	t.Parallel()
	f := &FileObjectStorage{projectRoot: t.TempDir()}

	f.dropDraftPlaneOnlyIDs(objects.KindBacklogItem, nil, nil)
	idSet := map[string]bool{}
	f.dropDraftPlaneOnlyIDs(objects.KindBacklogItem, idSet, map[string]bool{})
	if len(idSet) != 0 {
		t.Fatalf("expected empty set, got %v", idSet)
	}
}
