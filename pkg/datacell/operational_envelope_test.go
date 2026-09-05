package datacell

import (
	"strings"
	"testing"
)

func TestOperationalEnvelopeForProfile_known(t *testing.T) {
	t.Parallel()
	env, ok := OperationalEnvelopeForProfile(ProfileStream)
	if !ok {
		t.Fatal("expected stream envelope")
	}
	if env.Profile != ProfileStream {
		t.Fatalf("profile: %v", env.Profile)
	}
	if len(env.Health) == 0 || len(env.Cleanup) == 0 {
		t.Fatalf("expected non-empty slices: %#v", env)
	}
	if env.SchedulerCategory != SchedulerCategoryDataCellEnvelope {
		t.Fatalf("scheduler_category: %q", env.SchedulerCategory)
	}
	if env.EnvelopeTickSchedulerJobID != EnvelopeTickSchedulerJobID {
		t.Fatalf("envelope_tick_scheduler_job_id: %q", env.EnvelopeTickSchedulerJobID)
	}
	if env.EnvelopeTickJobType != EnvelopeTickJobType {
		t.Fatalf("envelope_tick_job_type: %q", env.EnvelopeTickJobType)
	}
}

func TestOperationalEnvelopeForProfile_unknown(t *testing.T) {
	t.Parallel()
	if _, ok := OperationalEnvelopeForProfile(StorageProfile("")); ok {
		t.Fatal("expected false for empty profile")
	}
	if _, ok := OperationalEnvelopeForProfile(StorageProfile("other")); ok {
		t.Fatal("expected false for unknown profile")
	}
}

func TestOperationalEnvelopeForKind_auditEventAugment(t *testing.T) {
	t.Parallel()
	base, ok := OperationalEnvelopeForProfile(ProfileStream)
	if !ok {
		t.Fatal("base stream")
	}
	kind, ok := OperationalEnvelopeForKind("audit_event", ProfileStream)
	if !ok {
		t.Fatal("kind envelope")
	}
	if len(kind.Health) <= len(base.Health) {
		t.Fatalf("expected extra health tokens for audit_event: base=%d kind=%d", len(base.Health), len(kind.Health))
	}
	found := false
	for _, h := range kind.Health {
		if h == "high_volume_event_cache" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing augment token: %#v", kind.Health)
	}
	// Unknown kinds behave like profile-only.
	plain, ok := OperationalEnvelopeForKind("backlog_item", ProfileCASEntity)
	if !ok {
		t.Fatal("cas")
	}
	plainBase, _ := OperationalEnvelopeForProfile(ProfileCASEntity)
	if len(plain.Health) != len(plainBase.Health) {
		t.Fatalf("unexpected augment for backlog_item: %#v", plain.Health)
	}
}

func TestOperationalEnvelopeSummary_CompactSummary(t *testing.T) {
	t.Parallel()
	env := OperationalEnvelopeSummary{
		Profile: ProfileCASEntity,
		Scan:    []string{"a", "b"},
		Health:  []string{"c"},
	}
	s := env.CompactSummary()
	if !strings.Contains(s, "scan=a,b") || !strings.Contains(s, "health=c") {
		t.Fatalf("unexpected: %q", s)
	}

	withSched := OperationalEnvelopeSummary{
		SchedulerCategory: SchedulerCategoryDataCellEnvelope,
		Scan:              []string{"x"},
	}
	s2 := withSched.CompactSummary()
	if !strings.HasPrefix(s2, "scheduler="+SchedulerCategoryDataCellEnvelope+";") {
		t.Fatalf("want scheduler first: %q", s2)
	}
}

