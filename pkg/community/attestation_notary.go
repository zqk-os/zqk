package community

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

var (
	ErrAttestationInvalid  = errors.New("attestation: claim is invalid or missing required fields")
	ErrAttestationTampered = errors.New("attestation: signature or digest verification failed")
	ErrReplayDetected      = errors.New("attestation: replay attack detected, monotonic epoch violation")
	ErrNotaryEntryNotFound = errors.New("attestation: notary entry not found in ledger")
)

// AttestationClaim encapsulates a verifiable claim made by an autonomous provider.
type AttestationClaim struct {
	ProviderID string    `json:"provider_id"`
	Epoch      int64     `json:"epoch"`
	Action     string    `json:"action"`
	PayloadSum string    `json:"payload_sum"`
	Timestamp  time.Time `json:"timestamp"`
	Signature  string    `json:"signature,omitempty"`
}

// Validate checks static structural invariants.
func (c *AttestationClaim) Validate() error {
	if c == nil {
		return ErrAttestationInvalid
	}
	if strutil.IsBlank(c.ProviderID) {
		return fmt.Errorf("%w: missing provider_id", ErrAttestationInvalid)
	}
	if c.Epoch <= 0 {
		return fmt.Errorf("%w: epoch must be strictly positive", ErrAttestationInvalid)
	}
	if strutil.IsBlank(c.Action) {
		return fmt.Errorf("%w: missing action", ErrAttestationInvalid)
	}
	if strutil.IsBlank(c.PayloadSum) {
		return fmt.Errorf("%w: missing payload_sum", ErrAttestationInvalid)
	}
	return nil
}

// ComputeDigest generates the canonical HMAC-SHA256 signature for the claim.
func (c *AttestationClaim) ComputeDigest(secretKey []byte) string {
	mac := hmac.New(sha256.New, secretKey)
	canonical := fmt.Sprintf("%s:%d:%s:%s:%d", c.ProviderID, c.Epoch, c.Action, c.PayloadSum, c.Timestamp.UnixNano())
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

// AttestationNotary manages notary ledger entries and tamper-proof verification.
type AttestationNotary struct {
	mu         sync.RWMutex
	secretKey  []byte
	lastEpochs map[string]int64
	entries    []*AttestationClaim
}

// NewAttestationNotary initializes an attestation notary with a shared signing secret.
func NewAttestationNotary(secretKey []byte) *AttestationNotary {
	keyCopy := append([]byte(nil), secretKey...)
	return &AttestationNotary{
		secretKey:  keyCopy,
		lastEpochs: make(map[string]int64),
		entries:    make([]*AttestationClaim, 0),
	}
}

// Attest creates, signs, and notarizes a new provider execution claim.
func (n *AttestationNotary) Attest(ctx context.Context, providerID, action string, epoch int64, payload []byte) (*AttestationClaim, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	hasher := sha256.New()
	hasher.Write(payload)
	payloadSum := hex.EncodeToString(hasher.Sum(nil))

	claim := &AttestationClaim{
		ProviderID: providerID,
		Epoch:      epoch,
		Action:     action,
		PayloadSum: payloadSum,
		Timestamp:  time.Now(),
	}

	if err := claim.Validate(); err != nil {
		return nil, err
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	lastEpoch := n.lastEpochs[providerID]
	if epoch <= lastEpoch {
		return nil, fmt.Errorf("%w: provided epoch %d <= last recorded epoch %d", ErrReplayDetected, epoch, lastEpoch)
	}

	claim.Signature = claim.ComputeDigest(n.secretKey)
	n.lastEpochs[providerID] = epoch
	n.entries = append(n.entries, claim)

	return claim, nil
}

// Verify attestation verifies that the claim signature and payload match.
func (n *AttestationNotary) Verify(claim *AttestationClaim, payload []byte) error {
	if err := claim.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(claim.Signature) == "" {
		return fmt.Errorf("%w: missing signature", ErrAttestationTampered)
	}

	hasher := sha256.New()
	hasher.Write(payload)
	actualSum := hex.EncodeToString(hasher.Sum(nil))

	if !strings.EqualFold(claim.PayloadSum, actualSum) {
		return fmt.Errorf("%w: payload digest mismatch", ErrAttestationTampered)
	}

	n.mu.RLock()
	expectedSig := claim.ComputeDigest(n.secretKey)
	n.mu.RUnlock()

	if !hmac.Equal([]byte(claim.Signature), []byte(expectedSig)) {
		return fmt.Errorf("%w: signature mismatch", ErrAttestationTampered)
	}

	return nil
}

// GetLedgerSize returns the number of notarized claims in the ledger.
func (n *AttestationNotary) GetLedgerSize() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.entries)
}
