package scheduler

import (
	"testing"
)

func TestScopeFingerprintSHA256FromFingerprintMap_Deterministic(t *testing.T) {
	m := map[string]string{"b": "pass", "a": "fail"}
	h1 := ScopeFingerprintSHA256FromFingerprintMap(m)
	h2 := ScopeFingerprintSHA256FromFingerprintMap(map[string]string{"a": "fail", "b": "pass"})
	if h1 != h2 {
		t.Fatalf("order of map should not matter: %s vs %s", h1, h2)
	}
	empty := ScopeFingerprintSHA256FromFingerprintMap(nil)
	if len(empty) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(empty))
	}
}

func TestComputeTombstoneDisparity_NoBefore(t *testing.T) {
	d := ComputeTombstoneDisparity(nil, map[string]any{"scope_fingerprint_sha256": "x"})
	if d["active_tombstone"] != false {
		t.Fatalf("expected no tombstone: %#v", d)
	}
}

func TestComputeTombstoneDisparity_Match(t *testing.T) {
	shared := map[string]any{
		"scope_fingerprint_sha256": "abc",
		"health_watermark_rfc3339": "2025-01-01T00:00:00Z",
	}
	d := ComputeTombstoneDisparity(shared, shared)
	if d["active_tombstone"] != true || d["scope_fingerprint_match"] != true || d["watermark_match"] != true {
		t.Fatalf("expected match: %#v", d)
	}
}

func TestComputeTombstoneDisparity_ScopeDrift(t *testing.T) {
	before := map[string]any{
		"scope_fingerprint_sha256": "aaa",
		"health_watermark_rfc3339": "2025-01-01T00:00:00Z",
	}
	after := map[string]any{
		"scope_fingerprint_sha256": "bbb",
		"health_watermark_rfc3339": "2025-01-01T00:00:00Z",
	}
	d := ComputeTombstoneDisparity(before, after)
	if d["scope_fingerprint_match"] != false {
		t.Fatalf("expected scope drift: %#v", d)
	}
}
