package memgraph

import (
	"context"
	"fmt"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/objects"
)

// mockTx implements provider.GraphTransaction for benchmarking
type mockTx struct {
	provider.GraphTransaction
	queryCount int
}

func (m *mockTx) ExecuteQuery(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
	m.queryCount++
	return &provider.QueryResult{}, nil
}

func (m *mockTx) Commit(ctx context.Context) error                         { return nil }
func (m *mockTx) Rollback(ctx context.Context) error                       { return nil }
func (m *mockTx) CreateNode(ctx context.Context, node provider.Node) error { return nil }
func (m *mockTx) CreateEdge(ctx context.Context, edge provider.Edge) error { return nil }

func BenchmarkExecuteBatch_Chunking(b *testing.B) {
	conn := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
		boltClient:     &boltClient{}, // Dummy client to pass nil check
	}

	// Inject a mock transaction
	mtx := &mockTx{}
	conn.SetOpenTransaction(mtx)

	ctx := pkgctx.NewSystemContext()

	// Prepare 10,000 nodes
	numNodes := 10000
	operations := make([]provider.Operation, 0, numNodes)
	for i := 0; i < numNodes; i++ {
		operations = append(operations, provider.Operation{
			Type: "create_node",
			Data: provider.Node{
				ID:     fmt.Sprintf("node-%d", i),
				Labels: []string{"PerfNode"},
				Properties: map[string]any{
					objects.FieldKeyName: i,
				},
			},
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mtx.queryCount = 0
		result, err := conn.ExecuteBatch(ctx, operations)
		if err != nil {
			b.Fatalf("ExecuteBatch failed: %v", err)
		}
		if result.SuccessCount != numNodes {
			b.Fatalf("Expected %d successes, got %d", numNodes, result.SuccessCount)
		}
		if mtx.queryCount != 1 {
			b.Logf("Expected 1 query (chunked), got %d. Fallback count? successCount=%d", mtx.queryCount, result.SuccessCount)
		}
	}
}

func TestExecuteBatch_RecordsBatchMetrics(t *testing.T) {
	t.Parallel()
	collector := provider.GetGlobalGraphProviderMetricsCollector()
	collector.Reset()

	conn := &memgraphConnection{
		BaseConnection: &provider.BaseConnection{},
		boltClient:     &boltClient{},
	}
	mtx := &mockTx{}
	conn.SetOpenTransaction(mtx)

	ctx := pkgctx.NewSystemContext()
	ops := []provider.Operation{
		{
			Type: "create_node",
			Data: provider.Node{ID: "n1", Labels: []string{"Node"}},
		},
		{
			Type: "create_node",
			Data: provider.Node{ID: "n2", Labels: []string{"Node"}},
		},
	}

	res, err := conn.ExecuteBatch(ctx, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.SuccessCount != 2 {
		t.Fatalf("expected 2 successes, got %d", res.SuccessCount)
	}

	snapshot := collector.GetMetrics()
	if snapshot.Batch.TotalBatches < 1 {
		t.Errorf("expected at least 1 batch recorded, got %d", snapshot.Batch.TotalBatches)
	}
	if snapshot.Batch.TotalItems < 2 {
		t.Errorf("expected at least 2 items recorded, got %d", snapshot.Batch.TotalItems)
	}
}
