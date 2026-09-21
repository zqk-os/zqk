// Package storage: load stream delta field lists from config and/or object specs for recordForStream.
// Prefer spec-derived fields (storage_role: runtime_delta) when present; fall back to
// stream_delta_fields.yaml and built-in defaults. See DATA_STORAGE_PRODUCTION_ROADMAP.md §4.

package storage

import (
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const streamDeltaFieldsConfigFile = "stream_delta_fields.yaml"

var (
	streamDeltaConfigs stampmemo.Table[streamDeltaFieldsByKind] // keyed by projectRoot
	specDeltaFields    stampmemo.Table[[]string]                // keyed by projectRoot+\0+kind
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
	if projectRoot != emptyValue && (kind == objects.KindChangeJournalEntry || isMetricKind(kind)) {
		fromSpec := getRuntimeDeltaFieldsFromSpec(projectRoot, kind)
		base = mergeSpecRuntimeDeltaInto(base, fromSpec)
	}
	return base
}

func getRuntimeDeltaFieldsFromSpec(projectRoot, kind string) []string {
	internalBase := paths.ResolvePathFromCacheOrConstant(projectRoot, ConstStreamProcessInternal, paths.ProcessInternalDir)
	specsDir := filepath.Join(internalBase, "object_specs")
	specPath := paths.FindDomainFile(specsDir, kind+".yaml")
	names, _ := specDeltaFields.Load(projectRoot+"\x00"+kind, stampmemo.Of(specPath), func() ([]string, error) {
		return readRuntimeDeltaFieldsFromSpec(specsDir, kind), nil
	})
	return names
}

func readRuntimeDeltaFieldsFromSpec(specsDir, kind string) []string {
	loader := objects.NewSpecLoader(specsDir)
	spec, err := loader.LoadSpecWithInheritance(kind + ".yaml")
	if err != nil || objects.SpecResolvedFieldsMissing(spec) {
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
	return names
}

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

func isMetricKind(kind string) bool {
	return kind == objects.KindAuditAggregationMetric ||
		(len(kind) > 7 && kind[len(kind)-7:] == "_metric")
}

func loadStreamDeltaConfig(projectRoot string) streamDeltaFieldsByKind {
	cands := streamDeltaConfigCandidates(projectRoot)
	cfg, _ := streamDeltaConfigs.Load(projectRoot, stampmemo.OfAll(cands...), func() (streamDeltaFieldsByKind, error) {
		path := stampmemo.FirstExisting(cands)
		if path == emptyValue {
			return nil, nil
		}
		data, err := fileutil.ReadFile(path)
		if err != nil {
			return nil, nil
		}
		var raw map[string][]string
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, nil
		}
		return raw, nil
	})
	return cfg
}

func streamDeltaConfigCandidates(projectRoot string) []string {
	if configPath := zqkenv.StreamDeltaFieldsConfig().Get(); configPath != emptyValue {
		return []string{configPath}
	}
	return processInternalYAMLConfigCandidates(projectRoot, streamDeltaFieldsConfigFile)
}
