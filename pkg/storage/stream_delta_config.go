// Package storage: load stream delta field lists from config and/or object specs for recordForStream.
// Prefer spec-derived fields (storage_role: runtime_delta) when present; fall back to
// stream_delta_fields.yaml and built-in defaults. See DATA_STORAGE_PRODUCTION_ROADMAP.md §4.

package storage

import (
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

const streamDeltaFieldsConfigFile = "stream_delta_fields.yaml"

var (
	streamDeltaConfigMu sync.RWMutex
	streamDeltaConfig   = make(map[string]streamDeltaFieldsByKind) // projectRoot -> config
	specDeltaCacheMu    sync.RWMutex
	specDeltaCache      = make(map[string][]string) // projectRoot+kind -> field names (nil = use fallback)
)

type streamDeltaFieldsByKind map[string][]string // kind or "metric" -> field names

// getStreamDeltaFieldsForKind returns the list of field names to persist for this kind.
// Base list from config or built-in; then any field with storage_role: runtime_delta from the
// kind's spec (ontology inheritance) is merged in so spec annotations stay the source of truth.
func getStreamDeltaFieldsForKind(projectRoot, kind string) []string {
	var base []string
	if projectRoot != emptyValue {
		cfg := loadStreamDeltaConfig(projectRoot)
		if cfg != nil {
			// Prefer kind-specific list (e.g. command_metric) over the shared "metric" bucket.
			if kindFields := cfg[kind]; len(kindFields) > 0 {
				base = kindFields
			} else if kind == objects.KindChangeJournalEntry {
				base = cfg[objects.KindChangeJournalEntry]
			} else if isMetricKind(kind) {
				base = cfg[objects.FieldKeyMetric]
			}
		}
	}
	if len(base) == 0 {
		switch kind {
		case objects.KindChangeJournalEntry:
			base = builtinChangeJournalDeltaFields()
		case objects.KindCommandMetric:
			base = builtinCommandMetricDeltaFields()
		default:
			if isMetricKind(kind) {
				base = builtinMetricDeltaFields()
			}
		}
	}
	if len(base) == 0 {
		return nil
	}
	// Merge in any runtime_delta fields from spec not already in base (ontology inheritance)
	if projectRoot != emptyValue && (kind == objects.KindChangeJournalEntry || isMetricKind(kind)) {
		fromSpec := getRuntimeDeltaFieldsFromSpec(projectRoot, kind)
		base = mergeSpecRuntimeDeltaInto(base, fromSpec)
	}
	return base
}

// getRuntimeDeltaFieldsFromSpec loads the kind's spec (with inheritance) and returns field names
// that have storage_role: runtime_delta in checklist. Cached per (projectRoot, kind). Returns nil
// on error or when no such fields (caller falls back to config/built-in).
func getRuntimeDeltaFieldsFromSpec(projectRoot, kind string) []string {
	cacheKey := projectRoot + "\x00" + kind
	specDeltaCacheMu.RLock()
	cached, ok := specDeltaCache[cacheKey]
	specDeltaCacheMu.RUnlock()
	if ok {
		return cached
	}
	internalBase := paths.ResolvePathFromCacheOrConstant(projectRoot, ConstStreamProcessInternal, paths.ProcessInternalDir)
	specsDir := filepath.Join(internalBase, "object_specs")
	loader := objects.NewSpecLoader(specsDir)
	spec, err := loader.LoadSpecWithInheritance(kind + ".yaml")
	if err != nil || objects.SpecResolvedFieldsMissing(spec) {
		specDeltaCacheMu.Lock()
		specDeltaCache[cacheKey] = nil
		specDeltaCacheMu.Unlock()
		return nil
	}
	var names []string
	for name, fieldDef := range spec.ResolvedFields {
		def, ok := fieldDef.(map[string]any)
		if !ok {
			continue
		}
		checklist, _ := def["checklist"].(map[string]any)
		if checklist != nil && checklist["storage_role"] == "runtime_delta" {
			names = append(names, name)
		}
	}
	specDeltaCacheMu.Lock()
	specDeltaCache[cacheKey] = names
	specDeltaCacheMu.Unlock()
	return names
}

// mergeSpecRuntimeDeltaInto appends any names from specFields that are not already in base.
func mergeSpecRuntimeDeltaInto(base, specFields []string) []string {
	seen := make(map[string]bool)
	for _, s := range base {
		seen[s] = true
	}
	for _, s := range specFields {
		if !seen[s] {
			seen[s] = true
			base = append(base, s)
		}
	}
	return base
}

func builtinChangeJournalDeltaFields() []string {
	return []string{
		objects.FieldKeyID, objects.FieldKeySchemaVersion, objects.FieldKeyObjectRef, objects.FieldKeyChangeType, objects.FieldKeyDiffSummary, objects.FieldKeyChangedPaths,
		objects.FieldKeyCreatedAt, objects.FieldKeyCreatedBy, objects.FieldKeyUpdatedAt, objects.FieldKeyUpdatedBy, objects.FieldKeyTitle,
		objects.FieldKeyPreviousState,
	}
}

func builtinMetricDeltaFields() []string {
	return []string{
		objects.FieldKeyID, objects.FieldKeyKind, objects.FieldKeySchemaVersion, objects.FieldKeyStatus, objects.FieldKeyCreatedAt, objects.FieldKeyCreatedBy, objects.FieldKeyUpdatedAt, objects.FieldKeyUpdatedBy,
		objects.FieldKeyWindowStart, objects.FieldKeyWindowEnd, objects.FieldKeyFirstSeen, objects.FieldKeyLastSeen,
		objects.FieldKeyAggregationWindowStart, objects.FieldKeyAggregationWindowEnd,
		ConstStreamAggregatedEntryCount, objects.FieldKeyEventCount, objects.FieldKeyEventTypeCounts, objects.FieldKeyMetricType,
		objects.FieldKeyTitle, objects.FieldKeySummary,
		objects.FieldKeyCollectionCount,
	}
}

// builtinCommandMetricDeltaFields is the stream SSOT field set when stream_delta_fields.yaml
// has no command_metric entry (common in sparse test layouts).
func builtinCommandMetricDeltaFields() []string {
	return []string{
		objects.FieldKeyID, objects.FieldKeyKind, objects.FieldKeySchemaVersion, objects.FieldKeyStatus,
		objects.FieldKeyCreatedAt, objects.FieldKeyCreatedBy, objects.FieldKeyUpdatedAt, objects.FieldKeyUpdatedBy,
		objects.FieldKeyTitle, objects.FieldKeyMetricType, objects.FieldKeyCollectionCount,
		objects.FieldKeyFirstSeen, objects.FieldKeyLastSeen,
		objects.FieldKeyCommand, objects.FieldKeyNormalizedCmd, objects.FieldKeyInvocationCount,
		objects.FieldKeySuccessCount, objects.FieldKeyFailureCount, objects.FieldKeyTimeoutCount,
		objects.FieldKeyErrorRate, objects.FieldKeyTimeoutRate,
		objects.FieldKeyAvgDurationSeconds, objects.FieldKeyBaselineDurationSeconds,
		objects.FieldKeyFastestDurationSeconds, objects.FieldKeySlowestDurationSeconds,
	}
}

// isMetricKind returns true for audit_aggregation_metric and *_metric kinds.
func isMetricKind(kind string) bool {
	return kind == objects.KindAuditAggregationMetric ||
		(len(kind) > 7 && kind[len(kind)-7:] == "_metric")
}

func loadStreamDeltaConfig(projectRoot string) streamDeltaFieldsByKind {
	streamDeltaConfigMu.RLock()
	c, ok := streamDeltaConfig[projectRoot]
	streamDeltaConfigMu.RUnlock()
	if ok {
		return c
	}

	configPath := os.Getenv(zqkenv.StreamDeltaFieldsConfig())
	if configPath == emptyValue {
		processBase := paths.ResolvePathFromCacheOrConstant(projectRoot, "process", paths.ProcessDir)
		configPath = filepath.Join(processBase, "_internal", "configs", streamDeltaFieldsConfigFile)
	}
	data, err := fileutil.ReadFile(configPath)
	if err != nil {
		streamDeltaConfigMu.Lock()
		streamDeltaConfig[projectRoot] = nil // cache miss so we don't retry every time
		streamDeltaConfigMu.Unlock()
		return nil
	}

	var raw map[string][]string
	if err := yaml.Unmarshal(data, &raw); err != nil {
		streamDeltaConfigMu.Lock()
		streamDeltaConfig[projectRoot] = nil
		streamDeltaConfigMu.Unlock()
		return nil
	}

	streamDeltaConfigMu.Lock()
	streamDeltaConfig[projectRoot] = raw
	streamDeltaConfigMu.Unlock()
	return raw
}
