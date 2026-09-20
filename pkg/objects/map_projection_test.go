package objects

import (
	"testing"
)

func TestPickTopLevelFields(t *testing.T) {
	src := map[string]any{
		FieldKeyID:    "X-1",
		FieldKeyKind:  KindBacklogItem,
		FieldKeyTitle: "t",
		"extra":       99,
	}
	got := PickTopLevelFields(src, []string{FieldKeyID, FieldKeyTitle, FieldKeyID})
	if len(got) != 2 || got[FieldKeyID] != "X-1" || got[FieldKeyTitle] != "t" {
		t.Fatalf("%#v", got)
	}
	if PickTopLevelFields(src, nil) != nil {
		t.Fatal("nil keys")
	}
}

func TestPickDotPaths_nested(t *testing.T) {
	src := map[string]any{
		FieldKeyID: "o-1",
		FieldKeyMetadata: map[string]any{
			FieldKeyVersion: float64(2),
			"nested": map[string]any{
				"k": "v",
			},
		},
	}
	got := PickDotPaths(src, []string{FieldKeyID, "metadata.version", "metadata.nested.k"})
	if got[FieldKeyID] != "o-1" {
		t.Fatalf("id: %#v", got)
	}
	meta, ok := got[FieldKeyMetadata].(map[string]any)
	if !ok {
		t.Fatalf("metadata: %#v", got)
	}
	if meta[FieldKeyVersion] != float64(2) {
		t.Fatalf("version: %#v", meta)
	}
	nested, ok := meta["nested"].(map[string]any)
	if !ok || nested["k"] != "v" {
		t.Fatalf("nested: %#v", got)
	}
}

func TestPickDotPaths_skipsMissing(t *testing.T) {
	src := map[string]any{FieldKeyID: "a"}
	got := PickDotPaths(src, []string{"nope.deep"})
	if got != nil {
		t.Fatalf("want nil when no path resolves, got %#v", got)
	}
}
