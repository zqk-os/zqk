// BLI-STARTER-COMMUNITY-041 / PRI-STARTER-COMMUNITY-041 coverage elevation
package bridge

import (
	"context"
	"testing"
)

func TestExtraGetStructuralContextNotImplemented(t *testing.T) {
	b := NewMemoryOntologyBridge(nil, nil)
	got, err := b.GetStructuralContext(context.Background(), "sem-1")
	if err == nil || got != nil {
		t.Fatalf("expected not implemented, got %#v %v", got, err)
	}
}
