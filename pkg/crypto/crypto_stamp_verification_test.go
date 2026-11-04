// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package crypto

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestGenerateStamp_InvalidPrivateKeyHex(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		privHex string
	}{
		{name: "non-hex characters", privHex: "zztop-not-hex-key!!"},
		{name: "odd length hex", privHex: "12345"},
		{name: "spaces in hex", privHex: "01 02 03 04"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stamp, err := GenerateStamp("commit-123", "agent-456", tc.privHex)
			if err == nil {
				t.Fatalf("expected error for hex %q, got nil", tc.privHex)
			}
			if stamp != "" {
				t.Fatalf("expected empty stamp on error, got %q", stamp)
			}
			if !strings.Contains(err.Error(), "invalid private key hex") {
				t.Fatalf("unexpected error message: %v", err)
			}
		})
	}
}

func TestGenerateStamp_InvalidPrivateKeyLength(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		byteCount int
	}{
		{name: "empty bytes", byteCount: 0},
		{name: "short 16 bytes", byteCount: 16},
		{name: "short 32 bytes", byteCount: 32},
		{name: "off-by-one 63 bytes", byteCount: 63},
		{name: "oversized 65 bytes", byteCount: 65},
		{name: "large 128 bytes", byteCount: 128},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			raw := make([]byte, tc.byteCount)
			privHex := hex.EncodeToString(raw)

			stamp, err := GenerateStamp("commit-123", "agent-456", privHex)
			if err == nil {
				t.Fatalf("expected error for length %d, got nil", tc.byteCount)
			}
			if stamp != "" {
				t.Fatalf("expected empty stamp on error, got %q", stamp)
			}
			expectedErrMsg := fmt.Sprintf("invalid private key length, expected %d bytes", ed25519.PrivateKeySize)
			if !strings.Contains(err.Error(), expectedErrMsg) {
				t.Fatalf("expected error containing %q, got %v", expectedErrMsg, err)
			}
		})
	}
}

func TestGenerateStamp_ValidGeneration(t *testing.T) {
	t.Parallel()

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}

	privHex := hex.EncodeToString(priv)
	pubHex := hex.EncodeToString(pub)
	commit := "9f8e7d6c5b4a3f2e1d0c"
	agentID := "agent-autonomous-185"

	beforeTime := time.Now().Add(-1 * time.Second)
	stamp, err := GenerateStamp(commit, agentID, privHex)
	if err != nil {
		t.Fatalf("failed to generate stamp: %v", err)
	}
	afterTime := time.Now().Add(1 * time.Second)

	parts := strings.Split(stamp, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts in JWT stamp, got %d", len(parts))
	}

	claims, err := VerifyStamp(stamp, pubHex)
	if err != nil {
		t.Fatalf("failed to verify freshly generated stamp: %v", err)
	}

	assertStampClaimsValid(t, claims, commit, agentID, beforeTime, afterTime)
}

func assertStampClaimsValid(t *testing.T, claims *StampClaims, commit, agentID string, before, after time.Time) {
	t.Helper()

	if claims.Commit != commit {
		t.Errorf("commit mismatch: got %q, want %q", claims.Commit, commit)
	}
	if claims.AgentID != agentID {
		t.Errorf("agentID mismatch: got %q, want %q", claims.AgentID, agentID)
	}
	if claims.Issuer != "zqk-agent" {
		t.Errorf("issuer mismatch: got %q, want %q", claims.Issuer, "zqk-agent")
	}
	if claims.IssuedAt == nil {
		t.Fatalf("issuedAt claim is nil")
	}
	if claims.IssuedAt.Time.Before(before) || claims.IssuedAt.Time.After(after) {
		t.Errorf("issuedAt %v outside expected range [%v, %v]", claims.IssuedAt.Time, before, after)
	}
	if claims.ExpiresAt == nil {
		t.Fatalf("expiresAt claim is nil")
	}
	expectedExpiry := claims.IssuedAt.Time.Add(24 * time.Hour)
	diff := claims.ExpiresAt.Time.Sub(expectedExpiry)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("expiresAt unexpected: got %v, expected near %v", claims.ExpiresAt.Time, expectedExpiry)
	}
}

func TestVerifyStamp_InvalidPublicKeyHex(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		pubHex string
	}{
		{name: "non-hex string", pubHex: "malformed-hex-key!!"},
		{name: "odd length hex", pubHex: "abcdef1"},
		{name: "invalid characters", pubHex: "1234567890abcdefghijklmnopqrstuv"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := VerifyStamp("some.jwt.token", tc.pubHex)
			if err == nil {
				t.Fatalf("expected error for public key %q, got nil", tc.pubHex)
			}
			if claims != nil {
				t.Fatalf("expected nil claims on error, got %+v", claims)
			}
			if !strings.Contains(err.Error(), "invalid public key hex") {
				t.Fatalf("unexpected error message: %v", err)
			}
		})
	}
}

