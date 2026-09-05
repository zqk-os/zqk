package agentprompt

import (
	"context"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

type listSpySP struct {
	*testMockSP
	listKinds []string
}

func (m *listSpySP) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	m.listKinds = append(m.listKinds, filter.Kind)
	return m.testMockSP.List(ctx, secCtx, storageCtx, filter)
}

func TestLoadBoundPolicies_readsIDsDoesNotListCatalog(t *testing.T) {
	inner := &testMockSP{
		objects: map[string]map[string]any{
			"POL-ONBOARD-001": {
				objects.FieldKeyKind:  objects.KindPolicy,
				objects.FieldKeyID:    "POL-ONBOARD-001",
				objects.FieldKeyTitle: "Process data CLI only",
			},
			"PER-123": {
				objects.FieldKeyKind:                  objects.KindPersona,
				objects.FieldKeyID:                    "PER-123",
				objects.FieldKeyInteractionPolicyRefs: []any{"POL-AGENT-001"},
			},
			"POL-AGENT-001": {
				objects.FieldKeyKind:  objects.KindPolicy,
				objects.FieldKeyID:    "POL-AGENT-001",
				objects.FieldKeyTitle: "Agent policy",
			},
		},
	}
	sp := &listSpySP{testMockSP: inner}
	got, err := LoadBoundPolicies(context.Background(), sp, pkgctx.NewSystemSecurityContext(), "PER-123")
	if err != nil {
		t.Fatal(err)
	}
	if len(sp.listKinds) != 0 {
		t.Fatalf("LoadBoundPolicies must not list the policy catalog, listed %v", sp.listKinds)
	}
	if !containsID(objectIDs(got.ActivePolicies), "POL-ONBOARD-001") {
		t.Fatalf("expected standing policy, got %#v", objectIDs(got.ActivePolicies))
	}
	if !containsID(objectIDs(got.ActivePolicies), "POL-AGENT-001") {
		t.Fatalf("expected persona policy, got %#v", objectIDs(got.ActivePolicies))
	}
}

func TestGeneratePromptSection_policyRefsNotBodies(t *testing.T) {
	p := &PolicyEnforcement{ActivePolicies: []map[string]any{
		{
			objects.FieldKeyID:          "POL-ONBOARD-001",
			objects.FieldKeyTitle:       "Process data CLI only",
			objects.FieldKeyDescription: "FULL BODY that must not be the default section.",
		},
	}}
	out := p.GeneratePromptSection()
	if !strings.Contains(out, "POL-ONBOARD-001") {
		t.Fatalf("missing id:\n%s", out)
	}
	if strings.Contains(out, "FULL BODY") {
		t.Fatalf("default section inlined body:\n%s", out)
	}
	bodies := p.GeneratePromptSectionBodies()
	if !strings.Contains(bodies, "FULL BODY") {
		t.Fatal("bodies helper should still inline for escape-hatch callers")
	}
}
