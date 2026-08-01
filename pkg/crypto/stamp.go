package crypto

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type StampClaims struct {
	Commit  string `json:"commit"`
	AgentID string `json:"agent_id"`
	jwt.RegisteredClaims
}

// GenerateStamp creates a JWT stamp signed with the provided EdDSA private key.
func GenerateStamp(commit, agentID, privateKeyHex string) (string, error) {
	keyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return "", fmt.Errorf("invalid private key hex: %w", err)
	}

	if len(keyBytes) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("invalid private key length, expected %d bytes", ed25519.PrivateKeySize)
	}

	privateKey := ed25519.PrivateKey(keyBytes)

	claims := StampClaims{
		Commit:  commit,
		AgentID: agentID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "zqk-agent",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	return token.SignedString(privateKey)
}

// VerifyStamp verifies the JWT stamp using the provided EdDSA public key.
func VerifyStamp(stampStr, publicKeyHex string) (*StampClaims, error) {
	keyBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid public key hex: %w", err)
	}

	if len(keyBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key length, expected %d bytes", ed25519.PublicKeySize)
	}

	publicKey := ed25519.PublicKey(keyBytes)

	token, err := jwt.ParseWithClaims(stampStr, &StampClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to verify stamp: %w", err)
	}

	claims, ok := token.Claims.(*StampClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid stamp claims")
	}

	return claims, nil
}
