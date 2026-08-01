package license

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func generateTestKeys(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate ed25519 keys: %v", err)
	}
	return pub, priv
}

func createToken(t *testing.T, priv ed25519.PrivateKey, features []string, exp time.Time) string {
	claims := Claims{
		Features: features,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
			Issuer:    "zqk-billing",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	signed, err := token.SignedString(priv)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

func TestValidator_Verify(t *testing.T) {
	pub, priv := generateTestKeys(t)
	validator := NewValidator(pub)

	t.Run("Valid Token", func(t *testing.T) {
		tokenString := createToken(t, priv, []string{"relay_access"}, time.Now().Add(1*time.Hour))
		claims, err := validator.Verify(tokenString)
		if err != nil {
			t.Fatalf("expected valid token, got error: %v", err)
		}
		if !claims.HasFeature("relay_access") {
			t.Error("expected relay_access feature")
		}
	})

	t.Run("Expired Token", func(t *testing.T) {
		tokenString := createToken(t, priv, []string{"relay_access"}, time.Now().Add(-1*time.Hour))
		_, err := validator.Verify(tokenString)
		if err != ErrExpiredToken {
			t.Errorf("expected ErrExpiredToken, got %v", err)
		}
	})

	t.Run("Invalid Signature (Wrong Key)", func(t *testing.T) {
		_, badPriv := generateTestKeys(t)
		tokenString := createToken(t, badPriv, []string{"relay_access"}, time.Now().Add(1*time.Hour))
		_, err := validator.Verify(tokenString)
		if err == nil {
			t.Error("expected error for token signed with wrong private key")
		}
	})
}

func TestValidator_RequireFeature(t *testing.T) {
	pub, priv := generateTestKeys(t)
	validator := NewValidator(pub)

	t.Run("Feature Present", func(t *testing.T) {
		tokenString := createToken(t, priv, []string{"relay_access", "mesh_federation"}, time.Now().Add(1*time.Hour))
		_, err := validator.RequireFeature(tokenString, "mesh_federation")
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("Feature Missing", func(t *testing.T) {
		tokenString := createToken(t, priv, []string{"relay_access"}, time.Now().Add(1*time.Hour))
		_, err := validator.RequireFeature(tokenString, "mesh_federation")
		if err != ErrMissingFeature {
			t.Errorf("expected ErrMissingFeature, got: %v", err)
		}
	})
}
