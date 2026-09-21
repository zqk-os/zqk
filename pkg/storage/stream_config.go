// Package storage: stream storage feature and kind configuration.
//
// Runtime enablement uses high_volume_kinds.yaml (see StreamStorageEnabledForKind). Object specs
// must still declare storage_profile: stream for those kinds so the spec plane and spec_index.json
// match stream routing (see objects.TestSpecIndexStorageProfileMatchesHighVolumeStreamKinds).
//
// Single source of truth: .zqk/specs/configs/high_volume_kinds.yaml.
// - storage: stream → stream create path + high-volume cache/retention
// - storage: cas (or other non-stream) → high-volume cache/retention only (not stream)
// Adding a new high-volume kind is a YAML (+ object spec) change — do not maintain parallel Go kind lists.

package storage

import (
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/stampmemo"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
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

const (
	highVolumeStorageStream = "stream"
)

// streamStorageKindsDefault is the compile-time fallback stream set when
// high_volume_kinds.yaml cannot be found or parsed.
// NOTE: scheduler_job is intentionally absent from stream — full CRUD semantics.
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

// highVolumeKindsDefault is the compile-time fallback for cache/retention membership
// (stream defaults plus CAS-backed HV kinds such as scheduler_job).
var highVolumeKindsDefault = func() map[string]bool {
	m := make(map[string]bool, len(streamStorageKindsDefault)+1)
	for k, v := range streamStorageKindsDefault {
		m[k] = v
	}
	m[objects.KindSchedulerJob] = true
	return m
}()

type hvKindMaps struct {
	stream map[string]bool
	all    map[string]bool
}

var hvKindMemo stampmemo.Table[hvKindMaps] // keyed by high_volume_kinds.yaml path (closed set)

func loadHighVolumeKindMapsFrom(configPath string) (stream map[string]bool, all map[string]bool) {
	if configPath == emptyValue {
		return streamStorageKindsDefault, highVolumeKindsDefault
	}
	data, err := fileutil.ReadFile(configPath)
	if err != nil {
		return streamStorageKindsDefault, highVolumeKindsDefault
	}
	var cfg highVolumeKindsFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return streamStorageKindsDefault, highVolumeKindsDefault
	}
	if len(cfg.Kinds) == 0 {
		return streamStorageKindsDefault, highVolumeKindsDefault
	}
	stream = make(map[string]bool, len(cfg.Kinds))
	all = make(map[string]bool, len(cfg.Kinds))
	for _, entry := range cfg.Kinds {
		if entry.Kind == emptyValue {
			continue
		}
		all[entry.Kind] = true
		if strings.EqualFold(entry.Storage, highVolumeStorageStream) {
			stream[entry.Kind] = true
		}
	}
	if len(stream) == 0 {
		return streamStorageKindsDefault, highVolumeKindsDefault
	}
	return stream, all
}

func loadHighVolumeKindMaps() (stream map[string]bool, all map[string]bool) {
	return loadHighVolumeKindMapsFrom(findHighVolumeKindsConfig())
}

func memoizedHighVolumeKindMaps(path string) hvKindMaps {
	cached, _ := hvKindMemo.Load(path, stampmemo.Of(path), func() (hvKindMaps, error) {
		stream, all := loadHighVolumeKindMapsFrom(path)
		return hvKindMaps{stream: stream, all: all}, nil
	})
	return hvKindMaps{stream: maps.Clone(cached.stream), all: maps.Clone(cached.all)}
}

func loadMemoizedHighVolumeKindMaps() hvKindMaps {
	return memoizedHighVolumeKindMaps(findHighVolumeKindsConfig())
}

func getStreamStorageEnabledKinds() map[string]bool {
	return loadMemoizedHighVolumeKindMaps().stream
}

func getHighVolumeKinds() map[string]bool {
	return loadMemoizedHighVolumeKindMaps().all
}

// findHighVolumeKindsConfig walks up from cwd looking for the project root and returns
// the path to .zqk/specs/configs/high_volume_kinds.yaml, or "" if not found.
func findHighVolumeKindsConfig() string {
	return paths.FirstExistingFromCwd(filepath.Join(paths.ProcessInternalConfigsDir, paths.HighVolumeKindsConfigFile))
}

// StreamStorageEnabledForKind returns true when stream storage is enabled for the kind.
// Default: on (opt-out via ZQK_STREAM_STORAGE_ENABLED=false or 0). When true, Create writes to
// append-only segment files instead of CAS. See BYPASS_KIND_STORAGE.md, HIGH_VOLUME_STORAGE_DEPRECATION.md.
func StreamStorageEnabledForKind(kind string) bool {
	if !getStreamStorageEnabledKinds()[kind] {
		return false
	}
	if v, ok := os.LookupEnv(zqkenv.StreamStorageEnabled().Name()); ok {
		v = strings.TrimSpace(v)
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
		if v == "0" {
			return false
		}
		if v == "1" {
			return true
		}
	}
	v := config.Get().System.StreamStorageEnabled
	if v == nil {
		return true
	}
	return *v
}

// IsHighVolumeKindForCache returns true for kinds that participate in the high-volume event cache
// (Count, OldestIDs, time-window) and retention HV fast paths. Any kind listed in
// high_volume_kinds.yaml qualifies (stream or cas). See high_volume_kinds.yaml.
func IsHighVolumeKindForCache(kind string) bool {
	return getHighVolumeKinds()[kind]
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

// HighVolumeKindsForCacheBuild returns kinds to scan when building the high-volume event cache.
// Derived from high_volume_kinds.yaml (all listed kinds). audit_event is ordered first when present.
func HighVolumeKindsForCacheBuild() []string {
	m := getHighVolumeKinds()
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	// Prefer audit_event first (highest volume / build cost bias).
	for i, k := range out {
		if k == objects.KindAuditEvent {
			if i > 0 {
				out[0], out[i] = out[i], out[0]
			}
			break
		}
	}
	return out
}
