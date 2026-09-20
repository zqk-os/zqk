package datacell

import (
	"strings"
	"testing"
)

func TestAllOperationalEnvelopeDiscoveryTokens_nonEmpty(t *testing.T) {
	t.Parallel()
	toks := AllOperationalEnvelopeDiscoveryTokens()
	if len(toks) < 10 {
		t.Fatalf("expected many discovery tokens, got %d", len(toks))
	}
}

func TestResolvedSchedulerJobTypesFromDiscoveryTokens_sortedUnique(t *testing.T) {
	t.Parallel()
	got := ResolvedSchedulerJobTypesFromDiscoveryTokens()
	if len(got) == 0 {
		t.Fatal("expected at least one resolved job type")
	}
	for i := 1; i < len(got); i++ {
		if got[i] <= got[i-1] {
			t.Fatalf("not sorted: %v", got)
		}
	}
	seen := map[string]struct{}{}
	for _, s := range got {
		seen[s] = struct{}{}
	}
	if len(seen) != len(got) {
		t.Fatalf("duplicates in %v", got)
	}
}

func TestEnvelopeDiscoveryTokensMappedOrDocumentationOnly(t *testing.T) {
	t.Parallel()
	toks := AllOperationalEnvelopeDiscoveryTokens()
	var missing []string
	for _, tok := range toks {
		if _, mapped := SchedulerJobTypeForEnvelopeToken(tok); mapped {
			continue
		}
		if _, doc := EnvelopeTokensDocumentationOnly[tok]; doc {
			continue
		}
		missing = append(missing, tok)
	}
	if len(missing) > 0 {
		t.Fatalf("envelope tokens missing map or doc-only entry: %s", strings.Join(missing, ", "))
	}
}

func TestResolvedEnvelopeTokenJobEdges_coversBindings(t *testing.T) {
	t.Parallel()
	edges := ResolvedEnvelopeTokenJobEdges()
	if len(edges) == 0 {
		t.Fatal("edges required")
	}
	got := ResolvedSchedulerJobTypesFromTokenEdgesWithPolicy(edges, nil, nil, false)
	want := ResolvedSchedulerJobTypesFromDiscoveryTokens()
	if len(got) != len(want) {
		t.Fatalf("edge policy none: len %d vs legacy %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("mismatch at %d: %v vs %v", i, got, want)
		}
	}
}

func TestResolvedSchedulerJobTypesFromTokenEdgesWithPolicy_denyToken(t *testing.T) {
	t.Parallel()
	edges := ResolvedEnvelopeTokenJobEdges()
	deny := map[string]struct{}{"json_schema_checks": {}}
	got := ResolvedSchedulerJobTypesFromTokenEdgesWithPolicy(edges, deny, nil, false)
	for _, jt := range got {
		if jt == schedulerJobWireIntegrityCheck {
			t.Fatalf("integrity_check should drop when json_schema_checks denied: %v", got)
		}
	}
}

func TestResolvedSchedulerJobTypesFromTokenEdgesWithPolicy_allowRestrictive(t *testing.T) {
	t.Parallel()
	edges := ResolvedEnvelopeTokenJobEdges()
	allow := map[string]struct{}{"json_schema_checks": {}}
	got := ResolvedSchedulerJobTypesFromTokenEdgesWithPolicy(edges, nil, allow, true)
	if len(got) != 1 || got[0] != schedulerJobWireIntegrityCheck {
		t.Fatalf("want only integrity_check, got %v", got)
	}
}

func TestResolvedSchedulerJobTypesFromTokenEdgesWithPolicy_allowEmptySuppressesAll(t *testing.T) {
	t.Parallel()
	edges := ResolvedEnvelopeTokenJobEdges()
	got := ResolvedSchedulerJobTypesFromTokenEdgesWithPolicy(edges, nil, map[string]struct{}{}, true)
	if len(got) != 0 {
		t.Fatalf("explicit empty allow-list should suppress all edges; got %v", got)
	}
}

func TestSchedulerJobTypeForEnvelopeToken_examples(t *testing.T) {
	t.Parallel()
	if jt, ok := SchedulerJobTypeForEnvelopeToken("segment_rotation"); !ok || jt != schedulerJobWireMaintenance {
		t.Fatalf("got %q ok=%v", jt, ok)
	}
	// Metrics sampling / pipeline observability tokens (CLI alpha PRI — scheduler daemon metrics path).
	if jt, ok := SchedulerJobTypeForEnvelopeToken("sampler_emission_paths"); !ok || jt != schedulerJobWireMetricsCollection {
		t.Fatalf("sampler_emission_paths: got %q ok=%v", jt, ok)
	}
	if jt, ok := SchedulerJobTypeForEnvelopeToken("metric_kind_family_contract"); !ok || jt != schedulerJobWireMetricsCollection {
		t.Fatalf("metric_kind_family_contract: got %q ok=%v", jt, ok)
	}
	if _, ok := SchedulerJobTypeForEnvelopeToken(""); ok {
		t.Fatal("empty token should miss")
	}
}
