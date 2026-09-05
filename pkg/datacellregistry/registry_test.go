package datacellregistry

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func findProjectRoot(t *testing.T) string {
	t.Helper()
	dir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found from test wd")
		}
		dir = parent
	}
}

func TestDataCellDescriptorsFromSpecIndex_roundTrip(t *testing.T) {
	t.Parallel()
	idx := &objects.SpecIndex{
		Kinds: map[string]objects.SpecKindSummary{
			"alpha": {Kind: "alpha", StorageProfile: "stream"},
			"beta":  {Kind: "beta", StorageProfile: ""},
		},
	}
	desc, err := DataCellDescriptorsFromSpecIndex(idx)
	if err != nil {
		t.Fatalf("DataCellDescriptorsFromSpecIndex: %v", err)
	}
	if len(desc) != 2 {
		t.Fatalf("len=%d", len(desc))
	}
	if desc[0].Kind != "alpha" || desc[0].CellID != "alpha" {
		t.Fatalf("alpha: %#v", desc[0])
	}
	if desc[0].StorageProfileWire != "stream" || desc[0].ParsedProfile != datacell.ProfileStream {
		t.Fatalf("alpha profile: %#v", desc[0])
	}
	if desc[1].Kind != "beta" || desc[1].HasStorageProfile() {
		t.Fatalf("beta: %#v", desc[1])
	}
}

