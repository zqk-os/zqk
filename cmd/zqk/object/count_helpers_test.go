package object

import (
	"testing"
)

func TestBuildAllKindsCountMeta_DocumentsExclusions(t *testing.T) {
	t.Parallel()
	hv := hvInventoryRollup{Stream: 10, CASDisk: 5, Orphan: 2}
	meta := buildAllKindsCountMeta(NamespaceQueryScope{}, false, 100,
		[]string{"glossary_term", "agent_task"}, nil, 3, false, hv)
	if meta["visibility_scope"] != "public_kinds_only" {
		t.Fatalf("visibility_scope=%v", meta["visibility_scope"])
	}
	if meta["skipped_internal_kind_count"] != 2 {
		t.Fatalf("skipped_internal_kind_count=%v", meta["skipped_internal_kind_count"])
	}
	if meta["omitted_zero_count_kinds"] != 3 {
		t.Fatalf("omitted_zero=%v", meta["omitted_zero_count_kinds"])
	}
	if _, ok := meta["intentional_exclusions"].(string); !ok {
		t.Fatal("missing intentional_exclusions")
	}
	if _, ok := meta["find_vs_count_recipe"].(string); !ok {
		t.Fatal("missing find_vs_count_recipe")
	}
	hvMap, ok := meta["high_volume_object_counts"].(map[string]any)
	if !ok {
		t.Fatal("missing high_volume_object_counts")
	}
	if hvMap["stream"] != 10 || hvMap["cas_disk"] != 5 || hvMap["orphan"] != 2 {
		t.Fatalf("hv rollup=%v", hvMap)
	}
}

func TestAllKindsCountScopeNote(t *testing.T) {
	t.Parallel()
	pub := allKindsCountScopeNote(false)
	if pub == "" || len(pub) < 20 {
		t.Fatal("public scope note too short")
	}
	elev := allKindsCountScopeNote(true)
	if elev == pub {
		t.Fatal("elevated scope note should differ")
	}
}
