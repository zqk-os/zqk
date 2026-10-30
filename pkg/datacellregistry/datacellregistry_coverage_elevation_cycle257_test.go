package datacellregistry

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestDataCellDescriptorsFromSpecIndex_NilKindsAndInvalidIdentity(t *testing.T) {
	t.Parallel()

	// idx != nil, but idx.Kinds == nil
	idxNilKinds := &objects.SpecIndex{
		Kinds: nil,
	}
	desc, err := DataCellDescriptorsFromSpecIndex(idxNilKinds)
	if err != nil {
		t.Fatalf("expected nil error for nil Kinds, got %v", err)
	}
	if desc != nil {
		t.Fatalf("expected nil slice for nil Kinds, got %#v", desc)
	}

	// Kind with empty string key violates ValidateV1Identity
	idxEmptyKind := &objects.SpecIndex{
		Kinds: map[string]objects.SpecKindSummary{
			"": {Kind: "", StorageProfile: "stream"},
		},
	}
	_, err = DataCellDescriptorsFromSpecIndex(idxEmptyKind)
	if err == nil {
		t.Fatalf("expected ValidateV1Identity error for empty kind name")
	}
}

func TestLoadSpecIndex_FallbackToSpecsDirAndErrorHandling(t *testing.T) {
	t.Parallel()

	// 1. Fallback to specsDir when spec_index.json does not exist
	tmpFallback := t.TempDir()
	specsDir := filepath.Join(tmpFallback, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir specsDir: %v", err)
	}
	specYAML := `ontology: fallback_cell
schema_version: "2.0.0"
visibility: internal
storage_profile: cas_entity
fields:
  id:
    type: string
`
	if err := fileutil.WriteFile(filepath.Join(specsDir, "fallback_cell.yaml"), []byte(specYAML), paths.FilePerm644); err != nil {
		t.Fatalf("write specYAML: %v", err)
	}

	desc, err := LoadDataCellDescriptors(tmpFallback)
	if err != nil {
		t.Fatalf("LoadDataCellDescriptors via specsDir fallback failed: %v", err)
	}
	if len(desc) != 1 || desc[0].Kind != "fallback_cell" {
		t.Fatalf("unexpected descriptors: %#v", desc)
	}
	if desc[0].ParsedProfile != datacell.ProfileCASEntity {
		t.Fatalf("unexpected profile: %v", desc[0].ParsedProfile)
	}

	// 2. Failure when both spec_index.json and object_specs are absent / invalid
	tmpEmpty := t.TempDir()
	_, err = LoadDataCellDescriptors(tmpEmpty)
	if err == nil {
		t.Fatalf("expected error loading descriptors from empty dir")
	}
}

func TestLoadDescriptorReadModel_ErrorPropagation(t *testing.T) {
	t.Parallel()

	// Spec index present but contains invalid storage_profile -> DataCellDescriptorsFromSpecIndex error
	tmp := t.TempDir()
	internalDir := filepath.Join(tmp, paths.ProcessInternalDir)
	if err := fileutil.MkdirAll(internalDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir internal: %v", err)
	}
	badIdx := objects.SpecIndex{
		Kinds: map[string]objects.SpecKindSummary{
			"broken_kind": {
				Kind:           "broken_kind",
				StorageProfile: "invalid_unsupported_profile",
			},
		},
		BuilderSpecCacheRevision: 42,
	}
	raw, err := json.Marshal(badIdx)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(internalDir, "spec_index.json"), raw, paths.FilePerm644); err != nil {
		t.Fatalf("write spec_index.json: %v", err)
	}

	_, err = LoadDescriptorReadModel(tmp)
	if err == nil {
		t.Fatalf("expected error from LoadDescriptorReadModel when spec index has invalid profile")
	}
}

func TestDescriptorReadModelForProject_RevisionZeroAndSnapshotError(t *testing.T) {
	// 1. Revision zero loads directly without caching
	root := findProjectRoot(t)
	rmZero, err := DescriptorReadModelForProject(root, 0)
	if err != nil {
		t.Fatalf("DescriptorReadModelForProject revision 0: %v", err)
	}
	if rmZero == nil || len(rmZero.Descriptors()) == 0 {
		t.Fatalf("expected non-empty read model for rev 0")
	}

	// 2. Snapshot error propagation when cache cannot load from invalid root
	tmpInvalid := t.TempDir()
	_, err = DescriptorReadModelForProject(tmpInvalid, 10)
	if err == nil {
		t.Fatalf("expected error when loading snapshot from invalid root")
	}
}

