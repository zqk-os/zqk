package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// Signer defines the interface for creating cryptographic signatures.
type Signer interface {
	Sign(data []byte) (string, error)
	PublicKey() string
}

// Verifier defines the interface for verifying cryptographic signatures.
type Verifier interface {
	Verify(data []byte, signature string, publicKey string) (bool, error)
}

// Ed25519Manager implements both Signer and Verifier using Ed25519.
type Ed25519Manager struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

// GenerateKeypair creates a new random Ed25519 keypair.
func GenerateKeypair() (*Ed25519Manager, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Ed25519Manager{
		privateKey: priv,
		publicKey:  pub,
	}, nil
}

// NewEd25519ManagerFromSeed recreates a manager from a hex-encoded private key.
func NewEd25519ManagerFromSeed(seedHex string) (*Ed25519Manager, error) {
	seed, err := hex.DecodeString(seedHex)
	if err != nil {
		return nil, err
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("invalid seed size: expected %d, got %d", ed25519.SeedSize, len(seed))
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	return &Ed25519Manager{
		privateKey: priv,
		publicKey:  pub,
	}, nil
}

func (m *Ed25519Manager) Sign(data []byte) (string, error) {
	sig := ed25519.Sign(m.privateKey, data)
	return hex.EncodeToString(sig), nil
}

func (m *Ed25519Manager) PublicKey() string {
	return hex.EncodeToString(m.publicKey)
}

func (m *Ed25519Manager) Verify(data []byte, signature string, publicKey string) (bool, error) {
	sig, err := hex.DecodeString(signature)
	if err != nil {
		return false, err
	}
	pub, err := hex.DecodeString(publicKey)
	if err != nil {
		return false, err
	}
	if len(pub) != ed25519.PublicKeySize {
		return false, fmt.Errorf("invalid public key size")
	}
	return ed25519.Verify(pub, data, sig), nil
}

// GlobalVerifier provides a stateless verification helper.
var GlobalVerifier Verifier = &Ed25519Manager{}
