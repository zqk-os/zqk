package authcred_test

import (
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/authcred"
	"github.com/lanceman/zqk/pkg/objects"
)

// testACCs holds account constants shared across tests.
var testACCs = struct {
	hashTestUser  string
	hashTestAgent string
	validHex      string
	validStandard string
}{
	hashTestUser:  "ACC-1785920548450214015-3df55bd1",
	hashTestAgent: "ACC-1785920548450214016-ace2aae1",
	validHex:      "ACC-1785920548450214015-deadbeef1234",
	validStandard: "ACC-999",
}

const (
	errPrototype     = "prototype/test"
	emptyStr         = ""
	errEmptyExpected = "expected error containing %q but got nil"
)

// -------------------------------------------------------------------
// TestIsPrototypeAccount_KnownHashes - exact hash matching.
// -------------------------------------------------------------------

func TestIsPrototypeAccount_KnownHashes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		ref      string
		expected bool
	}{
		{"known hash test-user lowercase", testACCs.hashTestUser, true},
		{"known hash test-agent uppercase", strings.ToUpper(testACCs.hashTestAgent), true},
		{"valid non-prototype account", testACCs.validHex, false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := authcred.IsPrototypeAccount(tt.ref); got != tt.expected {
				t.Errorf("IsPrototypeAccount(%q) = %v; want %v", tt.ref, got, tt.expected)
			}
		})
	}
}

// -------------------------------------------------------------------
// TestIsPrototypeAccount_SuffixDetection - hash-suffix keywords.
// -------------------------------------------------------------------

func TestIsPrototypeAccount_SuffixDetection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		ref      string
		expected bool
	}{
		{"lowercase proto", "ACC-1785920548450214016-a-proto-b", true},
		{"uppercase TEST", "ACC-1785920548450214017-c-TEST-d", true},
		{"mixed PROTOTYPE-test", "ACC-1785920548450214018-e-PROTO-test", true},
		{"valid hex suffix", testACCs.validHex, false},
		{"standard ACC no hint", testACCs.validStandard, false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := authcred.IsPrototypeAccount(tt.ref); got != tt.expected {
				t.Errorf("IsPrototypeAccount(%q) = %v; want %v", tt.ref, got, tt.expected)
			}
		})
	}
}

// -------------------------------------------------------------------
// TestIsPrototypeAccount_EmptyEdgeCases - empty/whitespace.
// -------------------------------------------------------------------

func TestIsPrototypeAccount_EmptyEdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		ref      string
		expected bool
	}{
		{"empty string", emptyStr, false},
		{"whitespace only", "   \t\n  ", false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := authcred.IsPrototypeAccount(tt.ref); got != tt.expected {
				t.Errorf("IsPrototypeAccount(%q) = %v; want %v", tt.ref, got, tt.expected)
			}
		})
	}
}

// -------------------------------------------------------------------
// TestValidateOwnerRefProduction_Rejections - all rejection vectors.
// -------------------------------------------------------------------

func TestValidateOwnerRefProduction_Rejections(t *testing.T) {
	t.Parallel()
	rejectTests := []struct {
		name        string
		owner_ref   string
		errContains string
	}{
		{"known hash test-user", testACCs.hashTestUser, errPrototype},
		{"known hash test-agent lowercase", testACCs.hashTestAgent, errPrototype},
		{"known hash test-agent uppercase", strings.ToUpper(testACCs.hashTestAgent), errPrototype},
		{"suffix proto", "ACC-1785920548450214016-x-proto-y", errPrototype},
		{"suffix test", "ACC-1785920548450214016-z-test-user", errPrototype},
		{"suffix PROTOTYPE", "ACC-1785920548450214017-a-PROTOTYPE-b", errPrototype},
	}
	for _, tt := range rejectTests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := authcred.ValidateOwnerRefProduction(tt.owner_ref)
			if err == nil {
				t.Errorf(errEmptyExpected, tt.owner_ref)
				return
			}
			msg := err.Error()
			if !strings.Contains(msg, tt.errContains) {
				t.Errorf("ValidateOwnerRefProduction(%q) = %q; want to contain %q", tt.owner_ref, msg, tt.errContains)
			}
		})
	}
}

