package datacell

import (
	"sort"
)

// Scheduler job_type wire strings (pkg/scheduler JobType* constants). Duplicated here so pkg/datacell
// never imports pkg/scheduler (import cycle).
const (
	schedulerJobWireAggregationMetricsCleanup  = "aggregation_metrics_cleanup"
	schedulerJobWireAuditEventAggregation      = "audit_event_aggregation"
	schedulerJobWireCacheInvalidation          = "cache_invalidation"
	schedulerJobWireCachePrewarm               = "cache_prewarm"
	schedulerJobWireChangeJournalAggregation   = "change_journal_aggregation"
	schedulerJobWireCleanup                    = "cleanup"
	schedulerJobWireConvergenceSessionTick     = "convergence_session_tick"
	schedulerJobWireIntegrityCheck             = "integrity_check"
	schedulerJobWireLifecycleCheck             = "lifecycle_check"
	schedulerJobWireMaintenance                = "maintenance"
	schedulerJobWireMetricsCollection          = "metrics_collection"
	schedulerJobWireObjectValidation           = "object_validation"
	schedulerJobWireRetentionTolerance         = "retention_tolerance"
	schedulerJobWireSchedulerEventsAggregation = "scheduler_events_aggregation"
)

// envelopeTokenSchedulerJobType maps operational envelope discovery tokens (see operational_envelope.go)
// to persisted scheduler job_type values when a stable binding exists. Tokens omitted are documentation-only
// or future RPC hooks — see [EnvelopeTokensDocumentationOnly].
var envelopeTokenSchedulerJobType = map[string]string{
	// CAS profile defaults
	"cas_reconcile":               schedulerJobWireCleanup,
	"overlay_trim":                schedulerJobWireCleanup,
	"lifecycle_status":            schedulerJobWireRetentionTolerance,
	"archival_when_set":           schedulerJobWireRetentionTolerance,
	"spec_validation":             schedulerJobWireObjectValidation,
	"deploy_gates":                schedulerJobWireObjectValidation,
	"instance_validation_batches": schedulerJobWireObjectValidation,
	"validation_pipeline":         schedulerJobWireObjectValidation,
	"spec_derived_indexes":        schedulerJobWireCachePrewarm,
	"materialized_caches":         schedulerJobWireCachePrewarm,
	"convergence_sessions":        schedulerJobWireConvergenceSessionTick,

	// Light file profile defaults
	"unversioned_local":     schedulerJobWireRetentionTolerance,
	"json_schema_checks":    schedulerJobWireIntegrityCheck,
	"config_reads":          schedulerJobWireCachePrewarm,
	"file_presence":         schedulerJobWireLifecycleCheck,
	"hook_profile_validity": schedulerJobWireLifecycleCheck,

	// Stream profile defaults
	"segment_rotation":            schedulerJobWireMaintenance,
	"stream_summary_rollups":      schedulerJobWireSchedulerEventsAggregation,
	"segment_retention_policy":    schedulerJobWireRetentionTolerance,
	"high_volume_kinds_alignment": schedulerJobWireObjectValidation,
	"stream_registry_checks":      schedulerJobWireLifecycleCheck,
	"runtime_delta_overlay":       schedulerJobWireCacheInvalidation,
	"stream_current_resolution":   schedulerJobWireCachePrewarm,
	"test_bundle_health_jsonl":    schedulerJobWireConvergenceSessionTick,
	"scheduler_test_bundles":      schedulerJobWireConvergenceSessionTick,

	// Kind augments (audit_event, metrics, sessions, journal, …)
	"high_volume_event_cache":            schedulerJobWireCachePrewarm,
	"stale_window_prune":                 schedulerJobWireAggregationMetricsCleanup,
	"aggregation_window_boundaries":      schedulerJobWireChangeJournalAggregation,
	"rollup_segment_index":               schedulerJobWireCachePrewarm,
	"metric_kind_family_contract":        schedulerJobWireMetricsCollection,
	"sampler_emission_paths":             schedulerJobWireMetricsCollection,
	"high_volume_metric_alignment":       schedulerJobWireObjectValidation,
	"metric_profiles_overlay":            schedulerJobWireCachePrewarm,
	"scheduler_daemon_self_heal":         schedulerJobWireLifecycleCheck,
	"cron_restart_signals":               schedulerJobWireLifecycleCheck,
	"missed_trigger_detection":           schedulerJobWireLifecycleCheck,
	"recovery_event_correlation":         schedulerJobWireLifecycleCheck,
	"lint_gate_rollups":                  schedulerJobWireObjectValidation,
	"policy_signal_alignment":            schedulerJobWireObjectValidation,
	"cli_subcommand_latency_sampling":    schedulerJobWireMetricsCollection,
	"command_schema_drift":               schedulerJobWireObjectValidation,
	"stale_lock_reaper_probe":            schedulerJobWireCleanup,
	"fs_advisory_lock_witness":           schedulerJobWireLifecycleCheck,
	"dynamic_kind_mapping_drift":         schedulerJobWireObjectValidation,
	"ontology_alias_checks":              schedulerJobWireObjectValidation,
	"mcp_transport_liveness":             schedulerJobWireLifecycleCheck,
	"session_ttl_boundaries":             schedulerJobWireRetentionTolerance,
	"fixture_window_alignment":           schedulerJobWireObjectValidation,
	"test_metric_segment_overlay":        schedulerJobWireCachePrewarm,
	"matrix_row_coverage_gates":          schedulerJobWireObjectValidation,
	"criterion_evidence_links":           schedulerJobWireObjectValidation,
	"session_recovery_hooks":             schedulerJobWireLifecycleCheck,
	"tray_runtime_manifest_reads":        schedulerJobWireCachePrewarm,
	"scheduler_job_execution_events":     schedulerJobWireSchedulerEventsAggregation,
	"trigger_queue_fairness":             schedulerJobWireSchedulerEventsAggregation,
	"scheduler_job_persisted_state":      schedulerJobWireObjectValidation,
	"wal_write_behind_coordination":      schedulerJobWireMaintenance,
	"change_journal_aggregation_windows": schedulerJobWireChangeJournalAggregation,
	"compaction_micro_gc_cjournal":       schedulerJobWireChangeJournalAggregation,
}

