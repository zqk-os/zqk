package scheduler

import (
	"strings"
	"testing"
)

func TestResolvePhaseRoutingProfile_customForArbitraryFlowVariant(t *testing.T) {
	t.Parallel()
	p := resolvePhaseRoutingProfile("acme_corp_incident_response")
	if p.ID != "custom" {
		t.Fatalf("ID: got %q want custom", p.ID)
	}
	if p.ExtraNotes == nil {
		t.Fatal("expected ExtraNotes")
	}
	notes := strings.Join(p.ExtraNotes(nil), " ")
	if !strings.Contains(notes, "Custom flow_variant") || !strings.Contains(notes, "acme_corp_incident_response") {
		t.Fatalf("notes: %s", notes)
	}
}

func TestResolvePhaseRoutingProfile_codeQualityVariants(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		flowVariant string
		wantID      string
		substr      string
	}{
		{"code_quality_go_matrix", "code_quality_go_matrix", "code_quality_go_matrix"},
		{"code_quality_drift_and_standardization", "code_quality_drift_and_standardization", "drift/hardcoded-literal"},
		{"vetting_matrix_c6", "vetting_matrix_c6", "vetting_matrix_c6"},
		{"expertise_docs_and_alpha_prep", "expertise_docs_and_alpha_prep", "docs/architecture"},
		{"bli_documentation_delivery", "bli_documentation_delivery", "BLI acceptance"},
		{"product_delivery_datacell", "product_delivery_datacell", "product_delivery_datacell"},
	} {
		tc := tc
		t.Run(tc.flowVariant, func(t *testing.T) {
			t.Parallel()
			p := resolvePhaseRoutingProfile(tc.flowVariant)
			if p.ID != tc.wantID {
				t.Fatalf("ID: got %q want %q", p.ID, tc.wantID)
			}
			if p.ExtraNotes == nil {
				t.Fatal("expected ExtraNotes")
			}
			notes := p.ExtraNotes(nil)
			if len(notes) < 1 || !strings.Contains(strings.Join(notes, " "), tc.substr) {
				t.Fatalf("notes: %v (want substring %q)", notes, tc.substr)
			}
		})
	}
}
