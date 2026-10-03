package objects

import (
	"fmt"
	"go/format"
	"sort"
	"strings"
	"unicode"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
)

// supplementalFieldKeyNames are map keys used across the codebase that may not appear
// in every spec index pass (e.g. cross-cutting keys, transitional paths). Unioned with
// spec-derived keys when generating field_keys.go.
var supplementalFieldKeyNames = []string{
	// Keys referenced in code but not present in object_specs ResolvedFields (e.g. traceability-only paths).
	"kind_under_test",
	"test_functions",
	"visibility",
	"effort_variance",
	"auth_strategy_ref",
	"method",
	"payload_schema",
	"source_url",
	"target_system",
	"timeout_ms",
	"translator_id",
	// base_metric extensions used by scheduler autofix batch metrics (not in base_metric.yaml fields).
	"batch_id",
	"processed",
	"fixed",
	"failed",
	"skipped",
	"duration_seconds",
	// base_metric JSON payload snapshots from async collectors.
	"cas_validation_cache_metrics_json",
	"graph_provider_metrics_json",
	"validation_metrics_json",
	"operation_id",
	// scenario builder synthetic fixture fields.
	"test_type",
	"expected_result",
	"old_value",
	// scheduler_job trigger fields (present on persisted jobs; ensure union even if spec index omits).
	"event_filter",
	"lifecycle_filter",
	// Referenced by spec builder / comprehensive tests; not in current object_specs union.
	"origin_lifecycle",
	"capability_tokens",
	"lifecycle_file",
	"lifecycle_kinds",
	// Leftover mixed path/ID list on backlog_item; kernel pointers are doc_entry_refs.
	"document_refs",
	// Cross-plan scheduler and orchestration payloads may reference more than one plan.
	"priority_plan_refs",
	"milestone_ref",
	// Nested lifecycle.statuses[] entry label (see *_lifecycle.yaml); key is the short name "display".
	// Layer 0 cellular microkernel foundational fields
	"enclave_scope",
	"epic_refs",
	"plane",
	"provenance",
	"schema_ref",
	"urn",
	// CVS measurement JSONL (cvs_measurement_events.jsonl) + convergence after_state_snapshot keys
	// used by convergence_session_tick / scheduler convergence measure (not always in object_specs union).
	"agent_prompt_cli",
	"next_action_hint",
	"primary_measurement_outcome",
	"primary_measurement_outcome_detail",
	"ready_for_session_completion",
	"session_id",
	"tick_job_id",
	"watermark",
	"asset_type",
	"asset_url_or_path",
	"brand_asset_refs",
	"brand_ref",
	"conversion_goal",
	"key_messages",
	"narrative_refs",
	"story_arc",
	"target_audience",
	"tone",
	"usage_guidelines",
	"instructions",
	"marketing_vision",
	"project_context",
	"scenes",
	"shots",
	"storage",
	"quality_metrics",
	"lineage",
	"verified_artifact_refs",
	"verification_hash",
	"verification_suites",
	"source_hash",
	"source_mtime",
	"last_checked_at",
	"phase",
	"session_mode",
	"term_type",
	"max_units",
	"consumed_units",
	"consumer_kernel_ref",
	"resource_ref",
	"token_id",
	"agreement_mode",
	"skills",
	"condition_query",
	"frequency",
	"heartbeat_enabled",
	"vision_ref",
	"notify_target_ref",
	"validation_overlays",
	"task_steps",
	"verification_attempts",
	"verification_feedback",
	"interjections",
	"resolved_related_object_refs",
	"status_history",
	"change_log",
	"artifacts",
}

// fieldKeyAcronyms maps lowercase spec segments to Go identifier fragments (for stable FieldKey* names).
var fieldKeyAcronyms = map[string]string{
	FieldKeyID: "ID",
	"ids":      "IDs",
	"api":      "API",
	"url":      "URL",
	"uri":      "URI",
	"html":     "HTML",
	"json":     "JSON",
	"yaml":     "YAML",
	"sql":      "SQL",
}

// UnionFieldNamesFromSpecIndex returns the sorted union of all field names across kinds.
func UnionFieldNamesFromSpecIndex(idx *SpecIndex) []string {
	if specIndexKindsMissing(idx) {
		return nil
	}
	set := make(map[string]struct{})
	for _, ks := range idx.Kinds {
		for _, f := range ks.Fields {
			if f.Name != emptyValue {
				set[f.Name] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// FieldKeyConstName returns the Go identifier suffix after "FieldKey" for a YAML field name,
// e.g. priority_plan_ref -> PriorityPlanRef, id -> ID.
func FieldKeyConstName(fieldName string) string {
	segs := strings.Split(fieldName, "_")
	var b strings.Builder
	for _, s := range segs {
		if s == emptyValue {
			continue
		}
		low := strings.ToLower(s)
		if frag, ok := fieldKeyAcronyms[low]; ok {
			b.WriteString(frag)
			continue
		}
		runes := []rune(strings.ToLower(s))
		runes[0] = unicode.ToUpper(runes[0])
		b.WriteString(string(runes))
	}
	return b.String()
}

// BuildUnionFieldKeyNames loads object_specs from specsDir and returns the sorted union of
// all field names (same basis as BuildSpecIndexFromSpecsDir), merged with supplementalFieldKeyNames.
func BuildUnionFieldKeyNames(specsDir string) ([]string, error) {
	idx, err := BuildSpecIndexFromSpecsDir(specsDir)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{})
	for _, k := range UnionFieldNamesFromSpecIndex(idx) {
		set[k] = struct{}{}
	}
	for _, k := range supplementalFieldKeyNames {
		if k != emptyValue {
			set[k] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

// GenerateFieldKeysGoSource returns formatted Go source for field_keys.go from the union of
// all spec field names (plus supplementalFieldKeyNames).
func GenerateFieldKeysGoSource(specsDir string) ([]byte, error) {
	names, err := BuildUnionFieldKeyNames(specsDir)
	if err != nil {
		return nil, err
	}

	header := fmt.Sprintf(`package objects

// Code generated by %q. DO NOT EDIT BY HAND.
// To regenerate after object spec changes:
//   %s
//
// Canonical keys for object instance fields (map/JSON). Union of ResolvedFields across all
// kinds under object_specs, plus supplementalFieldKeyNames in field_keys_generate.go.
// Canonical kind strings live in pkg/kindnames (re-exported as objects.Kind* in constants.go).

const (
`, paths.CLIUsage("system", "generate-field-keys"), paths.CLIUsage("system", "generate-field-keys"))

	var body strings.Builder
	body.WriteString(header)

	// Detect duplicate const names (different YAML keys mapping to same Go name).
	used := make(map[string]string)
	for _, name := range names {
		suffix := FieldKeyConstName(name)
		if suffix == emptyValue {
			continue
		}
		full := "FieldKey" + suffix
		if prev, ok := used[full]; ok {
			return nil, errfmt.Errorf("duplicate generated const %s for field keys %q and %q", full, prev, name)
		}
		used[full] = name
	}

	for _, name := range names {
		suffix := FieldKeyConstName(name)
		if suffix == emptyValue {
			continue
		}
		fmt.Fprintf(&body, "\tFieldKey%s = %q\n", suffix, name)
	}

	body.WriteString(")\n")
	return FormatGeneratedGoSource([]byte(body.String()))
}

// FormatGeneratedGoSource formats generated Go source code using standard go/format.
func FormatGeneratedGoSource(source []byte) ([]byte, error) {
	out, err := format.Source(source)
	if err != nil {
		return nil, errfmt.Errorf("format generated source: %w", err)
	}
	return out, nil
}
