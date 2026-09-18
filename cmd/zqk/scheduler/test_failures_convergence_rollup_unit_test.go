package scheduler

import "testing"

func TestCountGoPathGrepHitLines(t *testing.T) {
	t.Parallel()
	raw := "pkg/foo/bar.go:12:\tcode\n=== A) section ===\n"
	if n := countGoPathGrepHitLines([]byte(raw)); n != 1 {
		t.Fatalf("got %d want 1", n)
	}
	if n := countGoPathGrepHitLines(nil); n != 0 {
		t.Fatalf("nil: got %d", n)
	}
}
