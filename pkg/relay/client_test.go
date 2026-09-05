package relay

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/lanceman/zqk/pkg/license"
)

// Helper to generate a valid test token
func generateTestToken(t *testing.T, priv ed25519.PrivateKey, feature string) string {
	claims := license.Claims{
		Features: []string{feature},
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	signed, err := token.SignedString(priv)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

func TestClient_Connect(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	validator := license.NewValidator(pub)
	client := NewClient("wss://relay.zqkos.com", validator)

	t.Run("Valid License Connects", func(t *testing.T) {
		token := generateTestToken(t, priv, "relay_access")
		err := client.Connect(context.Background(), token)
		if err != nil {
			t.Errorf("expected successful connection, got: %v", err)
		}
	})

	t.Run("Missing Feature Denies Access", func(t *testing.T) {
		token := generateTestToken(t, priv, "some_other_feature")
		err := client.Connect(context.Background(), token)
		if err == nil {
			t.Error("expected connection to fail due to missing feature")
		}
	})

	t.Run("Invalid Token Denies Access", func(t *testing.T) {
		err := client.Connect(context.Background(), "invalid.token.string")
		if err == nil {
			t.Error("expected connection to fail due to invalid token format")
		}
	})
}
