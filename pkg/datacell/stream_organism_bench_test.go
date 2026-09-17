// Benchmarks for data-cell / stream organism hot paths (synthetic spec_index, envelope map).
// Use -run '^$' for benchmarks only (avoids running package tests first).
// Run: go test ./pkg/datacell -run '^$' -bench 'BenchmarkStreamOrganism' -benchmem -count=3 -timeout 60s
package datacell_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/datacellregistry"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func BenchmarkStreamOrganism_OperationalEnvelopeForProfile(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, _ = datacell.OperationalEnvelopeForProfile(datacell.ProfileStream)
	}
}

func BenchmarkStreamOrganism_OperationalEnvelope_CompactSummary(b *testing.B) {
	env, ok := datacell.OperationalEnvelopeForProfile(datacell.ProfileCASEntity)
	if !ok {
		b.Fatal("expected cas_entity envelope")
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = env.CompactSummary()
	}
}

func BenchmarkStreamOrganism_LoadDataCellDescriptors(b *testing.B) {
	root := b.TempDir()
	internal := filepath.Join(root, paths.ProcessInternalDir)
	if err := fileutil.EnsureDir(internal); err != nil {
		b.Fatal(err)
	}
	idx := &objects.SpecIndex{
		Kinds: map[string]objects.SpecKindSummary{
			"bench_stream": {Kind: "bench_stream", StorageProfile: "stream"},
			"bench_cas":    {Kind: "bench_cas", StorageProfile: "cas_entity"},
		},
	}
	data, err := json.Marshal(idx)
	if err != nil {
		b.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(internal, "spec_index.json"), data); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		_, err := datacellregistry.LoadDataCellDescriptors(root)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStreamOrganism_DataCellDescriptorsFromSpecIndex measures CPU-only work:
// DataCellDescriptorsFromSpecIndex after the spec index is already in memory.
// Contrast with BenchmarkStreamOrganism_LoadDataCellDescriptors (disk read + JSON + same transform).
func BenchmarkStreamOrganism_DataCellDescriptorsFromSpecIndex(b *testing.B) {
	idx := &objects.SpecIndex{
		Kinds: map[string]objects.SpecKindSummary{
			"bench_stream": {Kind: "bench_stream", StorageProfile: "stream"},
			"bench_cas":    {Kind: "bench_cas", StorageProfile: "cas_entity"},
		},
	}
	b.ReportAllocs()
	for b.Loop() {
		_, err := datacellregistry.DataCellDescriptorsFromSpecIndex(idx)
		if err != nil {
			b.Fatal(err)
		}
	}
}