func TestVerifyStamp_InvalidPublicKeyLength(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		byteCount int
	}{
		{name: "empty bytes", byteCount: 0},
		{name: "short 16 bytes", byteCount: 16},
		{name: "off-by-one 31 bytes", byteCount: 31},
		{name: "oversized 33 bytes", byteCount: 33},
		{name: "private key length 64 bytes", byteCount: 64},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			raw := make([]byte, tc.byteCount)
			pubHex := hex.EncodeToString(raw)

			claims, err := VerifyStamp("some.jwt.token", pubHex)
			if err == nil {
				t.Fatalf("expected error for byte count %d, got nil", tc.byteCount)
			}
			if claims != nil {
				t.Fatalf("expected nil claims on error, got %+v", claims)
			}
			expectedErrMsg := fmt.Sprintf("invalid public key length, expected %d bytes", ed25519.PublicKeySize)
			if !strings.Contains(err.Error(), expectedErrMsg) {
				t.Fatalf("expected error containing %q, got %v", expectedErrMsg, err)
			}
		})
	}
}

func TestVerifyStamp_CorruptedTokenEnvelope(t *testing.T) {
	t.Parallel()

	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	pubHex := hex.EncodeToString(pub)

	corruptTokens := []struct {
		name  string
		token string
	}{
		{name: "empty token", token: ""},
		{name: "single segment", token: "justonepieceoftext"},
		{name: "two segments missing signature", token: "header.payload"},
		{name: "four segments oversized", token: "part1.part2.part3.part4"},
		{name: "malformed base64 segments", token: "???.###.&&&"},
	}

	for _, tc := range corruptTokens {
		t.Run(tc.name, func(t *testing.T) {
			claims, verifyErr := VerifyStamp(tc.token, pubHex)
			if verifyErr == nil {
				t.Fatalf("expected verification error for corrupt token %q, got nil", tc.name)
			}
			if claims != nil {
				t.Fatalf("expected nil claims for corrupt token, got %+v", claims)
			}
			if !strings.Contains(verifyErr.Error(), "failed to verify stamp") {
				t.Fatalf("unexpected error message: %v", verifyErr)
			}
		})
	}
}

func TestVerifyStamp_ExpiredTokenRejection(t *testing.T) {
	t.Parallel()

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	pubHex := hex.EncodeToString(pub)

	pastExpiry := time.Now().Add(-2 * time.Hour)
	pastIssued := time.Now().Add(-26 * time.Hour)

	expiredClaims := StampClaims{
		Commit:  "expired-commit-001",
		AgentID: "expired-agent-001",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(pastExpiry),
			IssuedAt:  jwt.NewNumericDate(pastIssued),
			Issuer:    "zqk-agent",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, expiredClaims)
	expiredStamp, err := token.SignedString(priv)
	if err != nil {
		t.Fatalf("failed to sign expired token: %v", err)
	}

	claims, err := VerifyStamp(expiredStamp, pubHex)
	if err == nil {
		t.Fatalf("expected error verifying expired stamp, got claims: %+v", claims)
	}
	if claims != nil {
		t.Fatalf("expected nil claims on expired token error, got %+v", claims)
	}
	if !strings.Contains(err.Error(), "token is expired") && !strings.Contains(err.Error(), "failed to verify stamp") {
		t.Fatalf("expected expired error, got: %v", err)
	}
}

func TestVerifyStamp_UnsupportedSigningMethodInjection(t *testing.T) {
	t.Parallel()

	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	pubHex := hex.EncodeToString(pub)

	testClaims := StampClaims{
		Commit:  "forged-commit",
		AgentID: "forged-agent",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "zqk-agent",
		},
	}

	t.Run("HMAC-SHA256 injection rejected", func(t *testing.T) {
		hmacToken := jwt.NewWithClaims(jwt.SigningMethodHS256, testClaims)
		hmacStamp, signErr := hmacToken.SignedString([]byte("shared-secret-key-32bytes-len!!"))
		if signErr != nil {
			t.Fatalf("failed to sign HMAC token: %v", signErr)
		}

		claims, verifyErr := VerifyStamp(hmacStamp, pubHex)
		if verifyErr == nil {
			t.Fatalf("expected error for HMAC injected token, got nil claims: %+v", claims)
		}
		if claims != nil {
			t.Fatalf("expected nil claims, got %+v", claims)
		}
	})

	t.Run("none algorithm injection rejected", func(t *testing.T) {
		noneToken := jwt.NewWithClaims(jwt.SigningMethodNone, testClaims)
		noneStamp, signErr := noneToken.SignedString(jwt.UnsafeAllowNoneSignatureType)
		if signErr != nil {
			t.Fatalf("failed to create none alg token: %v", signErr)
		}

		claims, verifyErr := VerifyStamp(noneStamp, pubHex)
		if verifyErr == nil {
			t.Fatalf("expected error for none alg token, got nil claims: %+v", claims)
		}
		if claims != nil {
			t.Fatalf("expected nil claims, got %+v", claims)
		}
	})
}

