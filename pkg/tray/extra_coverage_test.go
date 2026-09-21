package tray

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"strings"
	"testing"
)

func TestFind(t *testing.T) {
	entries := []Entry{
		{Name: "quick-start", Argv: []string{"system", "status"}},
		{Name: "intake", Argv: []string{"intake", "new task"}},
	}

	found := Find(entries, "quick-start")
	if found == nil || found.Name != "quick-start" {
		t.Fatalf("expected to find quick-start, got %v", found)
	}

	if missing := Find(entries, "non-existent"); missing != nil {
		t.Fatalf("expected nil for non-existent entry, got %v", missing)
	}
}

func TestFormatExplainLine(t *testing.T) {
	// Empty binary name defaults to zqk
	line1 := FormatExplainLine("", []string{"status", "--all"})
	if line1 != "zqk status --all" {
		t.Fatalf("expected 'zqk status --all', got %q", line1)
	}

	// Argv with whitespace/quotes
	line2 := FormatExplainLine("myzqk", []string{"intake", "task with spaces", "simple"})
	if !strings.Contains(line2, `"task with spaces"`) {
		t.Fatalf("expected quoted argument in line: %s", line2)
	}
}

func TestListNames(t *testing.T) {
	entries := []Entry{
		{Name: "zebra"},
		{Name: "apple"},
		{Name: "mango"},
	}

	names := ListNames(entries)
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}
	if names[0] != "apple" || names[1] != "mango" || names[2] != "zebra" {
		t.Fatalf("expected sorted names, got %v", names)
	}
}

func TestSecurity_ErrorBranches(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// SignEntry nil checks
	if err := SignEntry(nil, priv, "acc-1"); err == nil {
		t.Fatal("expected error for nil entry")
	}
	entry := &Entry{Name: "test", Argv: []string{"echo"}}
	if err := SignEntry(entry, nil, "acc-1"); err == nil {
		t.Fatal("expected error for nil private key")
	}

	// ParsePublicKeyHex edge cases
	if _, err := ParsePublicKeyHex(""); err == nil {
		t.Fatal("expected error for empty public key")
	}
	if _, err := ParsePublicKeyHex("04short"); err == nil {
		t.Fatal("expected error for short hex")
	}
	// Invalid hex coordinates
	invalidHex := strings.Repeat("z", 64)
	if _, err := ParsePublicKeyHex(invalidHex); err == nil {
		t.Fatal("expected error for invalid hex characters")
	}

	// VerifyEntry nil and unsigned checks
	if err := VerifyEntry(nil, "abc"); err == nil {
		t.Fatal("expected error for nil entry")
	}
	unsignedEntry := &Entry{Name: "unsigned", Argv: []string{"run"}}
	if err := VerifyEntry(unsignedEntry, "abc"); err == nil {
		t.Fatal("expected error for unsigned entry")
	}
}
