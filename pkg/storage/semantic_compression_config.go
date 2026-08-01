package storage

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
)

// CompressionPolicy represents a loaded compression policy configuration
type CompressionPolicy struct {
	TargetKind         string `yaml:"target_kind"`
	Algorithm          string `yaml:"algorithm"`
	OmitSchemaDefaults bool   `yaml:"omit_schema_defaults"`
	ThresholdBytes     int    `yaml:"threshold_bytes"`
	CompressFieldKeys  bool   `yaml:"compress_field_keys"`
	ZlibThresholdBytes int    `yaml:"zlib_threshold_bytes"`
}

// FieldRegistry represents a loaded field registry configuration
type FieldRegistry struct {
	ActiveFields []string `yaml:"active_fields"`

	// internal map for fast lookup: string -> byte
	fieldMap map[string]byte
}

var (
	semanticCompressionMu sync.RWMutex
	compressionPolicies   map[string]*CompressionPolicy // keyed by projectRoot + ":" + targetKind
	fieldRegistries       map[string]*FieldRegistry     // keyed by projectRoot
)

func init() {
	compressionPolicies = make(map[string]*CompressionPolicy)
	fieldRegistries = make(map[string]*FieldRegistry)
}

// GetCompressionPolicy returns the compression policy for a specific kind in a project.
func GetCompressionPolicy(projectRoot, kind string) *CompressionPolicy {
	key := projectRoot + ":" + kind

	semanticCompressionMu.RLock()
	policy, ok := compressionPolicies[key]
	semanticCompressionMu.RUnlock()

	if ok {
		return policy
	}

	semanticCompressionMu.Lock()
	defer semanticCompressionMu.Unlock()

	// Double check
	if policy, ok := compressionPolicies[key]; ok {
		return policy
	}

	// Load policies for this project
	loadCompressionPoliciesForProject(projectRoot)

	if policy, ok := compressionPolicies[key]; ok {
		return policy
	}

	// Not found, check fallback default
	defaultPolicy := GetDefaultCompressionPolicyForKind(kind)
	compressionPolicies[key] = defaultPolicy
	return defaultPolicy
}

// GetDefaultCompressionPolicyForKind provides a fallback default policy for high-volume graph memory nodes.
func GetDefaultCompressionPolicyForKind(kind string) *CompressionPolicy {
	if kind == "graph_node" || kind == "memory_node" || kind == objects.KindAuditEvent || kind == objects.KindSchedulerJob {
		return &CompressionPolicy{
			TargetKind:         kind,
			Algorithm:          "zlib",
			OmitSchemaDefaults: true,
			ThresholdBytes:     512,
			CompressFieldKeys:  true,
			ZlibThresholdBytes: 1024,
		}
	}
	return &CompressionPolicy{
		TargetKind:         kind,
		Algorithm:          "none",
		OmitSchemaDefaults: false,
		ThresholdBytes:     4096,
		CompressFieldKeys:  false,
		ZlibThresholdBytes: 8192,
	}
}

// GetFieldRegistry returns the field registry for a project, providing string to byte ID mappings.
func GetFieldRegistry(projectRoot string) *FieldRegistry {
	semanticCompressionMu.RLock()
	registry, ok := fieldRegistries[projectRoot]
	semanticCompressionMu.RUnlock()

	if ok {
		return registry
	}

	semanticCompressionMu.Lock()
	defer semanticCompressionMu.Unlock()

	// Double check
	if registry, ok := fieldRegistries[projectRoot]; ok {
		return registry
	}

	// Load registry for this project
	registry = loadFieldRegistryForProject(projectRoot)
	fieldRegistries[projectRoot] = registry

	return registry
}

// loadCompressionPoliciesForProject parses all compression_policy YAML files in the project.
// Must be called with semanticCompressionMu held for write.
func loadCompressionPoliciesForProject(projectRoot string) {
	dir := filepath.Join(projectRoot, paths.ProjectDataDir, "..", "docs", "process", ConstMiscCompressionPolicy)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
			path := filepath.Join(dir, entry.Name())
			data, err := os.ReadFile(path)
			if err == nil {
				var policy CompressionPolicy
				if err := yaml.Unmarshal(data, &policy); err == nil && policy.TargetKind != "" {
					compressionPolicies[projectRoot+":"+policy.TargetKind] = &policy
				}
			}
		}
	}
}

// loadFieldRegistryForProject parses all field_registry YAML files and merges them.
// Must be called with semanticCompressionMu held for write.
func loadFieldRegistryForProject(projectRoot string) *FieldRegistry {
	registry := &FieldRegistry{
		fieldMap: make(map[string]byte),
	}

	dir := filepath.Join(projectRoot, paths.ProjectDataDir, "..", "docs", "process", ConstMiscFieldRegistry)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return registry
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
			path := filepath.Join(dir, entry.Name())
			data, err := os.ReadFile(path)
			if err == nil {
				var reg FieldRegistry
				if err := yaml.Unmarshal(data, &reg); err == nil {
					registry.ActiveFields = append(registry.ActiveFields, reg.ActiveFields...)
				}
			}
		}
	}

	// Parse mappings (e.g. "title:0x01")
	for _, mapping := range registry.ActiveFields {
		parts := strings.SplitN(mapping, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			valStr := strings.TrimSpace(parts[1])

			// Parse hex or decimal
			var id uint64
			var err error
			if strings.HasPrefix(valStr, "0x") {
				id, err = strconv.ParseUint(valStr[2:], 16, 8)
			} else {
				id, err = strconv.ParseUint(valStr, 10, 8)
			}

			if err == nil {
				registry.fieldMap[key] = byte(id)
			}
		}
	}

	return registry
}

// TranslateFieldKeys applies binary projection to the object using the field registry.
func (reg *FieldRegistry) TranslateFieldKeys(obj map[string]any) map[string]any {
	if reg == nil || len(reg.fieldMap) == 0 {
		return obj
	}

	translated := make(map[string]any, len(obj))
	for k, v := range obj {
		if id, ok := reg.fieldMap[k]; ok {
			// Prepend a special marker to indicate binary key.
			// Since json.Marshal only supports string keys in Go maps, we use a string representation of the byte.
			// In a real optimized binary format, we'd use BSON or MessagePack.
			// For this Phase 1, we map to a tiny string marker to simulate projection size reduction.
			translated[string([]byte{0x00, id})] = v
		} else {
			translated[k] = v
		}
	}
	return translated
}

// RestoreFieldKeys reverses TranslateFieldKeys, mapping binary keys back to their original string names.
func (reg *FieldRegistry) RestoreFieldKeys(obj map[string]any) map[string]any {
	if reg == nil || len(reg.fieldMap) == 0 {
		return obj
	}

	// Build inverse map if not already done (small optimization possible here if needed)
	inverseMap := make(map[byte]string, len(reg.fieldMap))
	for k, v := range reg.fieldMap {
		inverseMap[v] = k
	}

	restored := make(map[string]any, len(obj))
	for k, v := range obj {
		if len(k) == 2 && k[0] == 0x00 {
			if originalKey, ok := inverseMap[k[1]]; ok {
				restored[originalKey] = v
				continue
			}
		}
		restored[k] = v
	}
	return restored
}
