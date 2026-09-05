package objectget

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestResolvedSidecarRoundTrip(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	obj := map[string]any{
		objects.FieldKeyID:                   "BLI-test-1",
		objects.FieldKeyKind:                 objects.KindBacklogItem,
		objects.FieldKeyTitle:                "raw",
		"resolved_priority_plan_ref":         map[string]any{objects.FieldKeyID: "PRI-1", objects.FieldKeyTitle: "P"},
		"reference_resolver_overlay_applied": true,
	}
	path, err := WriteResolvedSidecar(root, obj, "lazy")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fileutil.Stat(path); err != nil {
		t.Fatal(err)
	}
	// CAS-like raw must not include overlay after strip
	raw := map[string]any{
		objects.FieldKeyID:    "BLI-test-1",
		objects.FieldKeyKind:  objects.KindBacklogItem,
		objects.FieldKeyTitle: "raw",
	}
	sc, err := ReadResolvedSidecar(root, objects.KindBacklogItem, "BLI-test-1")
	if err != nil {
		t.Fatal(err)
	}
	if sc.Hydration != "lazy" || len(sc.Overlay) != 2 {
		t.Fatalf("sidecar=%+v", sc)
	}
	merged := MergeOverlayOnto(raw, sc.Overlay)
	if merged[objects.FieldKeyTitle] != "raw" {
		t.Fatalf("raw title lost")
	}
	if merged["reference_resolver_overlay_applied"] != true {
		t.Fatalf("overlay missing")
	}
	// path uses 2-hex shard
	if filepath.Base(filepath.Dir(path)) == objects.KindBacklogItem {
		t.Fatalf("expected shard dir under kind, path=%s", path)
	}
}

func TestExtractOverlayFields(t *testing.T) {
	t.Parallel()
	got := ExtractOverlayFields(map[string]any{
		objects.FieldKeyTitle:               "x",
		objects.FieldKeyResolvedAt:          "2020-01-01", // durable, not overlay
		"resolved_goal_ref":                 "G",
		"reference_resolver_overlay_capped": true,
	})
	if _, ok := got[objects.FieldKeyResolvedAt]; ok {
		t.Fatal("resolved_at must not be treated as overlay")
	}
	if len(got) != 2 {
		t.Fatalf("got=%v", got)
	}
}
