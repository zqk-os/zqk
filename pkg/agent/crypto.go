// Package agent provides cryptographic utilities for the ZQK agent subsystem.
//
// Key Management & Rotation Practices:
//
//  1. Algorithm: ED25519 is used for all agent stamp (JWT) signing and verification.
//     It is chosen for its performance, small signature size, and strong security properties.
//  2. Secret Management: Private keys MUST NOT be hardcoded or stored in plaintext
//     configuration files. They must be loaded dynamically via secure environment
//     variables (e.g., AGENT_PUB_KEY, brand-prefixed), a dedicated secrets manager, or an OIDC endpoint.
//  3. Key Rotation: Keys should be rotated on a regular schedule (e.g., every 90 days)
//     or immediately upon suspected compromise. During rotation, systems should temporarily
//     trust both old and new public keys until all stamps signed by the old key have
//     expired.
//  4. Fail-Closed Security: There is NO hardcoded public key fallback.
//     Environments MUST supply a valid public key via AGENT_PUB_KEY (brand-prefixed) or
//     verification fails closed.
package agent

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zqk-os/zqk/pkg/zqkenv"
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
	keyB64 := zqkenv.AgentPubKey().Get()
	if keyB64 == "" {
		return nil, fmt.Errorf("%s environment variable is not set; cryptographic verification must fail closed", zqkenv.AgentPubKey().Name())
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
