package policy

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/infrastructure/crypto"
)

func TestTrustDomainEnvelope_SignAndVerify(t *testing.T) {
	signer, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("failed to generate keypair: %v", err)
	}

	tde := &TrustDomainEnvelope{
		ID:                "TDE-123",
		Scope:             "mutate",
		AllowedNamespaces: []string{"zqk:kernel"},
		MaxRuntimeSeconds: 300,
		MCPPermissions:    []string{"test:permission"},
		Neurological: &NeurologicalGovernance{
			Confidence:     0.95,
			DecisionBranch: "test_branch",
		},
		Payload: map[string]string{"action": "migrate"},
	}

	err = tde.Verify()
	if err == nil {
		t.Fatalf("expected verification to fail without signature")
	}

	err = tde.Sign(signer)
	if err != nil {
		t.Fatalf("failed to sign TDE: %v", err)
	}

	err = tde.Verify()
	if err != nil {
		t.Fatalf("failed to verify signed TDE: %v", err)
	}

	// Tamper with payload
	tde.Scope = "admin"
	err = tde.Verify()
	if err == nil {
		t.Fatalf("expected verification to fail after tampering")
	}
}

func TestTrustDomainEnvelope_NeurologicalGovernance(t *testing.T) {
	signer, _ := crypto.GenerateKeypair()

	tde := &TrustDomainEnvelope{
		ID:                "TDE-123",
		Scope:             "mutate",
		AllowedNamespaces: []string{"zqk:kernel"},
		MaxRuntimeSeconds: 300,
		MCPPermissions:    []string{"test:permission"},
		Payload:           map[string]string{"action": "migrate"},
	}

	// Missing Neurological Governance
	_ = tde.Sign(signer)
	if err := tde.Verify(); err == nil {
		t.Fatalf("expected failure when Neurological governance is missing")
	}

	// Low Confidence
	tde.Neurological = &NeurologicalGovernance{
		Confidence:     0.5,
		DecisionBranch: "test_branch",
	}
	_ = tde.Sign(signer)
	if err := tde.Verify(); err == nil {
		t.Fatalf("expected failure due to low confidence")
	}

	// Missing DecisionBranch
	tde.Neurological = &NeurologicalGovernance{
		Confidence:     0.95,
		DecisionBranch: "",
	}
	_ = tde.Sign(signer)
	if err := tde.Verify(); err == nil {
		t.Fatalf("expected failure due to missing DecisionBranch")
	}

	// Valid
	tde.Neurological = &NeurologicalGovernance{
		Confidence:     0.95,
		DecisionBranch: "test_branch",
	}
	_ = tde.Sign(signer)
	if err := tde.Verify(); err != nil {
		t.Fatalf("expected success with valid Neurological governance, got: %v", err)
	}
}
