package storage

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// RuntimeDeltaCurrentDirNameForTest is the state subdirectory for runtime_delta overlays (see [runtimeDeltaCurrentDirName]).
const RuntimeDeltaCurrentDirNameForTest = runtimeDeltaCurrentDirName

// RuntimeDeltaKindsConfigFileForTest is the kinds config filename under process configs (see [runtimeDeltaKindsConfigFile]).
const RuntimeDeltaKindsConfigFileForTest = runtimeDeltaKindsConfigFile

// ClassifyUpdateMutationForTest exposes [classifyUpdateMutation] for external tests.
func ClassifyUpdateMutationForTest(idUpdated, runtimeDeltaOnly bool) string {
	return classifyUpdateMutation(idUpdated, runtimeDeltaOnly)
}

// Expected return strings from [classifyUpdateMutation] (see unexported constants in object_storage_file_update.go).
const (
	UpdateMutationClassIDChangeForTest     = updateMutationClassIDChange
	UpdateMutationClassRuntimeDeltaForTest = updateMutationClassRuntimeDelta
	UpdateMutationClassStructuralForTest   = updateMutationClassStructural
)

// WriteRuntimeDeltaKindsConfigForTest writes minimal scheduler_job runtime-delta YAML under the process configs dir
// and clears in-memory cached maps for root so the loader re-reads (same as [writeRuntimeDeltaKindsConfig] in tests).
func WriteRuntimeDeltaKindsConfigForTest(t *testing.T, root string) {
	t.Helper()
	cfgDir := filepath.Join(root, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(cfgDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfg := "kinds:\n  - kind: scheduler_job\n"
	if err := fileutil.WriteFile(filepath.Join(cfgDir, runtimeDeltaKindsConfigFile), []byte(cfg), paths.FilePerm644); err != nil {
		t.Fatalf("write runtime delta config: %v", err)
	}
	fields := "scheduler_job:\n  - status\n  - updated_at\n  - updated_by\n  - last_run_at\n  - next_run_at\n"
	if err := fileutil.WriteFile(filepath.Join(cfgDir, runtimeDeltaFieldsConfigFile), []byte(fields), paths.FilePerm644); err != nil {
		t.Fatalf("write runtime delta fields config: %v", err)
	}
	runtimeDeltaKindSets.Delete(root)
	runtimeDeltaFieldSets.Delete(root)
}