// EnvelopeTokensDocumentationOnly lists envelope discovery tokens that intentionally have no scheduler
// job_type binding (cold/archive/export operators, callbacks, purely local edits). Keep in sync when
// adding envelope tokens — [TestEnvelopeDiscoveryTokensMappedOrDocumentationOnly] guards drift.
var EnvelopeTokensDocumentationOnly = map[string]struct{}{
	"aggregation_metric_cold_tier":     {},
	"audit_stream_cold_export_lineage": {},
	"callback_logs":                    {},
	"cjournal_segment_cold_archive":    {},
	"content_addressed_shards":         {},
	"metric_rollups_cold_tier":         {},
	"operator_local_edits":             {},
	"optional_export":                  {},
	"scheduler_health_cold_snapshot":   {},
	"segment_cold_storage_optional":    {},
	"test_metric_fixture_cold_export":  {},
}

// SchedulerJobTypeForEnvelopeToken returns the persisted scheduler job_type for an envelope token when bound.
func SchedulerJobTypeForEnvelopeToken(token string) (jobType string, ok bool) {
	if token == "" {
		return "", false
	}
	jt, ok := envelopeTokenSchedulerJobType[token]
	return jt, ok
}

// EnvelopeTokenJobEdge pairs one operational envelope discovery token with its scheduler job_type binding.
type EnvelopeTokenJobEdge struct {
	Token   string
	JobType string
}

// ResolvedEnvelopeTokenJobEdges returns every (token, job_type) edge with a binding, in token iteration order.
func ResolvedEnvelopeTokenJobEdges() []EnvelopeTokenJobEdge {
	tokens := AllOperationalEnvelopeDiscoveryTokens()
	var edges []EnvelopeTokenJobEdge
	for _, tok := range tokens {
		jt, ok := SchedulerJobTypeForEnvelopeToken(tok)
		if !ok {
			continue
		}
		edges = append(edges, EnvelopeTokenJobEdge{Token: tok, JobType: jt})
	}
	return edges
}

// ResolvedSchedulerJobTypesFromTokenEdgesWithPolicy builds sorted unique job types from edges after optional
// token deny list and optional restrictive allow list.
// denyTokens: when non-empty, edges whose Token is present are dropped.
// allowConfigured: false — no allow-side filtering. true — allowTokens controls which edges contribute (whitelist).
// When allowConfigured is true and allowTokens is empty, no edges pass (explicit empty whitelist).
func ResolvedSchedulerJobTypesFromTokenEdgesWithPolicy(edges []EnvelopeTokenJobEdge, denyTokens map[string]struct{}, allowTokens map[string]struct{}, allowConfigured bool) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, e := range edges {
		if len(denyTokens) > 0 {
			if _, denied := denyTokens[e.Token]; denied {
				continue
			}
		}
		if allowConfigured {
			if len(allowTokens) == 0 {
				continue
			}
			if _, ok := allowTokens[e.Token]; !ok {
				continue
			}
		}
		if _, dup := seen[e.JobType]; dup {
			continue
		}
		seen[e.JobType] = struct{}{}
		out = append(out, e.JobType)
	}
	sort.Strings(out)
	return out
}

// ResolvedSchedulerJobTypesFromDiscoveryTokens returns sorted unique scheduler job_type strings implied by
// all v1 envelope tokens (profile + kind augments) that have a binding in [envelopeTokenSchedulerJobType].
func ResolvedSchedulerJobTypesFromDiscoveryTokens() []string {
	return ResolvedSchedulerJobTypesFromTokenEdgesWithPolicy(ResolvedEnvelopeTokenJobEdges(), nil, nil, false)
}
