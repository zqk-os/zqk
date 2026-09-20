package authcred

import (
	"strings"
	"testing"
)

// TestValidateOwnerRefProduction_CaseInsensitivePrefix tests the edge case where
// owner_ref uses lowercase "acc-" prefix instead of "ACC-".
func TestValidateOwnerRefProduction_CaseInsensitivePrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		owner_ref   string
		errContains string
	}{
		{
			name:        "lowercase acc with PROTO suffix",
			owner_ref:   "acc-1785920548450214016-a-PROTO-b",
			errContains: "prototype/test",
		},
		{
			name:        "lowercase acc with TEST suffix",
			owner_ref:   "acc-1785920548450214016-z-test-user",
			errContains: "prototype/test",
		},
		{
			name:        "mixed case ACC with prototype keyword",
			owner_ref:   "Acc-1785920548450214016-x-Prototype-y",
			errContains: "prototype/test",
		},
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

// TestValidateOwnerRefProduction_ValidLowercasePrefix tests that valid lowercase acc-
// owner_refs are correctly accepted.
func TestValidateOwnerRefProduction_ValidLowercasePrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		owner_ref string
	}{
		{
			name:      "valid lowercase acc hex suffix",
			owner_ref: "acc-1785920548450214015-deadbeef1234",
		},
		{
			name:      "valid mixed case ACC standard ID",
			owner_ref: "Acc-999",
		},
		{
			name:      "trimmed lowercase acc",
			owner_ref: "  acc-1785920548450214015-deadbeef1234  ",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateOwnerRefProduction(tt.owner_ref)
			if err != nil {
				t.Errorf("ValidateOwnerRefProduction(%q): unexpected error = %v", tt.owner_ref, err)
			}
		})
	}
}
