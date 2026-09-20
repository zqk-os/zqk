package stamping

import (
	"crypto/ed25519"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func genKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	_ = pub
	return priv
}

func TestGenerateStamp(t *testing.T) {
	priv := genKey(t)
	claims := jwt.MapClaims{"sub": "test"}
	stamp, err := GenerateStamp(priv, claims)
	if err != nil {
		t.Fatalf("GenerateStamp: %v", err)
	}
	if !strings.HasPrefix(stamp, "ey") {
		t.Fatalf("expected stamp starting with 'ey', got %q", stamp[:20])
	}
	_ = stamp // actual verification tested next
}

func TestVerifyStamp_roundtrip(t *testing.T) {
	priv := genKey(t)
	pub := priv.Public().(ed25519.PublicKey)
	claims := jwt.MapClaims{"user": "alice", "exp": 9999999999}

	stamp, err := GenerateStamp(priv, claims)
	if err != nil {
		t.Fatalf("GenerateStamp: %v", err)
	}

	result, err := VerifyStamp(stamp, pub)
	if err != nil {
		t.Fatalf("VerifyStamp: %v", err)
	}
	if result["user"] != "alice" {
		t.Errorf("expected user=alice, got %v", result["user"])
	}
	_ = claims
}

func TestVerifyStamp_wrongKey(t *testing.T) {
	goodPriv := genKey(t)
	badPub := genKey(t).Public().(ed25519.PublicKey)
	claims := jwt.MapClaims{"x": "y"}

	stamp, err := GenerateStamp(goodPriv, claims)
	if err != nil {
		t.Fatalf("GenerateStamp: %v", err)
	}

	_, err = VerifyStamp(stamp, badPub)
	if err == nil {
		t.Fatal("expected error verifying with wrong key")
	}
	if !strings.Contains(err.Error(), "invalid stamp") &&
		!strings.Contains(err.Error(), ErrInvalidMethod.Error()) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVerifyStamp_invalidSignature(t *testing.T) {
	badPub := genKey(t).Public().(ed25519.PublicKey)
	_, err := VerifyStamp("corrupt.token.here", badPub)
	if err == nil {
		t.Fatal("expected error for corrupt token")
	}
}

func TestErrValues(t *testing.T) {
	for _, e := range []error{ErrInvalidMethod, ErrInvalidPublicKeySize, ErrInvalidStamp} {
		if e.Error() == "" {
			t.Errorf("%T has empty Error()", e)
		}
	}
}
