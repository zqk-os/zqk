package meshbroker

import (
	"strings"
	"testing"
)

func TestNewFederationLeaseToken_CryptoQuality(t *testing.T) {
	t.Parallel()

	a, err := newFederationLeaseToken()
	if err != nil {
		t.Fatalf("newFederationLeaseToken: %v", err)
	}
	b, err := newFederationLeaseToken()
	if err != nil {
		t.Fatalf("newFederationLeaseToken: %v", err)
	}
	if !strings.HasPrefix(a, "TOK-") || !strings.HasPrefix(b, "TOK-") {
		t.Fatalf("tokens missing TOK- prefix: %q %q", a, b)
	}
	if a == b {
		t.Fatalf("consecutive tokens collided: %q", a)
	}
	// 16 random bytes → 32 hex chars after TOK-
	if got := len(a); got != len("TOK-")+32 {
		t.Fatalf("token length = %d, want %d (%q)", got, len("TOK-")+32, a)
	}
}
