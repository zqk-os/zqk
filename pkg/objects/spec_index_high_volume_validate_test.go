package objects

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestValidateHighVolumeStreamKindsMatchSpecIndex_mismatch(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	specsDir := filepath.Join(tmp, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	// Deliberately not stream — conflicts with high_volume_kinds listing below.
	specYAML := `ontology: hv_mismatch
schema_version: "2.0.0"
visibility: internal
storage_profile: cas_entity
fields:
  id:
    type: string
`
	if err := fileutil.WriteFile(filepath.Join(specsDir, "hv_mismatch.yaml"), []byte(specYAML), paths.FilePerm644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	idx, err := BuildSpecIndexFromSpecsDir(specsDir)
	if err != nil {
		t.Fatalf("BuildSpecIndexFromSpecsDir: %v", err)
	}
	cfgDir := filepath.Join(tmp, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(cfgDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	hv := `kinds:
  - kind: hv_mismatch
    storage: stream
`
	if err := fileutil.WriteFile(filepath.Join(cfgDir, paths.HighVolumeKindsConfigFile), []byte(hv), paths.FilePerm644); err != nil {
		t.Fatalf("write high_volume_kinds: %v", err)
	}
	err = ValidateHighVolumeStreamKindsMatchSpecIndex(idx, tmp)
	if err == nil {
		t.Fatal("expected validation error for cas_entity vs high_volume stream")
	}
	if !strings.Contains(err.Error(), "hv_mismatch") {
		t.Fatalf("error should mention kind: %v", err)
	}
}

func TestValidateHighVolumeStreamKindsMatchSpecIndex_duplicateKindInYAML(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	specsDir := filepath.Join(tmp, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(specsDir, "only_kind.yaml"), []byte(`ontology: only_kind
schema_version: "2.0.0"
visibility: internal
fields:
  id:
    type: string
`), paths.FilePerm644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	idx, err := BuildSpecIndexFromSpecsDir(specsDir)
	if err != nil {
		t.Fatalf("BuildSpecIndexFromSpecsDir: %v", err)
	}
	cfgDir := filepath.Join(tmp, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(cfgDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	hv := `kinds:
  - kind: audit_event
    storage: stream
  - kind: audit_event
    storage: stream
`
	if err := fileutil.WriteFile(filepath.Join(cfgDir, paths.HighVolumeKindsConfigFile), []byte(hv), paths.FilePerm644); err != nil {
		t.Fatalf("write high_volume_kinds: %v", err)
	}
	err = ValidateHighVolumeStreamKindsMatchSpecIndex(idx, tmp)
	if err == nil {
		t.Fatal("expected duplicate kind error")
	}
	if !strings.Contains(err.Error(), "duplicate kind") {
		t.Fatalf("want duplicate kind in error, got: %v", err)
	}
}

func TestValidateHighVolumeStreamKindsMatchSpecIndex_emptyStorageInYAML(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	specsDir := filepath.Join(tmp, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(specsDir, "only_kind.yaml"), []byte(`ontology: only_kind
schema_version: "2.0.0"
visibility: internal
fields:
  id:
    type: string
`), paths.FilePerm644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	idx, err := BuildSpecIndexFromSpecsDir(specsDir)
	if err != nil {
		t.Fatalf("BuildSpecIndexFromSpecsDir: %v", err)
	}
	cfgDir := filepath.Join(tmp, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(cfgDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	hv := `kinds:
  - kind: audit_event
    storage: ""
`
	if err := fileutil.WriteFile(filepath.Join(cfgDir, paths.HighVolumeKindsConfigFile), []byte(hv), paths.FilePerm644); err != nil {
		t.Fatalf("write high_volume_kinds: %v", err)
	}
	err = ValidateHighVolumeStreamKindsMatchSpecIndex(idx, tmp)
	if err == nil {
		t.Fatal("expected empty storage error")
	}
	if !strings.Contains(err.Error(), "empty storage") {
		t.Fatalf("want empty storage in error, got: %v", err)
	}
}

func TestValidateHighVolumeStreamKindsMatchSpecIndex_unknownStorageInYAML(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	specsDir := filepath.Join(tmp, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(specsDir, "only_kind.yaml"), []byte(`ontology: only_kind
schema_version: "2.0.0"
visibility: internal
fields:
  id:
    type: string
`), paths.FilePerm644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	idx, err := BuildSpecIndexFromSpecsDir(specsDir)
	if err != nil {
		t.Fatalf("BuildSpecIndexFromSpecsDir: %v", err)
	}
	cfgDir := filepath.Join(tmp, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(cfgDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	hv := `kinds:
  - kind: audit_event
    storage: timeseries
`
	if err := fileutil.WriteFile(filepath.Join(cfgDir, paths.HighVolumeKindsConfigFile), []byte(hv), paths.FilePerm644); err != nil {
		t.Fatalf("write high_volume_kinds: %v", err)
	}
	err = ValidateHighVolumeStreamKindsMatchSpecIndex(idx, tmp)
	if err == nil {
		t.Fatal("expected unknown storage error")
	}
	if !strings.Contains(err.Error(), "unknown storage") {
		t.Fatalf("want unknown storage in error, got: %v", err)
	}
}

func TestValidateHighVolumeStreamKindsMatchSpecIndex_skipsWithoutConfigFile(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	specsDir := filepath.Join(tmp, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(specsDir, "only_kind.yaml"), []byte(`ontology: only_kind
schema_version: "2.0.0"
visibility: internal
fields:
  id:
    type: string
`), paths.FilePerm644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	idx, err := BuildSpecIndexFromSpecsDir(specsDir)
	if err != nil {
		t.Fatalf("BuildSpecIndexFromSpecsDir: %v", err)
	}
	if err := ValidateHighVolumeStreamKindsMatchSpecIndex(idx, tmp); err != nil {
		t.Fatalf("without high_volume_kinds.yaml want nil, got %v", err)
	}
}
