// Package authcred resolves CLI/MCP credentials to ACC-* accounts.
// TRACK: unique issued ZQK_API_KEY + seating inject.
package authcred

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	// APIKeyPrefix marks generated agent API keys (never an ACC-* id).
	APIKeyPrefix = "zqk_ak_"

	keyTypeAPIKey              = "api_key"
	keyTypePersonalAccessToken = "personal_access_token"
	sha256HashPrefix           = "sha256:"
	tokenMetaFingerprint       = "fingerprint"
	tokenMetaExpiration        = "expiration"
)

// HashAPIKey returns the stored credential_hash form for an API key / PAT secret.
func HashAPIKey(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return sha256HashPrefix + hex.EncodeToString(sum[:])
}

// NormalizeCredentialHash returns the hex digest for comparison.
func NormalizeCredentialHash(stored string) string {
	stored = strings.TrimSpace(stored)
	if strings.HasPrefix(stored, sha256HashPrefix) {
		return strings.TrimSpace(stored[len(sha256HashPrefix):])
	}
	return stored
}

// Match holds a keystore hit for a presented secret.
type Match struct {
	AccountID string
	KeyID     string
}

// ResolveAPIKey looks up an api_key / personal_access_token keystore entry by SHA256(rawKey).
// Returns ErrNotFound when no active match exists.
func ResolveAPIKey(projectRoot, rawKey string) (Match, error) {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return Match{}, errfmt.Errorf("empty credential")
	}
	want := NormalizeCredentialHash(HashAPIKey(rawKey))
	recs, err := ListKeystoreRecords(projectRoot)
	if err != nil {
		return Match{}, errfmt.Newf("keystore unavailable").Wrap(err)
	}
	for _, rec := range recs {
		kt := rec.KeyType()
		if kt != keyTypeAPIKey && kt != keyTypePersonalAccessToken {
			continue
		}
		if rec.Revoked() {
			continue
		}
		if expiresAt := rec.ExpiresAt(); expiresAt != "" {
			if t, parseErr := time.Parse(time.RFC3339, expiresAt); parseErr == nil && time.Now().UTC().After(t) {
				continue
			}
		}
		if NormalizeCredentialHash(rec.CredentialHash()) != want {
			continue
		}
		accountID := rec.AccountID()
		if accountID == "" {
			continue
		}
		return Match{AccountID: accountID, KeyID: rec.KeyID}, nil
	}
	return Match{}, errfmt.Errorf("credential not found in keystore")
}

// LooksLikeIssuedAPIKey reports whether raw is an opaque issued key
// (not ACC-*, session ZS-/ZQK-*, or retired account: form).
func LooksLikeIssuedAPIKey(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if LooksLikeSessionToken(raw) {
		return false
	}
	if strings.HasPrefix(raw, "ACC-") {
		return false
	}
	if strings.HasPrefix(raw, "account:") {
		return false
	}
	return true
}

// LooksLikeSessionToken reports ZS-* (canonical session) or the ZQK-* synonym
// written by older login paths into credentials.
func LooksLikeSessionToken(raw string) bool {
	raw = strings.TrimSpace(raw)
	return strings.HasPrefix(raw, "ZS-") || strings.HasPrefix(raw, "ZQK-")
}

// TokenFingerprintMeta builds an account.tokens[] entry (fingerprint only; never raw secret).
func TokenFingerprintMeta(keyID, fingerprintHash, expiresAt string) map[string]any {
	meta := map[string]any{
		objects.FieldKeyTokenID: keyID,
		tokenMetaFingerprint:    fingerprintHash,
	}
	if expiresAt != "" {
		meta[tokenMetaExpiration] = expiresAt
	}
	return meta
}
