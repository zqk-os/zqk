package crypto

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
)

func TestStamp_EdgeCases(t *testing.T) {
	t.Parallel()

	// 1. Invalid private key hex
	_, err := GenerateStamp("commit", "agent", "not_a_hex")
	if err == nil {
		t.Errorf("expected error for non-hex private key")
	}

	// 2. Invalid private key length
	_, err = GenerateStamp("commit", "agent", "01020304")
	if err == nil {
		t.Errorf("expected error for short private key")
	}

	// 3. Invalid public key hex
	_, err = VerifyStamp("some.jwt.token", "not_a_hex")
	if err == nil {
		t.Errorf("expected error for non-hex public key")
	}

	// 4. Invalid public key length
	_, err = VerifyStamp("some.jwt.token", "01020304")
	if err == nil {
		t.Errorf("expected error for short public key")
	}

	// 5. Valid key length but invalid jwt token string
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("ed25519 generate key: %v", err)
	}
	pubHex := hex.EncodeToString(pub)
	_, err = VerifyStamp("not_a_valid_jwt", pubHex)
	if err == nil {
		t.Errorf("expected error for malformed token string")
	}
}
