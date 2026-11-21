package community

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

const testClaimType = "ast_digest_membership"

func newTestExchangeFixture(t *testing.T, fixedNow time.Time) (*ZKProofExchange, TenantDomainBinding, []byte, []byte) {
	t.Helper()
	fedKey := []byte("federation-shared-secret-key-32b")
	cfg := DefaultExchangeConfig()
	ex, err := NewZKProofExchange(fedKey, cfg)
	if err != nil {
		t.Fatalf("NewZKProofExchange failed: %v", err)
	}
	ex.nowFn = func() time.Time { return fixedNow }
	domain := TenantDomainBinding{
		SourceTenant:     "tenant-alpha",
		TargetTenant:     "tenant-beta",
		FederationDomain: "zqk.fed.prod",
		Epoch:            42,
	}
	artifact := []byte("ast:pkg/community/zk_proof_exchange:digest-input")
	witness := []byte("top-secret-private-tenant-witness-payload-991827")
	return ex, domain, artifact, witness
}

func assertZKErrorIs(t *testing.T, label string, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("%s: expected error %v, got %v", label, want, got)
	}
}

func mustNewBlinding(t *testing.T) *BlindingFactor {
	t.Helper()
	b, err := NewBlindingFactor()
	if err != nil {
		t.Fatalf("NewBlindingFactor failed: %v", err)
	}
	return b
}

func TestZKProofExchange_StaticFloorInvariant(t *testing.T) {
	fixedNow := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	ex, domain, artifact, witness := newTestExchangeFixture(t, fixedNow)

	blinding := mustNewBlinding(t)
	if err := blinding.Validate(); err != nil {
		t.Fatalf("blinding.Validate failed: %v", err)
	}

	proof := mustGenerateProof(t, ex, "stmt-static-01", testClaimType, artifact, witness, blinding, domain)
	if err := proof.Validate(); err != nil {
		t.Fatalf("proof.Validate failed: %v", err)
	}

	encoded, err := json.Marshal(proof)
	if err != nil {
		t.Fatalf("json.Marshal(proof) failed: %v", err)
	}
	serialized := string(encoded)
	if strings.Contains(serialized, string(witness)) {
		t.Fatalf("zero-disclosure violated: raw witness leaked in serialized proof")
	}
	if bytes.Contains(encoded, blinding.Salt) || strings.Contains(serialized, hex.EncodeToString(blinding.Salt)) {
		t.Fatalf("zero-disclosure violated: blinding salt leaked in serialized proof")
	}

	bfJSON, err := json.Marshal(blinding)
	if err != nil {
		t.Fatalf("json.Marshal(blinding) failed: %v", err)
	}
	if string(bfJSON) != "{}" {
		t.Fatalf("expected blinding JSON to be empty object, got %s", string(bfJSON))
	}
}

func mustDeriveBlinding(t *testing.T, seed []byte, domain string) *BlindingFactor {
	t.Helper()
	b, err := DeriveBlindingFactor(seed, domain)
	if err != nil {
		t.Fatalf("DeriveBlindingFactor failed: %v", err)
	}
	return b
}

func mustGenerateProof(t *testing.T, ex *ZKProofExchange, stmtID, claimType string, artifact, witness []byte, blinding *BlindingFactor, domain TenantDomainBinding) *ZKProof {
	t.Helper()
	p, err := ex.GenerateProof(stmtID, claimType, artifact, witness, blinding, domain)
	if err != nil {
		t.Fatalf("GenerateProof(%s) failed: %v", stmtID, err)
	}
	return p
}

func TestZKProofExchange_OperationalDynamicProof_Deterministic(t *testing.T) {
	fixedNow := time.Date(2026, 11, 20, 12, 30, 0, 0, time.UTC)
	ex, domain, artifact, witness := newTestExchangeFixture(t, fixedNow)

	seed := []byte("deterministic-seed-material-32bt")
	blinding1 := mustDeriveBlinding(t, seed, domain.FederationDomain)
	blinding2 := mustDeriveBlinding(t, seed, domain.FederationDomain)

	proof1 := mustGenerateProof(t, ex, "stmt-det-01", "artifact_integrity", artifact, witness, blinding1, domain)
	proof2 := mustGenerateProof(t, ex, "stmt-det-01", "artifact_integrity", artifact, witness, blinding2, domain)

	if proof1.ProofID != proof2.ProofID || proof1.Challenge != proof2.Challenge || proof1.Response != proof2.Response {
		t.Fatalf("expected deterministic proof generation, got %+v vs %+v", proof1, proof2)
	}
	if err := proof1.Commitment.Open(witness, blinding1); err != nil {
		t.Fatalf("Commitment.Open failed: %v", err)
	}

	expectedDigest := ComputeArtifactDigest(artifact)
	if err := ex.VerifyProof(proof1, domain, expectedDigest); err != nil {
		t.Fatalf("VerifyProof failed: %v", err)
	}
}

