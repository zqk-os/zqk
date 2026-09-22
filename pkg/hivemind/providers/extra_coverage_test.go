// BLI-STARTER-COMMUNITY-053 / PRI-STARTER-COMMUNITY-053 coverage elevation
package providers_test

import (
	"context"
	"errors"
	"testing"

	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/hivemind"
	"github.com/zqk-os/zqk/pkg/hivemind/providers"
	"github.com/zqk-os/zqk/pkg/objects"
)

type extraMemConn struct {
	provider.GraphConnection
	vecRes  *provider.QueryResult
	vecErr  error
	qRes    *provider.QueryResult
	qErr    error
	travRes *provider.QueryResult
	travErr error
}

func (m extraMemConn) ExecuteVectorQuery(context.Context, provider.VectorQuery) (*provider.QueryResult, error) {
	return m.vecRes, m.vecErr
}
func (m extraMemConn) ExecuteQuery(context.Context, provider.Query) (*provider.QueryResult, error) {
	return m.qRes, m.qErr
}
func (m extraMemConn) ExecuteTraversal(context.Context, provider.TraversalQuery) (*provider.QueryResult, error) {
	return m.travRes, m.travErr
}

func TestExtraMemGraphMemoryStoreCoverage(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")

	okVec := extraMemConn{
		vecRes: &provider.QueryResult{Rows: []map[string]any{
			{objects.FieldKeyID: "m1", objects.FieldKeyScore: float32(0.4)},
		}},
	}
	store := providers.NewMemGraphMemoryStore(okVec)
	got, err := store.RetrieveSemantically(ctx, "q", 3)
	if err != nil || len(got) != 1 {
		t.Fatalf("semantic: %v %#v", err, got)
	}
	failVec := extraMemConn{vecErr: boom}
	if _, err := providers.NewMemGraphMemoryStore(failVec).RetrieveSemantically(ctx, "q", 1); err == nil {
		t.Fatal("semantic err")
	}

	okTrav := extraMemConn{travRes: &provider.QueryResult{
		Nodes: []*provider.Node{{ID: "n1", Properties: map[string]any{"k": "v"}}},
		Edges: []*provider.Edge{{FromID: "n1", ToID: "n2", Type: "REL", Properties: map[string]any{"p": 1}}},
	}}
	sg, err := providers.NewMemGraphMemoryStore(okTrav).RetrieveGraphContext(ctx, "n1", 2)
	if err != nil || sg == nil || len(sg.Nodes) != 1 || len(sg.Edges) != 1 {
		t.Fatalf("graph ctx: %v %#v", err, sg)
	}
	if _, err := providers.NewMemGraphMemoryStore(extraMemConn{travErr: boom}).RetrieveGraphContext(ctx, "n1", 1); err == nil {
		t.Fatal("graph ctx err")
	}

	if _, err := providers.NewMemGraphMemoryStore(failVec).QueryHybrid(ctx, "q", 0, hivemind.HybridConstraints{}); err == nil {
		t.Fatal("hybrid err")
	}
	empty, err := providers.NewMemGraphMemoryStore(extraMemConn{vecRes: &provider.QueryResult{}}).QueryHybrid(ctx, "q", 0, hivemind.HybridConstraints{})
	if err != nil || empty != nil {
		t.Fatalf("hybrid empty: %v %#v", err, empty)
	}
	hybridOK := extraMemConn{vecRes: &provider.QueryResult{Rows: []map[string]any{
		{"chunk_id": "c1", objects.FieldKeyScore: float64(0.9)},
		{"chunk_id": "c2", "parent_id": "p1", "parent_props": map[string]any{"k": 1}, objects.FieldKeyScore: float32(0.2)},
	}}}
	if _, err := providers.NewMemGraphMemoryStore(hybridOK).QueryHybrid(ctx, "q", -1, hivemind.HybridConstraints{}); err != nil {
		t.Fatal(err)
	}

	okQ := extraMemConn{qRes: &provider.QueryResult{}}
	if err := providers.NewMemGraphMemoryStore(okQ).Upsert(ctx, "id1", []float32{0.1}); err != nil {
		t.Fatal(err)
	}
	if err := providers.NewMemGraphMemoryStore(extraMemConn{qErr: boom}).Upsert(ctx, "id1", nil); err == nil {
		t.Fatal("upsert err")
	}
	if err := providers.NewMemGraphMemoryStore(extraMemConn{qErr: boom}).LinkVectorID(ctx, "id1", "v1"); err == nil {
		t.Fatal("link err")
	}
	if _, err := providers.NewMemGraphMemoryStore(extraMemConn{qErr: boom}).FindObjectsMissingVectors(ctx, 2); err == nil {
		t.Fatal("missing err")
	}
}
