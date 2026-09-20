package validation

import (
	"testing"
)

func TestConstantsInternalMessages_NonEmpty(t *testing.T) {
	if len(ConstMagica1b6c793) == 0 {
		t.Fatal("expected non-empty constant message")
	}
}
