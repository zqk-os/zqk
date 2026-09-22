// BLI-STARTER-COMMUNITY-059 / PRI-STARTER-COMMUNITY-059 coverage elevation
package resolver

import (
	"context"
	"errors"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/provider"
)

type extraErrConn struct{ mockGraphConnection }

func (m *extraErrConn) ExecuteQuery(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
	return nil, errors.New("boom")
}

func TestExtraResolveErrorAndStringSlice(t *testing.T) {
	ctx := pkgctx.NewSystemContext()
	ok := NewReferenceResolver(&mockGraphConnection{})
	_ = ok.ResolveReferenceField(ctx, "refs", []string{"MIL-001", "MIL-999"})
	_ = ok.ResolveReferenceField(ctx, "other", 42)
	_ = ok.ResolveReferenceField(ctx, "mixed", []any{"MIL-001", 7, "MIL-999"})

	bad := NewReferenceResolver(&extraErrConn{})
	exists, err := bad.ResolveReference(ctx, "MIL-001")
	if err == nil || exists {
		t.Fatalf("expected query error, got exists=%v err=%v", exists, err)
	}
	got := bad.ResolveReferences(ctx, []string{"MIL-001", "GOAL-001"})
	if got["MIL-001"] || got["GOAL-001"] {
		t.Fatalf("expected unresolved on error: %v", got)
	}
	unresolved := bad.ResolveReferenceField(ctx, "milestone_ref", "MIL-001")
	if len(unresolved) != 1 {
		t.Fatalf("expected unresolved on error, got %v", unresolved)
	}
	_ = bad.ResolveReferenceField(ctx, "refs", []string{"MIL-001"})
	_ = bad.ResolveReferenceField(ctx, "arr", []any{"MIL-001"})
}
