package objects

import (
	"strings"
	"testing"
)

func TestResolveAndValidateKindForProject_EmptyArg(t *testing.T) {
	_, err := ResolveAndValidateKindForProject("/tmp", "   ")
	if err == nil {
		t.Fatal("expected error for empty kind after trim")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty-kind message, got %v", err)
	}
}
