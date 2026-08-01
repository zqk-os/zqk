// Stream / data-cell organism — isolated end-to-end checks on a synthetic project layout (no migration of real docs/process data).
// Exercises: materialized spec_index.json → datacellregistry, high-volume stream vs spec alignment,
// operational envelope + scheduler policy dry-run, runtime organism paths + manifest.
//
// Run: go test ./pkg/datacell -timeout 60s -run TestStreamOrganism -count=1
package datacell_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/datacellregistry"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/scheduler"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

const (
	streamOrganismKind = "organism_stream_kind"
	casOrganismKind    = "organism_cas_kind"
)

func TestStreamOrganism_OperationalEnvelopeCoversKnownProfiles(t *testing.T) {
	t.Parallel()
	for _, p := range datacell.KnownStorageProfiles {
		env, ok := datacell.OperationalEnvelopeForProfile(p)
		if !ok {
			t.Fatalf("profile %q: missing operational envelope", p)
		}
		if env.SchedulerCategory != datacell.SchedulerCategoryDataCellEnvelope {
			t.Fatalf("profile %q: scheduler_category %q", p, env.SchedulerCategory)
		}
		if env.CompactSummary() == "" {
			t.Fatalf("profile %q: empty CompactSummary", p)
		}
	}
}

func TestStreamOrganism_FilterDescriptorsByStorageProfile(t *testing.T) {
	t.Parallel()
	root := writeSyntheticDataCellProject(t)
	desc, err := datacellregistry.LoadDataCellDescriptors(root)
	if err != nil {
		t.Fatalf("LoadDataCellDescriptors: %v", err)
	}
	stream := datacell.FilterDescriptorsByStorageProfile(desc, datacell.ProfileStream, false)
	if len(stream) != 1 || stream[0].Kind != streamOrganismKind {
		t.Fatalf("stream filter: %#v", stream)
	}
	cas := datacell.FilterDescriptorsByStorageProfile(desc, datacell.ProfileCASEntity, false)
	if len(cas) != 1 || cas[0].Kind != casOrganismKind {
		t.Fatalf("cas filter: %#v", cas)
	}
}

func TestStreamOrganism_ContractMatchesDescriptorProfiles(t *testing.T) {
	t.Parallel()
	root := writeSyntheticDataCellProject(t)
	desc, err := datacellregistry.LoadDataCellDescriptors(root)
	if err != nil {
		t.Fatalf("LoadDataCellDescriptors: %v", err)
	}
	for _, d := range desc {
		if !d.HasStorageProfile() {
			t.Fatalf("synthetic kinds should have storage_profile: %#v", d)
		}
		if _, ok := datacell.ContractForProfile(d.ParsedProfile); !ok {
			t.Fatalf("kind %q: no contract for profile %v", d.Kind, d.ParsedProfile)
		}
	}
}

func TestStreamOrganism_SpecIndexRegistryAndStreamAlignment(t *testing.T) {
	t.Parallel()
	root := writeSyntheticDataCellProject(t)

	desc, err := datacellregistry.LoadDataCellDescriptors(root)
	if err != nil {
		t.Fatalf("LoadDataCellDescriptors: %v", err)
	}
	if len(desc) < 2 {
		t.Fatalf("expected at least 2 kinds, got %d", len(desc))
	}
	byKind := datacellregistry.IndexDescriptorsByKind(desc)
	sk, ok := byKind[streamOrganismKind]
	if !ok || sk.ParsedProfile != datacell.ProfileStream {
		t.Fatalf("stream kind: %#v ok=%v", sk, ok)
	}
	ck, ok := byKind[casOrganismKind]
	if !ok || ck.ParsedProfile != datacell.ProfileCASEntity {
		t.Fatalf("cas kind: %#v ok=%v", ck, ok)
	}

	mis, err := datacellregistry.HighVolumeStreamSpecProfileMismatches(root, []string{streamOrganismKind})
	if err != nil {
		t.Fatalf("HighVolumeStreamSpecProfileMismatches: %v", err)
	}
	if len(mis) != 0 {
		t.Fatalf("expected no spec/stream drift for aligned index, got %v", mis)
	}

	wantStreamPath := datacell.CellStreamOverlayKindDir(root, streamOrganismKind)
	if got := datacell.CellStreamOverlayKindDir(root, byKind[streamOrganismKind].Kind); got != wantStreamPath {
		t.Fatalf("stream primary path: got %q want %q", got, wantStreamPath)
	}

	env, ok := datacell.OperationalEnvelopeForProfile(datacell.ProfileStream)
	if !ok || env.SchedulerCategory != datacell.SchedulerCategoryDataCellEnvelope {
		t.Fatalf("stream envelope: %#v ok=%v", env, ok)
	}
	rep, err := scheduler.DryRunDataCellEnvelopePolicy(root, datacell.ProfileStream)
	if err != nil {
		t.Fatalf("DryRunDataCellEnvelopePolicy: %v", err)
	}
	if rep.PolicyAction != "execute" {
		t.Fatalf("policy: %+v", rep)
	}
}

