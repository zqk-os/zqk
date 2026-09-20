package dna

import (
	"testing"
)

func TestProvenance_AttestWithURN(t *testing.T) {
	var prov Provenance

	agentURN, err := NewURN("kernel", "account", "ACC-777")
	if err != nil {
		t.Fatalf("failed to create agent URN: %v", err)
	}

	payload := []byte("mutation content")
	prov.Attest(agentURN, payload)

	if prov.AgentURN.String() != agentURN.String() {
		t.Errorf("expected AgentURN %q, got %q", agentURN.String(), prov.AgentURN.String())
	}
	if prov.AgentID != "ACC-777" {
		t.Errorf("expected AgentID 'ACC-777', got %q", prov.AgentID)
	}
	if prov.Actor() != agentURN.String() {
		t.Errorf("expected Actor() to return canonical URN %q, got %q", agentURN.String(), prov.Actor())
	}
	if prov.Hash == "" {
		t.Errorf("expected non-empty hash")
	}
	if prov.Signature == "" {
		t.Errorf("expected non-empty signature")
	}
}

func TestProvenance_AttestWithURNString(t *testing.T) {
	var prov Provenance

	urnStr := "urn:zqk:kernel:account:ACC-888"
	payload := []byte("mutation content")
	prov.Attest(urnStr, payload)

	if prov.AgentURN.String() != urnStr {
		t.Errorf("expected AgentURN %q, got %q", urnStr, prov.AgentURN.String())
	}
	if prov.AgentID != "ACC-888" {
		t.Errorf("expected AgentID 'ACC-888', got %q", prov.AgentID)
	}
	if prov.Actor() != urnStr {
		t.Errorf("expected Actor() %q, got %q", urnStr, prov.Actor())
	}
}

func TestProvenance_AttestWithLegacyRawString(t *testing.T) {
	var prov Provenance

	rawID := "legacy-agent-id"
	payload := []byte("mutation content")
	prov.Attest(rawID, payload)

	if !prov.AgentURN.IsZero() {
		t.Errorf("expected zero AgentURN for raw ID, got %q", prov.AgentURN.String())
	}
	if prov.AgentID != rawID {
		t.Errorf("expected AgentID %q, got %q", rawID, prov.AgentID)
	}
	if prov.Actor() != rawID {
		t.Errorf("expected Actor() %q, got %q", rawID, prov.Actor())
	}
}