func TestStreamKindNamesFromHighVolumeConfig_StatAndReadErrors(t *testing.T) {
	t.Parallel()

	// 1. Stat error other than IsNotExist: make .zqk/specs a regular file so configs dir cannot be traversed
	tmpStatErr := t.TempDir()
	fakeFile := filepath.Join(tmpStatErr, paths.ProcessInternalDir)
	if err := fileutil.MkdirAll(filepath.Dir(fakeFile), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := fileutil.WriteFile(fakeFile, []byte("i am a file"), paths.FilePerm644); err != nil {
		t.Fatalf("write fake file: %v", err)
	}
	_, err := StreamKindNamesFromHighVolumeConfig(tmpStatErr)
	if err == nil {
		t.Fatalf("expected stat error when .zqk/specs is a file")
	}

	// 2. Read error: high_volume_kinds.yaml is a directory instead of a regular file
	tmpReadErr := t.TempDir()
	cfgDir := filepath.Join(tmpReadErr, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(cfgDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir cfgDir: %v", err)
	}
	yamlAsDir := filepath.Join(cfgDir, paths.HighVolumeKindsConfigFile)
	if err := fileutil.MkdirAll(yamlAsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir yamlAsDir: %v", err)
	}
	_, err = StreamKindNamesFromHighVolumeConfig(tmpReadErr)
	if err == nil {
		t.Fatalf("expected read error when high_volume_kinds.yaml is a directory")
	}

	// 3. YAML containing empty kind, non-stream kinds, and duplicates
	tmpFilter := t.TempDir()
	cfgDir2 := filepath.Join(tmpFilter, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(cfgDir2, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir cfgDir2: %v", err)
	}
	yamlContent := `kinds:
  - kind: ""
    storage: stream
  - kind: ignored_cas
    storage: cas_entity
  - kind: valid_stream
    storage: stream
  - kind: valid_stream
    storage: stream
`
	if err := fileutil.WriteFile(filepath.Join(cfgDir2, paths.HighVolumeKindsConfigFile), []byte(yamlContent), paths.FilePerm644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	kinds, err := StreamKindNamesFromHighVolumeConfig(tmpFilter)
	if err != nil {
		t.Fatalf("StreamKindNamesFromHighVolumeConfig failed: %v", err)
	}
	if len(kinds) != 1 || kinds[0] != "valid_stream" {
		t.Fatalf("expected [valid_stream], got %v", kinds)
	}
}

func TestDescriptorReadModelForProject_ConcurrencyAndIsolation(t *testing.T) {
	root := findProjectRoot(t)
	rm, err := LoadDescriptorReadModel(root)
	if err != nil {
		t.Fatalf("LoadDescriptorReadModel: %v", err)
	}
	rev := rm.BuiltAtSpecCacheRevision()
	if rev == 0 {
		rev = 1
	}

	// Concurrently query DescriptorReadModelForProject and perform cache invalidations
	const goroutines = 8
	const iterations = 15
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		workerID := g
		goroutinelabels.NewGoroutine("datacellregistry_concurrency_test", fmt.Sprintf("worker-%d", workerID)).StartSimple(func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				switch i % 4 {
				case 0:
					got, qErr := DescriptorReadModelForProject(root, rev)
					if qErr != nil {
						t.Errorf("worker %d query error: %v", workerID, qErr)
						return
					}
					if got == nil || len(got.Descriptors()) == 0 {
						t.Errorf("worker %d empty descriptors", workerID)
						return
					}
				case 1:
					got0, qErr := DescriptorReadModelForProject(root, 0)
					if qErr != nil {
						t.Errorf("worker %d rev0 query error: %v", workerID, qErr)
						return
					}
					if got0 == nil || len(got0.Descriptors()) == 0 {
						t.Errorf("worker %d rev0 empty descriptors", workerID)
						return
					}
				case 2:
					if workerID%3 == 0 {
						InvalidateDescriptorReadModelCache(root)
					}
				case 3:
					cache := descriptorCacheForProject(fmt.Sprintf("%s/fake-%d", root, workerID))
					if cache == nil {
						t.Errorf("worker %d nil cache", workerID)
						return
					}
				}
			}
		})
	}

	wg.Wait()
}

func TestHighVolumeStreamStewardshipDrift_ErrorPropagation(t *testing.T) {
	t.Parallel()

	// Missing project root or corrupt directory propagation
	tmp := t.TempDir()
	fakeFile := filepath.Join(tmp, paths.ProcessInternalDir)
	if err := fileutil.MkdirAll(filepath.Dir(fakeFile), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := fileutil.WriteFile(fakeFile, []byte("not a dir"), paths.FilePerm644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	_, err := HighVolumeStreamStewardshipDrift(tmp)
	if err == nil {
		t.Fatalf("expected error from HighVolumeStreamStewardshipDrift when .zqk/specs is a file")
	}
}
