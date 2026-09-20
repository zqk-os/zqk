package crypto_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/infrastructure/crypto"
)

func TestEd25519Manager_SignAndVerify(t *testing.T) {
	manager, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("failed to generate keypair: %v", err)
	}

	data := []byte("absolute truth")
	sig, err := manager.Sign(data)
	if err != nil {
		t.Fatalf("failed to sign: %v", err)
	}

	// 1. Valid signature
	valid, err := crypto.GlobalVerifier.Verify(data, sig, manager.PublicKey())
	if err != nil {
		t.Fatalf("verification error: %v", err)
	}
	if !valid {
		t.Errorf("expected signature to be valid")
	}

	// 2. Invalid data
	valid, _ = crypto.GlobalVerifier.Verify([]byte("different data"), sig, manager.PublicKey())
	if valid {
		t.Errorf("expected signature to be invalid for different data")
	}

	// 3. Invalid signature
	valid, _ = crypto.GlobalVerifier.Verify(data, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", manager.PublicKey())
	if valid {
		t.Errorf("expected signature to be invalid for tampered signature")
	}
}

func TestNewEd25519ManagerFromSeed(t *testing.T) {
	// A fixed seed for deterministic testing (32 bytes hex)
	seedHex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	manager, err := crypto.NewEd25519ManagerFromSeed(seedHex)
	if err != nil {
		t.Fatalf("failed to create manager from seed: %v", err)
	}

	pubKey := manager.PublicKey()
	if pubKey == "" {
		t.Errorf("expected public key to be populated")
	}

	// Re-create from same seed and check public key
	manager2, _ := crypto.NewEd25519ManagerFromSeed(seedHex)
	if manager2.PublicKey() != pubKey {
		t.Errorf("expected consistent public key from same seed")
	}
}
