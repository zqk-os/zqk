package datacell

import (
	"sort"
	"strings"
)

// OperationalEnvelopeSummary lists stable subsystem tokens for operator discovery (DATA_CELL_MODEL
// “operational envelope”). This is a v1 static map keyed by storage profile. Persisted scheduler
// integration: job_type data_cell_envelope_tick (e.g. SCH-dce-tick from scheduler_maintenance_config).
// Per-kind overrides (remove/replace profile defaults) and augments merge in [OperationalEnvelopeForKind]
// (override first, then augments). Richer per-token RPC bindings remain roadmap.
type OperationalEnvelopeSummary struct {
	Profile StorageProfile `json:"profile"`
	// SchedulerCategory is the reserved scheduler_job category for envelope-scoped work (v1: documentation + dry-run + data_cell_envelope_tick; see pkg/scheduler.DryRunDataCellEnvelopePolicy and handlers_datacell_envelope_tick.go).
	SchedulerCategory string `json:"scheduler_category,omitempty"`
	// EnvelopeTickSchedulerJobID is the persisted maintenance job that runs the envelope tick (observability / contract pointer).
	EnvelopeTickSchedulerJobID string `json:"envelope_tick_scheduler_job_id,omitempty"`
	// EnvelopeTickJobType is scheduler_job.job_type for that tick (data_cell_envelope_tick).
	EnvelopeTickJobType string `json:"envelope_tick_job_type,omitempty"`
	// Each slice holds short snake_case tokens naming responsibilities (docs + code pointers, not RPC names).
	Cleanup   []string `json:"cleanup,omitempty"`
	Retention []string `json:"retention,omitempty"`
	Scan      []string `json:"scan,omitempty"`
	Cache     []string `json:"cache,omitempty"`
	Archive   []string `json:"archive,omitempty"`
	Health    []string `json:"health,omitempty"`
}

// operationalEnvelopeAugment holds optional per-(kind,profile) tokens merged onto profile defaults
// in [OperationalEnvelopeForKind]. Any field may be empty.
type operationalEnvelopeAugment struct {
	ExtraCleanup   []string
	ExtraRetention []string
	ExtraScan      []string
	ExtraCache     []string
	ExtraArchive   []string
	ExtraHealth    []string
}

// kindOperationalEnvelopeAugments adds tokens for specific (kind, profile) pairs on top of
// [operationalEnvelopes]. Empty slices are omitted at merge time.
var kindOperationalEnvelopeAugments = map[string]map[StorageProfile]operationalEnvelopeAugment{
	"audit_event": {
		ProfileStream: {
			ExtraHealth:  []string{"high_volume_event_cache"},
			ExtraArchive: []string{"audit_stream_cold_export_lineage"},
		},
	},
	"audit_aggregation_metric": {
		ProfileStream: {
			ExtraCleanup:   []string{"stale_window_prune"},
			ExtraRetention: []string{"aggregation_window_boundaries"},
			ExtraCache:     []string{"rollup_segment_index"},
			ExtraArchive:   []string{"aggregation_metric_cold_tier"},
		},
	},
	"base_metric": {
		ProfileStream: {
			ExtraHealth:  []string{"metric_kind_family_contract", "sampler_emission_paths"},
			ExtraScan:    []string{"high_volume_metric_alignment"},
			ExtraCache:   []string{"metric_profiles_overlay"},
			ExtraArchive: []string{"metric_rollups_cold_tier"},
		},
	},
	"scheduler_health_metric": {
		ProfileStream: {
			ExtraHealth:  []string{"scheduler_daemon_self_heal", "cron_restart_signals"},
			ExtraScan:    []string{"missed_trigger_detection", "recovery_event_correlation"},
			ExtraArchive: []string{"scheduler_health_cold_snapshot"},
		},
	},
	// Remaining stream kinds from spec_index (high_volume / session / metric siblings).
	"code_quality_metric": {
		ProfileStream: {
			ExtraScan: []string{"lint_gate_rollups", "policy_signal_alignment"},
		},
	},
	"command_metric": {
		ProfileStream: {
			ExtraHealth: []string{"cli_subcommand_latency_sampling"},
			ExtraScan:   []string{"command_schema_drift"},
		},
	},
	"file_lock_metric": {
		ProfileStream: {
			ExtraCleanup: []string{"stale_lock_reaper_probe"},
			ExtraHealth:  []string{"fs_advisory_lock_witness"},
		},
	},
	"kind_mapping_metric": {
		ProfileStream: {
			ExtraScan: []string{"dynamic_kind_mapping_drift", "ontology_alias_checks"},
		},
	},
	"mcp_session": {
		ProfileStream: {
			ExtraHealth:    []string{"mcp_transport_liveness"},
			ExtraRetention: []string{"session_ttl_boundaries"},
		},
	},
	"test_audit_aggregation_metric": {
		ProfileStream: {
			ExtraScan:    []string{"fixture_window_alignment"},
			ExtraCache:   []string{"test_metric_segment_overlay"},
			ExtraArchive: []string{"test_metric_fixture_cold_export"},
		},
	},
	"verification_matrix": {
		ProfileStream: {
			ExtraScan: []string{"matrix_row_coverage_gates", "criterion_evidence_links"},
		},
	},
	"zqk_session": {
		ProfileStream: {
			ExtraHealth: []string{"session_recovery_hooks"},
			ExtraCache:  []string{"tray_runtime_manifest_reads"},
		},
	},
	"scheduler_job": {
		ProfileStream: {
			ExtraScan: []string{"scheduler_job_execution_events", "trigger_queue_fairness"},
		},
		ProfileCASEntity: {
			ExtraScan: []string{"scheduler_job_persisted_state"},
		},
	},
	"change_journal_entry": {
		// Spec: stream (high_volume_kinds); CAS-only path was stale — augments apply to stream profile.
		ProfileStream: {
			ExtraHealth:  []string{"wal_write_behind_coordination", "change_journal_aggregation_windows"},
			ExtraScan:    []string{"compaction_micro_gc_cjournal"},
			ExtraArchive: []string{"cjournal_segment_cold_archive"},
		},
	},
}

