package license

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestElevationCycle307_LicenseBoundaries(t *testing.T) {
	pub, _ := generateTestKeys(t)
	validator := NewValidator(pub)

	// 1. HasFeature false branch
	claims := &Claims{Features: []string{"relay_access"}}
	if claims.HasFeature("nonexistent_feature") {
		t.Fatalf("expected false for nonexistent feature")
	}

	// 2. RequireFeature error pass-through
	if _, err := validator.RequireFeature("malformed-token-string", "relay_access"); err == nil {
		t.Fatalf("expected error for malformed token string in RequireFeature")
	}

	// 3. Unexpected signing method (downgrade attack prevention)
	hmacClaims := Claims{
		Features: []string{"relay_access"},
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			Issuer:    "attacker",
		},
	}
	hmacToken := jwt.NewWithClaims(jwt.SigningMethodHS256, hmacClaims)
	signedHMAC, err := hmacToken.SignedString([]byte("symmetric-secret-key-12345"))
	if err != nil {
		t.Fatalf("failed to sign HMAC token: %v", err)
	}

	if _, err := validator.Verify(signedHMAC); err == nil {
		t.Fatalf("expected error when validating HMAC token against Ed25519 validator")
	} else if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken wrap, got: %v", err)
	}

	// 4. Verify error strings
	if ErrInvalidToken.Error() != "invalid cryptographic license token" {
		t.Fatalf("unexpected ErrInvalidToken message: %s", ErrInvalidToken.Error())
	}
	if ErrExpiredToken.Error() != "license has expired" {
		t.Fatalf("unexpected ErrExpiredToken message: %s", ErrExpiredToken.Error())
	}
	if ErrMissingFeature.Error() != "license does not grant access to this feature" {
		t.Fatalf("unexpected ErrMissingFeature message: %s", ErrMissingFeature.Error())
	}
}