func TestStreamOrganism_RuntimeOrganismPathsAndManifest(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cfgDir := filepath.Join(root, paths.ProjectDataDir, paths.ConfigDir)
	if err := fileutil.EnsureDir(cfgDir); err != nil {
		t.Fatal(err)
	}
	ff := filepath.Join(cfgDir, paths.FeatureFlagsFile)
	if err := fileutil.WriteSecureFile(ff, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(cfgDir, paths.DataCellRuntimeManifestFile)
	manifest := `{"protocol_version":"1"}
`
	if err := fileutil.WriteSecureFile(manifestPath, []byte(manifest)); err != nil {
		t.Fatal(err)
	}
	rp := datacell.AllRuntimePaths(root)
	if rp.FeatureFlags != ff {
		t.Fatalf("feature flags path: %q vs %q", rp.FeatureFlags, ff)
	}
	m, err := datacell.ReadRuntimeManifest(root)
	if err != nil {
		t.Fatalf("ReadRuntimeManifest: %v", err)
	}
	if datacell.EffectiveProtocolVersion(m) != datacell.ProtocolVersion {
		t.Fatalf("protocol: got %q want %q", datacell.EffectiveProtocolVersion(m), datacell.ProtocolVersion)
	}
}

func TestStreamOrganism_HighVolumeKindsYAMLAgreesWithSpecIndex(t *testing.T) {
	t.Parallel()
	root := writeSyntheticDataCellProject(t)
	cfgDir := filepath.Join(root, paths.ProcessInternalConfigsDir)
	if err := fileutil.EnsureDir(cfgDir); err != nil {
		t.Fatal(err)
	}
	// Same shape as docs/architecture/_internal/configs/high_volume_kinds.yaml; stream list must match spec_index storage_profile.
	yml := `kinds:
  - kind: organism_stream_kind
    storage: stream
  - kind: organism_cas_kind
    storage: cas
    note: "fixture — not stream-backed"
`
	p := filepath.Join(cfgDir, paths.HighVolumeKindsConfigFile)
	if err := fileutil.WriteSecureFile(p, []byte(yml)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Kinds []struct {
			Kind    string `yaml:"kind"`
			Storage string `yaml:"storage"`
		} `yaml:"kinds"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	var streamKinds []string
	for _, e := range cfg.Kinds {
		if e.Kind != "" && e.Storage == "stream" {
			streamKinds = append(streamKinds, e.Kind)
		}
	}
	mis, err := datacellregistry.HighVolumeStreamSpecProfileMismatches(root, streamKinds)
	if err != nil {
		t.Fatalf("HighVolumeStreamSpecProfileMismatches: %v", err)
	}
	if len(mis) != 0 {
		t.Fatalf("expected spec index and stream-kind list to agree, got %v", mis)
	}
}

func TestStreamOrganism_PostRetentionStreamStewardshipSmoke(t *testing.T) {
	t.Parallel()
	root := writeSyntheticDataCellProject(t)
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	storage.PostRetentionStreamStewardship(root, "organism-e2e", logger)
}

func TestStreamOrganism_SpecStreamDriftDetected(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	internal := filepath.Join(root, paths.ProcessInternalDir)
	if err := fileutil.EnsureDir(internal); err != nil {
		t.Fatal(err)
	}
	// Index says stream kind is cas_entity — operational list expects stream.
	idx := &objects.SpecIndex{
		Kinds: map[string]objects.SpecKindSummary{
			streamOrganismKind: {Kind: streamOrganismKind, StorageProfile: "cas_entity"},
		},
	}
	b, err := json.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(internal, "spec_index.json"), b); err != nil {
		t.Fatal(err)
	}
	mis, err := datacellregistry.HighVolumeStreamSpecProfileMismatches(root, []string{streamOrganismKind})
	if err != nil {
		t.Fatalf("HighVolumeStreamSpecProfileMismatches: %v", err)
	}
	if len(mis) != 1 {
		t.Fatalf("expected one drift detail, got %v", mis)
	}
}

func writeSyntheticDataCellProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	internal := filepath.Join(root, paths.ProcessInternalDir)
	if err := fileutil.EnsureDir(internal); err != nil {
		t.Fatal(err)
	}
	idx := &objects.SpecIndex{
		Kinds: map[string]objects.SpecKindSummary{
			streamOrganismKind: {
				Kind:           streamOrganismKind,
				StorageProfile: "stream",
				Fields:         []objects.SpecFieldSummary{},
			},
			casOrganismKind: {
				Kind:           casOrganismKind,
				StorageProfile: "cas_entity",
				Fields:         []objects.SpecFieldSummary{},
			},
		},
	}
	b, err := json.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(internal, "spec_index.json"), b); err != nil {
		t.Fatal(err)
	}
	// Runtime organism: minimal files so ReadRuntimeManifest / paths resolve like a real project.
	cfgDir := filepath.Join(root, paths.ProjectDataDir, paths.ConfigDir)
	if err := fileutil.EnsureDir(cfgDir); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(cfgDir, paths.FeatureFlagsFile), []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	return root
}