func TestOperationalEnvelopeForKind_changeJournalStreamAugment(t *testing.T) {
	t.Parallel()
	base, ok := OperationalEnvelopeForProfile(ProfileStream)
	if !ok {
		t.Fatal("base stream")
	}
	kind, ok := OperationalEnvelopeForKind("change_journal_entry", ProfileStream)
	if !ok {
		t.Fatal("kind envelope")
	}
	if len(kind.Health) <= len(base.Health) {
		t.Fatalf("expected extra health tokens for change_journal_entry: base=%d kind=%d", len(base.Health), len(kind.Health))
	}
	// Scan: kind override removes one profile-default token and augments add compaction — length may tie base.
	for _, s := range kind.Scan {
		if s == "high_volume_kinds_alignment" {
			t.Fatalf("override should drop high_volume_kinds_alignment from stream scan; got %#v", kind.Scan)
		}
	}
	found := map[string]bool{}
	for _, h := range kind.Health {
		found[h] = true
	}
	for _, want := range []string{"wal_write_behind_coordination", "change_journal_aggregation_windows"} {
		if !found[want] {
			t.Fatalf("missing health augment %q: %#v", want, kind.Health)
		}
	}
	foundScan := false
	for _, s := range kind.Scan {
		if s == "compaction_micro_gc_cjournal" {
			foundScan = true
			break
		}
	}
	if !foundScan {
		t.Fatalf("missing scan augment: %#v", kind.Scan)
	}
	// CAS profile has no kind-specific augments for change_journal_entry (stream-only in spec).
	casPlain, ok := OperationalEnvelopeForKind("change_journal_entry", ProfileCASEntity)
	if !ok {
		t.Fatal("cas")
	}
	casBase, _ := OperationalEnvelopeForProfile(ProfileCASEntity)
	if len(casPlain.Health) != len(casBase.Health) || len(casPlain.Scan) != len(casBase.Scan) {
		t.Fatalf("unexpected augments on cas for change_journal_entry: %#v", casPlain)
	}
}

func TestOperationalEnvelopeForKind_auditAggregationMetric_extraCategories(t *testing.T) {
	t.Parallel()
	base, ok := OperationalEnvelopeForProfile(ProfileStream)
	if !ok {
		t.Fatal("base stream")
	}
	kind, ok := OperationalEnvelopeForKind("audit_aggregation_metric", ProfileStream)
	if !ok {
		t.Fatal("kind envelope")
	}
	if len(kind.Cleanup) <= len(base.Cleanup) {
		t.Fatalf("expected cleanup augments: base=%d kind=%d", len(base.Cleanup), len(kind.Cleanup))
	}
	if len(kind.Retention) <= len(base.Retention) {
		t.Fatalf("expected retention augments: base=%d kind=%d", len(base.Retention), len(kind.Retention))
	}
	if len(kind.Cache) <= len(base.Cache) {
		t.Fatalf("expected cache augments: base=%d kind=%d", len(base.Cache), len(kind.Cache))
	}
	found := func(slice []string, want string) bool {
		for _, s := range slice {
			if s == want {
				return true
			}
		}
		return false
	}
	for _, pair := range []struct {
		slice []string
		token string
	}{
		{kind.Cleanup, "stale_window_prune"},
		{kind.Retention, "aggregation_window_boundaries"},
		{kind.Cache, "rollup_segment_index"},
	} {
		if !found(pair.slice, pair.token) {
			t.Fatalf("missing token %q in slice %#v", pair.token, pair.slice)
		}
	}
}

func TestOperationalEnvelopeForKind_baseMetricAndSchedulerHealthStreamAugments(t *testing.T) {
	t.Parallel()
	base, _ := OperationalEnvelopeForProfile(ProfileStream)

	bm, ok := OperationalEnvelopeForKind("base_metric", ProfileStream)
	if !ok {
		t.Fatal("base_metric")
	}
	if len(bm.Health) <= len(base.Health) || len(bm.Scan) <= len(base.Scan) || len(bm.Cache) <= len(base.Cache) {
		t.Fatalf("expected base_metric stream augments: %#v", bm)
	}

	sh, ok := OperationalEnvelopeForKind("scheduler_health_metric", ProfileStream)
	if !ok {
		t.Fatal("scheduler_health_metric")
	}
	if len(sh.Health) <= len(base.Health) || len(sh.Scan) <= len(base.Scan) {
		t.Fatalf("expected scheduler_health_metric stream augments: %#v", sh)
	}
}

