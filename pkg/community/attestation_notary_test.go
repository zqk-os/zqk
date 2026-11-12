package community

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAttestationNotary_StaticFloorInvariant(t *testing.T) {
	claim := &AttestationClaim{
		ProviderID: "zqk.provider.notary-test",
		Epoch:      1,
		Action:     "task.execute",
		PayloadSum: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	}

	if err := claim.Validate(); err != nil {
		t.Fatalf("expected valid claim, got error: %v", err)
	}

	secret := []byte("test-notary-signing-secret-key-12345")
	sig := claim.ComputeDigest(secret)
	if len(sig) == 0 {
		t.Fatalf("expected non-empty cryptographic digest")
	}
}

func TestAttestationNotary_OperationalDynamicProof(t *testing.T) {
	ctx := context.Background()
	secret := []byte("master-audit-proof-signing-secret")
	notary := NewAttestationNotary(secret)

	providerID := "zqk.provider.worker-1"
	action := "inference.generate"
	payload := []byte("sample-attestation-execution-payload-content")

	// Step 1: Create and sign attestation
	claim1, err := notary.Attest(ctx, providerID, action, 1, payload)
	if err != nil {
		t.Fatalf("expected attestation to succeed, got: %v", err)
	}
	if claim1.Signature == "" {
		t.Fatalf("expected signature to be populated")
	}
	if notary.GetLedgerSize() != 1 {
		t.Errorf("expected ledger size 1, got %d", notary.GetLedgerSize())
	}

	// Step 2: Verify attestation validity
	if err := notary.Verify(claim1, payload); err != nil {
		t.Fatalf("expected verification to pass, got: %v", err)
	}

	// Step 3: Monotonic epoch progression
	claim2, err := notary.Attest(ctx, providerID, action, 2, payload)
	if err != nil {
		t.Fatalf("expected epoch 2 attestation to succeed, got: %v", err)
	}
	if claim2.Epoch != 2 {
		t.Errorf("expected epoch 2, got %d", claim2.Epoch)
	}
	if notary.GetLedgerSize() != 2 {
		t.Errorf("expected ledger size 2, got %d", notary.GetLedgerSize())
	}
}

func TestAttestationNotary_NegativeBoundaryAndErrorRecovery(t *testing.T) {
	ctx := context.Background()
	secret := []byte("boundary-test-secret-key")
	notary := NewAttestationNotary(secret)

	providerID := "zqk.provider.bad-actor"
	payload := []byte("valid-payload-content")

	// Case 1: Empty Provider ID
	if _, err := notary.Attest(ctx, "", "action", 1, payload); !errors.Is(err, ErrAttestationInvalid) {
		t.Errorf("expected ErrAttestationInvalid for empty provider id, got %v", err)
	}

	// Case 2: Zero or Negative Epoch
	if _, err := notary.Attest(ctx, providerID, "action", 0, payload); !errors.Is(err, ErrAttestationInvalid) {
		t.Errorf("expected ErrAttestationInvalid for non-positive epoch, got %v", err)
	}

	// Case 3: Empty Action
	if _, err := notary.Attest(ctx, providerID, "", 1, payload); !errors.Is(err, ErrAttestationInvalid) {
		t.Errorf("expected ErrAttestationInvalid for empty action, got %v", err)
	}

	// Seed first valid attestation at epoch 10
	claim, err := notary.Attest(ctx, providerID, "action", 10, payload)
	if err != nil {
		t.Fatalf("expected initial attestation to pass, got: %v", err)
	}

	// Case 4: Replay attack (stale or equal epoch)
	if _, err := notary.Attest(ctx, providerID, "action", 10, payload); !errors.Is(err, ErrReplayDetected) {
		t.Errorf("expected ErrReplayDetected for replay of epoch 10, got %v", err)
	}
	if _, err := notary.Attest(ctx, providerID, "action", 5, payload); !errors.Is(err, ErrReplayDetected) {
		t.Errorf("expected ErrReplayDetected for backwards epoch 5, got %v", err)
	}

	// Case 5: Tampered payload content
	tamperedPayload := []byte("tampered-corrupted-content")
	if err := notary.Verify(claim, tamperedPayload); !errors.Is(err, ErrAttestationTampered) {
		t.Errorf("expected ErrAttestationTampered for tampered payload, got %v", err)
	}

	// Case 6: Forged signature
	tamperedClaim := *claim
	tamperedClaim.Signature = "0000000000000000000000000000000000000000000000000000000000000000"
	if err := notary.Verify(&tamperedClaim, payload); !errors.Is(err, ErrAttestationTampered) {
		t.Errorf("expected ErrAttestationTampered for forged signature, got %v", err)
	}
}

func TestAttestationNotary_DocumentationVerification(t *testing.T) {
	adrPath := filepath.Join("..", "..", "docs", "architecture", "ADR_AUTONOMOUS_PROVIDER_CRYPTOGRAPHIC_ATTESTATION_AND_AUDIT_NOTARY_V036.md")
	data, err := fileutil.ReadFile(adrPath)
	if err != nil {
		t.Fatalf("expected ADR-0055 to exist at %s, got: %v", adrPath, err)
	}
	content := string(data)
	if len(content) == 0 {
		t.Fatalf("ADR-0055 content cannot be empty")
	}
	if len(content) > 8000 {
		t.Errorf("ADR-0055 exceeds POL-DOC-001 character limit (8000), size: %d", len(content))
	}
}
