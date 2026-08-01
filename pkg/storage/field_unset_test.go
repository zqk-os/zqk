package storage

import (
	"testing"
)

func TestIsFieldUnset(t *testing.T) {
	if !IsFieldUnset(FieldUnset) {
		t.Error("FieldUnset sentinel should be recognized")
	}
	if IsFieldUnset("x") || IsFieldUnset(nil) || IsFieldUnset(struct{}{}) {
		t.Error("non-sentinel values must not match IsFieldUnset")
	}
}
