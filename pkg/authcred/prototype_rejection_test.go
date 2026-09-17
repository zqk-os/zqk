package authcred

import (
	"strings"
	"testing"
)

const (
	hashTestUser2    = "ACC-1785920548450214015-3df55bd1"
	hashTestAgent2   = "ACC-1785920548450214016-ace2aae1"
	validHexSuffix2  = "ACC-1785920548450214015-deadbeef1234"
	validStandardID2 = "ACC-999"
)

func TestIsPrototypeAccount_KnownHashesUnit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		ref      string
		expected bool
	}{
		{"known test-user", hashTestUser2, true},
		{"known test-agent uppercase", strings.ToUpper(hashTestAgent2), true},
		{"valid account", validHexSuffix2, false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsPrototypeAccount(tt.ref); got != tt.expected {
				t.Errorf("IsPrototypeAccount(%q) = %v; want %v", tt.ref, got, tt.expected)
			}
		})
	}
}

func TestIsPrototypeAccount_SuffixDetectionUnit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		ref      string
		expected bool
	}{
		{"suffix proto", "ACC-abc-abcd-proto-xyz", true},
		{"suffix test", "ACC-def-abcd-test-user", true},
		{"suffix PROTOTYPE", "ACC-ghi-PROTOTYPE-env", true},
		{"valid hex", validHexSuffix2, false},
		{"standard", validStandardID2, false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsPrototypeAccount(tt.ref); got != tt.expected {
				t.Errorf("IsPrototypeAccount(%q) = %v; want %v", tt.ref, got, tt.expected)
			}
		})
	}
}

func TestIsPrototypeAccount_EmptyUnit(t *testing.T) {
	t.Parallel()
	if got := IsPrototypeAccount(""); got != false {
		t.Errorf("empty string should return false: got %v", got)
	}
	if got := IsPrototypeAccount("   \t\n  "); got != false {
		t.Errorf("whitespace only should return false: got %v", got)
	}
}

func TestValidateOwnerRefProduction_RejectUnit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		owner_ref   string
		errContains string
	}{
		{"known hash", hashTestUser2, "prototype/test"},
		{"known hash uppercase", strings.ToUpper(hashTestAgent2), "prototype/test"},
		{"proto suffix", "ACC-123-abcd-proto-env", "prototype/test"},
		{"human string", "human", "not a valid account ID"},
		{"no acc prefix", "1785920548450214015-3df55bd1", "not a valid account ID"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateOwnerRefProduction(tt.owner_ref)
			if err == nil {
				t.Errorf("expected error containing %q for %q", tt.errContains, tt.owner_ref)
				return
			}
			msg := err.Error()
			if !strings.Contains(msg, tt.errContains) {
				t.Errorf("ValidateOwnerRefProduction(%q) = %q; want to contain %q", tt.owner_ref, msg, tt.errContains)
			}
		})
	}
}

func TestValidateOwnerRefProduction_AcceptUnit(t *testing.T) {
	t.Parallel()
	if err := ValidateOwnerRefProduction(validHexSuffix2); err != nil {
		t.Errorf("valid account should accept: %v", err)
	}
	if err := ValidateOwnerRefProduction(""); err != nil {
		t.Errorf("empty should tolerate: %v", err)
	}
}

func TestValidateOwnerRefProduction_DeterministicErrorUnit(t *testing.T) {
	err := ValidateOwnerRefProduction(hashTestUser2)
	if err == nil {
		t.Fatal("expected error for prototype hash")
	}
	expected := "owner_ref \"ACC-1785920548450214015-3df55bd1\" is a prototype/test account and is not allowed in production"
	if err.Error() != expected {
		t.Errorf("got  %q\nwant %q", err.Error(), expected)
	}
}

func TestIsPrototypeAccount_BoundariesUnit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		ref      string
		expected bool
	}{
		{"no ACC prefix", "1785920548450214015-3df55bd1", false},
		{"trailing dash", "ACC-1234-", false},
		{"short no dash", "ACC-5", false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsPrototypeAccount(tt.ref); got != tt.expected {
				t.Errorf("IsPrototypeAccount(%q) = %v; want %v", tt.ref, got, tt.expected)
			}
		})
	}
}