func TestOperationalEnvelopeForKind_schedulerJobAugments(t *testing.T) {
	t.Parallel()
	stream, ok := OperationalEnvelopeForKind("scheduler_job", ProfileStream)
	if !ok {
		t.Fatal("stream kind envelope")
	}
	foundScan := false
	for _, s := range stream.Scan {
		if s == "scheduler_job_execution_events" {
			foundScan = true
			break
		}
	}
	if !foundScan {
		t.Fatalf("expected scheduler_job stream scan augment: %#v", stream.Scan)
	}
	cas, ok := OperationalEnvelopeForKind("scheduler_job", ProfileCASEntity)
	if !ok {
		t.Fatal("cas kind envelope")
	}
	foundCAS := false
	for _, s := range cas.Scan {
		if s == "scheduler_job_persisted_state" {
			foundCAS = true
			break
		}
	}
	if !foundCAS {
		t.Fatalf("expected scheduler_job cas scan augment: %#v", cas.Scan)
	}
}

func TestAllOperationalEnvelopeCompactSummaries_coversKnownProfiles(t *testing.T) {
	t.Parallel()
	m := AllOperationalEnvelopeCompactSummaries()
	if len(m) != len(KnownStorageProfiles) {
		t.Fatalf("want one summary per known profile, got %d keys: %+v", len(m), m)
	}
	for _, p := range KnownStorageProfiles {
		s, ok := m[p]
		if !ok || s == "" {
			t.Fatalf("missing or empty summary for %q: %q", p, s)
		}
		if !strings.Contains(s, "scheduler=") {
			t.Fatalf("expected scheduler category in summary for %q: %q", p, s)
		}
	}
}

func TestEnvelopeTickKindAugmentSummaries_nonEmpty(t *testing.T) {
	t.Parallel()
	s := EnvelopeTickKindAugmentSummaries()
	if len(s) == 0 {
		t.Fatal("expected at least one kind@profile summary")
	}
	for _, line := range s {
		if !strings.Contains(line, "@") || !strings.Contains(line, "=") {
			t.Fatalf("malformed summary line: %q", line)
		}
	}
}

func TestOperationalEnvelopeForKind_additionalStreamKindsAugmented(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind   string
		needle string
		pick   func(OperationalEnvelopeSummary) []string
	}{
		{"code_quality_metric", "lint_gate_rollups", func(e OperationalEnvelopeSummary) []string { return e.Scan }},
		{"command_metric", "cli_subcommand_latency_sampling", func(e OperationalEnvelopeSummary) []string { return e.Health }},
		{"file_lock_metric", "stale_lock_reaper_probe", func(e OperationalEnvelopeSummary) []string { return e.Cleanup }},
		{"kind_mapping_metric", "dynamic_kind_mapping_drift", func(e OperationalEnvelopeSummary) []string { return e.Scan }},
		{"mcp_session", "mcp_transport_liveness", func(e OperationalEnvelopeSummary) []string { return e.Health }},
		{"test_audit_aggregation_metric", "fixture_window_alignment", func(e OperationalEnvelopeSummary) []string { return e.Scan }},
		{"verification_matrix", "matrix_row_coverage_gates", func(e OperationalEnvelopeSummary) []string { return e.Scan }},
		{"zqk_session", "tray_runtime_manifest_reads", func(e OperationalEnvelopeSummary) []string { return e.Cache }},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			t.Parallel()
			env, ok := OperationalEnvelopeForKind(tt.kind, ProfileStream)
			if !ok {
				t.Fatal("envelope")
			}
			slice := tt.pick(env)
			for _, s := range slice {
				if s == tt.needle {
					return
				}
			}
			t.Fatalf("missing token %q in picked slice: %#v", tt.needle, slice)
		})
	}
}

func TestOperationalEnvelopeHasKindAugment(t *testing.T) {
	t.Parallel()
	if !OperationalEnvelopeHasKindAugment("audit_event", ProfileStream) {
		t.Fatal("expected augment for audit_event stream")
	}
	if OperationalEnvelopeHasKindAugment("backlog_item", ProfileCASEntity) {
		t.Fatal("unexpected augment for backlog_item cas")
	}
	if OperationalEnvelopeHasKindAugment("audit_event", ProfileCASEntity) {
		t.Fatal("audit_event augment is stream-only in v1 map")
	}
}

func TestOperationalEnvelopeForKind_archiveColdPathAugments(t *testing.T) {
	t.Parallel()
	env, ok := OperationalEnvelopeForKind("audit_event", ProfileStream)
	if !ok {
		t.Fatal("envelope")
	}
	found := false
	for _, a := range env.Archive {
		if a == "audit_stream_cold_export_lineage" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected archive cold token: %#v", env.Archive)
	}
}

