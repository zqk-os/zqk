package bldr_v2

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestOccupancyFieldConstantsMatchFieldKeys(t *testing.T) {
	t.Parallel()
	if FieldClaimedBy != objects.FieldKeyClaimedBy {
		t.Fatalf("FieldClaimedBy=%q want %q", FieldClaimedBy, objects.FieldKeyClaimedBy)
	}
	if FieldClaimedAt != objects.FieldKeyClaimedAt {
		t.Fatalf("FieldClaimedAt=%q want %q", FieldClaimedAt, objects.FieldKeyClaimedAt)
	}
}
