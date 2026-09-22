// BLI-STARTER-COMMUNITY-030 / PRI-STARTER-COMMUNITY-030 coverage elevation
package objectrecord

import (
	"context"
	"testing"
)

func TestWithRecorder_NilGuards(t *testing.T) {
	t.Parallel()
	rec := &fakeRecorder{}
	if WithRecorder(nil, rec) != nil {
		t.Fatal("nil ctx")
	}
	ctx := context.Background()
	if WithRecorder(ctx, nil) != ctx {
		t.Fatal("nil recorder should return same ctx")
	}
	if FromContext(nil) != nil {
		t.Fatal("nil from")
	}
}
