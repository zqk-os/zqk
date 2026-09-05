package audit

import "testing"

func TestNormalizeEventTypeAllowed(t *testing.T) {
	t.Parallel()
	allowed := map[string]struct{}{"object_creation": {}}
	got, md := NormalizeEventType("object_creation", nil, allowed, "system_config_change")
	if got != "object_creation" {
		t.Fatalf("got %q", got)
	}
	if _, ok := md[MetadataKeyOriginalEventType]; ok {
		t.Fatal("should not record original for allowed type")
	}
}

func TestNormalizeEventTypeFallback(t *testing.T) {
	t.Parallel()
	allowed := map[string]struct{}{"object_creation": {}}
	got, md := NormalizeEventType("not_a_type", nil, allowed, "system_config_change")
	if got != "system_config_change" {
		t.Fatalf("got %q", got)
	}
	if md[MetadataKeyOriginalEventType] != "not_a_type" {
		t.Fatalf("original = %v", md[MetadataKeyOriginalEventType])
	}
}

func TestNormalizeEventTypeNoFallback(t *testing.T) {
	t.Parallel()
	got, _ := NormalizeEventType("keep_me", nil, map[string]struct{}{}, "")
	if got != "keep_me" {
		t.Fatalf("got %q", got)
	}
}
