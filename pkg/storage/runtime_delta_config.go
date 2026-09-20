package storage

import (
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const runtimeDeltaKindsConfigFile = "runtime_delta_kinds.yaml"
const runtimeDeltaFieldsConfigFile = "runtime_delta_fields.yaml"

type runtimeDeltaKindsConfig struct {
	Kinds []struct {
		Kind string `yaml:"kind"`
	} `yaml:"kinds"`
}

var (
	runtimeDeltaKindsMu sync.RWMutex
	runtimeDeltaKinds   = make(map[string]map[string]bool) // projectRoot -> kind -> true
	runtimeDeltaFields  = make(map[string]map[string][]string)
)

func RuntimeDeltaEnabledForKind(projectRoot, kind string) bool {
	if projectRoot == emptyValue || kind == emptyValue {
		return false
	}
	cfg := loadRuntimeDeltaKindsConfig(projectRoot)
	return cfg[kind]
}

func RuntimeDeltaEnabledKindsList(projectRoot string) []string {
	cfg := loadRuntimeDeltaKindsConfig(projectRoot)
	out := make([]string, 0, len(cfg))
	for kind := range cfg {
		out = append(out, kind)
	}
	return out
}

func loadRuntimeDeltaKindsConfig(projectRoot string) map[string]bool {
	runtimeDeltaKindsMu.RLock()
	cached, ok := runtimeDeltaKinds[projectRoot]
	runtimeDeltaKindsMu.RUnlock()
	if ok {
		return cached
	}

	configPath := filepath.Join(projectRoot, paths.ProcessInternalConfigsDir, runtimeDeltaKindsConfigFile)
	if _, err := fileutil.Stat(configPath); fileutil.IsNotExist(err) {
		processBase := paths.ResolvePathFromCacheOrConstant(projectRoot, "process", paths.ProcessDir)
		configPath = filepath.Join(processBase, "_internal", "configs", runtimeDeltaKindsConfigFile)
	}
	data, err := fileutil.ReadFile(configPath)
	if err != nil {
		runtimeDeltaKindsMu.Lock()
		runtimeDeltaKinds[projectRoot] = map[string]bool{}
		runtimeDeltaKindsMu.Unlock()
		return map[string]bool{}
	}
	var parsed runtimeDeltaKindsConfig
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		runtimeDeltaKindsMu.Lock()
		runtimeDeltaKinds[projectRoot] = map[string]bool{}
		runtimeDeltaKindsMu.Unlock()
		return map[string]bool{}
	}
	out := make(map[string]bool, len(parsed.Kinds))
	for _, k := range parsed.Kinds {
		if k.Kind != emptyValue {
			out[k.Kind] = true
		}
	}
	runtimeDeltaKindsMu.Lock()
	runtimeDeltaKinds[projectRoot] = out
	runtimeDeltaKindsMu.Unlock()
	return out
}

func updateIsRuntimeDeltaOnly(projectRoot, kind string, updates map[string]any) bool {
	if len(updates) == 0 || !RuntimeDeltaEnabledForKind(projectRoot, kind) {
		return false
	}
	allowed := make(map[string]bool)
	fields := getRuntimeDeltaFieldsFromSpec(projectRoot, kind)
	if len(fields) == 0 {
		fields = loadRuntimeDeltaFieldsConfig(projectRoot)[kind]
	}
	for _, field := range fields {
		allowed[field] = true
	}
	if len(allowed) == 0 {
		return false
	}
	for key := range updates {
		if key == ConstMiscExpectedUpdatedAt {
			continue
		}
		if !allowed[key] {
			return false
		}
	}
	return true
}

func loadRuntimeDeltaFieldsConfig(projectRoot string) map[string][]string {
	runtimeDeltaKindsMu.RLock()
	cached, ok := runtimeDeltaFields[projectRoot]
	runtimeDeltaKindsMu.RUnlock()
	if ok {
		return cached
	}
	configPath := filepath.Join(projectRoot, paths.ProcessInternalConfigsDir, runtimeDeltaFieldsConfigFile)
	if _, err := fileutil.Stat(configPath); fileutil.IsNotExist(err) {
		processBase := paths.ResolvePathFromCacheOrConstant(projectRoot, "process", paths.ProcessDir)
		configPath = filepath.Join(processBase, "_internal", "configs", runtimeDeltaFieldsConfigFile)
	}
	data, err := fileutil.ReadFile(configPath)
	if err != nil {
		runtimeDeltaKindsMu.Lock()
		runtimeDeltaFields[projectRoot] = map[string][]string{}
		runtimeDeltaKindsMu.Unlock()
		return map[string][]string{}
	}
	var parsed map[string][]string
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		runtimeDeltaKindsMu.Lock()
		runtimeDeltaFields[projectRoot] = map[string][]string{}
		runtimeDeltaKindsMu.Unlock()
		return map[string][]string{}
	}
	runtimeDeltaKindsMu.Lock()
	runtimeDeltaFields[projectRoot] = parsed
	runtimeDeltaKindsMu.Unlock()
	return parsed
}
