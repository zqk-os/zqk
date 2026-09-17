package nildecode

import "testing"

func TestDecodeNonNilPayloadPointer(t *testing.T) {
	var p *struct{ X int }
	if _, ok := DecodeNonNilPayload[*struct{ X int }](any(p)); ok {
		t.Fatal("nil pointer should fail")
	}
	s := &struct{ X int }{X: 1}
	got, ok := DecodeNonNilPayload[*struct{ X int }](any(s))
	if !ok || got != s {
		t.Fatalf("got %v ok=%v", got, ok)
	}
}

func TestDecodeNonNilPayloadAny(t *testing.T) {
	if _, ok := DecodeNonNilPayload[any](nil); ok {
		t.Fatal("nil any should fail")
	}
	if _, ok := DecodeNonNilPayload[any]("x"); !ok {
		t.Fatal("non-empty string in any should succeed")
	}
}
