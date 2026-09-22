// BLI-STARTER-COMMUNITY-042 / PRI-STARTER-COMMUNITY-042 coverage elevation
package resolver

import (
	"context"
	"errors"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/provider"
)

type extraErrConn struct {
	mockGraphConnection
}

func (extraErrConn) ExecuteQuery(context.Context, provider.Query) (*provider.QueryResult, error) {
	return nil, errors.New("boom")
}

func TestExtraResolveErrorsAndStringSlice(t *testing.T) {
	ctx := pkgctx.NewSystemContext()
	ok := NewReferenceResolver(&mockGraphConnection{})
	got := ok.ResolveReferenceField(ctx, "refs", []string{"MIL-001", "MIL-999"})
	if len(got) != 1 || got[0] != "MIL-999" {
		t.Fatalf("string slice = %#v", got)
	}
	if unresolved := ok.ResolveReferenceField(ctx, "n", 42); len(unresolved) != 0 {
		t.Fatalf("other type = %#v", unresolved)
	}
	if unresolved := ok.ResolveReferenceField(ctx, "n", []any{1, "MIL-999"}); len(unresolved) != 1 {
		t.Fatalf("mixed any = %#v", unresolved)
	}

	fail := NewReferenceResolver(&extraErrConn{})
	exists, err := fail.ResolveReference(ctx, "MIL-001")
	if err == nil || exists {
		t.Fatalf("query err = %v %v", exists, err)
	}
	results := fail.ResolveReferences(ctx, []string{"MIL-001"})
	if results["MIL-001"] {
		t.Fatal("error should mark unresolved")
	}
	if unresolved := fail.ResolveReferenceField(ctx, "r", "MIL-001"); len(unresolved) != 1 {
		t.Fatalf("field err = %#v", unresolved)
	}
}
