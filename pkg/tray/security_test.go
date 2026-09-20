package tray

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"testing"
)

func TestIsPrivilegedArgv(t *testing.T) {
	tests := []struct {
		name      string
		argv      []string
		wantPriv  bool
		wantToken string
	}{
		{
			name:      "override flag",
			argv:      []string{"object", "promote", "--override", "--reason-code", "justification"},
			wantPriv:  true,
			wantToken: "--override",
		},
		{
			name:      "override flag with equals",
			argv:      []string{"object", "promote", "--override=true"},
			wantPriv:  true,
			wantToken: "--override",
		},
		{
			name:      "force flag",
			argv:      []string{"cache", "clear", "--force"},
			wantPriv:  true,
			wantToken: "--force",
		},
		{
			name:      "clear-cache flag",
			argv:      []string{"test", "run", "--clear-cache"},
			wantPriv:  true,
			wantToken: "--clear-cache",
		},
		{
			name:      "internal flag",
			argv:      []string{"system", "diagnostics", "--internal"},
			wantPriv:  true,
			wantToken: "--internal",
		},
		{
			name:      "object delete verb",
			argv:      []string{"object", "delete", "policy", "--all"},
			wantPriv:  true,
			wantToken: "object delete",
		},
		{
			name:      "standalone delete verb",
			argv:      []string{"delete", "item-123"},
			wantPriv:  true,
			wantToken: "delete",
		},
		{
			name:      "system shutdown verb",
			argv:      []string{"system", "shutdown"},
			wantPriv:  true,
			wantToken: "system shutdown",
		},
		{
			name:      "system purge verb",
			argv:      []string{"system", "purge"},
			wantPriv:  true,
			wantToken: "system purge",
		},
		{
			name:      "io reap verb",
			argv:      []string{"io", "reap"},
			wantPriv:  true,
			wantToken: "io reap",
		},
		{
			name:      "safe command whats-next",
			argv:      []string{"workflow", "whats-next"},
			wantPriv:  false,
			wantToken: "",
		},
		{
			name:      "safe command scheduler-status",
			argv:      []string{"scheduler", "status"},
			wantPriv:  false,
			wantToken: "",
		},
		{
			name:      "safe command object list",
			argv:      []string{"object", "list", "backlog_item"},
			wantPriv:  false,
			wantToken: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPriv, gotToken := IsPrivilegedArgv(tt.argv)
			if gotPriv != tt.wantPriv {
				t.Errorf("IsPrivilegedArgv(%v) gotPriv = %v, want %v", tt.argv, gotPriv, tt.wantPriv)
			}
			if gotToken != tt.wantToken {
				t.Errorf("IsPrivilegedArgv(%v) gotToken = %q, want %q", tt.argv, gotToken, tt.wantToken)
			}
		})
	}
}

func TestCanonicalPayload_Determinism(t *testing.T) {
	name := "purge-cache"
	argv := []string{"cache", "clear", "--force"}

	payload1, err := CanonicalPayload(name, argv)
	if err != nil {
		t.Fatalf("CanonicalPayload failed: %v", err)
	}

	payload2, err := CanonicalPayload(name, argv)
	if err != nil {
		t.Fatalf("CanonicalPayload failed: %v", err)
	}

	if string(payload1) != string(payload2) {
		t.Errorf("payload not deterministic: %q vs %q", payload1, payload2)
	}

	expected := `{"argv":["cache","clear","--force"],"name":"purge-cache"}`
	if string(payload1) != expected {
		t.Errorf("expected payload %s, got %s", expected, string(payload1))
	}

	// Test nil argv handles gracefully as empty array
	nilPayload, err := CanonicalPayload("empty", nil)
	if err != nil {
		t.Fatalf("CanonicalPayload nil argv failed: %v", err)
	}
	expectedNil := `{"argv":[],"name":"empty"}`
	if string(nilPayload) != expectedNil {
		t.Errorf("expected %s, got %s", expectedNil, string(nilPayload))
	}
}

func TestSignAndVerifyEntry_Success(t *testing.T) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pubHex := fmt.Sprintf("%x%x", privKey.PublicKey.X, privKey.PublicKey.Y)

	entry := Entry{
		Name:        "purge-cache",
		Description: "Hard reset local cache",
		Argv:        []string{"cache", "clear", "--force"},
	}

	accountID := "ACC-1785920548450214012-68b850c0"
	if err := SignEntry(&entry, privKey, accountID); err != nil {
		t.Fatalf("SignEntry failed: %v", err)
	}

	if entry.Signature == "" {
		t.Fatal("expected non-empty signature")
	}
	if entry.SignedBy != accountID {
		t.Fatalf("expected signed_by %q, got %q", accountID, entry.SignedBy)
	}

	if err := VerifyEntry(&entry, pubHex); err != nil {
		t.Fatalf("VerifyEntry failed: %v", err)
	}
}

func TestVerifyEntry_TamperDetection(t *testing.T) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pubHex := fmt.Sprintf("%x%x", privKey.PublicKey.X, privKey.PublicKey.Y)

	entry := Entry{
		Name: "test-task",
		Argv: []string{"test", "run"},
	}

	if err := SignEntry(&entry, privKey, "ACC-1"); err != nil {
		t.Fatalf("SignEntry failed: %v", err)
	}

	// Tamper with argv
	tampered := entry
	tampered.Argv = []string{"object", "delete", "policy", "--all"}

	err = VerifyEntry(&tampered, pubHex)
	if err == nil {
		t.Fatal("expected tamper detection error, got nil")
	}
	if err.Error() != "cryptographic signature verification failed: signature does not match entry contents" {
		t.Fatalf("unexpected error message: %v", err)
	}

	// Tamper with name
	tamperedName := entry
	tamperedName.Name = "malicious-task"
	err = VerifyEntry(&tamperedName, pubHex)
	if err == nil {
		t.Fatal("expected tamper detection error for name change, got nil")
	}

	// Unsigned entry
	unsigned := Entry{
		Name: "unsigned",
		Argv: []string{"test"},
	}
	err = VerifyEntry(&unsigned, pubHex)
	if err == nil || err.Error() != "entry is not signed" {
		t.Fatalf("expected 'entry is not signed', got: %v", err)
	}
}
