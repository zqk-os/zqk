package storage

import (
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
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
	runtimeDeltaKindSets  stampmemo.Table[map[string]bool]
	runtimeDeltaFieldSets stampmemo.Table[map[string][]string]
)

func processInternalYAMLConfigCandidates(projectRoot, fileName string) []string {
	primary := filepath.Join(projectRoot, paths.ProcessInternalConfigsDir, fileName)
	processBase := paths.ResolvePathFromCacheOrConstant(projectRoot, "process", paths.ProcessDir)
	fallback := filepath.Join(processBase, "_internal", "configs", fileName)
	if primary == fallback {
		return []string{primary}
	}
	return []string{primary, fallback}
}

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
	cands := processInternalYAMLConfigCandidates(projectRoot, runtimeDeltaKindsConfigFile)
	cfg, _ := runtimeDeltaKindSets.Load(projectRoot, stampmemo.OfAll(cands...), func() (map[string]bool, error) {
		path := stampmemo.FirstExisting(cands)
		if path == emptyValue {
			return map[string]bool{}, nil
		}
		data, err := fileutil.ReadFile(path)
		if err != nil {
			return map[string]bool{}, nil
		}
		var parsed runtimeDeltaKindsConfig
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			return map[string]bool{}, nil
		}
		out := make(map[string]bool, len(parsed.Kinds))
		for _, k := range parsed.Kinds {
			if k.Kind != emptyValue {
				out[k.Kind] = true
			}
		}
		return out, nil
	})
	if cfg == nil {
		return map[string]bool{}
	}
	return cfg
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
	cands := processInternalYAMLConfigCandidates(projectRoot, runtimeDeltaFieldsConfigFile)
	cfg, _ := runtimeDeltaFieldSets.Load(projectRoot, stampmemo.OfAll(cands...), func() (map[string][]string, error) {
		path := stampmemo.FirstExisting(cands)
		if path == emptyValue {
			return map[string][]string{}, nil
		}
		data, err := fileutil.ReadFile(path)
		if err != nil {
			return map[string][]string{}, nil
		}
		var parsed map[string][]string
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			return map[string][]string{}, nil
		}
		if parsed == nil {
			parsed = map[string][]string{}
		}
		return parsed, nil
	})
	if cfg == nil {
		return map[string][]string{}
	}
	return cfg
}
