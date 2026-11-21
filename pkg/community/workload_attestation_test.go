// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package community

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func newTestAttestor(t *testing.T, maxFailures int, quarantineCritical bool) *WorkloadAttestor {
	t.Helper()
	cfg := DefaultAttestationConfig()
	cfg.MaxFailuresBeforeQuarantine = maxFailures
	cfg.QuarantineOnCriticalFailure = quarantineCritical
	attestor, err := NewWorkloadAttestor(cfg)
	if err != nil {
		t.Fatalf("NewWorkloadAttestor failed: %v", err)
	}
	return attestor
}

func registerEd25519Host(t *testing.T, attestor *WorkloadAttestor, hostID string) ed25519.PrivateKey {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey failed: %v", err)
	}
	if err := attestor.RegisterHost(HostIdentity{
		HostID:    hostID,
		Algorithm: KeyAlgorithmEd25519,
		PublicKey: pub,
	}); err != nil {
		t.Fatalf("RegisterHost(%s) failed: %v", hostID, err)
	}
	return priv
}

func registerECDSAHost(t *testing.T, attestor *WorkloadAttestor, hostID string) *ecdsa.PrivateKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey failed: %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("x509.MarshalPKIXPublicKey failed: %v", err)
	}
	if err := attestor.RegisterHost(HostIdentity{
		HostID:    hostID,
		Algorithm: KeyAlgorithmECDSAP256,
		PublicKey: pubDER,
	}); err != nil {
		t.Fatalf("RegisterHost(%s) failed: %v", hostID, err)
	}
	return priv
}

func registerHMACHost(t *testing.T, attestor *WorkloadAttestor, hostID string) []byte {
	t.Helper()
	secret := []byte("super-secret-hmac-key-material-32")
	if err := attestor.RegisterHost(HostIdentity{
		HostID:       hostID,
		Algorithm:    KeyAlgorithmHMACSHA256,
		SharedSecret: secret,
	}); err != nil {
		t.Fatalf("RegisterHost(%s) failed: %v", hostID, err)
	}
	return secret
}

func issueAndVerifyWorkload(t *testing.T, attestor *WorkloadAttestor, hostID, taskID string, payload WorkloadPayload, signer any) (*AttestationChallenge, *AttestationReceipt) {
	t.Helper()
	ch, err := attestor.IssueChallenge(hostID, taskID)
	if err != nil {
		t.Fatalf("IssueChallenge(%s, %s) failed: %v", hostID, taskID, err)
	}
	receipt, err := attestor.AttestWorkload(ch, payload, signer)
	if err != nil {
		t.Fatalf("AttestWorkload(%s, %s) failed: %v", hostID, taskID, err)
	}
	if err := attestor.VerifyReceipt(receipt, payload); err != nil {
		t.Fatalf("VerifyReceipt(%s, %s) failed: %v", hostID, taskID, err)
	}
	return ch, receipt
}

func assertErrorIs(t *testing.T, got, want error, label string) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("%s: expected error %v, got %v", label, want, got)
	}
}

func TestWorkloadAttestation_StaticFloorInvariant(t *testing.T) {
	attestor := newTestAttestor(t, 3, true)

	badIdentities := []struct {
		name string
		id   HostIdentity
	}{
		{"blank_host_id", HostIdentity{HostID: "   ", Algorithm: KeyAlgorithmHMACSHA256, SharedSecret: []byte("0123456789abcdef")}},
		{"short_hmac_secret", HostIdentity{HostID: "host-a", Algorithm: KeyAlgorithmHMACSHA256, SharedSecret: []byte("short")}},
		{"bad_ed25519_key", HostIdentity{HostID: "host-b", Algorithm: KeyAlgorithmEd25519, PublicKey: []byte("too-short")}},
		{"bad_ecdsa_key", HostIdentity{HostID: "host-c", Algorithm: KeyAlgorithmECDSAP256, PublicKey: []byte("invalid-der")}},
		{"unknown_algorithm", HostIdentity{HostID: "host-d", Algorithm: HostKeyAlgorithm("rsa-1024")}},
	}
	for _, tc := range badIdentities {
		err := attestor.RegisterHost(tc.id)
		assertErrorIs(t, err, ErrInvalidHostIdentity, tc.name)
	}

	_, err := attestor.IssueChallenge("  ", "task-1")
	assertErrorIs(t, err, ErrInvalidHostIdentity, "blank_challenge_host")

	_, err = attestor.IssueChallenge("unregistered-host", "task-1")
	assertErrorIs(t, err, ErrHostNotRegistered, "unregistered_host")
}

