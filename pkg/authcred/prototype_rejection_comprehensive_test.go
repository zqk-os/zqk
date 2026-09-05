package authcred

import (
	"testing"
)

// TestPrototypeRejection_Comprehensive covers all rejection and acceptance vectors
// to ensure ACC with prototype/test owner_ref is properly rejected on production.

func TestPrototypeRejection_KnownHashesComprehensive(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		ref      string
		isProto  bool
		hasError bool
	}{
		// Known hashes - definite prototype accounts
		{"known test-user exact", "ACC-1785920548450214015-3df55bd1", true, true},
		{"known test-agent exact", "ACC-1785920548450214016-ace2aae1", true, true},
		{"known test-user uppercase", "ACC-1785920548450214015-3DF55BD1", true, true},
		{"known test-agent uppercase", "ACC-1785920548450214016-ACE2AAE1", true, true},
		// Suffix detection - prototype/test keywords
		{"proto in suffix", "ACC-abcd-proto-env", true, true},
		{"test in suffix", "ACC-efgh-test-user", true, true},
		{"PROTOTYPE uppercase", "ACC-ijkl-PROTOTYPE-z", true, true},
		{"Proto mixed case", "ACC-mnop-Proto-qry", true, true},
		// Valid accounts - should NOT be rejected
		{"valid hex suffix", "ACC-1785920548450214015-deadbeef1234", false, false},
		{"valid standard ID", "ACC-999", false, false},
		{"valid numeric", "ACC-123456789", false, false},
		// Edge cases - non-ACC prefixes are not caught by IsPrototypeAccount (no ACC prefix to match).
		// ValidateOwnerRefProduction only rejects prototype/test; format check is out of scope.
		{"no prefix", "1785920548450214015-3df55bd1", false, true},
		{"human", "human", false, true}, // No ACC prefix => not detected as prototype => no validation error
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsPrototypeAccount(tt.ref); got != tt.isProto {
				t.Errorf("IsPrototypeAccount(%q) = %v; want %v", tt.ref, got, tt.isProto)
			}
			err := ValidateOwnerRefProduction(tt.ref)
			hasErr := err != nil
			if hasErr != tt.hasError {
				t.Errorf("ValidateOwnerRefProduction(%q) error = %v; want error=%v", tt.ref, err, tt.hasError)
			}
		})
	}
}

func TestPrototypeRejection_ErrorFormatDeterministic(t *testing.T) {
	t.Parallel()
	tests := []string{
		"ACC-1785920548450214015-3df55bd1",
		"ACC-1785920548450214016-ace2aae1",
		"ACC-abcd-proto-env",
	}
	for _, ref := range tests {
		t.Run(ref, func(t *testing.T) {
			err := ValidateOwnerRefProduction(ref)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", ref)
			}
			msg := err.Error()
			expected := "owner_ref \"" + ref + "\" is a prototype/test account and is not allowed in production"
			if msg != expected {
				t.Errorf("got  %q\nwant %q", msg, expected)
			}
		})
	}
}

func TestPrototypeRejection_EmptyAndWhitespace(t *testing.T) {
	t.Parallel()
	for _, ref := range []string{"", "   ", "\t", "\n"} {
		err := ValidateOwnerRefProduction(ref)
		if err != nil {
			t.Errorf("expected no error for %q, got: %v", ref, err)
		}
	}
	for _, ref := range []string{"", "   ", "\t", "\n"} {
		if IsPrototypeAccount(ref) {
			t.Errorf("IsPrototypeAccount(%q) should be false for empty/whitespace", ref)
		}
	}
}

func TestPrototypeRejection_CaseInsensitiveACC(t *testing.T) {
	t.Parallel()
	tests := []struct {
		ref       string
		expectErr bool
	}{
		{"acc-1785920548450214016-x-proto-y", true},     // lowercase acc, proto suffix
		{"ACC-1785920548450214016-z-test-user", true},   // uppercase ACC, test suffix
		{"Acc-1785920548450214016-x-Prototype-y", true}, // mixed case, Prototype suffix
		{"acc-1785920548450214015-deadbeef1234", false}, // lowercase acc, valid hex
		{"Acc-999", false}, // mixed case, valid standard
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			err := ValidateOwnerRefProduction(tt.ref)
			hasErr := err != nil
			if hasErr != tt.expectErr {
				t.Errorf("ValidateOwnerRefProduction(%q) error = %v; want error=%v", tt.ref, err, tt.expectErr)
			}
		})
	}
}

func TestPrototypeRejection_BoundaryCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		ref       string
		expectErr bool
	}{
		{"ACC-1234-", false},                // trailing dash, no meaningful suffix
		{"ACC-5", false},                    // too short for meaningful detection
		{"1785920548450214015-proto", true}, // non-ACC prefix => not proto detected by IsPrototypeAccount
		{"acc-abcd-proto-env", true},        // lowercase with proto
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			err := ValidateOwnerRefProduction(tt.ref)
			hasErr := err != nil
			if hasErr != tt.expectErr {
				t.Errorf("ValidateOwnerRefProduction(%q) error = %v; want error=%v", tt.ref, err, tt.expectErr)
			}
		})
	}
}
