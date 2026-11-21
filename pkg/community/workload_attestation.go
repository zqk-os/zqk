// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package community

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

// HostKeyAlgorithm identifies the cryptographic key binding used by an attested host.
type HostKeyAlgorithm string

const (
	// KeyAlgorithmHMACSHA256 binds a host using a symmetric HMAC-SHA256 shared secret.
	KeyAlgorithmHMACSHA256 HostKeyAlgorithm = "hmac-sha256"
	// KeyAlgorithmEd25519 binds a host using an asymmetric Ed25519 public key.
	KeyAlgorithmEd25519 HostKeyAlgorithm = "ed25519"
	// KeyAlgorithmECDSAP256 binds a host using an asymmetric ECDSA P-256 public key.
	KeyAlgorithmECDSAP256 HostKeyAlgorithm = "ecdsa-p256"

	defaultChallengeTTL          = 5 * time.Minute
	defaultMaxFailuresQuarantine = 3
	defaultNonceByteLen          = 16
	defaultTokenSecretLen        = 32
	minHMACKeyBytes              = 16
	attestationDomainTag         = "zqk:workload-attestation:v1:"
)

var (
	// ErrInvalidHostIdentity is returned when host registration parameters are missing or invalid.
	ErrInvalidHostIdentity = errors.New("workload_attestation: invalid host identity")
	// ErrHostNotRegistered is returned when an operation references an unknown host.
	ErrHostNotRegistered = errors.New("workload_attestation: host not registered")
	// ErrHostQuarantined is returned when a host has been quarantined due to attestation violations.
	ErrHostQuarantined = errors.New("workload_attestation: host is quarantined")
	// ErrStaleOrReplayedNonce is returned when a challenge nonce is expired, unknown, out-of-order, or already consumed.
	ErrStaleOrReplayedNonce = errors.New("workload_attestation: stale or replayed challenge nonce")
	// ErrTamperedWorkloadDigest is returned when the workload payload digest does not match the attested receipt.
	ErrTamperedWorkloadDigest = errors.New("workload_attestation: tampered workload payload digest")
	// ErrInvalidAttestationSignature is returned when cryptographic signature verification fails.
	ErrInvalidAttestationSignature = errors.New("workload_attestation: invalid attestation signature")
)

// HostIdentity binds a worker host identifier to its cryptographic verification key.
type HostIdentity struct {
	HostID       string           `json:"host_id"`
	Algorithm    HostKeyAlgorithm `json:"algorithm"`
	PublicKey    []byte           `json:"public_key,omitempty"`
	SharedSecret []byte           `json:"shared_secret,omitempty"`
	RegisteredAt time.Time        `json:"registered_at"`
	Quarantined  bool             `json:"quarantined"`
	FailureCount int              `json:"failure_count"`
}

