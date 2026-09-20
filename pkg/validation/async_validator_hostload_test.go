package validation

import (
	"testing"
	"time"
)

func TestHostloadValidationYield_Positive(t *testing.T) {
	t.Parallel()
	if hostloadValidationYield != 50*time.Millisecond {
		t.Fatalf("got %s", hostloadValidationYield)
	}
}
