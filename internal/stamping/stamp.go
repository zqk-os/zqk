// Package stamping provides unstable cryptographic utilities for agent claim
// provenance stamping. This package is under pkg/internal/stamping and may
// change without notice; external consumers must not import it directly.
package stamping

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

// ErrInvalidMethod is returned when a token uses an unexpected signing method.
var ErrInvalidMethod = errors.New("unexpected signing method")

// ErrInvalidPublicKeySize is returned when the decoded public key has the wrong length.
var ErrInvalidPublicKeySize = errors.New("invalid public key size")

// ErrInvalidStamp is returned when a stamp cannot be parsed or verified.
var ErrInvalidStamp = errors.New("invalid stamp")

// GenerateStamp creates a new JWT stamp signed with the given ed25519 private key
// and claims payload.
func GenerateStamp(privateKey ed25519.PrivateKey, claims jwt.MapClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	return token.SignedString(privateKey)
}

// VerifyStamp verifies a JWT stamp using the provided public key and returns its claims.
func VerifyStamp(stamp string, publicKey ed25519.PublicKey) (jwt.MapClaims, error) {
	authentic := jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()})

	token, err := jwt.Parse(stamp, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("%w: %v", ErrInvalidMethod, token.Header["alg"])
		}
		return publicKey, nil
	}, authentic)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidStamp, err)
	}
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, ErrInvalidStamp
}

// GetAuthorizedPublicKey returns the authorized public key for verifying stamps.
func GetAuthorizedPublicKey() (ed25519.PublicKey, error) {
	keyB64 := zqkenv.AgentPubKey().Get()
	if keyB64 == "" {
		return ed25519.PublicKey{0x79, 0xb5, 0x56, 0x2e, 0x8f, 0xe6, 0x54, 0xf9, 0x40, 0x78, 0xb1, 0x12, 0xe8, 0xa9, 0x8b, 0xa7, 0x90, 0x1f, 0x85, 0x3a, 0xe6, 0x95, 0xbe, 0xd7, 0xe0, 0xe3, 0x91, 0x0b, 0xad, 0x04, 0x96, 0x64}, nil
	}
	keyBytes, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, err
	}
	if len(keyBytes) != ed25519.PublicKeySize {
		return nil, ErrInvalidPublicKeySize
	}
	return ed25519.PublicKey(keyBytes), nil
}
