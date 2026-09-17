package storage

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestIsFieldUnset(t *testing.T) {
	if !IsFieldUnset(FieldUnset) {
		t.Error("FieldUnset sentinel should be recognized")
	}
	if IsFieldUnset("x") || IsFieldUnset(nil) || IsFieldUnset(struct{}{}) {
		t.Error("non-sentinel values must not match IsFieldUnset")
	}
}

func TestUnsetFieldKeys(t *testing.T) {
	got := UnsetFieldKeys(map[string]any{
		"keep":                      "v",
		objects.FieldKeyActiveOrder: FieldUnset,
	})
	if len(got) != 1 || got[0] != "active_order" {
		t.Fatalf("UnsetFieldKeys = %v, want [active_order]", got)
	}
	if UnsetFieldKeys(nil) != nil {
		t.Fatal("nil updates must return nil")
	}
}
