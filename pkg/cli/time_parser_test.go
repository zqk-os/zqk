package cli

import (
	"testing"
)

func TestResolveTimeTokens(t *testing.T) {
	now := ResolveTimeTokens("now")
	if str, ok := now.(string); !ok || str == "" {
		t.Errorf("expected string for 'now', got %v", now)
	}

	nonToken := ResolveTimeTokens("sample-token")
	if nonToken != "sample-token" {
		t.Errorf("expected 'sample-token' unchanged, got %v", nonToken)
	}
}
