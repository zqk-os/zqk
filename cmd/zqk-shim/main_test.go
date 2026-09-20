package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/authcred"
	"github.com/zqk-os/zqk/pkg/crypto"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestIsMutatingOperation(t *testing.T) {
	tests := []struct {
		cmdName  string
		args     []string
		expected bool
	}{
		{"git", []string{"commit", "-m", "msg"}, true},
		{"git", []string{"status"}, false},
		{"gh", []string{"pr", "create"}, true},
		{"gh", []string{"issue", "list"}, false},
		{"other", []string{"commit"}, false},
	}

	for _, tt := range tests {
		result := isMutatingOperation(tt.cmdName, tt.args)
		if result != tt.expected {
			t.Errorf("isMutatingOperation(%q, %v) = %v; want %v", tt.cmdName, tt.args, result, tt.expected)
		}
	}
}

func TestValidatePOLCODE009_BypassRequiresBreakGlass(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		wantErr     bool
		errContains string
	}{
		{
			name: "bypass without break glass is rejected",
			env: map[string]string{
				zqkenv.ZqkShimBypassPolCode009().Name(): "1",
			},
			wantErr:     true,
			errContains: "unvalidated POL-CODE-009 bypass rejected: human break-glass requires explicit justification",
		},
		{
			name: "bypass with short break glass reason is rejected",
			env: map[string]string{
				zqkenv.ZqkShimBypassPolCode009().Name(): "1",
				zqkenv.BreakGlassReason().Name():        "too short",
			},
			wantErr:     true,
			errContains: "justification too short",
		},
		{
			name: "bypass with valid break glass reason (>= 30 chars) succeeds",
			env: map[string]string{
				zqkenv.ZqkShimBypassPolCode009().Name(): "1",
				zqkenv.BreakGlassReason().Name():        "emergency hotfix for outage approved by lead engineer",
			},
			wantErr: false,
		},
		{
			name: "git prefixed bypass with valid break glass reason succeeds",
			env: map[string]string{
				"GIT_ZQK_SHIM_BYPASS_POLCODE009": "1",
				"ZQK_BREAK_GLASS_REASON":         "emergency hotfix for outage approved by lead engineer",
			},
			wantErr: false,
		},
		{
			name: "bypass with whitespace-only break glass is rejected",
			env: map[string]string{
				zqkenv.ZqkShimBypassPolCode009().Name(): "1",
				zqkenv.BreakGlassReason().Name():        "                                  ",
			},
			wantErr:     true,
			errContains: "human break-glass requires explicit justification in ZQK_BREAK_GLASS_REASON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string {
				return tt.env[key]
			}
			err := validatePOLCODE009WithEnv("/nonexistent/fake/root", getenv)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			}
		})
	}
}

func TestResolveTrustedAgentPrivateKey(t *testing.T) {
	tempDir := t.TempDir()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	privHex := hex.EncodeToString(priv)
	pubHex := hex.EncodeToString(pub)

	agentID := "ACC-1785920548450214000-80bb9c63"

	// 1. Ambient key without break-glass is rejected
	t.Run("ambient key without break-glass is rejected", func(t *testing.T) {
		getenv := func(key string) string {
			if key == zqkenv.AgentPrivateKey().Name() {
				return privHex
			}
			return ""
		}
		_, err := resolveTrustedAgentPrivateKey(tempDir, agentID, getenv)
		if err == nil {
			t.Fatal("expected ambient key to be rejected without break-glass")
		}
		if !strings.Contains(err.Error(), "unvalidated ambient private key rejected") {
			t.Errorf("expected rejection error, got: %v", err)
		}
	})

	// 2. Ambient key with valid break-glass reason is accepted
	t.Run("ambient key with valid break-glass reason is accepted", func(t *testing.T) {
		getenv := func(key string) string {
			if key == zqkenv.AgentPrivateKey().Name() {
				return privHex
			}
			if key == zqkenv.BreakGlassReason().Name() {
				return "manual break-glass for isolated test environment execution"
			}
			return ""
		}
		key, err := resolveTrustedAgentPrivateKey(tempDir, agentID, getenv)
		if err != nil {
			t.Fatalf("expected ambient key with break-glass to succeed, got: %v", err)
		}
		if key != privHex {
			t.Errorf("expected %q, got %q", privHex, key)
		}
	})

	// 3. Project seating credential is trusted without ambient env
	t.Run("project seating credential is trusted", func(t *testing.T) {
		if err := authcred.WriteSeatCredential(tempDir, agentID, privHex); err != nil {
			t.Fatalf("WriteSeatCredential: %v", err)
		}
		getenv := func(key string) string {
			return "" // No ambient env
		}
		key, err := resolveTrustedAgentPrivateKey(tempDir, agentID, getenv)
		if err != nil {
			t.Fatalf("expected seating key to be loaded, got: %v", err)
		}
		if key != privHex {
			t.Errorf("expected %q, got %q", privHex, key)
		}

		// Verify stamp can be generated and verified with public key
		stamp := generateCryptographicStampWithEnv(tempDir, func(key string) string {
			if key == zqkenv.AgentID().Name() {
				return agentID
			}
			return ""
		})
		claims, verifyErr := crypto.VerifyStamp(stamp, pubHex)
		if verifyErr != nil {
			t.Fatalf("VerifyStamp failed: %v", verifyErr)
		}
		if claims.AgentID != agentID {
			t.Errorf("claims.AgentID = %q; want %q", claims.AgentID, agentID)
		}
	})
}