// operationalEnvelopeStreamArchiveSegmentColdOptional is the stream profile’s default archive token
// ([operationalEnvelopes] ProfileStream). Referenced by per-kind overrides so removals stay aligned.
const operationalEnvelopeStreamArchiveSegmentColdOptional = "segment_cold_storage_optional"

// operationalEnvelopeKindOverride adjusts profile-default tokens before per-kind augments are applied.
// Remove* subtracts tokens from the profile envelope for that (kind, profile). Replace*, when non-nil,
// substitutes the category slice after removals (empty slice clears the category).
type operationalEnvelopeKindOverride struct {
	RemoveCleanup    []string
	RemoveRetention  []string
	RemoveScan       []string
	RemoveCache      []string
	RemoveArchive    []string
	RemoveHealth     []string
	AddCleanup       []string
	AddRetention     []string
	AddScan          []string
	AddCache         []string
	AddArchive       []string
	AddHealth        []string
	ReplaceCleanup   *[]string
	ReplaceRetention *[]string
	ReplaceScan      *[]string
	ReplaceCache     *[]string
	ReplaceArchive   *[]string
	ReplaceHealth    *[]string
}

func (o operationalEnvelopeKindOverride) empty() bool {
	return len(o.RemoveCleanup) == 0 && len(o.RemoveRetention) == 0 && len(o.RemoveScan) == 0 &&
		len(o.RemoveCache) == 0 && len(o.RemoveArchive) == 0 && len(o.RemoveHealth) == 0 &&
		len(o.AddCleanup) == 0 && len(o.AddRetention) == 0 && len(o.AddScan) == 0 &&
		len(o.AddCache) == 0 && len(o.AddArchive) == 0 && len(o.AddHealth) == 0 &&
		o.ReplaceCleanup == nil && o.ReplaceRetention == nil && o.ReplaceScan == nil &&
		o.ReplaceCache == nil && o.ReplaceArchive == nil && o.ReplaceHealth == nil
}

// kindOperationalEnvelopeOverrides removes or replaces profile-default tokens for specific
// (kind, profile) pairs before [kindOperationalEnvelopeAugments] are merged.
var kindOperationalEnvelopeOverrides = map[string]map[StorageProfile]operationalEnvelopeKindOverride{
	"audit_event": {
		ProfileStream: {
			RemoveArchive: []string{operationalEnvelopeStreamArchiveSegmentColdOptional},
		},
	},
	"change_journal_entry": {
		ProfileStream: {
			RemoveScan: []string{"high_volume_kinds_alignment"},
		},
	},
}

