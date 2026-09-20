package llm

import "testing"

func TestIsMockFallback(t *testing.T) {
	t.Parallel()

	if !IsMockFallback("[mock-fallback] structured completion unavailable") {
		t.Fatal("expected static mock response to be detected")
	}
	if IsMockFallback("real model completion") {
		t.Fatal("real completion must not be classified as mock fallback")
	}
}
