// Package storage: stream storage feature and kind configuration.
//
// Runtime enablement uses high_volume_kinds.yaml (see StreamStorageEnabledForKind). Object specs
// must still declare storage_profile: stream for those kinds so the spec plane and spec_index.json
// match stream routing (see objects.TestSpecIndexStorageProfileMatchesHighVolumeStreamKinds).

package storage

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"gopkg.in/yaml.v3"
)

// highVolumeKindEntry is the per-kind entry in high_volume_kinds.yaml.
type highVolumeKindEntry struct {
	Kind    string `yaml:"kind"`
	Storage string `yaml:"storage"`
	Note    string `yaml:"note,omitempty"`
}

type highVolumeKindsFileConfig struct {
	Kinds []highVolumeKindEntry `yaml:"kinds"`
}

// streamStorageKindsDefault is the compile-time fallback for when
// docs/architecture/_internal/configs/high_volume_kinds.yaml cannot be found or parsed.
// NOTE: scheduler_job is intentionally absent – it requires full CRUD semantics
// (update status, delete by ID) that stream storage does not support.
// See HIGH_VOLUME_STORAGE_DEPRECATION.md.
var streamStorageKindsDefault = map[string]bool{
	objects.KindAuditEvent:             true,
	objects.KindChangeJournalEntry:     true,
	objects.KindAuditAggregationMetric: true,
	objects.KindBaseMetric:             true,
	objects.KindCommandMetric:          true,
	objects.KindFileLockMetric:         true,
	objects.KindCodeQualityMetric:      true,
	objects.KindKindMappingMetric:      true,
	objects.KindSchedulerHealthMetric:  true,
	objects.KindMcpSession:             true,
	objects.KindZqkSession:             true,
	objects.KindVerificationMatrix:     true,
	objects.KindAgentInstruction:       true,
}

var (
	streamStorageKindsOnce sync.Once
	streamStorageKindsMap  map[string]bool
)

// getStreamStorageEnabledKinds returns the effective stream-storage-enabled kinds map.
// On first call it reads docs/architecture/_internal/configs/high_volume_kinds.yaml by walking
// up from cwd; falls back to streamStorageKindsDefault if the file is absent or unreadable.
func getStreamStorageEnabledKinds() map[string]bool {
	streamStorageKindsOnce.Do(func() {
		streamStorageKindsMap = loadStreamStorageKindsFromYAML()
	})
	return streamStorageKindsMap
}

// loadStreamStorageKindsFromYAML walks up the directory tree from cwd looking for the
// canonical high_volume_kinds.yaml. Returns streamStorageKindsDefault on any error.
func loadStreamStorageKindsFromYAML() map[string]bool {
	configPath := findHighVolumeKindsConfig()
	if configPath == emptyValue {
		return streamStorageKindsDefault
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return streamStorageKindsDefault
	}
	var cfg highVolumeKindsFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return streamStorageKindsDefault
	}
	if len(cfg.Kinds) == 0 {
		return streamStorageKindsDefault
	}
	m := make(map[string]bool, len(cfg.Kinds))
	for _, entry := range cfg.Kinds {
		if entry.Kind != emptyValue && entry.Storage == "stream" {
			m[entry.Kind] = true
		}
	}
	return m
}

// findHighVolumeKindsConfig walks up from cwd looking for the project root and returns
// the path to docs/architecture/_internal/configs/high_volume_kinds.yaml, or "" if not found.
func findHighVolumeKindsConfig() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := wd
	markers := []string{paths.ProcessInternalConfigsDir, paths.ProjectDataDir, "go.mod"}
	for {
		for _, marker := range markers {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				candidate := filepath.Join(dir, paths.ProcessInternalConfigsDir, paths.HighVolumeKindsConfigFile)
				if _, err := os.Stat(candidate); err == nil {
					return candidate
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// StreamStorageEnabledForKind returns true when stream storage is enabled for the kind.
// Default: on (opt-out via ZQK_STREAM_STORAGE_ENABLED=false or 0). When true, Create writes to
// append-only segment files instead of CAS. See BYPASS_KIND_STORAGE.md, HIGH_VOLUME_STORAGE_DEPRECATION.md.
func StreamStorageEnabledForKind(kind string) bool {
	if !getStreamStorageEnabledKinds()[kind] {
		return false
	}
	v := os.Getenv(zqkenv.StreamStorageEnabled())
	// Default on: unset or "true"/"1" => enabled; "false"/"0" => opt-out
	if v == emptyValue {
		return true
	}
	return strings.EqualFold(v, "true") || v == "1"
}

// IsHighVolumeKindForCache returns true for kinds that participate in the high-volume event cache
// (Count, OldestIDs, time-window). Used by Count/List to use cache when populated. See high_volume_kinds.yaml.
func IsHighVolumeKindForCache(kind string) bool {
	return getStreamStorageEnabledKinds()[kind]
}

// StreamStorageEnabledKindsList returns all kinds that can use stream storage (for path cache build).
func StreamStorageEnabledKindsList() []string {
	m := getStreamStorageEnabledKinds()
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