var operationalEnvelopes = map[StorageProfile]OperationalEnvelopeSummary{
	ProfileCASEntity: {
		Profile: ProfileCASEntity,
		Cleanup: []string{"cas_reconcile", "overlay_trim"},
		Retention: []string{
			"lifecycle_status",
			"archival_when_set",
		},
		Scan: []string{
			"spec_validation",
			"deploy_gates",
			"instance_validation_batches",
		},
		Cache: []string{
			"spec_derived_indexes",
			"materialized_caches",
		},
		Archive: []string{"content_addressed_shards"},
		Health: []string{
			"validation_pipeline",
			"convergence_sessions",
		},
	},
	ProfileLightFile: {
		Profile: ProfileLightFile,
		Cleanup: []string{"operator_local_edits"},
		Retention: []string{
			"unversioned_local",
		},
		Scan: []string{
			"json_schema_checks",
		},
		Cache:   []string{"config_reads"},
		Archive: []string{"optional_export"},
		Health: []string{
			"file_presence",
			"hook_profile_validity",
		},
	},
	ProfileStream: {
		Profile: ProfileStream,
		Cleanup: []string{
			"segment_rotation",
			"stream_summary_rollups",
		},
		Retention: []string{
			"segment_retention_policy",
		},
		Scan: []string{
			"high_volume_kinds_alignment",
			"stream_registry_checks",
		},
		Cache: []string{
			"runtime_delta_overlay",
			"stream_current_resolution",
		},
		Archive: []string{operationalEnvelopeStreamArchiveSegmentColdOptional},
		Health: []string{
			"test_bundle_health_jsonl",
			"scheduler_test_bundles",
			"callback_logs",
		},
	},
}

// AllOperationalEnvelopeCompactSummaries returns [CompactSummary] for every entry in
// [KnownStorageProfiles] that has a v1 envelope map. Used by the scheduler
// data_cell_envelope_tick job so logs reflect the full three-profile model (CAS, light file, stream)
// in one place — operator visibility without duplicating profile lists at call sites.
func AllOperationalEnvelopeCompactSummaries() map[StorageProfile]string {
	out := make(map[StorageProfile]string, len(KnownStorageProfiles))
	for _, p := range KnownStorageProfiles {
		if env, ok := OperationalEnvelopeForProfile(p); ok {
			out[p] = env.CompactSummary()
		}
	}
	return out
}

// OperationalEnvelopeForProfile returns the v1 envelope summary when the profile is known and mapped.
func OperationalEnvelopeForProfile(p StorageProfile) (OperationalEnvelopeSummary, bool) {
	if !p.IsKnown() {
		return OperationalEnvelopeSummary{}, false
	}
	env, ok := operationalEnvelopes[p]
	if !ok {
		return OperationalEnvelopeSummary{}, false
	}
	env.SchedulerCategory = SchedulerCategoryDataCellEnvelope
	env.EnvelopeTickSchedulerJobID = EnvelopeTickSchedulerJobID
	env.EnvelopeTickJobType = EnvelopeTickJobType
	return env, true
}

// OperationalEnvelopeForKind returns the v1 envelope for a kind and storage profile: profile
// defaults, optional overrides from [kindOperationalEnvelopeOverrides], then augments from
// [kindOperationalEnvelopeAugments]. Use this for operator-facing per-row discovery (e.g. zqk system
// data-cells --json); table footers may still summarize by profile only.
func OperationalEnvelopeForKind(kind string, p StorageProfile) (OperationalEnvelopeSummary, bool) {
	base, ok := OperationalEnvelopeForProfile(p)
	if !ok {
		return OperationalEnvelopeSummary{}, false
	}
	if ov, ok := operationalEnvelopeOverrideFor(kind, p); ok {
		base = applyOperationalEnvelopeKindOverride(base, ov)
	}
	byKind, ok := kindOperationalEnvelopeAugments[kind]
	if !ok {
		return base, true
	}
	aug, ok := byKind[p]
	if !ok {
		return base, true
	}
	base.Cleanup = appendUniqueStrings(base.Cleanup, aug.ExtraCleanup...)
	base.Retention = appendUniqueStrings(base.Retention, aug.ExtraRetention...)
	base.Scan = appendUniqueStrings(base.Scan, aug.ExtraScan...)
	base.Cache = appendUniqueStrings(base.Cache, aug.ExtraCache...)
	base.Archive = appendUniqueStrings(base.Archive, aug.ExtraArchive...)
	base.Health = appendUniqueStrings(base.Health, aug.ExtraHealth...)
	return base, true
}

