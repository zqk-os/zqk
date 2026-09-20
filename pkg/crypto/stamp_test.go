package crypto

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
)

func TestGenerateAndVerifyStamp(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	privHex := hex.EncodeToString(priv)
	pubHex := hex.EncodeToString(pub)

	commit := "abc123def456"
	agentID := "test-agent"

	stamp, err := GenerateStamp(commit, agentID, privHex)
	if err != nil {
		t.Fatalf("failed to generate stamp: %v", err)
	}
	if stamp == "" {
		t.Fatal("expected non-empty stamp")
	}

	claims, err := VerifyStamp(stamp, pubHex)
	if err != nil {
		t.Fatalf("failed to verify stamp: %v", err)
	}

	if claims.Commit != commit {
		t.Errorf("expected commit %q, got %q", commit, claims.Commit)
	}
	if claims.AgentID != agentID {
		t.Errorf("expected agentID %q, got %q", agentID, claims.AgentID)
	}
}

func TestVerifyStamp_InvalidKey(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	privHex := hex.EncodeToString(priv)
	stamp, err := GenerateStamp("commit", "agent", privHex)
	if err != nil {
		t.Fatalf("failed to generate stamp: %v", err)
	}

	// Generate a different key
	pub2, _, _ := ed25519.GenerateKey(nil)
	pubHex2 := hex.EncodeToString(pub2)

	_, err = VerifyStamp(stamp, pubHex2)
	if err == nil {
		t.Fatal("expected verification to fail with wrong public key")
	}
}
