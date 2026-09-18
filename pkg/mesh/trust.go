package mesh

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
)

// SyncedState represents state coming from a remote Hive Mind node.
type SyncedState struct {
	NodeID    string
	Payload   []byte
	Signature string // Hex encoded signature
}

var (
	ErrInvalidSignature = errors.New("invalid cryptographic signature")
	ErrUnknownNode      = errors.New("unknown remote node")
	ErrInvalidPublicKey = errors.New("invalid public key format")
)

// TrustVerifier ensures that state synced from remote nodes is cryptographically valid.
type TrustVerifier struct {
	registry Registry
}

// NewTrustVerifier creates a new TrustVerifier.
func NewTrustVerifier(r Registry) *TrustVerifier {
	return &TrustVerifier{
		registry: r,
	}
}

// VerifyState cryptographically verifies a piece of state from a remote node.
func (t *TrustVerifier) VerifyState(ctx context.Context, state SyncedState) error {
	remote, err := t.registry.Verify(ctx, state.NodeID)
	if err != nil {
		return err
	}
	if remote == nil {
		return ErrUnknownNode
	}

	pubKeyBytes, err := hex.DecodeString(remote.PublicKey)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		return ErrInvalidPublicKey
	}

	sigBytes, err := hex.DecodeString(state.Signature)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return ErrInvalidSignature
	}

	pubKey := ed25519.PublicKey(pubKeyBytes)

	if !ed25519.Verify(pubKey, state.Payload, sigBytes) {
		return ErrInvalidSignature
	}

	return nil
}
