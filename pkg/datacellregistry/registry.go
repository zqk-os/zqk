// Package datacellregistry bridges the spec plane (objects.SpecIndex) and pkg/datacell identity types
// without an import cycle: pkg/objects imports pkg/datacell, so registry loading lives here.
//
// Use [LoadDataCellDescriptors] or [DataCellDescriptorsFromSpecIndex] to list cells, then
// [DescriptorForKind] or [IndexDescriptorsByKind] to resolve by kind name.
package datacellregistry

import (
	"fmt"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// DataCellDescriptorsFromSpecIndex returns one descriptor per kind in the materialized spec index,
// including storage_profile when set (inherited along extends). CellID is the kind string for v1.
func DataCellDescriptorsFromSpecIndex(idx *objects.SpecIndex) ([]datacell.CellKindDescriptor, error) {
	if idx == nil || idx.Kinds == nil {
		return nil, nil
	}
	kindNames := objects.GetAllKindsFromIndex(idx)
	out := make([]datacell.CellKindDescriptor, 0, len(kindNames))
	for _, k := range kindNames {
		ks, ok := idx.GetKindSummary(k)
		if !ok {
			continue
		}
		wire := ks.StorageProfile
		var parsed datacell.StorageProfile
		if wire != "" {
			p, err := datacell.ParseStorageProfile(wire)
			if err != nil {
				return nil, errfmt.Errorf("kind %q storage_profile: %w", k, err)
			}
			parsed = p
		}
		d := datacell.CellKindDescriptor{
			Kind:               k,
			CellID:             k,
			StorageProfileWire: wire,
			ParsedProfile:      parsed,
		}
		if err := d.ValidateV1Identity(); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	// Deterministic order (GetAllKindsFromIndex already sorts)
	return out, nil
}

// DescriptorForKind returns the descriptor for a spec-backed kind name, or false if absent.
func DescriptorForKind(desc []datacell.CellKindDescriptor, kind string) (datacell.CellKindDescriptor, bool) {
	for i := range desc {
		if desc[i].Kind == kind {
			return desc[i], true
		}
	}
	return datacell.CellKindDescriptor{}, false
}

// IndexDescriptorsByKind builds a map from kind name to descriptor for O(1) lookups.
// If desc contains duplicate Kind entries, the last entry wins (the spec index does not duplicate kinds).
func IndexDescriptorsByKind(desc []datacell.CellKindDescriptor) map[string]datacell.CellKindDescriptor {
	return datacell.MapDescriptorsByKind(desc)
}

func loadSpecIndexForDataCell(projectRoot string) (*objects.SpecIndex, error) {
	indexPath := filepath.Join(projectRoot, paths.ProcessInternalDir, "spec_index.json")
	idx, err := objects.LoadSpecIndex(indexPath)
	if err != nil {
		specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
		idx, err = objects.BuildSpecIndexFromSpecsDir(specsDir)
		if err != nil {
			return nil, errfmt.Newf("load spec index (spec_index.json and object_specs)").Wrap(err)
		}
	}
	return idx, nil
}

// LoadDataCellDescriptors loads docs/process/_internal/spec_index.json when present; otherwise
// builds from docs/process/_internal/object_specs (same fallback pattern as tooling that needs an index).
func LoadDataCellDescriptors(projectRoot string) ([]datacell.CellKindDescriptor, error) {
	if projectRoot == "" {
		return nil, errfmt.Errorf("project root is required")
	}
	idx, err := loadSpecIndexForDataCell(projectRoot)
	if err != nil {
		return nil, err
	}
	return DataCellDescriptorsFromSpecIndex(idx)
}

// LoadDescriptorReadModel loads cell descriptors and correlates them with the materialized
// spec_index revision fields (BuilderSpecCacheRevision / GlobalSpecCacheRevision). Compare
// [datacell.DescriptorReadModel.BuiltAtSpecCacheRevision] to [objects.SpecLoader.SpecCacheRevision]
// for staleness (see [datacell.ReadModelIsStale]).
func LoadDescriptorReadModel(projectRoot string) (*datacell.DescriptorReadModel, error) {
	if projectRoot == "" {
		return nil, errfmt.Errorf("project root is required")
	}
	idx, err := loadSpecIndexForDataCell(projectRoot)
	if err != nil {
		return nil, err
	}
	desc, err := DataCellDescriptorsFromSpecIndex(idx)
	if err != nil {
		return nil, err
	}
	rev := datacell.MaxSpecRevision(idx.BuilderSpecCacheRevision, idx.GlobalSpecCacheRevision)
	return datacell.NewDescriptorReadModel(desc, rev)
}

// HighVolumeStreamSpecProfileMismatchesFromIndex reports kinds in streamEnabledKinds that are missing
// from the index or whose storage_profile is not stream. Empty means the spec plane agrees with the
// stream-config list for those kinds (see objects.TestSpecIndexStorageProfileMatchesHighVolumeStreamKinds).
func HighVolumeStreamSpecProfileMismatchesFromIndex(idx *objects.SpecIndex, streamEnabledKinds []string) []string {
	if idx == nil || len(streamEnabledKinds) == 0 {
		return nil
	}
	want := string(datacell.ProfileStream)
	seen := make(map[string]bool, len(streamEnabledKinds))
	var out []string
	for _, kind := range streamEnabledKinds {
		if kind == "" || seen[kind] {
			continue
		}
		seen[kind] = true
		ks, ok := idx.GetKindSummary(kind)
		if !ok {
			out = append(out, fmt.Sprintf("kind %q in stream config but missing from spec index", kind))
			continue
		}
		if ks.StorageProfile != want {
			out = append(out, fmt.Sprintf("kind %q: spec storage_profile=%q want %q", kind, ks.StorageProfile, want))
		}
	}
	slices.Sort(out)
	return out
}

// HighVolumeStreamSpecProfileMismatches loads the spec index from projectRoot and runs
// HighVolumeStreamSpecProfileMismatchesFromIndex. Used by stream stewardship and operational checks.
func HighVolumeStreamSpecProfileMismatches(projectRoot string, streamEnabledKinds []string) ([]string, error) {
	if projectRoot == "" {
		return nil, errfmt.Errorf("project root is required")
	}
	idx, err := loadSpecIndexForDataCell(projectRoot)
	if err != nil {
		return nil, err
	}
	return HighVolumeStreamSpecProfileMismatchesFromIndex(idx, streamEnabledKinds), nil
}

// highVolumeKindsYAML is the subset of docs/process/_internal/configs/high_volume_kinds.yaml we need.
type highVolumeKindsYAML struct {
	Kinds []struct {
		Kind    string `yaml:"kind"`
		Storage string `yaml:"storage"`
	} `yaml:"kinds"`
}

// StreamKindNamesFromHighVolumeConfig returns kind names that declare storage: stream in high_volume_kinds.yaml.
// If the config file is absent, returns (nil, nil). Other read/parse errors are returned.
func StreamKindNamesFromHighVolumeConfig(projectRoot string) ([]string, error) {
	if projectRoot == "" {
		return nil, errfmt.Errorf("project root is required")
	}
	p := filepath.Join(projectRoot, paths.ProcessInternalConfigsDir, paths.HighVolumeKindsConfigFile)
	if _, err := fileutil.Stat(p); err != nil {
		if fileutil.IsNotExist(err) {
			return nil, nil
		}
		return nil, errfmt.Newf("stat %s", p).Wrap(err)
	}
	data, err := fileutil.ReadFile(p)
	if err != nil {
		return nil, errfmt.Newf("read %s", p).Wrap(err)
	}
	var cfg highVolumeKindsYAML
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, errfmt.Newf("parse %s", p).Wrap(err)
	}
	seen := make(map[string]bool)
	var out []string
	for _, e := range cfg.Kinds {
		if e.Kind == "" || e.Storage != "stream" {
			continue
		}
		if seen[e.Kind] {
			continue
		}
		seen[e.Kind] = true
		out = append(out, e.Kind)
	}
	slices.Sort(out)
	return out, nil
}

// HighVolumeStreamStewardshipDrift reports spec_index vs high_volume_kinds stream entries (YAML-listed kinds
// must exist in the index with storage_profile stream). Empty means no drift. Missing YAML is treated as no check.
func HighVolumeStreamStewardshipDrift(projectRoot string) ([]string, error) {
	kinds, err := StreamKindNamesFromHighVolumeConfig(projectRoot)
	if err != nil {
		return nil, err
	}
	if len(kinds) == 0 {
		return nil, nil
	}
	return HighVolumeStreamSpecProfileMismatches(projectRoot, kinds)
}
