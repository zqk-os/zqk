package storage

import (
	"os"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
)

const runtimeDeltaCurrentDirName = "runtime_delta_current"

func runtimeDeltaCurrentDir(projectRoot, kind string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, runtimeDeltaCurrentDirName, kind)
}

func runtimeDeltaCurrentPath(projectRoot, kind, id string) string {
	return filepath.Join(runtimeDeltaCurrentDir(projectRoot, kind), id+".yaml")
}

// WriteRuntimeDeltaCurrentState persists the merged current state for runtime-delta-only updates
// on CAS-backed kinds so Read/List can observe fresh runtime fields without a CAS rewrite.
func WriteRuntimeDeltaCurrentState(projectRoot, kind, id string, data []byte) error {
	dir := runtimeDeltaCurrentDir(projectRoot, kind)
	if err := os.MkdirAll(dir, paths.DirPerm755); err != nil {
		return err
	}
	return os.WriteFile(runtimeDeltaCurrentPath(projectRoot, kind, id), data, paths.FilePerm600)
}

func ReadRuntimeDeltaCurrentState(projectRoot, kind, id string) map[string]any {
	data, err := os.ReadFile(runtimeDeltaCurrentPath(projectRoot, kind, id))
	if err != nil {
		return nil
	}
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return nil
	}
	return obj
}

func RemoveRuntimeDeltaCurrentState(projectRoot, kind, id string) error {
	if projectRoot == emptyValue || kind == emptyValue || id == emptyValue {
		return nil
	}
	p := runtimeDeltaCurrentPath(projectRoot, kind, id)
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
