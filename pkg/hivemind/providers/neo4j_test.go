package providers_test

import (
	"context"
	"os"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/graph/memgraph"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/hivemind"
	"github.com/lanceman/zqk/pkg/hivemind/providers"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/testservices"
)

func TestMemGraphMemoryStore_Integration(t *testing.T) {
	if os.Getenv("GRAPH_ENABLED") != "true" {
		t.Skip("Skipping MemGraph integration test because GRAPH_ENABLED is not true")
	}

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 30*time.Second)
	defer cancel()

	services, err := testservices.SetupTestServices(ctx)
	if err != nil {
		t.Fatalf("failed to setup test services: %v", err)
	}
	t.Cleanup(func() { _ = services.Cleanup() })

	config := provider.ConnectionConfig{
		Host:     "127.0.0.1",
		Port:     7687,
		Username: "",
		Password: "",
	}

	mgProvider := memgraph.NewMemGraphProvider(&memgraph.MemGraphConfig{
		Host: "127.0.0.1",
		Port: 7687,
	})

	pool, err := mgProvider.CreatePool(ctx, config)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	conn, err := pool.GetConnection(ctx)
	if err != nil {
		t.Fatalf("failed to get connection: %v", err)
	}
	defer func() { _ = pool.ReturnConnection(conn) }()

	store := providers.NewMemGraphMemoryStore(conn)

	t.Run("Semantic retrieval empty", func(t *testing.T) {
		res, err := store.RetrieveSemantically(ctx, "test", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 0 {
			t.Errorf("expected empty result, got: %v", res)
		}
	})

	t.Run("Graph context retrieval empty", func(t *testing.T) {
		subgraph, err := store.RetrieveGraphContext(ctx, "1", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if subgraph == nil || len(subgraph.Nodes) != 0 {
			t.Errorf("expected empty subgraph, got: %v", subgraph)
		}
	})

	t.Run("Hybrid query empty results", func(t *testing.T) {
		// Test broad constraints
		res, err := store.QueryHybrid(ctx, "query", 1, hivemind.HybridConstraints{Kind: "other"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 0 {
			t.Errorf("expected empty result, got: %v", res)
		}

		// Test restrictive constraints
		res, err = store.QueryHybrid(ctx, "query", 1, hivemind.HybridConstraints{Kind: "policy"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 0 {
			t.Errorf("expected empty result, got: %v", res)
		}
	})
}

type mockVectorConn struct {
	provider.GraphConnection
	res *provider.QueryResult
	err error
}

func (m *mockVectorConn) ExecuteVectorQuery(ctx context.Context, query provider.VectorQuery) (*provider.QueryResult, error) {
	return m.res, m.err
}

func TestMemGraphMemoryStore_QueryHybrid_Unit(t *testing.T) {
	ctx := context.Background()

	mockConn := &mockVectorConn{
		res: &provider.QueryResult{
			Rows: []map[string]any{
				{
					"chunk_id":            "chunk1",
					"parent_id":           "macro1",
					"parent_props":        map[string]any{objects.FieldKeyKind: "policy"},
					objects.FieldKeyScore: float64(0.95), // standard float64 parsing
				},
				{
					"chunk_id":            "chunk2",
					"parent_id":           "macro1",
					"parent_props":        map[string]any{objects.FieldKeyKind: "policy"},
					objects.FieldKeyScore: float32(0.85), // standard float32 parsing
				},
				{
					"chunk_id":            "chunk3",
					"parent_id":           "macro2",
					"parent_props":        map[string]any{objects.FieldKeyKind: "rule"},
					objects.FieldKeyScore: "invalid", // invalid score logging path
				},
			},
		},
	}

	store := providers.NewMemGraphMemoryStore(mockConn)
	res, err := store.QueryHybrid(ctx, "query", 10, hivemind.HybridConstraints{Kind: "policy"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res) != 2 {
		t.Errorf("expected 2 macros returned from mock rows, got %d", len(res))
	}
}
