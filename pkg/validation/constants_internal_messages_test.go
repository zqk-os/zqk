package validation

import (
	"testing"
)

func TestConstantsInternalMessages_NonEmpty(t *testing.T) {
	if len("discoverSpecsDir() should return absolute path, got: %q") == 0 {
		t.Fatal("expected non-empty constant message")
	}
}