func TestOperationalEnvelopeForKind_auditEventOverrideRemovesStreamDefaultArchive(t *testing.T) {
	t.Parallel()
	env, ok := OperationalEnvelopeForKind("audit_event", ProfileStream)
	if !ok {
		t.Fatal("envelope")
	}
	for _, a := range env.Archive {
		if a == operationalEnvelopeStreamArchiveSegmentColdOptional {
			t.Fatalf("stream default cold token should be removed for audit_event: %#v", env.Archive)
		}
	}
}

func TestOperationalEnvelopeForKind_changeJournalOverrideRemovesStreamDefaultScan(t *testing.T) {
	t.Parallel()
	env, ok := OperationalEnvelopeForKind("change_journal_entry", ProfileStream)
	if !ok {
		t.Fatal("envelope")
	}
	for _, s := range env.Scan {
		if s == "high_volume_kinds_alignment" {
			t.Fatalf("stream default alignment scan should be removed for change_journal_entry: %#v", env.Scan)
		}
	}
	foundRegistry := false
	foundCompaction := false
	for _, s := range env.Scan {
		switch s {
		case "stream_registry_checks":
			foundRegistry = true
		case "compaction_micro_gc_cjournal":
			foundCompaction = true
		}
	}
	if !foundRegistry || !foundCompaction {
		t.Fatalf("want stream_registry_checks + compaction augment; scan=%#v", env.Scan)
	}
}

func TestOperationalEnvelopeHasKindOverride(t *testing.T) {
	t.Parallel()
	if !OperationalEnvelopeHasKindOverride("audit_event", ProfileStream) {
		t.Fatal("expected override for audit_event+stream")
	}
	if !OperationalEnvelopeHasKindOverride("change_journal_entry", ProfileStream) {
		t.Fatal("expected override for change_journal_entry+stream")
	}
	if OperationalEnvelopeHasKindOverride("audit_event", ProfileCASEntity) {
		t.Fatal("unexpected override for audit_event+cas in v1 map")
	}
	if OperationalEnvelopeHasKindOverride("backlog_item", ProfileStream) {
		t.Fatal("unexpected override for backlog_item+stream")
	}
}

func TestEnvelopeTickKindOverrideSummaries_nonEmpty(t *testing.T) {
	t.Parallel()
	s := EnvelopeTickKindOverrideSummaries()
	if len(s) == 0 {
		t.Fatal("expected at least one kind@profile override summary")
	}
	var sawAudit, sawCJE bool
	for _, line := range s {
		if !strings.Contains(line, "@") || !strings.Contains(line, "=") {
			t.Fatalf("malformed summary line: %q", line)
		}
		if strings.HasPrefix(line, "audit_event@stream=") {
			sawAudit = true
		}
		if strings.HasPrefix(line, "change_journal_entry@stream=") {
			sawCJE = true
		}
	}
	if !sawAudit || !sawCJE {
		t.Fatalf("expected audit_event + change_journal_entry stream override lines, got %#v", s)
	}
}

func TestApplyOperationalEnvelopeKindOverride_replaceAndRemove(t *testing.T) {
	t.Parallel()
	t.Run("replace clears category", func(t *testing.T) {
		t.Parallel()
		z := []string{"z"}
		base := OperationalEnvelopeSummary{Health: []string{"a", "b", "c"}}
		got := applyOperationalEnvelopeKindOverride(base, operationalEnvelopeKindOverride{ReplaceHealth: &z})
		if len(got.Health) != 1 || got.Health[0] != "z" {
			t.Fatalf("health: %#v", got.Health)
		}
	})
	t.Run("remove then augment path via OperationalEnvelopeForKind ordering", func(t *testing.T) {
		t.Parallel()
		base := OperationalEnvelopeSummary{Cleanup: []string{"x", "y"}}
		got := applyOperationalEnvelopeKindOverride(base, operationalEnvelopeKindOverride{RemoveCleanup: []string{"x"}})
		if len(got.Cleanup) != 1 || got.Cleanup[0] != "y" {
			t.Fatalf("cleanup: %#v", got.Cleanup)
		}
	})
}
