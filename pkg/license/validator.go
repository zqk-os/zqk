package license

import (
	"crypto/ed25519"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken   = errors.New("invalid cryptographic license token")
	ErrExpiredToken   = errors.New("license has expired")
	ErrMissingFeature = errors.New("license does not grant access to this feature")
)

// Claims represents the ZQK specific payload inside the JWT.
type Claims struct {
	Features []string `json:"features"`
	jwt.RegisteredClaims
}

// HasFeature checks if a specific feature (e.g., "relay_access") is granted by the license.
func (c *Claims) HasFeature(feature string) bool {
	for _, f := range c.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// Validator handles offline cryptographic verification of ZQK licenses.
type Validator struct {
	publicKey ed25519.PublicKey
}

// NewValidator creates a new license validator initialized with ZQK's public Ed25519 key.
// In production, this public key will be hardcoded into the compiled binary.
func NewValidator(publicKey ed25519.PublicKey) *Validator {
	return &Validator{
		publicKey: publicKey,
	}
}

// Verify decodes, verifies the cryptographic signature, and checks the expiration of the token.
func (v *Validator) Verify(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Force the algorithm to be EdDSA (Ed25519) to prevent downgrade attacks.
		if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return v.publicKey, nil
	}, jwt.WithValidMethods([]string{"EdDSA"}))

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrInvalidToken
}

// RequireFeature wraps Verify and enforces that a specific feature exists in the claims.
func (v *Validator) RequireFeature(tokenString, feature string) (*Claims, error) {
	claims, err := v.Verify(tokenString)
	if err != nil {
		return nil, err
	}

	if !claims.HasFeature(feature) {
		return nil, ErrMissingFeature
	}

	return claims, nil
}