func operationalEnvelopeOverrideFor(kind string, p StorageProfile) (operationalEnvelopeKindOverride, bool) {
	byKind, ok := kindOperationalEnvelopeOverrides[kind]
	if !ok {
		return operationalEnvelopeKindOverride{}, false
	}
	ov, ok := byKind[p]
	if !ok || ov.empty() {
		return operationalEnvelopeKindOverride{}, false
	}
	return ov, true
}

func applyOperationalEnvelopeKindOverride(base OperationalEnvelopeSummary, ov operationalEnvelopeKindOverride) OperationalEnvelopeSummary {
	base.Cleanup = applyRemoveReplaceAdd(base.Cleanup, ov.RemoveCleanup, ov.ReplaceCleanup, ov.AddCleanup)
	base.Retention = applyRemoveReplaceAdd(base.Retention, ov.RemoveRetention, ov.ReplaceRetention, ov.AddRetention)
	base.Scan = applyRemoveReplaceAdd(base.Scan, ov.RemoveScan, ov.ReplaceScan, ov.AddScan)
	base.Cache = applyRemoveReplaceAdd(base.Cache, ov.RemoveCache, ov.ReplaceCache, ov.AddCache)
	base.Archive = applyRemoveReplaceAdd(base.Archive, ov.RemoveArchive, ov.ReplaceArchive, ov.AddArchive)
	base.Health = applyRemoveReplaceAdd(base.Health, ov.RemoveHealth, ov.ReplaceHealth, ov.AddHealth)
	return base
}

func applyRemoveReplaceAdd(slice []string, remove []string, replace *[]string, add []string) []string {
	out := subtractTokens(slice, remove)
	if replace != nil {
		out = append([]string(nil), (*replace)...)
	}
	if len(add) > 0 {
		out = appendUniqueStrings(out, add...)
	}
	return out
}

func subtractTokens(slice []string, remove []string) []string {
	if len(remove) == 0 {
		return slice
	}
	rm := make(map[string]struct{}, len(remove))
	for _, r := range remove {
		if r != "" {
			rm[r] = struct{}{}
		}
	}
	out := make([]string, 0, len(slice))
	for _, s := range slice {
		if _, drop := rm[s]; !drop {
			out = append(out, s)
		}
	}
	return out
}

func (a operationalEnvelopeAugment) nonEmpty() bool {
	return len(a.ExtraCleanup) > 0 || len(a.ExtraRetention) > 0 || len(a.ExtraScan) > 0 ||
		len(a.ExtraCache) > 0 || len(a.ExtraArchive) > 0 || len(a.ExtraHealth) > 0
}

// OperationalEnvelopeHasKindAugment reports whether [kindOperationalEnvelopeAugments] defines a
// non-empty augment for (kind, profile). Used by operator JSON (e.g. zqk system data-cells --json)
// to distinguish profile-only envelope rows from kind-augmented rows.
func OperationalEnvelopeHasKindAugment(kind string, p StorageProfile) bool {
	byKind, ok := kindOperationalEnvelopeAugments[kind]
	if !ok {
		return false
	}
	aug, ok := byKind[p]
	if !ok {
		return false
	}
	return aug.nonEmpty()
}

// OperationalEnvelopeHasKindOverride reports whether [kindOperationalEnvelopeOverrides] defines a
// non-empty override for (kind, profile).
func OperationalEnvelopeHasKindOverride(kind string, p StorageProfile) bool {
	_, ok := operationalEnvelopeOverrideFor(kind, p)
	return ok
}