func TestVerifyStamp_TamperedPayloadAndSignature(t *testing.T) {
	t.Parallel()

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	privHex := hex.EncodeToString(priv)
	pubHex := hex.EncodeToString(pub)

	stamp, err := GenerateStamp("initial-commit-val", "legit-agent", privHex)
	if err != nil {
		t.Fatalf("failed to generate valid stamp: %v", err)
	}

	parts := strings.Split(stamp, ".")
	if len(parts) != 3 {
		t.Fatalf("unexpected token format: %s", stamp)
	}

	t.Run("tampered payload rejected", func(t *testing.T) {
		tamperedPayload := tamperBase64Segment(parts[1])
		tamperedStamp := parts[0] + "." + tamperedPayload + "." + parts[2]

		claims, verifyErr := VerifyStamp(tamperedStamp, pubHex)
		if verifyErr == nil {
			t.Fatalf("expected verification failure for tampered payload, got %+v", claims)
		}
		if claims != nil {
			t.Fatalf("expected nil claims for tampered payload, got %+v", claims)
		}
	})

	t.Run("tampered signature rejected", func(t *testing.T) {
		tamperedSig := tamperBase64Segment(parts[2])
		tamperedStamp := parts[0] + "." + parts[1] + "." + tamperedSig

		claims, verifyErr := VerifyStamp(tamperedStamp, pubHex)
		if verifyErr == nil {
			t.Fatalf("expected verification failure for tampered signature, got %+v", claims)
		}
		if claims != nil {
			t.Fatalf("expected nil claims for tampered signature, got %+v", claims)
		}
	})
}

func tamperBase64Segment(segment string) string {
	decoded, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		// Fallback simple flip if raw decode fails
		if len(segment) > 0 {
			if segment[0] == 'A' {
				return "B" + segment[1:]
			}
			return "A" + segment[1:]
		}
		return segment + "x"
	}
	if len(decoded) > 0 {
		decoded[0] ^= 0xFF
	}
	return base64.RawURLEncoding.EncodeToString(decoded)
}

func TestCrypto_ConcurrentSigningAndVerificationStress(t *testing.T) {
	t.Parallel()

	const workerCount = 10
	const iterations = 30

	var wg sync.WaitGroup
	errCh := make(chan error, workerCount*iterations*2)

	for w := 0; w < workerCount; w++ {
		workerID := w
		wg.Add(1)
		goroutinelabels.NewGoroutine("crypto_concurrency_worker", fmt.Sprintf("worker-%d", workerID)).
			StartSimple(func() {
				defer wg.Done()
				runConcurrentCryptoWorker(workerID, iterations, errCh)
			})
	}

	wg.Wait()
	close(errCh)

	for workerErr := range errCh {
		if workerErr != nil {
			t.Errorf("concurrency stress failure: %v", workerErr)
		}
	}
}

func runConcurrentCryptoWorker(workerID, iterations int, errCh chan<- error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		errCh <- fmt.Errorf("worker %d: generate key: %w", workerID, err)
		return
	}
	privHex := hex.EncodeToString(priv)
	pubHex := hex.EncodeToString(pub)

	for i := 0; i < iterations; i++ {
		commit := fmt.Sprintf("commit-%d-%d", workerID, i)
		agentID := fmt.Sprintf("agent-%d-%d", workerID, i)

		stamp, genErr := GenerateStamp(commit, agentID, privHex)
		if genErr != nil {
			errCh <- fmt.Errorf("worker %d iter %d: generate stamp: %w", workerID, i, genErr)
			return
		}

		claims, verifyErr := VerifyStamp(stamp, pubHex)
		if verifyErr != nil {
			errCh <- fmt.Errorf("worker %d iter %d: verify stamp: %w", workerID, i, verifyErr)
			return
		}

		if claims.Commit != commit || claims.AgentID != agentID {
			errCh <- fmt.Errorf("worker %d iter %d: claim mismatch (commit: %s, agent: %s)",
				workerID, i, claims.Commit, claims.AgentID)
			return
		}

		// Also verify negative check concurrently: wrong pub key must fail
		badPub := make([]byte, ed25519.PublicKeySize)
		copy(badPub, pub)
		badPub[0] ^= 0xFF
		badPubHex := hex.EncodeToString(badPub)

		_, negErr := VerifyStamp(stamp, badPubHex)
		if negErr == nil {
			errCh <- fmt.Errorf("worker %d iter %d: expected negative verification error, got nil", workerID, i)
			return
		}
	}
}
