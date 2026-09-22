// BLI-STARTER-COMMUNITY-042 / PRI-STARTER-COMMUNITY-042 coverage elevation
package impl

import (
	"context"
	"testing"
)

func TestExtraGetStructuralContextNotImplemented(t *testing.T) {
	b := NewBridge(nil, nil)
	got, err := b.GetStructuralContext(context.Background(), "sem-1")
	if err == nil || got != nil {
		t.Fatalf("expected not implemented, got %#v %v", got, err)
	}
}