// -------------------------------------------------------------------
// TestValidateOwnerRefProduction_Acceptances - valid accounts.
// -------------------------------------------------------------------

func TestValidateOwnerRefProduction_Acceptances(t *testing.T) {
	t.Parallel()
	acceptTests := []struct {
		name      string
		owner_ref string
	}{
		{"valid hex suffix", testACCs.validHex},
		{"standard ACC ID", testACCs.validStandard},
		{"empty tolerance", emptyStr},
		{"whitespace-prefixed", "  ACC-1785920548450214015-deadbeef1234  "},
	}
	for _, tt := range acceptTests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := authcred.ValidateOwnerRefProduction(tt.owner_ref)
			if err != nil {
				t.Errorf("ValidateOwnerRefProduction(%q): unexpected error = %v", tt.owner_ref, err)
			}
		})
	}
}

// -------------------------------------------------------------------
// TestValidateOwnerRefProduction_DeterministicError - exact format.
// -------------------------------------------------------------------

func TestValidateOwnerRefProduction_DeterministicError(t *testing.T) {
	err := authcred.ValidateOwnerRefProduction(testACCs.hashTestUser)
	if err == nil {
		t.Fatal("expected error for known prototype hash, got nil")
	}
	expected := "owner_ref \"ACC-1785920548450214015-3df55bd1\" is a prototype/test account and is not allowed in production"
	if err.Error() != expected {
		t.Errorf("error message mismatch:\ngot  %q\nwant %q", err.Error(), expected)
	}
}

// -------------------------------------------------------------------
// TestValidateOwnerRefProduction_Integration - object-level scenario.
// -------------------------------------------------------------------

func TestValidateOwnerRefProduction_Integration(t *testing.T) {
	t.Parallel()

	protoObj := map[string]any{
		objects.FieldKeyID:          "BLI-PROTOTYPE-INTEG",
		objects.FieldKeyKind:        "backlog_item",
		objects.FieldKeyOwnerRef:    testACCs.hashTestUser,
		objects.FieldKeyStatus:      objects.ObjectStatusDraft,
		objects.FieldKeyNamespaceID: "zqk:kernel",
	}

	t.Run("prototype hash is rejected", func(t *testing.T) {
		ref := protoObj[objects.FieldKeyOwnerRef].(string)
		err := authcred.ValidateOwnerRefProduction(ref)
		if err == nil {
			t.Errorf("expected rejection for prototype account %q, got nil", ref)
			return
		}
		protoErr := "prototype/test" // reuse constant pattern via hardcoded check
		if !strings.Contains(err.Error(), protoErr) {
			t.Errorf("error should contain 'prototype/test': %v", err)
		}
	})

	validObj := map[string]any{
		objects.FieldKeyID:       "BLI-VALID-INTEG",
		objects.FieldKeyKind:     "workstream",
		objects.FieldKeyOwnerRef: testACCs.validHex,
	}

	t.Run("valid production account passes", func(t *testing.T) {
		ref := validObj[objects.FieldKeyOwnerRef].(string)
		err := authcred.ValidateOwnerRefProduction(ref)
		if err != nil {
			t.Errorf("expected acceptance for %q, got: %v", ref, err)
		}
	})

	t.Run("empty owner_ref is tolerated", func(t *testing.T) {
		err := authcred.ValidateOwnerRefProduction("")
		if err != nil {
			t.Errorf("empty owner_ref should be tolerated: %v", err)
		}
	})
}

// -------------------------------------------------------------------
// TestIsPrototypeAccount_Boundaries - edge-case structure checks.
// -------------------------------------------------------------------

func TestIsPrototypeAccount_Boundaries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		ref      string
		expected bool
	}{
		{"no ACC prefix", "1785920548450214015-3df55bd1", false},
		{"ACC with trailing dash only", "ACC-1234-", false},
		{"short ACC no internal dash", "ACC-5", false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := authcred.IsPrototypeAccount(tt.ref); got != tt.expected {
				t.Errorf("IsPrototypeAccount(%q) = %v; want %v", tt.ref, got, tt.expected)
			}
		})
	}
}
