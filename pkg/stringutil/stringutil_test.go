package stringutil

import (
	"testing"
)

func TestStringUtilForwarding(t *testing.T) {
	if err := ErrorOnBlank("", "blank error"); err == nil || err.Error() != "blank error" {
		t.Fatalf("expected 'blank error', got %v", err)
	}

	if err := ErrorOnBlank("ok", "blank error"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if err := RequireNonBlank("   ", "token"); err == nil || err.Error() != "token cannot be blank" {
		t.Fatalf("expected 'token cannot be blank', got %v", err)
	}

	if !IsBlank("   ") {
		t.Errorf("expected whitespace to be blank")
	}

	if !IsNonBlank("hello") {
		t.Errorf("expected hello to be non-blank")
	}
}