// AllOperationalEnvelopeDiscoveryTokens returns every distinct v1 envelope token (profile defaults plus
// kind augments) for drift tests and scheduler job-resolution tables (see envelope_token_jobs.go).
func AllOperationalEnvelopeDiscoveryTokens() []string {
	seen := make(map[string]struct{})
	var out []string
	addTokens := func(env OperationalEnvelopeSummary) {
		for _, s := range [][]string{env.Cleanup, env.Retention, env.Scan, env.Cache, env.Archive, env.Health} {
			for _, t := range s {
				if t == "" {
					continue
				}
				if _, ok := seen[t]; ok {
					continue
				}
				seen[t] = struct{}{}
				out = append(out, t)
			}
		}
	}
	for _, p := range KnownStorageProfiles {
		if env, ok := OperationalEnvelopeForProfile(p); ok {
			addTokens(env)
		}
	}
	kindUnion := make(map[string]struct{})
	for k := range kindOperationalEnvelopeAugments {
		kindUnion[k] = struct{}{}
	}
	for k := range kindOperationalEnvelopeOverrides {
		kindUnion[k] = struct{}{}
	}
	for kind := range kindUnion {
		for _, p := range KnownStorageProfiles {
			if env, ok := OperationalEnvelopeForKind(kind, p); ok {
				addTokens(env)
			}
		}
	}
	sort.Strings(out)
	return out
}

// EnvelopeTickKindOverrideSummaries returns one-line per-(kind,profile) summaries for entries in
// [kindOperationalEnvelopeOverrides], sorted by kind then profile — for scheduler tick logging.
func EnvelopeTickKindOverrideSummaries() []string {
	kinds := sortedMapKeys(kindOperationalEnvelopeOverrides)
	out := make([]string, 0, len(kinds)*2)
	profiles := []StorageProfile{ProfileCASEntity, ProfileLightFile, ProfileStream}
	for _, kind := range kinds {
		byProf := kindOperationalEnvelopeOverrides[kind]
		for _, p := range profiles {
			ov, ok := byProf[p]
			if !ok || ov.empty() {
				continue
			}
			if env, ok := OperationalEnvelopeForKind(kind, p); ok {
				out = append(out, kind+"@"+string(p)+"="+env.CompactSummary())
			}
		}
	}
	return out
}

func appendUniqueStrings(base []string, add ...string) []string {
	seen := make(map[string]struct{}, len(base)+len(add))
	for _, s := range base {
		if s != "" {
			seen[s] = struct{}{}
		}
	}
	for _, s := range add {
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		base = append(base, s)
	}
	return base
}

// CompactSummary joins non-empty categories for table / log lines (single line, semicolon-separated).
// When SchedulerCategory is set (see OperationalEnvelopeForProfile), it is listed first for operator discovery.
func (s OperationalEnvelopeSummary) CompactSummary() string {
	var b strings.Builder
	first := true
	if cat := strings.TrimSpace(s.SchedulerCategory); cat != "" {
		b.WriteString("scheduler=")
		b.WriteString(cat)
		first = false
	}
	emit := func(label string, vals []string) {
		if len(vals) == 0 {
			return
		}
		if !first {
			b.WriteString("; ")
		}
		first = false
		b.WriteString(label)
		b.WriteByte('=')
		b.WriteString(strings.Join(vals, ","))
	}
	emit("cleanup", s.Cleanup)
	emit("retention", s.Retention)
	emit("scan", s.Scan)
	emit("cache", s.Cache)
	emit("archive", s.Archive)
	emit("health", s.Health)
	if id := strings.TrimSpace(s.EnvelopeTickSchedulerJobID); id != "" {
		if b.Len() > 0 {
			b.WriteString("; ")
		}
		b.WriteString("envelope_tick_job=")
		b.WriteString(id)
		if jt := strings.TrimSpace(s.EnvelopeTickJobType); jt != "" {
			b.WriteByte('@')
			b.WriteString(jt)
		}
	}
	return b.String()
}

// EnvelopeTickKindAugmentSummaries returns one-line per-(kind,profile) envelope summaries for
// entries in [kindOperationalEnvelopeAugments], sorted by kind then profile — for scheduler
// data_cell_envelope_tick logging.
func EnvelopeTickKindAugmentSummaries() []string {
	kinds := sortedMapKeys(kindOperationalEnvelopeAugments)
	out := make([]string, 0, len(kinds)*2)
	profiles := []StorageProfile{ProfileCASEntity, ProfileLightFile, ProfileStream}
	for _, kind := range kinds {
		byProf := kindOperationalEnvelopeAugments[kind]
		for _, p := range profiles {
			if _, has := byProf[p]; !has {
				continue
			}
			if env, ok := OperationalEnvelopeForKind(kind, p); ok {
				out = append(out, kind+"@"+string(p)+"="+env.CompactSummary())
			}
		}
	}
	return out
}

func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