func TestDataCellDescriptorsFromSpecIndex_invalidProfile(t *testing.T) {
	t.Parallel()
	idx := &objects.SpecIndex{
		Kinds: map[string]objects.SpecKindSummary{
			"bad": {Kind: "bad", StorageProfile: "not_a_profile"},
		},
	}
	_, err := DataCellDescriptorsFromSpecIndex(idx)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDataCellDescriptorsFromSpecIndex_allProfilesParse(t *testing.T) {
	t.Parallel()
	idx := &objects.SpecIndex{
		Kinds: map[string]objects.SpecKindSummary{
			"k_cas":   {Kind: "k_cas", StorageProfile: "cas_entity"},
			"k_light": {Kind: "k_light", StorageProfile: "light_file"},
			"k_str":   {Kind: "k_str", StorageProfile: "stream"},
		},
	}
	desc, err := DataCellDescriptorsFromSpecIndex(idx)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(desc) != 3 {
		t.Fatalf("len=%d", len(desc))
	}
	for _, d := range desc {
		if !d.ParsedProfile.IsKnown() && d.StorageProfileWire != "" {
			t.Fatalf("kind %q: profile not known", d.Kind)
		}
	}
}

func TestHighVolumeStreamSpecProfileMismatchesFromIndex(t *testing.T) {
	t.Parallel()
	idx := &objects.SpecIndex{
		Kinds: map[string]objects.SpecKindSummary{
			"good":    {Kind: "good", StorageProfile: "stream"},
			"wrong":   {Kind: "wrong", StorageProfile: "cas_entity"},
			"missing": {Kind: "missing", StorageProfile: "stream"},
		},
	}
	cfgKinds := []string{"good", "wrong", "nope", "good"}
	got := HighVolumeStreamSpecProfileMismatchesFromIndex(idx, cfgKinds)
	if len(got) != 2 {
		t.Fatalf("len=%d: %v", len(got), got)
	}
	if got[0] != `kind "nope" in stream config but missing from spec index` {
		t.Fatalf("first: %q", got[0])
	}
	if got[1] != `kind "wrong": spec storage_profile="cas_entity" want "stream"` {
		t.Fatalf("second: %q", got[1])
	}
}

func TestHighVolumeStreamSpecProfileMismatchesFromIndex_edges(t *testing.T) {
	t.Parallel()
	t.Run("nil index", func(t *testing.T) {
		t.Parallel()
		if got := HighVolumeStreamSpecProfileMismatchesFromIndex(nil, []string{"k"}); got != nil {
			t.Fatalf("nil index: got %#v", got)
		}
	})
	t.Run("nil kinds", func(t *testing.T) {
		t.Parallel()
		idx := &objects.SpecIndex{Kinds: map[string]objects.SpecKindSummary{
			"k": {Kind: "k", StorageProfile: "stream"},
		}}
		if got := HighVolumeStreamSpecProfileMismatchesFromIndex(idx, nil); got != nil {
			t.Fatalf("nil kinds: got %#v", got)
		}
	})
	t.Run("empty kinds", func(t *testing.T) {
		t.Parallel()
		idx := &objects.SpecIndex{Kinds: map[string]objects.SpecKindSummary{
			"k": {Kind: "k", StorageProfile: "stream"},
		}}
		if got := HighVolumeStreamSpecProfileMismatchesFromIndex(idx, []string{}); got != nil {
			t.Fatalf("empty kinds: got %#v", got)
		}
	})
	t.Run("aligned stream kind", func(t *testing.T) {
		t.Parallel()
		idx := &objects.SpecIndex{Kinds: map[string]objects.SpecKindSummary{
			"only": {Kind: "only", StorageProfile: "stream"},
		}}
		if got := HighVolumeStreamSpecProfileMismatchesFromIndex(idx, []string{"only"}); len(got) != 0 {
			t.Fatalf("want no mismatches, got %v", got)
		}
	})
}

func TestHighVolumeStreamSpecProfileMismatches_repo(t *testing.T) {
	t.Parallel()
	root := findProjectRoot(t)
	desc, err := LoadDataCellDescriptors(root)
	if err != nil {
		t.Fatalf("LoadDataCellDescriptors: %v", err)
	}
	stream := datacell.FilterDescriptorsByStorageProfile(desc, datacell.ProfileStream, false)
	if len(stream) < 2 {
		t.Fatalf("need at least 2 stream kinds in spec index, got %d", len(stream))
	}
	kinds := []string{stream[0].Kind, stream[1].Kind}
	m, err := HighVolumeStreamSpecProfileMismatches(root, kinds)
	if err != nil {
		t.Fatalf("HighVolumeStreamSpecProfileMismatches: %v", err)
	}
	if len(m) != 0 {
		t.Fatalf("expected no drift for sample stream kinds, got %v", m)
	}
}

func TestStreamKindNamesFromHighVolumeConfig_repo(t *testing.T) {
	t.Parallel()
	root := findProjectRoot(t)
	names, err := StreamKindNamesFromHighVolumeConfig(root)
	if err != nil {
		t.Fatalf("StreamKindNamesFromHighVolumeConfig: %v", err)
	}
	if len(names) < 3 {
		t.Fatalf("expected several stream kinds in repo config, got %d", len(names))
	}
}

func TestStreamKindNamesFromHighVolumeConfig_synthetic(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(cfgDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	yamlPath := filepath.Join(cfgDir, paths.HighVolumeKindsConfigFile)
	content := `kinds:
  - kind: audit_event
    storage: stream
  - kind: mcp_session
    storage: stream
  - kind: cas_only
    storage: cas_entity
  - kind: dup
    storage: stream
  - kind: dup
    storage: stream
`
	if err := fileutil.WriteFile(yamlPath, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	names, err := StreamKindNamesFromHighVolumeConfig(tmp)
	if err != nil {
		t.Fatalf("StreamKindNamesFromHighVolumeConfig: %v", err)
	}
	want := []string{"audit_event", "dup", "mcp_session"}
	if len(names) != len(want) {
		t.Fatalf("got len %d %v want %v", len(names), names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names[%d]=%q want %q (full %v)", i, names[i], want[i], names)
		}
	}
}

func TestStreamKindNamesFromHighVolumeConfig_missingFile(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	names, err := StreamKindNamesFromHighVolumeConfig(tmp)
	if err != nil {
		t.Fatalf("StreamKindNamesFromHighVolumeConfig: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("missing YAML: want empty list, got %#v", names)
	}
}

// TestHighVolumeStreamStewardshipDrift_synthetic_aligned ensures [HighVolumeStreamStewardshipDrift]
// is empty when high_volume_kinds stream entries match spec_index (storage_profile stream).
// Do not parallelize: [objects.RefreshMaterializedSpecIndex] uses the global spec loader.
func TestHighVolumeStreamStewardshipDrift_synthetic_aligned(t *testing.T) {
	tmp := t.TempDir()
	specsDir := filepath.Join(tmp, "docs", "process", "_internal", "object_specs")
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	specYAML := `ontology: hv_stream
schema_version: "2.0.0"
visibility: internal
storage_profile: stream
fields:
  id:
    type: string
`
	if err := fileutil.WriteFile(filepath.Join(specsDir, "hv_stream.yaml"), []byte(specYAML), paths.FilePerm644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	if _, err := objects.RefreshMaterializedSpecIndex(tmp); err != nil {
		t.Fatalf("RefreshMaterializedSpecIndex: %v", err)
	}
	cfgDir := filepath.Join(tmp, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(cfgDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	hv := `kinds:
  - kind: hv_stream
    storage: stream
`
	if err := fileutil.WriteFile(filepath.Join(cfgDir, paths.HighVolumeKindsConfigFile), []byte(hv), paths.FilePerm644); err != nil {
		t.Fatalf("write high_volume_kinds: %v", err)
	}
	drift, err := HighVolumeStreamStewardshipDrift(tmp)
	if err != nil {
		t.Fatalf("HighVolumeStreamStewardshipDrift: %v", err)
	}
	if len(drift) != 0 {
		t.Fatalf("want no drift, got %v", drift)
	}
}

// TestHighVolumeStreamStewardshipDrift_synthetic_missingKind reports a kind listed for stream in
// high_volume_kinds.yaml but absent from the materialized spec index.
func TestHighVolumeStreamStewardshipDrift_synthetic_missingKind(t *testing.T) {
	tmp := t.TempDir()
	specsDir := filepath.Join(tmp, "docs", "process", "_internal", "object_specs")
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	specYAML := `ontology: hv_stream
schema_version: "2.0.0"
visibility: internal
storage_profile: stream
fields:
  id:
    type: string
`
	if err := fileutil.WriteFile(filepath.Join(specsDir, "hv_stream.yaml"), []byte(specYAML), paths.FilePerm644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	if _, err := objects.RefreshMaterializedSpecIndex(tmp); err != nil {
		t.Fatalf("RefreshMaterializedSpecIndex: %v", err)
	}
	cfgDir := filepath.Join(tmp, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(cfgDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	hv := `kinds:
  - kind: hv_stream
    storage: stream
  - kind: only_in_yaml
    storage: stream
`
	if err := fileutil.WriteFile(filepath.Join(cfgDir, paths.HighVolumeKindsConfigFile), []byte(hv), paths.FilePerm644); err != nil {
		t.Fatalf("write high_volume_kinds: %v", err)
	}
	drift, err := HighVolumeStreamStewardshipDrift(tmp)
	if err != nil {
		t.Fatalf("HighVolumeStreamStewardshipDrift: %v", err)
	}
	if len(drift) != 1 {
		t.Fatalf("want 1 drift line, got %d: %v", len(drift), drift)
	}
	wantSub := `kind "only_in_yaml" in stream config but missing from spec index`
	if drift[0] != wantSub {
		t.Fatalf("drift[0]=%q want %q", drift[0], wantSub)
	}
}

// TestHighVolumeStreamStewardshipDrift_synthetic_wrongProfile reports a kind listed for stream in
// high_volume_kinds.yaml whose materialized spec_index storage_profile is not stream.
func TestHighVolumeStreamStewardshipDrift_synthetic_wrongProfile(t *testing.T) {
	tmp := t.TempDir()
	specsDir := filepath.Join(tmp, "docs", "process", "_internal", "object_specs")
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	specYAML := `ontology: hv_cas
schema_version: "2.0.0"
visibility: internal
storage_profile: cas_entity
fields:
  id:
    type: string
`
	if err := fileutil.WriteFile(filepath.Join(specsDir, "hv_cas.yaml"), []byte(specYAML), paths.FilePerm644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	if _, err := objects.RefreshMaterializedSpecIndex(tmp); err != nil {
		t.Fatalf("RefreshMaterializedSpecIndex: %v", err)
	}
	cfgDir := filepath.Join(tmp, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(cfgDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	hv := `kinds:
  - kind: hv_cas
    storage: stream
`
	if err := fileutil.WriteFile(filepath.Join(cfgDir, paths.HighVolumeKindsConfigFile), []byte(hv), paths.FilePerm644); err != nil {
		t.Fatalf("write high_volume_kinds: %v", err)
	}
	drift, err := HighVolumeStreamStewardshipDrift(tmp)
	if err != nil {
		t.Fatalf("HighVolumeStreamStewardshipDrift: %v", err)
	}
	if len(drift) != 1 {
		t.Fatalf("want 1 drift line, got %d: %v", len(drift), drift)
	}
	want := `kind "hv_cas": spec storage_profile="cas_entity" want "stream"`
	if drift[0] != want {
		t.Fatalf("drift[0]=%q want %q", drift[0], want)
	}
}

func TestHighVolumeStreamStewardshipDrift_repo(t *testing.T) {
	t.Parallel()
	root := findProjectRoot(t)
	drift, err := HighVolumeStreamStewardshipDrift(root)
	if err != nil {
		t.Fatalf("HighVolumeStreamStewardshipDrift: %v", err)
	}
	if len(drift) != 0 {
		t.Fatalf("expected no stewardship drift in healthy repo, got %v", drift)
	}
}

func TestLoadDataCellDescriptors_repo(t *testing.T) {
	t.Parallel()
	root := findProjectRoot(t)
	desc, err := LoadDataCellDescriptors(root)
	if err != nil {
		t.Fatalf("LoadDataCellDescriptors: %v", err)
	}
	if len(desc) < 5 {
		t.Fatalf("expected several kinds from repo spec index, got %d", len(desc))
	}
	stream := datacell.FilterDescriptorsByStorageProfile(desc, datacell.ProfileStream, false)
	if len(stream) == 0 {
		t.Fatal("expected at least one stream-backed kind in spec index")
	}
}

func TestLoadDescriptorReadModel_repo(t *testing.T) {
	t.Parallel()
	root := findProjectRoot(t)
	rm, err := LoadDescriptorReadModel(root)
	if err != nil {
		t.Fatalf("LoadDescriptorReadModel: %v", err)
	}
	if len(rm.Descriptors()) < 5 {
		t.Fatalf("expected several kinds, got %d", len(rm.Descriptors()))
	}
}

func TestInvalidateDescriptorReadModelCache(t *testing.T) {
	// Uses package-level projectDescriptorCaches; do not parallelize with other cache tests.
	root := findProjectRoot(t)
	rm, err := LoadDescriptorReadModel(root)
	if err != nil {
		t.Fatalf("LoadDescriptorReadModel: %v", err)
	}
	rev := rm.BuiltAtSpecCacheRevision()
	if rev == 0 {
		t.Skip("spec_index has no revision fields")
	}
	a, err := DescriptorReadModelForProject(root, rev)
	if err != nil {
		t.Fatalf("DescriptorReadModelForProject: %v", err)
	}
	InvalidateDescriptorReadModelCache(root)
	b, err := DescriptorReadModelForProject(root, rev)
	if err != nil {
		t.Fatalf("DescriptorReadModelForProject after invalidate: %v", err)
	}
	if a == b {
		t.Fatal("expected new snapshot after InvalidateDescriptorReadModelCache")
	}
}

func TestInvalidateDescriptorReadModelCache_equivalentProjectRoots(t *testing.T) {
	// Package-level cache; do not parallelize with other cache tests.
	// InvalidateDescriptorReadModelCache keys by filepath.Abs; trailing "/." must clear the same entry.
	root := findProjectRoot(t)
	rm, err := LoadDescriptorReadModel(root)
	if err != nil {
		t.Fatalf("LoadDescriptorReadModel: %v", err)
	}
	rev := rm.BuiltAtSpecCacheRevision()
	if rev == 0 {
		t.Skip("spec_index has no revision fields")
	}
	a, err := DescriptorReadModelForProject(root, rev)
	if err != nil {
		t.Fatalf("DescriptorReadModelForProject: %v", err)
	}
	// Path forms that resolve to the same directory (Abs) must share one cache key.
	equivRoot := root + string(filepath.Separator) + "."
	InvalidateDescriptorReadModelCache(equivRoot)
	b, err := DescriptorReadModelForProject(root, rev)
	if err != nil {
		t.Fatalf("DescriptorReadModelForProject after invalidate via equivalent root: %v", err)
	}
	if a == b {
		t.Fatalf("expected cache cleared when invalidating equivalent root %q (canonical %q)", equivRoot, root)
	}
}

func TestDescriptorReadModelForProject_cacheHit(t *testing.T) {
	// Uses package-level projectDescriptorCaches; do not parallelize with TestInvalidateDescriptorReadModelCache.
	root := findProjectRoot(t)
	rm, err := LoadDescriptorReadModel(root)
	if err != nil {
		t.Fatalf("LoadDescriptorReadModel: %v", err)
	}
	rev := rm.BuiltAtSpecCacheRevision()
	if rev == 0 {
		t.Skip("spec_index has no builder/global revision fields")
	}
	a, err := DescriptorReadModelForProject(root, rev)
	if err != nil {
		t.Fatalf("DescriptorReadModelForProject: %v", err)
	}
	b, err := DescriptorReadModelForProject(root, rev)
	if err != nil {
		t.Fatalf("DescriptorReadModelForProject: %v", err)
	}
	if a != b {
		t.Fatal("expected same cached DescriptorReadModel instance")
	}
}

// TestDescriptorReadModelForProject_reloadsWhenSpecRevisionAdvances ensures the per-project
// descriptor cache follows [datacell.ReadModelIsStale]: when the materialized spec_index revision
// advances (here via [objects.RefreshMaterializedSpecIndex]), the next [DescriptorReadModelForProject]
// with the new generation reloads from disk without requiring [InvalidateDescriptorReadModelCache].
//
// Do not parallelize: [objects.RefreshMaterializedSpecIndex] clears the global spec loader cache.
func TestDescriptorReadModelForProject_reloadsWhenSpecRevisionAdvances(t *testing.T) {
	tmp := t.TempDir()
	specsDir := filepath.Join(tmp, "docs", "process", "_internal", "object_specs")
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeSpec := func(fileName, ontology string) {
		t.Helper()
		content := "ontology: " + ontology + "\nschema_version: \"2.0.0\"\nvisibility: internal\nfields:\n  id:\n    type: string\n"
		if err := fileutil.WriteFile(filepath.Join(specsDir, fileName), []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("write %s: %v", fileName, err)
		}
	}
	writeSpec("kind_one.yaml", "kind_one")

	idx1, err := objects.RefreshMaterializedSpecIndex(tmp)
	if err != nil {
		t.Fatalf("RefreshMaterializedSpecIndex: %v", err)
	}
	rev1 := datacell.MaxSpecRevision(idx1.BuilderSpecCacheRevision, idx1.GlobalSpecCacheRevision)

	rm1, err := DescriptorReadModelForProject(tmp, rev1)
	if err != nil {
		t.Fatalf("DescriptorReadModelForProject: %v", err)
	}
	n1 := len(rm1.Descriptors())

	writeSpec("kind_two.yaml", "kind_two")
	idx2, err := objects.RefreshMaterializedSpecIndex(tmp)
	if err != nil {
		t.Fatalf("RefreshMaterializedSpecIndex (second): %v", err)
	}
	rev2 := datacell.MaxSpecRevision(idx2.BuilderSpecCacheRevision, idx2.GlobalSpecCacheRevision)
	if rev2 == rev1 {
		t.Fatalf("expected spec revision to advance after second materialize, got %d both times", rev1)
	}

	rm2, err := DescriptorReadModelForProject(tmp, rev2)
	if err != nil {
		t.Fatalf("DescriptorReadModelForProject (rev2): %v", err)
	}
	if len(rm2.Descriptors()) != n1+1 {
		t.Fatalf("descriptors: got %d want %d (after adding one kind)", len(rm2.Descriptors()), n1+1)
	}
	a, err := DescriptorReadModelForProject(tmp, rev2)
	if err != nil {
		t.Fatalf("DescriptorReadModelForProject (rev2 cache hit): %v", err)
	}
	b, err := DescriptorReadModelForProject(tmp, rev2)
	if err != nil {
		t.Fatalf("DescriptorReadModelForProject (rev2 second): %v", err)
	}
	if a != b {
		t.Fatal("expected same cached DescriptorReadModel instance for same rev")
	}
}

func TestDescriptorForKind(t *testing.T) {
	t.Parallel()
	desc := []datacell.CellKindDescriptor{
		{Kind: "a", CellID: "a", StorageProfileWire: "stream", ParsedProfile: datacell.ProfileStream},
		{Kind: "b", CellID: "b"},
	}
	got, ok := DescriptorForKind(desc, "a")
	if !ok || got.Kind != "a" || got.ParsedProfile != datacell.ProfileStream {
		t.Fatalf("a: %#v ok=%v", got, ok)
	}
	_, ok = DescriptorForKind(desc, "missing")
	if ok {
		t.Fatal("expected false")
	}
}

func TestIndexDescriptorsByKind(t *testing.T) {
	t.Parallel()
	desc := []datacell.CellKindDescriptor{
		{Kind: "first", CellID: "first", StorageProfileWire: "cas_entity", ParsedProfile: datacell.ProfileCASEntity},
		{Kind: "second", CellID: "second"},
	}
	m := IndexDescriptorsByKind(desc)
	if len(m) != 2 {
		t.Fatalf("len=%d", len(m))
	}
	if m["first"].StorageProfileWire != "cas_entity" {
		t.Fatalf("%#v", m["first"])
	}
	// duplicate kind: last wins
	desc2 := []datacell.CellKindDescriptor{
		{Kind: "k", CellID: "k", StorageProfileWire: "stream", ParsedProfile: datacell.ProfileStream},
		{Kind: "k", CellID: "k", StorageProfileWire: "cas_entity", ParsedProfile: datacell.ProfileCASEntity},
	}
	m2 := IndexDescriptorsByKind(desc2)
	if m2["k"].ParsedProfile != datacell.ProfileCASEntity {
		t.Fatalf("last should win: %#v", m2["k"])
	}
}