func TestWorkloadAttestation_OperationalDynamicProof(t *testing.T) {
	attestor := newTestAttestor(t, 3, true)
	edPriv := registerEd25519Host(t, attestor, "host-ed25519")
	ecPriv := registerECDSAHost(t, attestor, "host-ecdsa")
	registerHMACHost(t, attestor, "host-hmac")

	payload := WorkloadPayload{
		TaskID:        "task-build-1",
		ArtifactBytes: []byte("compiled-binary-payload-v1"),
		Metadata:      map[string]string{"arch": "arm64", "env": "prod"},
	}

	ch1, _ := issueAndVerifyWorkload(t, attestor, "host-ed25519", "task-build-1", payload, edPriv)
	ch2, _ := issueAndVerifyWorkload(t, attestor, "host-ed25519", "task-build-1", payload, edPriv)
	if ch2.Sequence != ch1.Sequence+1 {
		t.Fatalf("expected monotonic sequence %d, got %d", ch1.Sequence+1, ch2.Sequence)
	}
	if ch2.PrevChainHash == ch1.PrevChainHash {
		t.Fatal("expected nonce chain hash to advance across challenges")
	}

	issueAndVerifyWorkload(t, attestor, "host-ecdsa", "task-build-1", payload, ecPriv)
	issueAndVerifyWorkload(t, attestor, "host-hmac", "task-build-1", payload, nil)

	const workers = 4
	wg := goroutinelabels.StartTestGoroutines(t, workers, "attest_worker", "concurrent workload attestation", func(id int) {
		hostID := fmt.Sprintf("concurrent-host-%d", id)
		priv := registerEd25519Host(t, attestor, hostID)
		p := WorkloadPayload{TaskID: fmt.Sprintf("task-%d", id), ArtifactBytes: []byte("concurrent-artifact")}
		issueAndVerifyWorkload(t, attestor, hostID, p.TaskID, p, priv)
	})
	wg.Wait()
}

func TestWorkloadAttestation_NegativeAdversarialBoundary(t *testing.T) {
	attestor := newTestAttestor(t, 2, true)
	edPriv := registerEd25519Host(t, attestor, "host-replay")
	registerHMACHost(t, attestor, "host-tamper")
	forgerPriv := registerEd25519Host(t, attestor, "host-forger")

	payload := WorkloadPayload{TaskID: "task-sec", ArtifactBytes: []byte("authentic-bytes")}

	// 1. Replay attack detection & repeated-failure quarantine
	_, receipt := issueAndVerifyWorkload(t, attestor, "host-replay", "task-sec", payload, edPriv)
	assertErrorIs(t, attestor.VerifyReceipt(receipt, payload), ErrStaleOrReplayedNonce, "replay_1")
	assertErrorIs(t, attestor.VerifyReceipt(receipt, payload), ErrStaleOrReplayedNonce, "replay_2")
	if !attestor.IsQuarantined("host-replay") {
		t.Fatal("expected host-replay to be quarantined after repeated replay failures")
	}
	_, err := attestor.IssueChallenge("host-replay", "task-sec")
	assertErrorIs(t, err, ErrHostQuarantined, "quarantined_issue")

	// 2. Tampered workload payload digest detection
	chTamper, err := attestor.IssueChallenge("host-tamper", "task-sec")
	if err != nil {
		t.Fatalf("IssueChallenge failed: %v", err)
	}
	tamperReceipt, err := attestor.AttestWorkload(chTamper, payload, nil)
	if err != nil {
		t.Fatalf("AttestWorkload failed: %v", err)
	}
	tamperedPayload := WorkloadPayload{TaskID: "task-sec", ArtifactBytes: []byte("malicious-bytes")}
	assertErrorIs(t, attestor.VerifyReceipt(tamperReceipt, tamperedPayload), ErrTamperedWorkloadDigest, "tampered_digest")
	if !attestor.IsQuarantined("host-tamper") {
		t.Fatal("expected host-tamper to be quarantined on critical digest tampering")
	}

	// 3. Invalid/forged signature detection & immediate quarantine
	chForge, err := attestor.IssueChallenge("host-forger", "task-sec")
	if err != nil {
		t.Fatalf("IssueChallenge failed: %v", err)
	}
	forgedReceipt, err := attestor.AttestWorkload(chForge, payload, forgerPriv)
	if err != nil {
		t.Fatalf("AttestWorkload failed: %v", err)
	}
	forgedReceipt.Signature[0] ^= 0xff
	assertErrorIs(t, attestor.VerifyReceipt(forgedReceipt, payload), ErrInvalidAttestationSignature, "forged_sig")
	if !attestor.IsQuarantined("host-forger") {
		t.Fatal("expected host-forger to be quarantined on signature forgery")
	}

	// 4. Expired challenge nonce rejection
	now := time.Now().UTC()
	attestor.now = func() time.Time { return now }
	expPriv := registerEd25519Host(t, attestor, "host-expired")
	expPayload := WorkloadPayload{TaskID: "task-exp", ArtifactBytes: []byte("authentic-bytes")}
	chExp, _ := issueAndVerifyWorkload(t, attestor, "host-expired", "task-exp", expPayload, expPriv)
	if chExp.ExpiresAt.Before(now) {
		t.Fatal("expected challenge expiry in the future")
	}
	chFresh, err := attestor.IssueChallenge("host-expired", "task-exp")
	if err != nil {
		t.Fatalf("IssueChallenge failed: %v", err)
	}
	freshReceipt, err := attestor.AttestWorkload(chFresh, expPayload, expPriv)
	if err != nil {
		t.Fatalf("AttestWorkload failed: %v", err)
	}
	attestor.now = func() time.Time { return now.Add(10 * time.Minute) }
	assertErrorIs(t, attestor.VerifyReceipt(freshReceipt, expPayload), ErrStaleOrReplayedNonce, "expired_nonce")
}
