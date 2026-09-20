package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestCryptoGenerateAndVerifyStamp(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}

	claims := jwt.MapClaims{
		"sub": "agent-007",
		"iss": "zqk:test",
	}

	stamp, err := GenerateStamp(priv, claims)
	if err != nil {
		t.Fatalf("GenerateStamp failed: %v", err)
	}
	if stamp == "" {
		t.Fatal("GenerateStamp returned empty stamp")
	}

	verifiedClaims, err := VerifyStamp(stamp, pub)
	if err != nil {
		t.Fatalf("VerifyStamp failed: %v", err)
	}

	if verifiedClaims["sub"] != "agent-007" {
		t.Errorf("expected sub 'agent-007', got %v", verifiedClaims["sub"])
	}
}

func TestGetAuthorizedPublicKey_FailClosed(t *testing.T) {
	// Ensure ZQK_AGENT_PUB_KEY is empty
	t.Setenv(zqkenv.AgentPubKey().Key, "")
	_, err := GetAuthorizedPublicKey()
	if err == nil {
		t.Fatal("expected error when ZQK_AGENT_PUB_KEY is unset, got nil")
	}
}

func TestGetAuthorizedPublicKey_Success(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	keyB64 := base64.StdEncoding.EncodeToString(pub)
	t.Setenv(zqkenv.AgentPubKey().Key, keyB64)

	gotPub, err := GetAuthorizedPublicKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(gotPub) != string(pub) {
		t.Fatal("retrieved public key does not match configured key")
	}
}
