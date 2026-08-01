package specorigination

import (
	"strings"
	"testing"
)

func TestTruncateRunes(t *testing.T) {
	t.Parallel()
	if got := truncateRunes("hello", 10); got != "hello" {
		t.Fatalf("got %q", got)
	}
	long := strings.Repeat("a", 3000)
	got := truncateRunes(long, 10)
	if len([]rune(got)) != 11 { // 10 + ellipsis
		t.Fatalf("expected 11 runes, got %d: %q", len([]rune(got)), got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis suffix: %q", got)
	}
}
