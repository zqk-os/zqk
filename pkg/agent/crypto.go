package agent

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"os"

	"github.com/lanceman/zqk/pkg/zqkenv"

	"github.com/golang-jwt/jwt/v5"
)

// GenerateStamp creates a new JWT stamp signed by a private key.
func GenerateStamp(privateKey ed25519.PrivateKey, claims jwt.MapClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	return token.SignedString(privateKey)
}

// VerifyStamp verifies a JWT stamp using an authorized public key.
func VerifyStamp(stamp string, publicKey ed25519.PublicKey) (jwt.MapClaims, error) {
	token, err := jwt.Parse(stamp, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return publicKey, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}))

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid stamp")
}

// GetAuthorizedPublicKey returns the authorized public key for verifying stamps.
// In a real implementation, this would read from a configured keystore or OIDC endpoint.
func GetAuthorizedPublicKey() (ed25519.PublicKey, error) {
	keyB64 := os.Getenv(zqkenv.AgentPubKey())
	if keyB64 == "" {
		// Fallback for development/testing: a hardcoded key pair.
		// DO NOT USE IN PRODUCTION.
		// Seeded key for testing purposes (seed: 1..32)
		pubKey := []byte{0x79, 0xb5, 0x56, 0x2e, 0x8f, 0xe6, 0x54, 0xf9, 0x40, 0x78, 0xb1, 0x12, 0xe8, 0xa9, 0x8b, 0xa7, 0x90, 0x1f, 0x85, 0x3a, 0xe6, 0x95, 0xbe, 0xd7, 0xe0, 0xe3, 0x91, 0x0b, 0xad, 0x04, 0x96, 0x64}
		return ed25519.PublicKey(pubKey), nil
	}
	keyBytes, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, err
	}
	if len(keyBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key size: %d", len(keyBytes))
	}
	return ed25519.PublicKey(keyBytes), nil
}
