// Package authcred resolves CLI/MCP credentials to ACC-* accounts.
// TRACK: BLI-REDACTED — unique issued ZQK_API_KEY + seating inject.
package authcred

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

const (
	// SecretPrefix marks generated agent API keys (never an ACC-* id).
	SecretPrefix = "zqk_ak_"

	keyTypeAPIKey              = "api_key"
	keyTypePersonalAccessToken = "personal_access_token"
	sha256HashPrefix           = "sha256:"
	tokenMetaFingerprint       = "fingerprint"
	tokenMetaExpiration        = "expiration"
)

// HashAPIKey returns the stored credential_hash form for an API key / PAT secret.
func HashAPIKey(secret string) string {
	sum := sha256.Sum256([]byte(secret))
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

// ResolveSecret looks up an api_key / personal_access_token keystore entry by SHA256(secret).
// Returns ErrNotFound when no active match exists.
func ResolveSecret(projectRoot, secret string) (Match, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return Match{}, errfmt.Errorf("empty credential")
	}
	want := NormalizeCredentialHash(HashAPIKey(secret))
	dir := filepath.Join(projectRoot, paths.ProcessKeystoreDir)
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		return Match{}, errfmt.Newf("keystore unavailable").Wrap(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, readErr := fileutil.ReadFile(filepath.Join(dir, e.Name()))
		if readErr != nil {
			continue
		}
		var entry map[string]any
		if yaml.Unmarshal(data, &entry) != nil {
			continue
		}
		kt, _ := entry[objects.FieldKeyKeyType].(string)
		if kt != keyTypeAPIKey && kt != keyTypePersonalAccessToken {
			continue
		}
		if revoked, ok := entry[objects.FieldKeyRevoked].(bool); ok && revoked {
			continue
		}
		if expiresAt, ok := entry[objects.FieldKeyExpiresAt].(string); ok && expiresAt != "" {
			if t, parseErr := time.Parse(time.RFC3339, expiresAt); parseErr == nil && time.Now().UTC().After(t) {
				continue
			}
		}
		stored, _ := entry[objects.FieldKeyCredentialHash].(string)
		if NormalizeCredentialHash(stored) != want {
			continue
		}
		accountID, _ := entry[objects.FieldKeyAccountID].(string)
		if accountID == "" {
			continue
		}
		keyID, _ := entry[objects.FieldKeyID].(string)
		if keyID == "" {
			keyID = strings.TrimSuffix(e.Name(), ".yaml")
		}
		return Match{AccountID: accountID, KeyID: keyID}, nil
	}
	return Match{}, errfmt.Errorf("credential not found in keystore")
}

// LooksLikeIssuedSecret reports whether raw is an opaque issued key (not ACC-/ZQK-).
func LooksLikeIssuedSecret(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if strings.HasPrefix(raw, "ACC-") || strings.HasPrefix(raw, "ZQK-") {
		return false
	}
	if strings.HasPrefix(raw, "account:") {
		return false
	}
	return true
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
