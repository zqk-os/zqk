package hivemind_test

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/hivemind"
	"github.com/zqk-os/zqk/pkg/objects"
)

// MockGraphConnection implements provider.GraphConnection for testing
type MockGraphConnection struct {
	provider.GraphConnection
	LastQuery provider.VectorQuery
}

func (m *MockGraphConnection) ExecuteVectorQuery(ctx context.Context, query provider.VectorQuery) (*provider.QueryResult, error) {
	m.LastQuery = query

	return &provider.QueryResult{
		Rows: []map[string]any{
			{objects.FieldKeyID: "1", "similarity": 0.9, objects.FieldKeyKind: "test_kind"},
			{objects.FieldKeyID: "2", "similarity": 0.8, objects.FieldKeyKind: "other_kind"},
			{objects.FieldKeyID: "3", "similarity": 0.7, objects.FieldKeyKind: "test_kind"},
		},
	}, nil
}

func TestDynamicQueryRouter_HighSelectivity(t *testing.T) {
	conn := &MockGraphConnection{}
	router := hivemind.NewDynamicQueryRouter(conn, hivemind.RouterConfig{})

	ctx := context.Background()
	vector := []float32{0.1, 0.2}
	constraints := hivemind.HybridConstraints{Kind: "test_kind"}

	results, err := router.RouteHybridQuery(ctx, vector, 5, constraints)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if conn.LastQuery.Query == "" {
		t.Error("expected Cypher query for high selectivity, got empty query")
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results after post-filtering, got %d", len(results))
	}
}

func TestDynamicQueryRouter_BroadConstraints(t *testing.T) {
	conn := &MockGraphConnection{}
	router := hivemind.NewDynamicQueryRouter(conn, hivemind.RouterConfig{OversampleFactor: 3})

	ctx := context.Background()
	vector := []float32{0.1, 0.2}
	constraints := hivemind.HybridConstraints{} // Broad constraint

	results, err := router.RouteHybridQuery(ctx, vector, 2, constraints)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if conn.LastQuery.Query != "" {
		t.Errorf("expected empty query for broad constraints to use index, got %q", conn.LastQuery.Query)
	}

	if conn.LastQuery.Limit != 6 {
		t.Errorf("expected limit to be oversampled to 6, got %d", conn.LastQuery.Limit)
	}

	if len(results) != 2 {
		t.Errorf("expected limit of 2 results to be respected, got %d", len(results))
	}
}