func TestZKProofExchange_OperationalDynamicProof_Concurrent(t *testing.T) {
	fixedNow := time.Date(2026, 11, 20, 13, 0, 0, 0, time.UTC)
	ex, baseDomain, artifact, witness := newTestExchangeFixture(t, fixedNow)
	expectedDigest := ComputeArtifactDigest(artifact)

	const workers = 8
	errCh := make(chan error, workers)
	wg := goroutinelabels.StartTestGoroutines(t, workers, "zk_exchange_worker", "concurrent ZK proof generation and verification", func(id int) {
		workerDomain := baseDomain
		workerDomain.Epoch = uint64(100 + id)
		blinding, err := NewBlindingFactor()
		if err != nil {
			errCh <- err
			return
		}
		stmtID := fmt.Sprintf("stmt-concurrent-%d", id)
		proof, err := ex.GenerateProof(stmtID, testClaimType, artifact, witness, blinding, workerDomain)
		if err != nil {
			errCh <- err
			return
		}
		errCh <- ex.VerifyProof(proof, workerDomain, expectedDigest)
	})
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent ZK proof worker failed: %v", err)
		}
	}
}

func newTestProofFixture(t *testing.T, fixedNow time.Time, stmtID string) (*ZKProofExchange, TenantDomainBinding, *BlindingFactor, *ZKProof, string) {
	t.Helper()
	ex, domain, artifact, witness := newTestExchangeFixture(t, fixedNow)
	blinding := mustNewBlinding(t)
	proof := mustGenerateProof(t, ex, stmtID, testClaimType, artifact, witness, blinding, domain)
	return ex, domain, blinding, proof, ComputeArtifactDigest(artifact)
}

func TestZKProofExchange_NegativeAdversarialBoundary_TamperAndMismatch(t *testing.T) {
	fixedNow := time.Date(2026, 11, 20, 14, 0, 0, 0, time.UTC)
	ex, domain, blinding, proof, digest := newTestProofFixture(t, fixedNow, "stmt-neg-01")

	// Tampered response
	tamperedResp := *proof
	tamperedResp.Response = strings.Repeat("a", len(proof.Response))
	assertZKErrorIs(t, "tampered response", ex.VerifyProof(&tamperedResp, domain, digest), ErrTamperedProof)

	// Falsified statement via mismatched expected artifact digest
	wrongDigest := ComputeArtifactDigest([]byte("other-artifact"))
	assertZKErrorIs(t, "falsified artifact digest", ex.VerifyProof(proof, domain, wrongDigest), ErrFalsifiedStatement)

	// Mismatched commitment between statement and commitment object
	mismatched := *proof
	mismatched.Commitment.Digest = strings.Repeat("b", 64)
	assertZKErrorIs(t, "mismatched commitment", ex.VerifyProof(&mismatched, domain, digest), ErrCommitmentMismatch)

	// Commitment.Open with wrong witness
	assertZKErrorIs(t, "wrong witness open", proof.Commitment.Open([]byte("wrong-witness"), blinding), ErrCommitmentMismatch)
}

func TestZKProofExchange_NegativeAdversarialBoundary_ReplayAndExpiry(t *testing.T) {
	fixedNow := time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC)
	ex, domain, _, proof, digest := newTestProofFixture(t, fixedNow, "stmt-neg-02")

	// Cross-domain / cross-tenant replay
	otherTenant := domain
	otherTenant.TargetTenant = "tenant-gamma"
	assertZKErrorIs(t, "cross-tenant replay", ex.VerifyProof(proof, otherTenant, digest), ErrCrossDomainReplay)

	// Valid first verification succeeds, second verification triggers duplicate replay error
	if err := ex.VerifyProof(proof, domain, digest); err != nil {
		t.Fatalf("initial VerifyProof failed: %v", err)
	}
	assertZKErrorIs(t, "duplicate proof replay", ex.VerifyProof(proof, domain, digest), ErrProofReplay)

	// Expired proof rejected
	ex.nowFn = func() time.Time { return fixedNow.Add(10 * time.Minute) }
	assertZKErrorIs(t, "expired proof", ex.VerifyProof(proof, domain, digest), ErrProofExpired)
}