// AttestationChallenge represents a single-use, nonce-chained cryptographic challenge.
type AttestationChallenge struct {
	ChallengeID   string    `json:"challenge_id"`
	HostID        string    `json:"host_id"`
	TaskID        string    `json:"task_id"`
	Sequence      uint64    `json:"sequence"`
	Nonce         string    `json:"nonce"`
	PrevChainHash string    `json:"prev_chain_hash"`
	TaskToken     string    `json:"task_token"`
	IssuedAt      time.Time `json:"issued_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	Consumed      bool      `json:"consumed"`
}

// WorkloadPayload describes the executed workload artifact and metadata being attested.
type WorkloadPayload struct {
	TaskID        string            `json:"task_id"`
	ArtifactBytes []byte            `json:"artifact_bytes"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// AttestationReceipt is a signed proof binding a host, challenge nonce, and workload digest.
type AttestationReceipt struct {
	ReceiptID     string           `json:"receipt_id"`
	HostID        string           `json:"host_id"`
	TaskID        string           `json:"task_id"`
	ChallengeID   string           `json:"challenge_id"`
	Sequence      uint64           `json:"sequence"`
	Nonce         string           `json:"nonce"`
	TaskToken     string           `json:"task_token"`
	PayloadDigest string           `json:"payload_digest"`
	Signature     []byte           `json:"signature"`
	Algorithm     HostKeyAlgorithm `json:"algorithm"`
	IssuedAt      time.Time        `json:"issued_at"`
}

// AttestationConfig configures challenge lifetimes and quarantine thresholds.
type AttestationConfig struct {
	ChallengeTTL                time.Duration `json:"challenge_ttl"`
	MaxFailuresBeforeQuarantine int           `json:"max_failures_before_quarantine"`
	QuarantineOnCriticalFailure bool          `json:"quarantine_on_critical_failure"`
	TokenSecret                 []byte        `json:"-"`
}

// DefaultAttestationConfig returns production-safe defaults for WorkloadAttestor.
func DefaultAttestationConfig() AttestationConfig {
	return AttestationConfig{
		ChallengeTTL:                defaultChallengeTTL,
		MaxFailuresBeforeQuarantine: defaultMaxFailuresQuarantine,
		QuarantineOnCriticalFailure: true,
	}
}

type hostRecord struct {
	identity     HostIdentity
	ecdsaPub     *ecdsa.PublicKey
	lastSequence uint64
	chainHash    string
}

// WorkloadAttestor manages host key bindings, nonce-chained challenges, receipt verification,
// replay detection, and automatic quarantine of malicious hosts.
type WorkloadAttestor struct {
	mu          sync.RWMutex
	config      AttestationConfig
	tokenSecret []byte
	hosts       map[string]*hostRecord
	challenges  map[string]*AttestationChallenge
	now         func() time.Time
}

// NewWorkloadAttestor constructs a WorkloadAttestor with the supplied configuration.
func NewWorkloadAttestor(cfg AttestationConfig) (*WorkloadAttestor, error) {
	if cfg.ChallengeTTL <= 0 {
		cfg.ChallengeTTL = defaultChallengeTTL
	}
	if cfg.MaxFailuresBeforeQuarantine <= 0 {
		cfg.MaxFailuresBeforeQuarantine = defaultMaxFailuresQuarantine
	}
	secret := append([]byte(nil), cfg.TokenSecret...)
	if len(secret) == 0 {
		var err error
		secret, err = randomBytes(defaultTokenSecretLen)
		if err != nil {
			return nil, fmt.Errorf("workload_attestation: failed to initialize token secret: %w", err)
		}
	}
	return &WorkloadAttestor{
		config:      cfg,
		tokenSecret: secret,
		hosts:       make(map[string]*hostRecord),
		challenges:  make(map[string]*AttestationChallenge),
		now:         time.Now,
	}, nil
}

// RegisterHost binds a host identifier to an HMAC-SHA256, Ed25519, or ECDSA P-256 key.
func (a *WorkloadAttestor) RegisterHost(identity HostIdentity) error {
	if err := strutil.ErrorOnBlank(identity.HostID, "%w: host_id cannot be blank", ErrInvalidHostIdentity); err != nil {
		return err
	}
	rec, err := buildHostRecord(identity, a.now().UTC())
	if err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	a.hosts[identity.HostID] = rec
	return nil
}

// IssueChallenge mints a monotonic, nonce-chained attestation challenge for a registered host.
func (a *WorkloadAttestor) IssueChallenge(hostID, taskID string) (*AttestationChallenge, error) {
	if err := validateNonBlankFields([]blankFieldCheck{
		{value: hostID, label: "host_id"},
		{value: taskID, label: "task_id"},
	}, ErrInvalidHostIdentity); err != nil {
		return nil, err
	}

	nonceHex, err := randomHex(defaultNonceByteLen)
	if err != nil {
		return nil, fmt.Errorf("workload_attestation: failed to generate nonce: %w", err)
	}
	challengeID, err := randomHex(defaultNonceByteLen)
	if err != nil {
		return nil, fmt.Errorf("workload_attestation: failed to generate challenge_id: %w", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	rec, ok := a.hosts[hostID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrHostNotRegistered, hostID)
	}
	if rec.identity.Quarantined {
		return nil, fmt.Errorf("%w: %s", ErrHostQuarantined, hostID)
	}

	rec.lastSequence++
	prevChain := rec.chainHash
	rec.chainHash = hashStringsSHA256(prevChain, nonceHex, fmt.Sprintf("%d", rec.lastSequence))
	now := a.now().UTC()

	ch := &AttestationChallenge{
		ChallengeID:   challengeID,
		HostID:        hostID,
		TaskID:        taskID,
		Sequence:      rec.lastSequence,
		Nonce:         nonceHex,
		PrevChainHash: prevChain,
		IssuedAt:      now,
		ExpiresAt:     now.Add(a.config.ChallengeTTL),
	}
	ch.TaskToken = computeTaskToken(a.tokenSecret, ch)
	a.challenges[challengeID] = ch

	copyCh := *ch
	return &copyCh, nil
}

// AttestWorkload computes the canonical workload digest and produces a signed AttestationReceipt.
// For HMAC-SHA256 hosts, signer may be nil (uses registered shared secret) or []byte.
// For Ed25519/ECDSA hosts, signer must be the host's private key (crypto.Signer).
func (a *WorkloadAttestor) AttestWorkload(challenge *AttestationChallenge, payload WorkloadPayload, signer any) (*AttestationReceipt, error) {
	if challenge == nil {
		return nil, fmt.Errorf("%w: challenge cannot be nil", ErrStaleOrReplayedNonce)
	}
	if err := validateNonBlankFields([]blankFieldCheck{
		{value: challenge.ChallengeID, label: "challenge_id"},
		{value: challenge.HostID, label: "host_id"},
		{value: challenge.Nonce, label: "nonce"},
		{value: payload.TaskID, label: "task_id"},
	}, ErrInvalidHostIdentity); err != nil {
		return nil, err
	}

	a.mu.RLock()
	rec, ok := a.hosts[challenge.HostID]
	if !ok {
		a.mu.RUnlock()
		return nil, fmt.Errorf("%w: %s", ErrHostNotRegistered, challenge.HostID)
	}
	if rec.identity.Quarantined {
		a.mu.RUnlock()
		return nil, fmt.Errorf("%w: %s", ErrHostQuarantined, challenge.HostID)
	}
	alg := rec.identity.Algorithm
	secretCopy := append([]byte(nil), rec.identity.SharedSecret...)
	a.mu.RUnlock()

	receiptID, err := randomHex(defaultNonceByteLen)
	if err != nil {
		return nil, fmt.Errorf("workload_attestation: failed to generate receipt_id: %w", err)
	}

	receipt := &AttestationReceipt{
		ReceiptID:     receiptID,
		HostID:        challenge.HostID,
		TaskID:        payload.TaskID,
		ChallengeID:   challenge.ChallengeID,
		Sequence:      challenge.Sequence,
		Nonce:         challenge.Nonce,
		TaskToken:     challenge.TaskToken,
		PayloadDigest: ComputeWorkloadDigest(payload),
		Algorithm:     alg,
		IssuedAt:      a.now().UTC(),
	}

	if signer == nil && alg == KeyAlgorithmHMACSHA256 {
		signer = secretCopy
	}
	sig, err := signCanonical(alg, canonicalReceiptBytes(receipt), signer)
	if err != nil {
		return nil, err
	}
	receipt.Signature = sig
	return receipt, nil
}

// VerifyReceipt validates challenge binding, replay protection, workload digest, and host signature.
// Failures increment the host's violation counter and quarantine malicious or spoofing hosts.
func (a *WorkloadAttestor) VerifyReceipt(receipt *AttestationReceipt, payload WorkloadPayload) error {
	if receipt == nil {
		return fmt.Errorf("%w: receipt cannot be nil", ErrInvalidAttestationSignature)
	}
	if err := validateNonBlankFields([]blankFieldCheck{
		{value: receipt.ReceiptID, label: "receipt_id"},
		{value: receipt.HostID, label: "host_id"},
		{value: receipt.TaskID, label: "task_id"},
		{value: receipt.ChallengeID, label: "challenge_id"},
		{value: receipt.Nonce, label: "nonce"},
		{value: receipt.TaskToken, label: "task_token"},
		{value: receipt.PayloadDigest, label: "payload_digest"},
	}, ErrInvalidAttestationSignature); err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	rec, ok := a.hosts[receipt.HostID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrHostNotRegistered, receipt.HostID)
	}
	if rec.identity.Quarantined {
		return fmt.Errorf("%w: %s", ErrHostQuarantined, receipt.HostID)
	}

	ch, err := a.validateChallengeLocked(receipt)
	if err != nil {
		a.recordFailureLocked(rec, false)
		return err
	}

	expectedDigest := ComputeWorkloadDigest(payload)
	if subtle.ConstantTimeCompare([]byte(expectedDigest), []byte(receipt.PayloadDigest)) != 1 {
		a.recordFailureLocked(rec, true)
		return fmt.Errorf("%w: expected %s, got %s", ErrTamperedWorkloadDigest, expectedDigest, receipt.PayloadDigest)
	}

	if err := verifyCanonicalSignature(rec, canonicalReceiptBytes(receipt), receipt.Signature); err != nil {
		a.recordFailureLocked(rec, true)
		return err
	}

	ch.Consumed = true
	rec.identity.FailureCount = 0
	return nil
}

// IsQuarantined reports whether the specified host has been quarantined.
func (a *WorkloadAttestor) IsQuarantined(hostID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	rec, ok := a.hosts[hostID]
	return ok && rec.identity.Quarantined
}

// ComputeWorkloadDigest returns the deterministic SHA-256 hex digest of a WorkloadPayload.
func ComputeWorkloadDigest(payload WorkloadPayload) string {
	h := sha256.New()
	h.Write([]byte(attestationDomainTag))
	h.Write([]byte(payload.TaskID))
	h.Write([]byte{0})
	h.Write(payload.ArtifactBytes)
	h.Write([]byte{0})

	if len(payload.Metadata) > 0 {
		keys := make([]string, 0, len(payload.Metadata))
		for k := range payload.Metadata {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			h.Write([]byte(k))
			h.Write([]byte{'='})
			h.Write([]byte(payload.Metadata[k]))
			h.Write([]byte{';'})
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (a *WorkloadAttestor) validateChallengeLocked(receipt *AttestationReceipt) (*AttestationChallenge, error) {
	ch, ok := a.challenges[receipt.ChallengeID]
	if !ok {
		return nil, fmt.Errorf("%w: unknown challenge %s", ErrStaleOrReplayedNonce, receipt.ChallengeID)
	}
	if ch.Consumed {
		return nil, fmt.Errorf("%w: challenge %s already consumed", ErrStaleOrReplayedNonce, receipt.ChallengeID)
	}
	if a.now().UTC().After(ch.ExpiresAt) {
		return nil, fmt.Errorf("%w: challenge %s expired", ErrStaleOrReplayedNonce, receipt.ChallengeID)
	}
	if ch.HostID != receipt.HostID || ch.TaskID != receipt.TaskID ||
		ch.Sequence != receipt.Sequence || ch.Nonce != receipt.Nonce {
		return nil, fmt.Errorf("%w: challenge binding mismatch", ErrStaleOrReplayedNonce)
	}
	expectedToken := computeTaskToken(a.tokenSecret, ch)
	if subtle.ConstantTimeCompare([]byte(expectedToken), []byte(receipt.TaskToken)) != 1 {
		return nil, fmt.Errorf("%w: invalid task token", ErrStaleOrReplayedNonce)
	}
	return ch, nil
}

func (a *WorkloadAttestor) recordFailureLocked(rec *hostRecord, critical bool) {
	rec.identity.FailureCount++
	if (critical && a.config.QuarantineOnCriticalFailure) ||
		rec.identity.FailureCount >= a.config.MaxFailuresBeforeQuarantine {
		rec.identity.Quarantined = true
	}
}

func buildHostRecord(identity HostIdentity, now time.Time) (*hostRecord, error) {
	copied := HostIdentity{
		HostID:       identity.HostID,
		Algorithm:    identity.Algorithm,
		PublicKey:    append([]byte(nil), identity.PublicKey...),
		SharedSecret: append([]byte(nil), identity.SharedSecret...),
		RegisteredAt: now,
	}
	rec := &hostRecord{
		identity:  copied,
		chainHash: hashStringsSHA256(attestationDomainTag, identity.HostID),
	}

	switch identity.Algorithm {
	case KeyAlgorithmHMACSHA256:
		if len(copied.SharedSecret) < minHMACKeyBytes {
			return nil, fmt.Errorf("%w: hmac shared secret must be at least %d bytes", ErrInvalidHostIdentity, minHMACKeyBytes)
		}
	case KeyAlgorithmEd25519:
		if len(copied.PublicKey) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("%w: ed25519 public key must be %d bytes", ErrInvalidHostIdentity, ed25519.PublicKeySize)
		}
	case KeyAlgorithmECDSAP256:
		pubAny, err := x509.ParsePKIXPublicKey(copied.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid ecdsa PKIX public key: %v", ErrInvalidHostIdentity, err)
		}
		ecPub, ok := pubAny.(*ecdsa.PublicKey)
		if !ok || ecPub.Curve != elliptic.P256() {
			return nil, fmt.Errorf("%w: expected ECDSA P-256 public key", ErrInvalidHostIdentity)
		}
		rec.ecdsaPub = ecPub
	default:
		return nil, fmt.Errorf("%w: unsupported algorithm %q", ErrInvalidHostIdentity, identity.Algorithm)
	}

	return rec, nil
}

func signCanonical(alg HostKeyAlgorithm, canonical []byte, signer any) ([]byte, error) {
	switch alg {
	case KeyAlgorithmHMACSHA256:
		secret, ok := signer.([]byte)
		if !ok || len(secret) < minHMACKeyBytes {
			return nil, fmt.Errorf("%w: hmac signer must be []byte of at least %d bytes", ErrInvalidAttestationSignature, minHMACKeyBytes)
		}
		mac := hmac.New(sha256.New, secret)
		mac.Write(canonical)
		return mac.Sum(nil), nil
	case KeyAlgorithmEd25519:
		cs, ok := signer.(crypto.Signer)
		if !ok {
			return nil, fmt.Errorf("%w: ed25519 signer must implement crypto.Signer", ErrInvalidAttestationSignature)
		}
		return cs.Sign(rand.Reader, canonical, crypto.Hash(0))
	case KeyAlgorithmECDSAP256:
		cs, ok := signer.(crypto.Signer)
		if !ok {
			return nil, fmt.Errorf("%w: ecdsa signer must implement crypto.Signer", ErrInvalidAttestationSignature)
		}
		digest := sha256.Sum256(canonical)
		return cs.Sign(rand.Reader, digest[:], crypto.SHA256)
	default:
		return nil, fmt.Errorf("%w: unsupported algorithm %q", ErrInvalidAttestationSignature, alg)
	}
}

func verifyCanonicalSignature(rec *hostRecord, canonical, sig []byte) error {
	if len(sig) == 0 {
		return fmt.Errorf("%w: empty signature", ErrInvalidAttestationSignature)
	}
	switch rec.identity.Algorithm {
	case KeyAlgorithmHMACSHA256:
		mac := hmac.New(sha256.New, rec.identity.SharedSecret)
		mac.Write(canonical)
		if !hmac.Equal(sig, mac.Sum(nil)) {
			return fmt.Errorf("%w: hmac verification failed", ErrInvalidAttestationSignature)
		}
		return nil
	case KeyAlgorithmEd25519:
		if !ed25519.Verify(ed25519.PublicKey(rec.identity.PublicKey), canonical, sig) {
			return fmt.Errorf("%w: ed25519 verification failed", ErrInvalidAttestationSignature)
		}
		return nil
	case KeyAlgorithmECDSAP256:
		digest := sha256.Sum256(canonical)
		if !ecdsa.VerifyASN1(rec.ecdsaPub, digest[:], sig) {
			return fmt.Errorf("%w: ecdsa verification failed", ErrInvalidAttestationSignature)
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported algorithm %q", ErrInvalidAttestationSignature, rec.identity.Algorithm)
	}
}

func canonicalReceiptBytes(r *AttestationReceipt) []byte {
	s := fmt.Sprintf("%s%s|%s|%s|%s|%d|%s|%s|%s|%s|%d",
		attestationDomainTag,
		r.ReceiptID,
		r.HostID,
		r.TaskID,
		r.ChallengeID,
		r.Sequence,
		r.Nonce,
		r.TaskToken,
		r.PayloadDigest,
		r.Algorithm,
		r.IssuedAt.UTC().UnixNano(),
	)
	return []byte(s)
}

func computeTaskToken(secret []byte, ch *AttestationChallenge) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(fmt.Sprintf("%s|%s|%d|%s|%s",
		ch.HostID, ch.TaskID, ch.Sequence, ch.Nonce, ch.PrevChainHash)))
	return hex.EncodeToString(mac.Sum(nil))
}

func hashStringsSHA256(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

type blankFieldCheck struct {
	value string
	label string
}

func validateNonBlankFields(checks []blankFieldCheck, sentinel error) error {
	for _, c := range checks {
		if err := strutil.ErrorOnBlank(c.value, "%w: %s cannot be blank", sentinel, c.label); err != nil {
			return err
		}
	}
	return nil
}

func randomBytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func randomHex(n int) (string, error) {
	b, err := randomBytes(n)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
